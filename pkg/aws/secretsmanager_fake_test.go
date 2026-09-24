package aws

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

var mutatingCalls = map[string]bool{
	"CreateAlias": true, "CreateKey": true, "CreateSecret": true, "DeleteAlias": true, "DeleteSecret": true,
	"EnableKeyRotation": true, "PutResourcePolicy": true, "RemoveRegionsFromReplication": true, "ScheduleKeyDeletion": true,
}

type fakeAWS struct {
	accountId string
	region    string

	secrets map[string]*fakeSecret
	keys    map[string]*fakeKey
	aliases map[string]string

	createAliasErr       error
	putResourcePolicyErr error
	removeReplicasErr    error
	calls                []string
}

type fakeSecret struct {
	arn         string
	description string
	kmsKeyId    string
	value       string
	versionId   string
	tags        map[string]string
	policy      string
	replicas    []string
}

type fakeKey struct {
	arn             string
	tags            map[string]string
	rotation        bool
	pendingDeletion bool
}

func newFakeAWS() *fakeAWS {
	return &fakeAWS{
		accountId: "123456789012",
		region:    "eu-west-1",
		secrets:   map[string]*fakeSecret{},
		keys:      map[string]*fakeKey{},
		aliases:   map[string]string{},
	}
}

func (f *fakeAWS) secretManager(isDryRun bool) *SecretManager {
	return &SecretManager{
		awsRegion: f.region,
		isDryRun:  isDryRun,
		smClient:  f,
		kmsClient: f,
		stsClient: f,
		ctx:       context.Background(),
	}
}

func (f *fakeAWS) addKey(keyId string) {
	f.keys[keyId] = &fakeKey{arn: fmt.Sprintf("arn:aws:kms:%s:%s:key/%s", f.region, f.accountId, keyId), tags: map[string]string{}}
}

func (f *fakeAWS) addSecret(name, kmsKeyId string, tags map[string]string) {
	f.secrets[name] = &fakeSecret{
		arn:      fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:%s-AbCdEf", f.region, f.accountId, name),
		kmsKeyId: kmsKeyId,
		tags:     tags,
	}
}

func (f *fakeAWS) secretByID(id string) *fakeSecret {
	if secret, exists := f.secrets[id]; exists {
		return secret
	}
	for _, secret := range f.secrets {
		if secret.arn == id {
			return secret
		}
	}
	return nil
}

func (f *fakeAWS) record(call string) {
	f.calls = append(f.calls, call)
}

func (f *fakeAWS) called(call string) bool {
	for _, c := range f.calls {
		if c == call {
			return true
		}
	}
	return false
}

func (f *fakeAWS) mutations() []string {
	mutations := []string{}
	for _, call := range f.calls {
		if mutatingCalls[call] {
			mutations = append(mutations, call)
		}
	}
	return mutations
}

func (f *fakeAWS) GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.record("GetCallerIdentity")
	return &sts.GetCallerIdentityOutput{Account: aws.String(f.accountId)}, nil
}

func (f *fakeAWS) CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	f.record("CreateSecret")
	name := aws.ToString(params.Name)
	if _, exists := f.secrets[name]; exists {
		return nil, &types.ResourceExistsException{Message: aws.String("exists")}
	}
	if len(params.Tags) > maxTags {
		return nil, &types.InvalidParameterException{Message: aws.String("too many tags")}
	}
	tags := map[string]string{}
	for _, tag := range params.Tags {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	f.addSecret(name, aws.ToString(params.KmsKeyId), tags)
	secret := f.secrets[name]
	secret.description = aws.ToString(params.Description)
	secret.value = aws.ToString(params.SecretString)
	secret.versionId = "11111111-2222-3333-4444-555555555555"
	return &secretsmanager.CreateSecretOutput{ARN: aws.String(secret.arn), Name: params.Name, VersionId: aws.String(secret.versionId)}, nil
}

func (f *fakeAWS) DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	f.record("DeleteSecret")
	name := aws.ToString(params.SecretId)
	secret, exists := f.secrets[name]
	if !exists {
		return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
	}
	if len(secret.replicas) > 0 {
		return nil, &types.InvalidRequestException{Message: aws.String("remove the replicas first")}
	}
	delete(f.secrets, name)
	return &secretsmanager.DeleteSecretOutput{}, nil
}

func (f *fakeAWS) DescribeSecret(ctx context.Context, params *secretsmanager.DescribeSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	f.record("DescribeSecret")
	secret, exists := f.secrets[aws.ToString(params.SecretId)]
	if !exists {
		return nil, &types.ResourceNotFoundException{Message: aws.String("Secrets Manager can't find the specified secret.")}
	}
	output := &secretsmanager.DescribeSecretOutput{ARN: aws.String(secret.arn), Name: params.SecretId}
	if secret.kmsKeyId != "" {
		output.KmsKeyId = aws.String(secret.kmsKeyId)
	}
	keys := make([]string, 0, len(secret.tags))
	for k := range secret.tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		output.Tags = append(output.Tags, types.Tag{Key: aws.String(k), Value: aws.String(secret.tags[k])})
	}
	for _, region := range secret.replicas {
		output.ReplicationStatus = append(output.ReplicationStatus, types.ReplicationStatusType{Region: aws.String(region)})
	}
	return output, nil
}

func (f *fakeAWS) GetResourcePolicy(ctx context.Context, params *secretsmanager.GetResourcePolicyInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetResourcePolicyOutput, error) {
	f.record("GetResourcePolicy")
	secret := f.secretByID(aws.ToString(params.SecretId))
	if secret == nil {
		return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
	}
	output := &secretsmanager.GetResourcePolicyOutput{ARN: aws.String(secret.arn)}
	if secret.policy != "" {
		output.ResourcePolicy = aws.String(secret.policy)
	}
	return output, nil
}

func (f *fakeAWS) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	f.record("GetSecretValue")
	secret, exists := f.secrets[aws.ToString(params.SecretId)]
	if !exists {
		return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(secret.value), VersionId: aws.String(secret.versionId)}, nil
}

func (f *fakeAWS) PutResourcePolicy(ctx context.Context, params *secretsmanager.PutResourcePolicyInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutResourcePolicyOutput, error) {
	f.record("PutResourcePolicy")
	if f.putResourcePolicyErr != nil {
		return nil, f.putResourcePolicyErr
	}
	secret := f.secretByID(aws.ToString(params.SecretId))
	if secret == nil {
		return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
	}
	secret.policy = aws.ToString(params.ResourcePolicy)
	return &secretsmanager.PutResourcePolicyOutput{}, nil
}

func (f *fakeAWS) RemoveRegionsFromReplication(ctx context.Context, params *secretsmanager.RemoveRegionsFromReplicationInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.RemoveRegionsFromReplicationOutput, error) {
	f.record("RemoveRegionsFromReplication")
	if f.removeReplicasErr != nil {
		return nil, f.removeReplicasErr
	}
	secret := f.secretByID(aws.ToString(params.SecretId))
	if secret == nil {
		return nil, &types.ResourceNotFoundException{Message: aws.String("not found")}
	}
	remaining := []string{}
	for _, region := range secret.replicas {
		removed := false
		for _, remove := range params.RemoveReplicaRegions {
			removed = removed || region == remove
		}
		if !removed {
			remaining = append(remaining, region)
		}
	}
	secret.replicas = remaining
	return &secretsmanager.RemoveRegionsFromReplicationOutput{}, nil
}

func (f *fakeAWS) ListAliases(ctx context.Context, params *kms.ListAliasesInput, optFns ...func(*kms.Options)) (*kms.ListAliasesOutput, error) {
	f.record("ListAliases")
	names := make([]string, 0, len(f.aliases))
	for name := range f.aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	output := &kms.ListAliasesOutput{}
	for _, name := range names {
		keyId := f.aliases[name]
		if params.KeyId != nil && aws.ToString(params.KeyId) != keyId {
			continue
		}
		output.Aliases = append(output.Aliases, kmstypes.AliasListEntry{AliasName: aws.String(name), TargetKeyId: aws.String(keyId)})
	}
	return output, nil
}

func (f *fakeAWS) CreateAlias(ctx context.Context, params *kms.CreateAliasInput, optFns ...func(*kms.Options)) (*kms.CreateAliasOutput, error) {
	f.record("CreateAlias")
	if f.createAliasErr != nil {
		return nil, f.createAliasErr
	}
	name := aws.ToString(params.AliasName)
	if !kmsAliasNameRe.MatchString(name) {
		return nil, &kmstypes.InvalidAliasNameException{Message: aws.String("invalid alias name")}
	}
	if _, exists := f.aliases[name]; exists {
		return nil, &kmstypes.AlreadyExistsException{Message: aws.String("alias exists")}
	}
	f.aliases[name] = aws.ToString(params.TargetKeyId)
	return &kms.CreateAliasOutput{}, nil
}

func (f *fakeAWS) CreateKey(ctx context.Context, params *kms.CreateKeyInput, optFns ...func(*kms.Options)) (*kms.CreateKeyOutput, error) {
	f.record("CreateKey")
	keyId := fmt.Sprintf("key-%d", len(f.keys)+1)
	f.addKey(keyId)
	for _, tag := range params.Tags {
		f.keys[keyId].tags[aws.ToString(tag.TagKey)] = aws.ToString(tag.TagValue)
	}
	return &kms.CreateKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{KeyId: aws.String(keyId), Arn: aws.String(f.keys[keyId].arn)}}, nil
}

func (f *fakeAWS) DeleteAlias(ctx context.Context, params *kms.DeleteAliasInput, optFns ...func(*kms.Options)) (*kms.DeleteAliasOutput, error) {
	f.record("DeleteAlias")
	name := aws.ToString(params.AliasName)
	if _, exists := f.aliases[name]; !exists {
		return nil, &kmstypes.NotFoundException{Message: aws.String("not found")}
	}
	delete(f.aliases, name)
	return &kms.DeleteAliasOutput{}, nil
}

func (f *fakeAWS) DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	f.record("DescribeKey")
	keyId := aws.ToString(params.KeyId)
	if aliasKeyId, isAlias := f.aliases[keyId]; isAlias {
		keyId = aliasKeyId
	}
	key, exists := f.keys[keyId]
	if !exists {
		return nil, &kmstypes.NotFoundException{Message: aws.String("not found")}
	}
	state := kmstypes.KeyStateEnabled
	if key.pendingDeletion {
		state = kmstypes.KeyStatePendingDeletion
	}
	return &kms.DescribeKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{KeyId: aws.String(keyId), Arn: aws.String(key.arn), KeyState: state}}, nil
}

func (f *fakeAWS) EnableKeyRotation(ctx context.Context, params *kms.EnableKeyRotationInput, optFns ...func(*kms.Options)) (*kms.EnableKeyRotationOutput, error) {
	f.record("EnableKeyRotation")
	f.keys[aws.ToString(params.KeyId)].rotation = true
	return &kms.EnableKeyRotationOutput{}, nil
}

func (f *fakeAWS) ListResourceTags(ctx context.Context, params *kms.ListResourceTagsInput, optFns ...func(*kms.Options)) (*kms.ListResourceTagsOutput, error) {
	f.record("ListResourceTags")
	key, exists := f.keys[aws.ToString(params.KeyId)]
	if !exists {
		return nil, &kmstypes.NotFoundException{Message: aws.String("not found")}
	}
	output := &kms.ListResourceTagsOutput{}
	for tagKey, tagValue := range key.tags {
		output.Tags = append(output.Tags, kmstypes.Tag{TagKey: aws.String(tagKey), TagValue: aws.String(tagValue)})
	}
	return output, nil
}

func (f *fakeAWS) ScheduleKeyDeletion(ctx context.Context, params *kms.ScheduleKeyDeletionInput, optFns ...func(*kms.Options)) (*kms.ScheduleKeyDeletionOutput, error) {
	f.record("ScheduleKeyDeletion")
	key, exists := f.keys[aws.ToString(params.KeyId)]
	if !exists {
		return nil, &kmstypes.NotFoundException{Message: aws.String("not found")}
	}
	key.pendingDeletion = true
	return &kms.ScheduleKeyDeletionOutput{}, nil
}
