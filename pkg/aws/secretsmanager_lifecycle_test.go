package aws

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/danieleborsaro/yago/internal/utils/logging"
)

func newSecretRequest(name string, isCreatedHere bool) CreateRequest {
	return CreateRequest{
		Name:                 name,
		PlaceholderKeys:      []string{"username", "private_key_b64"},
		PlaintextPlaceholder: PlaceholderValue,
		IsCreatedHere:        isCreatedHere,
		SecretTags:           map[string]string{"Name": name, "Foo:Environment:ResourceType": ":AWS::SecretsManager::Secret"},
		KmsKeyTags:           map[string]string{"Name": name, "Foo:Environment:ResourceType": ":AWS::KMS::Key"},
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	logging.SetOutput(&logs)
	t.Cleanup(func() { logging.SetOutput(os.Stderr) })
	return &logs
}

func Test_Create_MakesTheSecretAndItsKey(t *testing.T) {
	fake := newFakeAWS()
	sm := fake.secretManager(false)

	result, isCreated, err := sm.Create(newSecretRequest("example/app.database", true))
	if err != nil {
		t.Fatal(err)
	}
	if !isCreated || result == nil {
		t.Fatalf("expected a new secret, got %v, %+v", isCreated, result)
	}

	secret := fake.secrets["example/app.database"]
	if secret == nil {
		t.Fatal("the secret was not created")
	}
	if secret.description != "Secret example/app.database" {
		t.Errorf("expected the default description, got %q", secret.description)
	}
	if secret.tags["isCreatedHere"] != "true" || secret.tags["Foo:Environment:ResourceType"] != ":AWS::SecretsManager::Secret" {
		t.Errorf("unexpected secret tags: %v", secret.tags)
	}

	var value map[string]string
	if err := json.Unmarshal([]byte(secret.value), &value); err != nil {
		t.Fatalf("the placeholder is not JSON: %v", err)
	}
	if value["username"] != "placeholder" || value["private_key_b64"] != "cGxhY2Vob2xkZXI=" {
		t.Errorf("unexpected placeholder: %v", value)
	}

	key := fake.keys[fake.aliases["alias/example/app-database"]]
	if key == nil || secret.kmsKeyId != key.arn || result.KmsAliasName != "alias/example/app-database" {
		t.Fatalf("the secret is not encrypted with its own key behind its alias: %+v, aliases %v", result, fake.aliases)
	}
	if !key.rotation || key.tags["Foo:Environment:ResourceType"] != ":AWS::KMS::Key" {
		t.Errorf("unexpected key: %+v", key)
	}
	if fake.called("PutResourcePolicy") || fake.called("GetResourcePolicy") {
		t.Error("the resource policy was touched without permissions")
	}
}

func Test_Create_LeavesAnExistingSecret(t *testing.T) {
	fake := newFakeAWS()
	fake.addSecret("example", "", map[string]string{})
	sm := fake.secretManager(false)

	result, isCreated, err := sm.Create(newSecretRequest("example", true))
	if err != nil || isCreated || result != nil {
		t.Fatalf("expected the existing secret to be left alone, got %+v, %v, %v", result, isCreated, err)
	}
	if mutations := fake.mutations(); len(mutations) != 0 {
		t.Fatalf("an existing secret was changed: %v", mutations)
	}
}

func Test_Create_ReadsASecretNotCreatedHere(t *testing.T) {
	fake := newFakeAWS()
	fake.addKey("shared")
	fake.addSecret("example", "shared", map[string]string{})
	sm := fake.secretManager(false)

	result, isCreated, err := sm.Create(newSecretRequest("example", false))
	if err != nil {
		t.Fatal(err)
	}
	if isCreated || result == nil || result.ARN != fake.secrets["example"].arn {
		t.Fatalf("expected the existing secret's details, not a new secret: %+v, %v", result, isCreated)
	}
	if fake.called("CreateSecret") {
		t.Fatal("a secret not created here was created")
	}
}

func Test_Create_FailsForAMissingSecretNotCreatedHere(t *testing.T) {
	sm := newFakeAWS().secretManager(false)

	_, _, err := sm.Create(newSecretRequest("example", false))
	if err == nil || !strings.Contains(err.Error(), "does not exist but isCreatedHere=False") {
		t.Fatalf("expected an error for the missing secret, got %v", err)
	}
}

func Test_Create_DryRunMakesNoChanges(t *testing.T) {
	logs := captureLogs(t)
	fake := newFakeAWS()
	fake.addSecret("existing", "", map[string]string{"isCreatedHere": "true"})
	sm := fake.secretManager(true)

	existing := newSecretRequest("existing", true)
	existing.Permissions.RestrictToRoles = []string{"app"}
	_, isCreated, err := sm.Create(existing)
	if err != nil || isCreated {
		t.Fatalf("an existing secret would be created: %v, %v", isCreated, err)
	}
	if !strings.Contains(logs.String(), "[Dry-Run] Would apply resource policy to secret 'existing'") {
		t.Errorf("expected the dry run to report the policy change:\n%s", logs.String())
	}

	missing := newSecretRequest("missing", true)
	missing.Permissions.RestrictToRoles = []string{"app"}
	result, isCreated, err := sm.Create(missing)
	if err != nil || !isCreated {
		t.Fatalf("a missing secret would not be created: %v, %v", isCreated, err)
	}
	if !strings.Contains(result.ARN, ":123456789012:") {
		t.Errorf("expected the real account in the planned ARN, got %s", result.ARN)
	}
	if mutations := fake.mutations(); len(mutations) != 0 {
		t.Fatalf("a dry run changed AWS: %v", mutations)
	}
}

func Test_Create_ReusesItsOwnKeyBehindItsAlias(t *testing.T) {
	fake := newFakeAWS()
	fake.addKey("existing-key")
	fake.keys["existing-key"].tags["Name"] = "example"
	fake.aliases["alias/example"] = "existing-key"
	sm := fake.secretManager(false)

	if _, _, err := sm.Create(newSecretRequest("example", true)); err != nil {
		t.Fatal(err)
	}
	if fake.called("CreateKey") || fake.secrets["example"].kmsKeyId != fake.keys["existing-key"].arn {
		t.Fatalf("expected the key behind alias/example to be reused: %v", fake.calls)
	}
	if !fake.keys["existing-key"].rotation {
		t.Error("the reused key's rotation was not enabled")
	}
}

func Test_Create_RefusesAKeyNotCreatedForTheSecret(t *testing.T) {
	fake := newFakeAWS()
	sm := fake.secretManager(false)
	if _, _, err := sm.Create(newSecretRequest("example/app.database", true)); err != nil {
		t.Fatal(err)
	}

	for _, isDryRun := range []bool{true, false} {
		_, _, err := fake.secretManager(isDryRun).Create(newSecretRequest("example/app-database", true))
		if err == nil || !strings.Contains(err.Error(), "was not created for secret 'example/app-database'") {
			t.Fatalf("expected the alias of example/app.database to be refused (dry run %v), got %v", isDryRun, err)
		}
	}
	if _, exists := fake.secrets["example/app-database"]; exists {
		t.Fatal("a secret was created with another secret's key")
	}
}

func Test_Create_RefusesAKeyPendingDeletion(t *testing.T) {
	fake := newFakeAWS()
	fake.addKey("old-key")
	fake.keys["old-key"].tags["Name"] = "example"
	fake.keys["old-key"].pendingDeletion = true
	fake.aliases["alias/example"] = "old-key"

	_, _, err := fake.secretManager(false).Create(newSecretRequest("example", true))
	if err == nil || !strings.Contains(err.Error(), "PendingDeletion") {
		t.Fatalf("expected the key pending deletion to be refused, got %v", err)
	}
}

func Test_Create_RefusesANameKmsCantUseInAnAlias(t *testing.T) {
	fake := newFakeAWS()

	_, _, err := fake.secretManager(false).Create(newSecretRequest("app/user@example.com", true))
	if err == nil || !strings.Contains(err.Error(), "KMS aliases may only use") {
		t.Fatalf("expected the name to be refused, got %v", err)
	}
	if mutations := fake.mutations(); len(mutations) != 0 {
		t.Fatalf("expected no changes, got %v", mutations)
	}
}

func Test_Create_SchedulesTheNewKeyForDeletionIfItsAliasFails(t *testing.T) {
	fake := newFakeAWS()
	fake.createAliasErr = errors.New("LimitExceededException")

	_, _, err := fake.secretManager(false).Create(newSecretRequest("example", true))
	if err == nil || !strings.Contains(err.Error(), "was scheduled for deletion") {
		t.Fatalf("expected the alias failure, got %v", err)
	}
	if !fake.keys["key-1"].pendingDeletion || fake.called("CreateSecret") {
		t.Fatalf("expected the new key to be scheduled for deletion and no secret: %v", fake.calls)
	}
}

func Test_Create_AppliesTheResourcePolicy(t *testing.T) {
	fake := newFakeAWS()
	request := newSecretRequest("example", true)
	request.Permissions.RestrictToRoles = []string{"app"}

	if _, _, err := fake.secretManager(false).Create(request); err != nil {
		t.Fatal(err)
	}
	policy := fake.secrets["example"].policy
	if !strings.Contains(policy, `"Effect":"Deny"`) || !strings.Contains(policy, "arn:aws:iam::123456789012:role/app") {
		t.Fatalf("expected a policy denying everyone but role/app, got %s", policy)
	}
}

func Test_Create_AppliesThePolicyAfterAFailedAttempt(t *testing.T) {
	fake := newFakeAWS()
	fake.putResourcePolicyErr = errors.New("MalformedPolicyDocumentException")
	request := newSecretRequest("example", true)
	request.Permissions.RestrictToUsers = []string{"alice"}

	_, _, err := fake.secretManager(false).Create(request)
	if err == nil || !strings.Contains(err.Error(), "Run create again") {
		t.Fatalf("expected the policy failure, got %v", err)
	}

	fake.putResourcePolicyErr = nil
	if _, _, err := fake.secretManager(false).Create(request); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.secrets["example"].policy, "arn:aws:iam::123456789012:user/alice") {
		t.Fatalf("the second run didn't apply the policy: %q", fake.secrets["example"].policy)
	}

	fake.calls = nil
	if _, _, err := fake.secretManager(false).Create(request); err != nil {
		t.Fatal(err)
	}
	if fake.called("PutResourcePolicy") {
		t.Fatal("an unchanged policy was applied again")
	}
}

func Test_Create_LeavesThePolicyOfASecretNotCreatedByYago(t *testing.T) {
	fake := newFakeAWS()
	fake.addSecret("example", "", map[string]string{"ManagedBy": "terraform"})
	request := newSecretRequest("example", true)
	request.Permissions.RestrictToRoles = []string{"app"}

	_, _, err := fake.secretManager(false).Create(request)
	if err == nil || !strings.Contains(err.Error(), "was not created by yago") {
		t.Fatalf("expected the policy change to be refused, got %v", err)
	}
	if fake.called("PutResourcePolicy") {
		t.Fatal("the policy of a secret yago didn't create was changed")
	}
}

func Test_CreateDoesNotLogTheSecretValue(t *testing.T) {
	const value = "s3cr3t-value-that-must-not-leak"
	logs := captureLogs(t)
	logging.SetLevel(logging.DEBUG)
	t.Cleanup(func() { logging.SetLevel(logging.INFO) })

	fake := newFakeAWS()
	request := newSecretRequest("example/app/database", true)
	request.PlaceholderKeys = nil
	request.PlaintextPlaceholder = value
	if _, _, err := fake.secretManager(false).Create(request); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(logs.String(), "example/app/database") {
		t.Fatalf("expected the secret name in the logs, got:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), value) {
		t.Fatalf("the logs contain the secret value:\n%s", logs.String())
	}
}

func Test_Destroy_DeletesASecretCreatedHere(t *testing.T) {
	fake := newFakeAWS()
	sm := fake.secretManager(false)
	if _, _, err := sm.Create(newSecretRequest("example", true)); err != nil {
		t.Fatal(err)
	}
	keyId := fake.aliases["alias/example"]

	if err := sm.Destroy("example"); err != nil {
		t.Fatal(err)
	}
	if _, exists := fake.secrets["example"]; exists {
		t.Error("the secret was not deleted")
	}
	if !fake.keys[keyId].pendingDeletion {
		t.Error("the secret's key was not scheduled for deletion")
	}
	if _, exists := fake.aliases["alias/example"]; exists {
		t.Error("the key's alias was not deleted")
	}
}

func Test_Destroy_RemovesTheReplicasFirst(t *testing.T) {
	fake := newFakeAWS()
	sm := fake.secretManager(false)
	if _, _, err := sm.Create(newSecretRequest("example", true)); err != nil {
		t.Fatal(err)
	}
	fake.secrets["example"].replicas = []string{"eu-west-2", "us-east-1"}

	if err := sm.Destroy("example"); err != nil {
		t.Fatal(err)
	}
	if _, exists := fake.secrets["example"]; exists {
		t.Error("the secret was not deleted")
	}
}

func Test_Destroy_StopsIfTheReplicasCantBeRemoved(t *testing.T) {
	fake := newFakeAWS()
	sm := fake.secretManager(false)
	if _, _, err := sm.Create(newSecretRequest("example", true)); err != nil {
		t.Fatal(err)
	}
	fake.secrets["example"].replicas = []string{"eu-west-2"}
	fake.removeReplicasErr = errors.New("AccessDeniedException")

	err := sm.Destroy("example")
	if err == nil || !strings.Contains(err.Error(), "failed to delete the replicas") {
		t.Fatalf("expected the replica failure, got %v", err)
	}
	if fake.called("DeleteSecret") || fake.called("ScheduleKeyDeletion") {
		t.Fatalf("the secret or its key was deleted although its replicas remain: %v", fake.calls)
	}
}

func Test_Destroy_DryRunMakesNoChanges(t *testing.T) {
	fake := newFakeAWS()
	if _, _, err := fake.secretManager(false).Create(newSecretRequest("example", true)); err != nil {
		t.Fatal(err)
	}
	fake.secrets["example"].replicas = []string{"eu-west-2"}
	fake.calls = nil

	if err := fake.secretManager(true).Destroy("example"); err != nil {
		t.Fatal(err)
	}
	if mutations := fake.mutations(); len(mutations) != 0 {
		t.Fatalf("a dry run changed AWS: %v", mutations)
	}
}

func Test_Destroy_RefusesASecretNotCreatedHere(t *testing.T) {
	fake := newFakeAWS()
	fake.addSecret("example", "", map[string]string{"ManagedBy": "terraform"})

	err := fake.secretManager(false).Destroy("example")
	if err == nil || !strings.Contains(err.Error(), "destruction blocked") {
		t.Fatalf("expected the destruction to be blocked, got %v", err)
	}
	if fake.called("DeleteSecret") {
		t.Fatal("a secret not created here was deleted")
	}

	err = fake.secretManager(true).Destroy("example")
	if err == nil || !strings.Contains(err.Error(), "destruction blocked") {
		t.Fatalf("expected the dry run to report the destruction as blocked, got %v", err)
	}
	if fake.called("DeleteSecret") {
		t.Fatal("a dry run deleted the secret")
	}
}

func Test_Destroy_KeepsASharedKey(t *testing.T) {
	fake := newFakeAWS()
	fake.addKey("shared")
	fake.addSecret("example", "shared", map[string]string{"isCreatedHere": "true"})

	if err := fake.secretManager(false).Destroy("example"); err != nil {
		t.Fatal(err)
	}
	if _, exists := fake.secrets["example"]; exists {
		t.Error("the secret was not deleted")
	}
	if fake.keys["shared"].pendingDeletion {
		t.Error("a key not behind alias/example was scheduled for deletion")
	}
}

func Test_Destroy_KeepsTheKeyOfAnotherSecretBehindTheSameAlias(t *testing.T) {
	fake := newFakeAWS()
	if _, _, err := fake.secretManager(false).Create(newSecretRequest("example/app.database", true)); err != nil {
		t.Fatal(err)
	}
	keyId := fake.aliases["alias/example/app-database"]
	fake.addSecret("example/app-database", fake.keys[keyId].arn, map[string]string{"isCreatedHere": "true"})

	if err := fake.secretManager(false).Destroy("example/app-database"); err != nil {
		t.Fatal(err)
	}
	if fake.keys[keyId].pendingDeletion || fake.aliases["alias/example/app-database"] != keyId {
		t.Fatal("the key of example/app.database was deleted with example/app-database")
	}
}

func Test_Destroy_FailsForAMissingSecret(t *testing.T) {
	err := newFakeAWS().secretManager(false).Destroy("missing")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected an error for the missing secret, got %v", err)
	}
}

func Test_Validate_ComparesTheKeysBothWays(t *testing.T) {
	fake := newFakeAWS()
	sm := fake.secretManager(false)
	if _, _, err := sm.Create(newSecretRequest("example", true)); err != nil {
		t.Fatal(err)
	}
	version := fake.secrets["example"].versionId

	result, err := sm.Validate("example", []string{"username", "private_key_b64"}, []string{version})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsExpectedKeysOk || !result.IsStoredKeysOk {
		t.Fatalf("expected the keys to match: %+v", result)
	}
	if result.KmsAliasName != "alias/example" || !strings.HasPrefix(result.KmsKeyArn, "arn:aws:kms:") {
		t.Fatalf("expected the key's ARN and alias: %+v", result)
	}

	result, err = sm.Validate("example", []string{"username", "password"}, []string{"AWSCURRENT"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsExpectedKeysOk || result.IsStoredKeysOk {
		t.Fatalf("expected a mismatch both ways: %+v", result)
	}
}

func Test_SecretInfoFromDescribe(t *testing.T) {
	info := secretInfoFromDescribe(&secretsmanager.DescribeSecretOutput{
		ARN:      aws.String("arn:aws:secretsmanager:eu-west-1:123456789012:secret:example-AbCdEf"),
		KmsKeyId: aws.String("arn:aws:kms:eu-west-1:123456789012:key/1234"),
		Tags:     []types.Tag{{Key: aws.String("isCreatedHere"), Value: aws.String("true")}},
	})
	if info.ReplicaRegions == nil || len(info.ReplicaRegions) != 0 {
		t.Fatalf("expected no replica regions, got %#v", info.ReplicaRegions)
	}
	if info.Tags["isCreatedHere"] != "true" {
		t.Fatalf("expected the tags, got %v", info.Tags)
	}

	info = secretInfoFromDescribe(&secretsmanager.DescribeSecretOutput{
		ReplicationStatus: []types.ReplicationStatusType{{Region: aws.String("eu-west-2")}, {Region: aws.String("us-east-1")}},
	})
	if strings.Join(info.ReplicaRegions, ",") != "eu-west-2,us-east-1" {
		t.Fatalf("expected the replica regions, got %v", info.ReplicaRegions)
	}
	if info.ARN != "" || info.KmsKeyId != "" {
		t.Fatalf("expected empty strings for missing fields, got %#v", info)
	}
}

func Test_KeyMatches(t *testing.T) {
	const keyId = "1234abcd-12ab-34cd-56ef-1234567890ab"
	const keyArn = "arn:aws:kms:eu-west-1:123456789012:key/" + keyId

	tests := []struct {
		name        string
		secretKmsId string
		aliasKeyId  string
		aliasKeyArn string
		want        bool
	}{
		{name: "secret uses the key ARN", secretKmsId: keyArn, aliasKeyId: keyId, aliasKeyArn: keyArn, want: true},
		{name: "secret uses the key ID", secretKmsId: keyId, aliasKeyId: keyId, aliasKeyArn: keyArn, want: true},
		{name: "alias ARN unknown", secretKmsId: keyArn, aliasKeyId: keyId, want: true},
		{name: "shared key", secretKmsId: "arn:aws:kms:eu-west-1:123456789012:key/other", aliasKeyId: keyId, aliasKeyArn: keyArn, want: false},
		{name: "no alias", secretKmsId: keyArn, want: false},
		{name: "no key", aliasKeyId: keyId, aliasKeyArn: keyArn, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyMatches(tt.secretKmsId, tt.aliasKeyId, tt.aliasKeyArn); got != tt.want {
				t.Fatalf("keyMatches(%q, %q, %q) = %v, want %v", tt.secretKmsId, tt.aliasKeyId, tt.aliasKeyArn, got, tt.want)
			}
		})
	}
}
