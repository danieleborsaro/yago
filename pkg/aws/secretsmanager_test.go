package aws

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Test_NewSecretManager tests the NewSecretManager constructor.
func Test_NewSecretManager(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		region   string
		isDryRun bool
	}{
		{name: "Valid AWS profile and region", profile: "default", region: "us-east-1"},
		{name: "Empty profile defaults to default", profile: "", region: "us-west-2"},
		{name: "Dry-run mode", profile: "default", region: "eu-west-1", isDryRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := NewSecretManager(tt.profile, tt.region, tt.isDryRun)
			if err != nil {
				t.Fatalf("NewSecretManager() error = %v, expected nil", err)
			}
			if sm.awsRegion != tt.region {
				t.Errorf("Region mismatch: got %s, want %s", sm.awsRegion, tt.region)
			}
			if sm.isDryRun != tt.isDryRun {
				t.Errorf("DryRun mismatch: got %v, want %v", sm.isDryRun, tt.isDryRun)
			}
		})
	}
}

// Test_GenerateKmsKeyPolicy tests KMS key policy generation.
func Test_GenerateKmsKeyPolicy(t *testing.T) {
	policy, err := newFakeAWS().secretManager(false).generateKmsKeyPolicy("123456789012")
	if err != nil {
		t.Fatalf("generateKmsKeyPolicy() error = %v, expected nil", err)
	}

	for _, want := range []string{
		`"Id":"key-default-1"`,
		"Enable IAM User Permissions",
		"arn:aws:iam::123456789012:root",
		"Allow SecretsManager to use the key",
		"secretsmanager.amazonaws.com",
	} {
		if !strings.Contains(policy, want) {
			t.Errorf("Policy missing %q: %s", want, policy)
		}
	}
}

// Test_GenerateResourcePolicy tests resource policy generation.
func Test_GenerateResourcePolicy(t *testing.T) {
	sm := newFakeAWS().secretManager(false)
	const secretArn = "arn:aws:secretsmanager:eu-west-1:123456789012:secret:test"

	if policy, err := sm.generateResourcePolicy("123456789012", secretArn, Permissions{}); policy != "" || err != nil {
		t.Errorf("expected no policy without permissions, got %s, %v", policy, err)
	}

	policy, err := sm.generateResourcePolicy("123456789012", secretArn, Permissions{
		RestrictToUsers:        []string{"alice"},
		RestrictToRoles:        []string{"app"},
		RestrictToAssumedRoles: []string{"deployer"},
		RestrictToSsoPolicies:  []string{"ReadOnly"},
		ExtraPolicyStatements:  []map[string]interface{}{{"Sid": "Extra"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var document struct {
		Statement []map[string]interface{}
	}
	if err := json.Unmarshal([]byte(policy), &document); err != nil || len(document.Statement) != 3 {
		t.Fatalf("expected three statements, got %s (%v)", policy, err)
	}
	deny := document.Statement[1]
	wantDeny := map[string]interface{}{
		"Sid":       "OnlyAllowNamedResources",
		"Effect":    "Deny",
		"Principal": map[string]interface{}{"AWS": "*"},
		"Resource":  secretArn,
		"Condition": map[string]interface{}{"ArnNotLike": map[string]interface{}{"aws:PrincipalArn": []interface{}{
			"arn:aws:iam::123456789012:user/alice",
			"arn:aws:iam::123456789012:role/app",
			"arn:aws:iam::123456789012:role/deployer",
			"arn:aws:iam::123456789012:role/aws-reserved/sso.amazonaws.com/eu-west-1/AWSReservedSSO_ReadOnly*",
		}}},
	}
	for key, want := range wantDeny {
		if !reflect.DeepEqual(deny[key], want) {
			t.Errorf("statement %s is %v, want %v", key, deny[key], want)
		}
	}
	if document.Statement[2]["Sid"] != "Extra" {
		t.Errorf("expected the extra statement last, got %v", document.Statement[2])
	}

	policy, err = sm.generateResourcePolicy("123456789012", secretArn, Permissions{ExtraPolicyStatements: []map[string]interface{}{{"Sid": "Extra"}}})
	if err != nil || strings.Contains(policy, "OnlyAllowNamedResources") || !strings.Contains(policy, `"Sid":"Extra"`) {
		t.Errorf("expected only the extra statement to restrict access, got %s, %v", policy, err)
	}
}

func Test_Permissions_Validate(t *testing.T) {
	if err := (Permissions{RestrictToGroups: []string{"admins"}}).Validate(); err == nil || !strings.Contains(err.Error(), "restrict_to_groups") {
		t.Errorf("expected groups to be refused, got %v", err)
	}
	unwritable := Permissions{ExtraPolicyStatements: []map[string]interface{}{{"Condition": math.Inf(1)}}}
	if err := unwritable.Validate(); err == nil || !strings.Contains(err.Error(), "extra_policy_statements") {
		t.Errorf("expected statements that aren't JSON to be refused, got %v", err)
	}
	if err := (Permissions{RestrictToRoles: []string{"app"}}).Validate(); err != nil {
		t.Errorf("expected roles to be accepted, got %v", err)
	}
}

// Test_GeneratePlaceholderSecretString tests placeholder secret string generation.
func Test_GeneratePlaceholderSecretString(t *testing.T) {
	sm := newFakeAWS().secretManager(false)

	tests := []struct {
		name      string
		keys      []string
		plaintext string
		want      string
	}{
		{name: "Plain text secret", plaintext: "placeholder", want: "placeholder"},
		{name: "JSON secret with keys", keys: []string{"username", "password"}, plaintext: "placeholder",
			want: `{"password":"placeholder","username":"placeholder"}`},
		{name: "Base64 field", keys: []string{"user", "ssh_private_key_b64"}, plaintext: "placeholder",
			want: `{"ssh_private_key_b64":"cGxhY2Vob2xkZXI=","user":"placeholder"}`}, //gitleaks:allow
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sm.generatePlaceholderSecretString(tt.keys, tt.plaintext)
			if err != nil {
				t.Fatalf("generatePlaceholderSecretString() error = %v", err)
			}
			if result != tt.want {
				t.Errorf("got %s, want %s", result, tt.want)
			}
		})
	}
}

func Test_KmsAliasName(t *testing.T) {
	tests := map[string]string{
		"my-secret":               "alias/my-secret",
		"my.secret.name":          "alias/my-secret-name",
		"example/app/database":    "alias/example/app/database",
		"name.with.multiple.dots": "alias/name-with-multiple-dots",
	}

	for secretName, want := range tests {
		if got := KmsAliasName(secretName); got != want {
			t.Errorf("KmsAliasName(%q) = %s, want %s", secretName, got, want)
		}
	}
}

func Test_ValidateKmsAlias(t *testing.T) {
	for _, name := range []string{"example/app.database", "a", "my_secret-1", strings.Repeat("a", 250)} {
		if err := ValidateKmsAlias(name); err != nil {
			t.Errorf("ValidateKmsAlias(%q) = %v, want no error", name, err)
		}
	}
	for _, name := range []string{"app/user@example.com", "a+b", "a=b", "aws/secret", strings.Repeat("a", 251)} {
		if err := ValidateKmsAlias(name); err == nil {
			t.Errorf("ValidateKmsAlias(%q) accepted an alias KMS rejects", name)
		}
	}
}

func Test_Read(t *testing.T) {
	fake := newFakeAWS()
	sm := fake.secretManager(false)
	request := newSecretRequest("example", true)
	request.PlaceholderKeys = nil
	if _, _, err := sm.Create(request); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ version, stage string }{
		{version: fake.secrets["example"].versionId},
		{stage: "AWSCURRENT"},
		{},
	} {
		value, versionID, err := sm.Read("example", tc.version, tc.stage)
		if err != nil || value != "placeholder" || versionID != fake.secrets["example"].versionId {
			t.Errorf("Read(%q, %q) = %q, %q, %v", tc.version, tc.stage, value, versionID, err)
		}
	}

	if _, _, err := sm.Read("missing", "", ""); err == nil {
		t.Error("expected an error for a missing secret")
	}
}
