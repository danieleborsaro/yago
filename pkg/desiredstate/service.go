package desiredstate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/jira"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	yamlutil "github.com/danieleborsaro/yago/internal/utils/yaml"
	"github.com/danieleborsaro/yago/pkg/aws"
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"gopkg.in/yaml.v3"
)

// Service provides business logic operations for desiredstate management.
// Service embeds wrapper.BaseService to inherit common wrapper functionality.
type Service struct {
	*wrapper.BaseService
}

// NewService creates a new desiredstate service instance.
func NewService(baseDir string, enableInterpolation bool) *Service {
	return &Service{
		BaseService: wrapper.NewBaseService(baseDir, enableInterpolation),
	}
}

// getMapKeys returns the keys of a map[string]interface{} for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ValidateDesiredState validates a GitOps desired state file.
func (s *Service) ValidateDesiredState(req wrapper.ValidateRequest) (*wrapper.ValidateResponse, error) {
	return s.Validate(req)
}

// AssembleDesiredState assembles a GitOps desired state file.
func (s *Service) AssembleDesiredState(req wrapper.AssembleRequest) (*wrapper.AssembleResponse, error) {
	return s.Assemble(req)
}

// PromoteRequest contains parameters for promotion operations.
type PromoteRequest struct {
	DesiredStateFile      string
	DestinationFile       string
	Environment           string
	AWSProfile            string
	AWSRegion             string
	SourceBranch          string
	TargetBranch          string
	IsResolveToCommit     bool
	IsCompareEnvironments bool
	IsForcePromotion      bool
	IsFailOnUnableToLock  bool
	IsDryRun              bool
	// New enhancement flags
	IsRemoveMissing bool   // Remove components from destination that don't exist in source
	IsRollback      bool   // Mark this promotion as an intentional rollback
	RollbackReason  string // Reason for the rollback
	IsInteractive   bool   // Interactive mode - prompt for each component change
}

// PromoteResponse contains the results of promotion operations.
type PromoteResponse struct {
	Success             bool
	SourceValidated     bool
	FilesModified       bool
	SourceSchemaVersion string
	DestinationPath     string
	ErrorMessage        string
	ComparisonResult    *ComparisonResult // Added for Phase 7
}

// ComparisonResult contains the results of comparing source and destination desiredstates.
type ComparisonResult struct {
	IsValid              bool
	TotalComponents      int
	ComponentsToUpgrade  int
	ComponentsUnchanged  int
	ComponentsDowngraded int
	ComponentChanges     []ComponentChange
	HasDowngrades        bool
	ErrorMessage         string
}

// ComponentChange represents a version change for a single component.
type ComponentChange struct {
	PartID            string
	ComponentType     string
	SourceVersion     string
	DestVersion       string
	IsUpgrade         bool
	IsDowngrade       bool
	IsUnchanged       bool
	IsLocked          bool
	IsRemoved         bool   // Component exists in dest but not in source
	VersionComparison string // e.g., "1.2.0 -> 1.3.0" or "sha256:abc... -> sha256:def..."
	ResolvedCommit    string // Git commit SHA if resolved
	IsRollback        bool   // This is an intentional rollback (not accidental downgrade)
	RollbackReason    string // Reason for rollback if IsRollback is true
}

// CompareDesiredStates compares source and destination desiredstates component-by-component.
// Returns comparison results including version changes and downgrade detection.
// Enhanced with rollback detection and missing component tracking.
func (s *Service) CompareDesiredStates(sourceFile, destFile, awsProfile, awsRegion string,
	isResolveToCommit, isRemoveMissing, isRollback bool, rollbackReason string) (*ComparisonResult, error) {
	result := &ComparisonResult{
		ComponentChanges: make([]ComponentChange, 0),
	}

	logging.Debug("Comparing desiredstates: %s vs %s", sourceFile, destFile)

	// Load and parse source desiredstate
	logging.Debug("Parsing source desiredstate components...")
	sourceDocument := core.NewGitOpsDocument()
	err := sourceDocument.LoadGitOpsFile(sourceFile, true, nil)
	if err != nil {
		return result, errors.Wrapf(errors.ErrParse, err, "failed to load source file")
	}

	sourceContent := sourceDocument.GetContent()
	if sourceContent == nil || sourceContent.Data == nil {
		return result, errors.New(errors.ErrParse, "source desiredstate content is nil")
	}

	sourceParser := NewComponentParser(isResolveToCommit, false)
	err = sourceParser.ParseFromYAML(sourceContent.Data, sourceFile)
	if err != nil {
		return result, errors.Wrapf(errors.ErrParse, err, "failed to parse source components")
	}
	sourceComponents := sourceParser.GetManager().GetAllComponents()

	// Load and parse destination desiredstate
	logging.Debug("Parsing destination desiredstate components...")
	destDocument := core.NewGitOpsDocument()
	err = destDocument.LoadGitOpsFile(destFile, true, nil)
	if err != nil {
		return result, errors.Wrapf(errors.ErrParse, err, "failed to load destination file")
	}

	destContent := destDocument.GetContent()
	if destContent == nil || destContent.Data == nil {
		return result, errors.New(errors.ErrParse, "destination desiredstate content is nil")
	}

	destParser := NewComponentParser(isResolveToCommit, false)
	err = destParser.ParseFromYAML(destContent.Data, destFile)
	if err != nil {
		return result, errors.Wrapf(errors.ErrParse, err, "failed to parse destination components")
	}
	destComponents := destParser.GetManager().GetAllComponents()

	// Initialize Git client for SHA resolution if needed
	var gitClient *GitClient
	if isResolveToCommit {
		gitClient = NewGitClient()
		logging.Info("Git SHA resolution enabled - will resolve tags/branches to commits")
	}

	// Create destination component map for quick lookup
	destMap := make(map[string]*VersionedComponent)
	for _, comp := range destComponents {
		destMap[comp.PartID] = comp
	}

	// Compare each source component with destination
	result.TotalComponents = len(sourceComponents)
	for _, sourceComp := range sourceComponents {
		destComp, exists := destMap[sourceComp.PartID]

		change := ComponentChange{
			PartID:        sourceComp.PartID,
			ComponentType: string(sourceComp.Type),
			SourceVersion: sourceComp.Version,
		}

		// Resolve Git commit SHA if requested and component is sourcecode
		if isResolveToCommit && gitClient != nil && sourceComp.Type == "sourcecode" {
			resolved, err := s.resolveGitCommit(gitClient, sourceComp)
			if err != nil {
				logging.Warn("Failed to resolve commit for %s: %v", sourceComp.PartID, err)
			} else {
				change.ResolvedCommit = resolved
				logging.Debug("Resolved %s to commit %s", sourceComp.PartID, resolved[:8])
			}
		}

		if !exists {
			// Component exists in source but not in destination - this is a new component
			change.DestVersion = "(new)"
			change.IsUpgrade = true
			change.VersionComparison = fmt.Sprintf("(new) -> %s", sourceComp.Version)
			result.ComponentsToUpgrade++
		} else {
			change.DestVersion = destComp.Version

			// Check if component is locked
			if destComp.IsLocked {
				change.IsLocked = true
				logging.Warn("Component %s is locked at version %s", destComp.PartID, destComp.Version)
			}

			// Compare versions
			if sourceComp.Version == destComp.Version {
				// Versions are identical
				change.IsUnchanged = true
				change.VersionComparison = fmt.Sprintf("%s (unchanged)", sourceComp.Version)
				result.ComponentsUnchanged++
			} else {
				// Versions differ - use intelligent comparison
				change.VersionComparison = fmt.Sprintf("%s -> %s", destComp.Version, sourceComp.Version)

				// Check if versions are hashes/digests (non-comparable)
				if strings.Contains(sourceComp.Version, "sha256:") ||
					strings.Contains(destComp.Version, "sha256:") ||
					len(sourceComp.Version) > 40 || len(destComp.Version) > 40 {
					// Digest or hash - treat as upgrade (different artifact)
					change.IsUpgrade = true
					result.ComponentsToUpgrade++
				} else {
					// Use semantic versioning comparison if possible, fall back to string comparison
					comparison := CompareVersions(destComp.Version, sourceComp.Version)

					if comparison < 0 {
						// dest < source: This is an upgrade
						change.IsUpgrade = true
						result.ComponentsToUpgrade++
					} else if comparison > 0 {
						// dest > source: This is a downgrade
						change.IsDowngrade = true
						result.ComponentsDowngraded++

						// Check if this is an intentional rollback
						if isRollback {
							change.IsRollback = true
							change.RollbackReason = rollbackReason
							logging.Info("⏮️  Intentional rollback: %s (%s -> %s) - %s",
								sourceComp.PartID, destComp.Version, sourceComp.Version, rollbackReason)
						} else {
							result.HasDowngrades = true
							logging.Warn("⚠️  Version downgrade detected: %s (%s -> %s) - use --rollback flag if intentional",
								sourceComp.PartID, destComp.Version, sourceComp.Version)
						}
					} else {
						// Versions are equivalent (should not happen as we already checked equality)
						change.IsUnchanged = true
						result.ComponentsUnchanged++
					}
				}
			}
		}

		result.ComponentChanges = append(result.ComponentChanges, change)
	} // Check for components in destination that don't exist in source
	removedCount := 0
	for _, destComp := range destComponents {
		found := false
		for _, sourceComp := range sourceComponents {
			if sourceComp.PartID == destComp.PartID {
				found = true
				break
			}
		}

		if !found {
			// Component exists in destination but not source
			change := ComponentChange{
				PartID:            destComp.PartID,
				ComponentType:     string(destComp.Type),
				SourceVersion:     "(removed)",
				DestVersion:       destComp.Version,
				IsRemoved:         true,
				VersionComparison: fmt.Sprintf("%s -> (removed)", destComp.Version),
			}

			if isRemoveMissing {
				// Will be removed - treat as downgrade for validation unless rollback mode
				change.IsDowngrade = !isRollback
				if isRollback {
					change.IsRollback = true
					change.RollbackReason = rollbackReason
					logging.Info("⏮️  Component removal (rollback): %s (was %s)", destComp.PartID, destComp.Version)
				} else {
					result.HasDowngrades = true
					logging.Warn("⚠️  Component will be removed: %s (was %s)", destComp.PartID, destComp.Version)
				}
				result.ComponentsDowngraded++
				removedCount++
			} else {
				// Won't be removed - just informational
				logging.Info("Component in destination only (will be kept): %s (version %s)", destComp.PartID, destComp.Version)
			}

			result.ComponentChanges = append(result.ComponentChanges, change)
		}
	}

	if removedCount > 0 && isRemoveMissing {
		logging.Warn("--remove-missing enabled: %d component(s) will be deleted from destination", removedCount)
	}

	// Determine if comparison is valid
	result.IsValid = !result.HasDowngrades
	if result.HasDowngrades {
		if isRollback {
			result.ErrorMessage = fmt.Sprintf("Rollback mode: %d component(s) will be downgraded or removed", result.ComponentsDowngraded)
			result.IsValid = true // Rollbacks are valid even with downgrades
		} else {
			result.ErrorMessage = fmt.Sprintf("Comparison failed: %d component(s) would be downgraded or removed - use --rollback if intentional", result.ComponentsDowngraded)
		}
	}

	logging.Debug("Comparison complete: %d components (%d upgrades, %d unchanged, %d downgrades)",
		result.TotalComponents, result.ComponentsToUpgrade, result.ComponentsUnchanged, result.ComponentsDowngraded)

	return result, nil
}

// PromoteDesiredState promotes a GitOps desired state.
// Enhanced in Phase 7 with component-level version comparison and validation.
func (s *Service) PromoteDesiredState(req PromoteRequest) (response *PromoteResponse, err error) {
	response = &PromoteResponse{DestinationPath: req.DestinationFile}

	// Validate all required parameters first
	if req.DesiredStateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}
	if req.DestinationFile == "" {
		return nil, errors.NewParamError("destination file must be specified")
	}
	if req.AWSRegion == "" {
		return nil, errors.NewParamError("AWS region must be specified")
	}
	if req.SourceBranch == "" {
		return nil, errors.NewParamError("source branch must be specified")
	}
	if req.TargetBranch == "" {
		return nil, errors.NewParamError("target branch must be specified")
	}

	// Log promotion parameters
	logging.Info("AWS profile:                 '%s'", req.AWSProfile)
	logging.Info("AWS region:                  '%s'", req.AWSRegion)
	logging.Info("Environment:                 '%s'", req.Environment)
	logging.Info("DesiredState source:         '%s'", req.DesiredStateFile)
	logging.Info("DesiredState destination:    '%s'", req.DestinationFile)
	logging.Info("Source branch:               '%s'", req.SourceBranch)
	logging.Info("Target branch:               '%s'", req.TargetBranch)
	logging.Info("Resolve refs:                '%t'", req.IsResolveToCommit)
	logging.Info("Compare, don't promote:      '%t'", req.IsCompareEnvironments)
	logging.Info("Force promotion:             '%t'", req.IsForcePromotion)
	logging.Info("Fail on no lock:             '%t'", req.IsFailOnUnableToLock)
	logging.Info("Dry run:                     '%t'", req.IsDryRun)

	logging.Spaces()

	// Load and validate source file (with parts assembled for component parsing)
	logging.Info("Loading source desiredstate...")
	sourceDocument := s.GetDocument()
	err = sourceDocument.LoadGitOpsFile(req.DesiredStateFile, true, nil)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to load source file")
	}

	// Load and validate destination file (with parts assembled for component parsing)
	logging.Info("Loading destination desiredstate...")
	destDocument := core.NewGitOpsDocument()
	err = destDocument.LoadGitOpsFile(req.DestinationFile, true, nil)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to load destination file")
	}

	logging.Spaces()

	// Phase 7: Perform component-level comparison
	logging.Info("Comparing desiredstates component-by-component...")
	logging.Info("Source:      %s:%s", req.DesiredStateFile, req.SourceBranch)
	logging.Info("Destination: %s:%s", req.DestinationFile, req.TargetBranch)
	logging.Spaces()

	comparisonResult, err := s.CompareDesiredStates(
		req.DesiredStateFile,
		req.DestinationFile,
		req.AWSProfile,
		req.AWSRegion,
		req.IsResolveToCommit,
		req.IsRemoveMissing,
		req.IsRollback,
		req.RollbackReason,
	)
	if err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "failed to compare desiredstates")
	}

	response.ComparisonResult = comparisonResult

	// Display comparison results
	logging.Info("Comparison Results:")
	logging.Info("  Total components:     %d", comparisonResult.TotalComponents)
	logging.Info("  Components to upgrade: %d", comparisonResult.ComponentsToUpgrade)
	logging.Info("  Components unchanged:  %d", comparisonResult.ComponentsUnchanged)
	logging.Info("  Components downgraded: %d", comparisonResult.ComponentsDowngraded)
	logging.Spaces()

	// Display component-by-component changes
	if len(comparisonResult.ComponentChanges) > 0 {
		logging.Info("Component Version Changes:")
		for _, change := range comparisonResult.ComponentChanges {
			if change.IsDowngrade {
				logging.Warn("  [DOWNGRADE] %s (%s): %s", change.PartID, change.ComponentType, change.VersionComparison)
			} else if change.IsUpgrade {
				logging.Info("  [UPGRADE]   %s (%s): %s", change.PartID, change.ComponentType, change.VersionComparison)
			} else if change.IsUnchanged {
				logging.Debug("  [UNCHANGED] %s (%s): %s", change.PartID, change.ComponentType, change.VersionComparison)
			}
		}
		logging.Spaces()
	}

	// Determine if promotion is valid
	isValid := true
	if req.IsForcePromotion {
		logging.Info("Force promotion enabled - skipping downgrade validation")
		isValid = true
	} else {
		isValid = comparisonResult.IsValid
		if !isValid {
			logging.Error("Comparison failed: %s", comparisonResult.ErrorMessage)
		}
	}

	logging.Info("Proposed promotion is valid: %t", isValid)

	if !isValid {
		return response, errors.New(errors.ErrFail, comparisonResult.ErrorMessage)
	}

	// Compare-only mode: stop here without promoting
	if req.IsCompareEnvironments {
		logging.Warn("Promotion disabled (compare-only mode), skipping")
		logging.Spaces()
		response.Success = true
		response.SourceValidated = true
		return response, nil
	}

	// Dry-run mode: show detailed preview without modifying files
	if req.IsDryRun {
		logging.Info("[Dry-Run] Promotion Preview:")
		logging.Info("[Dry-Run] Source:      %s:%s", req.DesiredStateFile, req.SourceBranch)
		logging.Info("[Dry-Run] Destination: %s:%s", req.DestinationFile, req.TargetBranch)
		logging.Spaces()

		logging.Info("[Dry-Run] Would update %d component(s) in destination:", comparisonResult.ComponentsToUpgrade)
		for _, change := range comparisonResult.ComponentChanges {
			if change.IsUpgrade {
				logging.Info("[Dry-Run]   %s (%s): %s", change.PartID, change.ComponentType, change.VersionComparison)
			}
		}

		if comparisonResult.ComponentsUnchanged > 0 {
			logging.Info("[Dry-Run] %d component(s) would remain unchanged", comparisonResult.ComponentsUnchanged)
		}

		logging.Spaces()
		logging.Info("[Dry-Run] Would write updated content to: %s", req.DestinationFile)
		logging.Info("[Dry-Run] No files were modified")
		logging.Spaces()

		response.Success = true
		response.SourceValidated = true
		return response, nil
	}

	// Perform actual promotion
	logging.Spaces()
	logging.Info("Promoting desiredstate component-by-component...")
	logging.Info("Source:      %s:%s", req.DesiredStateFile, req.SourceBranch)
	logging.Info("Destination: %s:%s", req.DestinationFile, req.TargetBranch)
	logging.Spaces()

	// Phase 7b/7c/9: Component-level promotion with file structure preservation
	// Parse source and destination to get individual component versions
	sourceContent := sourceDocument.GetContent()
	if sourceContent == nil || sourceContent.Data == nil {
		return response, errors.New(errors.ErrParse, "source content is nil")
	}

	destContent := destDocument.GetContent()
	if destContent == nil || destContent.Data == nil {
		return response, errors.New(errors.ErrParse, "destination content is nil")
	}

	sourceParser := NewComponentParser(req.IsResolveToCommit, false)
	err = sourceParser.ParseFromYAML(sourceContent.Data, req.DesiredStateFile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to parse source components")
	}

	destParser := NewComponentParser(req.IsResolveToCommit, false)
	err = destParser.ParseFromYAML(destContent.Data, req.DestinationFile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to parse destination components")
	}

	sourceComponents := sourceParser.GetManager().GetAllComponents()
	destManager := destParser.GetManager()

	// Track which components were updated and which part files need saving
	componentsUpdated := 0
	modifiedPartFiles := make(map[string]bool)

	// Update destination components with source versions
	for _, sourceComp := range sourceComponents {
		// Find matching component in destination
		destComp, err := destManager.GetComponent(sourceComp.PartFile, sourceComp.PartID)

		if err != nil {
			// Component doesn't exist in destination - it's new
			logging.Info("Adding new component: %s", sourceComp.PartID)
			newComp := *sourceComp
			// Use destination file path, not source file path
			newComp.PartFile = req.DestinationFile
			destManager.AddComponent(&newComp)
			destManager.IsUpdated = true
			componentsUpdated++
			modifiedPartFiles[req.DestinationFile] = true
		} else {
			// Component exists - check if version needs updating
			if destComp.Version != sourceComp.Version {
				logging.Info("Updating component: %s (%s -> %s)",
					sourceComp.PartID, destComp.Version, sourceComp.Version)

				// Update version in destination component
				destComp.Version = sourceComp.Version
				destComp.Tag = sourceComp.Tag
				destComp.Branch = sourceComp.Branch
				destComp.IsLocked = false // Unlock when promoting

				destManager.IsUpdated = true
				componentsUpdated++
				modifiedPartFiles[destComp.PartFile] = true
			} else {
				logging.Debug("Component unchanged: %s (version: %s)",
					sourceComp.PartID, destComp.Version)
			}
		}
	}

	if !destManager.IsUpdated {
		logging.Info("No components need updating")
		logging.Spaces()

		response.Success = true
		response.SourceValidated = true
		return response, nil
	}

	// Phase 7c/9: Update component versions in each modified part file
	logging.Spaces()
	logging.Info("Updating %d modified part file(s)...", len(modifiedPartFiles))

	// Get the destination's metadata to access part files
	destMeta := destDocument.GetMeta()
	if destMeta == nil || destMeta.Data == nil {
		return response, errors.New(errors.ErrParse, "destination metadata is nil")
	}

	// For single-file desiredstates (most common case), update the root file
	if len(modifiedPartFiles) == 1 {
		for partFile := range modifiedPartFiles {
			if partFile == req.DestinationFile {
				// This is the root file - update components in the metadata structure
				logging.Info("Updating root file: %s", partFile)

				// Update components in the destination metadata's content section
				err = s.updateComponentsInMetadata(destMeta.Data, destManager.GetAllComponents())
				if err != nil {
					return response, errors.Wrapf(errors.ErrFail, err, "failed to update components in metadata")
				}

				// Save the complete document (preserving schema, kind, namespace, meta)
				err = s.saveCompleteDocument(req.DestinationFile, destMeta.Data, req.IsDryRun)
				if err != nil {
					return response, errors.Wrapf(errors.ErrFail, err, "failed to save destination file")
				}
			} else {
				// This is a separate part file - Phase 9 multi-file support
				logging.Info("Updating part file: %s", partFile)

				// Load the individual part file
				partContent, err := s.loadPartFile(partFile)
				if err != nil {
					logging.Warn("Failed to load part file %s: %v", partFile, err)
					continue
				}

				// Update components in this part
				partComponents := destManager.GetComponentsByPartFile(partFile)
				err = s.updateComponentsInPartContent(partContent, partComponents)
				if err != nil {
					logging.Warn("Failed to update components in part %s: %v", partFile, err)
					continue
				}

				// Save the part file
				err = destDocument.SavePart(partFile, partContent, req.IsDryRun)
				if err != nil {
					return response, errors.Wrapf(errors.ErrFail, err, "failed to save part file %s", partFile)
				}
			}
		}
	} else {
		// Multi-file desiredstate - Phase 9
		logging.Info("Processing multi-file desiredstate...")
		for partFile := range modifiedPartFiles {
			logging.Info("Updating part file: %s", partFile)

			// Load the individual part file
			partContent, err := s.loadPartFile(partFile)
			if err != nil {
				logging.Warn("Failed to load part file %s: %v", partFile, err)
				continue
			}

			// Update components in this part
			partComponents := destManager.GetComponentsByPartFile(partFile)
			err = s.updateComponentsInPartContent(partContent, partComponents)
			if err != nil {
				logging.Warn("Failed to update components in part %s: %v", partFile, err)
				continue
			}

			// Save the part file
			err = destDocument.SavePart(partFile, partContent, req.IsDryRun)
			if err != nil {
				return response, errors.Wrapf(errors.ErrFail, err, "failed to save part file %s", partFile)
			}
		}
	}

	logging.Spaces()
	logging.Info("Promotion completed successfully")
	logging.Info("Updated %d component(s) across %d file(s)", componentsUpdated, len(modifiedPartFiles))

	response.FilesModified = true
	response.Success = true
	response.SourceValidated = true
	return response, nil
}

// updateComponentVersionInYAML updates a component's version in the YAML data structure.
// This navigates the YAML path and updates the appropriate version field(s).
func (s *Service) updateComponentVersionInYAML(yamlData map[string]interface{}, comp *VersionedComponent) error {
	// Navigate to desiredstate.content.components
	desiredstate, ok := yamlData["desiredstate"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("desiredstate section not found")
	}

	content, ok := desiredstate["content"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("content section not found")
	}

	components, ok := content["components"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("components section not found")
	}

	// Parse PartID to navigate to the component
	// PartID format examples:
	// - "sourcecode.infrastructure-repo"
	// - "artifacts.backend-service.docker.us-east-1"
	// - "artifacts.config-data.s3.us-east-1"

	parts := strings.Split(comp.PartID, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid PartID format: %s", comp.PartID)
	}

	section := parts[0]       // "sourcecode" or "artifacts"
	componentName := parts[1] // component name

	sectionData, ok := components[section].(map[string]interface{})
	if !ok {
		// Section doesn't exist yet - create it
		sectionData = make(map[string]interface{})
		components[section] = sectionData
	}

	componentData, ok := sectionData[componentName].(map[string]interface{})
	if !ok {
		// Component doesn't exist - this is a new component, skip for now
		// TODO: Implement adding new components to YAML
		logging.Debug("Component %s not found in YAML, skipping", comp.PartID)
		return nil
	}

	// Update the version based on component type
	switch comp.Type {
	case ComponentTypeSourcecode:
		// For sourcecode: update "tag" field
		if comp.Tag != "" {
			componentData["tag"] = comp.Tag
			logging.Debug("Updated %s tag to %s", comp.PartID, comp.Tag)
		}
		if comp.Branch != "" {
			componentData["branch"] = comp.Branch
			logging.Debug("Updated %s branch to %s", comp.PartID, comp.Branch)
		}

	case ComponentTypeDocker, ComponentTypeS3, ComponentTypeAMI:
		// For artifacts: navigate to type.region and update "tag" or "version_id"
		if len(parts) < 4 {
			return fmt.Errorf("invalid artifact PartID format: %s", comp.PartID)
		}

		artifactType := parts[2] // "docker", "s3", "ami", "web"
		region := parts[3]       // region name

		typeData, ok := componentData[artifactType].(map[string]interface{})
		if !ok {
			logging.Warn("Artifact type %s not found for %s", artifactType, comp.PartID)
			return nil
		}

		regionData, ok := typeData[region].(map[string]interface{})
		if !ok {
			logging.Warn("Region %s not found for %s", region, comp.PartID)
			return nil
		}

		// Update version field based on artifact type
		switch artifactType {
		case "docker":
			regionData["tag"] = comp.Version
			logging.Debug("Updated %s docker tag to %s", comp.PartID, comp.Version)
		case "s3":
			regionData["version_id"] = comp.Version
			logging.Debug("Updated %s s3 version_id to %s", comp.PartID, comp.Version)
		case "ami":
			regionData["name"] = comp.Version
			logging.Debug("Updated %s ami name to %s", comp.PartID, comp.Version)
		case "web":
			regionData["uri"] = comp.URL // For web artifacts, URL contains the full URI with version
			logging.Debug("Updated %s web uri to %s", comp.PartID, comp.URL)
		}

	default:
		logging.Warn("Unknown component type %s for %s", comp.Type, comp.PartID)
	}

	return nil
}

// savePartFile saves YAML data to a file, with optional dry-run mode.
func (s *Service) savePartFile(filePath string, yamlData map[string]interface{}, isDryRun bool) error {
	return yamlutil.SaveYAMLFile(filePath, yamlData, isDryRun)
}

// updateComponentsInMetadata updates component versions in the metadata YAML structure.
// This preserves the complete document structure including schema, kind, namespace, and meta sections.
func (s *Service) updateComponentsInMetadata(metadata map[string]interface{}, components []*VersionedComponent) error {
	// Navigate to desiredstate.content.components
	desiredstate, ok := metadata["desiredstate"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("desiredstate section not found in metadata")
	}

	content, ok := desiredstate["content"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("content section not found in desiredstate")
	}

	componentsSection, ok := content["components"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("components section not found in content")
	}

	// Update each component
	for _, comp := range components {
		err := s.updateComponentInSection(componentsSection, comp)
		if err != nil {
			logging.Warn("Failed to update component %s: %v", comp.PartID, err)
			// Continue with other components
		}
	}

	return nil
}

// updateComponentsInPartContent updates component versions in a part file's content.
func (s *Service) updateComponentsInPartContent(partContent map[string]interface{}, components []*VersionedComponent) error {
	// Check if this is a full desiredstate structure or just content
	var componentsSection map[string]interface{}

	if desiredstate, ok := partContent["desiredstate"].(map[string]interface{}); ok {
		// Full desiredstate structure
		if content, ok := desiredstate["content"].(map[string]interface{}); ok {
			if comp, ok := content["components"].(map[string]interface{}); ok {
				componentsSection = comp
			}
		}
	} else if components, ok := partContent["components"].(map[string]interface{}); ok {
		// Direct components section
		componentsSection = components
	}

	if componentsSection == nil {
		return fmt.Errorf("components section not found in part content")
	}

	// Update each component
	for _, comp := range components {
		err := s.updateComponentInSection(componentsSection, comp)
		if err != nil {
			logging.Warn("Failed to update component %s: %v", comp.PartID, err)
			// Continue with other components
		}
	}

	return nil
}

// updateComponentInSection updates a single component in a components section.
func (s *Service) updateComponentInSection(componentsSection map[string]interface{}, comp *VersionedComponent) error {
	// Parse PartID to navigate to the component
	// PartID format examples:
	// - "sourcecode.infrastructure-repo"
	// - "artifacts.backend-service.docker.us-east-1"

	parts := strings.Split(comp.PartID, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid PartID format: %s", comp.PartID)
	}

	section := parts[0]       // "sourcecode" or "artifacts"
	componentName := parts[1] // component name

	sectionData, ok := componentsSection[section].(map[string]interface{})
	if !ok {
		return fmt.Errorf("section %s not found", section)
	}

	componentData, ok := sectionData[componentName].(map[string]interface{})
	if !ok {
		return fmt.Errorf("component %s not found in section %s", componentName, section)
	}

	// Update the version based on component type
	switch comp.Type {
	case ComponentTypeSourcecode:
		// For sourcecode: update "tag" and/or "branch" field
		if comp.Tag != "" {
			componentData["tag"] = comp.Tag
			logging.Debug("Updated %s tag to %s", comp.PartID, comp.Tag)
		}
		if comp.Branch != "" {
			componentData["branch"] = comp.Branch
			logging.Debug("Updated %s branch to %s", comp.PartID, comp.Branch)
		}

	case ComponentTypeDocker, ComponentTypeS3, ComponentTypeAMI:
		// For artifacts: navigate to type.region and update version field
		if len(parts) < 4 {
			return fmt.Errorf("invalid artifact PartID format: %s", comp.PartID)
		}

		artifactType := parts[2] // "docker", "s3", "ami", "web"
		region := parts[3]       // region name

		typeData, ok := componentData[artifactType].(map[string]interface{})
		if !ok {
			return fmt.Errorf("artifact type %s not found for %s", artifactType, comp.PartID)
		}

		regionData, ok := typeData[region].(map[string]interface{})
		if !ok {
			return fmt.Errorf("region %s not found for %s", region, comp.PartID)
		}

		// Update version field based on artifact type
		switch artifactType {
		case "docker":
			regionData["tag"] = comp.Version
			logging.Debug("Updated %s docker tag to %s", comp.PartID, comp.Version)
		case "s3":
			regionData["version_id"] = comp.Version
			logging.Debug("Updated %s s3 version_id to %s", comp.PartID, comp.Version)
		case "ami":
			regionData["name"] = comp.Version
			logging.Debug("Updated %s ami name to %s", comp.PartID, comp.Version)
		case "web":
			regionData["uri"] = comp.URL
			logging.Debug("Updated %s web uri to %s", comp.PartID, comp.URL)
		}
	}

	return nil
}

// saveCompleteDocument saves a complete document including all top-level fields.
// This preserves schema, kind, namespace, and meta sections.
func (s *Service) saveCompleteDocument(filePath string, documentData map[string]interface{}, isDryRun bool) error {
	return yamlutil.SaveYAMLFile(filePath, documentData, isDryRun)
}

// loadPartFile loads a part file and returns its YAML content.
func (s *Service) loadPartFile(filePath string) (map[string]interface{}, error) {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read part file %s: %w", filePath, err)
	}

	// Parse YAML
	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", filePath, err)
	}

	return result, nil
}

// SchemaRequest contains parameters for schema operations.
type SchemaRequest struct {
	SchemaVersion string
	ShowAll       bool
	ListVersions  bool
}

// SchemaResponse contains the results of schema operations.
type SchemaResponse struct {
	Versions      []string
	SchemaContent map[string]string
	ErrorMessage  string
}

// PrintSchema retrieves schema information.
func (s *Service) PrintSchema(req SchemaRequest) (*SchemaResponse, error) {
	if req.SchemaVersion == "" && !req.ShowAll && !req.ListVersions {
		return nil, errors.NewParamError("please specify an option")
	}

	response := &SchemaResponse{SchemaContent: make(map[string]string)}
	response.Versions = s.GetSupportedSchemaVersions()

	if req.SchemaVersion != "" {
		handler := s.GetHandler()
		schemaContent, err := handler.GetSchemaContent(schema.SchemaVersion(req.SchemaVersion), schema.SchemaTypeDesiredStateMeta)
		if err != nil {
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to get schema content")
		}

		jsonBytes, err := json.MarshalIndent(schemaContent, "", "  ")
		if err != nil {
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to marshal schema")
		}

		response.SchemaContent[req.SchemaVersion] = string(jsonBytes)
	}

	if req.ShowAll {
		handler := s.GetHandler()
		for _, versionStr := range response.Versions {
			schemaContent, err := handler.GetSchemaContent(schema.SchemaVersion(versionStr), schema.SchemaTypeDesiredStateMeta)
			if err != nil {
				continue
			}

			jsonBytes, err := json.MarshalIndent(schemaContent, "", "  ")
			if err != nil {
				continue
			}

			response.SchemaContent[versionStr] = string(jsonBytes)
		}
	}

	return response, nil
}

// CheckVersionsRequest contains parameters for version checking operations.
type CheckVersionsRequest struct {
	DesiredStateFile     string
	Environment          string
	AWSProfile           string
	AWSRegion            string
	IsResolveToCommit    bool
	IsFailOnUnableToLock bool
	IsRetrieveJiraIssue  bool
	IsResolveToJiraIssue bool
}

// CheckVersionsResponse contains the results of version checking operations.
type CheckVersionsResponse struct {
	Success            bool
	TotalComponents    int
	LockedComponents   int
	UnlockedComponents int
	ErrorMessage       string
}

// CheckVersions checks the lock status of components in a desiredstate file.
// This implementation parses components and checks their lock status.
func (s *Service) CheckVersions(req CheckVersionsRequest) (response *CheckVersionsResponse, err error) {
	response = &CheckVersionsResponse{}

	// Validate all required parameters first
	if req.DesiredStateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}
	if req.AWSRegion == "" {
		return nil, errors.NewParamError("AWS region must be specified")
	}

	// Log parameters
	logging.Info("AWS profile:     '%s'", req.AWSProfile)
	logging.Info("AWS region:      '%s'", req.AWSRegion)
	logging.Info("Environment:     '%s'", req.Environment)
	logging.Info("DesiredState:    '%s'", req.DesiredStateFile)
	logging.Info("Resolve refs:    '%t'", req.IsResolveToCommit)
	logging.Info("Fail on no lock: '%t'", req.IsFailOnUnableToLock)
	logging.Info("Retrieve Jira:   '%t'", req.IsRetrieveJiraIssue)
	logging.Info("Resolve Jira:    '%t'", req.IsResolveToJiraIssue)

	logging.Spaces()

	// Load and validate desiredstate file
	logging.Info("Loading desiredstate...")
	document := s.GetDocument()
	err = document.LoadGitOpsFile(req.DesiredStateFile, true, nil)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate file")
	}

	logging.Spaces()

	// Parse components from desiredstate
	logging.Info("Parsing components...")

	// Use the loaded document's content
	dsContent := document.GetContent()
	if dsContent == nil || dsContent.Data == nil {
		return response, errors.New(errors.ErrParse, "desiredstate content is nil")
	}

	// Debug: log top-level keys in the YAML data
	logging.Debug("YAML data top-level keys: %v", getMapKeys(dsContent.Data))

	// Create component parser
	componentParser := NewComponentParser(req.IsResolveToCommit, req.IsFailOnUnableToLock)

	// Parse components from the YAML data
	err = componentParser.ParseFromYAML(dsContent.Data, req.DesiredStateFile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to parse components")
	}

	componentManager := componentParser.GetManager()

	// Get all components
	allComponents := componentManager.GetAllComponents()
	logging.Debug("Found %d total components", len(allComponents))

	// Filter by environment if needed
	components := FilterComponentsByEnvironment(allComponents, req.Environment)
	logging.Debug("After environment filter: %d components", len(components))

	logging.Spaces()
	logging.Info("Computing locks...")
	logging.Spaces()

	// Count locked vs unlocked components
	totalComponents := 0
	lockedComponents := 0
	unlockedComponents := 0

	for _, component := range components {
		totalComponents++

		if component.IsLocked {
			lockedComponents++
			logging.Debug("  [LOCKED]   %s: %s", component.PartID, component.Version)
		} else {
			unlockedComponents++
			logging.Info("  [UNLOCKED] %s: %s", component.PartID, component.Version)
		}

		// Optionally retrieve Jira issue numbers
		if req.IsRetrieveJiraIssue {
			jiraIssue, err := component.GetJiraIssueNumber(req.IsResolveToJiraIssue)
			if err != nil {
				logging.Warn("Failed to get Jira issue for %s: %v", component.PartID, err)
			} else if jiraIssue != "" && jiraIssue != "NOT FOUND" {
				logging.Debug("  Jira issue: %s", jiraIssue)
			}
		}
	}

	logging.Spaces()

	// Report summary
	if unlockedComponents > 0 {
		logging.Warn("Found %d unlocked components", unlockedComponents)
		if req.IsFailOnUnableToLock {
			return response, errors.New(errors.ErrFail, "unlocked components found and fail-on-unable-to-lock is enabled")
		}
	} else {
		logging.Info("All components are locked")
	}

	response.Success = true
	response.TotalComponents = totalComponents
	response.LockedComponents = lockedComponents
	response.UnlockedComponents = unlockedComponents

	return response, nil
}

// LockRequest contains parameters for lock operations.
type LockRequest struct {
	DesiredStateFile     string
	Environment          string
	AWSProfile           string
	AWSRegion            string
	IsResolveToCommit    bool
	IsFailOnUnableToLock bool
	IsDryRun             bool
}

// LockResponse contains the results of lock operations.
type LockResponse struct {
	Success          bool
	LockedComponents int
	FilesModified    bool
	ErrorMessage     string
}

// Lock locks component versions to their current values.
// This is a simplified Phase 1 implementation.
// Full component-level locking requires additional architecture.
func (s *Service) Lock(req LockRequest) (response *LockResponse, err error) {
	response = &LockResponse{}

	// Validate all required parameters first
	if req.DesiredStateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}
	if req.AWSRegion == "" {
		return nil, errors.NewParamError("AWS region must be specified")
	}

	// Log parameters
	logging.Info("AWS profile:     '%s'", req.AWSProfile)
	logging.Info("AWS region:      '%s'", req.AWSRegion)
	logging.Info("Environment:     '%s'", req.Environment)
	logging.Info("DesiredState:    '%s'", req.DesiredStateFile)
	logging.Info("Resolve refs:    '%t'", req.IsResolveToCommit)
	logging.Info("Fail on no lock: '%t'", req.IsFailOnUnableToLock)
	logging.Info("Dry run:         '%t'", req.IsDryRun)

	logging.Spaces()

	// Load and validate desiredstate file
	logging.Info("Loading desiredstate...")
	document := s.GetDocument()
	err = document.LoadGitOpsFile(req.DesiredStateFile, true, nil)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate file")
	}

	logging.Spaces()
	logging.Info("Computing locks...")
	logging.Spaces()

	if req.IsDryRun {
		logging.Info("[Dry-Run] Would compute locked versions for unlocked components")
		logging.Info("[Dry-Run] Would save updated desiredstate file")
		logging.Spaces()
		response.Success = true
		return response, nil
	}

	// TODO: Implement component-level locking
	// This requires:
	// 1. Parse desiredstate into VersionedComponents
	// 2. For each unlocked component, query ECR/S3 for current version
	// 3. Lock component to that version
	// 4. Save updated desiredstate file
	//
	// See docs/desiredstate-commands-assessment.md for full implementation plan

	// Parse components from desiredstate
	dsContent := document.GetContent()
	if dsContent == nil || dsContent.Data == nil {
		return response, errors.New(errors.ErrParse, "desiredstate content is nil")
	}

	componentParser := NewComponentParser(req.IsResolveToCommit, req.IsFailOnUnableToLock)
	err = componentParser.ParseFromYAML(dsContent.Data, req.DesiredStateFile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to parse components")
	}
	componentManager := componentParser.GetManager()

	// Initialize AWS managers
	ecrManager, err := aws.NewECRManager(req.AWSRegion, req.AWSProfile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "failed to create ECR manager")
	}
	s3Manager, err := aws.NewS3Manager(req.AWSRegion, req.AWSProfile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "failed to create S3 manager")
	}
	componentManager.SetAWSManagers(ecrManager, s3Manager)

	lockedComponents := 0
	filesModified := false
	componentsToUpdate := make(map[string]string) // yamlPath -> newVersion

	// Lock all unlocked components
	for _, component := range componentManager.GetAllComponents() {
		if !component.IsLocked {
			err := component.Lock(ecrManager, s3Manager)
			if err != nil {
				logging.Warn("Unable to lock component %s: %v", component.PartID, err)
				if req.IsFailOnUnableToLock {
					return response, errors.Wrapf(errors.ErrFail, err, "failed to lock component %s", component.PartID)
				}
				continue
			}

			// Track the update
			if component.YAMLPathToLock != "" && component.Version != component.OldVersion {
				componentsToUpdate[component.YAMLPathToLock] = component.Version
				filesModified = true
				logging.Info("  [LOCKED] %s: %s -> %s", component.PartID, component.OldVersion, component.Version)
			}
		}
		if component.IsLocked {
			lockedComponents++
		}
	}

	// Save updated desiredstate file if any locks were applied
	if filesModified {
		logging.Spaces()
		logging.Info("Saving updated desiredstate file...")

		if req.IsDryRun {
			logging.Info("[Dry-Run] Would update %d components in %s", len(componentsToUpdate), req.DesiredStateFile)
			for yamlPath, newVersion := range componentsToUpdate {
				logging.Info("[Dry-Run]   %s -> %s", yamlPath, newVersion)
			}
		} else {
			// Apply updates to YAML data
			updatedCount, err := yamlutil.UpdateMultipleComponents(dsContent.Data, componentsToUpdate, req.IsDryRun)
			if err != nil {
				return response, errors.Wrapf(errors.ErrFail, err, "failed to update YAML data")
			}

			if updatedCount > 0 {
				// Save the updated YAML file
				err = yamlutil.SaveYAMLFile(req.DesiredStateFile, dsContent.Data, req.IsDryRun)
				if err != nil {
					return response, errors.Wrapf(errors.ErrFail, err, "failed to save desiredstate file")
				}

				logging.Info("Updated %d components in %s", updatedCount, req.DesiredStateFile)
			} else {
				logging.Warn("No components were actually updated in the YAML")
			}
		}
	} else {
		logging.Info("No components needed locking")
	}

	response.Success = true
	response.LockedComponents = lockedComponents
	response.FilesModified = filesModified
	return response, nil
}

// UnlockRequest contains parameters for unlock operations.
type UnlockRequest struct {
	DesiredStateFile     string
	Environment          string
	AWSProfile           string
	AWSRegion            string
	IsResolveToCommit    bool
	IsFailOnUnableToLock bool
	IsDryRun             bool
}

// UnlockResponse contains the results of unlock operations.
type UnlockResponse struct {
	Success            bool
	UnlockedComponents int
	FilesModified      bool
	ErrorMessage       string
}

// Unlock unlocks components and updates them to latest versions.
// This is a simplified Phase 1 implementation.
// Full component-level unlocking requires additional architecture.
func (s *Service) Unlock(req UnlockRequest) (response *UnlockResponse, err error) {
	response = &UnlockResponse{}

	// Validate all required parameters first
	if req.DesiredStateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}
	if req.AWSRegion == "" {
		return nil, errors.NewParamError("AWS region must be specified")
	}

	// Log parameters
	logging.Info("AWS profile:     '%s'", req.AWSProfile)
	logging.Info("AWS region:      '%s'", req.AWSRegion)
	logging.Info("Environment:     '%s'", req.Environment)
	logging.Info("DesiredState:    '%s'", req.DesiredStateFile)
	logging.Info("Resolve refs:    '%t'", req.IsResolveToCommit)
	logging.Info("Fail on no lock: '%t'", req.IsFailOnUnableToLock)
	logging.Info("Dry run:         '%t'", req.IsDryRun)

	logging.Spaces()

	// Load and validate desiredstate file
	logging.Info("Loading desiredstate...")
	document := s.GetDocument()
	err = document.LoadGitOpsFile(req.DesiredStateFile, true, nil)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate file")
	}

	logging.Spaces()
	logging.Info("Computing latest versions...")
	logging.Spaces()

	if req.IsDryRun {
		logging.Info("[Dry-Run] Would query ECR/S3 for latest component versions")
		logging.Info("[Dry-Run] Would unlock components and update to latest")
		logging.Info("[Dry-Run] Would save updated desiredstate file")
		logging.Spaces()
		response.Success = true
		return response, nil
	}

	// Parse components from desiredstate
	dsContent := document.GetContent()
	if dsContent == nil || dsContent.Data == nil {
		return response, errors.New(errors.ErrParse, "desiredstate content is nil")
	}

	componentParser := NewComponentParser(req.IsResolveToCommit, req.IsFailOnUnableToLock)
	err = componentParser.ParseFromYAML(dsContent.Data, req.DesiredStateFile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to parse components")
	}
	componentManager := componentParser.GetManager()

	// Initialize AWS managers
	ecrManager, err := aws.NewECRManager(req.AWSRegion, req.AWSProfile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "failed to create ECR manager")
	}
	s3Manager, err := aws.NewS3Manager(req.AWSRegion, req.AWSProfile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "failed to create S3 manager")
	}
	componentManager.SetAWSManagers(ecrManager, s3Manager)

	unlockedComponents := 0
	filesModified := false
	componentsToUpdate := make(map[string]string) // yamlPath -> newVersion

	// Unlock all locked components
	for _, component := range componentManager.GetAllComponents() {
		if component.IsLocked {
			err := component.Unlock(ecrManager, s3Manager)
			if err != nil {
				logging.Warn("Unable to unlock component %s: %v", component.PartID, err)
				if req.IsFailOnUnableToLock {
					return response, errors.Wrapf(errors.ErrFail, err, "failed to unlock component %s", component.PartID)
				}
				continue
			}

			// Track the update
			if component.YAMLPathToUnlock != "" && component.Version != component.OldVersion {
				componentsToUpdate[component.YAMLPathToUnlock] = component.Version
				filesModified = true
				logging.Info("  [UNLOCKED] %s: %s -> %s", component.PartID, component.OldVersion, component.Version)
			}
		}
		if !component.IsLocked {
			unlockedComponents++
		}
	}

	// Save updated desiredstate file if any unlocks were applied
	if filesModified {
		logging.Spaces()
		logging.Info("Saving updated desiredstate file...")

		if req.IsDryRun {
			logging.Info("[Dry-Run] Would update %d components in %s", len(componentsToUpdate), req.DesiredStateFile)
			for yamlPath, newVersion := range componentsToUpdate {
				logging.Info("[Dry-Run]   %s -> %s", yamlPath, newVersion)
			}
		} else {
			// Apply updates to YAML data
			updatedCount, err := yamlutil.UpdateMultipleComponents(dsContent.Data, componentsToUpdate, req.IsDryRun)
			if err != nil {
				return response, errors.Wrapf(errors.ErrFail, err, "failed to update YAML data")
			}

			if updatedCount > 0 {
				// Save the updated YAML file
				err = yamlutil.SaveYAMLFile(req.DesiredStateFile, dsContent.Data, req.IsDryRun)
				if err != nil {
					return response, errors.Wrapf(errors.ErrFail, err, "failed to save desiredstate file")
				}

				logging.Info("Updated %d components in %s", updatedCount, req.DesiredStateFile)
			} else {
				logging.Warn("No components were actually updated in the YAML")
			}
		}
	} else {
		logging.Info("No components needed unlocking")
	}

	response.Success = true
	response.UnlockedComponents = unlockedComponents
	response.FilesModified = filesModified
	return response, nil
}

// LockStatusComponent represents a single component's lock status
type LockStatusComponent struct {
	PartID        string
	ComponentType string
	Version       string
	IsLocked      bool
	LockReason    string // Why it's considered locked (e.g., "SHA commit", "SHA256 digest", "version path")
}

// LockStatusResponse contains the results of lock status check
type LockStatusResponse struct {
	LockedComponents   []LockStatusComponent
	UnlockedComponents []LockStatusComponent
	TotalComponents    int
}

// GetLockStatus analyzes a desiredstate file and returns which components are locked vs unlocked
func (s *Service) GetLockStatus(desiredstateFile, environment, awsProfile, awsRegion string) (*LockStatusResponse, error) {
	response := &LockStatusResponse{
		LockedComponents:   make([]LockStatusComponent, 0),
		UnlockedComponents: make([]LockStatusComponent, 0),
	}

	// Validate required parameters
	if desiredstateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}

	// Load and parse desiredstate file
	logging.Debug("Loading desiredstate file: %s", desiredstateFile)
	document := core.NewGitOpsDocument()
	err := document.LoadGitOpsFile(desiredstateFile, true, nil)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate file")
	}

	dsContent := document.GetContent()
	if dsContent == nil || dsContent.Data == nil {
		return nil, errors.New(errors.ErrParse, "desiredstate content is nil")
	}

	// Parse components
	logging.Debug("Parsing components from desiredstate...")
	parser := NewComponentParser(false, false) // Don't resolve or lock during status check
	err = parser.ParseFromYAML(dsContent.Data, desiredstateFile)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to parse components")
	}

	// Get component manager with handlers
	manager := parser.GetManager()
	components := manager.GetAllComponents()

	logging.Debug("Analyzing %d components...", len(components))

	// Analyze each component's lock status
	for _, component := range components {
		statusComp := LockStatusComponent{
			PartID:        component.PartID,
			ComponentType: string(component.Type),
			Version:       component.Version,
		}

		// Check if version is locked using the base helper function
		statusComp.IsLocked = IsVersionLocked(component.Version)

		if statusComp.IsLocked {
			// Determine lock reason based on component type and version format
			switch component.Type {
			case ComponentTypeDocker:
				if strings.Contains(component.Version, "sha256:") {
					statusComp.LockReason = "SHA256 image digest"
				} else {
					statusComp.LockReason = "explicit tag"
				}
			case ComponentTypeSourcecode, ComponentTypeGitHub:
				if len(component.Version) == 40 && isHexString(component.Version) {
					statusComp.LockReason = "SHA commit hash"
				} else {
					statusComp.LockReason = "explicit commit reference"
				}
			case ComponentTypeS3:
				statusComp.LockReason = "versioned path"
			default:
				statusComp.LockReason = "explicit version"
			}
			response.LockedComponents = append(response.LockedComponents, statusComp)
		} else {
			response.UnlockedComponents = append(response.UnlockedComponents, statusComp)
		}
	}

	response.TotalComponents = len(components)
	return response, nil
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// GetIssuesRequest contains parameters for Jira issue extraction.
type GetIssuesRequest struct {
	DesiredStateFile     string
	Environment          string
	AWSProfile           string
	AWSRegion            string
	IsResolveToCommit    bool
	IsResolveToJiraIssue bool
	IsFailOnUnableToLock bool
	JiraSaveDir          string
	JiraSaveFile         string
}

// GetIssuesResponse contains the results of Jira issue extraction.
type GetIssuesResponse struct {
	Success      bool
	TotalIssues  int
	SavedToFile  string
	ErrorMessage string
}

// GetIssues extracts Jira issue numbers from desiredstate components.
// This is a simplified Phase 1 implementation.
// Full Jira integration requires component parsing and Jira API access.
func (s *Service) GetIssues(req GetIssuesRequest) (response *GetIssuesResponse, err error) {
	response = &GetIssuesResponse{}

	// Validate all required parameters first
	if req.DesiredStateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}
	if req.AWSRegion == "" {
		return nil, errors.NewParamError("AWS region must be specified")
	}

	// Log parameters
	logging.Info("AWS profile:     '%s'", req.AWSProfile)
	logging.Info("AWS region:      '%s'", req.AWSRegion)
	logging.Info("Environment:     '%s'", req.Environment)
	logging.Info("DesiredState:    '%s'", req.DesiredStateFile)
	logging.Info("Resolve refs:    '%t'", req.IsResolveToCommit)
	logging.Info("Resolve Jira:    '%t'", req.IsResolveToJiraIssue)
	logging.Info("Fail on no lock: '%t'", req.IsFailOnUnableToLock)
	if req.JiraSaveDir != "" {
		logging.Info("Jira save dir:   '%s'", req.JiraSaveDir)
	}
	if req.JiraSaveFile != "" {
		logging.Info("Jira save file:  '%s'", req.JiraSaveFile)
	}

	logging.Spaces()

	// Load and validate desiredstate file
	logging.Info("Loading desiredstate...")
	document := s.GetDocument()
	err = document.LoadGitOpsFile(req.DesiredStateFile, true, nil)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate file")
	}

	logging.Spaces()
	logging.Info("Extracting Jira issue numbers...")
	logging.Spaces()

	// Parse components from desiredstate
	dsContent := document.GetContent()
	if dsContent == nil || dsContent.Data == nil {
		return response, errors.New(errors.ErrParse, "desiredstate content is nil")
	}

	componentParser := NewComponentParser(req.IsResolveToCommit, req.IsFailOnUnableToLock)
	err = componentParser.ParseFromYAML(dsContent.Data, req.DesiredStateFile)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to parse components")
	}
	componentManager := componentParser.GetManager()

	// Get all components
	allComponents := componentManager.GetAllComponents()
	logging.Debug("Found %d total components", len(allComponents))

	// Filter by environment if needed
	components := FilterComponentsByEnvironment(allComponents, req.Environment)
	logging.Debug("After environment filter: %d components", len(components))

	// Extract Jira issues from components
	var jiraIssues []jira.IssueInfo
	totalIssues := 0

	for _, component := range components {
		issueNumber, err := component.GetJiraIssueNumber(req.IsResolveToJiraIssue)
		if err != nil {
			logging.Warn("Failed to get Jira issue for component %s: %v", component.PartID, err)
			continue
		}

		if issueNumber != "" && issueNumber != "NOT FOUND" {
			totalIssues++
			logging.Info("  %s: %s (version: %s)", component.PartID, issueNumber, component.Version)

			jiraIssues = append(jiraIssues, jira.IssueInfo{
				Component: component.PartID,
				Issue:     issueNumber,
				URL:       component.URL,
				Version:   component.Version,
			})
		}
	}

	logging.Spaces()

	if totalIssues == 0 {
		logging.Info("No Jira issues found in components")
	} else {
		logging.Info("Found %d Jira issues across %d components", totalIssues, len(components))
	}

	// Save to file if requested
	var savedFilePath string
	if req.JiraSaveDir != "" {
		logging.Spaces()
		logging.Info("Saving Jira issues to file...")

		savedFilePath, err = jira.SaveIssuesToFile(jiraIssues, req.JiraSaveDir, req.JiraSaveFile)
		if err != nil {
			return response, errors.Wrapf(errors.ErrFail, err, "failed to save Jira issues to file")
		}

		logging.Info("Jira issues saved to: %s", savedFilePath)
	}

	logging.Spaces()

	response.Success = true
	response.TotalIssues = totalIssues
	response.SavedToFile = savedFilePath

	return response, nil
}

// resolveGitCommit resolves a Git tag or branch to a commit SHA for a source code component
func (s *Service) resolveGitCommit(gitClient *GitClient, comp *VersionedComponent) (string, error) {
	if comp.Type != "sourcecode" {
		return "", fmt.Errorf("not a sourcecode component")
	}

	// If already a commit SHA, return as-is
	if gitClient.IsCommitSHA(comp.Version) {
		return comp.Version, nil
	}

	// Try to resolve tag first, then branch
	var resolved string
	var err error

	if comp.Tag != "" {
		resolved, err = gitClient.ResolveTagToCommit(comp.URL, comp.Tag)
		if err == nil {
			return resolved, nil
		}
		logging.Debug("Failed to resolve tag %s: %v", comp.Tag, err)
	}

	if comp.Branch != "" {
		resolved, err = gitClient.ResolveBranchToCommit(comp.URL, comp.Branch)
		if err == nil {
			return resolved, nil
		}
		logging.Debug("Failed to resolve branch %s: %v", comp.Branch, err)
	}

	// Fall back to trying the version string itself
	resolved, err = gitClient.ResolveRefToCommit(comp.URL, comp.Version)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s to commit: %w", comp.Version, err)
	}

	return resolved, nil
}
