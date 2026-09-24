package secretsmanager

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func yagoRootFromTestFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to locate test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func parseConfiguration(t *testing.T, content string) map[string]interface{} {
	t.Helper()
	configuration := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(content), &configuration); err != nil {
		t.Fatal(err)
	}
	return configuration
}

const validSecrets = `---
secrets:
  eu-west-1:
    database:
      is_created_here: true
      name: example/app/database
      description: Database credentials
      versions:
        version_a: 6324c1f6-e0eb-4066-af51-29059f772d48
      keys:
        username_path: username
        password_path: password_b64
      permissions:
        restrict_to_users:
          - alice
        extra_policy_statements:
          - Sid: DenyOthers
            Effect: Deny
    upstream_token:
      is_created_here: false
      name: example/app/token
      keys: {}
`

func TestParseSecrets_ReadsTheConfigurationFormatFixture(t *testing.T) {
	fixture := filepath.Join(yagoRootFromTestFile(t), "tests", "assets", "4.2.0", "configurations",
		"concourse-cluster", "concourse", "secrets-cluster.yaml")
	content, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}

	secrets, err := ParseSecrets(parseConfiguration(t, string(content)))
	if err != nil {
		t.Fatalf("ParseSecrets(%s) returned error: %v", fixture, err)
	}

	regionSecrets, keys, exists := secrets.Region("eu-west-1")
	if !exists || len(keys) != len(regionSecrets) || len(keys) == 0 {
		t.Fatalf("expected the fixture's secrets, got %d", len(keys))
	}

	root, exists := regionSecrets["pgsql_user_root"]
	if !exists {
		t.Fatalf("expected pgsql_user_root in the fixture, got %v", keys)
	}
	if !root.IsCreatedHere || root.Name != "secrets/devops-dev/concourse/postgres/users/admin" {
		t.Fatalf("unexpected pgsql_user_root: %+v", root)
	}
	if got := strings.Join(root.KeyNames(), ","); got != "password_b64,username" {
		t.Fatalf("expected the JSON fields of the keys, got %s", got)
	}
	if got := strings.Join(root.VersionValues(), ","); got != "6324c1f6-e0eb-4066-af51-29059f772d48" {
		t.Fatalf("unexpected versions: %s", got)
	}

	if localAuth := regionSecrets["local_auth"]; len(localAuth.KeyNames()) != 0 {
		t.Fatalf("expected local_auth to have no keys, got %v", localAuth.KeyNames())
	}
}

func TestParseSecrets_Region(t *testing.T) {
	secrets, err := ParseSecrets(parseConfiguration(t, validSecrets))
	if err != nil {
		t.Fatalf("ParseSecrets returned error: %v", err)
	}

	_, keys, exists := secrets.Region("eu-west-1")
	if !exists || strings.Join(keys, ",") != "database,upstream_token" {
		t.Fatalf("expected sorted secret keys, got %v", keys)
	}

	if _, _, exists := secrets.Region("us-east-1"); exists {
		t.Fatal("expected no secrets for a region the configuration doesn't have")
	}
}

func TestParseSecrets_OnlyChecksTheAliasOfSecretsCreatedHere(t *testing.T) {
	content := "secrets:\n  eu-west-1:\n    a:\n      name: example/user@example.com\n    b:\n      name: example/user-example.com\n"
	if _, err := ParseSecrets(parseConfiguration(t, content)); err != nil {
		t.Fatalf("expected secrets read from elsewhere to need no KMS alias, got %v", err)
	}
}

func TestParseSecrets_WithoutASecretsBlock(t *testing.T) {
	secrets, err := ParseSecrets(map[string]interface{}{"workspace": "example"})
	if err != nil || len(secrets) != 0 {
		t.Fatalf("expected no secrets, got %v, %v", secrets, err)
	}
}

func TestSecret_CreateRequest(t *testing.T) {
	secrets, err := ParseSecrets(parseConfiguration(t, validSecrets))
	if err != nil {
		t.Fatal(err)
	}

	request := secrets["eu-west-1"]["database"].createRequest(map[string]string{"Name": "a"}, map[string]string{"Name": "b"})
	if request.Name != "example/app/database" || !request.IsCreatedHere || request.PlaintextPlaceholder != "placeholder" {
		t.Fatalf("unexpected request: %+v", request)
	}
	if strings.Join(request.PlaceholderKeys, ",") != "password_b64,username" {
		t.Fatalf("expected the JSON fields as placeholder keys, got %v", request.PlaceholderKeys)
	}
	if strings.Join(request.Permissions.RestrictToUsers, ",") != "alice" ||
		len(request.Permissions.ExtraPolicyStatements) != 1 ||
		request.Permissions.ExtraPolicyStatements[0]["Sid"] != "DenyOthers" {
		t.Fatalf("unexpected permissions: %+v", request.Permissions)
	}
	if request.SecretTags["Name"] != "a" || request.KmsKeyTags["Name"] != "b" {
		t.Fatalf("unexpected tags: %v, %v", request.SecretTags, request.KmsKeyTags)
	}
}

func TestParseSecrets_RejectsInvalidSecrets(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown field",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      version_id: 6324c1f6-e0eb-4066-af51-29059f772d48\n",
			want:    "version_id",
		},
		{
			name:    "list format",
			content: "secrets:\n  - name: example/a\n    secretString: value\n",
			want:    "failed to parse",
		},
		{
			name:    "missing name",
			content: "secrets:\n  eu-west-1:\n    a:\n      is_created_here: true\n",
			want:    "name is required",
		},
		{
			name:    "invalid name",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: has spaces\n",
			want:    "may only use",
		},
		{
			name:    "duplicate name",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: example/a\n    b:\n      name: example/a\n",
			want:    "also used by",
		},
		{
			name:    "empty key",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      keys:\n        username_path: \"\"\n",
			want:    "has no field name",
		},
		{
			name:    "duplicate key field",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      keys:\n        one: username\n        two: username\n",
			want:    "both use field",
		},
		{
			name:    "empty version",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      versions:\n        version_a: \"\"\n",
			want:    "is empty",
		},
		{
			name:    "unknown permission",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      permissions:\n        restrict_to_everyone: true\n",
			want:    "restrict_to_everyone",
		},
		{
			name:    "group permission",
			content: "secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      permissions:\n        restrict_to_groups: [admins]\n",
			want:    "restrict_to_groups can't be enforced",
		},
		{
			name:    "name KMS can't use in an alias",
			content: "secrets:\n  eu-west-1:\n    a:\n      is_created_here: true\n      name: example/user@example.com\n",
			want:    "KMS aliases may only use",
		},
		{
			name: "shared KMS alias",
			content: "secrets:\n  eu-west-1:\n    a:\n      is_created_here: true\n      name: example/app.database\n" +
				"    b:\n      is_created_here: true\n      name: example/app-database\n",
			want: `"alias/example/app-database" is also the alias of a`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSecrets(parseConfiguration(t, tt.content))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected an error containing %q, got %v", tt.want, err)
			}
		})
	}
}
