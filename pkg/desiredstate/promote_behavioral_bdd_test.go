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

	"gopkg.in/yaml.v3"
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

func sourcecodeYAML(branch, tag string) string {
	return fmt.Sprintf("  content:\n    components:\n      sourcecode:\n        infra:\n          url: git@example.com:example/infra.git\n          branch: %q\n          tag: %q\n          path: .\n",
		branch, tag)
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

func sourcecodeRef(t *testing.T, yamlText string) (branch, tag string) {
	t.Helper()
	var doc struct {
		Desiredstate struct {
			Content struct {
				Components struct {
					Sourcecode map[string]struct {
						Branch string `yaml:"branch"`
						Tag    string `yaml:"tag"`
					} `yaml:"sourcecode"`
				} `yaml:"components"`
			} `yaml:"content"`
		} `yaml:"desiredstate"`
	}
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		t.Fatalf("parsing destination: %v", err)
	}
	infra := doc.Desiredstate.Content.Components.Sourcecode["infra"]
	return infra.Branch, infra.Tag
}

func TestPromote_SourcecodeRefIsReplaced_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Promoting a sourcecode component replaces both its branch and its tag",
		CurrentImpl:     "updateComponentInSection writes the source branch and tag, empty ones included",
		ExpectedOutcome: "The destination ends up on exactly the source ref",
		Rationale:       "Empty values used to be skipped, leaving an old tag next to the new branch",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name                string
		srcBranch, srcTag   string
		dstBranch, dstTag   string
		wantBranch, wantTag string
	}{
		{name: "branch replaces tag", srcBranch: "main", dstTag: "1.0.0", wantBranch: "main"},
		{name: "tag replaces branch", srcTag: "1.1.0", dstBranch: "main", wantTag: "1.1.0"},
		{name: "tag replaces tag", srcTag: "1.1.0", dstTag: "1.0.0", wantTag: "1.1.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the destination on a different ref to the source
			f := newPromoteFixture(t)
			src := f.singleFileWithContent("src", sourcecodeYAML(tt.srcBranch, tt.srcTag))
			dst := f.singleFileWithContent("dst", sourcecodeYAML(tt.dstBranch, tt.dstTag))
			req := promoteRequest(src, dst)
			req.IsForcePromotion = true

			// When: the source is promoted
			_, err := NewService(".", false).PromoteDesiredState(req)

			// Then: the destination has exactly the source ref
			if err != nil {
				t.Fatalf("PromoteDesiredState: %v", err)
			}
			branch, tag := sourcecodeRef(t, f.read(dst))
			if branch != tt.wantBranch || tag != tt.wantTag {
				t.Errorf("branch=%q tag=%q, want branch=%q tag=%q", branch, tag, tt.wantBranch, tt.wantTag)
			}
		})
	}
}

func TestPromote_RemoveMissing_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "--remove-missing deletes destination components the source no longer has",
		CurrentImpl:     "PromoteDesiredState collects destination only components and removeComponentFromSection deletes them",
		ExpectedOutcome: "The component is gone after a rollback, and without --rollback the promotion is refused",
		Rationale:       "The flag used to warn that components would be deleted and then never delete them",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name        string
		rollback    bool
		selected    map[string]bool
		wantErr     bool
		wantRemoved bool
	}{
		{name: "rollback removes it", rollback: true, wantRemoved: true},
		{name: "without rollback it's refused", wantErr: true},
		{name: "not selected it stays", rollback: true, selected: map[string]bool{"artifacts.api.docker.eu-west-1": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the destination has a worker the source dropped
			f := newPromoteFixture(t)
			src := f.singleFile("src", map[string]string{"api": "2.1.0"})
			dst := f.singleFile("dst", map[string]string{"api": "2.0.0", "worker": "3.0.0"})
			req := promoteRequest(src, dst)
			req.IsRemoveMissing = true
			req.IsRollback = tt.rollback
			req.SelectedPartIDs = tt.selected

			// When: the source is promoted with --remove-missing
			_, err := NewService(".", false).PromoteDesiredState(req)

			// Then: worker is removed only when that was allowed and selected
			if tt.wantErr {
				if err == nil {
					t.Fatal("removal without --rollback succeeded, want it refused")
				}
				return
			}
			if err != nil {
				t.Fatalf("PromoteDesiredState: %v", err)
			}
			got := f.read(dst)
			assertTag(t, got, "api", "2.1.0")
			if removed := !strings.Contains(got, "example/worker"); removed != tt.wantRemoved {
				t.Errorf("worker removed = %v, want %v:\n%s", removed, tt.wantRemoved, got)
			}
		})
	}
}

func TestPromote_FailedWriteChangesNothing_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "A promotion that can't write every file changes none of them",
		CurrentImpl:     "writeAllOrNothing writes temp files for every part and only renames once all are written",
		ExpectedOutcome: "An error, with every destination file as it was",
		Rationale:       "Files used to be saved one by one, so a failure part way left a half promoted destination",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a destination split over two parts, the second in a directory that can't be written to
	f := newPromoteFixture(t)
	mk := func(name, apiTag, webTag string) (root, apiPart, webPart string) {
		root = filepath.Join(f.base, name, "desiredstate.yaml")
		apiPart = filepath.Join(f.base, name, "a", "api.yaml")
		webPart = filepath.Join(f.base, name, "b", "web.yaml")
		f.write(root, fmt.Sprintf("---\nschema: 4.2.0\nnamespace: legacy\ndesiredstate:\n  meta:\n    parts:\n      self: %s\n      api: %s\n      web: %s\n",
			root, apiPart, webPart))
		f.write(apiPart, "---\ndesiredstate:\n"+artifactsYAML(map[string]string{"api": apiTag}))
		f.write(webPart, "---\ndesiredstate:\n"+artifactsYAML(map[string]string{"web": webTag}))
		return root, apiPart, webPart
	}
	src, _, _ := mk("src", "2.1.0", "1.5.0")
	dst, dstAPI, dstWeb := mk("dst", "2.0.0", "1.0.0")
	apiBefore, webBefore := f.read(dstAPI), f.read(dstWeb)
	if err := os.Chmod(filepath.Dir(dstWeb), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(dstWeb), 0o755) })

	// When: the source is promoted
	_, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, dst))

	// Then: it fails and neither part changed
	if err == nil {
		t.Fatal("promotion into a read only directory succeeded, want an error")
	}
	if got := f.read(dstAPI); got != apiBefore {
		t.Errorf("api part changed although the promotion failed:\n%s", got)
	}
	if got := f.read(dstWeb); got != webBefore {
		t.Errorf("web part changed although the promotion failed:\n%s", got)
	}
}

func TestPromote_UnsupportedLayoutFails_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "Promote fails when it can't find any components in the source",
		CurrentImpl:     "CompareDesiredStates errors when the source parses to zero components",
		ExpectedOutcome: "An error naming the layout promote understands, with the destination unchanged",
		Rationale:       "2.0.0 files used to parse to nothing and promote reported success without changing anything",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: source and destination in the 2.0.0 layout, components straight under desiredstate
	f := newPromoteFixture(t)
	mk := func(name, tag string) string {
		path := filepath.Join(f.base, name+".yaml")
		f.write(path, fmt.Sprintf("---\nschema: 2.0.0\nnamespace: yago\nkind: DesiredState\ndesiredstate:\n  meta:\n    parts:\n      self: %s\n  components:\n    api:\n      docker:\n        eu-west-1:\n          image: example/api\n          tag: %s\n",
			path, tag))
		return path
	}
	src, dst := mk("src", "2.1.0"), mk("dst", "2.0.0")
	before := f.read(dst)

	// When: the source is promoted
	_, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, dst))

	// Then: it fails instead of reporting success
	if err == nil || !strings.Contains(err.Error(), "no components found") {
		t.Fatalf("error = %v, want one saying no components were found", err)
	}
	if got := f.read(dst); got != before {
		t.Errorf("destination changed:\n%s", got)
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

func TestPromote_KeepsFilePermissions_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "A promoted file keeps its permissions",
		CurrentImpl:     "writeAllOrNothing gives each temp file the mode of the file it replaces",
		ExpectedOutcome: "A 0600 destination is still 0600 after promotion",
		Rationale:       "Replacing files through temp files used to reset them to 0644 and make private files readable",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a destination only its owner can read
	f := newPromoteFixture(t)
	src := f.singleFile("src", map[string]string{"api": "2.1.0"})
	dst := f.singleFile("dst", map[string]string{"api": "2.0.0"})
	if err := os.Chmod(dst, 0o600); err != nil {
		t.Fatal(err)
	}

	// When: the source is promoted
	_, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, dst))

	// Then: it's updated and still private
	if err != nil {
		t.Fatalf("PromoteDesiredState: %v", err)
	}
	assertTag(t, f.read(dst), "api", "2.1.0")
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %v, want 0600", mode)
	}
}

func TestPromote_WritesThroughSymlinks_BehavioralBDD(t *testing.T) {
	contract := PromoteBehavioralContract{
		Behavior:        "A symlinked destination file stays a symlink and its target gets the new versions",
		CurrentImpl:     "writeAllOrNothing resolves each path with filepath.EvalSymlinks and replaces the target",
		ExpectedOutcome: "The link and its text are unchanged, the file it points at is promoted",
		Rationale:       "Replacing through a temp file used to swap the link for a regular file and leave its target on the old version",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// link makes path a relative symlink to a real file under shared, holding content
	link := func(f *promoteFixture, path, content string) (target, linkText string) {
		target = filepath.Join(f.base, "shared", filepath.Base(path))
		f.write(target, content)
		linkText, err := filepath.Rel(filepath.Dir(path), target)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(linkText, path); err != nil {
			t.Fatal(err)
		}
		return target, linkText
	}

	tests := []struct {
		name string
		// given writes the destination with one symlinked file and returns the root, the link and its target
		given func(f *promoteFixture) (root, linkPath, target, linkText string)
		src   func(f *promoteFixture) string
	}{
		{
			name: "symlinked root",
			given: func(f *promoteFixture) (string, string, string, string) {
				root := filepath.Join(f.base, "dst.yaml")
				target, linkText := link(f, root, fmt.Sprintf("---\nschema: 4.2.0\nnamespace: legacy\ndesiredstate:\n  meta:\n    parts:\n      self: %s\n%s",
					root, artifactsYAML(map[string]string{"api": "2.0.0"})))
				return root, root, target, linkText
			},
			src: func(f *promoteFixture) string {
				return f.singleFile("src", map[string]string{"api": "2.1.0"})
			},
		},
		{
			name: "symlinked part",
			given: func(f *promoteFixture) (string, string, string, string) {
				root := filepath.Join(f.base, "dst", "desiredstate.yaml")
				part := filepath.Join(f.base, "dst", "app.yaml")
				f.write(root, fmt.Sprintf("---\nschema: 4.2.0\nnamespace: legacy\ndesiredstate:\n  meta:\n    parts:\n      self: %s\n      app: %s\n",
					root, part))
				target, linkText := link(f, part, "---\ndesiredstate:\n"+artifactsYAML(map[string]string{"api": "2.0.0"}))
				return root, part, target, linkText
			},
			src: func(f *promoteFixture) string {
				root, _ := f.withPart("src", map[string]string{"api": "2.1.0"})
				return root
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a destination with one of its files symlinked to a file elsewhere
			f := newPromoteFixture(t)
			root, linkPath, target, linkText := tt.given(f)
			src := tt.src(f)

			// When: the source is promoted
			_, err := NewService(".", false).PromoteDesiredState(promoteRequest(src, root))

			// Then: the link is untouched and the file it points at has the new version
			if err != nil {
				t.Fatalf("PromoteDesiredState: %v", err)
			}
			info, err := os.Lstat(linkPath)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("%s was replaced by a regular file", linkPath)
			}
			if got, err := os.Readlink(linkPath); err != nil || got != linkText {
				t.Errorf("link points at %q (%v), want %q", got, err, linkText)
			}
			assertTag(t, f.read(target), "api", "2.1.0")
		})
	}
}
