package aws

import (
	"context"
	"encoding/json"
	"fmt"
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

// SecretManager manages AWS Secrets Manager operations for secret lifecycle management.
type SecretManager struct {
	awsProfile string
	awsRegion  string
	isDryRun   bool

	// AWS clients
	smClient  *secretsmanager.Client
	kmsClient *kms.Client
	stsClient *sts.Client

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

	// Create AWS clients
	smClient := secretsmanager.NewFromConfig(cfg)
	kmsClient := kms.NewFromConfig(cfg)
	stsClient := sts.NewFromConfig(cfg)

	return &SecretManager{
		awsProfile: awsProfile,
		awsRegion:  awsRegion,
		isDryRun:   isDryRun,
		smClient:   smClient,
		kmsClient:  kmsClient,
		stsClient:  stsClient,
		ctx:        ctx,
	}, nil
}

// Create creates a new secret in AWS Secrets Manager with KMS encryption.
func (sm *SecretManager) Create(
	name string,
	description string,
	placeholderKeys []string,
	plaintextPlaceholder string,
	recoveryWindowInDays int32,
	isCreatedHere bool,
	tags map[string]string,
	autoRotationIsEnabled bool,
	autoRotationIntervalInDays int32,
	autoRotationLambdaArn string,
	restrictToUsers []string,
	restrictToGroups []string,
	restrictToRoles []string,
	restrictToAssumedRoles []string,
	restrictToSsoPolicies []string,
	extraPolicyStatements []map[string]interface{},
) (*SecretCreationResult, error) {
	logging.Info("Processing secret: %s (isCreatedHere=%v, isDryRun=%v)",
		name, isCreatedHere, sm.isDryRun)

	// Handle dry-run mode - return early without AWS calls
	if sm.isDryRun {
		logging.Info("[Dry-Run] Would create secret '%s' with KMS key", name)
		return &SecretCreationResult{
			ARN:          fmt.Sprintf("arn:aws:secretsmanager:%s:123456789012:secret:%s-XXXXXX", sm.awsRegion, name),
			ID:           name,
			Name:         name,
			KmsKeyId:     "XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX",
			KmsKeyArn:    fmt.Sprintf("arn:aws:kms:%s:123456789012:key/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX", sm.awsRegion),
			KmsAliasName: fmt.Sprintf("alias/%s", strings.ReplaceAll(name, ".", "-")),
			VersionId:    "XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX",
		}, nil
	}

	// Get AWS account ID
	stsOutput, err := sm.stsClient.GetCallerIdentity(sm.ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to get AWS account ID")
	}
	accountId := aws.ToString(stsOutput.Account)

	// Handle isCreatedHere=False: retrieve existing secret
	if !isCreatedHere {
		logging.Info("isCreatedHere=False: retrieving existing secret '%s'", name)

		exists, secretInfo, err := sm.checkSecretExists(name)
		if err != nil {
			return nil, err
		}

		if !exists {
			return nil, errors.New(errors.ErrFail,
				fmt.Sprintf("Secret '%s' does not exist but isCreatedHere=False", name))
		}

		secretArn := secretInfo["ARN"].(string)
		kmsKeyId := secretInfo["KmsKeyId"].(string)
		if kmsKeyId == "" {
			kmsKeyId = "aws/secretsmanager"
		}
		versionId := secretInfo["VersionId"].(string)

		// Get KMS key details if using custom key
		kmsKeyArn := kmsKeyId
		aliasName := fmt.Sprintf("alias/%s", strings.ReplaceAll(name, ".", "-"))

		if !strings.HasPrefix(kmsKeyId, "aws/") {
			keyOutput, err := sm.kmsClient.DescribeKey(sm.ctx, &kms.DescribeKeyInput{
				KeyId: aws.String(kmsKeyId),
			})
			if err == nil && keyOutput != nil && keyOutput.KeyMetadata != nil {
				kmsKeyArn = aws.ToString(keyOutput.KeyMetadata.Arn)
			}

			// Get alias name
			aliasesOutput, err := sm.kmsClient.ListAliases(sm.ctx, &kms.ListAliasesInput{
				KeyId: aws.String(kmsKeyId),
			})
			if err == nil && len(aliasesOutput.Aliases) > 0 {
				aliasName = aws.ToString(aliasesOutput.Aliases[0].AliasName)
			}
		}

		logging.Info("Retrieved existing secret '%s' (ARN: %s)", name, secretArn)

		return &SecretCreationResult{
			ARN:          secretArn,
			ID:           name,
			Name:         name,
			KmsKeyId:     kmsKeyId,
			KmsKeyArn:    kmsKeyArn,
			KmsAliasName: aliasName,
			VersionId:    versionId,
		}, nil
	}

	// Handle isCreatedHere=True: create resources with idempotency

	// Check if secret already exists
	exists, secretInfo, err := sm.checkSecretExists(name)
	if err != nil {
		return nil, err
	}

	if exists {
		logging.Warn("Secret '%s' already exists, returning existing information", name)

		secretArn := secretInfo["ARN"].(string)
		kmsKeyId := secretInfo["KmsKeyId"].(string)
		if kmsKeyId == "" {
			kmsKeyId = "aws/secretsmanager"
		}
		versionId := secretInfo["VersionId"].(string)

		kmsKeyArn := kmsKeyId
		aliasName := fmt.Sprintf("alias/%s", strings.ReplaceAll(name, ".", "-"))

		if !strings.HasPrefix(kmsKeyId, "aws/") {
			keyOutput, err := sm.kmsClient.DescribeKey(sm.ctx, &kms.DescribeKeyInput{
				KeyId: aws.String(kmsKeyId),
			})
			if err == nil && keyOutput != nil && keyOutput.KeyMetadata != nil {
				kmsKeyArn = aws.ToString(keyOutput.KeyMetadata.Arn)
			}
		}

		return &SecretCreationResult{
			ARN:          secretArn,
			ID:           name,
			Name:         name,
			KmsKeyId:     kmsKeyId,
			KmsKeyArn:    kmsKeyArn,
			KmsAliasName: aliasName,
			VersionId:    versionId,
		}, nil
	}

	// Secret doesn't exist - create it
	logging.Info("Secret '%s' does not exist, will create", name)

	// Step 1: Create or get KMS key
	aliasName := fmt.Sprintf("alias/%s", strings.ReplaceAll(name, ".", "-"))
	kmsExists, existingKeyId, existingKeyArn, err := sm.checkKmsKeyExists(aliasName)
	if err != nil {
		return nil, err
	}

	var kmsInfo *KmsKeyResult

	if kmsExists {
		logging.Warn("KMS key with alias '%s' already exists, using existing key", aliasName)
		kmsInfo = &KmsKeyResult{
			KeyId:     existingKeyId,
			KeyArn:    existingKeyArn,
			AliasName: aliasName,
		}
	} else {
		logging.Info("Creating new KMS key for secret '%s'", name)
		kmsInfo, err = sm.createKmsKeyForSecret(name, accountId, fmt.Sprintf("KMS key for secret %s", name), tags)
		if err != nil {
			return nil, err
		}
	}

	// Step 2: Generate placeholder secret string
	secretString, err := sm.generatePlaceholderSecretString(placeholderKeys, plaintextPlaceholder)
	if err != nil {
		return nil, err
	}

	// Step 3: Create the secret with KMS encryption
	logging.Info("Creating secret '%s'", name)

	createParams := &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		KmsKeyId:     aws.String(kmsInfo.KeyId),
		SecretString: aws.String(secretString),
	}

	if description != "" {
		createParams.Description = aws.String(description)
	}

	// Add tags if provided
	if len(tags) > 0 {
		secretTags := make([]types.Tag, 0, len(tags))
		for k, v := range tags {
			secretTags = append(secretTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		createParams.Tags = secretTags
	}

	createOutput, err := sm.smClient.CreateSecret(sm.ctx, createParams)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create secret '%s'", name)
	}

	secretArn := aws.ToString(createOutput.ARN)
	versionId := aws.ToString(createOutput.VersionId)

	logging.Info("Created secret '%s' (ARN: %s)", name, secretArn)

	// Step 4: Apply resource policy if restrictions specified
	resourcePolicy := sm.generateResourcePolicy(accountId, secretArn, restrictToUsers, restrictToGroups,
		restrictToRoles, restrictToAssumedRoles, restrictToSsoPolicies, extraPolicyStatements)

	if resourcePolicy != "" {
		logging.Info("Applying resource policy to secret '%s'", name)
		_, err := sm.smClient.PutResourcePolicy(sm.ctx, &secretsmanager.PutResourcePolicyInput{
			SecretId:       aws.String(secretArn),
			ResourcePolicy: aws.String(resourcePolicy),
		})
		if err != nil {
			logging.Warn("Failed to apply resource policy: %v", err)
			// Don't fail - secret is created, policy can be added later
		}
	}

	// Step 5: Configure automatic rotation if specified
	if autoRotationIsEnabled {
		if autoRotationLambdaArn == "" {
			logging.Warn("Auto-rotation enabled for '%s' but no Lambda ARN provided", name)
		} else {
			logging.Info("Configuring auto-rotation for secret '%s'", name)
			_, err := sm.smClient.RotateSecret(sm.ctx, &secretsmanager.RotateSecretInput{
				SecretId:          aws.String(secretArn),
				RotationLambdaARN: aws.String(autoRotationLambdaArn),
				RotationRules: &types.RotationRulesType{
					AutomaticallyAfterDays: aws.Int64(int64(autoRotationIntervalInDays)),
				},
			})
			if err != nil {
				logging.Warn("Failed to configure rotation for '%s': %v", name, err)
				// Don't fail - secret is created, rotation can be added later
			}
		}
	}

	return &SecretCreationResult{
		ARN:          secretArn,
		ID:           name,
		Name:         name,
		KmsKeyId:     kmsInfo.KeyId,
		KmsKeyArn:    kmsInfo.KeyArn,
		KmsAliasName: kmsInfo.AliasName,
		VersionId:    versionId,
	}, nil
}

// Validate validates a secret against expected keys.
func (sm *SecretManager) Validate(
	secretName string,
	expectedKeys []string,
	versions []string,
) (*SecretValidationResult, error) {
	logging.Debug("Validating secret: %s", secretName)

	if sm.isDryRun {
		logging.Debug("[Dry-Run] Would validate secret '%s'", secretName)
		sort.Strings(expectedKeys)
		return &SecretValidationResult{
			IsExpectedKeysOk:  true,
			IsStoredKeysOk:    true,
			ExpectedKeys:      expectedKeys,
			StoredKeys:        expectedKeys,
			VersionsValidated: []string{},
			ARN:               fmt.Sprintf("arn:aws:secretsmanager:%s:123456789012:secret:%s-XXXXXX", sm.awsRegion, secretName),
		}, nil
	}

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
		kmsKeyId = "aws/secretsmanager"
	}

	// Get KMS key details
	kmsKeyArn := kmsKeyId
	kmsAliasName := kmsKeyId
	if !strings.HasPrefix(kmsKeyId, "aws/") {
		keyOutput, err := sm.kmsClient.DescribeKey(sm.ctx, &kms.DescribeKeyInput{
			KeyId: aws.String(kmsKeyId),
		})
		if err == nil && keyOutput != nil && keyOutput.KeyMetadata != nil {
			kmsKeyArn = aws.ToString(keyOutput.KeyMetadata.Arn)
		}

		aliasesOutput, err := sm.kmsClient.ListAliases(sm.ctx, &kms.ListAliasesInput{
			KeyId: aws.String(kmsKeyId),
		})
		if err == nil && len(aliasesOutput.Aliases) > 0 {
			kmsAliasName = aws.ToString(aliasesOutput.Aliases[0].AliasName)
		}
	}

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
		KmsAliasName:      kmsAliasName,
	}, nil
}

// Read reads a secret value from Secrets Manager.
func (sm *SecretManager) Read(name, version, stage string) (string, error) {
	logging.Debug("Reading secret: %s (version: %s, stage: %s)", name, version, stage)

	if sm.isDryRun {
		logging.Debug("[Dry-Run] Would read secret '%s'", name)
		return "<placeholder>", nil
	}

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
		return "", errors.Wrapf(errors.ErrFail, err, "failed to read secret '%s'", name)
	}

	if getOutput.SecretString != nil {
		return aws.ToString(getOutput.SecretString), nil
	}

	if getOutput.SecretBinary != nil {
		return string(getOutput.SecretBinary), nil
	}

	return "", errors.New(errors.ErrFail, fmt.Sprintf("secret '%s' has no value", name))
}

// Helper Methods

// checkSecretExists checks if a secret exists.
func (sm *SecretManager) checkSecretExists(secretName string) (bool, map[string]interface{}, error) {
	describeOutput, err := sm.smClient.DescribeSecret(sm.ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		// Check if error is ResourceNotFoundException
		if strings.Contains(err.Error(), "ResourceNotFoundException") {
			return false, nil, nil
		}
		return false, nil, errors.Wrapf(errors.ErrFail, err, "failed to check if secret exists")
	}

	info := map[string]interface{}{
		"ARN":       aws.ToString(describeOutput.ARN),
		"KmsKeyId":  aws.ToString(describeOutput.KmsKeyId),
		"VersionId": "", // Will get version from name in many cases
	}

	return true, info, nil
}

// checkKmsKeyExists checks if a KMS key exists by alias.
func (sm *SecretManager) checkKmsKeyExists(aliasName string) (bool, string, string, error) {
	listOutput, err := sm.kmsClient.ListAliases(sm.ctx, &kms.ListAliasesInput{})
	if err != nil {
		return false, "", "", errors.Wrapf(errors.ErrFail, err, "failed to list KMS aliases")
	}

	for _, alias := range listOutput.Aliases {
		if aws.ToString(alias.AliasName) == aliasName {
			keyId := aws.ToString(alias.TargetKeyId)

			// Get key ARN
			describeOutput, err := sm.kmsClient.DescribeKey(sm.ctx, &kms.DescribeKeyInput{
				KeyId: aws.String(keyId),
			})
			if err != nil {
				return true, keyId, "", nil // Return key ID even if we can't get ARN
			}

			keyArn := aws.ToString(describeOutput.KeyMetadata.Arn)
			return true, keyId, keyArn, nil
		}
	}

	return false, "", "", nil
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
		KeyUsage:    "ENCRYPT_DECRYPT",
		Origin:      "AWS_KMS",
		MultiRegion: aws.Bool(false),
		Policy:      aws.String(policy),
	}

	// Add tags if provided
	if len(tags) > 0 {
		kmsTags := make([]kmstypes.Tag, 0, len(tags))
		for k, v := range tags {
			kmsTags = append(kmsTags, kmstypes.Tag{
				TagKey:   aws.String(k),
				TagValue: aws.String(v),
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

	// Enable automatic key rotation
	logging.Debug("Enabling automatic key rotation for KMS key %s", keyId)
	_, err = sm.kmsClient.EnableKeyRotation(sm.ctx, &kms.EnableKeyRotationInput{
		KeyId: aws.String(keyId),
	})
	if err != nil {
		logging.Warn("Failed to enable key rotation: %v", err)
	}

	// Create alias
	aliasName := fmt.Sprintf("alias/%s", strings.ReplaceAll(secretName, ".", "-"))
	_, err = sm.kmsClient.CreateAlias(sm.ctx, &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: aws.String(keyId),
	})
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create KMS alias")
	}

	logging.Debug("Created KMS key %s with alias %s", keyId, aliasName)

	return &KmsKeyResult{
		KeyId:     keyId,
		KeyArn:    keyArn,
		AliasName: aliasName,
	}, nil
}

// generateKmsKeyPolicy generates the KMS key policy JSON.
func (sm *SecretManager) generateKmsKeyPolicy(accountId string) (string, error) {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
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
				"Sid":    "Allow SecretsManager Service",
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
func (sm *SecretManager) generateResourcePolicy(
	accountId string,
	secretArn string,
	restrictToUsers []string,
	restrictToGroups []string,
	restrictToRoles []string,
	restrictToAssumedRoles []string,
	restrictToSsoPolicies []string,
	extraPolicyStatements []map[string]interface{},
) string {
	// Check if any restrictions specified
	hasRestrictions := len(restrictToUsers) > 0 || len(restrictToGroups) > 0 ||
		len(restrictToRoles) > 0 || len(restrictToAssumedRoles) > 0 ||
		len(restrictToSsoPolicies) > 0 || len(extraPolicyStatements) > 0

	if !hasRestrictions {
		return "" // No policy needed
	}

	// Build principal ARNs
	principalArns := []string{}

	for _, user := range restrictToUsers {
		principalArns = append(principalArns, fmt.Sprintf("arn:aws:iam::%s:user/%s", accountId, user))
	}

	for _, group := range restrictToGroups {
		principalArns = append(principalArns, fmt.Sprintf("arn:aws:iam::%s:group/%s", accountId, group))
	}

	for _, role := range restrictToRoles {
		principalArns = append(principalArns, fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, role))
	}

	for _, role := range restrictToAssumedRoles {
		principalArns = append(principalArns, fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, role))
	}

	for _, policy := range restrictToSsoPolicies {
		principalArns = append(principalArns,
			fmt.Sprintf("arn:aws:iam::%s:role/aws-reserved/sso.amazonaws.com/%s/AWSReservedSSO_%s*",
				accountId, sm.awsRegion, policy))
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
		{
			"Sid":    "OnlyAllowNamedResources",
			"Effect": "Allow",
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
				"ArnEquals": map[string]interface{}{
					"aws:PrincipalArn": principalArns,
				},
			},
		},
	}

	// Append extra policy statements
	if len(extraPolicyStatements) > 0 {
		policyStatements = append(policyStatements, extraPolicyStatements...)
	}

	policy := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": policyStatements,
	}

	policyBytes, err := json.Marshal(policy)
	if err != nil {
		logging.Warn("Failed to marshal resource policy: %v", err)
		return ""
	}

	return string(policyBytes)
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
		secretObj[key] = plaintextPlaceholder
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
	exists, secretInfo, err := sm.checkSecretExists(secretName)
	if err != nil {
		return err
	}

	if !exists {
		return errors.Newf(errors.ErrFail, "secret '%s' does not exist", secretName)
	}

	logging.Info("Found secret '%s' to destroy", secretName)

	secretArn := secretInfo["ARN"].(string)
	kmsKeyId := secretInfo["KmsKeyId"].(string)
	replicaRegions := secretInfo["ReplicaRegions"].([]string)

	// Step 2: In dry-run mode, just log and return
	if sm.isDryRun {
		logging.Info("[Dry-Run] Would destroy secret '%s' (ARN: %s)", secretName, secretArn)
		if kmsKeyId != "" && !strings.HasPrefix(kmsKeyId, "aws/") {
			logging.Info("[Dry-Run] Would destroy KMS key '%s'", kmsKeyId)
		}
		for _, region := range replicaRegions {
			logging.Info("[Dry-Run] Would destroy replica in region '%s'", region)
		}
		return nil
	}

	// Step 3: Delete replicas in other regions first
	if len(replicaRegions) > 0 {
		logging.Info("Destroying %d replicas in other regions", len(replicaRegions))
		for _, region := range replicaRegions {
			if err := sm.deleteSecretInRegion(secretName, region); err != nil {
				logging.Warn("Failed to delete replica in region '%s': %v", region, err)
				// Continue with other regions
			}
		}
	}

	// Step 4: Delete main secret in primary region
	logging.Info("Destroying main secret '%s'", secretName)
	if err := sm.deleteSecret(secretName); err != nil {
		return err
	}

	// Step 5: Delete KMS key if it was created for this secret
	if kmsKeyId != "" && !strings.HasPrefix(kmsKeyId, "aws/") {
		logging.Info("Deleting KMS key '%s' associated with secret", kmsKeyId)

		// Delete alias first
		aliasName := fmt.Sprintf("alias/%s", strings.ReplaceAll(secretName, ".", "-"))
		if err := sm.deleteKeyAlias(aliasName); err != nil {
			logging.Warn("Failed to delete KMS alias '%s': %v", aliasName, err)
			// Continue anyway
		}

		// Schedule key deletion
		if err := sm.deleteKmsKey(kmsKeyId); err != nil {
			logging.Warn("Failed to delete KMS key '%s': %v", kmsKeyId, err)
			// This may fail if key is in use or doesn't exist - not fatal
		}
	}

	logging.Info("Successfully destroyed secret '%s' and associated resources", secretName)
	return nil
}

// deleteSecret deletes a secret from Secrets Manager in the current region.
func (sm *SecretManager) deleteSecret(secretName string) error {
	logging.Debug("Deleting secret '%s' from Secrets Manager", secretName)

	_, err := sm.smClient.DeleteSecret(sm.ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretName),
		ForceDeleteWithoutRecovery: aws.Bool(true), // Immediate deletion, no recovery period
	})

	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to delete secret '%s'", secretName)
	}

	logging.Info("Successfully deleted secret '%s'", secretName)
	return nil
}

// deleteSecretInRegion deletes a secret replica from a specific region.
func (sm *SecretManager) deleteSecretInRegion(secretName, region string) error {
	logging.Debug("Deleting secret replica '%s' from region '%s'", secretName, region)

	// Create a new SM client for the target region
	cfg, err := config.LoadDefaultConfig(sm.ctx,
		config.WithRegion(region),
		config.WithSharedConfigProfile(sm.awsProfile),
	)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to load config for region '%s'", region)
	}

	regionalSmClient := secretsmanager.NewFromConfig(cfg)
	_, err = regionalSmClient.DeleteSecret(sm.ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretName),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})

	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to delete secret replica in region '%s'", region)
	}

	logging.Info("Successfully deleted secret replica '%s' from region '%s'", secretName, region)
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
