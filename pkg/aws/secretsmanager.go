package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

const (
	EncryptionKms = "kms"
	// Secrets Manager's default encryption, with the AWS managed key.
	EncryptionSse = "sse"
	awsManagedKey = "aws/secretsmanager"
)

const (
	PlaceholderValue = "placeholder"
	b64Suffix        = "_b64"

	// Destroy only deletes secrets with this tag, which Create sets.
	isCreatedHereTagKey   = "isCreatedHere"
	isCreatedHereTagValue = "true"
)

// SecretCreationResult represents the result of secret creation.
type SecretCreationResult struct {
	ARN          string `json:"arn"`            // Secret ARN
	ID           string `json:"id"`             // Secret ID
	Name         string `json:"name"`           // Secret name
	KmsKeyId     string `json:"kms_key_id"`     // KMS key ID
	KmsKeyArn    string `json:"kms_key_arn"`    // KMS key ARN
	KmsAliasName string `json:"kms_alias_name"` // KMS alias
	VersionId    string `json:"version_id"`     // Version ID
}

// SecretValidationResult represents the result of secret validation.
type SecretValidationResult struct {
	IsExpectedKeysOk  bool     `json:"is_expected_keys_ok"`
	IsStoredKeysOk    bool     `json:"is_stored_keys_ok"`
	ExpectedKeys      []string `json:"expected_keys"`
	StoredKeys        []string `json:"stored_keys"`
	VersionsValidated []string `json:"versions_validated"`
	ARN               string   `json:"arn"`
	KmsKeyId          string   `json:"kms_key_id"`
	KmsKeyArn         string   `json:"kms_key_arn"`
	KmsAliasName      string   `json:"kms_alias_name"`
}

// KmsKeyResult represents KMS key creation result.
type KmsKeyResult struct {
	KeyId     string
	KeyArn    string
	AliasName string
}

type Permissions struct {
	RestrictToUsers        []string                 `yaml:"restrict_to_users"`
	RestrictToGroups       []string                 `yaml:"restrict_to_groups"`
	RestrictToRoles        []string                 `yaml:"restrict_to_roles"`
	RestrictToAssumedRoles []string                 `yaml:"restrict_to_assumed_roles"`
	RestrictToSsoPolicies  []string                 `yaml:"restrict_to_sso_policies"`
	ExtraPolicyStatements  []map[string]interface{} `yaml:"extra_policy_statements"`
}

func (p Permissions) Validate() error {
	if len(p.RestrictToGroups) > 0 {
		return errors.New(errors.ErrParam,
			"restrict_to_groups can't be enforced: IAM groups aren't principals, so the policy can't match their members. List the users or roles instead")
	}
	if _, err := json.Marshal(p.ExtraPolicyStatements); err != nil {
		return errors.Wrapf(errors.ErrParam, err, "extra_policy_statements can't be written as JSON")
	}
	return nil
}

type CreateRequest struct {
	Name                 string
	Description          string
	PlaceholderKeys      []string
	PlaintextPlaceholder string
	IsCreatedHere        bool
	Encryption           string
	KmsKeyId             string
	SecretTags           map[string]string
	KmsKeyTags           map[string]string
	Permissions          Permissions
}

type secretsManagerAPI interface {
	CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	DescribeSecret(ctx context.Context, params *secretsmanager.DescribeSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
	GetResourcePolicy(ctx context.Context, params *secretsmanager.GetResourcePolicyInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetResourcePolicyOutput, error)
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	PutResourcePolicy(ctx context.Context, params *secretsmanager.PutResourcePolicyInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutResourcePolicyOutput, error)
	RemoveRegionsFromReplication(ctx context.Context, params *secretsmanager.RemoveRegionsFromReplicationInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.RemoveRegionsFromReplicationOutput, error)
}

type kmsAPI interface {
	kms.ListAliasesAPIClient
	CreateAlias(ctx context.Context, params *kms.CreateAliasInput, optFns ...func(*kms.Options)) (*kms.CreateAliasOutput, error)
	CreateKey(ctx context.Context, params *kms.CreateKeyInput, optFns ...func(*kms.Options)) (*kms.CreateKeyOutput, error)
	DeleteAlias(ctx context.Context, params *kms.DeleteAliasInput, optFns ...func(*kms.Options)) (*kms.DeleteAliasOutput, error)
	DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error)
	EnableKeyRotation(ctx context.Context, params *kms.EnableKeyRotationInput, optFns ...func(*kms.Options)) (*kms.EnableKeyRotationOutput, error)
	ListResourceTags(ctx context.Context, params *kms.ListResourceTagsInput, optFns ...func(*kms.Options)) (*kms.ListResourceTagsOutput, error)
	ScheduleKeyDeletion(ctx context.Context, params *kms.ScheduleKeyDeletionInput, optFns ...func(*kms.Options)) (*kms.ScheduleKeyDeletionOutput, error)
}

type stsAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// SecretManager manages AWS Secrets Manager operations for secret lifecycle management.
type SecretManager struct {
	awsRegion string
	isDryRun  bool
	accountId string

	// AWS clients
	smClient  secretsManagerAPI
	kmsClient kmsAPI
	stsClient stsAPI

	// Context for AWS operations
	ctx context.Context
}

// NewSecretManager creates a new SecretManager instance.
func NewSecretManager(awsProfile, awsRegion string, isDryRun bool) (*SecretManager, error) {
	ctx := context.Background()

	logging.Debug("Creating SecretManager for region %s (profile: %s, dryRun: %v)",
		awsRegion, awsProfile, isDryRun)

	// Handle default profile
	if awsProfile == "" {
		awsProfile = "default"
	}

	// Load AWS config
	var cfg aws.Config
	var err error

	if awsProfile != "default" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(awsRegion),
			config.WithSharedConfigProfile(awsProfile),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(awsRegion),
		)
	}

	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to load AWS config")
	}

	return &SecretManager{
		awsRegion: awsRegion,
		isDryRun:  isDryRun,
		smClient:  secretsmanager.NewFromConfig(cfg),
		kmsClient: kms.NewFromConfig(cfg),
		stsClient: sts.NewFromConfig(cfg),
		ctx:       ctx,
	}, nil
}

func (sm *SecretManager) AccountID() (string, error) {
	if sm.accountId != "" {
		return sm.accountId, nil
	}

	stsOutput, err := sm.stsClient.GetCallerIdentity(sm.ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get AWS account ID")
	}
	sm.accountId = aws.ToString(stsOutput.Account)

	return sm.accountId, nil
}

var kmsAliasNameRe = regexp.MustCompile(`^alias/[a-zA-Z0-9/_-]{1,250}$`)

func KmsAliasName(secretName string) string {
	return fmt.Sprintf("alias/%s", strings.ReplaceAll(secretName, ".", "-"))
}

func ValidateKmsAlias(secretName string) error {
	aliasName := KmsAliasName(secretName)
	if !kmsAliasNameRe.MatchString(aliasName) || strings.HasPrefix(aliasName, "alias/aws/") {
		return errors.Newf(errors.ErrParam,
			"the KMS alias of secret '%s' would be '%s', but KMS aliases may only use letters, digits and /_- (up to 256 characters) and can't start with alias/aws/",
			secretName, aliasName)
	}
	return nil
}

// Create creates a new secret in AWS Secrets Manager with KMS encryption.
func (sm *SecretManager) Create(req CreateRequest) (*SecretCreationResult, bool, error) {
	name := req.Name

	logging.Info("Processing secret: %s (isCreatedHere=%v, isDryRun=%v)",
		name, req.IsCreatedHere, sm.isDryRun)

	accountId, err := sm.AccountID()
	if err != nil {
		return nil, false, err
	}

	// Handle isCreatedHere=False: retrieve existing secret
	if !req.IsCreatedHere {
		logging.Info("isCreatedHere=False: retrieving existing secret '%s'", name)

		exists, info, err := sm.checkSecretExists(name)
		if err != nil {
			return nil, false, err
		}

		if !exists {
			return nil, false, errors.Newf(errors.ErrFail,
				"secret '%s' does not exist but isCreatedHere=False. Cannot proceed without existing secret", name)
		}

		kmsKeyArn, aliasName := sm.describeKmsKey(name, info.KmsKeyId)
		logging.Info("Retrieved existing secret '%s' (ARN: %s)", name, info.ARN)

		return &SecretCreationResult{
			ARN:          info.ARN,
			ID:           name,
			Name:         name,
			KmsKeyId:     info.KmsKeyId,
			KmsKeyArn:    kmsKeyArn,
			KmsAliasName: aliasName,
		}, false, nil
	}

	// Handle isCreatedHere=True: create resources with idempotency
	isSse := req.Encryption == EncryptionSse
	if !isSse && req.KmsKeyId == "" {
		if err := ValidateKmsAlias(name); err != nil {
			return nil, false, err
		}
	}

	exists, info, err := sm.checkSecretExists(name)
	if err != nil {
		return nil, false, err
	}

	if exists {
		logging.Warn("Secret '%s' already exists", name)
		return nil, false, sm.updateResourcePolicy(name, info, accountId, req.Permissions)
	}

	logging.Info("Secret '%s' does not exist, will create", name)

	aliasName := KmsAliasName(name)
	var existingKey *kmstypes.KeyMetadata
	if isSse {
		aliasName = awsManagedKey
	} else if req.KmsKeyId != "" {
		existingKey, err = sm.findKmsKey(req.KmsKeyId)
		if err != nil {
			return nil, false, err
		}
		if existingKey == nil {
			return nil, false, errors.Newf(errors.ErrFail, "KMS key '%s' does not exist", req.KmsKeyId)
		}
		aliasName = ""
		if strings.Contains(req.KmsKeyId, "alias/") {
			aliasName = req.KmsKeyId
		}
	} else {
		existingKey, err = sm.findKmsKey(aliasName)
		if err != nil {
			return nil, false, err
		}
		if existingKey != nil {
			isForSecret, err := sm.isKeyForSecret(aws.ToString(existingKey.KeyId), name)
			if err != nil {
				return nil, false, err
			}
			if !isForSecret {
				return nil, false, errors.Newf(errors.ErrFail,
					"KMS alias '%s' already exists for a key that was not created for secret '%s'", aliasName, name)
			}
		}
	}
	if existingKey != nil && existingKey.KeyState != kmstypes.KeyStateEnabled {
		return nil, false, errors.Newf(errors.ErrFail,
			"KMS key '%s' can't encrypt secret '%s' as it is %s", aws.ToString(existingKey.KeyId), name, existingKey.KeyState)
	}

	// Handle dry-run mode
	if sm.isDryRun {
		if isSse {
			logging.Info("[Dry-Run] Would create secret '%s' with the AWS managed key", name)
		} else if req.KmsKeyId != "" {
			logging.Info("[Dry-Run] Would create secret '%s' with KMS key '%s'", name, req.KmsKeyId)
		} else {
			logging.Info("[Dry-Run] Would create secret '%s' with KMS key", name)
		}
		result := &SecretCreationResult{
			ARN:          fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:%s-XXXXXX", sm.awsRegion, accountId, name),
			ID:           name,
			Name:         name,
			KmsKeyId:     "XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX",
			KmsKeyArn:    fmt.Sprintf("arn:aws:kms:%s:%s:key/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX", sm.awsRegion, accountId),
			KmsAliasName: aliasName,
			VersionId:    "XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX",
		}
		if isSse {
			result.KmsKeyId, result.KmsKeyArn = awsManagedKey, awsManagedKey
		} else if existingKey != nil {
			result.KmsKeyId, result.KmsKeyArn = aws.ToString(existingKey.KeyId), aws.ToString(existingKey.Arn)
		}
		return result, true, nil
	}

	// Step 1: Create or get KMS key
	var kmsInfo *KmsKeyResult

	if isSse {
		logging.Info("Using the AWS managed key for secret '%s'", name)
		kmsInfo = &KmsKeyResult{KeyId: awsManagedKey, KeyArn: awsManagedKey, AliasName: awsManagedKey}
	} else if req.KmsKeyId != "" {
		logging.Info("Using KMS key '%s' for secret '%s'", req.KmsKeyId, name)
		kmsInfo = &KmsKeyResult{
			KeyId:     aws.ToString(existingKey.KeyId),
			KeyArn:    aws.ToString(existingKey.Arn),
			AliasName: aliasName,
		}
	} else if existingKey != nil {
		logging.Warn("KMS key with alias '%s' already exists, using existing key", aliasName)
		kmsInfo = &KmsKeyResult{
			KeyId:     aws.ToString(existingKey.KeyId),
			KeyArn:    aws.ToString(existingKey.Arn),
			AliasName: aliasName,
		}
		if err := sm.enableKeyRotation(kmsInfo.KeyId); err != nil {
			return nil, false, err
		}
	} else {
		logging.Info("Creating new KMS key for secret '%s'", name)
		kmsInfo, err = sm.createKmsKeyForSecret(name, accountId, fmt.Sprintf("KMS key for secret %s", name), req.KmsKeyTags)
		if err != nil {
			return nil, false, err
		}
	}

	// Step 2: Generate placeholder secret string
	secretString, err := sm.generatePlaceholderSecretString(req.PlaceholderKeys, req.PlaintextPlaceholder)
	if err != nil {
		return nil, false, err
	}

	// Step 3: Create the secret with KMS encryption
	logging.Info("Creating secret '%s'", name)

	description := req.Description
	if description == "" {
		description = fmt.Sprintf("Secret %s", name)
	}

	secretTags := map[string]string{}
	for k, v := range req.SecretTags {
		secretTags[k] = v
	}
	secretTags[isCreatedHereTagKey] = isCreatedHereTagValue

	createInput := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		Description:  aws.String(description),
		SecretString: aws.String(secretString),
		Tags:         secretsManagerTags(secretTags),
	}
	if !isSse {
		createInput.KmsKeyId = aws.String(kmsInfo.KeyArn) // Another account's key needs its ARN.
	}
	createOutput, err := sm.smClient.CreateSecret(sm.ctx, createInput)
	if err != nil {
		return nil, false, errors.Wrapf(errors.ErrFail, err, "failed to create secret '%s'", name)
	}

	secretArn := aws.ToString(createOutput.ARN)
	versionId := aws.ToString(createOutput.VersionId)

	logging.Info("Created secret '%s' (ARN: %s)", name, secretArn)

	// Step 4: Apply resource policy if restrictions specified
	resourcePolicy, err := sm.generateResourcePolicy(accountId, secretArn, req.Permissions)
	if err == nil && resourcePolicy != "" {
		err = sm.putResourcePolicy(name, secretArn, resourcePolicy)
	}
	if err != nil {
		return nil, false, errors.Wrapf(errors.ErrFail, err,
			"secret '%s' was created, but its resource policy could not be applied. Run create again to apply it", name)
	}

	return &SecretCreationResult{
		ARN:          secretArn,
		ID:           name,
		Name:         name,
		KmsKeyId:     kmsInfo.KeyId,
		KmsKeyArn:    kmsInfo.KeyArn,
		KmsAliasName: kmsInfo.AliasName,
		VersionId:    versionId,
	}, true, nil
}

// Existing secrets get the policy too, so a change to permissions, or a failed first attempt, is applied.
func (sm *SecretManager) updateResourcePolicy(name string, info *secretInfo, accountId string, permissions Permissions) error {
	resourcePolicy, err := sm.generateResourcePolicy(accountId, info.ARN, permissions)
	if err != nil || resourcePolicy == "" {
		return err
	}

	output, err := sm.smClient.GetResourcePolicy(sm.ctx, &secretsmanager.GetResourcePolicyInput{
		SecretId: aws.String(info.ARN),
	})
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to read the resource policy of secret '%s'", name)
	}
	if isSamePolicy(aws.ToString(output.ResourcePolicy), resourcePolicy) {
		logging.Debug("Secret '%s' already has its resource policy", name)
		return nil
	}

	if info.Tags[isCreatedHereTagKey] != isCreatedHereTagValue {
		return errors.Newf(errors.ErrFail, "secret '%s' was not created by yago (it has no %s=%s tag), so its resource policy is not changed",
			name, isCreatedHereTagKey, isCreatedHereTagValue)
	}

	if sm.isDryRun {
		logging.Info("[Dry-Run] Would apply resource policy to secret '%s'", name)
		return nil
	}
	if err := sm.putResourcePolicy(name, info.ARN, resourcePolicy); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to apply the resource policy of secret '%s'", name)
	}
	return nil
}

func (sm *SecretManager) putResourcePolicy(name, secretArn, resourcePolicy string) error {
	logging.Info("Applying resource policy to secret '%s'", name)
	_, err := sm.smClient.PutResourcePolicy(sm.ctx, &secretsmanager.PutResourcePolicyInput{
		SecretId:       aws.String(secretArn),
		ResourcePolicy: aws.String(resourcePolicy),
	})
	return err
}

func isSamePolicy(current, desired string) bool {
	var currentPolicy, desiredPolicy interface{}
	if json.Unmarshal([]byte(current), &currentPolicy) != nil || json.Unmarshal([]byte(desired), &desiredPolicy) != nil {
		return false
	}
	return reflect.DeepEqual(currentPolicy, desiredPolicy)
}

func (sm *SecretManager) describeKmsKey(secretName, kmsKeyId string) (string, string) {
	if kmsKeyId == "" || strings.HasPrefix(kmsKeyId, "aws/") {
		return kmsKeyId, kmsKeyId
	}

	kmsKeyArn := kmsKeyId
	aliasName := KmsAliasName(secretName)

	keyOutput, err := sm.kmsClient.DescribeKey(sm.ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(kmsKeyId),
	})
	if err == nil && keyOutput != nil && keyOutput.KeyMetadata != nil {
		kmsKeyArn = aws.ToString(keyOutput.KeyMetadata.Arn)
	} else if err != nil {
		logging.Warn("Could not retrieve KMS key details: %v", err)
	}

	aliasesOutput, err := sm.kmsClient.ListAliases(sm.ctx, &kms.ListAliasesInput{
		KeyId: aws.String(kmsKeyId),
	})
	if err == nil && len(aliasesOutput.Aliases) > 0 {
		aliasName = aws.ToString(aliasesOutput.Aliases[0].AliasName)
	}

	return kmsKeyArn, aliasName
}

// Validate validates a secret against expected keys.
func (sm *SecretManager) Validate(
	secretName string,
	expectedKeys []string,
	versions []string,
) (*SecretValidationResult, error) {
	logging.Debug("Validating secret: %s", secretName)

	// Get secret metadata
	describeOutput, err := sm.smClient.DescribeSecret(sm.ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to describe secret '%s'", secretName)
	}

	secretArn := aws.ToString(describeOutput.ARN)
	kmsKeyId := aws.ToString(describeOutput.KmsKeyId)
	if kmsKeyId == "" {
		kmsKeyId = awsManagedKey
	}

	// Get KMS key details
	kmsKeyArn, aliasName := sm.describeKmsKey(secretName, kmsKeyId)

	// Determine which versions to validate
	versionsToValidate := versions
	if len(versionsToValidate) == 0 {
		versionsToValidate = []string{"AWSCURRENT"}
	}

	// Validate each version
	allStoredKeys := make(map[string]bool)
	validatedVersionIds := []string{}

	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	for _, version := range versionsToValidate {
		var getOutput *secretsmanager.GetSecretValueOutput
		var err error

		// Determine if version is UUID or stage name
		if uuidRegex.MatchString(version) {
			// Version ID
			getOutput, err = sm.smClient.GetSecretValue(sm.ctx, &secretsmanager.GetSecretValueInput{
				SecretId:  aws.String(secretName),
				VersionId: aws.String(version),
			})
		} else {
			// Stage name
			getOutput, err = sm.smClient.GetSecretValue(sm.ctx, &secretsmanager.GetSecretValueInput{
				SecretId:     aws.String(secretName),
				VersionStage: aws.String(version),
			})
		}

		if err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to retrieve secret version '%s'", version)
		}

		validatedVersionIds = append(validatedVersionIds, aws.ToString(getOutput.VersionId))

		// Parse secret to extract keys
		if getOutput.SecretString != nil {
			secretStr := aws.ToString(getOutput.SecretString)
			var secretData map[string]interface{}
			if err := json.Unmarshal([]byte(secretStr), &secretData); err == nil {
				for key := range secretData {
					allStoredKeys[key] = true
				}
			}
		}
	}

	// Bidirectional validation
	expectedSet := make(map[string]bool)
	for _, key := range expectedKeys {
		expectedSet[key] = true
	}

	isExpectedKeysOk := true
	for key := range expectedSet {
		if !allStoredKeys[key] {
			isExpectedKeysOk = false
			break
		}
	}

	isStoredKeysOk := true
	for key := range allStoredKeys {
		if !expectedSet[key] {
			isStoredKeysOk = false
			break
		}
	}

	// Convert to sorted slices
	storedKeysList := make([]string, 0, len(allStoredKeys))
	for key := range allStoredKeys {
		storedKeysList = append(storedKeysList, key)
	}
	sort.Strings(storedKeysList)
	sort.Strings(expectedKeys)

	logging.Debug("Validation result: expected_ok=%v, stored_ok=%v", isExpectedKeysOk, isStoredKeysOk)

	return &SecretValidationResult{
		IsExpectedKeysOk:  isExpectedKeysOk,
		IsStoredKeysOk:    isStoredKeysOk,
		ExpectedKeys:      expectedKeys,
		StoredKeys:        storedKeysList,
		VersionsValidated: validatedVersionIds,
		ARN:               secretArn,
		KmsKeyId:          kmsKeyId,
		KmsKeyArn:         kmsKeyArn,
		KmsAliasName:      aliasName,
	}, nil
}

// Read reads a secret value from Secrets Manager.
func (sm *SecretManager) Read(name, version, stage string) (string, string, error) {
	logging.Debug("Reading secret: %s (version: %s, stage: %s)", name, version, stage)

	var getOutput *secretsmanager.GetSecretValueOutput
	var err error

	if version != "" {
		getOutput, err = sm.smClient.GetSecretValue(sm.ctx, &secretsmanager.GetSecretValueInput{
			SecretId:  aws.String(name),
			VersionId: aws.String(version),
		})
	} else if stage != "" {
		getOutput, err = sm.smClient.GetSecretValue(sm.ctx, &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(name),
			VersionStage: aws.String(stage),
		})
	} else {
		// Default to AWSCURRENT
		getOutput, err = sm.smClient.GetSecretValue(sm.ctx, &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(name),
			VersionStage: aws.String("AWSCURRENT"),
		})
	}

	if err != nil {
		return "", "", errors.Wrapf(errors.ErrFail, err, "failed to read secret '%s'", name)
	}

	versionID := aws.ToString(getOutput.VersionId)
	if getOutput.SecretString != nil {
		return aws.ToString(getOutput.SecretString), versionID, nil
	}

	if getOutput.SecretBinary != nil {
		return string(getOutput.SecretBinary), versionID, nil
	}

	return "", "", errors.New(errors.ErrFail, fmt.Sprintf("secret '%s' has no value", name))
}

// Helper Methods

type secretInfo struct {
	ARN            string
	KmsKeyId       string
	ReplicaRegions []string
	Tags           map[string]string
}

// checkSecretExists checks if a secret exists.
func (sm *SecretManager) checkSecretExists(secretName string) (bool, *secretInfo, error) {
	describeOutput, err := sm.smClient.DescribeSecret(sm.ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if stderrors.As(err, &notFound) {
			return false, nil, nil
		}
		return false, nil, errors.Wrapf(errors.ErrFail, err, "failed to check if secret exists")
	}

	return true, secretInfoFromDescribe(describeOutput), nil
}

func secretInfoFromDescribe(describeOutput *secretsmanager.DescribeSecretOutput) *secretInfo {
	info := &secretInfo{
		ARN:            aws.ToString(describeOutput.ARN),
		KmsKeyId:       aws.ToString(describeOutput.KmsKeyId),
		ReplicaRegions: make([]string, 0, len(describeOutput.ReplicationStatus)),
		Tags:           make(map[string]string, len(describeOutput.Tags)),
	}
	for _, replica := range describeOutput.ReplicationStatus {
		info.ReplicaRegions = append(info.ReplicaRegions, aws.ToString(replica.Region))
	}
	for _, tag := range describeOutput.Tags {
		info.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	return info
}

func secretsManagerTags(tags map[string]string) []types.Tag {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	smTags := make([]types.Tag, 0, len(keys))
	for _, k := range keys {
		smTags = append(smTags, types.Tag{Key: aws.String(k), Value: aws.String(tags[k])})
	}
	return smTags
}

// findKmsKey takes a key ID, key ARN, alias or alias ARN, and returns nil if there is no such key.
func (sm *SecretManager) findKmsKey(keyId string) (*kmstypes.KeyMetadata, error) {
	describeOutput, err := sm.kmsClient.DescribeKey(sm.ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(keyId),
	})
	if err != nil {
		var notFound *kmstypes.NotFoundException
		if stderrors.As(err, &notFound) {
			return nil, nil
		}
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to look up KMS key '%s'", keyId)
	}
	return describeOutput.KeyMetadata, nil
}

// Secret names that differ only by '.' and '-' get the same alias, so a key is only the secret's own if its Name tag says so.
func (sm *SecretManager) isKeyForSecret(keyId, secretName string) (bool, error) {
	tagsOutput, err := sm.kmsClient.ListResourceTags(sm.ctx, &kms.ListResourceTagsInput{
		KeyId: aws.String(keyId),
	})
	if err != nil {
		return false, errors.Wrapf(errors.ErrFail, err, "failed to read the tags of KMS key '%s'", keyId)
	}
	for _, tag := range tagsOutput.Tags {
		if aws.ToString(tag.TagKey) == nameTagKey {
			return aws.ToString(tag.TagValue) == secretName, nil
		}
	}
	return false, nil
}

// createKmsKeyForSecret creates a new KMS key with policy.
func (sm *SecretManager) createKmsKeyForSecret(
	secretName string,
	accountId string,
	description string,
	tags map[string]string,
) (*KmsKeyResult, error) {
	// Generate KMS key policy
	policy, err := sm.generateKmsKeyPolicy(accountId)
	if err != nil {
		return nil, err
	}

	// Create key
	createKeyInput := &kms.CreateKeyInput{
		Description: aws.String(description),
		KeyUsage:    kmstypes.KeyUsageTypeEncryptDecrypt,
		KeySpec:     kmstypes.KeySpecSymmetricDefault,
		Origin:      kmstypes.OriginTypeAwsKms,
		MultiRegion: aws.Bool(false),
		Policy:      aws.String(policy),
	}

	// Add tags if provided
	if len(tags) > 0 {
		keys := make([]string, 0, len(tags))
		for k := range tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		kmsTags := make([]kmstypes.Tag, 0, len(keys))
		for _, k := range keys {
			kmsTags = append(kmsTags, kmstypes.Tag{
				TagKey:   aws.String(k),
				TagValue: aws.String(tags[k]),
			})
		}
		createKeyInput.Tags = kmsTags
	}

	createKeyOutput, err := sm.kmsClient.CreateKey(sm.ctx, createKeyInput)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create KMS key")
	}

	keyId := aws.ToString(createKeyOutput.KeyMetadata.KeyId)
	keyArn := aws.ToString(createKeyOutput.KeyMetadata.Arn)

	// Create alias
	aliasName := KmsAliasName(secretName)
	_, err = sm.kmsClient.CreateAlias(sm.ctx, &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: aws.String(keyId),
	})
	if err != nil {
		// A retry finds the key by its alias, so without one the key would be left behind.
		if deleteErr := sm.deleteKmsKey(keyId); deleteErr != nil {
			return nil, errors.Wrapf(errors.ErrFail, err,
				"failed to create KMS alias '%s', and the new KMS key '%s' could not be scheduled for deletion (%v)", aliasName, keyId, deleteErr)
		}
		return nil, errors.Wrapf(errors.ErrFail, err,
			"failed to create KMS alias '%s', so the new KMS key '%s' was scheduled for deletion", aliasName, keyId)
	}

	// Enable automatic key rotation
	if err := sm.enableKeyRotation(keyId); err != nil {
		return nil, err
	}

	logging.Debug("Created KMS key %s with alias %s (auto-rotation enabled)", keyId, aliasName)

	return &KmsKeyResult{
		KeyId:     keyId,
		KeyArn:    keyArn,
		AliasName: aliasName,
	}, nil
}

func (sm *SecretManager) enableKeyRotation(keyId string) error {
	logging.Debug("Enabling automatic key rotation for KMS key %s", keyId)
	_, err := sm.kmsClient.EnableKeyRotation(sm.ctx, &kms.EnableKeyRotationInput{
		KeyId: aws.String(keyId),
	})
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to enable rotation of KMS key '%s'", keyId)
	}
	return nil
}

// generateKmsKeyPolicy generates the KMS key policy JSON.
func (sm *SecretManager) generateKmsKeyPolicy(accountId string) (string, error) {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Id":      "key-default-1",
		"Statement": []map[string]interface{}{
			{
				"Sid":    "Enable IAM User Permissions",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"AWS": fmt.Sprintf("arn:aws:iam::%s:root", accountId),
				},
				"Action":   "kms:*",
				"Resource": "*",
			},
			{
				"Sid":    "Allow SecretsManager to use the key",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"Service": "secretsmanager.amazonaws.com",
				},
				"Action": []string{
					"kms:Decrypt",
					"kms:GenerateDataKey",
					"kms:CreateGrant",
				},
				"Resource": "*",
			},
		},
	}

	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to marshal KMS policy")
	}

	return string(policyBytes), nil
}

// generateResourcePolicy generates IAM resource policy for secret access restrictions.
func (sm *SecretManager) generateResourcePolicy(accountId string, secretArn string, permissions Permissions) (string, error) {
	if err := permissions.Validate(); err != nil {
		return "", err
	}

	// Build principal ARNs
	principalArns := []string{}

	for _, user := range permissions.RestrictToUsers {
		principalArns = append(principalArns, fmt.Sprintf("arn:aws:iam::%s:user/%s", accountId, user))
	}

	for _, role := range permissions.RestrictToRoles {
		principalArns = append(principalArns, fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, role))
	}

	for _, role := range permissions.RestrictToAssumedRoles {
		principalArns = append(principalArns, fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, role))
	}

	for _, policy := range permissions.RestrictToSsoPolicies {
		principalArns = append(principalArns,
			fmt.Sprintf("arn:aws:iam::%s:role/aws-reserved/sso.amazonaws.com/%s/AWSReservedSSO_%s*",
				accountId, sm.awsRegion, policy))
	}

	// Check if any restrictions specified
	if len(principalArns) == 0 && len(permissions.ExtraPolicyStatements) == 0 {
		return "", nil // No policy needed
	}

	// Build policy statements
	policyStatements := []map[string]interface{}{
		{
			"Sid":    "Enable IAM User Permissions",
			"Effect": "Allow",
			"Principal": map[string]interface{}{
				"AWS": fmt.Sprintf("arn:aws:iam::%s:root", accountId),
			},
			"Action":   "secretsmanager:*",
			"Resource": secretArn,
		},
	}

	if len(principalArns) > 0 {
		// An Allow can't restrict anyone the account already allows, so everyone else is denied.
		policyStatements = append(policyStatements, map[string]interface{}{
			"Sid":    "OnlyAllowNamedResources",
			"Effect": "Deny",
			"Principal": map[string]interface{}{
				"AWS": "*",
			},
			"Action": []string{
				"secretsmanager:DeleteSecret",
				"secretsmanager:DeleteResourcePolicy",
				"secretsmanager:GetSecretValue",
				"secretsmanager:PutSecretValue",
				"secretsmanager:PutResourcePolicy",
				"secretsmanager:ReplicateSecretToRegions",
				"secretsmanager:RestoreSecret",
				"secretsmanager:UpdateSecret",
				"secretsmanager:UpdateSecretVersionStage",
			},
			"Resource": secretArn,
			"Condition": map[string]interface{}{
				"ArnNotLike": map[string]interface{}{
					"aws:PrincipalArn": principalArns,
				},
			},
		})
	}

	// Append extra policy statements
	policyStatements = append(policyStatements, permissions.ExtraPolicyStatements...)

	policy := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": policyStatements,
	}

	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to marshal resource policy")
	}

	return string(policyBytes), nil
}

// generatePlaceholderSecretString generates a placeholder secret JSON.
func (sm *SecretManager) generatePlaceholderSecretString(
	placeholderKeys []string,
	plaintextPlaceholder string,
) (string, error) {
	if len(placeholderKeys) == 0 {
		// Plain text secret
		return plaintextPlaceholder, nil
	}

	// JSON structured secret
	secretObj := make(map[string]interface{})
	for _, key := range placeholderKeys {
		if strings.HasSuffix(key, b64Suffix) {
			secretObj[key] = base64.StdEncoding.EncodeToString([]byte(plaintextPlaceholder))
		} else {
			secretObj[key] = plaintextPlaceholder
		}
	}

	secretBytes, err := json.Marshal(secretObj)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to marshal placeholder secret")
	}

	return string(secretBytes), nil
}

// Destroy destroys a secret and its associated AWS resources.
// This includes the secret itself, its replicas in other regions, and any KMS keys created for it.
// Respects isDryRun setting - if true, no actual destruction occurs.
func (sm *SecretManager) Destroy(secretName string) error {
	logging.Info("Starting destroy operation for secret '%s'", secretName)

	// Step 1: Verify secret exists and get metadata
	exists, info, err := sm.checkSecretExists(secretName)
	if err != nil {
		return err
	}

	if !exists {
		return errors.Newf(errors.ErrFail, "secret '%s' does not exist", secretName)
	}

	// A dry run fails here too, as the real run would.
	if info.Tags[isCreatedHereTagKey] != isCreatedHereTagValue {
		return errors.Newf(errors.ErrFail, "secret '%s' was not created by yago (it has no %s=%s tag) - destruction blocked",
			secretName, isCreatedHereTagKey, isCreatedHereTagValue)
	}

	// Only the key Create made for this secret is deleted: a key it merely uses may encrypt other secrets too.
	aliasName := KmsAliasName(secretName)
	isKeyCreatedForSecret := false
	if info.KmsKeyId != "" && !strings.HasPrefix(info.KmsKeyId, "aws/") {
		aliasKey, err := sm.findKmsKey(aliasName)
		if err != nil {
			return err
		}
		if aliasKey != nil && keyMatches(info.KmsKeyId, aws.ToString(aliasKey.KeyId), aws.ToString(aliasKey.Arn)) {
			isKeyCreatedForSecret, err = sm.isKeyForSecret(aws.ToString(aliasKey.KeyId), secretName)
			if err != nil {
				return err
			}
		}
		if !isKeyCreatedForSecret {
			logging.Info("KMS key '%s' was not created for secret '%s', so it is kept", info.KmsKeyId, secretName)
		}
	}

	// Step 2: In dry-run mode, just log and return
	if sm.isDryRun {
		logging.Info("[Dry-Run] Would delete secret '%s' (ARN: %s)", secretName, info.ARN)
		for _, region := range info.ReplicaRegions {
			logging.Info("[Dry-Run] Would delete replica in region '%s'", region)
		}
		if isKeyCreatedForSecret {
			logging.Info("[Dry-Run] Would schedule KMS key '%s' for deletion", info.KmsKeyId)
			logging.Info("[Dry-Run] Would delete KMS alias '%s'", aliasName)
		}
		return nil
	}

	// Step 3: Delete replicas in other regions first
	if len(info.ReplicaRegions) > 0 {
		logging.Info("Destroying replicas in %s", strings.Join(info.ReplicaRegions, ", "))
		_, err := sm.smClient.RemoveRegionsFromReplication(sm.ctx, &secretsmanager.RemoveRegionsFromReplicationInput{
			SecretId:             aws.String(secretName),
			RemoveReplicaRegions: info.ReplicaRegions,
		})
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to delete the replicas of secret '%s'", secretName)
		}
	}

	// Step 4: Delete main secret in primary region
	logging.Info("Destroying main secret '%s'", secretName)
	if err := sm.deleteSecret(secretName); err != nil {
		return err
	}

	// Step 5: Delete KMS key if it was created for this secret
	if isKeyCreatedForSecret {
		if err := sm.deleteKmsKey(info.KmsKeyId); err != nil {
			return err
		}
		if err := sm.deleteKeyAlias(aliasName); err != nil {
			return err
		}
	}

	logging.Info("Successfully destroyed secret '%s'", secretName)
	return nil
}

func keyMatches(secretKmsKeyId, aliasKeyId, aliasKeyArn string) bool {
	if secretKmsKeyId == "" || aliasKeyId == "" {
		return false
	}
	return secretKmsKeyId == aliasKeyId ||
		(aliasKeyArn != "" && secretKmsKeyId == aliasKeyArn) ||
		strings.HasSuffix(secretKmsKeyId, ":key/"+aliasKeyId)
}

// deleteSecret deletes a secret from Secrets Manager in the current region.
func (sm *SecretManager) deleteSecret(secretName string) error {
	logging.Debug("Deleting secret '%s' from Secrets Manager", secretName)

	_, err := sm.smClient.DeleteSecret(sm.ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretName),
		ForceDeleteWithoutRecovery: aws.Bool(true), // Immediate deletion, no recovery period
	})

	if err != nil {
		var notFound *types.ResourceNotFoundException
		if stderrors.As(err, &notFound) {
			logging.Warn("Secret '%s' not found (may have been already deleted)", secretName)
			return nil
		}
		return errors.Wrapf(errors.ErrFail, err, "failed to delete secret '%s'", secretName)
	}

	logging.Info("Successfully deleted secret '%s'", secretName)
	return nil
}

// deleteKmsKey schedules deletion of a KMS key.
// Note: KMS keys cannot be immediately deleted - AWS requires a 7-30 day waiting period.
func (sm *SecretManager) deleteKmsKey(keyId string) error {
	logging.Debug("Scheduling deletion of KMS key '%s'", keyId)

	_, err := sm.kmsClient.ScheduleKeyDeletion(sm.ctx, &kms.ScheduleKeyDeletionInput{
		KeyId:               aws.String(keyId),
		PendingWindowInDays: aws.Int32(7), // Minimum waiting period
	})

	if err != nil {
		var notFound *kmstypes.NotFoundException
		var invalidState *kmstypes.KMSInvalidStateException
		switch {
		case stderrors.As(err, &notFound):
			logging.Warn("KMS key '%s' not found (may have been already deleted)", keyId)
			return nil
		case stderrors.As(err, &invalidState):
			logging.Warn("KMS key '%s' is in invalid state (may already be scheduled for deletion)", keyId)
			return nil
		}
		return errors.Wrapf(errors.ErrFail, err, "failed to schedule deletion of KMS key '%s'", keyId)
	}

	logging.Info("Scheduled deletion of KMS key '%s' (7-day waiting period)", keyId)
	return nil
}

// deleteKeyAlias deletes a KMS key alias.
func (sm *SecretManager) deleteKeyAlias(aliasName string) error {
	logging.Debug("Deleting KMS alias '%s'", aliasName)

	_, err := sm.kmsClient.DeleteAlias(sm.ctx, &kms.DeleteAliasInput{
		AliasName: aws.String(aliasName),
	})

	if err != nil {
		var notFound *kmstypes.NotFoundException
		if stderrors.As(err, &notFound) {
			logging.Warn("KMS alias '%s' not found (may have been already deleted)", aliasName)
			return nil
		}
		return errors.Wrapf(errors.ErrFail, err, "failed to delete KMS alias '%s'", aliasName)
	}

	logging.Info("Successfully deleted KMS alias '%s'", aliasName)
	return nil
}

// Close closes the SecretManager and releases resources.
func (sm *SecretManager) Close() error {
	// No resources to release in AWS SDK v2
	return nil
}
