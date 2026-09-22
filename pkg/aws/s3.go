package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// S3Client is an interface for S3 operations.
// This allows for mock implementations in tests.
type S3Client interface {
	GetObjectVersion(bucket, key string) (string, error)
	GetLatestObjectVersion(bucket, key string) (string, error)
	ListObjectVersions(bucket, key string) ([]string, error)
	ObjectExists(bucket, key, version string) (bool, error)
}

// S3Manager manages AWS S3 operations for artifact version management.
type S3Manager struct {
	region  string
	profile string
	ctx     context.Context
	client  *s3.Client
}

// NewS3Manager creates a new S3 manager instance.
func NewS3Manager(region, profile string) (*S3Manager, error) {
	ctx := context.Background()

	logging.Debug("Creating S3 manager for region %s (profile: %s)", region, profile)

	// Load AWS config
	var cfg aws.Config
	var err error

	if profile != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithSharedConfigProfile(profile),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	}

	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to load AWS config")
	}

	client := s3.NewFromConfig(cfg)

	return &S3Manager{
		region:  region,
		profile: profile,
		ctx:     ctx,
		client:  client,
	}, nil
}

// GetObjectVersion gets the version ID for an S3 object.
// This resolves the current version of an object.
func (sm *S3Manager) GetObjectVersion(bucket, key string) (string, error) {
	logging.Debug("Querying S3 for object version: s3://%s/%s", bucket, key)

	// Get object metadata to retrieve version ID
	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	result, err := sm.client.HeadObject(sm.ctx, input)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get S3 object metadata: s3://%s/%s", bucket, key)
	}

	versionID := aws.ToString(result.VersionId)
	logging.Debug("Found version for s3://%s/%s -> %s", bucket, key, versionID)

	return versionID, nil
}

// GetLatestObjectVersion gets the latest version ID for an S3 object.
func (sm *S3Manager) GetLatestObjectVersion(bucket, key string) (string, error) {
	logging.Debug("Querying S3 for latest object version: s3://%s/%s", bucket, key)

	// For S3, "latest" is just the current object version
	return sm.GetObjectVersion(bucket, key)
}

// ListObjectVersions lists all version IDs for an S3 object.
func (sm *S3Manager) ListObjectVersions(bucket, key string) ([]string, error) {
	logging.Debug("Listing S3 object versions for: s3://%s/%s", bucket, key)

	input := &s3.ListObjectVersionsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String(key),
	}

	result, err := sm.client.ListObjectVersions(sm.ctx, input)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to list S3 object versions: s3://%s/%s", bucket, key)
	}

	var versions []string
	for _, version := range result.Versions {
		if aws.ToString(version.Key) == key {
			versions = append(versions, aws.ToString(version.VersionId))
		}
	}

	return versions, nil
}

// ObjectExists checks if an S3 object with the specified version exists.
func (sm *S3Manager) ObjectExists(bucket, key, version string) (bool, error) {
	logging.Debug("Checking if S3 object exists: s3://%s/%s (version: %s)", bucket, key, version)

	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	if version != "" {
		input.VersionId = aws.String(version)
	}

	_, err := sm.client.HeadObject(sm.ctx, input)
	if err != nil {
		return false, nil // Object doesn't exist
	}

	return true, nil
}

// ParseS3URL parses an S3 URL to extract bucket and key.
// S3 URLs are in format: s3://bucket-name/path/to/object
func ParseS3URL(s3URL string) (bucket, key string, err error) {
	// Example: s3://my-bucket/path/to/artifact.tar.gz

	if !strings.HasPrefix(s3URL, "s3://") {
		return "", "", errors.New(errors.ErrParam, fmt.Sprintf("invalid S3 URL format (must start with s3://): %s", s3URL))
	}

	// Remove s3:// prefix
	path := strings.TrimPrefix(s3URL, "s3://")

	// Split into bucket and key
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return "", "", errors.New(errors.ErrParam, fmt.Sprintf("invalid S3 URL format (missing key): %s", s3URL))
	}

	bucket = parts[0]
	key = parts[1]

	return bucket, key, nil
}

// BuildS3URL constructs an S3 URL from bucket and key.
func BuildS3URL(bucket, key string) string {
	return fmt.Sprintf("s3://%s/%s", bucket, key)
}

// Close closes the S3 manager and releases resources.
func (sm *S3Manager) Close() error {
	// No resources to release in this stub implementation
	return nil
}
