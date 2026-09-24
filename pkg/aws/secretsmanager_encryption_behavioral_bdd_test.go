package aws

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

func TestSecretManager_Encryption_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Encrypt a new secret with a KMS key of its own, an existing KMS key, or the AWS managed key (SSE)",
		CurrentImpl:     "SecretManager.Create reads CreateRequest.Encryption and CreateRequest.KmsKeyId",
		ExpectedOutcome: "Only the default creates a KMS key; a given key or SSE leaves KMS as it is",
		Rationale:       "Teams choose how secrets are encrypted, and yago never changes or deletes a key it didn't create",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	t.Run("sse_uses_the_AWS_managed_key", func(t *testing.T) {
		for _, isDryRun := range []bool{true, false} {
			// Given: a secret to create with SSE, whose name couldn't be a KMS alias
			fake := newFakeAWS()
			request := newSecretRequest("app/user@example.com", true)
			request.Encryption = EncryptionSse

			// When: create runs
			result, isCreated, err := fake.secretManager(isDryRun).Create(request)

			// Then: the secret uses the AWS managed key and no KMS call is made
			if err != nil || !isCreated {
				t.Fatalf("dry run %v: expected the secret, got %v, %v", isDryRun, isCreated, err)
			}
			if result.KmsKeyId != "aws/secretsmanager" || result.KmsAliasName != "aws/secretsmanager" {
				t.Errorf("dry run %v: expected the AWS managed key, got %+v", isDryRun, result)
			}
			for _, call := range fake.calls {
				if strings.Contains(call, "Key") || strings.Contains(call, "Alias") {
					t.Fatalf("dry run %v: SSE made a KMS call: %v", isDryRun, fake.calls)
				}
			}
			if secret := fake.secrets["app/user@example.com"]; !isDryRun && (secret == nil || secret.kmsKeyId != "") {
				t.Fatalf("expected the secret without a KMS key, got %+v", secret)
			}
		}
	})

	t.Run("a_given_key_is_used_as_it_is", func(t *testing.T) {
		for _, kmsKeyId := range []string{
			"shared-key",
			"arn:aws:kms:eu-west-1:123456789012:key/shared-key",
			"alias/shared",
			"arn:aws:kms:eu-west-1:123456789012:alias/shared",
		} {
			// Given: an existing key, named by its ID, ARN, alias or alias ARN
			fake := newFakeAWS()
			fake.addKey("shared-key")
			fake.aliases["alias/shared"] = "shared-key"
			request := newSecretRequest("app/user@example.com", true)
			request.KmsKeyId = kmsKeyId

			// When: create runs
			result, _, err := fake.secretManager(false).Create(request)

			// Then: the secret is encrypted with the key's ARN, and the key isn't created or changed
			if err != nil {
				t.Fatalf("%s: %v", kmsKeyId, err)
			}
			if fake.secrets["app/user@example.com"].kmsKeyId != fake.keys["shared-key"].arn || result.KmsKeyId != "shared-key" {
				t.Fatalf("%s: expected the given key, got %+v", kmsKeyId, result)
			}
			if fake.called("CreateKey") || fake.called("CreateAlias") || fake.called("EnableKeyRotation") {
				t.Fatalf("%s: a given key was created or changed: %v", kmsKeyId, fake.calls)
			}
			if isAlias := strings.Contains(kmsKeyId, "alias/"); (result.KmsAliasName == kmsKeyId) != isAlias {
				t.Errorf("%s: unexpected alias %q", kmsKeyId, result.KmsAliasName)
			}
		}
	})

	t.Run("a_dry_run_with_a_given_key_changes_nothing", func(t *testing.T) {
		// Given: an existing key for a secret to create
		fake := newFakeAWS()
		fake.addKey("shared-key")
		request := newSecretRequest("example", true)
		request.KmsKeyId = "shared-key"

		// When: create runs as a dry run
		result, isCreated, err := fake.secretManager(true).Create(request)

		// Then: the plan shows the key, and AWS isn't changed
		if err != nil || !isCreated || result.KmsKeyArn != fake.keys["shared-key"].arn {
			t.Fatalf("expected the given key in the plan, got %+v, %v, %v", result, isCreated, err)
		}
		if mutations := fake.mutations(); len(mutations) != 0 {
			t.Fatalf("a dry run changed AWS: %v", mutations)
		}
	})

	t.Run("a_given_key_that_cant_encrypt_is_refused", func(t *testing.T) {
		// Given: a missing key, and a key pending deletion
		fake := newFakeAWS()
		fake.addKey("old-key")
		fake.keys["old-key"].pendingDeletion = true

		for kmsKeyId, want := range map[string]string{"alias/missing": "does not exist", "old-key": "PendingDeletion"} {
			request := newSecretRequest("example", true)
			request.KmsKeyId = kmsKeyId

			// When: create runs
			_, _, err := fake.secretManager(false).Create(request)

			// Then: it fails before creating the secret
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%s: expected an error containing %q, got %v", kmsKeyId, want, err)
			}
		}
		if len(fake.secrets) != 0 {
			t.Fatal("a secret was created without a usable key")
		}
	})

	t.Run("destroy_keeps_a_given_key", func(t *testing.T) {
		// Given: a secret created with a given key, even one tagged with the secret's name
		fake := newFakeAWS()
		fake.addKey("shared-key")
		fake.keys["shared-key"].tags["Name"] = "example"
		request := newSecretRequest("example", true)
		request.KmsKeyId = "shared-key"
		if _, _, err := fake.secretManager(false).Create(request); err != nil {
			t.Fatal(err)
		}

		// When: destroy runs
		err := fake.secretManager(false).Destroy("example")

		// Then: the secret is deleted and the key is kept
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := fake.secrets["example"]; exists || fake.keys["shared-key"].pendingDeletion {
			t.Fatalf("expected the secret deleted and the given key kept: %v", fake.calls)
		}
	})
}
