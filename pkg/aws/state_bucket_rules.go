package aws

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

const (
	stateLifecycleRuleID = "expire-noncurrent-state"
	// what a bucket and a key made here say about where they came from
	createdByTag   = "CreatedBy"
	createdByValue = "Yago"

	sidDenyInsecureTransport = "DenyInsecureTransport"
	sidDenyStateDeletion     = "DenyStateDeletion"
	sidDenyVersionDeletion   = "DenyVersionDeletion"
)

// how long old state is kept around
type StateBucketLifecycle struct {
	NoncurrentVersionRetentionDays int32 `yaml:"noncurrent_version_retention_days"`
	// 0 keeps no fixed number, old versions then go by age alone
	NoncurrentVersionsRetained int32 `yaml:"noncurrent_versions_retained"`
	AbortIncompleteUploadDays  int32 `yaml:"abort_incomplete_upload_days"`
}

// the statements of the bucket policy, each can be switched off
type StateBucketPolicy struct {
	DenyInsecureTransport bool `yaml:"deny_insecure_transport"`
	DenyStateDeletion     bool `yaml:"deny_state_deletion"`
	DenyVersionDeletion   bool `yaml:"deny_version_deletion"`
}

type stateStatement struct {
	sid string
	// what it means for us, in words for the plan
	does      string
	action    string
	resources []string
	condition map[string]interface{}
}

func DefaultStateBucketLifecycle() StateBucketLifecycle {
	return StateBucketLifecycle{NoncurrentVersionRetentionDays: 90, NoncurrentVersionsRetained: 10, AbortIncompleteUploadDays: 7}
}

func DefaultStateBucketPolicy() StateBucketPolicy {
	return StateBucketPolicy{DenyInsecureTransport: true, DenyStateDeletion: true, DenyVersionDeletion: true}
}

// PutBucketLifecycleConfiguration replaces every rule, so ours goes in next to the ones the bucket already has
func (b *StateBucket) planLifecycle(plan *StateBucketPlan, current *bucketSettings) error {
	lifecycle := DefaultStateBucketLifecycle()
	if b.spec.Lifecycle != nil {
		lifecycle = *b.spec.Lifecycle
	}
	if lifecycle.NoncurrentVersionRetentionDays < 1 || lifecycle.AbortIncompleteUploadDays < 1 ||
		lifecycle.NoncurrentVersionsRetained < 0 || lifecycle.NoncurrentVersionsRetained > 100 {
		return errors.New(errors.ErrParam,
			"lifecycle needs noncurrent_version_retention_days and abort_incomplete_upload_days of at least 1, and noncurrent_versions_retained from 0 to 100")
	}

	expiry := &types.NoncurrentVersionExpiration{NoncurrentDays: aws.Int32(lifecycle.NoncurrentVersionRetentionDays)}
	kept := ""
	if lifecycle.NoncurrentVersionsRetained > 0 {
		expiry.NewerNoncurrentVersions = aws.Int32(lifecycle.NoncurrentVersionsRetained)
		kept = fmt.Sprintf(" with the newest %d of them always kept", lifecycle.NoncurrentVersionsRetained)
	}
	plan.lifecycle = []types.LifecycleRule{{
		ID:                             aws.String(stateLifecycleRuleID),
		Status:                         types.ExpirationStatusEnabled,
		Filter:                         &types.LifecycleRuleFilter{Prefix: aws.String("")},
		Expiration:                     &types.LifecycleExpiration{ExpiredObjectDeleteMarker: aws.Bool(true)},
		NoncurrentVersionExpiration:    expiry,
		AbortIncompleteMultipartUpload: &types.AbortIncompleteMultipartUpload{DaysAfterInitiation: aws.Int32(lifecycle.AbortIncompleteUploadDays)},
	}}
	change := fmt.Sprintf("set lifecycle rule %s, old versions of a state expire %d days after they are replaced%s, "+
		"delete markers with nothing left behind them are removed and unfinished uploads are aborted after %d days",
		stateLifecycleRuleID, lifecycle.NoncurrentVersionRetentionDays, kept, lifecycle.AbortIncompleteUploadDays)

	if current != nil {
		others := 0
		for _, rule := range current.lifecycle {
			if aws.ToString(rule.ID) != stateLifecycleRuleID {
				plan.lifecycle = append(plan.lifecycle, rule)
				others++
			} else if !reflect.DeepEqual(rule, plan.lifecycle[0]) {
				change += ", replacing the rule of that name it has, which is not written the same"
			}
		}
		if others > 0 {
			change += fmt.Sprintf(", the other lifecycle rules it has stay as they are, %d of them", others)
		}
		plan.minimumObjectSize = current.minimumObjectSize
	}
	plan.Changes = append(plan.Changes, change)
	return nil
}

// PutBucketPolicy replaces the whole policy, so ours go in by Sid and every other statement is carried over, a
// statement that is switched off is only ever left out, never taken off a bucket that has it
func (b *StateBucket) planPolicy(plan *StateBucketPlan, current *bucketSettings) error {
	policy := DefaultStateBucketPolicy()
	if b.spec.Policy != nil {
		policy = *b.spec.Policy
	}
	arn := "arn:" + partition(b.spec.Region) + ":s3:::" + b.name

	statements := []stateStatement{
		{sidDenyInsecureTransport, "every request that doesn't come over TLS is refused", "s3:*",
			[]string{arn, arn + "/*"}, map[string]interface{}{"Bool": map[string]interface{}{"aws:SecureTransport": "false"}}},
		{sidDenyStateDeletion, "nobody can delete an object whose key ends in .tfstate", "s3:DeleteObject",
			[]string{arn + "/*.tfstate"}, nil},
		{sidDenyVersionDeletion, "nobody can delete a version of an object, the lifecycle rule still expires them", "s3:DeleteObjectVersion",
			[]string{arn + "/*"}, nil},
	}
	isOn := map[string]bool{
		sidDenyInsecureTransport: policy.DenyInsecureTransport,
		sidDenyStateDeletion:     policy.DenyStateDeletion,
		sidDenyVersionDeletion:   policy.DenyVersionDeletion,
	}

	// what the bucket has today, the statements that carry one of our Sids by that Sid, as JSON to compare, and the rest
	document := map[string]interface{}{"Version": "2012-10-17"}
	has := map[string]string{}
	kept, others := []interface{}{}, 0
	if current != nil && current.policy != nil {
		document = maps.Clone(current.policy)
		existing, isList := document["Statement"].([]interface{})
		if !isList && document["Statement"] != nil {
			existing = []interface{}{document["Statement"]}
		}
		for _, statement := range existing {
			fields, _ := statement.(map[string]interface{})
			sid, _ := fields["Sid"].(string)
			isOurs := slices.ContainsFunc(statements, func(ours stateStatement) bool { return ours.sid == sid })
			if !isOurs {
				kept = append(kept, statement)
				others++
				continue
			}
			content, err := json.Marshal(unwrapSingles(statement))
			if err != nil {
				return errors.Wrapf(errors.ErrFail, err, "failed to read statement %s of the bucket policy of %s", sid, b.name)
			}
			has[sid] = string(content)
			if !isOn[sid] {
				kept = append(kept, statement)
			}
		}
	}

	wanted := []interface{}{}
	lines := []string{}
	for _, statement := range statements {
		existing, isThere := has[statement.sid]
		switch {
		case !isOn[statement.sid] && isThere:
			lines = append(lines, statement.sid+" is switched off in the configuration, the one the bucket has stays as it is")
			continue
		case !isOn[statement.sid]:
			lines = append(lines, statement.sid+" is left out, the configuration switches it off")
			continue
		}
		fields := map[string]interface{}{
			"Sid": statement.sid, "Effect": "Deny", "Principal": "*", "Action": statement.action, "Resource": statement.resources,
		}
		if statement.condition != nil {
			fields["Condition"] = statement.condition
		}
		content, err := json.Marshal(unwrapSingles(fields))
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to write statement %s for %s", statement.sid, b.name)
		}
		line := statement.sid + ", " + statement.does
		if isThere && existing != string(content) {
			line += ", replacing the statement of that name it has, which is not written the same"
		}
		wanted = append(wanted, fields)
		lines = append(lines, line)
	}
	if len(wanted) == 0 {
		plan.Changes = append(plan.Changes, "leave its bucket policy alone, the configuration switches every statement off")
		return nil
	}

	document["Statement"] = append(kept, wanted...)
	content, err := json.Marshal(document)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to write the bucket policy of %s", b.name)
	}
	plan.policy = string(content)

	if others > 0 {
		lines = append(lines, fmt.Sprintf("the other statements its policy has stay as they are, %d of them", others))
	}
	plan.Changes = append(plan.Changes, "put these statements in its bucket policy\n"+strings.Join(lines, "\n"))
	return nil
}

// S3 hands a list of one back as the bare value, to a policy the two are the same, so they compare as the same here
func unwrapSingles(value interface{}) interface{} {
	switch typed := value.(type) {
	case []string:
		if len(typed) == 1 {
			return typed[0]
		}
	case []interface{}:
		if len(typed) == 1 {
			return unwrapSingles(typed[0])
		}
		unwrapped := make([]interface{}, len(typed))
		for i, item := range typed {
			unwrapped[i] = unwrapSingles(item)
		}
		return unwrapped
	case map[string]interface{}:
		unwrapped := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			unwrapped[key] = unwrapSingles(item)
		}
		return unwrapped
	}
	return value
}

func partition(region string) string {
	switch {
	case strings.HasPrefix(region, "us-gov-"):
		return "aws-us-gov"
	case strings.HasPrefix(region, "cn-"):
		return "aws-cn"
	}
	return "aws"
}

func (b *StateBucket) readRules(settings *bucketSettings) error {
	lifecycle, err := b.s3Client.GetBucketLifecycleConfiguration(b.ctx, &s3.GetBucketLifecycleConfigurationInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(),
	})
	if err != nil && !isAPIError(err, "NoSuchLifecycleConfiguration") {
		return errors.Wrapf(errors.ErrFail, err, "failed to read the lifecycle rules of %s", b.name)
	}
	if lifecycle != nil {
		settings.lifecycle, settings.minimumObjectSize = lifecycle.Rules, lifecycle.TransitionDefaultMinimumObjectSize
	}

	policy, err := b.s3Client.GetBucketPolicy(b.ctx, &s3.GetBucketPolicyInput{Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner()})
	if err != nil && !isAPIError(err, "NoSuchBucketPolicy") {
		return errors.Wrapf(errors.ErrFail, err, "failed to read the bucket policy of %s", b.name)
	}
	if policy != nil && aws.ToString(policy.Policy) != "" {
		if err := json.Unmarshal([]byte(aws.ToString(policy.Policy)), &settings.policy); err != nil {
			return errors.Wrapf(errors.ErrFail, err, "the bucket policy of %s can't be read as JSON", b.name)
		}
	}
	return nil
}

func (b *StateBucket) applyRules(plan *StateBucketPlan) error {
	if _, err := b.s3Client.PutBucketLifecycleConfiguration(b.ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(),
		LifecycleConfiguration:             &types.BucketLifecycleConfiguration{Rules: plan.lifecycle},
		TransitionDefaultMinimumObjectSize: plan.minimumObjectSize,
	}); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to set the lifecycle rules of %s", b.name)
	}

	if plan.policy == "" {
		return nil
	}
	if _, err := b.s3Client.PutBucketPolicy(b.ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(b.name), ExpectedBucketOwner: b.owner(), Policy: aws.String(plan.policy),
	}); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to set the bucket policy of %s", b.name)
	}
	return nil
}
