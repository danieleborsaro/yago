package desiredstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/jira"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
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
	// nil promotes everything
	SelectedPartIDs map[string]bool
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
	logging.Debug("Comparing desiredstates: %s vs %s", sourceFile, destFile)

	sourceDocument := core.NewGitOpsDocument()
	if err := sourceDocument.LoadGitOpsFile(sourceFile, true, nil); err != nil {
		return &ComparisonResult{}, errors.Wrapf(errors.ErrParse, err, "failed to load source file")
	}
	destDocument := core.NewGitOpsDocument()
	if err := destDocument.LoadGitOpsFile(destFile, true, nil); err != nil {
		return &ComparisonResult{}, errors.Wrapf(errors.ErrParse, err, "failed to load destination file")
	}
	return s.compareDocuments(sourceDocument, destDocument, sourceFile, destFile,
		isResolveToCommit, isRemoveMissing, isRollback, rollbackReason)
}

// takes loaded documents so each side can come from its own branch
func (s *Service) compareDocuments(sourceDocument, destDocument *core.GitOpsDocument, sourceFile, destFile string,
	isResolveToCommit, isRemoveMissing, isRollback bool, rollbackReason string) (*ComparisonResult, error) {
	result := &ComparisonResult{
		ComponentChanges: make([]ComponentChange, 0),
	}

	logging.Debug("Parsing source desiredstate components...")
	sourceContent := sourceDocument.GetContent()
	if sourceContent == nil || sourceContent.Data == nil {
		return result, errors.New(errors.ErrParse, "source desiredstate content is nil")
	}

	sourceParser := NewComponentParser(isResolveToCommit, false)
	err := sourceParser.ParseFromYAML(sourceContent.Data, sourceFile)
	if err != nil {
		return result, errors.Wrapf(errors.ErrParse, err, "failed to parse source components")
	}
	sourceComponents := sourceParser.GetManager().GetAllComponents()
	// 2.0.0 keeps components straight under desiredstate, which the parser doesn't read yet, and an
	// empty result used to look like a successful promotion with nothing to do
	if len(sourceComponents) == 0 {
		return result, errors.Newf(errors.ErrParse,
			"no components found under desiredstate.content.components in %s, compare and promote only support that layout for now",
			sourceFile)
	}

	logging.Debug("Parsing destination desiredstate components...")
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

	// components come out of a map, sorting keeps the prompts in the same order every run
	sort.Slice(result.ComponentChanges, func(i, j int) bool {
		return result.ComponentChanges[i].PartID < result.ComponentChanges[j].PartID
	})

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

func selectionIsValid(result *ComparisonResult, selected map[string]bool) (bool, string) {
	refused := 0
	for _, change := range result.ComponentChanges {
		if selected[change.PartID] && change.IsDowngrade && !change.IsRollback {
			refused++
		}
	}
	if refused > 0 {
		return false, fmt.Sprintf("Comparison failed: %d selected component(s) would be downgraded or removed - use --rollback if intentional", refused)
	}
	return true, ""
}

// PromoteDesiredState promotes a GitOps desired state.
// Enhanced in Phase 7 with component-level version comparison and validation.
func (s *Service) PromoteDesiredState(req PromoteRequest) (*PromoteResponse, error) {
	if err := validatePromoteRequest(req); err != nil {
		return nil, err
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

	p, err := s.preparePromotion(req, s.GetDocument())
	if err != nil {
		return &PromoteResponse{DestinationPath: req.DestinationFile}, err
	}
	return s.applyPromotion(req, p)
}

func validatePromoteRequest(req PromoteRequest) error {
	if req.DesiredStateFile == "" {
		return errors.NewParamError("desiredstate file must be specified")
	}
	if req.DestinationFile == "" {
		return errors.NewParamError("destination file must be specified")
	}
	if req.AWSRegion == "" {
		return errors.NewParamError("AWS region must be specified")
	}
	if req.SourceBranch == "" {
		return errors.NewParamError("source branch must be specified")
	}
	if req.TargetBranch == "" {
		return errors.NewParamError("target branch must be specified")
	}
	return nil
}

type promotion struct {
	source, dest *core.GitOpsDocument
	comparison   *ComparisonResult
	destination  *destinationSnapshot
}

// the destination as it was compared, an interactive promotion is confirmed a while later and the
// confirmation only covers this, so it's written on top of these bytes and refused if the files or the
// checkout changed since
type destinationSnapshot struct {
	head  uRepo.Head
	files map[string][]byte
}

func snapshotDestination(doc *core.GitOpsDocument, root string, head uRepo.Head) (*destinationSnapshot, error) {
	paths := []string{root}
	partFiles, err := doc.GetAllPartFiles(doc.GetPropertyPartsToLoad())
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to list destination part files")
	}
	for _, partCombo := range partFiles {
		for _, partFile := range partCombo {
			paths = append(paths, filepath.Join(doc.GetWorkdir(), partFile))
		}
	}

	snapshot := &destinationSnapshot{head: head, files: make(map[string][]byte, len(paths))}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to read destination file %s", path)
		}
		snapshot.files[path] = content
	}
	return snapshot, nil
}

func (d *destinationSnapshot) verify(root string) error {
	head, err := uRepo.WorktreeHead(root)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to read what the destination has checked out")
	}
	if head != d.head {
		return errors.Newf(errors.ErrFail,
			"the destination was on %s when it was compared and is on %s now, nothing was written, run the promotion again",
			d.head, head)
	}
	for _, path := range slices.Sorted(maps.Keys(d.files)) {
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(current, d.files[path]) {
			return errors.Newf(errors.ErrFail,
				"%s changed after it was compared, nothing was written, run the promotion again", path)
		}
	}
	return nil
}

// the source is loaded with all its parts before the target branch is checked out, so the same path on
// two branches is read on each, and the comparison works on the loaded documents without reading the
// source again
func (s *Service) preparePromotion(req PromoteRequest, source *core.GitOpsDocument) (*promotion, error) {
	logging.Info("Loading source desiredstate from %s...", req.SourceBranch)
	if err := uRepo.SwitchWorktree(req.DesiredStateFile, req.SourceBranch); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to check out the source branch")
	}
	if err := source.LoadGitOpsFile(req.DesiredStateFile, true, nil); err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to load source file")
	}

	logging.Info("Loading destination desiredstate from %s...", req.TargetBranch)
	if err := uRepo.SwitchWorktree(req.DestinationFile, req.TargetBranch); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to check out the target branch")
	}
	head, err := uRepo.WorktreeHead(req.DestinationFile)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to read what the destination has checked out")
	}
	dest := core.NewGitOpsDocument()
	if err := dest.LoadGitOpsFile(req.DestinationFile, true, nil); err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to load destination file")
	}
	destination, err := snapshotDestination(dest, req.DestinationFile, head)
	if err != nil {
		return nil, err
	}

	logging.Spaces()
	logging.Info("Comparing desiredstates component-by-component...")
	logging.Info("Source:      %s:%s", req.DesiredStateFile, req.SourceBranch)
	logging.Info("Destination: %s:%s", req.DestinationFile, req.TargetBranch)
	logging.Spaces()

	comparison, err := s.compareDocuments(source, dest, req.DesiredStateFile, req.DestinationFile,
		req.IsResolveToCommit, req.IsRemoveMissing, req.IsRollback, req.RollbackReason)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to compare desiredstates")
	}
	return &promotion{source: source, dest: dest, comparison: comparison, destination: destination}, nil
}

func (s *Service) applyPromotion(req PromoteRequest, p *promotion) (response *PromoteResponse, err error) {
	response = &PromoteResponse{DestinationPath: req.DestinationFile}
	sourceDocument, destDocument, comparisonResult := p.source, p.dest, p.comparison

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
	var isValid bool
	if req.IsForcePromotion {
		logging.Info("Force promotion enabled - skipping downgrade validation")
		isValid = true
	} else if req.SelectedPartIDs != nil {
		isValid, comparisonResult.ErrorMessage = selectionIsValid(comparisonResult, req.SelectedPartIDs)
		if !isValid {
			logging.Error("Comparison failed: %s", comparisonResult.ErrorMessage)
		}
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

		if req.IsRemoveMissing {
			for _, change := range comparisonResult.ComponentChanges {
				if change.IsRemoved {
					logging.Info("[Dry-Run] Would remove %s (was %s)", change.PartID, change.DestVersion)
				}
			}
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

	destMeta := destDocument.GetMeta()
	if destMeta == nil || destMeta.Data == nil {
		return response, errors.New(errors.ErrParse, "destination metadata is nil")
	}

	// parsed file by file so each component gets written back to the part it came from
	destManager, destParts, err := s.parseDestinationComponents(destDocument, destMeta.Data, req, p.destination.files)
	if err != nil {
		return response, err
	}

	// a component can be defined in the root and again in a part that overrides it
	destDefinitions := make(map[string][]*VersionedComponent)
	for _, comp := range componentsInOrder(destManager) {
		destDefinitions[comp.PartID] = append(destDefinitions[comp.PartID], comp)
	}

	// Track which components were updated and which part files need saving
	componentsUpdated := 0
	modifiedPartFiles := make(map[string]bool)
	var missing, ambiguous []string

	// Update destination components with source versions
	for _, sourceComp := range componentsInOrder(sourceParser.GetManager()) {
		if req.SelectedPartIDs != nil && !req.SelectedPartIDs[sourceComp.PartID] {
			logging.Debug("Component not selected, leaving as is: %s", sourceComp.PartID)
			continue
		}

		definitions := destDefinitions[sourceComp.PartID]
		if len(definitions) == 0 {
			missing = append(missing, sourceComp.PartID)
			continue
		}
		// which definition wins depends on part order, so rather than guess and maybe update the one
		// that's overridden, refuse
		if len(definitions) > 1 {
			var files []string
			changed := false
			for _, definition := range definitions {
				files = append(files, definition.PartFile)
				changed = changed || definition.Version != sourceComp.Version
			}
			if changed {
				ambiguous = append(ambiguous, fmt.Sprintf("%s (in %s)", sourceComp.PartID, strings.Join(files, " and ")))
			}
			continue
		}
		destComp := definitions[0]

		if destComp.Version == sourceComp.Version {
			logging.Debug("Component unchanged: %s (version: %s)", sourceComp.PartID, destComp.Version)
			continue
		}

		logging.Info("Updating component: %s (%s -> %s)",
			sourceComp.PartID, destComp.Version, sourceComp.Version)
		destComp.Version = sourceComp.Version
		destComp.Tag = sourceComp.Tag
		destComp.Branch = sourceComp.Branch
		destComp.IsLocked = false // Unlock when promoting

		componentsUpdated++
		modifiedPartFiles[destComp.PartFile] = true
	}

	// adding components isn't supported yet, fail before writing anything rather than quietly drop them
	if len(missing) > 0 {
		return response, errors.Newf(errors.ErrFail,
			"%d component(s) exist in the source but not in the destination, add them to %s first: %s",
			len(missing), req.DestinationFile, strings.Join(missing, ", "))
	}
	if len(ambiguous) > 0 {
		return response, errors.Newf(errors.ErrFail,
			"%d component(s) are defined in more than one destination file, keep one definition before promoting: %s",
			len(ambiguous), strings.Join(ambiguous, ", "))
	}

	removals := make(map[string][]*VersionedComponent)
	componentsRemoved := 0
	if req.IsRemoveMissing {
		inSource := make(map[string]bool)
		for _, comp := range sourceParser.GetManager().GetAllComponents() {
			inSource[comp.PartID] = true
		}
		for _, destComp := range componentsInOrder(destManager) {
			if inSource[destComp.PartID] {
				continue
			}
			if req.SelectedPartIDs != nil && !req.SelectedPartIDs[destComp.PartID] {
				continue
			}
			logging.Info("Removing component: %s (was %s)", destComp.PartID, destComp.Version)
			removals[destComp.PartFile] = append(removals[destComp.PartFile], destComp)
			componentsRemoved++
			modifiedPartFiles[destComp.PartFile] = true
		}
	}

	if componentsUpdated == 0 && componentsRemoved == 0 {
		logging.Info("No components need updating")
		logging.Spaces()

		response.Success = true
		response.SourceValidated = true
		return response, nil
	}

	logging.Spaces()
	logging.Info("Updating %d modified part file(s)...", len(modifiedPartFiles))

	var writes []pendingWrite
	for _, partFile := range sortedKeys(modifiedPartFiles) {
		data, path := destParts[partFile], filepath.Join(destDocument.GetWorkdir(), partFile)
		if partFile == req.DestinationFile {
			data, path = destMeta.Data, req.DestinationFile
		}

		logging.Info("Updating %s", partFile)
		if err := applyComponentChanges(data, destManager.GetComponentsByPartFile(partFile), removals[partFile]); err != nil {
			return response, errors.Wrapf(errors.ErrFail, err, "failed to update components in %s", partFile)
		}
		content, err := marshalDesiredState(data)
		if err != nil {
			return response, errors.Wrapf(errors.ErrFail, err, "failed to render %s", partFile)
		}
		writes = append(writes, pendingWrite{path: path, content: content})
	}

	if err := p.destination.verify(req.DestinationFile); err != nil {
		return response, err
	}
	if err := writeAllOrNothing(writes); err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "failed to save the destination")
	}

	logging.Spaces()
	logging.Info("Promotion completed successfully")
	logging.Info("Updated %d and removed %d component(s) across %d file(s)", componentsUpdated, componentsRemoved, len(modifiedPartFiles))

	response.FilesModified = true
	response.Success = true
	response.SourceValidated = true
	return response, nil
}

// part components are keyed by the path the root lists, which is relative to the document workdir
func (s *Service) parseDestinationComponents(destDocument *core.GitOpsDocument, rootData map[string]interface{}, req PromoteRequest, files map[string][]byte) (*ComponentManager, map[string]map[string]interface{}, error) {
	destParser := NewComponentParser(req.IsResolveToCommit, false)
	if err := destParser.ParseFromYAML(rootData, req.DestinationFile); err != nil {
		return nil, nil, errors.Wrapf(errors.ErrParse, err, "failed to parse destination components")
	}

	partFiles, err := destDocument.GetAllPartFiles(destDocument.GetPropertyPartsToLoad())
	if err != nil {
		return nil, nil, errors.Wrapf(errors.ErrParse, err, "failed to list destination part files")
	}

	parts := make(map[string]map[string]interface{})
	for _, partCombo := range partFiles {
		for _, partFile := range partCombo {
			content, err := parsePartFile(filepath.Join(destDocument.GetWorkdir(), partFile), files)
			if err != nil {
				return nil, nil, errors.Wrapf(errors.ErrParse, err, "failed to load destination part %s", partFile)
			}
			if err := destParser.ParseFromYAML(content, partFile); err != nil {
				return nil, nil, errors.Wrapf(errors.ErrParse, err, "failed to parse destination part %s", partFile)
			}
			parts[partFile] = content
		}
	}

	return destParser.GetManager(), parts, nil
}

func componentsInOrder(cm *ComponentManager) []*VersionedComponent {
	components := cm.GetAllComponents()
	sort.Slice(components, func(i, j int) bool { return components[i].PartID < components[j].PartID })
	return components
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func applyComponentChanges(data map[string]interface{}, updates, removals []*VersionedComponent) error {
	var componentsSection map[string]interface{}
	if desiredstate, ok := data["desiredstate"].(map[string]interface{}); ok {
		if content, ok := desiredstate["content"].(map[string]interface{}); ok {
			componentsSection, _ = content["components"].(map[string]interface{})
		}
	} else {
		componentsSection, _ = data["components"].(map[string]interface{})
	}
	if componentsSection == nil {
		return fmt.Errorf("components section not found")
	}

	for _, comp := range updates {
		if err := updateComponentInSection(componentsSection, comp); err != nil {
			return fmt.Errorf("failed to update component %s: %w", comp.PartID, err)
		}
	}
	for _, comp := range removals {
		if err := removeComponentFromSection(componentsSection, comp.PartID); err != nil {
			return fmt.Errorf("failed to remove component %s: %w", comp.PartID, err)
		}
	}
	return nil
}

// a PartID is the key path under components, the leaf goes and so does any parent it leaves empty,
// the sourcecode and artifacts sections always stay
func removeComponentFromSection(componentsSection map[string]interface{}, partID string) error {
	keys := strings.Split(partID, ".")
	if len(keys) < 2 {
		return fmt.Errorf("invalid PartID format: %s", partID)
	}

	maps := []map[string]interface{}{componentsSection}
	for _, key := range keys[:len(keys)-1] {
		next, ok := maps[len(maps)-1][key].(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s not found", partID)
		}
		maps = append(maps, next)
	}
	if _, ok := maps[len(maps)-1][keys[len(keys)-1]]; !ok {
		return fmt.Errorf("%s not found", partID)
	}

	for i := len(keys) - 1; i >= 1; i-- {
		delete(maps[i], keys[i])
		if len(maps[i]) > 0 {
			break
		}
	}
	return nil
}

type pendingWrite struct {
	path    string
	content []byte
}

// every file goes to a temp file next to it first, so a failed write leaves the destination untouched,
// only a failed rename part way through can leave it half written
func writeAllOrNothing(writes []pendingWrite) error {
	// writing to a symlink always went through to its target, so the target is what gets replaced and
	// the link stays, the replacement also keeps the target's permissions so a 0600 file stays private
	targets := make([]string, len(writes))
	modes := make([]os.FileMode, len(writes))
	for i, w := range writes {
		target, err := filepath.EvalSymlinks(w.path)
		if err != nil {
			return fmt.Errorf("resolving %s: %w", w.path, err)
		}
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("reading %s: %w", w.path, err)
		}
		targets[i], modes[i] = target, info.Mode().Perm()
	}

	temps := make([]string, 0, len(writes))
	cleanup := func() {
		for _, temp := range temps {
			_ = os.Remove(temp)
		}
	}

	for i, w := range writes {
		f, err := os.CreateTemp(filepath.Dir(targets[i]), "."+filepath.Base(targets[i])+".tmp-")
		if err != nil {
			cleanup()
			return fmt.Errorf("writing %s: %w", w.path, err)
		}
		temps = append(temps, f.Name())
		_, err = f.Write(w.content)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Chmod(f.Name(), modes[i])
		}
		if err != nil {
			cleanup()
			return fmt.Errorf("writing %s: %w", w.path, err)
		}
	}

	for i, w := range writes {
		if err := os.Rename(temps[i], targets[i]); err != nil {
			temps = temps[i:]
			cleanup()
			return fmt.Errorf("replacing %s, files before it were already replaced: %w", w.path, err)
		}
	}
	return nil
}

func marshalDesiredState(data map[string]interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func updateComponentInSection(componentsSection map[string]interface{}, comp *VersionedComponent) error {
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
		// empty values are written too, otherwise an old tag survives next to the new branch
		componentData["tag"] = comp.Tag
		componentData["branch"] = comp.Branch
		logging.Debug("Updated %s to branch %q tag %q", comp.PartID, comp.Branch, comp.Tag)

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

func parsePartFile(filePath string, files map[string][]byte) (map[string]interface{}, error) {
	data, ok := files[filePath]
	if !ok {
		return nil, fmt.Errorf("part file %s wasn't read when the destination was compared", filePath)
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
