package secretsmanager

import (
	"strings"
	"testing"
)

type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

func TestSecrets_Encryption_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Read each secret's encryption: a KMS key of its own (the default), an existing KMS key, or SSE",
		CurrentImpl:     "Secret.Encryption and Secret.KmsKeyId, checked by Secrets.Validate",
		ExpectedOutcome: "Only secrets that get a key of their own need a valid, unshared alias and KMS key tags",
		Rationale:       "A secret using an existing key or SSE must not be held to rules for a key yago won't create",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	t.Run("each_encryption_is_read", func(t *testing.T) {
		// Given: secrets with a key of their own, an existing key, and SSE. The last two have names that
		// couldn't be a KMS alias, or would share one.
		content := `secrets:
  eu-west-1:
    own_key:
      is_created_here: true
      name: example/app.database
    given_key:
      is_created_here: true
      name: example/app-database
      kms_key_id: alias/shared
    sse:
      is_created_here: true
      name: example/user@example.com
      encryption: sse
`

		// When: the secrets are parsed
		secrets, err := ParseSecrets(parseConfiguration(t, content))

		// Then: they are valid, and only the first gets a key of its own
		if err != nil {
			t.Fatal(err)
		}
		region := secrets["eu-west-1"]
		for key, want := range map[string]bool{"own_key": true, "given_key": false, "sse": false} {
			if got := region[key].createsKmsKey(); got != want {
				t.Errorf("%s: createsKmsKey() = %v, want %v", key, got, want)
			}
		}
		if request := region["given_key"].createRequest(nil, nil); request.KmsKeyId != "alias/shared" || request.Encryption != "" {
			t.Errorf("unexpected request for the given key: %+v", request)
		}
		if request := region["sse"].createRequest(nil, nil); request.Encryption != "sse" || request.KmsKeyId != "" {
			t.Errorf("unexpected request for SSE: %+v", request)
		}
	})

	t.Run("invalid_encryption_is_refused", func(t *testing.T) {
		for content, want := range map[string]string{
			"secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      encryption: aes\n":                                 `encryption must be kms or sse, not "aes"`,
			"secrets:\n  eu-west-1:\n    a:\n      name: example/a\n      encryption: sse\n      kms_key_id: alias/shared\n": "kms_key_id can't be used with encryption: sse",
		} {
			// Given: an unknown encryption, or a KMS key with SSE
			// When: the secrets are parsed
			_, err := ParseSecrets(parseConfiguration(t, content))

			// Then: parsing fails and says why
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("expected an error containing %q, got %v", want, err)
			}
		}
	})

	t.Run("only_a_key_yago_creates_is_tagged", func(t *testing.T) {
		captureLogs(t)
		for field, value := range map[string]string{"encryption": "sse", "kms_key_id": "alias/shared"} {
			// Given: a secret to create with SSE or an existing key
			configuration := parseConfiguration(t, libConfiguration)
			newOne := configuration["secrets"].(map[string]interface{})["eu-west-1"].(map[string]interface{})["new_one"]
			newOne.(map[string]interface{})[field] = value
			fake := &fakeSecretManager{existing: map[string]bool{"example/existing": true, "example/read-only": true}}
			lib := &Lib{awsRegion: "eu-west-1", secMan: fake}
			if err := lib.SetConfiguration(configuration); err != nil {
				t.Fatal(err)
			}

			// When: create runs
			if err := lib.Create(); err != nil {
				t.Fatal(err)
			}

			// Then: the secret gets its tags, and there are no KMS key tags
			if len(fake.created) != 1 || fake.created[0].SecretTags == nil || fake.created[0].KmsKeyTags != nil {
				t.Fatalf("%s: expected the secret's tags and no KMS key tags, got %+v", field, fake.created)
			}
		}
	})
}
