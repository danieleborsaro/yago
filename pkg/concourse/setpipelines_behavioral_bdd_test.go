package concourse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/spf13/cobra"
)

// isFlagRequired checks cobra's required annotation on a named flag.
func isFlagRequired(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return false
	}
	annotations := flag.Annotations
	if annotations == nil {
		return false
	}
	_, required := annotations[cobra.BashCompOneRequiredFlag]
	return required
}

func findSubcommandByName(cmd *cobra.Command, name string) *cobra.Command {
	for _, child := range cmd.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func hasAlias(cmd *cobra.Command, alias string) bool {
	for _, a := range cmd.Aliases {
		if a == alias {
			return true
		}
	}
	return false
}

func TestSetPipelines_CommandRegistered_BDD(t *testing.T) {
	root := NewConcourseCommand()
	setPipelinesCmd := findSubcommandByName(root, "setpipelines")
	if setPipelinesCmd == nil {
		t.Fatalf("expected 'setpipelines' command to be registered under cci")
	}
}

func TestSetPipelines_CommandAliasParity_BDD(t *testing.T) {
	root := NewConcourseCommand()
	setPipelinesCmd := findSubcommandByName(root, "setpipelines")
	if setPipelinesCmd == nil {
		t.Fatalf("expected 'setpipelines' command to be registered under cci")
	}

	if !hasAlias(setPipelinesCmd, "p") {
		t.Fatalf("expected 'setpipelines' alias 'p' for gitops parity")
	}
}

func TestSetPipelines_CommandHasFlyAndModePlumbingFlags_BDD(t *testing.T) {
	root := NewConcourseCommand()
	setPipelinesCmd := findSubcommandByName(root, "setpipelines")
	if setPipelinesCmd == nil {
		t.Fatalf("expected 'setpipelines' command to be registered under cci")
	}

	requiredFlags := []string{
		"desiredstate-root",
		"configuration-root",
		"configuration-repo-workdir",
		"environment",
		"pipelines-repo-workdir",
		"pipelines-workdir",
		"save",
		"master-pipeline",
		"slave-pipelines",
		"local-pipelines",
		"target-name",
		"concourse-url",
		"fly-team",
		"fly-username",
		"fly-password-b64",
		"force",
	}

	for _, flagName := range requiredFlags {
		if setPipelinesCmd.Flags().Lookup(flagName) == nil {
			t.Fatalf("expected setpipelines to expose flag --%s", flagName)
		}
	}
}

func TestSetPipelines_CommandDocumentsDryRunFlyLifecycle_BDD(t *testing.T) {
	root := NewConcourseCommand()
	setPipelinesCmd := findSubcommandByName(root, "setpipelines")
	if setPipelinesCmd == nil {
		t.Fatalf("expected 'setpipelines' command to be registered under cci")
	}

	doc := strings.ToLower(setPipelinesCmd.Long)
	expectedKeywords := []string{"dry", "sync", "login", "update", "logout", "fly"}
	for _, keyword := range expectedKeywords {
		if !strings.Contains(doc, keyword) {
			t.Fatalf("expected setpipelines command documentation to mention '%s'", keyword)
		}
	}
}

// TestSetPipelines_FlyIdentityFlagsAreMandatory_BDD asserts that the three fly target
// identity flags (-n -u -T) are marked required.
//
// The three flags are required with empty defaults.
// -U/-P are optional; -P is validated at runtime when -U is set.
func TestSetPipelines_FlyIdentityFlagsAreMandatory_BDD(t *testing.T) {
	root := NewConcourseCommand()
	setPipelinesCmd := findSubcommandByName(root, "setpipelines")
	if setPipelinesCmd == nil {
		t.Fatalf("expected 'setpipelines' command to be registered under cci")
	}

	mandatoryFlags := []string{
		"target-name",   // -n: required in gitops; required in yago
		"concourse-url", // -u: required in gitops; required in yago
		"fly-team",      // -T: required in gitops; required in yago
	}

	for _, name := range mandatoryFlags {
		if !isFlagRequired(setPipelinesCmd, name) {
			t.Fatalf("expected flag --%s to be marked required in setpipelines (not optional with default)", name)
		}
	}

	// -U/-P are optional in both gitops and yago (SSO/pre-configured target supported).
	optionalFlags := []string{"fly-username", "fly-password-b64"}
	for _, name := range optionalFlags {
		if isFlagRequired(setPipelinesCmd, name) {
			t.Fatalf("expected flag --%s to be optional (not required) to preserve SSO path", name)
		}
	}
}

// TestSetPipelines_DocumentsIsManualRunEnvVar_BDD asserts IS_MANUAL_RUN is documented
// in the setpipelines command long description.
//
// IS_MANUAL_RUN=1 skips fly binary download (enabling local/manual execution).
func TestSetPipelines_DocumentsIsManualRunEnvVar_BDD(t *testing.T) {
	root := NewConcourseCommand()
	setPipelinesCmd := findSubcommandByName(root, "setpipelines")
	if setPipelinesCmd == nil {
		t.Fatalf("expected 'setpipelines' command to be registered under cci")
	}

	if !strings.Contains(setPipelinesCmd.Long, "IS_MANUAL_RUN") {
		t.Fatalf("expected setpipelines command documentation to document IS_MANUAL_RUN env var")
	}
}

// TestSetPipelines_IsManualRun_SkipsFlyDownloadInPlan_BDD asserts that when
// IsManualRun=true, the sync fly-lifecycle operation preview communicates that
// the fly download step is skipped (ManualRun mode).
func TestSetPipelines_IsManualRun_SkipsFlyDownloadInPlan_BDD(t *testing.T) {
	yagoRoot := yagoRootFromTestFile(t)
	chdirForTest(t, yagoRoot)
	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	pipelinesWorkdir := t.TempDir()
	templateFile := filepath.Join(pipelinesWorkdir, "pipelines", "check-workers-and-tools-oci.yaml")
	if err := os.MkdirAll(filepath.Dir(templateFile), 0o755); err != nil {
		t.Fatalf("failed to create pipeline template directory: %v", err)
	}
	if err := os.WriteFile(templateFile, []byte("resources: []\njobs: []\n"), 0o644); err != nil {
		t.Fatalf("failed to write pipeline template fixture: %v", err)
	}

	svc := NewService(".", false)
	resp, err := svc.SetPipelines(SetPipelinesRequest{
		DesiredStateRoot:  "tests/assets/4.2.0/desiredstates/concourse-cluster/desiredstate.yaml",
		ConfigurationRoot: "tests/assets/4.2.0/configurations/concourse-cluster/configuration.yaml",
		Environment:       "all",
		PipelinesWorkdir:  pipelinesWorkdir,
		IsLocal:           true,
		TargetName:        "my-target",
		ConcourseURL:      "https://ci.example.com",
		FlyTeam:           "main",
		FlyUsername:       "admin",
		FlyPasswordB64:    "cGFzc3dvcmQ=",
		IsManualRun:       true,
		IsDryRun:          true,
	})
	if err != nil {
		t.Fatalf("SetPipelines returned error: %v", err)
	}

	var syncOp *FlyOperation
	for i := range resp.Operations {
		if resp.Operations[i].Stage == "sync" {
			syncOp = &resp.Operations[i]
			break
		}
	}
	if syncOp == nil {
		t.Fatalf("expected a 'sync' operation in the fly lifecycle plan")
	}

	lower := strings.ToLower(syncOp.Preview)
	if !strings.Contains(lower, "manualrun") && !strings.Contains(lower, "manual_run") && !strings.Contains(lower, "skipping") {
		t.Fatalf("expected sync operation preview to communicate ManualRun/skipping when IsManualRun=true, got: %q", syncOp.Preview)
	}
}
