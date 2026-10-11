package aws

import (
	"context"
	stderrors "errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

const bucketExistsTimeout = time.Minute

type stateBucketAPI interface {
	s3.HeadBucketAPIClient
	s3.ListBucketsAPIClient
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	DeleteBucket(ctx context.Context, params *s3.DeleteBucketInput, optFns ...func(*s3.Options)) (*s3.DeleteBucketOutput, error)
	GetBucketEncryption(ctx context.Context, params *s3.GetBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.GetBucketEncryptionOutput, error)
	GetBucketLifecycleConfiguration(ctx context.Context, params *s3.GetBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLifecycleConfigurationOutput, error)
	GetBucketPolicy(ctx context.Context, params *s3.GetBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error)
	GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error)
	ListObjectVersions(ctx context.Context, params *s3.ListObjectVersionsInput, optFns ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
	PutBucketEncryption(ctx context.Context, params *s3.PutBucketEncryptionInput, optFns ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error)
	PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error)
	PutBucketOwnershipControls(ctx context.Context, params *s3.PutBucketOwnershipControlsInput, optFns ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error)
	PutBucketPolicy(ctx context.Context, params *s3.PutBucketPolicyInput, optFns ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error)
	PutBucketTagging(ctx context.Context, params *s3.PutBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error)
	PutBucketVersioning(ctx context.Context, params *s3.PutBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error)
	PutPublicAccessBlock(ctx context.Context, params *s3.PutPublicAccessBlockInput, optFns ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)
}

type stateKeyAPI interface {
	CreateKey(ctx context.Context, params *kms.CreateKeyInput, optFns ...func(*kms.Options)) (*kms.CreateKeyOutput, error)
	DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error)
	EnableKeyRotation(ctx context.Context, params *kms.EnableKeyRotationInput, optFns ...func(*kms.Options)) (*kms.EnableKeyRotationOutput, error)
	ScheduleKeyDeletion(ctx context.Context, params *kms.ScheduleKeyDeletionInput, optFns ...func(*kms.Options)) (*kms.ScheduleKeyDeletionOutput, error)
}

// what a backend says about its bucket, whatever it leaves empty is worked out when the bucket is made
type StateBucketSpec struct {
	// empty until the backend pins one, the bucket is then named the way the tagging provider would name it
	Name     string
	Region   string
	KMSKeyID string
	// EncryptionKms when empty
	Encryption string
	// the configuration's project properties, the name and the tags are made from them
	Properties        map[string]interface{}
	RoleExtend        string
	VersioningSources []string
	// the defaults when nil
	Lifecycle *StateBucketLifecycle
	Policy    *StateBucketPolicy
}

// what the backend has to pin once the bucket is there
type StateBucketResult struct {
	Name     string
	KMSKeyID string
}

type StateBucket struct {
	spec StateBucketSpec

	ctx       context.Context
	s3Client  stateBucketAPI
	kmsClient stateKeyAPI
	stsClient stsAPI

	// worked out on first use and kept
	account   string
	tagConfig *TagConfiguration
	name      string
}

type bucketSettings struct {
	algorithm types.ServerSideEncryption
	keyID     string
	tags      map[string]string
	lifecycle []types.LifecycleRule
	// handed back with the rules, a put without it would reset it
	minimumObjectSize types.TransitionDefaultMinimumObjectSize
	// nil when the bucket has no policy
	policy map[string]interface{}
}

// what a create or update is going to do, decided from reads alone, so it can be shown and agreed to before anything
// is changed and a dry run can't disagree with a real one
type StateBucketPlan struct {
	// every change in words, nothing is done that isn't in here
	Changes []string

	creates bool
	// the whole set the bucket ends up with, nil when there is nothing to tag it with
	tags      map[string]string
	keyTags   map[string]string
	algorithm types.ServerSideEncryption
	keyID     string
	// at most one of these is set, neither means the algorithm and key above are put on the bucket
	createsKey      bool
	keepsEncryption bool
	// every rule the bucket ends up with, ours and the ones it already had
	lifecycle         []types.LifecycleRule
	minimumObjectSize types.TransitionDefaultMinimumObjectSize
	// the whole policy document, empty when none is written
	policy string
}

func NewStateBucket(spec StateBucketSpec, profile string) (*StateBucket, error) {
	ctx := context.Background()
	cfg, err := loadConfig(ctx, spec.Region, profile)
	if err != nil {
		return nil, err
	}
	return &StateBucket{
		spec: spec, ctx: ctx,
		s3Client: s3.NewFromConfig(cfg), kmsClient: kms.NewFromConfig(cfg), stsClient: sts.NewFromConfig(cfg),
	}, nil
}

func (b *StateBucket) accountID() (string, error) {
	if b.account == "" {
		account, err := callerAccount(b.ctx, b.stsClient)
		if err != nil {
			return "", err
		}
		b.account = account
	}
	return b.account, nil
}

// nil when the configuration has no project properties
func (b *StateBucket) tagConfiguration() (*TagConfiguration, error) {
	if b.tagConfig != nil || len(b.spec.Properties) == 0 {
		return b.tagConfig, nil
	}
	account, err := b.accountID()
	if err != nil {
		return nil, err
	}
	config, err := TagConfigurationFromProperties(b.spec.Properties, account)
	if err != nil {
		return nil, err
	}
	config.RoleExtend, config.VersioningSources = b.spec.RoleExtend, b.spec.VersioningSources
	b.tagConfig = &config
	return b.tagConfig, nil
}

func (b *StateBucket) Name() (string, error) {
	if b.name != "" {
		return b.name, nil
	}
	if b.spec.Name != "" {
		b.name = b.spec.Name
		return b.name, nil
	}
	config, err := b.tagConfiguration()
	if err != nil {
		return "", err
	}
	if config == nil {
		return "", errors.New(errors.ErrParam, "the backend names no bucket and the configuration has no project properties to name one from")
	}
	name, err := BucketName(*config, b.spec.Region)
	if err != nil {
		return "", err
	}
	b.name = name
	return b.name, nil
}

// every S3 call names the account it expects, a bucket somebody else got to first is then an error and never ours
func (b *StateBucket) owner() *string {
	return aws.String(b.account)
}

func (b *StateBucket) exists() (bool, error) {
	if _, err := b.Name(); err != nil {
		return false, err
	}
	if _, err := b.accountID(); err != nil {
		return false, err
	}
	_, err := b.s3Client.HeadBucket(b.ctx, &s3.HeadBucketInput{Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner()})
	if err == nil {
		return true, nil
	}
	var notFound *types.NotFound
	if stderrors.As(err, &notFound) {
		return false, nil
	}
	return false, errors.Wrapf(errors.ErrFail, err, "cannot check state bucket %s, the name may be taken by another account", b.name)
}

// stops if the bucket is already there, this way one somebody tuned by hand is never reset
func (b *StateBucket) PlanCreate() (*StateBucketPlan, error) {
	found, err := b.exists()
	if err != nil {
		return nil, err
	}
	if found {
		return nil, errors.Newf(errors.ErrFail, "state bucket %s already exists, use update to put its settings back", b.name)
	}
	plan, err := b.plan(nil)
	if err != nil {
		return nil, err
	}
	plan.creates = true
	plan.Changes = slices.Insert(plan.Changes, 0, "create state bucket "+b.name+" in "+b.spec.Region+", in account "+b.account)
	return plan, nil
}

func (b *StateBucket) PlanUpdate() (*StateBucketPlan, error) {
	found, err := b.exists()
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.Newf(errors.ErrFail, "state bucket %s does not exist, use create first", b.name)
	}
	current, err := b.settings()
	if err != nil {
		return nil, err
	}
	plan, err := b.plan(current)
	if err != nil {
		return nil, err
	}
	plan.Changes = slices.Insert(plan.Changes, 0, "update state bucket "+b.name+" in account "+b.account)
	return plan, nil
}

// Apply does what the plan says and nothing else, so the plan is what gets shown and agreed to first
func (b *StateBucket) Apply(plan *StateBucketPlan) (*StateBucketResult, error) {
	if !plan.creates {
		return b.applySettings(plan)
	}

	input := &s3.CreateBucketInput{Bucket: aws.String(b.name)}
	// us-east-1 is the one region that rejects its own name as a location constraint
	if b.spec.Region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(b.spec.Region),
		}
	}
	if _, err := b.s3Client.CreateBucket(b.ctx, input); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create state bucket %s", b.name)
	}
	logging.Info("Created state bucket %s in %s, account %s", b.name, b.spec.Region, b.account)

	waiter := s3.NewBucketExistsWaiter(b.s3Client)
	err := waiter.Wait(b.ctx, &s3.HeadBucketInput{Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner()}, bucketExistsTimeout)
	var result *StateBucketResult
	if err == nil {
		result, err = b.applySettings(plan)
	}
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "state bucket %s was created but not finished, run update to complete it", b.name)
	}
	return result, nil
}

func (b *StateBucket) settings() (*bucketSettings, error) {
	settings := &bucketSettings{tags: map[string]string{}}
	var err error
	if settings.algorithm, settings.keyID, err = b.defaultEncryption(b.name); err != nil {
		return nil, err
	}

	tagging, err := b.s3Client.GetBucketTagging(b.ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner()})
	if err != nil && !isAPIError(err, "NoSuchTagSet") {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to read the tags of %s", b.name)
	}
	if tagging != nil {
		for _, tag := range tagging.TagSet {
			settings.tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}
	}
	if err := b.readRules(settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func (b *StateBucket) defaultEncryption(bucket string) (types.ServerSideEncryption, string, error) {
	encryption, err := b.s3Client.GetBucketEncryption(b.ctx, &s3.GetBucketEncryptionInput{Bucket: aws.String(bucket), ExpectedBucketOwner: b.owner()})
	if err != nil {
		if isAPIError(err, "ServerSideEncryptionConfigurationNotFoundError") {
			return "", "", nil
		}
		return "", "", errors.Wrapf(errors.ErrFail, err, "failed to read the encryption of %s", bucket)
	}
	if encryption.ServerSideEncryptionConfiguration != nil {
		for _, rule := range encryption.ServerSideEncryptionConfiguration.Rules {
			if byDefault := rule.ApplyServerSideEncryptionByDefault; byDefault != nil {
				return byDefault.SSEAlgorithm, aws.ToString(byDefault.KMSMasterKeyID), nil
			}
		}
	}
	return "", "", nil
}

// current is nil for a bucket that isn't there yet
func (b *StateBucket) plan(current *bucketSettings) (*StateBucketPlan, error) {
	plan := &StateBucketPlan{Changes: []string{"turn on versioning, block all public access and switch ACLs off, so the account owns every object"}}
	for _, step := range []func(*StateBucketPlan, *bucketSettings) error{b.planEncryption, b.planLifecycle, b.planPolicy, b.planTags} {
		if err := step(plan, current); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func (b *StateBucket) planTags(plan *StateBucketPlan, current *bucketSettings) error {
	config, err := b.tagConfiguration()
	if err != nil {
		return err
	}
	switch {
	case current == nil:
		// only a bucket made here says so
		if plan.tags, err = b.tags(config, ResourceTypeBucket, b.name, true); err != nil {
			return err
		}
		plan.Changes = append(plan.Changes, "tag it with\n"+FormatTags(plan.tags))
	case config == nil:
		plan.Changes = append(plan.Changes, "leave its tags as they are, the configuration has no project properties to make tags from")
	default:
		// a bucket that says yago made it keeps saying so and keeps the place that takes, one that doesn't is never given it
		ours, err := b.tags(config, ResourceTypeBucket, b.name, current.tags[createdByTag] == createdByValue)
		if err != nil {
			return err
		}
		// PutBucketTagging replaces the whole set, so ours go on top of what the bucket already has
		plan.tags = maps.Clone(current.tags)
		maps.Copy(plan.tags, ours)
		// the aws: tags a bucket made by CloudFormation carries have to be sent back, and S3 doesn't count them
		counted := 0
		for key := range plan.tags {
			if !strings.HasPrefix(key, "aws:") {
				counted++
			}
		}
		if counted > maxTags {
			return errors.Newf(errors.ErrParam, "state bucket %s would end up with %d tags and S3 allows %d, update only adds and overwrites tags, so remove some it no longer needs first", b.name, counted, maxTags)
		}
		plan.Changes = append(plan.Changes, "tag it with\n"+FormatTags(plan.tags))
	}
	if plan.createsKey {
		if plan.keyTags, err = b.tags(config, ResourceTypeKmsKey, "", true); err != nil {
			return err
		}
		plan.Changes = append(plan.Changes, "tag the new key with\n"+FormatTags(plan.keyTags))
	}
	return nil
}

func (b *StateBucket) planEncryption(plan *StateBucketPlan, current *bucketSettings) error {
	wantsKMS := true
	switch b.spec.Encryption {
	case "", EncryptionKms:
	case EncryptionSse:
		if b.spec.KMSKeyID != "" {
			return errors.Newf(errors.ErrParam, "encryption %s can't be used together with a kms_key_id", EncryptionSse)
		}
		wantsKMS = false
	default:
		return errors.Newf(errors.ErrParam, "encryption must be %s or %s, not '%s'", EncryptionKms, EncryptionSse, b.spec.Encryption)
	}

	// state is already written under the bucket's key, so its encryption is left exactly as it is, the bucket key
	// setting included, and a configuration that disagrees is refused
	if current != nil && current.keyID != "" {
		if !wantsKMS {
			return errors.Newf(errors.ErrParam,
				"state bucket %s is encrypted with KMS key %s and the configuration says %s, yago doesn't change the key under existing state",
				b.name, current.keyID, EncryptionSse)
		}
		if b.spec.KMSKeyID != "" && keyName(b.spec.KMSKeyID) != keyName(current.keyID) {
			return errors.Newf(errors.ErrParam,
				"state bucket %s is encrypted with KMS key %s and the configuration pins %s, yago doesn't change the key under existing state, "+
					"and if the two are the same key written differently, pin it the way the bucket has it",
				b.name, current.keyID, b.spec.KMSKeyID)
		}
		plan.algorithm, plan.keyID, plan.keepsEncryption = current.algorithm, current.keyID, true
		plan.Changes = append(plan.Changes, fmt.Sprintf("leave its encryption as it is, %s with KMS key %s", current.algorithm, current.keyID))
		return nil
	}

	plan.algorithm = types.ServerSideEncryptionAes256
	if !wantsKMS && current != nil && current.algorithm == plan.algorithm {
		plan.keepsEncryption = true
		plan.Changes = append(plan.Changes, fmt.Sprintf("leave its encryption as it is, %s with S3's own key", current.algorithm))
		return nil
	}
	const refusesSSEC = "refuse uploads that bring an encryption key of their own, SSE-C"
	encryption := "encrypt it with S3's own key, " + string(plan.algorithm) + ", and " + refusesSSEC
	if wantsKMS {
		plan.algorithm = types.ServerSideEncryptionAwsKms
		keyID, source, err := b.pinnedOrSiblingKey()
		if err != nil {
			return err
		}
		plan.keyID, plan.createsKey = keyID, keyID == ""
		encryption = fmt.Sprintf("encrypt it with KMS key %s, %s, turn on its S3 bucket key and %s", keyID, source, refusesSSEC)
		if plan.createsKey {
			encryption = fmt.Sprintf("create a KMS key described as %q with rotation on, encrypt it with that key, turn on its S3 bucket key and %s, "+
				"and if any of that fails schedule the new key for deletion", b.keyDescription(), refusesSSEC)
		}
	}
	// a bucket without a key of its own, a half made one for example, is the one case where an update moves the encryption
	if current != nil {
		today := "has no default encryption"
		if current.algorithm != "" {
			today = fmt.Sprintf("is %s without a customer key", current.algorithm)
		}
		encryption = fmt.Sprintf("CHANGE ITS ENCRYPTION, which %s today, objects already in it keep the encryption they were written with, %s", today, encryption)
	}
	plan.Changes = append(plan.Changes, encryption)
	return nil
}

// an ARN and a bare key ID name the same key, so only what follows the last slash is compared
func keyName(keyID string) string {
	return keyID[strings.LastIndex(keyID, "/")+1:]
}

// the key and where it comes from, in words for the plan, the key is empty when one has to be made
func (b *StateBucket) pinnedOrSiblingKey() (keyID, source string, err error) {
	if b.spec.KMSKeyID != "" {
		key, err := b.kmsClient.DescribeKey(b.ctx, &kms.DescribeKeyInput{KeyId: aws.String(b.spec.KMSKeyID)})
		if err != nil {
			return "", "", errors.Wrapf(errors.ErrFail, err, "cannot check the pinned KMS key %s", b.spec.KMSKeyID)
		}
		if key.KeyMetadata.KeyState != kmstypes.KeyStateEnabled {
			return "", "", errors.Newf(errors.ErrParam, "the pinned KMS key %s is %s, state can't be encrypted with it", b.spec.KMSKeyID, key.KeyMetadata.KeyState)
		}
		return b.spec.KMSKeyID, "which the configuration pins", nil
	}

	config, err := b.tagConfiguration()
	if err != nil || config == nil {
		return "", "", err
	}
	prefix, shape, err := siblingBucketNames(*config, b.spec.Region)
	if err != nil {
		return "", "", err
	}
	siblings := []string{}
	pages := s3.NewListBucketsPaginator(b.s3Client, &s3.ListBucketsInput{Prefix: aws.String(prefix), BucketRegion: aws.String(b.spec.Region)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(b.ctx)
		if err != nil {
			return "", "", errors.Wrapf(errors.ErrFail, err, "cannot look for the key of another state bucket, pin a kms_key_id to skip the search")
		}
		for _, bucket := range page.Buckets {
			if name := aws.ToString(bucket.Name); name != b.name && shape.MatchString(name) {
				siblings = append(siblings, name)
			}
		}
	}
	slices.Sort(siblings)
	for _, sibling := range siblings {
		_, keyID, err := b.defaultEncryption(sibling)
		if err != nil {
			return "", "", errors.Wrapf(errors.ErrFail, err, "cannot look at the key of state bucket %s, pin a kms_key_id to skip the search", sibling)
		}
		if keyID != "" && b.isOurEnabledKey(keyID) {
			return keyID, "which state bucket " + sibling + " already uses", nil
		}
	}
	return "", "", nil
}

// a key we can't vouch for is not shared, the bucket gets its own instead
func (b *StateBucket) isOurEnabledKey(keyID string) bool {
	key, err := b.kmsClient.DescribeKey(b.ctx, &kms.DescribeKeyInput{KeyId: aws.String(keyID)})
	if err != nil {
		logging.Warn("Not sharing KMS key %s, it could not be checked, %v", keyID, err)
		return false
	}
	metadata := key.KeyMetadata
	if metadata.KeyManager != kmstypes.KeyManagerTypeCustomer || metadata.KeyState != kmstypes.KeyStateEnabled || aws.ToString(metadata.AWSAccountId) != b.account {
		logging.Warn("Not sharing KMS key %s, it is not an enabled customer key of account %s", keyID, b.account)
		return false
	}
	return true
}

// the provider's tags, none when the configuration has no project properties, and CreatedBy on top for what is made here
func (b *StateBucket) tags(config *TagConfiguration, resourceType, name string, isMadeHere bool) (map[string]string, error) {
	tags := map[string]string{}
	if config != nil {
		room := 0
		if isMadeHere {
			room = 1
		}
		var dropped []string
		var err error
		if tags, dropped, err = tagsWithRoom(*config, resourceType, name, room); err != nil {
			return nil, err
		}
		if len(dropped) > 0 {
			logging.Warn("AWS allows %d tags, so %s of %s doesn't get: %s", maxTags, resourceType, b.name, strings.Join(dropped, ", "))
		}
	}
	if isMadeHere {
		tags[createdByTag] = createdByValue
	}
	return tags, nil
}

func (b *StateBucket) applySettings(plan *StateBucketPlan) (*StateBucketResult, error) {
	// tags go on first, update never adds CreatedBy, so a create that stops after this has already left it
	if plan.tags != nil {
		tagSet := make([]types.Tag, 0, len(plan.tags))
		for _, key := range slices.Sorted(maps.Keys(plan.tags)) {
			tagSet = append(tagSet, types.Tag{Key: aws.String(key), Value: aws.String(plan.tags[key])})
		}
		if _, err := b.s3Client.PutBucketTagging(b.ctx, &s3.PutBucketTaggingInput{
			Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(), Tagging: &types.Tagging{TagSet: tagSet},
		}); err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to tag %s", b.name)
		}
	}
	// and ACLs next, the one setting an existing bucket can refuse, before anything but its tags is changed
	if _, err := b.s3Client.PutBucketOwnershipControls(b.ctx, &s3.PutBucketOwnershipControlsInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(),
		OwnershipControls: &types.OwnershipControls{Rules: []types.OwnershipControlsRule{{ObjectOwnership: types.ObjectOwnershipBucketOwnerEnforced}}},
	}); err != nil {
		hint := ""
		if isAPIError(err, "InvalidBucketAclWithObjectOwnership") {
			hint = ", its ACL grants something to others and that has to be removed first, nothing was changed"
			if plan.tags != nil {
				hint = ", its ACL grants something to others and that has to be removed first, its tags were already updated and nothing else was"
			}
		}
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to switch ACLs off for %s%s", b.name, hint)
	}

	if _, err := b.s3Client.PutBucketVersioning(b.ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(),
		VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled},
	}); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to turn on versioning for %s", b.name)
	}

	if _, err := b.s3Client.PutPublicAccessBlock(b.ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	}); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to block public access for %s", b.name)
	}

	keyID := plan.keyID
	if plan.createsKey {
		var err error
		if keyID, err = b.createKey(plan.keyTags); err != nil {
			return nil, err
		}
	}
	if !plan.keepsEncryption {
		if err := b.putEncryption(plan.algorithm, keyID); err != nil {
			if plan.createsKey {
				return nil, b.discardNewKey(keyID, err)
			}
			return nil, err
		}
	}
	if err := b.applyRules(plan); err != nil {
		return nil, err
	}

	logging.Info("State bucket %s has versioning on, public access blocked, %s encryption and its lifecycle rule", b.name, plan.algorithm)
	return &StateBucketResult{Name: b.name, KMSKeyID: keyID}, nil
}

func (b *StateBucket) putEncryption(algorithm types.ServerSideEncryption, keyID string) error {
	byDefault := &types.ServerSideEncryptionByDefault{SSEAlgorithm: algorithm}
	if keyID != "" {
		byDefault.KMSMasterKeyID = aws.String(keyID)
	}
	if _, err := b.s3Client.PutBucketEncryption(b.ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(),
		ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{{
				ApplyServerSideEncryptionByDefault: byDefault,
				BucketKeyEnabled:                   aws.Bool(keyID != ""),
				BlockedEncryptionTypes:             &types.BlockedEncryptionTypes{EncryptionType: []types.EncryptionType{types.EncryptionTypeSseC}},
			}},
		},
	}); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to set encryption for %s", b.name)
	}
	return nil
}

func (b *StateBucket) keyDescription() string {
	return "Encrypts Terraform state, made for " + b.name
}

func (b *StateBucket) createKey(tags map[string]string) (string, error) {
	created, err := b.kmsClient.CreateKey(b.ctx, &kms.CreateKeyInput{
		Description: aws.String(b.keyDescription()),
		Tags:        kmsTags(tags),
	})
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to create a KMS key for %s", b.name)
	}
	keyARN := aws.ToString(created.KeyMetadata.Arn)

	if _, err := b.kmsClient.EnableKeyRotation(b.ctx, &kms.EnableKeyRotationInput{KeyId: aws.String(keyARN)}); err != nil {
		return "", b.discardNewKey(keyARN, errors.Wrapf(errors.ErrFail, err, "failed to turn on rotation for a new KMS key"))
	}
	logging.Info("Created KMS key %s with rotation on", keyARN)
	return keyARN, nil
}

// a key made in this run that nothing ended up using would only be found again on the bill, and the next run would make another
func (b *StateBucket) discardNewKey(keyARN string, cause error) error {
	if _, err := b.kmsClient.ScheduleKeyDeletion(b.ctx, &kms.ScheduleKeyDeletionInput{
		KeyId: aws.String(keyARN), PendingWindowInDays: aws.Int32(kmsKeyDeletionDays),
	}); err != nil {
		return errors.Wrapf(errors.ErrFail, cause, "the new KMS key %s is not in use and could not be scheduled for deletion (%v)", keyARN, err)
	}
	return errors.Wrapf(errors.ErrFail, cause, "the new KMS key %s is not in use, so it was scheduled for deletion", keyARN)
}

func (b *StateBucket) EnsureDeletable() error {
	found, err := b.exists()
	if err != nil {
		return err
	}
	if !found {
		return errors.Newf(errors.ErrFail, "state bucket %s does not exist", b.name)
	}

	// old versions and delete markers count, a state somebody deleted last week is still recoverable from them
	listed, err := b.s3Client.ListObjectVersions(b.ctx, &s3.ListObjectVersionsInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(), MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "cannot check that state bucket %s is empty", b.name)
	}
	if len(listed.Versions)+len(listed.DeleteMarkers) > 0 {
		return errors.Newf(errors.ErrFail, "state bucket %s still holds objects or old versions of them, yago does not delete state, empty it yourself first", b.name)
	}
	return nil
}

// the KMS key is left alone, another bucket may be encrypting with it
func (b *StateBucket) Delete() error {
	if err := b.EnsureDeletable(); err != nil {
		return err
	}
	if _, err := b.s3Client.DeleteBucket(b.ctx, &s3.DeleteBucketInput{Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner()}); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to delete state bucket %s", b.name)
	}
	logging.Info("Deleted state bucket %s", b.name)
	return nil
}
