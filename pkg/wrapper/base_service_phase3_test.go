package wrapper

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/danieleborsaro/yago/internal/core"
)

func yagoRootFromWrapperTestFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

type recordingAssembleHooks struct {
	calls         []string
	cacheObserved *CacheContext

	postCacheFn func(req AssembleRequest, response *AssembleResponse, ctx *CacheContext) error
}

func (h *recordingAssembleHooks) PreAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	h.calls = append(h.calls, "pre")
	return nil
}

func (h *recordingAssembleHooks) PostDesiredStateAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	h.calls = append(h.calls, "post_desiredstate")
	return nil
}

func (h *recordingAssembleHooks) PostConfigurationAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	h.calls = append(h.calls, "post_configuration")
	return nil
}

func (h *recordingAssembleHooks) PostAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	h.calls = append(h.calls, "post_assemble")
	return nil
}

func (h *recordingAssembleHooks) PostCache(req AssembleRequest, response *AssembleResponse, ctx *CacheContext) error {
	h.calls = append(h.calls, "post_cache")
	h.cacheObserved = ctx
	if h.postCacheFn != nil {
		return h.postCacheFn(req, response, ctx)
	}
	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}

func TestGetConfigRepoLocators_FallbackOnlyWhenDocumentMissing_Phase3(t *testing.T) {
	service := NewBaseService(".", false)
	locators := service.getConfigRepoLocators("terraform", "dev", nil)

	expected := []string{
		"desiredstate.content.environments.dev.configurations.terraform",
		"desiredstate.content.environments.dev.configurations.terraform.git",
		"desiredstate.configuration.dev.terraform",
		"desiredstate.configuration.dev.terraform.git",
	}

	if len(locators) != len(expected) {
		t.Fatalf("expected %d locator(s), got %d: %v", len(expected), len(locators), locators)
	}

	for index, value := range expected {
		if locators[index] != value {
			t.Fatalf("expected locator %d to be %q, got %q", index, value, locators[index])
		}
	}
}

func TestGetConfigRepoLocators_UsesSchemaTemplatesBeforeFallback_Phase3(t *testing.T) {
	service := NewBaseService(".", false)

	doc := core.NewGitOpsDocument()
	doc.SetSchemaAndNamespaceOverrides("1.0.0", "test-custom-paths")
	doc.GetMeta().Data = map[string]interface{}{
		"namespace": "test-custom-paths",
	}

	locators := service.getConfigRepoLocators("terraform", "all", doc)

	schemaExpected := []string{
		"desiredstate.meta.locators.primary.all.terraform",
		"desiredstate.meta.locators.primary.all.terraform.git",
		"desiredstate.meta.locators.legacy.all.terraform",
		"desiredstate.meta.locators.legacy.all.terraform.git",
	}

	if len(locators) < len(schemaExpected)+4 {
		t.Fatalf("expected schema locators + fallbacks, got %d entries: %v", len(locators), locators)
	}

	for index, expected := range schemaExpected {
		if locators[index] != expected {
			t.Fatalf("expected schema locator %d to be %q, got %q", index, expected, locators[index])
		}
	}

	fallbackExpected := []string{
		"desiredstate.content.environments.all.configurations.terraform",
		"desiredstate.content.environments.all.configurations.terraform.git",
		"desiredstate.configuration.all.terraform",
		"desiredstate.configuration.all.terraform.git",
	}

	for _, fallback := range fallbackExpected {
		if !containsString(locators, fallback) {
			t.Fatalf("expected fallback locator %q to be present; got %v", fallback, locators)
		}
	}
}

func TestSetAssembleHooks_NilResetsToNoOp_PhaseA(t *testing.T) {
	service := NewBaseService(".", false)
	service.SetAssembleHooks(nil)

	req := AssembleRequest{}
	response := &AssembleResponse{}

	if err := service.invokePreAssembleHook(req, response); err != nil {
		t.Fatalf("expected no-op pre hook to succeed, got error: %v", err)
	}
	if err := service.invokePostDesiredStateAssembleHook(req, response); err != nil {
		t.Fatalf("expected no-op post desiredstate hook to succeed, got error: %v", err)
	}
	if err := service.invokePostConfigurationAssembleHook(req, response); err != nil {
		t.Fatalf("expected no-op post configuration hook to succeed, got error: %v", err)
	}
	if err := service.invokePostAssembleHook(req, response); err != nil {
		t.Fatalf("expected no-op post assemble hook to succeed, got error: %v", err)
	}
	if err := service.invokePostCacheHook(req, response); err != nil {
		t.Fatalf("expected no-op post cache hook to succeed, got error: %v", err)
	}
}

func TestAssembleHooks_CallOrder_IsDeterministic_PhaseA(t *testing.T) {
	service := NewBaseService(".", false)
	hooks := &recordingAssembleHooks{}
	service.SetAssembleHooks(hooks)

	req := AssembleRequest{}
	response := &AssembleResponse{}

	if err := service.invokePreAssembleHook(req, response); err != nil {
		t.Fatalf("pre hook failed: %v", err)
	}
	if err := service.invokePostDesiredStateAssembleHook(req, response); err != nil {
		t.Fatalf("post desiredstate hook failed: %v", err)
	}
	if err := service.invokePostConfigurationAssembleHook(req, response); err != nil {
		t.Fatalf("post configuration hook failed: %v", err)
	}
	if err := service.invokePostAssembleHook(req, response); err != nil {
		t.Fatalf("post assemble hook failed: %v", err)
	}

	expected := []string{"pre", "post_desiredstate", "post_configuration", "post_assemble"}
	if len(hooks.calls) != len(expected) {
		t.Fatalf("expected %d hook calls, got %d (%v)", len(expected), len(hooks.calls), hooks.calls)
	}

	for i, exp := range expected {
		if hooks.calls[i] != exp {
			t.Fatalf("expected call %d to be %q, got %q", i, exp, hooks.calls[i])
		}
	}
}

func TestCacheAssembledFiles_InvokesPostCacheHook_PhaseA(t *testing.T) {
	service := NewBaseService(".", false)
	tmpDir := t.TempDir()

	service.document = core.NewGitOpsDocument()
	service.document.GetContent().Data = map[string]interface{}{
		"desiredstate": map[string]interface{}{"content": map[string]interface{}{"service": "demo"}},
	}

	hooks := &recordingAssembleHooks{}
	service.SetAssembleHooks(hooks)

	req := AssembleRequest{
		CacheDirectory: tmpDir,
		OutputFormat:   "yaml",
	}
	response := &AssembleResponse{
		DesiredStateAssembled:  true,
		ConfigurationAssembled: true,
		AssembledConfigurationContent: map[string]interface{}{
			"configuration": map[string]interface{}{"content": map[string]interface{}{"key": "value"}},
		},
	}

	if err := service.cacheAssembledFiles(req, response); err != nil {
		t.Fatalf("cacheAssembledFiles returned error: %v", err)
	}

	if len(hooks.calls) == 0 || hooks.calls[len(hooks.calls)-1] != "post_cache" {
		t.Fatalf("expected post_cache hook to be called last, got calls: %v", hooks.calls)
	}

	if hooks.cacheObserved == nil {
		t.Fatalf("expected cache context to be captured")
	}

	if hooks.cacheObserved.CacheDirectory != tmpDir {
		t.Fatalf("expected cache directory %q, got %q", tmpDir, hooks.cacheObserved.CacheDirectory)
	}

	if response.DesiredStateFile == "" || response.ConfigurationFile == "" {
		t.Fatalf("expected cached file paths to be set, got desiredstate=%q configuration=%q", response.DesiredStateFile, response.ConfigurationFile)
	}

	if _, err := os.Stat(response.DesiredStateFile); err != nil {
		t.Fatalf("expected desiredstate cache file to exist: %v", err)
	}

	if _, err := os.Stat(response.ConfigurationFile); err != nil {
		t.Fatalf("expected configuration cache file to exist: %v", err)
	}

	if filepath.Dir(response.DesiredStateFile) != tmpDir {
		t.Fatalf("expected desiredstate cache file under %q, got %q", tmpDir, response.DesiredStateFile)
	}

	if filepath.Dir(response.ConfigurationFile) != tmpDir {
		t.Fatalf("expected configuration cache file under %q, got %q", tmpDir, response.ConfigurationFile)
	}
}

func TestPrepareConfigurationDocumentForWrapper_EquivalentAcrossLoadStrategies_Phase3(t *testing.T) {
	service := NewBaseService(".", false)
	yagoRoot := yagoRootFromWrapperTestFile(t)
	configurationRoot := filepath.Join(yagoRoot, "tests", "assets", "2.0.0", "configurations", "concourse-cluster", "dev", "configuration.yaml")

	directFlowDoc := core.NewGitOpsDocument()
	if err := directFlowDoc.LoadMetadata(configurationRoot); err != nil {
		t.Fatalf("LoadMetadata failed for direct flow: %v", err)
	}

	cloneLikeDoc := core.NewGitOpsDocument()
	if err := cloneLikeDoc.LoadGitOpsFile(configurationRoot, false, nil); err != nil {
		t.Fatalf("LoadGitOpsFile failed for clone-like flow: %v", err)
	}

	if err := service.prepareConfigurationDocumentForWrapper(directFlowDoc, "concourse"); err != nil {
		t.Fatalf("prepareConfigurationDocumentForWrapper failed for direct flow: %v", err)
	}
	if err := service.prepareConfigurationDocumentForWrapper(cloneLikeDoc, "concourse"); err != nil {
		t.Fatalf("prepareConfigurationDocumentForWrapper failed for clone-like flow: %v", err)
	}

	if directFlowDoc.GetSchemaVersion() != cloneLikeDoc.GetSchemaVersion() {
		t.Fatalf("expected matching schema versions, got direct=%q cloneLike=%q", directFlowDoc.GetSchemaVersion(), cloneLikeDoc.GetSchemaVersion())
	}

	directContent := normalizeYAMLNode(directFlowDoc.GetContent().Data)
	cloneLikeContent := normalizeYAMLNode(cloneLikeDoc.GetContent().Data)
	if !reflect.DeepEqual(directContent, cloneLikeContent) {
		t.Fatalf("expected equivalent wrapper-assembled configuration content across load strategies")
	}
}
