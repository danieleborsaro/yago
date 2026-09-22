package concourse

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// Pipeline represents a Concourse pipeline assembled from a template and GitOps configuration.
//
// Each pipeline maps to one entry in concourse.pipelines.instances, structured as:
//
//	instance:
//	  name: my-pipeline
//	  template: path/to/template.yaml
//	  team: main
//	  region: eu-west-1
//	  environment: dev      # optional
//	  repo: ds.content.path  # optional - locator for pipeline repo in desiredstate
//	  service: my-service   # optional
//	configuration: {}       # instance-specific config overrides
type Pipeline struct {
	// Instance properties (from the "instance" sub-key in configuration)
	Name                    string
	Template                string
	Team                    string
	Environment             string
	AWSRegion               string
	Service                 string
	DesiredStateRepoLocator string

	// Configuration
	Configuration       map[string]interface{} // instance-specific overrides (from "configuration" sub-key)
	GlobalConfiguration map[string]interface{} // global config, set by Parser after loading

	// Internal state
	content    map[string]interface{} // loaded pipeline template YAML
	bufferFile string                 // path to cached pipeline.yaml
	handler    *parser.YAMLHandler
}

// Hardcoded YAML keys used across schema versions, matching the keys in
// concourse.pipelines.instances list entries.
const (
	keyInstance        = "instance"
	keyMeta            = "meta"
	keyConfiguration   = "configuration"
	keyName            = "name"
	keyTemplate        = "template"
	keyTeam            = "team"
	keyRegion          = "region"
	keyEnvironment     = "environment"
	keyRepo            = "repo"
	keyService         = "service"
	defaultEnvironment = "all"
	defaultService     = "default"
	templateFileSuffix = "pipeline.yaml"
	configFileSuffix   = "configuration.yaml"
	buildDirPrefix     = "concourse"
)

// NewPipelineFromConfig initialises a Pipeline from a single entry in concourse.pipelines.instances.
//
// Expected input structure:
//
//	instance:
//	  name: my-pipeline
//	  template: template.yaml
//	  team: main
//	  region: eu-west-1
//	configuration: {}
func NewPipelineFromConfig(cfg map[string]interface{}) (*Pipeline, error) {
	if cfg == nil {
		return nil, errors.NewParamError("pipeline config entry is nil")
	}

	// Extract instance metadata from either:
	// - legacy shape: {instance: {...}, configuration: {...}}
	// - split shape:  {meta: {...}, configuration: {...}}
	instanceRaw, ok := cfg[keyInstance]
	instanceKey := keyInstance
	if !ok {
		instanceRaw, ok = cfg[keyMeta]
		instanceKey = keyMeta
	}
	if !ok {
		return nil, errors.NewParamError("pipeline config entry missing 'instance' or 'meta' key")
	}
	instance, ok := instanceRaw.(map[string]interface{})
	if !ok {
		return nil, errors.NewParamError(fmt.Sprintf("pipeline '%s' is not a map: %T", instanceKey, instanceRaw))
	}

	// Required fields
	name, err := stringField(instance, keyName)
	if err != nil {
		return nil, fmt.Errorf("pipeline instance: %w", err)
	}
	template, err := stringField(instance, keyTemplate)
	if err != nil {
		return nil, fmt.Errorf("pipeline instance: %w", err)
	}
	team, err := stringField(instance, keyTeam)
	if err != nil {
		return nil, fmt.Errorf("pipeline instance: %w", err)
	}
	region, err := stringField(instance, keyRegion)
	if err != nil {
		return nil, fmt.Errorf("pipeline instance: %w", err)
	}

	// Optional fields with defaults
	environment := stringFieldOr(instance, keyEnvironment, defaultEnvironment)
	service := stringFieldOr(instance, keyService, defaultService)
	repo := stringFieldOr(instance, keyRepo, "")

	// Extract "configuration" sub-map (instance-specific overrides); may be absent or null
	var instanceConfig map[string]interface{}
	if cfgRaw, exists := cfg[keyConfiguration]; exists && cfgRaw != nil {
		if cfgMap, ok := cfgRaw.(map[string]interface{}); ok {
			instanceConfig = cfgMap
		}
	}
	if instanceConfig == nil {
		instanceConfig = make(map[string]interface{})
	}

	p := &Pipeline{
		Name:                    name,
		Template:                template,
		Team:                    team,
		Environment:             environment,
		AWSRegion:               region,
		Service:                 service,
		DesiredStateRepoLocator: repo,
		Configuration:           instanceConfig,
		GlobalConfiguration:     make(map[string]interface{}),
		handler:                 parser.NewYAMLHandler("/"),
	}

	logging.Debug("Loaded pipeline: name=%s team=%s region=%s environment=%s", p.Name, p.Team, p.AWSRegion, p.Environment)
	return p, nil
}

// LoadTemplate loads the pipeline YAML template from the pipelines workdir.
func (p *Pipeline) LoadTemplate(pipelinesWorkdir string, envVars map[string]string) error {
	templatePath := filepath.Join(pipelinesWorkdir, p.Template)

	logging.Debug("Loading pipeline template: %s", templatePath)

	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		return fmt.Errorf("pipeline template not found: %s", templatePath)
	}

	p.handler = parser.NewYAMLHandler(pipelinesWorkdir)
	content, err := p.handler.LoadFile(templatePath, envVars)
	if err != nil {
		return fmt.Errorf("failed to load pipeline template %s: %w", templatePath, err)
	}

	p.content = content
	logging.Debug("Loaded pipeline template: %s", templatePath)
	return nil
}

// Cache writes the pipeline template YAML to a file in the build directory.
// Returns the path to the written file.
func (p *Pipeline) Cache(buildDir string) (string, error) {
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create pipeline build directory %s: %w", buildDir, err)
	}

	filePath := filepath.Join(buildDir, templateFileSuffix)

	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create pipeline file %s: %w", filePath, err)
	}
	defer f.Close()

	if err := p.handler.ToFile(p.content, f); err != nil {
		return "", fmt.Errorf("failed to write pipeline YAML: %w", err)
	}

	p.bufferFile = filePath
	logging.Debug("Cached pipeline template: %s", filePath)
	return filePath, nil
}

// GetBufferFile returns the path to the last cached pipeline.yaml file.
func (p *Pipeline) GetBufferFile() string {
	return p.bufferFile
}

// stringField extracts a required string value from a map, returning an error if absent or wrong type.
func stringField(m map[string]interface{}, key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("missing required field '%s'", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field '%s' is not a string: %T", key, v)
	}
	return s, nil
}

// stringFieldOr extracts an optional string value from a map, returning defaultVal if absent.
func stringFieldOr(m map[string]interface{}, key, defaultVal string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}
