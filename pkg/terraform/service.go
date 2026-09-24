package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultTerraformVersion is the default Terraform version if not specified in desiredstate
	DefaultTerraformVersion = "1.5.1"
)

// Service extends BaseService with terraform-specific operations.
// It provides methods for running terraform commands (init, plan, apply, destroy, etc.)
type Service struct {
	*wrapper.BaseService

	// Terraform-specific fields
	tfVersion       string
	awsProfile      string
	awsRegion       string
	workspace       string
	useLocalBackend bool
	isDryRun        bool
	assembleParser  *Parser
	codeDir         string
	secretReader    secretValueReader
}

// NewService creates a new terraform service instance.
func NewService(baseDir string, enableInterpolation bool) *Service {
	// Check for IS_DRY_RUN environment variable
	isDryRun := os.Getenv("IS_DRY_RUN") == "1"

	service := &Service{
		BaseService: wrapper.NewBaseService(baseDir, enableInterpolation),
		tfVersion:   DefaultTerraformVersion,
		workspace:   "default",
		isDryRun:    isDryRun,
		codeDir:     baseDir,
	}

	service.SetAssembleHooks(service)
	return service
}

// SetDryRun sets the dry-run mode for the service
func (s *Service) SetDryRun(dryRun bool) {
	s.isDryRun = dryRun
}

// SetAWSProfile sets the AWS profile for terraform operations.
func (s *Service) SetAWSProfile(profile string) {
	s.awsProfile = profile
}

// SetAWSRegion sets the AWS region for terraform operations.
func (s *Service) SetAWSRegion(region string) {
	s.awsRegion = region
}

// SetWorkspace sets the Terraform workspace.
func (s *Service) SetWorkspace(workspace string) {
	s.workspace = workspace
}

// SetTerraformVersion sets the Terraform version to use.
func (s *Service) SetTerraformVersion(version string) {
	s.tfVersion = version
}

// SetUseLocalBackend sets whether to use local backend instead of remote.
func (s *Service) SetUseLocalBackend(useLocal bool) {
	s.useLocalBackend = useLocal
}

// TerraformAssembleRequest contains parameters for terraform assemble orchestration.
type TerraformAssembleRequest struct {
	DesiredStateFile  string
	ConfigFile        string
	ConfigRepoWorkdir string
	Environment       string
	AWSProfile        string
	AWSRegion         string
	TerraformSource   string
	OutputFormat      string
}

// TerraformAssembleResponse contains assembled artifact locations for terraform workflows.
type TerraformAssembleResponse struct {
	*wrapper.AssembleResponse
	BuildDirectory   string
	CodeDirectory    string
	BackendFile      string
	SourceCodeCloned bool
}

// AssembleTerraform performs terraform-specific assemble orchestration via BaseService.
// Command layer should delegate here and only print output.
func (s *Service) AssembleTerraform(req TerraformAssembleRequest) (*TerraformAssembleResponse, error) {
	if req.DesiredStateFile == "" {
		return nil, errors.New(errors.ErrParam, "desiredstate file must be specified")
	}

	if _, err := os.Stat(req.DesiredStateFile); os.IsNotExist(err) {
		return nil, errors.Newf(errors.ErrDesiredStateMissing, "desiredstate file '%s' is not readable", req.DesiredStateFile)
	}

	envVars := make(map[string]string)
	if req.AWSProfile != "" {
		envVars["AWS_PROFILE"] = req.AWSProfile
	}

	parser := NewParser(req.Environment, envVars)
	parser.SetConfigurationWorkdir(req.ConfigRepoWorkdir)
	if err := parser.LoadGitOpsFilesExtended(req.DesiredStateFile, req.AWSRegion, req.ConfigFile, req.TerraformSource); err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to load terraform GitOps files")
	}

	// Absolute, so the backend and var file paths still resolve when Terraform runs inside codeDir.
	codeDir := parser.GetSourceCodeDir()
	if codeDir == "" {
		return nil, errors.New(errors.ErrParam, "terraform source workdir could not be resolved")
	}

	buildDir := filepath.Join(codeDir, ".gitops")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create build directory %s", buildDir)
	}

	format := req.OutputFormat
	if format == "" {
		format = "json"
	}

	s.SetAWSProfile(req.AWSProfile)
	s.SetAWSRegion(req.AWSRegion)
	s.assembleParser = parser
	defer func() {
		s.assembleParser = nil
	}()

	assembleResp, err := s.BaseService.Assemble(wrapper.AssembleRequest{
		DesiredStateFile:  req.DesiredStateFile,
		ConfigFile:        req.ConfigFile,
		ConfigRepoWorkdir: req.ConfigRepoWorkdir,
		Environment:       req.Environment,
		Wrapper:           "terraform",
		CacheDirectory:    buildDir,
		OutputFormat:      format,
	})
	if err != nil {
		return nil, err
	}

	return &TerraformAssembleResponse{
		AssembleResponse: assembleResp,
		BuildDirectory:   buildDir,
		CodeDirectory:    codeDir,
		BackendFile:      parser.GetBackendFilePath(),
		SourceCodeCloned: parser.IsClonedSourceCode(),
	}, nil
}

// PreAssemble is a no-op hook for terraform.
func (s *Service) PreAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	return nil
}

// PostDesiredStateAssemble is a no-op hook for terraform.
func (s *Service) PostDesiredStateAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	return nil
}

// PostConfigurationAssemble is a no-op hook for terraform.
func (s *Service) PostConfigurationAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	return nil
}

// PostAssemble is a no-op hook for terraform.
func (s *Service) PostAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	return nil
}

// PostCache emits terraform backend artifact after BaseService caches desiredstate and configuration.
func (s *Service) PostCache(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.CacheContext) error {
	if s.assembleParser == nil {
		return nil
	}

	generateJSON := req.OutputFormat == "json"
	if err := s.assembleParser.CacheBackendConfig(req.CacheDirectory, generateJSON); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to cache terraform backend configuration")
	}

	if response.DesiredStateFile != "" {
		if err := keepOnlyDesiredStateVariable(response.DesiredStateFile, generateJSON); err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to prepare desiredstate var file %s", response.DesiredStateFile)
		}
	}
	if err := cacheSecretInputs(response.ConfigurationFile, req.CacheDirectory, s.awsProfile, s.awsRegion, generateJSON); err != nil {
		return errors.Wrap(errors.ErrFail, "failed to prepare Terraform secret references", err)
	}

	return nil
}

// Terraform warns about every undeclared variable in a -var-file, so keep only the desiredstate key.
func keepOnlyDesiredStateVariable(path string, generateJSON bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}

	value, ok := doc["desiredstate"]
	if !ok {
		return nil
	}
	trimmed := map[string]interface{}{"desiredstate": value}

	if generateJSON {
		data, err = json.MarshalIndent(trimmed, "", "  ")
	} else {
		data, err = yaml.Marshal(trimmed)
	}
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// InitRequest contains parameters for terraform init operation.
type InitRequest struct {
	WorkingDir        string
	BackendConfigFile string
	Reconfigure       bool
	Upgrade           bool
	MigrateState      bool
	NoBackend         bool
	Get               bool // Download modules (default true, set false to skip)
	LockProviders     bool // Lock provider versions
}

// InitResponse contains the results of terraform init operation.
type InitResponse struct {
	Success bool
	Output  string
	Error   string
}

// Init runs terraform init to initialize the working directory.
// Supports get, upgrade, reconfigure, migrate-state, lock-providers flags.
func (s *Service) Init(req InitRequest) (*InitResponse, error) {
	logging.Debug("Running terraform init in directory: %s", req.WorkingDir)

	args := []string{"init"}

	// Module download flag (default is true, only add if explicitly false)
	if !req.Get {
		args = append(args, "-get=false")
	}

	if req.Reconfigure {
		args = append(args, "-reconfigure")
	}

	if req.Upgrade {
		args = append(args, "-upgrade")
	}

	if req.MigrateState {
		args = append(args, "-migrate-state")
	}

	if req.NoBackend {
		args = append(args, "-backend=false")
	}

	if req.BackendConfigFile != "" {
		args = append(args, fmt.Sprintf("-backend-config=%s", req.BackendConfigFile))
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &InitResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	// If lock providers was requested, run providers lock after init
	if req.LockProviders {
		logging.Info("Locking provider versions...")
		lockReq := ProvidersLockRequest{
			WorkingDir: req.WorkingDir,
		}
		if _, err := s.ProvidersLock(lockReq); err != nil {
			logging.Warn("Failed to lock providers: %v", err)
		}
	}

	logging.Info("Terraform init completed successfully")
	return &InitResponse{
		Success: true,
		Output:  output,
	}, nil
}

// PlanRequest contains parameters for terraform plan operation.
type PlanRequest struct {
	WorkingDir       string
	DesiredStateVar  string // Path to assembled desiredstate JSON file
	ConfigurationVar string // Path to assembled configuration JSON file
	AwsRegion        string // AWS region variable
	Environment      string // Environment variable for TF_VAR_gitops_environment
	VarsFile         string // Additional vars file (deprecated, use DesiredStateVar/ConfigurationVar)
	OutFile          string
	Destroy          bool
	NoLock           bool
}

// PlanResponse contains the results of terraform plan operation.
type PlanResponse struct {
	Success    bool
	Output     string
	Error      string
	PlanFile   string
	HasChanges bool
}

// Plan runs terraform plan to generate an execution plan.
func (s *Service) Plan(req PlanRequest) (*PlanResponse, error) {
	logging.Debug("Running terraform plan in directory: %s", req.WorkingDir)

	args := []string{"plan"}

	// Add var files (desiredstate and configuration)
	if req.DesiredStateVar != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.DesiredStateVar))
	}
	if req.ConfigurationVar != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.ConfigurationVar))
	}

	// Add AWS region variable
	if req.AwsRegion != "" {
		args = append(args, fmt.Sprintf("-var=aws_provider_region=%s", req.AwsRegion))
	}

	// Legacy support for single VarsFile
	if req.VarsFile != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.VarsFile))
	}

	if req.OutFile != "" {
		args = append(args, fmt.Sprintf("-out=%s", req.OutFile))
	}

	if req.Destroy {
		args = append(args, "-destroy")
	}

	if req.NoLock {
		args = append(args, "-lock=false")
	}

	// Set environment variables including TF_VAR_gitops_environment
	originalEnv := os.Getenv("TF_VAR_gitops_environment")
	if req.Environment != "" {
		os.Setenv("TF_VAR_gitops_environment", req.Environment)
		defer os.Setenv("TF_VAR_gitops_environment", originalEnv)
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &PlanResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Terraform plan completed successfully")
	if req.OutFile != "" && !s.isDryRun {
		manifest, err := readSecretManifest(defaultSecretManifest(req.WorkingDir))
		if err != nil {
			return nil, err
		}
		if err := writeSecretManifest(terraformPlanPath(req.WorkingDir, req.OutFile)+planSecretSuffix, manifest); err != nil {
			return nil, fmt.Errorf("failed to save secret references alongside Terraform plan: %w", err)
		}
	}
	return &PlanResponse{
		Success:    true,
		Output:     output,
		PlanFile:   req.OutFile,
		HasChanges: true, // TODO: Parse output to determine if there are changes
	}, nil
}

// ApplyRequest contains parameters for terraform apply operation.
type ApplyRequest struct {
	WorkingDir  string
	VarsFile    string
	PlanFile    string
	AutoApprove bool
	NoLock      bool
}

// ApplyResponse contains the results of terraform apply operation.
type ApplyResponse struct {
	Success bool
	Output  string
	Error   string
}

// Apply runs terraform apply to apply the planned changes.
func (s *Service) Apply(req ApplyRequest) (*ApplyResponse, error) {
	logging.Debug("Running terraform apply in directory: %s", req.WorkingDir)

	args := []string{"apply"}

	if req.PlanFile != "" {
		// If plan file is provided, just apply it
		args = append(args, req.PlanFile)
	} else {
		// Otherwise, use var file and auto-approve
		if req.VarsFile != "" {
			args = append(args, fmt.Sprintf("-var-file=%s", req.VarsFile))
		}

		if req.AutoApprove {
			args = append(args, "-auto-approve")
		}
	}

	if req.NoLock {
		args = append(args, "-lock=false")
	}

	manifestPath := defaultSecretManifest(req.WorkingDir)
	if req.PlanFile != "" {
		planManifest := terraformPlanPath(req.WorkingDir, req.PlanFile) + planSecretSuffix
		if _, err := os.Stat(planManifest); err == nil {
			manifestPath = planManifest
		} else if !os.IsNotExist(err) {
			return nil, err
		} else {
			manifest, err := readSecretManifest(manifestPath)
			if err != nil {
				return nil, err
			}
			if len(manifest.Variables) > 0 {
				return nil, fmt.Errorf("saved plan has no secret reference manifest; generate the plan again with yago")
			}
		}
	}
	output, err := s.runTerraformCommandWithSecrets(req.WorkingDir, manifestPath, args...)
	if err != nil {
		return &ApplyResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Terraform apply completed successfully")
	return &ApplyResponse{
		Success: true,
		Output:  output,
	}, nil
}

// DestroyRequest contains parameters for terraform destroy operation.
type DestroyRequest struct {
	WorkingDir       string
	DesiredStateVar  string // Path to assembled desiredstate JSON file
	ConfigurationVar string // Path to assembled configuration JSON file
	AwsRegion        string // AWS region variable
	Environment      string // Environment variable for TF_VAR_gitops_environment
	VarsFile         string // Additional vars file (deprecated, use DesiredStateVar/ConfigurationVar)
	AutoApprove      bool
	NoLock           bool
}

// DestroyResponse contains the results of terraform destroy operation.
type DestroyResponse struct {
	Success bool
	Output  string
	Error   string
}

// Destroy runs terraform destroy to destroy all managed infrastructure.
func (s *Service) Destroy(req DestroyRequest) (*DestroyResponse, error) {
	logging.Debug("Running terraform destroy in directory: %s", req.WorkingDir)

	args := []string{"destroy"}

	// Add var files (desiredstate and configuration)
	if req.DesiredStateVar != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.DesiredStateVar))
	}
	if req.ConfigurationVar != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.ConfigurationVar))
	}

	// Add AWS region variable
	if req.AwsRegion != "" {
		args = append(args, fmt.Sprintf("-var=aws_provider_region=%s", req.AwsRegion))
	}

	// Legacy support for single VarsFile
	if req.VarsFile != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.VarsFile))
	}

	if req.AutoApprove {
		args = append(args, "-auto-approve")
	}

	if req.NoLock {
		args = append(args, "-lock=false")
	}

	// Set environment variables including TF_VAR_gitops_environment
	originalEnv := os.Getenv("TF_VAR_gitops_environment")
	if req.Environment != "" {
		os.Setenv("TF_VAR_gitops_environment", req.Environment)
		defer os.Setenv("TF_VAR_gitops_environment", originalEnv)
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &DestroyResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Terraform destroy completed successfully")
	return &DestroyResponse{
		Success: true,
		Output:  output,
	}, nil
}

// OutputRequest contains parameters for terraform output operation.
type OutputRequest struct {
	WorkingDir string
	OutputName string
	JSON       bool
}

// OutputResponse contains the results of terraform output operation.
type OutputResponse struct {
	Success bool
	Output  string
	Error   string
}

// Output runs terraform output to retrieve output values.
func (s *Service) Output(req OutputRequest) (*OutputResponse, error) {
	logging.Debug("Running terraform output in directory: %s", req.WorkingDir)

	args := []string{"output"}

	if req.JSON {
		args = append(args, "-json")
	}

	if req.OutputName != "" {
		args = append(args, req.OutputName)
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &OutputResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Terraform output completed successfully")
	return &OutputResponse{
		Success: true,
		Output:  output,
	}, nil
}

// ValidateRequest contains parameters for terraform validate operation.
type ValidateRequest struct {
	WorkingDir string
	JSON       bool
}

// ValidateResponse contains the results of terraform validate operation.
type ValidateResponse struct {
	Success bool
	Output  string
	Error   string
}

// ValidateTerraform runs terraform validate to check configuration syntax.
func (s *Service) ValidateTerraform(req ValidateRequest) (*ValidateResponse, error) {
	logging.Debug("Running terraform validate in directory: %s", req.WorkingDir)

	args := []string{"validate"}

	if req.JSON {
		args = append(args, "-json")
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &ValidateResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Terraform validate completed successfully")
	return &ValidateResponse{
		Success: true,
		Output:  output,
	}, nil
}

// CheckDependencies verifies that the installed terraform version matches the requirement.
// Parses `terraform --version` and compares with the desiredstate version.
func (s *Service) CheckDependencies() error {
	logging.Info("Checking terraform version...")

	// In codeDir, so version managers (mise, asdf) pick the same version as the other terraform commands.
	cmd := exec.Command("terraform", "--version")
	cmd.Dir = s.codeDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(errors.ErrMissingTool, "terraform command not found, please install terraform", err)
	}

	outputStr := string(output)
	logging.Debug("Terraform version output: %s", outputStr)

	// Parse version from output (format: "Terraform vX.Y.Z")
	// Example: "Terraform v1.5.1\non linux_amd64\n..."
	var installedVersion string
	lines := strings.Split(outputStr, "\n")
	if len(lines) > 0 {
		// First line usually contains "Terraform vX.Y.Z"
		parts := strings.Fields(lines[0])
		if len(parts) >= 2 && strings.HasPrefix(parts[1], "v") {
			installedVersion = strings.TrimPrefix(parts[1], "v")
		}
	}

	if installedVersion == "" {
		return errors.New(errors.ErrMissingTool, "could not parse terraform version from output")
	}

	logging.Info("Installed terraform version: %s", installedVersion)

	// Compare with required version
	// We only check major version compatibility (e.g., 1.x is compatible with 1.y)
	// Minor version differences are not treated as a mismatch
	requiredVersion := s.tfVersion
	installedMajor := strings.Split(installedVersion, ".")[0]
	requiredMajor := strings.Split(requiredVersion, ".")[0]

	if installedMajor != requiredMajor {
		return errors.Newf(errors.ErrMissingTool,
			"terraform major version mismatch: installed %s.x, required %s.x",
			installedMajor, requiredMajor)
	}

	logging.Info("Terraform ok")
	return nil
}

// Reset cleans the terraform state directory (.terraform) to start fresh.
func (s *Service) Reset(workingDir string) error {
	logging.Info("Resetting terraform state directory...")

	terraformDir := filepath.Join(workingDir, ".terraform")

	// Check if .terraform directory exists
	if _, err := os.Stat(terraformDir); os.IsNotExist(err) {
		logging.Debug(".terraform directory does not exist, nothing to clean")
		return nil
	}

	// Handle dry-run mode - just log what would be removed
	if s.isDryRun {
		logging.Info("[Dry-Run] Would remove: %s", terraformDir)
		return nil
	}

	// Remove .terraform directory
	if err := os.RemoveAll(terraformDir); err != nil {
		return errors.Wrap(errors.ErrFail, "failed to remove .terraform directory", err)
	}

	logging.Info("Successfully removed .terraform directory")
	return nil
}

// WorkspaceRequest contains parameters for terraform workspace operations.
type WorkspaceRequest struct {
	WorkingDir string
	Name       string
	Operation  string // "select", "list", "new", "delete"
}

// WorkspaceResponse contains the results of workspace operation.
type WorkspaceResponse struct {
	Success bool
	Output  string
	Error   string
}

// Workspace manages terraform workspaces (select, list, new, delete).
func (s *Service) Workspace(req WorkspaceRequest) (*WorkspaceResponse, error) {
	logging.Info("Terraform workspace operation: %s", req.Operation)

	var args []string

	switch req.Operation {
	case "select":
		// First check if workspace exists
		listCmd := exec.Command("terraform", "workspace", "list")
		listCmd.Dir = req.WorkingDir
		if s.awsProfile != "" {
			listCmd.Env = append(os.Environ(), fmt.Sprintf("AWS_PROFILE=%s", s.awsProfile))
		}
		listOutput, _ := listCmd.CombinedOutput()

		// Check if workspace exists in the list
		workspaceExists := false
		for _, line := range strings.Split(string(listOutput), "\n") {
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "*"))
			if trimmed == req.Name {
				workspaceExists = true
				break
			}
		}

		if workspaceExists {
			logging.Info("Selecting existing workspace: %s", req.Name)
			args = []string{"workspace", "select", req.Name}
		} else {
			logging.Info("Workspace %s does not exist, creating new one", req.Name)
			args = []string{"workspace", "new", req.Name}
		}

	case "list":
		args = []string{"workspace", "list"}

	case "new":
		args = []string{"workspace", "new", req.Name}

	case "delete":
		args = []string{"workspace", "delete", req.Name}

	default:
		return nil, errors.Newf(errors.ErrParam, "invalid workspace operation: %s", req.Operation)
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &WorkspaceResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	return &WorkspaceResponse{
		Success: true,
		Output:  output,
	}, nil
}

// GraphRequest contains parameters for terraform graph operation.
type GraphRequest struct {
	WorkingDir string
	Type       string // "plan", "apply", "plan-destroy"
	OutputFile string // Optional: write SVG to file
}

// GraphResponse contains the results of graph operation.
type GraphResponse struct {
	Success bool
	Output  string // DOT format graph
	SVGFile string // Path to SVG file if generated
	Error   string
}

// Graph generates a visual dependency graph of terraform resources.
func (s *Service) Graph(req GraphRequest) (*GraphResponse, error) {
	logging.Info("Generating terraform dependency graph...")

	args := []string{"graph"}

	if req.Type != "" {
		args = append(args, fmt.Sprintf("-type=%s", req.Type))
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &GraphResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	response := &GraphResponse{
		Success: true,
		Output:  output,
	}

	// If output file specified, convert DOT to SVG using graphviz (if available)
	if req.OutputFile != "" {
		if _, err := exec.LookPath("dot"); err == nil {
			// Create SVG file
			svgPath := req.OutputFile
			if !strings.HasSuffix(svgPath, ".svg") {
				svgPath = svgPath + ".svg"
			}

			dotCmd := exec.Command("dot", "-Tsvg", "-o", svgPath)
			dotCmd.Dir = req.WorkingDir
			dotCmd.Stdin = strings.NewReader(output)

			if err := dotCmd.Run(); err != nil {
				logging.Warn("Failed to convert graph to SVG: %v", err)
			} else {
				response.SVGFile = svgPath
				logging.Info("Graph saved to: %s", svgPath)
			}
		} else {
			logging.Warn("graphviz 'dot' command not found, cannot generate SVG")
		}
	}

	return response, nil
}

// UnlockRequest contains parameters for terraform unlock operation.
type UnlockRequest struct {
	WorkingDir string
	LockID     string
	Force      bool
}

// UnlockResponse contains the results of unlock operation.
type UnlockResponse struct {
	Success bool
	Output  string
	Error   string
}

// Unlock force-unlocks the terraform state.
func (s *Service) Unlock(req UnlockRequest) (*UnlockResponse, error) {
	logging.Info("Unlocking terraform state...")

	if req.LockID == "" {
		return nil, errors.New(errors.ErrParam, "lock ID is required for unlock operation")
	}

	args := []string{"force-unlock"}

	if req.Force {
		args = append(args, "-force")
	}

	args = append(args, req.LockID)

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &UnlockResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Successfully unlocked terraform state")
	return &UnlockResponse{
		Success: true,
		Output:  output,
	}, nil
}

// ImportRequest contains parameters for terraform import operation.
type ImportRequest struct {
	WorkingDir       string
	ResourceAddress  string // e.g., "aws_instance.example"
	ResourceID       string // e.g., "i-1234567890abcdef0"
	DesiredStateFile string // Path to desiredstate var file
	ConfigFile       string // Path to configuration var file
	AwsRegion        string // AWS region for aws_provider_region var
	Environment      string // Environment for TF_VAR_gitops_environment
}

// ImportResponse contains the results of import operation.
type ImportResponse struct {
	Success bool
	Output  string
	Error   string
}

// Import imports existing infrastructure into terraform state.
func (s *Service) Import(req ImportRequest) (*ImportResponse, error) {
	logging.Info("Importing resource into terraform state...")

	if req.ResourceAddress == "" || req.ResourceID == "" {
		return nil, errors.New(errors.ErrParam, "both resource address and ID are required for import")
	}

	// Set TF_VAR_gitops_environment if environment is provided
	originalEnv := os.Getenv("TF_VAR_gitops_environment")
	if req.Environment != "" {
		os.Setenv("TF_VAR_gitops_environment", req.Environment)
		defer os.Setenv("TF_VAR_gitops_environment", originalEnv)
	}

	args := []string{"import"}

	// Add var files (desiredstate and configuration)
	if req.DesiredStateFile != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.DesiredStateFile))
	}
	if req.ConfigFile != "" {
		args = append(args, fmt.Sprintf("-var-file=%s", req.ConfigFile))
	}

	// Add AWS region var
	if req.AwsRegion != "" {
		args = append(args, fmt.Sprintf("-var=aws_provider_region=%s", req.AwsRegion))
	}

	// Add resource address and ID
	args = append(args, req.ResourceAddress, req.ResourceID)

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &ImportResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Successfully imported resource: %s", req.ResourceAddress)
	return &ImportResponse{
		Success: true,
		Output:  output,
	}, nil
}

// ProvidersLockRequest contains parameters for terraform providers lock operation.
type ProvidersLockRequest struct {
	WorkingDir string
	Platforms  []string // e.g., ["windows_amd64", "darwin_amd64", "linux_amd64"]
}

// ProvidersLockResponse contains the results of providers lock operation.
type ProvidersLockResponse struct {
	Success bool
	Output  string
	Error   string
}

// ProvidersLock generates/updates the dependency lock file for providers.
func (s *Service) ProvidersLock(req ProvidersLockRequest) (*ProvidersLockResponse, error) {
	logging.Info("Locking terraform provider versions...")

	args := []string{"providers", "lock"}

	// Add platforms
	if len(req.Platforms) == 0 {
		// Default platforms
		req.Platforms = []string{"windows_amd64", "darwin_amd64", "linux_amd64"}
	}

	for _, platform := range req.Platforms {
		args = append(args, fmt.Sprintf("-platform=%s", platform))
	}

	output, err := s.runTerraformCommand(req.WorkingDir, args...)
	if err != nil {
		return &ProvidersLockResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Successfully locked provider versions")
	return &ProvidersLockResponse{
		Success: true,
		Output:  output,
	}, nil
}

// CostsRequest contains parameters for infracost operation.
type CostsRequest struct {
	WorkingDir       string
	PlanFile         string
	UsePlanFile      bool
	DesiredStateFile string // Path to desiredstate var file
	ConfigFile       string // Path to configuration var file
	AwsRegion        string // AWS region for aws_provider_region var
}

// CostsResponse contains the results of cost estimation.
type CostsResponse struct {
	Success bool
	Output  string
	Error   string
}

// Costs estimates infrastructure costs using infracost.
func (s *Service) Costs(req CostsRequest) (*CostsResponse, error) {
	logging.Info("Infracost breakdown...")

	// Check for dry-run mode first (before checking for tool availability)
	isDryRun := os.Getenv("IS_DRY_RUN") == "1"

	// Check if infracost is available (skip in dry-run mode)
	if !isDryRun {
		if _, err := exec.LookPath("infracost"); err != nil {
			return nil, errors.New(errors.ErrMissingTool, "infracost command not found, please install infracost")
		}
	}

	var args []string
	if req.UsePlanFile && req.PlanFile != "" {
		args = []string{"breakdown", fmt.Sprintf("--path=%s", req.PlanFile)}
	} else {
		args = []string{"breakdown", "--path=."}

		if req.DesiredStateFile != "" {
			args = append(args, fmt.Sprintf("--terraform-var-file=%s", req.DesiredStateFile))
		}
		if req.ConfigFile != "" {
			args = append(args, fmt.Sprintf("--terraform-var-file=%s", req.ConfigFile))
		}
		if req.AwsRegion != "" {
			args = append(args, fmt.Sprintf("--terraform-var=aws_provider_region=%s", req.AwsRegion))
		}
	}

	if isDryRun {
		logging.Info("[Dry-Run] Would execute: infracost %s", strings.Join(args, " "))
		logging.Info("[Dry-Run] Working directory: %s", req.WorkingDir)
		return &CostsResponse{
			Success: true,
			Output:  "[Dry-Run] Infracost execution skipped",
		}, nil
	}

	cmd := exec.Command("infracost", args...)
	cmd.Dir = req.WorkingDir

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		logging.Error("Infracost command failed: %v", err)
		return &CostsResponse{
			Success: false,
			Output:  outputStr,
			Error:   err.Error(),
		}, err
	}

	logging.Info("Cost estimation completed")
	return &CostsResponse{
		Success: true,
		Output:  outputStr,
	}, nil
}

// runTerraformCommand executes a terraform command and returns the output.
func (s *Service) runTerraformCommand(workingDir string, args ...string) (string, error) {
	return s.runTerraformCommandWithSecrets(workingDir, defaultSecretManifest(workingDir), args...)
}

func (s *Service) runTerraformCommandWithSecrets(workingDir, manifestPath string, args ...string) (string, error) {
	// Check if terraform is available
	tfPath, err := exec.LookPath("terraform")
	if err != nil {
		return "", errors.Newf(errors.ErrFail, "terraform command not found in PATH. Please install Terraform %s", s.tfVersion)
	}

	logging.Debug("Using terraform binary: %s", tfPath)

	// Build command string for logging
	cmdStr := fmt.Sprintf("terraform %s", strings.Join(args, " "))
	logging.Debug("Executing: %s", cmdStr)

	// Handle dry-run mode - just log the command without executing
	if s.isDryRun {
		logging.Info("[Dry-Run] Would execute: %s", cmdStr)
		logging.Info("[Dry-Run] Working directory: %s", workingDir)
		if s.awsProfile != "" {
			logging.Info("[Dry-Run] AWS_PROFILE=%s", s.awsProfile)
		}
		if s.awsRegion != "" {
			logging.Info("[Dry-Run] AWS_REGION=%s", s.awsRegion)
		}
		return "", nil
	}

	// Create command
	cmd := exec.Command("terraform", args...)
	cmd.Dir = workingDir

	// Resolve references only at execution time, never during assembly or dry runs.
	var secrets map[string]string
	if needsSecretVariables(args) {
		secrets, err = s.secretEnvironment(manifestPath)
		if err != nil {
			return "", err
		}
	}
	overrides := make(map[string]string, len(secrets)+2)
	for key, value := range secrets {
		overrides[key] = value
	}
	if s.awsProfile != "" {
		overrides["AWS_PROFILE"] = s.awsProfile
	}
	if s.awsRegion != "" {
		overrides["AWS_REGION"] = s.awsRegion
	}
	cmd.Env = mergeCommandEnvironment(os.Environ(), overrides)

	// Capture output
	output, err := cmd.CombinedOutput()
	outputStr := redactSecretOutput(string(output), secrets)

	if err != nil {
		logging.Error("Terraform command failed: %v", err)
		logging.Debug("Terraform output:\n%s", outputStr)
		return outputStr, errors.Wrapf(errors.ErrFail, err, "terraform command failed")
	}

	logging.Debug("Terraform output:\n%s", outputStr)
	return outputStr, nil
}
