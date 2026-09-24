package aws

import (
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
			if sm.awsProfile != "default" {
				t.Errorf("Profile mismatch: got %s, want default", sm.awsProfile)
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

	if policy := sm.generateResourcePolicy("123456789012", secretArn, nil, nil, nil, nil, nil, nil); policy != "" {
		t.Errorf("generateResourcePolicy() should return empty string for no restrictions, got %s", policy)
	}

	policy := sm.generateResourcePolicy("123456789012", secretArn,
		[]string{"alice"}, []string{"admins"}, []string{"app"}, nil, []string{"ReadOnly"},
		[]map[string]interface{}{{"Sid": "Extra"}})
	for _, want := range []string{
		"arn:aws:iam::123456789012:user/alice",
		"arn:aws:iam::123456789012:group/admins",
		"arn:aws:iam::123456789012:role/app",
		"arn:aws:iam::123456789012:role/aws-reserved/sso.amazonaws.com/eu-west-1/AWSReservedSSO_ReadOnly*",
		`"Sid":"Extra"`,
	} {
		if !strings.Contains(policy, want) {
			t.Errorf("Policy missing %q: %s", want, policy)
		}
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
			want: `{"ssh_private_key_b64":"cGxhY2Vob2xkZXI=","user":"placeholder"}`},
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
		if got := kmsAliasName(secretName); got != want {
			t.Errorf("kmsAliasName(%q) = %s, want %s", secretName, got, want)
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
		value, err := sm.Read("example", tc.version, tc.stage)
		if err != nil || value != "placeholder" {
			t.Errorf("Read(%q, %q) = %q, %v", tc.version, tc.stage, value, err)
		}
	}

	if _, err := sm.Read("missing", "", ""); err == nil {
		t.Error("expected an error for a missing secret")
	}
}
