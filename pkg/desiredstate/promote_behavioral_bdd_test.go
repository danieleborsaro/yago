package desiredstate

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type PromoteBehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// part paths resolve against the working directory, so the files live under
// the repo root and are referenced by relative path, the fixture is its own git
// repo on main, so promoting from main to main never checks anything out and a
// test can never switch branches in the yago checkout itself
type promoteFixture struct {
	t    *testing.T
	base string
}

func newPromoteFixture(t *testing.T) *promoteFixture {
	t.Helper()
	root := yagoRootFromTestFile(t)
	chdirForTest(t, root)
	abs, err := os.MkdirTemp(root, ".promote-test-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(abs) })
	base, err := filepath.Rel(root, abs)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	// keeps the developer's own git config out, commit signing breaks these
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(name+"_NAME", "yago")
		t.Setenv(name+"_EMAIL", "yago@example.com")
	}
	f := &promoteFixture{t: t, base: base}
	f.git("init", "-q", "-b", "main")
	return f
}

func (f *promoteFixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", f.base}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func artifactsYAML(tags map[string]string) string {
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("  content:\n    components:\n      artifacts:\n")
	for _, name := range names {
		fmt.Fprintf(&b, "        %s:\n          docker:\n            eu-west-1:\n              image: example/%s\n              tag: %s\n",
			name, name, tags[name])
	}
	return b.String()
}

func (f *promoteFixture) singleFile(name string, tags map[string]string) string {
	f.t.Helper()
	return f.singleFileWithContent(name, artifactsYAML(tags))
}

// content is everything under desiredstate, indented two spaces
func (f *promoteFixture) singleFileWithContent(name, content string) string {
	f.t.Helper()
	path := filepath.Join(f.base, name+".yaml")
	f.write(path, fmt.Sprintf("---\nschema: 4.2.0\nnamespace: legacy\ndesiredstate:\n  meta:\n    parts:\n      self: %s\n%s",
		path, content))
	return path
}

// every component lives in the part, the root only lists it
func (f *promoteFixture) withPart(name string, tags map[string]string) (root, part string) {
	f.t.Helper()
	root = filepath.Join(f.base, name, "desiredstate.yaml")
	part = filepath.Join(f.base, name, "app.yaml")
	f.write(root, fmt.Sprintf("---\nschema: 4.2.0\nnamespace: legacy\ndesiredstate:\n  meta:\n    parts:\n      self: %s\n      app: %s\n",
		root, part))
	f.write(part, "---\ndesiredstate:\n"+artifactsYAML(tags))
	return root, part
}

func (f *promoteFixture) write(path, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatalf("WriteFile: %v", err)
	}
}

func (f *promoteFixture) read(path string) string {
	f.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}

// both branches are the fixture's current one, so nothing is checked out
func promoteRequest(src, dst string) PromoteRequest {
	return PromoteRequest{
		DesiredStateFile: src,
		DestinationFile:  dst,
		AWSRegion:        "eu-west-1",
		SourceBranch:     "main",
		TargetBranch:     "main",
	}
}

func assertTag(t *testing.T, yamlText, name, want string) {
	t.Helper()
	marker := "image: example/" + name + "\n"
	i := strings.Index(yamlText, marker)
	if i < 0 {
		t.Fatalf("component %s not found in:\n%s", name, yamlText)
	}
	line := strings.TrimSpace(strings.SplitN(yamlText[i+len(marker):], "\n", 2)[0])
	if line != "tag: "+want {
		t.Errorf("%s: got %q, want %q", name, line, "tag: "+want)
	}
}

func TestPromote_UpdatesRootFile_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Promote new versions into a single file desired state",
		CurrentImpl:     "PromoteDesiredState updates the root file through applyComponentChanges",
		ExpectedOutcome: "Every changed component in the destination takes the source version",
		Rationale:       "This is the core job of promote",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a destination behind the source on two components
	f := newPromoteFixture(t)
	src := f.singleFile("src", map[string]string{"api": "2.1.0", "web": "1.5.0"})
	dst := f.singleFile("dst", map[string]string{"api": "2.0.0", "web": "1.0.0"})

	// When: the source is promoted
	resp, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, dst))

	// Then: both components move to the source versions
	if err != nil {
		t.Fatalf("PromoteDesiredState: %v", err)
	}
	if !resp.Success || !resp.FilesModified {
		t.Errorf("response = %+v, want Success and FilesModified", resp)
	}
	got := f.read(dst)
	assertTag(t, got, "api", "2.1.0")
	assertTag(t, got, "web", "1.5.0")
}

func TestPromote_UpdatesComponentsInPartFile_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Promote components that live in a part file",
		CurrentImpl:     "parseDestinationComponents keys each component to the part file it was read from",
		ExpectedOutcome: "The part file gets the new version and the root file is untouched",
		Rationale:       "Components in parts used to be looked for in the root file, so they were never written",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: source and destination keep their components in a part file
	f := newPromoteFixture(t)
	src, _ := f.withPart("src", map[string]string{"api": "2.1.0"})
	dst, dstPart := f.withPart("dst", map[string]string{"api": "2.0.0"})
	rootBefore := f.read(dst)

	// When: the source is promoted
	_, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, dst))

	// Then: the part is updated and the root isn't
	if err != nil {
		t.Fatalf("PromoteDesiredState: %v", err)
	}
	assertTag(t, f.read(dstPart), "api", "2.1.0")
	if got := f.read(dst); got != rootBefore {
		t.Errorf("root file changed although no component lives in it:\n%s", got)
	}
}

func TestPromote_NewComponentFailsBeforeWriting_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Refuse to promote when the source has a component the destination doesn't",
		CurrentImpl:     "PromoteDesiredState collects missing components and fails before saving anything",
		ExpectedOutcome: "An error naming the missing component, with the destination unchanged",
		Rationale:       "New components used to be logged as added, then left out while promote reported success",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a worker component that only exists in the source
	f := newPromoteFixture(t)
	src := f.singleFile("src", map[string]string{"api": "2.1.0", "worker": "3.0.0"})
	dst := f.singleFile("dst", map[string]string{"api": "2.0.0"})
	before := f.read(dst)

	// When: the source is promoted
	_, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, dst))

	// Then: it fails, names the worker and writes nothing
	if err == nil || !strings.Contains(err.Error(), "artifacts.worker.docker.eu-west-1") {
		t.Fatalf("error = %v, want one naming artifacts.worker.docker.eu-west-1", err)
	}
	if got := f.read(dst); got != before {
		t.Errorf("destination changed after a failed promotion:\n%s", got)
	}
}

func TestPromote_SelectedComponentsOnly_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Promote only the components in SelectedPartIDs",
		CurrentImpl:     "PromoteDesiredState skips source components missing from req.SelectedPartIDs",
		ExpectedOutcome: "Selected components move, the rest keep their destination version",
		Rationale:       "Interactive promote relies on this to leave declined components alone",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: two components behind the source, only api selected
	f := newPromoteFixture(t)
	src := f.singleFile("src", map[string]string{"api": "2.1.0", "web": "1.5.0"})
	dst := f.singleFile("dst", map[string]string{"api": "2.0.0", "web": "1.0.0"})
	req := promoteRequest(src, dst)
	req.SelectedPartIDs = map[string]bool{"artifacts.api.docker.eu-west-1": true}

	// When: the source is promoted
	_, err := NewService(".", false).PromoteDesiredState(req)

	// Then: only api moves
	if err != nil {
		t.Fatalf("PromoteDesiredState: %v", err)
	}
	got := f.read(dst)
	assertTag(t, got, "api", "2.1.0")
	assertTag(t, got, "web", "1.0.0")
}

func TestPromote_DowngradesOnlyBlockWhenSelected_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Only a selected downgrade blocks a selective promotion",
		CurrentImpl:     "selectionIsValid checks downgrades among the selected components only",
		ExpectedOutcome: "Skipping the downgrade lets the upgrade through, selecting it is refused",
		Rationale:       "Skipping a downgrade interactively used to still fail the whole promotion",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: api is an upgrade and web a downgrade
	f := newPromoteFixture(t)
	src := f.singleFile("src", map[string]string{"api": "2.1.0", "web": "0.9.0"})
	dst := f.singleFile("dst", map[string]string{"api": "2.0.0", "web": "1.0.0"})
	service := NewService(".", false)

	// When: everything is promoted
	_, err := service.PromoteDesiredState(promoteRequest(src, dst))

	// Then: the downgrade blocks it
	if err == nil {
		t.Fatal("promotion with a downgrade succeeded, want it refused")
	}

	// When: only the upgrade is selected
	req := promoteRequest(src, dst)
	req.SelectedPartIDs = map[string]bool{"artifacts.api.docker.eu-west-1": true}
	_, err = service.PromoteDesiredState(req)

	// Then: the upgrade goes through and web stays put
	if err != nil {
		t.Fatalf("promoting only the upgrade: %v", err)
	}
	got := f.read(dst)
	assertTag(t, got, "api", "2.1.0")
	assertTag(t, got, "web", "1.0.0")

	// When: only the downgrade is selected
	req.SelectedPartIDs = map[string]bool{"artifacts.web.docker.eu-west-1": true}
	_, err = service.PromoteDesiredState(req)

	// Then: it's refused
	if err == nil {
		t.Fatal("promoting a selected downgrade succeeded, want it refused")
	}
}

func TestInteractivePromotion_Answers_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Interactive promote does exactly what was answered",
		CurrentImpl:     "runInteractivePromotion reads one fresh line per question and passes the selection on",
		ExpectedOutcome: "Declined components stay put, Enter means no and input ending early aborts",
		Rationale:       "Declined components used to be promoted, and Enter at the last question reused an earlier y",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name    string
		answers string
		wantErr string
		wantAPI string
		wantWeb string
	}{
		{name: "apply all", answers: "y\na\ny\n", wantAPI: "2.1.0", wantWeb: "1.5.0"},
		{name: "declined component is left alone", answers: "y\ny\ny\nn\ny\n", wantAPI: "2.1.0", wantWeb: "1.0.0"},
		{name: "enter at final confirmation cancels", answers: "y\ny\ny\ny\n\n", wantAPI: "2.0.0", wantWeb: "1.0.0"},
		{name: "input ending mid review aborts", answers: "y\ny\ny\n", wantErr: "before input ended", wantAPI: "2.0.0", wantWeb: "1.0.0"},
		{name: "no input at all aborts", answers: "", wantErr: "before input ended", wantAPI: "2.0.0", wantWeb: "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: api and web are both behind the source
			f := newPromoteFixture(t)
			src := f.singleFile("src", map[string]string{"api": "2.1.0", "web": "1.5.0"})
			dst := f.singleFile("dst", map[string]string{"api": "2.0.0", "web": "1.0.0"})

			// When: the promotion runs with these answers
			err := runInteractivePromotion(NewService(".", false), promoteRequest(src, dst),
				strings.NewReader(tt.answers), io.Discard)

			// Then: the destination matches the answers
			if tt.wantErr == "" && err != nil {
				t.Fatalf("runInteractivePromotion: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
			got := f.read(dst)
			assertTag(t, got, "api", tt.wantAPI)
			assertTag(t, got, "web", tt.wantWeb)
		})
	}
}

func TestPromote_ComponentDefinedTwiceIsRefused_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Refuse to promote a component the destination defines in more than one file",
		CurrentImpl:     "PromoteDesiredState groups destination definitions by PartID and errors when a changed one has several",
		ExpectedOutcome: "An error naming the component and both files, with neither file changed",
		Rationale:       "Which definition wins depends on part order, promote used to update the shadowed one and report success",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a destination defining api in the root and again in an overriding part
	f := newPromoteFixture(t)
	src := f.singleFile("src", map[string]string{"api": "2.1.0"})
	root := filepath.Join(f.base, "dst", "desiredstate.yaml")
	part := filepath.Join(f.base, "dst", "app.yaml")
	f.write(root, fmt.Sprintf("---\nschema: 4.2.0\nnamespace: legacy\ndesiredstate:\n  meta:\n    parts:\n      self: %s\n      app: %s\n%s",
		root, part, artifactsYAML(map[string]string{"api": "1.0.0"})))
	f.write(part, "---\ndesiredstate:\n"+artifactsYAML(map[string]string{"api": "2.0.0"}))
	rootBefore, partBefore := f.read(root), f.read(part)

	// When: the source is promoted
	_, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, root))

	// Then: it's refused and nothing is written
	if err == nil || !strings.Contains(err.Error(), "defined in more than one destination file") {
		t.Fatalf("error = %v, want one about api being defined twice", err)
	}
	if f.read(root) != rootBefore || f.read(part) != partBefore {
		t.Error("destination changed after a refused promotion")
	}
}
