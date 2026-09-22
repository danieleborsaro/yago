package concourse

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// Service extends BaseService for concourse-specific operations.
// The base implementation provides Validate, Assemble, and GetSupportedSchemaVersions
// which are sufficient for standard GitOps file validation and assembly.
// Concourse-specific operations (pipeline assembly, fly deployment) are handled
// via the Parser directly in commands.go.
type Service struct {
	*wrapper.BaseService
}

// ConcourseAssembleRequest contains parameters for concourse assemble orchestration.
type ConcourseAssembleRequest struct {
	DesiredStateRoot  string
	ConfigurationRoot string
	ConfigRepoWorkdir string
	Environment       string
	PipelinesWorkdir  string
	Save              string
	IsMaster          bool
	IsSlave           bool
	IsLocal           bool
}

// PipelineArtifact records assembled file paths for a single pipeline.
type PipelineArtifact struct {
	DesiredStateFile  string
	PipelineName      string
	ConfigurationFile string
	TemplateFile      string
}

// ConcourseAssembleResponse captures assembled concourse outputs.
type ConcourseAssembleResponse struct {
	BuildDirectory string
	TotalPipelines int
	Pipelines      []PipelineArtifact
}

// SetPipelinesRequest contains parameters for setpipelines orchestration.
// It extends assemble inputs with fly target/auth/plumbing flags.
type SetPipelinesRequest struct {
	DesiredStateRoot  string
	ConfigurationRoot string
	ConfigRepoWorkdir string
	Environment       string
	PipelinesWorkdir  string
	Save              string
	IsMaster          bool
	IsSlave           bool
	IsLocal           bool

	TargetName     string
	ConcourseURL   string
	FlyTeam        string
	FlyUsername    string
	FlyPasswordB64 string
	IsForce        bool
	IsDryRun       bool
	IsManualRun    bool
}

// FlyOperation captures one fly lifecycle plumbing step planned by setpipelines.
type FlyOperation struct {
	Stage        string
	PipelineName string
	Preview      string
}

// SetPipelinesResponse captures setpipelines dry-run outputs.
type SetPipelinesResponse struct {
	AssembleResult *ConcourseAssembleResponse
	Operations     []FlyOperation
}

// NewService creates a new concourse service instance.
func NewService(baseDir string, enableInterpolation bool) *Service {
	return &Service{
		BaseService: wrapper.NewBaseService(baseDir, enableInterpolation),
	}
}

// AssembleConcourse assembles concourse pipeline files from desiredstate/configuration.
// Command handlers should delegate orchestration to this method.
func (s *Service) AssembleConcourse(req ConcourseAssembleRequest) (*ConcourseAssembleResponse, error) {
	modeCount := 0
	if req.IsMaster {
		modeCount++
	}
	if req.IsSlave {
		modeCount++
	}
	if req.IsLocal {
		modeCount++
	}
	if modeCount != 1 {
		return nil, fmt.Errorf("exactly one of master/slave/local pipeline modes must be specified (got %d)", modeCount)
	}

	if _, err := os.Stat(req.DesiredStateRoot); os.IsNotExist(err) {
		return nil, fmt.Errorf("desiredstate file '%s' is not readable", req.DesiredStateRoot)
	}

	buildDir, err := resolveConcourseBuildDir(req.Save, req.PipelinesWorkdir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve build directory: %w", err)
	}

	logging.Info("Build directory:  '%s'", buildDir)
	logging.Spaces()

	parser := NewParser(req.Environment, nil)
	if err := parser.LoadGitOpsFilesExtended(
		req.DesiredStateRoot,
		req.ConfigurationRoot,
		req.ConfigRepoWorkdir,
		req.PipelinesWorkdir,
		req.IsLocal || req.IsMaster,
	); err != nil {
		return nil, fmt.Errorf("failed to load GitOps files: %w", err)
	}

	parsers, err := parser.LoadDesiredStates(req.IsMaster, req.IsSlave, req.IsLocal)
	if err != nil {
		return nil, fmt.Errorf("failed to load desired states: %w", err)
	}

	response := &ConcourseAssembleResponse{
		BuildDirectory: buildDir,
		Pipelines:      make([]PipelineArtifact, 0),
	}

	for _, p := range parsers {
		dsFile := ""
		if p.GetDesiredState() != nil && p.GetDesiredState().GetDocument() != nil {
			dsFile = p.GetDesiredState().GetDocument().GetMetaFile()
		}

		logging.Info("Assembling pipelines from: %s", dsFile)

		if len(p.GetPipelines()) == 0 {
			if err := p.loadPipelines(); err != nil {
				return nil, fmt.Errorf("failed to load pipelines for desiredstate %s: %w", dsFile, err)
			}
		}

		for _, pipeline := range p.GetPipelines() {
			pipelineBuildDir := filepath.Join(buildDir, pipeline.Name)

			cfgFile, pipelineFile, err := p.CachePipeline(pipeline, pipelineBuildDir)
			if err != nil {
				return nil, fmt.Errorf("failed to cache pipeline '%s': %w", pipeline.Name, err)
			}

			response.Pipelines = append(response.Pipelines, PipelineArtifact{
				DesiredStateFile:  dsFile,
				PipelineName:      pipeline.Name,
				ConfigurationFile: cfgFile,
				TemplateFile:      pipelineFile,
			})
			response.TotalPipelines++
		}
	}

	return response, nil
}

// SetPipelines performs setpipelines orchestration in dry-run mode.
// It reuses AssembleConcourse for parser loading and artifact generation, then builds
// the fly lifecycle operation plan (sync/login/update/logout) without executing fly.
func (s *Service) SetPipelines(req SetPipelinesRequest) (*SetPipelinesResponse, error) {
	assembleResp, err := s.AssembleConcourse(ConcourseAssembleRequest{
		DesiredStateRoot:  req.DesiredStateRoot,
		ConfigurationRoot: req.ConfigurationRoot,
		ConfigRepoWorkdir: req.ConfigRepoWorkdir,
		Environment:       req.Environment,
		PipelinesWorkdir:  req.PipelinesWorkdir,
		Save:              req.Save,
		IsMaster:          req.IsMaster,
		IsSlave:           req.IsSlave,
		IsLocal:           req.IsLocal,
	})
	if err != nil {
		return nil, err
	}

	operations := make([]FlyOperation, 0, 2+(assembleResp.TotalPipelines*2)+1)
	syncPreview := fmt.Sprintf("dry-run: would download/install fly for target '%s' at '%s'", req.TargetName, req.ConcourseURL)
	if req.IsManualRun {
		syncPreview = fmt.Sprintf("[ManualRun] Skipping fly command downloading for target '%s'", req.TargetName)
	}
	operations = append(operations, FlyOperation{
		Stage:   "sync",
		Preview: syncPreview,
	})

	operations = append(operations, FlyOperation{
		Stage:   "login",
		Preview: fmt.Sprintf("dry-run: would login target '%s' as team '%s' (username configured=%t)", req.TargetName, req.FlyTeam, req.FlyUsername != ""),
	})

	for _, artifact := range assembleResp.Pipelines {
		operations = append(operations,
			FlyOperation{
				Stage:        "update.validate",
				PipelineName: artifact.PipelineName,
				Preview:      fmt.Sprintf("dry-run: would fly validate-pipeline using template '%s' and vars '%s'", artifact.TemplateFile, artifact.ConfigurationFile),
			},
			FlyOperation{
				Stage:        "update.apply",
				PipelineName: artifact.PipelineName,
				Preview:      fmt.Sprintf("dry-run: would fly set-pipeline for '%s' (force=%t)", artifact.PipelineName, req.IsForce),
			},
		)
	}

	logoutPreview := fmt.Sprintf("dry-run: would logout target '%s'", req.TargetName)
	if req.IsManualRun {
		logoutPreview = fmt.Sprintf("[ManualRun] Skipping logout for target '%s'", req.TargetName)
	}
	operations = append(operations, FlyOperation{
		Stage:   "logout",
		Preview: logoutPreview,
	})

	return &SetPipelinesResponse{
		AssembleResult: assembleResp,
		Operations:     operations,
	}, nil
}

// resolveConcourseBuildDir determines the build directory path.
// Priority: explicit save flag > temporary .gitops-* directory.
//
// When no explicit save path is provided, setpipelines/cache
// writes under a temporary directory created with a .gitops- prefix.
func resolveConcourseBuildDir(save, pipelinesWorkdir string) (string, error) {
	if save != "" {
		return filepath.Clean(save), nil
	}
	tmpBuildDir, err := os.MkdirTemp("", ".gitops-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary build directory: %w", err)
	}
	return tmpBuildDir, nil
}
