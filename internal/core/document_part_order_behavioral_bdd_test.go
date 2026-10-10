package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePartOrderFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// mount points written out of alphabetical order, enough of them that a map walk
// almost never lands on the written order by luck
var partOrderMountPoints = []string{"zulu", "mike", "alpha", "yankee", "bravo", "november", "charlie", "xray"}

func TestGitOpsDocument_ConfigurationPartsMergeInWrittenOrder_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Configuration parts are merged in the order the root file lists them",
		CurrentImpl:     "GetAllPartFiles reads the mount point order from the root file, not from a map walk",
		ExpectedOutcome: "A value set in several parts always comes from the part listed last",
		Rationale:       "A map walk made the winner change between runs, so the same commit could plan differently, and the Python tool merged in written order",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a configuration whose terraform parts all set the same value
	dir := t.TempDir()
	var parts strings.Builder
	files := map[string]string{}
	for _, mountPoint := range partOrderMountPoints {
		partFile := "example/deploy/terraform/" + mountPoint + ".yaml"
		fmt.Fprintf(&parts, "        %s: %s\n", mountPoint, partFile)
		files[partFile] = "---\nwinner: " + mountPoint + "\nonly_" + mountPoint + ": true\n"
	}
	files["example/deploy/configuration.yaml"] = `---
schema: 2.0.0
namespace: yago
kind: Configuration
configuration:
  meta:
    parts:
      self: example/deploy/configuration.yaml
  wrappers:
    terraform:
      parts:
` + parts.String()
	writePartOrderFiles(t, dir, files)
	rootFile := filepath.Join(dir, "example/deploy/configuration.yaml")

	for run := 0; run < 20; run++ {
		// When: the configuration is loaded for terraform
		doc := NewGitOpsDocument()
		if err := doc.LoadGitOpsFile(rootFile, false, nil); err != nil {
			t.Fatalf("LoadGitOpsFile failed: %v", err)
		}
		doc.SetWrapperForConfiguration("terraform")
		if err := doc.LoadParts(); err != nil {
			t.Fatalf("LoadParts failed: %v", err)
		}

		// Then: the files come back in written order and the last one wins
		partFiles, err := doc.GetAllPartFiles(doc.GetPropertyPartsToLoad())
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, partCombo := range partFiles {
			for mountPoint := range partCombo {
				got = append(got, mountPoint)
			}
		}
		if strings.Join(got, ",") != strings.Join(partOrderMountPoints, ",") {
			t.Fatalf("run %d, parts ordered %v, want %v", run, got, partOrderMountPoints)
		}
		content := doc.GetContent().Data
		if content["winner"] != "xray" {
			t.Fatalf("run %d, winner = %v, want the last part listed, xray", run, content["winner"])
		}
		for _, mountPoint := range partOrderMountPoints {
			if content["only_"+mountPoint] != true {
				t.Errorf("run %d, part %s wasn't merged in", run, mountPoint)
			}
		}
	}
}

func TestGitOpsDocument_DesiredStatePartsMergeInWrittenOrder_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "DesiredState parts are merged in the order the root file lists them",
		CurrentImpl:     "GetAllPartFiles reads the mount point order from the root file, not from a map walk",
		ExpectedOutcome: "A value set in several parts always comes from the part listed last",
		Rationale:       "The terraform version or a repo tag set in two parts decided what got deployed by chance",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a desiredstate whose parts all set the terraform version
	dir := t.TempDir()
	var parts strings.Builder
	files := map[string]string{}
	for i, mountPoint := range partOrderMountPoints {
		partFile := "example/deploy/parts/" + mountPoint + ".yaml"
		fmt.Fprintf(&parts, "      %s: %s\n", mountPoint, partFile)
		files[partFile] = fmt.Sprintf("---\ndesiredstate:\n  tools:\n    terraform: !!str 1.6.%d\n", i)
	}
	files["example/deploy/desiredstate.yaml"] = `---
schema: 2.0.0
namespace: yago
kind: DesiredState
desiredstate:
  meta:
    repo:
      git:
        url: git@example.com:example/desiredstates.git
        branch: main
        tag: ''
    parts:
      self: example/deploy/desiredstate.yaml
` + parts.String()
	writePartOrderFiles(t, dir, files)
	want := fmt.Sprintf("1.6.%d", len(partOrderMountPoints)-1)

	for run := 0; run < 20; run++ {
		// When: the desiredstate is assembled
		doc := NewGitOpsDocument()
		if err := doc.LoadGitOpsFile(filepath.Join(dir, "example/deploy/desiredstate.yaml"), true, nil); err != nil {
			t.Fatalf("LoadGitOpsFile failed: %v", err)
		}

		// Then: the version comes from the last part listed
		got, err := doc.GetContent().GetValue("desiredstate.tools.terraform")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("run %d, terraform version = %v, want %s from the last part listed", run, got, want)
		}
	}
}

func TestGitOpsDocument_PartsWithNoWrittenOrderAreSorted_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Mount points the root file doesn't show are merged in alphabetical order",
		CurrentImpl:     "partMountPoints puts the written ones first and sorts whatever is left",
		ExpectedOutcome: "The order is the same on every run even when the root file can't be read for it",
		Rationale:       "A document built in memory has no file to read the order from and still has to merge the same way every time",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a document with no root file on disk
	doc := NewGitOpsDocument()
	block := map[string]interface{}{}
	for _, mountPoint := range partOrderMountPoints {
		block[mountPoint] = mountPoint + ".yaml"
	}

	// When: the mount points are ordered
	got := doc.partMountPoints("meta.parts", block)

	// Then: they come back sorted
	want := "alpha,bravo,charlie,mike,november,xray,yankee,zulu"
	if strings.Join(got, ",") != want {
		t.Errorf("mount points = %v, want %s", got, want)
	}
}
