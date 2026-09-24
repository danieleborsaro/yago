package concourse

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yamlparser "github.com/danieleborsaro/yago/internal/parser"
	gitrepo "github.com/danieleborsaro/yago/internal/repo"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// Default YAML paths used when schema x-gitops-paths entries are not available.
const (
	defaultPathPipelineInstances = "concourse.pipelines.instances"
	defaultPathGlobalConfig      = "concourse.pipelines.configuration.global"
	defaultPathInstanceConfig    = "concourse.pipelines.configuration.instance"
	defaultPathSlavePipelines    = "desiredstate.meta.master_pipeline.slaves"
	defaultPathPipelinesRepo     = "desiredstate.content.components.sourcecode.pipelines"
)

// Parser extends BaseParser for concourse-specific parsing operations.
type Parser struct {
	*wrapper.BaseParser

	// Concourse-specific fields
	configWorkdir           string
	pipelinesWorkdir        string
	pipelines               []*Pipeline
	parserList              []*Parser
	isUpdatingSlavesNotSelf bool
	globalConfig            map[string]interface{}
	globalConfigPath        string
	instanceConfigPath      string
	handler                 *yamlparser.YAMLHandler
}

// NewParser creates a new concourse parser instance.
func NewParser(env string, envVars map[string]string) *Parser {
	return &Parser{
		BaseParser: wrapper.NewBaseParser(env, envVars),
		handler:    yamlparser.NewYAMLHandler("."),
	}
}

// LoadGitOpsFiles implements wrapper.Parser and delegates to LoadGitOpsFilesExtended
// with isLoadPipelines=false (basic load without pipeline templates).
func (p *Parser) LoadGitOpsFiles(req wrapper.LoadRequest) error {
	return p.LoadGitOpsFilesExtended(req.GetDesiredStateRoot(), req.GetConfigRoot(), "", "", false)
}

// Cache implements wrapper.Parser via the embedded BaseParser.
func (p *Parser) Cache(req wrapper.CacheRequest) (*wrapper.CacheResponse, error) {
	return p.BaseParser.Cache(req)
}

// Clear implements wrapper.Parser via the embedded BaseParser.
func (p *Parser) Clear() error {
	return p.BaseParser.Clear()
}

// LoadGitOpsFilesExtended loads GitOps files with all concourse-specific parameters.
//
//   - dsRoot: path to desiredstate root file (required)
//   - cfgRoot: path to concourse configuration root file (optional; cloned from DS if empty)
//   - configWorkdir: directory where configuration repo is cloned/loaded when cfgRoot is empty (optional)
//   - pipelinesWorkdir: directory containing pipeline repo/templates (optional override)
//   - isLoadPipelines: if true, also resolve and load pipeline instances + templates
func (p *Parser) LoadGitOpsFilesExtended(dsRoot, cfgRoot, configWorkdir, pipelinesWorkdir string, isLoadPipelines bool) error {
	logging.Debug("Loading concourse GitOps files — ds=%s cfg=%s pipelines=%s loadPipelines=%v",
		dsRoot, cfgRoot, pipelinesWorkdir, isLoadPipelines)

	// 1. Load desiredstate
	ds := wrapper.NewBaseDesiredState(p.GetEnvironment(), "concourse", p.GetEnvVariables())
	if err := ds.LoadGitOpsFile(dsRoot); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate from %s", dsRoot)
	}
	p.SetDesiredState(ds)
	p.handler = yamlparser.NewYAMLHandler(ds.GetDocument().GetWorkdir())
	p.configWorkdir = configWorkdir
	p.pipelinesWorkdir = pipelinesWorkdir

	logging.Debug("Desiredstate loaded from: %s", dsRoot)

	// 2. Load configuration
	schemaVersionOverride := ds.GetSchemaVersion()

	if cfgRoot != "" {
		cfg := wrapper.NewBaseConfiguration(p.GetEnvironment(), "concourse", p.GetEnvVariables())
		cfg.SetMetaFile(cfgRoot)
		if err := cfg.LoadGitOpsFile(schemaVersionOverride); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to load configuration from %s", cfgRoot)
		}
		p.SetConfiguration(cfg)
		logging.Debug("Configuration loaded from: %s", cfgRoot)
	} else {
		dsDoc := ds.GetDocument()
		dsContent := map[string]interface{}{}
		dsMeta := map[string]interface{}{}
		schemaVersion := ""
		if dsDoc != nil {
			if dsDoc.GetContent() != nil && dsDoc.GetContent().Data != nil {
				dsContent = dsDoc.GetContent().Data
			}
			if dsDoc.GetMeta() != nil && dsDoc.GetMeta().Data != nil {
				dsMeta = dsDoc.GetMeta().Data
			}
			schemaVersion = dsDoc.GetSchemaVersion()
		}

		configRepoLocator, err := p.resolveConfigRepoLocator(dsContent, dsMeta, schemaVersion)
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to resolve configuration repo locator from desiredstate")
		}

		cfg := wrapper.NewBaseConfiguration(p.GetEnvironment(), "concourse", p.GetEnvVariables())
		if err := cfg.CloneRepoAndLoad(wrapper.CloneRequest{
			CloneDir:              p.configWorkdir,
			MetaRepoLocatorPath:   configRepoLocator,
			DesiredStateContent:   ds.GetDocument().GetContent().Data,
			SchemaVersionOverride: schemaVersionOverride,
		}); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to clone and load configuration from desiredstate")
		}
		p.SetConfiguration(cfg)
		logging.Debug("Configuration cloned and loaded from desiredstate")
	}

	// 3. Load pipeline instances and templates when requested
	if isLoadPipelines {
		if err := p.loadPipelines(); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to load pipeline instances")
		}
	}

	return nil
}

// LoadDesiredStates returns a list of parsers based on the selected pipeline mode.
//
// Three modes (exactly one must be true):
//   - isLocal/isMaster: use the current parser (pipelines already loaded).
//     Master is de facto a double of local as long as master pipelines are defined on
//     the same desiredstate; isMaster will eventually be removed.
//   - isSlave: iterate slave repo list from desiredstate, create one Parser per slave
func (p *Parser) LoadDesiredStates(isMaster, isSlave, isLocal bool) ([]*Parser, error) {
	if isLocal || isMaster {
		logging.Debug("Local/master mode: using current parser")
		return []*Parser{p}, nil
	}

	// Slave mode: recursively resolve slave desiredstates (leaf -> root order).
	logging.Info("Assembling recursive slave pipelines list...")
	p.isUpdatingSlavesNotSelf = true

	seen := make(map[string]bool)
	inProgress := make(map[string]bool)
	parsers, err := p.collectSlaveParsersRecursive(p, seen, inProgress)
	if err != nil {
		return nil, err
	}

	return parsers, nil
}

// Writes two files per pipeline:
//   - configuration.yaml: merged desiredstate (meta+content) + globalConfig + instanceConfig
//   - pipeline.yaml: the pipeline template
//
// Returns (configFilePath, pipelineFilePath, error).
func (p *Parser) CachePipeline(pipeline *Pipeline, buildDir string) (string, string, error) {
	logging.Debug("Caching pipeline: %s", pipeline.Name)

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create build directory %s: %w", buildDir, err)
	}

	dsDoc := p.GetDesiredState().GetDocument()
	if dsDoc == nil {
		return "", "", errors.NewParamError("desiredstate not loaded")
	}

	h := yamlparser.NewYAMLHandler(dsDoc.GetWorkdir())

	// Serialise desiredstate meta and content to YAML strings
	dsMetaSource := deepCopyMap(dsDoc.GetMeta().Data)
	dsMetaSource = p.pruneDesiredStateMetaEcosystem(dsMetaSource, dsDoc.GetSchemaVersion())
	dsMeta, err := h.ToString(dsMetaSource)
	if err != nil {
		return "", "", fmt.Errorf("failed to serialise desiredstate meta: %w", err)
	}

	dsContent, err := h.ToString(dsDoc.GetContent().Data)
	if err != nil {
		return "", "", fmt.Errorf("failed to serialise desiredstate content: %w", err)
	}

	// Global config scoped under concourse.pipelines.configuration.global
	globalCfgData := buildScopedConfigMap(valueOrDefault(p.globalConfigPath, defaultPathGlobalConfig), pipeline.GlobalConfiguration)
	globalCfgStr, err := h.ToString(globalCfgData)
	if err != nil {
		return "", "", fmt.Errorf("failed to serialise global config: %w", err)
	}

	// Instance-specific config scoped under concourse.pipelines.configuration.instance
	instanceCfgData := buildScopedConfigMap(
		valueOrDefault(p.instanceConfigPath, defaultPathInstanceConfig),
		buildInstanceConfig(pipeline.Configuration, pipeline.GlobalConfiguration),
	)
	instanceCfgStr, err := h.ToString(instanceCfgData)
	if err != nil {
		return "", "", fmt.Errorf("failed to serialise instance config: %w", err)
	}

	// Merge all buffers: DS meta → DS content → global config → instance config
	buffers := []string{dsMeta, dsContent, globalCfgStr, instanceCfgStr}
	pipelineConfig, err := h.LoadBuffers(buffers)
	if err != nil {
		return "", "", fmt.Errorf("failed to merge pipeline configuration buffers: %w", err)
	}

	// Normalize merged YAML maps to map[string]interface{} before applying
	// recursive path-based pruning/deletion operations.
	if normalized, ok := normalizeYAMLNodeForConcourse(pipelineConfig).(map[string]interface{}); ok {
		pipelineConfig = normalized
	}

	// Enforce selected environment recursively for any desiredstate objects that
	// might be introduced by merged buffers (including ecosystem sub-documents).
	if err := pruneDesiredStateEnvironmentInMergedConfig(pipelineConfig, p.GetEnvironment()); err != nil {
		return "", "", fmt.Errorf("failed to prune merged pipeline configuration by environment: %w", err)
	}

	// Keep assembled ecosystem only at contentEcosystem. Remove metaEcosystem
	// from each desiredstate document when the two schema paths differ.
	p.pruneDesiredStateEcosystemMetaInMergedConfig(pipelineConfig, dsDoc.GetSchemaVersion())

	// Write configuration.yaml
	cfgFilePath := filepath.Join(buildDir, configFileSuffix)
	cfgFile, err := os.Create(cfgFilePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create configuration file %s: %w", cfgFilePath, err)
	}
	defer cfgFile.Close()

	if err := h.ToFile(pipelineConfig, cfgFile); err != nil {
		return "", "", fmt.Errorf("failed to write configuration YAML: %w", err)
	}
	logging.Debug("Assembled config: %s", cfgFilePath)

	// Write pipeline.yaml
	pipelineFilePath, err := pipeline.Cache(buildDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to cache pipeline template: %w", err)
	}
	logging.Debug("Assembled pipeline: %s", pipelineFilePath)

	return cfgFilePath, pipelineFilePath, nil
}

// GetPipelines returns the list of pipeline instances loaded by this parser.
func (p *Parser) GetPipelines() []*Pipeline {
	return p.pipelines
}

// GetGlobalConfig returns the global concourse configuration.
func (p *Parser) GetGlobalConfig() map[string]interface{} {
	return p.globalConfig
}

// loadPipelines reads the pipeline instances from the configuration document and loads each template.
func (p *Parser) loadPipelines() error {
	cfg := p.GetConfiguration()
	if cfg == nil {
		return errors.NewParamError("configuration not loaded; cannot load pipelines")
	}

	cfgDoc := cfg.GetDocument()
	if cfgDoc == nil {
		return errors.NewParamError("configuration document is nil")
	}

	cfgContent := cfgDoc.GetContent()
	if cfgContent == nil || cfgContent.Data == nil {
		return errors.NewParamError("configuration content is empty")
	}

	h := yamlparser.NewYAMLHandler(cfgDoc.GetWorkdir())
	metaData := map[string]interface{}{}
	if cfgDoc.GetMeta() != nil && cfgDoc.GetMeta().Data != nil {
		metaData = cfgDoc.GetMeta().Data
	}

	globalPath := p.getConfigurationPropertyPath(metaData, cfgDoc.GetSchemaVersion(), "concourseGlobalConfig", defaultPathGlobalConfig)
	instancePath := deriveInstanceConfigPath(globalPath)
	instancesPath := p.getConfigurationPropertyPath(metaData, cfgDoc.GetSchemaVersion(), "concoursePipelineInstances", defaultPathPipelineInstances)
	p.globalConfigPath = globalPath
	p.instanceConfigPath = instancePath

	// Read global configuration
	globalRaw, err := h.GetValue(cfgContent.Data, globalPath)
	if err != nil {
		logging.Warn("No global configuration found at %s: %v", globalPath, err)
	} else if globalMap, ok := globalRaw.(map[string]interface{}); ok {
		p.globalConfig = globalMap
	}

	// Read pipeline instances list
	instancesRaw, err := h.GetValue(cfgContent.Data, instancesPath)
	if err != nil {
		return fmt.Errorf("no pipeline instances found at %s: %w", instancesPath, err)
	}

	var instances []interface{}
	switch value := instancesRaw.(type) {
	case []interface{}:
		instances = value
	case map[string]interface{}:
		// Support split/singular format where path points to one pipeline entry
		// (e.g. concourse.pipelines.instance).
		if _, hasInstance := value["instance"]; hasInstance {
			instances = []interface{}{value}
		} else if _, hasMeta := value["meta"]; hasMeta {
			instances = []interface{}{value}
		} else {
			// Also support map-of-entries shape for schema customisations.
			for _, entry := range value {
				instances = append(instances, entry)
			}
		}
	default:
		return fmt.Errorf("pipeline instances is not a supported shape (list/object): %T", instancesRaw)
	}

	for _, instRaw := range instances {
		instMap, ok := instRaw.(map[string]interface{})
		if !ok {
			logging.Warn("Skipping unexpected pipeline instance type: %T", instRaw)
			continue
		}

		pipeline, err := NewPipelineFromConfig(instMap)
		if err != nil {
			return fmt.Errorf("failed to parse pipeline instance: %w", err)
		}

		// Assign a deep copy of global config to this pipeline
		pipeline.GlobalConfiguration = deepCopyMap(p.globalConfig)

		// Build env vars for template expansion
		envVars := map[string]string{
			"AWS_REGION":  pipeline.AWSRegion,
			"ENVIRONMENT": pipeline.Environment,
			"SERVICE":     pipeline.Service,
		}
		for k, v := range p.GetEnvVariables() {
			envVars[k] = v
		}

		// Load pipeline template from its repository workdir.
		templateWorkdir, err := p.resolvePipelineTemplateWorkdir(pipeline.DesiredStateRepoLocator)
		if err != nil {
			return fmt.Errorf("failed to resolve template workdir for pipeline %s: %w", pipeline.Name, err)
		}
		if err := pipeline.LoadTemplate(templateWorkdir, envVars); err != nil {
			return fmt.Errorf("failed to load template for pipeline %s from repo %s: %w",
				pipeline.Name, templateWorkdir, err)
		}

		p.pipelines = append(p.pipelines, pipeline)
	}

	logging.Info("Loaded %d pipeline instance(s)", len(p.pipelines))
	return nil
}

func (p *Parser) getConfigurationPropertyPath(configMeta map[string]interface{}, schemaVersion string, key string, fallback string) string {
	return p.getSchemaPropertyPath(configMeta, schemaVersion, false, key, fallback)
}

func (p *Parser) getDesiredStatePropertyPath(dsMeta map[string]interface{}, schemaVersion string, key string, fallback string) string {
	return p.getSchemaPropertyPath(dsMeta, schemaVersion, true, key, fallback)
}

func (p *Parser) getSchemaPropertyPath(documentMeta map[string]interface{}, schemaVersion string, isDesiredState bool, key string, fallback string) string {
	return wrapper.DiscoverPropertyPathOrFallback(documentMeta, schemaVersion, isDesiredState, key, fallback)
}

func (p *Parser) getDesiredStatePipelinesRepoLocatorPath() (string, error) {
	ds := p.GetDesiredState()
	if ds == nil || ds.GetDocument() == nil {
		return "", errors.NewParamError("desiredstate document not loaded")
	}

	dsDoc := ds.GetDocument()
	dsMetaData := map[string]interface{}{}
	if dsDoc.GetMeta() != nil && dsDoc.GetMeta().Data != nil {
		dsMetaData = dsDoc.GetMeta().Data
	}

	return p.getDesiredStatePropertyPath(dsMetaData, dsDoc.GetSchemaVersion(), "concoursePipelinesRepo", defaultPathPipelinesRepo), nil
}

func (p *Parser) resolvePipelineTemplateWorkdir(locator string) (string, error) {
	// When an explicit pipelines workdir is provided, use it directly.
	if p.pipelinesWorkdir != "" {
		absWorkdir, err := filepath.Abs(p.pipelinesWorkdir)
		if err != nil {
			return "", fmt.Errorf("failed to resolve pipelines workdir '%s': %w", p.pipelinesWorkdir, err)
		}
		return absWorkdir, nil
	}

	return p.resolvePipelineRepoPath(locator)
}

func (p *Parser) resolvePipelineRepoPath(locator string) (string, error) {
	if locator != "" {
		repoPath, err := p.resolveRepoPathFromDS(locator)
		if err == nil {
			return repoPath, nil
		}
		logging.Warn("Pipeline repo locator '%s' could not be resolved (%v); trying schema fallback", locator, err)
	}

	fallbackLocator, err := p.getDesiredStatePipelinesRepoLocatorPath()
	if err != nil {
		if locator != "" {
			return "", fmt.Errorf("pipeline repo locator '%s' failed and schema fallback is unavailable: %w", locator, err)
		}
		return "", fmt.Errorf("no pipeline repo locator configured and schema fallback is unavailable: %w", err)
	}

	repoPath, err := p.resolveRepoPathFromDS(fallbackLocator)
	if err != nil {
		if locator != "" {
			return "", fmt.Errorf("pipeline repo locator '%s' failed and schema fallback '%s' failed: %w", locator, fallbackLocator, err)
		}
		return "", fmt.Errorf("pipeline repo locator fallback '%s' failed: %w", fallbackLocator, err)
	}

	return repoPath, nil
}

func (p *Parser) resolveConfigRepoLocator(dsContent map[string]interface{}, dsMeta map[string]interface{}, schemaVersion string) (string, error) {
	if dsContent == nil {
		return "", fmt.Errorf("desiredstate content is empty")
	}

	locators, err := wrapper.UsableConfigRepoLocators("concourse", p.GetEnvironment(), dsContent, dsMeta, schemaVersion)
	if err != nil {
		return "", err
	}
	return locators[0], nil
}

// resolveRepoPathFromDS resolves a pipeline repo path from desiredstate content.
// The locator identifies a repo descriptor in desiredstate; repo.Path (if present)
// is always interpreted relative to that repo workdir, not desiredstate workdir.
func (p *Parser) resolveRepoPathFromDS(locator string) (string, error) {
	dsDoc := p.GetDesiredState().GetDocument()
	if dsDoc == nil {
		return "", errors.NewParamError("desiredstate document not loaded")
	}
	dsContent := dsDoc.GetContent()
	if dsContent == nil || dsContent.Data == nil {
		return "", errors.NewParamError("desiredstate content is empty")
	}

	repo, err := gitrepo.NewRepoFromDesiredState(dsContent.Data, locator, p.pipelinesWorkdir, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to load repo from locator '%s': %w", locator, err)
	}

	repoPath := repo.GetFilePath()
	if repoPath == "" {
		return "", fmt.Errorf("repo locator '%s' resolved to empty workdir", locator)
	}

	return repoPath, nil
}

// resolveDesiredStatePath extracts the absolute path from a slave repo descriptor.
// A slave repo entry looks like: {url, ref, path, watch}
func resolveDesiredStatePath(repo map[string]interface{}) (string, error) {
	pathVal, ok := repo["path"]
	if !ok {
		return "", fmt.Errorf("slave repo entry missing 'path' key")
	}
	pathStr, ok := pathVal.(string)
	if !ok {
		return "", fmt.Errorf("slave repo 'path' is not a string: %T", pathVal)
	}
	absPath, err := filepath.Abs(pathStr)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for %s: %w", pathStr, err)
	}
	return absPath, nil
}

func resolveDesiredStatePathWithBase(repo map[string]interface{}, baseDir string) (string, error) {
	pathVal, ok := repo["path"]
	if !ok {
		return "", fmt.Errorf("slave repo entry missing 'path' key")
	}

	pathStr, ok := pathVal.(string)
	if !ok {
		return "", fmt.Errorf("slave repo 'path' is not a string: %T", pathVal)
	}

	if filepath.IsAbs(pathStr) {
		return filepath.Clean(pathStr), nil
	}

	if baseDir == "" {
		return filepath.Abs(pathStr)
	}

	return filepath.Abs(filepath.Join(baseDir, pathStr))
}

func (p *Parser) collectSlaveParsersRecursive(current *Parser, seen map[string]bool, inProgress map[string]bool) ([]*Parser, error) {
	ds := current.GetDesiredState()
	if ds == nil || ds.GetDocument() == nil {
		return nil, errors.NewParamError("desiredstate document not loaded")
	}
	dsDoc := ds.GetDocument()

	slaveList, err := current.getSlavePipelinesRepoListForCurrentDesiredState()
	if err != nil {
		return nil, err
	}
	if len(slaveList) == 0 {
		return []*Parser{}, nil
	}

	baseDir := dsDoc.GetWorkdir()
	result := make([]*Parser, 0)

	for _, slaveRaw := range slaveList {
		slaveRepo, ok := slaveRaw.(map[string]interface{})
		if !ok {
			logging.Warn("Skipping unexpected slave repo entry type: %T", slaveRaw)
			continue
		}

		dsAbsPath, err := resolveDesiredStatePathWithBase(slaveRepo, baseDir)
		if err != nil {
			logging.Warn("Skipping slave repo: %v", err)
			continue
		}

		dsAbsPath = filepath.Clean(dsAbsPath)
		if inProgress[dsAbsPath] {
			return nil, fmt.Errorf("recursive slave desiredstate reference detected: %s", dsAbsPath)
		}
		if seen[dsAbsPath] {
			continue
		}

		inProgress[dsAbsPath] = true
		logging.Info("Assembling pipeline from: %s", dsAbsPath)

		slaveParser := NewParser(p.GetEnvironment(), p.GetEnvVariables())
		if err := slaveParser.LoadGitOpsFilesExtended(dsAbsPath, "", p.configWorkdir, p.pipelinesWorkdir, true); err != nil {
			delete(inProgress, dsAbsPath)
			return nil, fmt.Errorf("failed to load slave desiredstate %s: %w", dsAbsPath, err)
		}

		children, err := p.collectSlaveParsersRecursive(slaveParser, seen, inProgress)
		if err != nil {
			delete(inProgress, dsAbsPath)
			return nil, err
		}

		// Post-order: children first, then parent node.
		result = append(result, children...)
		result = append(result, slaveParser)

		seen[dsAbsPath] = true
		delete(inProgress, dsAbsPath)
	}

	return result, nil
}

func (p *Parser) getSlavePipelinesRepoListForCurrentDesiredState() ([]interface{}, error) {
	ds := p.GetDesiredState()
	if ds == nil || ds.GetDocument() == nil {
		return nil, errors.NewParamError("desiredstate document not loaded")
	}
	dsDoc := ds.GetDocument()

	dsMetaData := map[string]interface{}{}
	if dsDoc.GetMeta() != nil && dsDoc.GetMeta().Data != nil {
		dsMetaData = dsDoc.GetMeta().Data
	}

	dsContentData := map[string]interface{}{}
	if dsDoc.GetContent() != nil && dsDoc.GetContent().Data != nil {
		dsContentData = dsDoc.GetContent().Data
	}

	slavePipelinesPath := p.getDesiredStatePropertyPath(dsMetaData, dsDoc.GetSchemaVersion(), "slavePipelinesRepoList", defaultPathSlavePipelines)

	// Slaves may be declared in desiredstate parts (for example pipelines.yaml), so
	// resolve from assembled content first, then fall back to root metadata.
	slavePipelinesRepoListRaw, err := p.handler.GetValue(dsContentData, slavePipelinesPath)
	if err != nil {
		slavePipelinesRepoListRaw, err = p.handler.GetValue(dsMetaData, slavePipelinesPath)
		if err != nil {
			return []interface{}{}, nil
		}
	}

	slaveList, ok := slavePipelinesRepoListRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("slave pipelines list is not a list: %T", slavePipelinesRepoListRaw)
	}

	return slaveList, nil
}

// deepCopyMap returns a deep copy of a map using JSON round-trip.
func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return make(map[string]interface{})
	}
	data, err := json.Marshal(src)
	if err != nil {
		return make(map[string]interface{})
	}
	var dst map[string]interface{}
	if err := json.Unmarshal(data, &dst); err != nil {
		return make(map[string]interface{})
	}
	return dst
}

// buildInstanceConfig builds the final instance-specific configuration map.
// It inherits "schedule" and "unit_tests" from globalConfig if absent in instanceConfig.
func buildInstanceConfig(instanceConfig, globalConfig map[string]interface{}) map[string]interface{} {
	result := deepCopyMap(instanceConfig)

	for _, key := range []string{"schedule", "unit_tests"} {
		if _, exists := result[key]; !exists {
			if globalVal, ok := globalConfig[key]; ok {
				if globalMap, ok := globalVal.(map[string]interface{}); ok {
					result[key] = deepCopyMap(globalMap)
				}
			}
		}
	}

	return result
}

func deriveInstanceConfigPath(globalPath string) string {
	if strings.HasSuffix(globalPath, ".global") {
		return strings.TrimSuffix(globalPath, ".global") + ".instance"
	}

	return defaultPathInstanceConfig
}

func buildScopedConfigMap(path string, value map[string]interface{}) map[string]interface{} {
	if path == "" {
		return deepCopyMap(value)
	}

	result := map[string]interface{}{}
	current := result
	segments := strings.Split(path, ".")
	last := len(segments) - 1

	for i, segment := range segments {
		if segment == "" {
			continue
		}

		if i == last {
			current[segment] = deepCopyMap(value)
			break
		}

		next := map[string]interface{}{}
		current[segment] = next
		current = next
	}

	return result
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

func (p *Parser) pruneDesiredStateMetaEcosystem(meta map[string]interface{}, schemaVersion string) map[string]interface{} {
	if meta == nil {
		return meta
	}

	metaPath := p.getDesiredStatePropertyPath(meta, schemaVersion, "metaEcosystem", "desiredstate.meta.ecosystem")
	contentPath := p.getDesiredStatePropertyPath(meta, schemaVersion, "contentEcosystem", "desiredstate.ecosystem")

	if metaPath == "" || contentPath == "" || metaPath == contentPath {
		return meta
	}

	deleteDottedPath(meta, metaPath)
	return meta
}

func deleteDottedPath(root map[string]interface{}, path string) {
	if root == nil || path == "" {
		return
	}

	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return
	}

	current := root
	for _, segment := range segments[:len(segments)-1] {
		nextRaw, ok := current[segment]
		if !ok {
			return
		}
		next, ok := nextRaw.(map[string]interface{})
		if !ok {
			return
		}
		current = next
	}

	delete(current, segments[len(segments)-1])
}

func (p *Parser) pruneDesiredStateEcosystemMetaInMergedConfig(root map[string]interface{}, defaultSchemaVersion string) {
	if root == nil {
		return
	}

	var walk func(node interface{})
	walk = func(node interface{}) {
		switch value := node.(type) {
		case map[string]interface{}:
			if _, ok := value["desiredstate"].(map[string]interface{}); ok {
				schemaVersion := defaultSchemaVersion
				if rawVersion, exists := value["schema"]; exists {
					if schemaStr, ok := rawVersion.(string); ok && schemaStr != "" {
						schemaVersion = schemaStr
					}
				}

				metaPath := p.getDesiredStatePropertyPath(value, schemaVersion, "metaEcosystem", "desiredstate.meta.ecosystem")
				contentPath := p.getDesiredStatePropertyPath(value, schemaVersion, "contentEcosystem", "desiredstate.ecosystem")
				if metaPath != "" && contentPath != "" && metaPath != contentPath {
					deleteDottedPath(value, metaPath)
				}
			}

			for _, child := range value {
				walk(child)
			}

		case []interface{}:
			for _, item := range value {
				walk(item)
			}
		}
	}

	walk(root)
}

func pruneDesiredStateEnvironmentInMergedConfig(root map[string]interface{}, environment string) error {
	if environment == "" {
		return nil
	}

	normalized, ok := normalizeYAMLNodeForConcourse(root).(map[string]interface{})
	if !ok {
		return nil
	}

	return pruneDesiredStateEnvironmentInNode(normalized, environment)
}

func pruneDesiredStateEnvironmentInNode(node interface{}, environment string) error {
	switch value := node.(type) {
	case map[string]interface{}:
		if rawDesiredstate, ok := value["desiredstate"]; ok {
			desiredstate, ok := rawDesiredstate.(map[string]interface{})
			if ok {
				if err := pruneDesiredStateEnvironmentMap(desiredstate, environment); err != nil {
					return err
				}
			}
		}

		for _, child := range value {
			if err := pruneDesiredStateEnvironmentInNode(child, environment); err != nil {
				return err
			}
		}

	case []interface{}:
		for _, item := range value {
			if err := pruneDesiredStateEnvironmentInNode(item, environment); err != nil {
				return err
			}
		}
	}

	return nil
}

func pruneDesiredStateEnvironmentMap(desiredstate map[string]interface{}, environment string) error {
	if rawContent, ok := desiredstate["content"]; ok {
		if content, ok := rawContent.(map[string]interface{}); ok {
			if rawEnvs, ok := content["environments"]; ok {
				if envs, ok := rawEnvs.(map[string]interface{}); ok {
					selected, exists := envs[environment]
					if !exists {
						return errors.Newf(errors.ErrParam,
							"environment '%s' is not defined under desiredstate.content.environments",
							environment,
						)
					}
					content["environments"] = map[string]interface{}{environment: selected}
					return nil
				}
			}
		}
	}

	if rawConfig, ok := desiredstate["configuration"]; ok {
		if config, ok := rawConfig.(map[string]interface{}); ok {
			selected, exists := config[environment]
			if !exists {
				return errors.Newf(errors.ErrParam,
					"environment '%s' is not defined under desiredstate.configuration",
					environment,
				)
			}
			desiredstate["configuration"] = map[string]interface{}{environment: selected}
			return nil
		}
	}

	return nil
}

func normalizeYAMLNodeForConcourse(node interface{}) interface{} {
	switch value := node.(type) {
	case map[string]interface{}:
		for key, child := range value {
			value[key] = normalizeYAMLNodeForConcourse(child)
		}
		return value

	case map[interface{}]interface{}:
		normalized := make(map[string]interface{}, len(value))
		for rawKey, child := range value {
			key, ok := rawKey.(string)
			if !ok {
				continue
			}
			normalized[key] = normalizeYAMLNodeForConcourse(child)
		}
		return normalized

	case []interface{}:
		for index, child := range value {
			value[index] = normalizeYAMLNodeForConcourse(child)
		}
		return value

	default:
		return node
	}
}
