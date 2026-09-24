package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_SecretsManagerHelp(t *testing.T) {
	binary := buildYagoBinary(t)
	for _, group := range []string{"sm", "secretsmanager"} {
		for _, name := range []string{"assemble", "plan", "create", "validate", "destroy"} {
			t.Run(group+"_"+name, func(t *testing.T) {
				stdout, stderr, code := runYago(t, binary, group, name, "--help")
				if code != 0 {
					t.Fatalf("help exited %d: %s", code, stderr)
				}
				if !strings.Contains(stdout, "yago sm "+name) {
					t.Fatalf("missing command usage: %s", stdout)
				}
			})
		}
	}
}

func writeSecretsFixture(t *testing.T) (string, string) {
	t.Helper()
	desiredStates := t.TempDir()
	configurations := t.TempDir()

	files := map[string]string{
		filepath.Join(desiredStates, "example/secrets/create/desiredstate.yaml"): `---
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
        watch:
          - example/secrets/create/{**,.}/*
    parts:
      self: example/secrets/create/desiredstate.yaml
  configuration:
    all:
      terraform:
        git:
          url: git@example.com:example/configurations.git
          branch: main
          tag: ''
          path: example/secrets/create/configuration.yaml
          watch:
            - example/{**,.}/*
`,
		filepath.Join(configurations, "example/secrets/create/configuration.yaml"): `---
schema: 2.0.0
namespace: yago
kind: Configuration
configuration:
  meta:
    parts:
      self: example/secrets/create/configuration.yaml
  wrappers:
    terraform:
      parts:
        secrets: example/shared/secrets.yaml
`,
		filepath.Join(configurations, "example/shared/secrets.yaml"): `---
secrets:
  eu-west-1:
    example:
      is_created_here: true
      name: example/app/database
      keys:
        value_path: value
`,
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	return filepath.Join(desiredStates, "example/secrets/create/desiredstate.yaml"),
		filepath.Join(configurations, "example/secrets/create/configuration.yaml")
}

func TestCLI_SecretsManagerAssemble(t *testing.T) {
	binary := buildYagoBinary(t)
	desiredState, configuration := writeSecretsFixture(t)
	cacheDir := t.TempDir()

	_, stderr, code := runYago(t, binary, "sm", "assemble", "-r", "eu-west-1", "-d", desiredState, "-c", configuration, "-C", cacheDir)
	if code != 0 {
		t.Fatalf("assemble exited %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Assembled config:       '"+filepath.Join(cacheDir, "configuration.tfvars.json")+"'") {
		t.Fatalf("missing the assembled configuration: %s", stderr)
	}

	cached, err := os.ReadFile(filepath.Join(cacheDir, "configuration.tfvars.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cached), "example/app/database") || !strings.Contains(string(cached), "project_properties") {
		t.Fatalf("unexpected assembled configuration: %s", cached)
	}
}

func TestCLI_SecretsManagerGlobalLogging(t *testing.T) {
	binary := buildYagoBinary(t)
	desiredState, configuration := writeSecretsFixture(t)

	for _, tc := range []struct {
		name      string
		env       string
		flags     []string
		wantDebug bool
	}{
		{name: "environment", env: "ERROR"},
		{name: "flag_overrides_environment", env: "DEBUG", flags: []string{"--log-level", "error"}},
		{name: "verbose_overrides_error", env: "ERROR", flags: []string{"--log-level", "error", "--verbose"}, wantDebug: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tc.env)
			args := append([]string{"sm", "assemble", "-r", "eu-west-1", "-d", desiredState, "-c", configuration, "-C", t.TempDir()}, tc.flags...)
			_, stderr, code := runYago(t, binary, args...)
			if code != 0 {
				t.Fatalf("assemble exited %d: %s", code, stderr)
			}
			if !strings.Contains(stderr, "Execution time:") {
				t.Fatalf("root command timer was not initialized: %s", stderr)
			}
			// The git loader logs at INFO whatever the log level, so only sm's own lines are checked
			if tc.wantDebug {
				if !strings.Contains(stderr, "[DEBUG]") || !strings.Contains(stderr, "AWS profile:") {
					t.Fatalf("verbose did not enable debug logging: %s", stderr)
				}
			} else if strings.Contains(stderr, "AWS profile:") || strings.Contains(stderr, "[DEBUG]") {
				t.Fatalf("error log level was not respected: %s", stderr)
			}
		})
	}
}
