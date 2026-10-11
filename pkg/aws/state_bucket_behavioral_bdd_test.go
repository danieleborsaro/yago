package aws

import (
	"context"
	stderrors "errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const (
	exampleStateBucketName = "foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8"
	exampleSiblingName     = "foo-m-management-inception-state--other-euw1-b5fc611b8f3dca8f95"
	examplePinnedName      = "example-dev-statefile-euw1"
	exampleKMSKeyID        = "00000000-0000-0000-0000-000000000000"
	exampleKMSKeyARN       = "arn:aws:kms:eu-west-1:123456789012:key/" + exampleKMSKeyID
	otherKMSKeyARN         = "arn:aws:kms:eu-west-1:123456789012:key/11111111-1111-1111-1111-111111111111"
)

type fakeStateBucket struct {
	// empty for the test account
	owner       string
	versioning  bool
	publicBlock bool
	bucketKey   bool
	algorithm   types.ServerSideEncryption
	kmsKeyID    string
	location    string
	tags        map[string]string
	versions    int
	markers     int
	ownership   types.ObjectOwnership
	blocksSSEC  bool
	lifecycle   []types.LifecycleRule
	minimumSize types.TransitionDefaultMinimumObjectSize
	// the JSON document, empty when the bucket has none
	policy string
}

type fakeS3 struct {
	account string
	buckets map[string]*fakeStateBucket
	// a call named here fails with that error
	errs  map[string]error
	calls []string
}

// the S3 side is faked here, KMS and STS come from the fake the secrets tests already use
type stateBucketFixture struct {
	s3  *fakeS3
	aws *fakeAWS
}

// buckets start the way S3 hands them out, in the test region and on its own key
func newStateBucketFixture(buckets map[string]*fakeStateBucket) *stateBucketFixture {
	if buckets == nil {
		buckets = map[string]*fakeStateBucket{}
	}
	fixture := &stateBucketFixture{aws: newFakeAWS()}
	for _, bucket := range buckets {
		if bucket.location == "" {
			bucket.location = fixture.aws.region
		}
		if bucket.algorithm == "" {
			bucket.algorithm = types.ServerSideEncryptionAes256
		}
		if bucket.kmsKeyID != "" {
			fixture.aws.addKey(keyName(bucket.kmsKeyID))
		}
	}
	fixture.s3 = &fakeS3{account: fixture.aws.accountId, buckets: buckets, errs: map[string]error{}}
	return fixture
}

// drives a bucket the way the command does, plan first and apply only what was planned, a dry run stops after the plan
type plannedBucket struct {
	*StateBucket
	isDryRun bool
	plan     *StateBucketPlan
}

func (f *stateBucketFixture) bucket(spec StateBucketSpec, isDryRun bool) *plannedBucket {
	if spec.Region == "" {
		spec.Region = f.aws.region
	}
	return &plannedBucket{
		StateBucket: &StateBucket{spec: spec, ctx: context.Background(), s3Client: f.s3, kmsClient: f.aws, stsClient: f.aws},
		isDryRun:    isDryRun,
	}
}

func (b *plannedBucket) Create() (*StateBucketResult, error) { return b.run(b.PlanCreate) }

func (b *plannedBucket) Update() (*StateBucketResult, error) { return b.run(b.PlanUpdate) }

func (b *plannedBucket) run(plan func() (*StateBucketPlan, error)) (*StateBucketResult, error) {
	planned, err := plan()
	if err != nil {
		return nil, err
	}
	b.plan = planned
	if b.isDryRun {
		return &StateBucketResult{Name: b.name, KMSKeyID: planned.keyID}, nil
	}
	return b.Apply(planned)
}

func (b *plannedBucket) changes() string {
	return strings.Join(b.plan.Changes, "\n")
}

func (f *stateBucketFixture) mutations() []string {
	mutations := f.aws.mutations()
	for _, call := range f.s3.calls {
		if strings.HasPrefix(call, "Put") || strings.HasPrefix(call, "Create") || strings.HasPrefix(call, "Delete") {
			mutations = append(mutations, call)
		}
	}
	return mutations
}

func exampleStateBucketProperties() map[string]interface{} {
	return map[string]interface{}{
		"company_name_short": "foo",
		"infra_environment":  "management",
		"project_name_short": "inception",
		"project_name_long":  "example project",
		"resource_set_short": "state",
		"resource_set_long":  "example resource set",
		"owner":              "team@example.com",
		"cost_centre":        "cc-1234",
		"compliance":         "internal",
		"accounts_coding": map[string]interface{}{
			"123456789012": map[string]interface{}{"class": "management", "name": "management", "name_canonical": "Management", "name_encoded": "mgmt"},
		},
	}
}

func (f *fakeS3) record(call string) error {
	f.calls = append(f.calls, call)
	return f.errs[call]
}

// what ExpectedBucketOwner does in S3, a call that doesn't name the account the bucket is in gets refused
func (f *fakeS3) owned(call string, name, expectedOwner *string) (*fakeStateBucket, error) {
	if err := f.record(call); err != nil {
		return nil, err
	}
	bucket, exists := f.buckets[aws.ToString(name)]
	if !exists {
		return nil, &types.NotFound{}
	}
	owner := bucket.owner
	if owner == "" {
		owner = f.account
	}
	if aws.ToString(expectedOwner) != owner {
		return nil, &smithy.GenericAPIError{Code: "AccessDenied", Message: "the bucket is not in the expected account"}
	}
	return bucket, nil
}

func (f *fakeS3) HeadBucket(_ context.Context, in *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	_, err := f.owned("HeadBucket", in.Bucket, in.ExpectedBucketOwner)
	return &s3.HeadBucketOutput{}, err
}

func (f *fakeS3) ListBuckets(_ context.Context, in *s3.ListBucketsInput, _ ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	if err := f.record("ListBuckets"); err != nil {
		return nil, err
	}
	out := &s3.ListBucketsOutput{}
	for name, bucket := range f.buckets {
		if strings.HasPrefix(name, aws.ToString(in.Prefix)) && bucket.location == aws.ToString(in.BucketRegion) {
			out.Buckets = append(out.Buckets, types.Bucket{Name: aws.String(name)})
		}
	}
	return out, nil
}

func (f *fakeS3) CreateBucket(_ context.Context, in *s3.CreateBucketInput, _ ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	if err := f.record("CreateBucket"); err != nil {
		return nil, err
	}
	bucket := &fakeStateBucket{location: "us-east-1", algorithm: types.ServerSideEncryptionAes256}
	if in.CreateBucketConfiguration != nil {
		bucket.location = string(in.CreateBucketConfiguration.LocationConstraint)
	}
	f.buckets[aws.ToString(in.Bucket)] = bucket
	return &s3.CreateBucketOutput{}, nil
}

func (f *fakeS3) PutBucketVersioning(_ context.Context, in *s3.PutBucketVersioningInput, _ ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
	bucket, err := f.owned("PutBucketVersioning", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	bucket.versioning = in.VersioningConfiguration.Status == types.BucketVersioningStatusEnabled
	return &s3.PutBucketVersioningOutput{}, nil
}

func (f *fakeS3) PutPublicAccessBlock(_ context.Context, in *s3.PutPublicAccessBlockInput, _ ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	bucket, err := f.owned("PutPublicAccessBlock", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	block := in.PublicAccessBlockConfiguration
	bucket.publicBlock = aws.ToBool(block.BlockPublicAcls) && aws.ToBool(block.BlockPublicPolicy) &&
		aws.ToBool(block.IgnorePublicAcls) && aws.ToBool(block.RestrictPublicBuckets)
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (f *fakeS3) GetBucketEncryption(_ context.Context, in *s3.GetBucketEncryptionInput, _ ...func(*s3.Options)) (*s3.GetBucketEncryptionOutput, error) {
	bucket, err := f.owned("GetBucketEncryption", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	byDefault := &types.ServerSideEncryptionByDefault{SSEAlgorithm: bucket.algorithm}
	if bucket.kmsKeyID != "" {
		byDefault.KMSMasterKeyID = aws.String(bucket.kmsKeyID)
	}
	return &s3.GetBucketEncryptionOutput{ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
		Rules: []types.ServerSideEncryptionRule{{ApplyServerSideEncryptionByDefault: byDefault, BucketKeyEnabled: aws.Bool(bucket.bucketKey)}},
	}}, nil
}

func (f *fakeS3) PutBucketEncryption(_ context.Context, in *s3.PutBucketEncryptionInput, _ ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	bucket, err := f.owned("PutBucketEncryption", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	rule := in.ServerSideEncryptionConfiguration.Rules[0]
	bucket.algorithm = rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm
	bucket.kmsKeyID = aws.ToString(rule.ApplyServerSideEncryptionByDefault.KMSMasterKeyID)
	bucket.bucketKey = aws.ToBool(rule.BucketKeyEnabled)
	bucket.blocksSSEC = rule.BlockedEncryptionTypes != nil && slices.Contains(rule.BlockedEncryptionTypes.EncryptionType, types.EncryptionTypeSseC)
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (f *fakeS3) PutBucketOwnershipControls(_ context.Context, in *s3.PutBucketOwnershipControlsInput, _ ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error) {
	bucket, err := f.owned("PutBucketOwnershipControls", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	bucket.ownership = in.OwnershipControls.Rules[0].ObjectOwnership
	return &s3.PutBucketOwnershipControlsOutput{}, nil
}

func (f *fakeS3) GetBucketLifecycleConfiguration(_ context.Context, in *s3.GetBucketLifecycleConfigurationInput, _ ...func(*s3.Options)) (*s3.GetBucketLifecycleConfigurationOutput, error) {
	bucket, err := f.owned("GetBucketLifecycleConfiguration", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	if len(bucket.lifecycle) == 0 {
		return nil, &smithy.GenericAPIError{Code: "NoSuchLifecycleConfiguration"}
	}
	return &s3.GetBucketLifecycleConfigurationOutput{Rules: bucket.lifecycle, TransitionDefaultMinimumObjectSize: bucket.minimumSize}, nil
}

func (f *fakeS3) PutBucketLifecycleConfiguration(_ context.Context, in *s3.PutBucketLifecycleConfigurationInput, _ ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	bucket, err := f.owned("PutBucketLifecycleConfiguration", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	bucket.lifecycle, bucket.minimumSize = in.LifecycleConfiguration.Rules, in.TransitionDefaultMinimumObjectSize
	return &s3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (f *fakeS3) GetBucketPolicy(_ context.Context, in *s3.GetBucketPolicyInput, _ ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error) {
	bucket, err := f.owned("GetBucketPolicy", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	if bucket.policy == "" {
		return nil, &smithy.GenericAPIError{Code: "NoSuchBucketPolicy"}
	}
	return &s3.GetBucketPolicyOutput{Policy: aws.String(bucket.policy)}, nil
}

func (f *fakeS3) PutBucketPolicy(_ context.Context, in *s3.PutBucketPolicyInput, _ ...func(*s3.Options)) (*s3.PutBucketPolicyOutput, error) {
	bucket, err := f.owned("PutBucketPolicy", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	// S3 refuses a policy without a statement
	if strings.Contains(aws.ToString(in.Policy), `"Statement":[]`) {
		return nil, &smithy.GenericAPIError{Code: "MalformedPolicy"}
	}
	bucket.policy = aws.ToString(in.Policy)
	return &s3.PutBucketPolicyOutput{}, nil
}

func (f *fakeS3) GetBucketTagging(_ context.Context, in *s3.GetBucketTaggingInput, _ ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error) {
	bucket, err := f.owned("GetBucketTagging", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	if len(bucket.tags) == 0 {
		return nil, &smithy.GenericAPIError{Code: "NoSuchTagSet"}
	}
	out := &s3.GetBucketTaggingOutput{}
	for key, value := range bucket.tags {
		out.TagSet = append(out.TagSet, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	return out, nil
}

func (f *fakeS3) PutBucketTagging(_ context.Context, in *s3.PutBucketTaggingInput, _ ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
	bucket, err := f.owned("PutBucketTagging", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	bucket.tags = map[string]string{}
	for _, tag := range in.Tagging.TagSet {
		bucket.tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return &s3.PutBucketTaggingOutput{}, nil
}

func (f *fakeS3) ListObjectVersions(_ context.Context, in *s3.ListObjectVersionsInput, _ ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error) {
	bucket, err := f.owned("ListObjectVersions", in.Bucket, in.ExpectedBucketOwner)
	if err != nil {
		return nil, err
	}
	return &s3.ListObjectVersionsOutput{
		Versions:      make([]types.ObjectVersion, bucket.versions),
		DeleteMarkers: make([]types.DeleteMarkerEntry, bucket.markers),
	}, nil
}

func (f *fakeS3) DeleteBucket(_ context.Context, in *s3.DeleteBucketInput, _ ...func(*s3.Options)) (*s3.DeleteBucketOutput, error) {
	if _, err := f.owned("DeleteBucket", in.Bucket, in.ExpectedBucketOwner); err != nil {
		return nil, err
	}
	delete(f.buckets, aws.ToString(in.Bucket))
	return &s3.DeleteBucketOutput{}, nil
}

func TestStateBucket_CreateNamesEncryptsAndTagsLikeTheProvider_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Create a state bucket the backend doesn't pin yet, named and tagged as the awstagging provider would, with a KMS key of its own",
		CurrentImpl:     "StateBucket.Create names it with BucketName, plans from the properties, then apply creates the key and sets versioning, the public access block, encryption and tags",
		ExpectedOutcome: "The provider's name and tags on the bucket, a rotating key carrying the provider's KMS tags as its default encryption with the bucket key on, and both handed back to pin",
		Rationale:       "The bucket has to exist before Terraform can make anything, and it should still look like Terraform made it",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: no bucket, and a backend that names neither a bucket nor a key
	fixture := newStateBucketFixture(nil)

	// When: the bucket is created
	result, err := fixture.bucket(StateBucketSpec{Properties: exampleStateBucketProperties()}, false).Create()

	// Then: it carries the provider's name, a new rotating key and the provider's tags
	if err != nil {
		t.Fatal(err)
	}
	bucket := fixture.s3.buckets[exampleStateBucketName]
	if bucket == nil {
		t.Fatalf("buckets = %v, want %s", fixture.s3.buckets, exampleStateBucketName)
	}
	key := fixture.aws.keys["key-1"]
	if key == nil || !key.rotation || key.pendingDeletion {
		t.Fatalf("key = %+v, want one new key with rotation on", key)
	}
	if !bucket.versioning || !bucket.publicBlock || !bucket.bucketKey || bucket.location != "eu-west-1" ||
		bucket.algorithm != types.ServerSideEncryptionAwsKms || bucket.kmsKeyID != key.arn {
		t.Errorf("bucket = %+v", bucket)
	}
	if result.Name != exampleStateBucketName || result.KMSKeyID != key.arn {
		t.Errorf("result = %+v", result)
	}

	// the same tags the provider's own code gives these inputs, less the Terraform module it would name
	wantTags := map[string]string{
		"Foo:Business:CostCentre":          "cc-1234",
		"Foo:Business:Owner":               "team@example.com",
		"Foo:Business:Project":             "Example Project",
		"Foo:Environment:Account":          "Management",
		"Foo:Environment:AppEnvironment":   "Management",
		"Foo:Environment:InfraEnvironment": "Management",
		"Foo:Environment:ResourceSet":      "Example Resource Set",
		"Foo:Environment:ResourceType":     ":AWS::KMS::Key",
		"Foo:Security:Compliance":          "internal",
		"CreatedBy":                        "Yago",
	}
	if !maps.Equal(key.tags, wantTags) {
		t.Errorf("key tags = %v, want %v", key.tags, wantTags)
	}
	wantTags["Foo:Environment:ResourceType"] = ":AWS::S3::Bucket"
	wantTags["Name"] = exampleStateBucketName
	if !maps.Equal(bucket.tags, wantTags) {
		t.Errorf("bucket tags = %v, want %v", bucket.tags, wantTags)
	}
}

func TestStateBucket_CreateSharesTheKeyOfAnotherStateBucket_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A new state bucket shares the KMS key of another state bucket the account has in the region, and gets its own when there is none it can vouch for",
		CurrentImpl:     "pinnedOrSiblingKey lists the buckets whose names differ only by role_extend, and isOurEnabledKey describes the key before it is shared",
		ExpectedOutcome: "A sibling's enabled customer key is shared, and a bucket of another role, project or region, a sibling on S3's own key, or a key that is disabled, AWS managed or from another account is not",
		Rationale:       "One key per account and region is enough for state, but state must never end up depending on a key something else owns",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	onKMS := func() *fakeStateBucket {
		return &fakeStateBucket{algorithm: types.ServerSideEncryptionAwsKms, kmsKeyID: exampleKMSKeyARN}
	}
	// the provider leaves a role out when it repeats the resource set, and brings it back once it is extended
	repeatedRole := func(properties map[string]interface{}) { properties["role"] = "state" }
	cases := []struct {
		name       string
		buckets    map[string]*fakeStateBucket
		properties func(map[string]interface{})
		key        func(*fakeKey)
		keyIsGone  bool
		shared     bool
	}{
		{name: "sibling on a KMS key", buckets: map[string]*fakeStateBucket{exampleSiblingName: onKMS()}, shared: true},
		{name: "role repeats the resource set, sibling with another extend", properties: repeatedRole, shared: true,
			buckets: map[string]*fakeStateBucket{"foo-m-management-inception-state-state-other-euw1-0123456789ab": onKMS()}},
		{name: "role repeats the resource set, bucket of another role", properties: repeatedRole,
			buckets: map[string]*fakeStateBucket{"foo-m-management-inception-state-logs-euw1-0123456789abcdef01": onKMS()}},
		{name: "role repeats the resource set, another app environment", properties: repeatedRole,
			buckets: map[string]*fakeStateBucket{"foo-m-management-inception-state-staging-euw1-0123456789abcd": onKMS()}},
		{name: "a sibling on S3's own key sorts first, the next one has a key", shared: true, buckets: map[string]*fakeStateBucket{
			"foo-m-management-inception-state--another-euw1-0123456789abcdef": {},
			exampleSiblingName: onKMS(),
		}},
		{name: "sibling on S3's own key", buckets: map[string]*fakeStateBucket{exampleSiblingName: {}}},
		{name: "bucket of another project", buckets: map[string]*fakeStateBucket{"foo-m-management-network-logs-euw1-0123456789abcdef0123456": onKMS()}},
		{name: "same resource set, another role", buckets: map[string]*fakeStateBucket{"foo-m-management-inception-state-logs-euw1-0123456789abcdef01": onKMS()}},
		{name: "same resource set, another app environment", buckets: map[string]*fakeStateBucket{"foo-m-management-inception-state-staging-euw1-0123456789abcd": onKMS()}},
		{name: "sibling in another region", buckets: map[string]*fakeStateBucket{
			"foo-m-management-inception-state--other-euc1-0123456789abcdef01": {
				algorithm: types.ServerSideEncryptionAwsKms, kmsKeyID: exampleKMSKeyARN, location: "eu-central-1",
			},
		}},
		{name: "sibling's key can't be described", buckets: map[string]*fakeStateBucket{exampleSiblingName: onKMS()}, keyIsGone: true},
		{name: "sibling's key is pending deletion", buckets: map[string]*fakeStateBucket{exampleSiblingName: onKMS()},
			key: func(key *fakeKey) { key.pendingDeletion = true }},
		{name: "sibling's key is AWS managed", buckets: map[string]*fakeStateBucket{exampleSiblingName: onKMS()},
			key: func(key *fakeKey) { key.manager = kmstypes.KeyManagerTypeAws }},
		{name: "sibling's key is another account's", buckets: map[string]*fakeStateBucket{exampleSiblingName: onKMS()},
			key: func(key *fakeKey) { key.account = "210987654321" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: those other buckets already in the account
			fixture := newStateBucketFixture(tc.buckets)
			if tc.key != nil {
				tc.key(fixture.aws.keys[exampleKMSKeyID])
			}
			if tc.keyIsGone {
				delete(fixture.aws.keys, exampleKMSKeyID)
			}
			keysBefore := len(fixture.aws.keys)
			properties := exampleStateBucketProperties()
			if tc.properties != nil {
				tc.properties(properties)
			}

			// When: a state bucket with another role_extend is created
			result, err := fixture.bucket(StateBucketSpec{Properties: properties, RoleExtend: "dev"}, false).Create()

			// Then: a key is only made when there was none to share
			if err != nil {
				t.Fatal(err)
			}
			if created := len(fixture.aws.keys) - keysBefore; (created == 0) != tc.shared {
				t.Errorf("keys created = %d, shared should be %v", created, tc.shared)
			}
			if tc.shared && result.KMSKeyID != exampleKMSKeyARN {
				t.Errorf("key = %q, want the sibling's %q", result.KMSKeyID, exampleKMSKeyARN)
			}
			if bucket := fixture.s3.buckets[result.Name]; bucket == nil || bucket.kmsKeyID != result.KMSKeyID || result.KMSKeyID == "" {
				t.Errorf("bucket = %+v, result = %+v", bucket, result)
			}
		})
	}
}

func TestStateBucket_CreateUsesWhatTheBackendPins_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Create uses the bucket name and KMS key the backend pins, and sse means S3's own key",
		CurrentImpl:     "StateBucket.Name returns the pinned name, planEncryption only looks for or makes a key when none is pinned and encryption isn't sse",
		ExpectedOutcome: "The pinned name and key are used as they are, no key is ever created, and us-east-1 gets no location constraint",
		Rationale:       "Buckets that predate yago have names it would never generate, and a second key for a bucket that has one is only a cost",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: no buckets, and a key somebody made earlier
	fixture := newStateBucketFixture(nil)
	fixture.aws.addKey(exampleKMSKeyID)

	// When: one is created with a pinned name and key, and one in us-east-1 with sse
	_, pinnedErr := fixture.bucket(StateBucketSpec{Name: examplePinnedName, KMSKeyID: exampleKMSKeyARN}, false).Create()
	_, sseErr := fixture.bucket(StateBucketSpec{Name: "example-dev-statefile-use1", Region: "us-east-1", Encryption: EncryptionSse}, false).Create()

	// Then: each has the encryption its backend asked for and no key was made
	if pinnedErr != nil || sseErr != nil {
		t.Fatalf("pinned = %v, sse = %v", pinnedErr, sseErr)
	}
	pinned, sse := fixture.s3.buckets[examplePinnedName], fixture.s3.buckets["example-dev-statefile-use1"]
	if pinned.algorithm != types.ServerSideEncryptionAwsKms || pinned.kmsKeyID != exampleKMSKeyARN || !pinned.bucketKey {
		t.Errorf("pinned bucket = %+v", pinned)
	}
	if sse.algorithm != types.ServerSideEncryptionAes256 || sse.kmsKeyID != "" || sse.bucketKey || sse.location != "us-east-1" {
		t.Errorf("sse bucket = %+v", sse)
	}
	if len(fixture.aws.keys) != 1 {
		t.Errorf("keys = %v, want only the pinned one", fixture.aws.keys)
	}
}

func TestStateBucket_CreateChecksEverythingBeforeItMakesTheBucket_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Create stops before CreateBucket on anything that could still fail, the name, the account, the tags, the encryption and the search for a key",
		CurrentImpl:     "StateBucket.Create runs exists and plan, which only read and validate, before the first write",
		ExpectedOutcome: "An error and nothing changed in S3 or KMS, for a real run and for a dry run alike",
		Rationale:       "A bucket name can't be given back, so a create that fails halfway on something it could have known leaves a bare bucket behind for good",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	untaggable := exampleStateBucketProperties()
	delete(untaggable, "owner")
	otherAccount := exampleStateBucketProperties()
	otherAccount["accounts_coding"] = map[string]interface{}{"210987654321": map[string]interface{}{"class": "production"}}

	cases := []struct {
		name    string
		spec    StateBucketSpec
		buckets map[string]*fakeStateBucket
		errs    map[string]error
		pending bool
	}{
		{name: "bucket exists", spec: StateBucketSpec{Name: examplePinnedName}, buckets: map[string]*fakeStateBucket{examplePinnedName: {}}},
		{name: "name held by another account", spec: StateBucketSpec{Name: examplePinnedName},
			buckets: map[string]*fakeStateBucket{examplePinnedName: {owner: "210987654321"}}},
		{name: "no name and nothing to name from", spec: StateBucketSpec{}},
		{name: "unknown encryption", spec: StateBucketSpec{Name: examplePinnedName, Encryption: "aes"}},
		{name: "sse with a key", spec: StateBucketSpec{Name: examplePinnedName, Encryption: EncryptionSse, KMSKeyID: exampleKMSKeyARN}},
		{name: "properties that name but can't tag", spec: StateBucketSpec{Properties: untaggable}},
		{name: "pinned name in an account the properties don't know", spec: StateBucketSpec{Name: examplePinnedName, Properties: otherAccount}},
		{name: "pinned key that doesn't exist", spec: StateBucketSpec{Name: examplePinnedName, KMSKeyID: otherKMSKeyARN}},
		{name: "pinned key pending deletion", spec: StateBucketSpec{Name: examplePinnedName, KMSKeyID: exampleKMSKeyARN}, pending: true},
		{name: "the search for a key fails", spec: StateBucketSpec{Properties: exampleStateBucketProperties()},
			errs: map[string]error{"ListBuckets": &smithy.GenericAPIError{Code: "AccessDenied"}}},
	}
	for _, tc := range cases {
		for _, isDryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, dry run %v", tc.name, isDryRun), func(t *testing.T) {
				// Given: that bucket, spec and failure
				fixture := newStateBucketFixture(tc.buckets)
				maps.Copy(fixture.s3.errs, tc.errs)
				fixture.aws.addKey(exampleKMSKeyID)
				fixture.aws.keys[exampleKMSKeyID].pendingDeletion = tc.pending

				// When: create runs
				_, err := fixture.bucket(tc.spec, isDryRun).Create()

				// Then: it stops and nothing was changed
				if err == nil {
					t.Error("expected an error")
				}
				if mutations := fixture.mutations(); len(mutations) > 0 {
					t.Errorf("mutating calls = %v", mutations)
				}
			})
		}
	}
}

func TestStateBucket_UpdateKeepsTheKeyAndTagsABucketHas_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Update puts the settings back without writing the encryption of a bucket that has a KMS key or dropping tags added by hand, and never makes a bucket",
		CurrentImpl:     "planEncryption marks the plan as keeping the encryption, so apply skips PutBucketEncryption, and plan merges the provider's tags over GetBucketTagging",
		ExpectedOutcome: "The bucket keeps its key, its algorithm, its bucket key setting and the hand added tag, gets versioning and the provider's tags, and a missing bucket is an error",
		Rationale:       "State written with one key has to stay readable, even turning the bucket key on changes what a key policy sees, and PutBucketTagging replaces the whole set",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := []struct {
		name      string
		algorithm types.ServerSideEncryption
		pinned    string
	}{
		{name: "no key pinned", algorithm: types.ServerSideEncryptionAwsKms},
		{name: "the same key pinned by its bare ID", algorithm: types.ServerSideEncryptionAwsKms, pinned: exampleKMSKeyID},
		{name: "dual layer encryption is kept as it is", algorithm: types.ServerSideEncryptionAwsKmsDsse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a bucket on its own KMS key with a tag added by hand
			fixture := newStateBucketFixture(map[string]*fakeStateBucket{exampleStateBucketName: {
				algorithm: tc.algorithm, kmsKeyID: exampleKMSKeyARN, tags: map[string]string{"AddedByHand": "yes", "Name": "by-hand"},
			}})

			// When: update runs on it, and on a name that doesn't exist
			result, err := fixture.bucket(StateBucketSpec{Properties: exampleStateBucketProperties(), KMSKeyID: tc.pinned}, false).Update()
			_, missingErr := fixture.bucket(StateBucketSpec{Name: "example-dev-statefile-typo"}, false).Update()

			// Then: the encryption was never written, the tag is still there, and the typo made no bucket
			if err != nil {
				t.Fatal(err)
			}
			bucket := fixture.s3.buckets[exampleStateBucketName]
			if !bucket.versioning || !bucket.publicBlock || bucket.algorithm != tc.algorithm || bucket.kmsKeyID != exampleKMSKeyARN || result.KMSKeyID != exampleKMSKeyARN {
				t.Errorf("bucket = %+v, result = %+v", bucket, result)
			}
			if bucket.bucketKey || slices.Contains(fixture.s3.calls, "PutBucketEncryption") {
				t.Errorf("bucket key = %v, calls = %v, the encryption should be left exactly as it was", bucket.bucketKey, fixture.s3.calls)
			}
			// ours win over a tag of the same name, and a bucket yago only updates doesn't say yago made it
			_, isClaimed := bucket.tags["CreatedBy"]
			if bucket.tags["AddedByHand"] != "yes" || bucket.tags["Name"] != exampleStateBucketName || isClaimed {
				t.Errorf("tags = %v", bucket.tags)
			}
			if bucket.ownership != types.ObjectOwnershipBucketOwnerEnforced {
				t.Errorf("ownership = %q, update should switch ACLs off too", bucket.ownership)
			}
			if len(fixture.aws.keys) != 1 {
				t.Errorf("keys = %v, update must not make one for a bucket that has one", fixture.aws.keys)
			}
			if missingErr == nil || len(fixture.s3.buckets) != 1 {
				t.Errorf("missing bucket: err = %v, buckets = %d", missingErr, len(fixture.s3.buckets))
			}
		})
	}
}

func TestStateBucket_CreateProtectsTheStateWithAPolicyAndALifecycleRule_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A new state bucket gets ACLs switched off, SSE-C refused, a lifecycle rule for old versions and a bucket policy, from defaults the configuration can override, and says Yago made it",
		CurrentImpl:     "planLifecycle and planPolicy build the rule and the statements from the spec or the defaults, applyRules puts them, planTags adds CreatedBy to a bucket and a key made here",
		ExpectedOutcome: "The defaults give one rule and three deny statements on the bucket's own ARN, an override changes only what it names, a statement switched off is left out, and a bad number stops before anything is made",
		Rationale:       "State holds secrets and is the one thing we can't rebuild, so it shouldn't be deletable or readable in the clear by default, and a bucket made outside Terraform should say who made it",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: no bucket, once with the defaults and once with a shorter retention and version deletion allowed
	defaults, overridden := newStateBucketFixture(nil), newStateBucketFixture(nil)
	override := StateBucketSpec{
		Name:      examplePinnedName,
		Lifecycle: &StateBucketLifecycle{NoncurrentVersionRetentionDays: 2, NoncurrentVersionsRetained: 3, AbortIncompleteUploadDays: 4},
		Policy:    &StateBucketPolicy{DenyInsecureTransport: true, DenyStateDeletion: true},
	}

	// When: both are created, and one more with a retention of no days at all
	_, err := defaults.bucket(StateBucketSpec{Properties: exampleStateBucketProperties()}, false).Create()
	planned := overridden.bucket(override, false)
	_, overrideErr := planned.Create()
	for name, lifecycle := range map[string]StateBucketLifecycle{
		"no retention days":          {NoncurrentVersionRetentionDays: 0, NoncurrentVersionsRetained: 10, AbortIncompleteUploadDays: 7},
		"no days to abort an upload": {NoncurrentVersionRetentionDays: 90, NoncurrentVersionsRetained: 10, AbortIncompleteUploadDays: 0},
		"fewer than no versions":     {NoncurrentVersionRetentionDays: 90, NoncurrentVersionsRetained: -1, AbortIncompleteUploadDays: 7},
		"more versions than S3 has":  {NoncurrentVersionRetentionDays: 90, NoncurrentVersionsRetained: 101, AbortIncompleteUploadDays: 7},
	} {
		invalid := newStateBucketFixture(nil)
		if _, invalidErr := invalid.bucket(StateBucketSpec{Name: examplePinnedName, Lifecycle: &lifecycle}, false).Create(); invalidErr == nil || len(invalid.mutations()) > 0 {
			t.Errorf("%s: err = %v, mutating calls = %v", name, invalidErr, invalid.mutations())
		}
	}
	// and one where the last write fails, and one whose properties make more tags than AWS allows
	halfMade := newStateBucketFixture(nil)
	halfMade.s3.errs["PutBucketPolicy"] = stderrors.New("example failure")
	_, halfMadeErr := halfMade.bucket(StateBucketSpec{Name: examplePinnedName}, false).Create()
	crowded := newStateBucketFixture(nil)
	properties := exampleStateBucketProperties()
	customTags := map[string]interface{}{}
	for i := range 60 {
		customTags[fmt.Sprintf("example-%d", i)] = "value"
	}
	properties["custom_tags"] = customTags
	_, crowdedErr := crowded.bucket(StateBucketSpec{Properties: properties}, false).Create()

	// Then: the default bucket carries the rule and all three statements
	if err != nil || overrideErr != nil {
		t.Fatalf("create = %v, with overrides = %v", err, overrideErr)
	}
	bucket := defaults.s3.buckets[exampleStateBucketName]
	if bucket.ownership != types.ObjectOwnershipBucketOwnerEnforced || !bucket.blocksSSEC || bucket.tags["CreatedBy"] != "Yago" {
		t.Errorf("bucket = %+v", bucket)
	}
	if len(bucket.lifecycle) != 1 {
		t.Fatalf("lifecycle = %+v, want one rule", bucket.lifecycle)
	}
	rule := bucket.lifecycle[0]
	// S3 refuses a rule without a filter, an empty prefix is how a rule says every object
	if rule.Filter == nil || rule.Filter.Prefix == nil || *rule.Filter.Prefix != "" {
		t.Errorf("filter = %+v, want an empty prefix", rule.Filter)
	}
	if aws.ToString(rule.ID) != "expire-noncurrent-state" || rule.Status != types.ExpirationStatusEnabled ||
		!aws.ToBool(rule.Expiration.ExpiredObjectDeleteMarker) || aws.ToInt32(rule.NoncurrentVersionExpiration.NoncurrentDays) != 90 ||
		aws.ToInt32(rule.NoncurrentVersionExpiration.NewerNoncurrentVersions) != 10 || aws.ToInt32(rule.AbortIncompleteMultipartUpload.DaysAfterInitiation) != 7 {
		t.Errorf("rule = %+v", rule)
	}
	arn := "arn:aws:s3:::" + exampleStateBucketName
	wantPolicy := `{"Statement":[` +
		`{"Action":"s3:*","Condition":{"Bool":{"aws:SecureTransport":"false"}},"Effect":"Deny","Principal":"*","Resource":["` + arn + `","` + arn + `/*"],"Sid":"DenyInsecureTransport"},` +
		`{"Action":"s3:DeleteObject","Effect":"Deny","Principal":"*","Resource":["` + arn + `/*.tfstate"],"Sid":"DenyStateDeletion"},` +
		`{"Action":"s3:DeleteObjectVersion","Effect":"Deny","Principal":"*","Resource":["` + arn + `/*"],"Sid":"DenyVersionDeletion"}` +
		`],"Version":"2012-10-17"}`
	if bucket.policy != wantPolicy {
		t.Errorf("policy = %s\nwant     %s", bucket.policy, wantPolicy)
	}

	// and the overrides change what they name, with the plan saying which statement is left out
	bucket = overridden.s3.buckets[examplePinnedName]
	expiry := bucket.lifecycle[0].NoncurrentVersionExpiration
	aborted := bucket.lifecycle[0].AbortIncompleteMultipartUpload
	if aws.ToInt32(expiry.NoncurrentDays) != 2 || aws.ToInt32(expiry.NewerNoncurrentVersions) != 3 || aws.ToInt32(aborted.DaysAfterInitiation) != 4 {
		t.Errorf("expiry = %+v, aborted = %+v, want 2 days, 3 kept and 4 days", expiry, aborted)
	}
	// the plan gives the numbers that are written, not the defaults
	if !strings.Contains(planned.changes(), "expire 2 days after they are replaced with the newest 3 of them always kept") ||
		!strings.Contains(planned.changes(), "aborted after 4 days") {
		t.Errorf("plan = %q", planned.changes())
	}
	// and no fixed number kept is left out of the rule and of the plan
	ageOnly := newStateBucketFixture(nil)
	byAge := ageOnly.bucket(StateBucketSpec{Name: examplePinnedName, Lifecycle: &StateBucketLifecycle{NoncurrentVersionRetentionDays: 1, AbortIncompleteUploadDays: 1}}, false)
	if _, err := byAge.Create(); err != nil {
		t.Fatal(err)
	}
	if kept := ageOnly.s3.buckets[examplePinnedName].lifecycle[0].NoncurrentVersionExpiration.NewerNoncurrentVersions; kept != nil ||
		!strings.Contains(byAge.changes(), "expire 1 days after they are replaced, delete markers") {
		t.Errorf("kept = %v, plan = %q", kept, byAge.changes())
	}
	if strings.Contains(bucket.policy, "DenyVersionDeletion") || !strings.Contains(bucket.policy, "DenyStateDeletion") {
		t.Errorf("policy = %s", bucket.policy)
	}
	if !strings.Contains(planned.changes(), "DenyVersionDeletion is left out, the configuration switches it off") {
		t.Errorf("plan = %q", planned.changes())
	}
	// a bucket without project properties still says who made it
	if !maps.Equal(bucket.tags, map[string]string{"CreatedBy": "Yago"}) {
		t.Errorf("tags = %v", bucket.tags)
	}
	// update never adds CreatedBy, so a create that stops after the tags are on has left it already
	if halfMadeErr == nil || halfMade.s3.buckets[examplePinnedName].tags["CreatedBy"] != "Yago" {
		t.Errorf("half made: err = %v, tags = %v", halfMadeErr, halfMade.s3.buckets[examplePinnedName].tags)
	}
	// and CreatedBy takes one of the 50 places, it isn't the tag that tips a full set over
	if crowdedErr != nil {
		t.Fatal(crowdedErr)
	}
	for what, tags := range map[string]map[string]string{"bucket": crowded.s3.buckets[exampleStateBucketName].tags, "key": crowded.aws.keys["key-1"].tags} {
		if len(tags) != maxTags || tags["CreatedBy"] != "Yago" {
			t.Errorf("%s has %d tags, CreatedBy = %q", what, len(tags), tags["CreatedBy"])
		}
	}
	// and an update of that bucket still fits, CreatedBy keeps its place
	if _, err := crowded.bucket(StateBucketSpec{Properties: properties}, false).Update(); err != nil {
		t.Errorf("update after a create with a full tag set: %v", err)
	}
	if tags := crowded.s3.buckets[exampleStateBucketName].tags; len(tags) != maxTags || tags["CreatedBy"] != "Yago" {
		t.Errorf("after update the bucket has %d tags, CreatedBy = %q", len(tags), tags["CreatedBy"])
	}

	// Given: a bucket that exists, and S3 refusing to switch its ACLs off
	refused := newStateBucketFixture(map[string]*fakeStateBucket{exampleStateBucketName: {}})
	refused.s3.errs["PutBucketOwnershipControls"] = &smithy.GenericAPIError{Code: "InvalidBucketAclWithObjectOwnership"}

	// When: update runs
	_, refusedErr := refused.bucket(StateBucketSpec{Properties: exampleStateBucketProperties()}, false).Update()

	// Then: it stopped there, with only the tags written, and says so
	if refusedErr == nil || !strings.Contains(refusedErr.Error(), "its tags were already updated and nothing else was") {
		t.Errorf("err = %v", refusedErr)
	}
	if mutations := refused.mutations(); !slices.Equal(mutations, []string{"PutBucketTagging", "PutBucketOwnershipControls"}) {
		t.Errorf("mutating calls = %v, want the tags and then the refused ACL switch", mutations)
	}

	// Given: the same refusal without project properties, so no tags to write, and a failure that isn't about the ACL
	untagged := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {}})
	untagged.s3.errs["PutBucketOwnershipControls"] = &smithy.GenericAPIError{Code: "InvalidBucketAclWithObjectOwnership"}
	denied := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {}})
	denied.s3.errs["PutBucketOwnershipControls"] = &smithy.GenericAPIError{Code: "AccessDenied"}

	// When: update runs on both
	_, untaggedErr := untagged.bucket(StateBucketSpec{Name: examplePinnedName}, false).Update()
	_, deniedErr := denied.bucket(StateBucketSpec{Name: examplePinnedName}, false).Update()

	// Then: the first says nothing was changed, which is true, and the second doesn't send us after an ACL
	if untaggedErr == nil || !strings.Contains(untaggedErr.Error(), "nothing was changed") || !slices.Equal(untagged.mutations(), []string{"PutBucketOwnershipControls"}) {
		t.Errorf("no tags to write: err = %v, mutating calls = %v", untaggedErr, untagged.mutations())
	}
	if deniedErr == nil || strings.Contains(deniedErr.Error(), "its ACL grants") {
		t.Errorf("access denied: err = %v", deniedErr)
	}

	// Given: no bucket, in the two other partitions
	for region, wantARN := range map[string]string{"us-gov-west-1": "arn:aws-us-gov:s3:::" + examplePinnedName, "cn-north-1": "arn:aws-cn:s3:::" + examplePinnedName} {
		elsewhere := newStateBucketFixture(nil)

		// When: a bucket is created there
		_, err := elsewhere.bucket(StateBucketSpec{Name: examplePinnedName, Region: region, Encryption: EncryptionSse}, false).Create()

		// Then: its policy names the bucket in that partition
		if err != nil || !strings.Contains(elsewhere.s3.buckets[examplePinnedName].policy, `"`+wantARN+`/*.tfstate"`) {
			t.Errorf("%s: err = %v, policy = %s", region, err, elsewhere.s3.buckets[examplePinnedName].policy)
		}
	}
}

func TestStateBucket_UpdateKeepsTheRulesAndStatementsABucketHas_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Update puts our lifecycle rule and policy statements on an existing bucket without dropping the ones it already has, and doesn't claim Yago made it",
		CurrentImpl:     "settings reads the rules and the policy, planLifecycle swaps the rule with our ID, planPolicy swaps statements by Sid and carries the rest of the document over, planTags adds CreatedBy on create only",
		ExpectedOutcome: "A hand written rule and statement survive next to ours, an older copy of ours is replaced and not doubled, a statement switched off stays on a bucket that has it, and the plan counts what stays",
		Rationale:       "Both puts replace the whole set, so an update that only sent ours would quietly remove somebody's replication rule or cross account grant",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a bucket with a rule and a statement of its own, an older copy of our rule and of two of our statements
	fixture := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {
		lifecycle: []types.LifecycleRule{
			{ID: aws.String("added-by-hand"), Status: types.ExpirationStatusEnabled},
			{ID: aws.String("expire-noncurrent-state"), Status: types.ExpirationStatusDisabled},
		},
		minimumSize: types.TransitionDefaultMinimumObjectSizeVariesByStorageClass,
		tags:        map[string]string{"AddedByHand": "yes"},
		policy: `{"Version":"2012-10-17","Id":"example","Statement":[` +
			`{"Sid":"AddedByHand","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::123456789012:root"},"Action":"s3:GetObject","Resource":"*"},` +
			`{"Sid":"DenyStateDeletion","Effect":"Allow"},{"Sid":"DenyVersionDeletion","Effect":"Allow"}]}`,
	}})
	spec := StateBucketSpec{Name: examplePinnedName, Policy: &StateBucketPolicy{DenyInsecureTransport: true, DenyStateDeletion: true}}

	// When: update runs with version deletion switched off
	planned := fixture.bucket(spec, false)
	_, err := planned.Update()

	// Then: the bucket's own rule and statement are still there, ours are in once, and the one switched off was not taken off
	if err != nil {
		t.Fatal(err)
	}
	bucket := fixture.s3.buckets[examplePinnedName]
	ids := []string{}
	for _, rule := range bucket.lifecycle {
		ids = append(ids, aws.ToString(rule.ID)+" "+string(rule.Status))
	}
	if !slices.Equal(ids, []string{"expire-noncurrent-state Enabled", "added-by-hand Enabled"}) {
		t.Errorf("rules = %v", ids)
	}
	for sid, want := range map[string]int{"AddedByHand": 1, "DenyInsecureTransport": 1, `"Effect":"Deny","Principal":"*","Resource":["arn:aws:s3:::` + examplePinnedName + `/*.tfstate"],"Sid":"DenyStateDeletion"`: 1,
		`"Effect":"Allow","Sid":"DenyVersionDeletion"`: 1, `"Id":"example"`: 1} {
		if got := strings.Count(bucket.policy, sid); got != want {
			t.Errorf("policy mentions %s %d times, want %d, policy = %s", sid, got, want, bucket.policy)
		}
	}
	if bucket.minimumSize != types.TransitionDefaultMinimumObjectSizeVariesByStorageClass || bucket.ownership != types.ObjectOwnershipBucketOwnerEnforced {
		t.Errorf("minimum object size = %q, ownership = %q", bucket.minimumSize, bucket.ownership)
	}
	// without project properties the tags are not written at all, a put of none would wipe the ones it has
	if !maps.Equal(bucket.tags, map[string]string{"AddedByHand": "yes"}) || slices.Contains(fixture.mutations(), "PutBucketTagging") {
		t.Errorf("tags = %v, mutating calls = %v, the tags should have been left alone", bucket.tags, fixture.mutations())
	}
	// and the plan says what it replaces, what it leaves on the bucket, and counts only what isn't ours
	for _, want := range []string{
		"aborted after 7 days, replacing the rule of that name it has, which is not written the same, the other lifecycle rules it has stay as they are, 1 of them",
		"DenyInsecureTransport, every request that doesn't come over TLS is refused\n",
		"ends in .tfstate, replacing the statement of that name it has, which is not written the same",
		"DenyVersionDeletion is switched off in the configuration, the one the bucket has stays as it is",
		"the other statements its policy has stay as they are, 1 of them",
		"leave its tags as they are",
	} {
		if !strings.Contains(planned.changes(), want) {
			t.Errorf("plan = %q, want it to mention %q", planned.changes(), want)
		}
	}

	// Given: a policy whose one statement is not in a list, as S3 accepts it
	single := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {policy: `{"Statement":{"Sid":"OnlyOne","Effect":"Allow"}}`}})

	// When: update runs with every statement switched off, then twice with the defaults
	off := single.bucket(StateBucketSpec{Name: examplePinnedName, Policy: &StateBucketPolicy{}}, false)
	_, offErr := off.Update()
	policyBefore := single.s3.buckets[examplePinnedName].policy
	_, firstErr := single.bucket(StateBucketSpec{Name: examplePinnedName}, false).Update()
	// S3 hands a list of one back as the bare value, seen on a real bucket, so the second run reads the policy that way
	arn := "arn:aws:s3:::" + examplePinnedName
	single.s3.buckets[examplePinnedName].policy = strings.NewReplacer(
		`["`+arn+`/*.tfstate"]`, `"`+arn+`/*.tfstate"`, `["`+arn+`/*"]`, `"`+arn+`/*"`,
	).Replace(single.s3.buckets[examplePinnedName].policy)
	again := single.bucket(StateBucketSpec{Name: examplePinnedName}, false)
	_, againErr := again.Update()

	// Then: switched off, the policy is not written at all, and after that the one statement is kept next to ours,
	// and a second run that finds our own statements and rule there doesn't claim to replace anything
	if offErr != nil || firstErr != nil || againErr != nil {
		t.Fatal(offErr, firstErr, againErr)
	}
	if !strings.Contains(off.changes(), "leave its bucket policy alone") || !strings.Contains(policyBefore, `"Statement":{`) {
		t.Errorf("all off: plan = %q, policy = %s", off.changes(), policyBefore)
	}
	policy := single.s3.buckets[examplePinnedName].policy
	for _, sid := range []string{"OnlyOne", "DenyInsecureTransport", "DenyStateDeletion", "DenyVersionDeletion"} {
		if strings.Count(policy, `"Sid":"`+sid+`"`) != 1 {
			t.Errorf("policy = %s, want %s once", policy, sid)
		}
	}
	if strings.Contains(again.changes(), "replacing") || strings.Contains(again.changes(), "left out") {
		t.Errorf("second run plan = %q, nothing is replaced or left out when ours are already there", again.changes())
	}
}

func TestStateBucket_UpdateRefusesToChangeTheKeyUnderExistingState_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Update refuses a configuration that pins another KMS key than the bucket has, or says sse for a bucket on a KMS key, or would leave it with more tags than S3 allows",
		CurrentImpl:     "planEncryption compares the pinned key and the encryption with what GetBucketEncryption returns, plan counts the merged tags",
		ExpectedOutcome: "An error naming what the bucket has, and nothing changed, for a real run and for a dry run alike",
		Rationale:       "New state written under another or a weaker key breaks every reader that has no rights on it, and nobody asked for that by running update",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// the properties make 10 tags, so 41 more is one too many, the three CloudFormation put there are not counted by S3
	crowded := map[string]string{"aws:cloudformation:stack-name": "example", "aws:cloudformation:stack-id": "example", "aws:cloudformation:logical-id": "Bucket"}
	for i := 0; i < 41; i++ {
		crowded[fmt.Sprintf("AddedByHand%d", i)] = "yes"
	}
	cases := []struct {
		name string
		spec StateBucketSpec
		tags map[string]string
		want string
	}{
		{name: "another key pinned", spec: StateBucketSpec{Name: examplePinnedName, KMSKeyID: otherKMSKeyARN}, want: exampleKMSKeyARN},
		{name: "sse for a bucket on KMS", spec: StateBucketSpec{Name: examplePinnedName, Encryption: EncryptionSse}, want: exampleKMSKeyARN},
		{name: "too many tags", spec: StateBucketSpec{Name: examplePinnedName, Properties: exampleStateBucketProperties()}, tags: crowded, want: "51 tags"},
	}
	for _, tc := range cases {
		for _, isDryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, dry run %v", tc.name, isDryRun), func(t *testing.T) {
				// Given: a bucket that already holds state under its own key
				fixture := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {
					algorithm: types.ServerSideEncryptionAwsKms, kmsKeyID: exampleKMSKeyARN, tags: tc.tags, versions: 3,
				}})
				fixture.aws.addKey(keyName(otherKMSKeyARN))

				// When: update runs with a configuration that disagrees
				_, err := fixture.bucket(tc.spec, isDryRun).Update()

				// Then: it says what the bucket has and leaves it alone
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("err = %v, want it to mention %q", err, tc.want)
				}
				if mutations := fixture.mutations(); len(mutations) > 0 {
					t.Errorf("mutating calls = %v", mutations)
				}
			})
		}
	}

	// Given: the same bucket with one hand added tag fewer, so 50 that count and the three aws: ones on top
	delete(crowded, "AddedByHand0")
	fixture := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {algorithm: types.ServerSideEncryptionAwsKms, kmsKeyID: exampleKMSKeyARN, tags: crowded}})

	// When: update runs
	_, err := fixture.bucket(StateBucketSpec{Name: examplePinnedName, Properties: exampleStateBucketProperties()}, false).Update()

	// Then: it goes through, and the aws: tags are sent back with the rest, S3 refuses a put that leaves them out
	tags := fixture.s3.buckets[examplePinnedName].tags
	if err != nil || len(tags) != 53 || tags["aws:cloudformation:logical-id"] != "Bucket" {
		t.Errorf("err = %v, %d tags = %v", err, len(tags), tags)
	}
}

func TestStateBucket_UpdateFinishesAHalfMadeBucket_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Update gives a bucket that is still on S3's own key the key create would have made, and its plan says that the encryption changes",
		CurrentImpl:     "planEncryption only keeps what the bucket has when that is a customer key, otherwise it plans like create and words the change into the plan",
		ExpectedOutcome: "One new rotating key, set as the bucket's default encryption and handed back to pin, and the change of encryption, the new key and the missing tags all in the plan, for a dry run as well as a real one",
		Rationale:       "A create that failed after CreateBucket can't be run again, so update has to be able to finish the job, but nothing it does may go unseen and unagreed",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	for _, isDryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry run %v", isDryRun), func(t *testing.T) {
			// Given: a bucket that exists with nothing set, as a create that died halfway leaves it, and no project properties
			fixture := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {}})
			updated := fixture.bucket(StateBucketSpec{Name: examplePinnedName}, isDryRun)

			// When: update runs
			result, err := updated.Update()

			// Then: the plan of both runs says it all, and only the real one gives the bucket a key of its own
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"update state bucket " + examplePinnedName + " in account 123456789012",
				"CHANGE ITS ENCRYPTION, which is AES256 without a customer key today",
				`create a KMS key described as "Encrypts Terraform state, made for ` + examplePinnedName + `" with rotation on`,
				"turn on its S3 bucket key",
				"if any of that fails schedule the new key for deletion",
				"no project properties",
			} {
				if !strings.Contains(updated.changes(), want) {
					t.Errorf("plan = %q, want it to mention %q", updated.changes(), want)
				}
			}
			key, bucket := fixture.aws.keys["key-1"], fixture.s3.buckets[examplePinnedName]
			if isDryRun {
				if key != nil || bucket.kmsKeyID != "" {
					t.Errorf("key = %+v, bucket = %+v, a dry run should change nothing", key, bucket)
				}
				return
			}
			if key == nil || !key.rotation || bucket.algorithm != types.ServerSideEncryptionAwsKms || bucket.kmsKeyID != key.arn || result.KMSKeyID != key.arn {
				t.Errorf("key = %+v, bucket = %+v, result = %+v", key, bucket, result)
			}
		})
	}
}

func TestStateBucket_ThePlanSaysWhatHappensToTheEncryption_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "The plan words the encryption exactly as apply then treats it, left alone, moved to S3's own key, or given a new key with its tags",
		CurrentImpl:     "planEncryption sets keepsEncryption whenever nothing would change, and plan lists the new key's tags next to the bucket's",
		ExpectedOutcome: "A plan that says leave makes no PutBucketEncryption call, one that says CHANGE ends on what it named, and a new key's tags are in the plan before the key exists",
		Rationale:       "We agree to the plan, so a plan that says leave and then writes, or tags a key with something we never saw, is invisible functionality",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := []struct {
		name          string
		bucket        *fakeStateBucket
		spec          StateBucketSpec
		wantPlan      []string
		wantPut       bool
		wantAlgorithm types.ServerSideEncryption
	}{
		{name: "already on S3's own key and the configuration says sse", bucket: &fakeStateBucket{},
			spec:     StateBucketSpec{Name: examplePinnedName, Encryption: EncryptionSse},
			wantPlan: []string{"leave its encryption as it is, AES256 with S3's own key"}, wantAlgorithm: types.ServerSideEncryptionAes256},
		{name: "on the AWS managed key and the configuration says sse", bucket: &fakeStateBucket{algorithm: types.ServerSideEncryptionAwsKms},
			spec:     StateBucketSpec{Name: examplePinnedName, Encryption: EncryptionSse},
			wantPlan: []string{"CHANGE ITS ENCRYPTION, which is aws:kms without a customer key today", "encrypt it with S3's own key, AES256"},
			wantPut:  true, wantAlgorithm: types.ServerSideEncryptionAes256},
		{name: "no bucket yet, a key is made and tagged", spec: StateBucketSpec{Properties: exampleStateBucketProperties()},
			wantPlan: []string{"tag the new key with\n", "Foo:Environment:ResourceType = :AWS::KMS::Key"},
			wantPut:  true, wantAlgorithm: types.ServerSideEncryptionAwsKms},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: that bucket, or none
			buckets := map[string]*fakeStateBucket{}
			if tc.bucket != nil {
				buckets[examplePinnedName] = tc.bucket
			}
			fixture := newStateBucketFixture(buckets)
			planned := fixture.bucket(tc.spec, false)

			// When: it is created or updated
			run := planned.Update
			if tc.bucket == nil {
				run = planned.Create
			}
			result, err := run()

			// Then: the plan said it, and apply did that and no more
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.wantPlan {
				if !strings.Contains(planned.changes(), want) {
					t.Errorf("plan = %q, want it to mention %q", planned.changes(), want)
				}
			}
			if put := slices.Contains(fixture.s3.calls, "PutBucketEncryption"); put != tc.wantPut {
				t.Errorf("PutBucketEncryption called = %v, want %v", put, tc.wantPut)
			}
			if got := fixture.s3.buckets[result.Name].algorithm; got != tc.wantAlgorithm {
				t.Errorf("algorithm = %s, want %s", got, tc.wantAlgorithm)
			}
		})
	}
}

func TestStateBucket_KeyNothingUsesIsNotLeftBehind_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A key made in this run is scheduled for deletion when the run can't finish putting it to use",
		CurrentImpl:     "discardNewKey calls ScheduleKeyDeletion when EnableKeyRotation or the PutBucketEncryption after it fails",
		ExpectedOutcome: "An error naming the key and telling us to run update, the key pending deletion or the error saying it couldn't be, and the bucket's encryption untouched",
		Rationale:       "The next run would make another key, so the choice is between removing this one now and paying for orphans nobody knows about",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := []struct {
		name        string
		breakIt     func(*stateBucketFixture)
		wantPending bool
		wantError   string
	}{
		{name: "rotation can't be turned on", wantPending: true, wantError: "so it was scheduled for deletion",
			breakIt: func(f *stateBucketFixture) { f.aws.enableRotationErr = stderrors.New("rotation refused") }},
		{name: "the key can't be set on the bucket", wantPending: true, wantError: "so it was scheduled for deletion",
			breakIt: func(f *stateBucketFixture) { f.s3.errs["PutBucketEncryption"] = stderrors.New("encryption refused") }},
		{name: "and the key can't be scheduled for deletion either", wantError: "could not be scheduled for deletion",
			breakIt: func(f *stateBucketFixture) {
				f.aws.enableRotationErr = stderrors.New("rotation refused")
				f.aws.scheduleDeletionErr = stderrors.New("deletion refused")
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: that step is going to fail
			fixture := newStateBucketFixture(nil)
			tc.breakIt(fixture)

			// When: the bucket is created
			_, err := fixture.bucket(StateBucketSpec{Name: examplePinnedName}, false).Create()

			// Then: the error names the key, says what became of it, and says the bucket needs an update
			key := fixture.aws.keys["key-1"]
			if key == nil || key.pendingDeletion != tc.wantPending {
				t.Fatalf("key = %+v, want pending deletion %v", key, tc.wantPending)
			}
			if err == nil || !strings.Contains(err.Error(), "run update") || !strings.Contains(err.Error(), tc.wantError) || !strings.Contains(err.Error(), key.arn) {
				t.Errorf("err = %v", err)
			}
			if bucket := fixture.s3.buckets[examplePinnedName]; bucket == nil || bucket.kmsKeyID != "" {
				t.Errorf("bucket = %+v, want it there with its encryption untouched", bucket)
			}
		})
	}
}

func TestStateBucket_AFailedReadStopsBeforeTheNextWrite_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Update and delete stop without changing anything when a read they depend on fails",
		CurrentImpl:     "Update reads the encryption, the tags, the lifecycle rules and the policy in settings before plan and apply, Delete lists the versions in EnsureDeletable before DeleteBucket",
		ExpectedOutcome: "An error and no mutating call",
		Rationale:       "A read that failed says nothing about what is there, tags, rules or a policy replaced or a bucket deleted on that basis can't be put back",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	update := func(b *plannedBucket) error { _, err := b.Update(); return err }
	cases := map[string]func(*plannedBucket) error{
		"GetBucketEncryption":             update,
		"GetBucketTagging":                update,
		"GetBucketLifecycleConfiguration": update,
		"GetBucketPolicy":                 update,
		"ListObjectVersions":              func(b *plannedBucket) error { return b.Delete() },
	}
	for call, run := range cases {
		t.Run(call, func(t *testing.T) {
			// Given: a bucket with tags, and that read failing
			fixture := newStateBucketFixture(map[string]*fakeStateBucket{exampleStateBucketName: {tags: map[string]string{"AddedByHand": "yes"}}})
			fixture.s3.errs[call] = &smithy.GenericAPIError{Code: "AccessDenied"}

			// When: the command runs
			err := run(fixture.bucket(StateBucketSpec{Properties: exampleStateBucketProperties()}, false))

			// Then: it stops with the bucket as it was
			if err == nil {
				t.Error("expected an error")
			}
			if mutations := fixture.mutations(); len(mutations) > 0 {
				t.Errorf("mutating calls = %v", mutations)
			}
		})
	}
}

func TestStateBucket_DeleteRefusesABucketThatHoldsAnything_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Refuse to delete a bucket that holds an object, an old version or a delete marker, one that doesn't exist, or one in another account",
		CurrentImpl:     "EnsureDeletable lists one object version, and Delete runs it again before DeleteBucket",
		ExpectedOutcome: "An error from both and the bucket still there",
		Rationale:       "A deleted state file is still recoverable from its versions until the bucket goes, so they count as state",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := map[string]map[string]*fakeStateBucket{
		"current object":       {examplePinnedName: {versions: 1}},
		"delete marker only":   {examplePinnedName: {markers: 1}},
		"no such bucket":       {},
		"another account's":    {examplePinnedName: {owner: "210987654321"}},
		"another bucket empty": {examplePinnedName: {versions: 1}, "example-dev-statefile-euc1": {}},
	}
	for name, buckets := range cases {
		t.Run(name, func(t *testing.T) {
			// Given: a bucket that still holds something, or isn't ours to delete
			fixture := newStateBucketFixture(buckets)
			bucket := fixture.bucket(StateBucketSpec{Name: examplePinnedName}, false)

			// When: it is checked, and deleted anyway
			checkErr, deleteErr := bucket.EnsureDeletable(), bucket.Delete()

			// Then: both refuse and nothing was changed
			if checkErr == nil || deleteErr == nil {
				t.Errorf("check = %v, delete = %v", checkErr, deleteErr)
			}
			if mutations := fixture.mutations(); len(mutations) > 0 {
				t.Errorf("mutating calls = %v", mutations)
			}
		})
	}
}

func TestStateBucket_DeleteRemovesAnEmptyBucketAndLeavesItsKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Delete removes an empty bucket and nothing else, and checking whether it can be deleted removes nothing",
		CurrentImpl:     "Delete calls DeleteBucket after EnsureDeletable and has no KMS call on its path, EnsureDeletable only reads",
		ExpectedOutcome: "The bucket is still there after the check and gone after the delete, and the key is untouched",
		Rationale:       "Another bucket may encrypt with the same key, and a key can't be brought back once its deletion window has passed",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: an empty bucket on a KMS key, in a configuration whose encryption setting is nonsense
	fixture := newStateBucketFixture(map[string]*fakeStateBucket{examplePinnedName: {
		algorithm: types.ServerSideEncryptionAwsKms, kmsKeyID: exampleKMSKeyARN,
	}})
	bucket := fixture.bucket(StateBucketSpec{Name: examplePinnedName, Encryption: "aes"}, false)

	// When: it is checked, which is all a dry run of the command does, and then deleted
	checkErr := bucket.EnsureDeletable()
	_, thereAfterCheck := fixture.s3.buckets[examplePinnedName]
	deleteErr := bucket.Delete()

	// Then: only the delete removes it, the key is left alone, and a setting delete has no use for didn't stop it
	if checkErr != nil || deleteErr != nil {
		t.Fatalf("check = %v, delete = %v", checkErr, deleteErr)
	}
	if _, thereAfterDelete := fixture.s3.buckets[examplePinnedName]; !thereAfterCheck || thereAfterDelete {
		t.Errorf("bucket there after the check = %v, after the delete = %v", thereAfterCheck, thereAfterDelete)
	}
	if mutations := fixture.aws.mutations(); len(mutations) > 0 || fixture.aws.keys[exampleKMSKeyID].pendingDeletion {
		t.Errorf("KMS mutations = %v, key = %+v", mutations, fixture.aws.keys[exampleKMSKeyID])
	}
}

func TestStateBucket_DryRunChangesNothing_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Planning a create or an update only reads, and the plan says which bucket, account, key and tags it is about",
		CurrentImpl:     "PlanCreate and PlanUpdate take every decision from reads alone and word each one into StateBucketPlan.Changes",
		ExpectedOutcome: "No mutating call in S3 or KMS, and the plan names the bucket a real run would make, its account, the key it would share and the bucket it shares it with",
		Rationale:       "The plan is what we are shown and asked to agree to, and all a dry run does, so it has to be complete and free of side effects",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: one bucket that exists with nothing set, a sibling on a KMS key, and no bucket for the name create would use
	fixture := newStateBucketFixture(map[string]*fakeStateBucket{
		examplePinnedName:  {},
		exampleSiblingName: {algorithm: types.ServerSideEncryptionAwsKms, kmsKeyID: exampleKMSKeyARN},
	})
	created := fixture.bucket(StateBucketSpec{Properties: exampleStateBucketProperties(), VersioningSources: []string{"v2", "blue"}}, true)

	// When: update and create are only planned
	_, updateErr := fixture.bucket(StateBucketSpec{Name: examplePinnedName}, true).Update()
	_, createErr := created.Create()

	// Then: nothing was changed, and the plan for create says all of what a real run would do
	if updateErr != nil || createErr != nil {
		t.Fatalf("update = %v, create = %v", updateErr, createErr)
	}
	if mutations := fixture.mutations(); len(mutations) > 0 {
		t.Errorf("mutating calls = %v", mutations)
	}
	for _, want := range []string{
		"create state bucket foo-m-management-inception-state-euw1-272655d0e52f307fbbca09987 in eu-west-1, in account 123456789012",
		"turn on versioning, block all public access and switch ACLs off",
		"encrypt it with KMS key " + exampleKMSKeyARN + ", which state bucket " + exampleSiblingName + " already uses",
		"refuse uploads that bring an encryption key of their own, SSE-C",
		"set lifecycle rule expire-noncurrent-state, old versions of a state expire 90 days after they are replaced with the newest 10 of them always kept",
		"unfinished uploads are aborted after 7 days",
		"DenyInsecureTransport, every request that doesn't come over TLS is refused",
		"DenyStateDeletion, nobody can delete an object whose key ends in .tfstate",
		"DenyVersionDeletion, nobody can delete a version of an object",
		"CreatedBy = Yago",
		"Foo:Environment:ResourceType = :AWS::S3::Bucket",
	} {
		if !strings.Contains(created.changes(), want) {
			t.Errorf("plan = %q, want it to mention %q", created.changes(), want)
		}
	}
}
