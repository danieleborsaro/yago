package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// ECRClient is an interface for ECR operations.
// This allows for mock implementations in tests.
type ECRClient interface {
	GetImageDigest(imageURL, tag string) (string, error)
	GetLatestImageTag(imageURL string) (string, error)
	ListImageTags(imageURL string) ([]string, error)
	ImageExists(imageURL, tag string) (bool, error)
}

// ECRManager manages AWS ECR operations for Docker image version management.
type ECRManager struct {
	region  string
	profile string
	ctx     context.Context
	client  *ecr.Client
}

// NewECRManager creates a new ECR manager instance.
func NewECRManager(region, profile string) (*ECRManager, error) {
	ctx := context.Background()

	logging.Debug("Creating ECR manager for region %s (profile: %s)", region, profile)

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

	client := ecr.NewFromConfig(cfg)

	return &ECRManager{
		region:  region,
		profile: profile,
		ctx:     ctx,
		client:  client,
	}, nil
}

// GetImageDigest gets the image digest for a specific tag.
// This resolves "latest" or any tag to a specific sha256 digest.
func (em *ECRManager) GetImageDigest(imageURL, tag string) (string, error) {
	// Parse the ECR image URL to extract repository name
	repositoryName, registryID, err := parseECRImageURL(imageURL)
	if err != nil {
		return "", err
	}

	logging.Debug("Querying ECR for image digest: %s:%s (registry: %s)", repositoryName, tag, registryID)

	// Describe images with the specific tag
	input := &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repositoryName),
		ImageIds: []types.ImageIdentifier{
			{
				ImageTag: aws.String(tag),
			},
		},
	}

	// Add registry ID if available
	if registryID != "" {
		input.RegistryId = aws.String(registryID)
	}

	result, err := em.client.DescribeImages(em.ctx, input)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to describe ECR image %s:%s", repositoryName, tag)
	}

	if len(result.ImageDetails) == 0 {
		return "", errors.New(errors.ErrFail, fmt.Sprintf("no image found for %s:%s", repositoryName, tag))
	}

	digest := aws.ToString(result.ImageDetails[0].ImageDigest)
	logging.Debug("Found digest for %s:%s -> %s", repositoryName, tag, digest)

	return digest, nil
}

// GetLatestImageTag gets the latest image tag based on push timestamp.
func (em *ECRManager) GetLatestImageTag(imageURL string) (string, error) {
	// Parse the ECR image URL
	repositoryName, registryID, err := parseECRImageURL(imageURL)
	if err != nil {
		return "", err
	}

	logging.Debug("Querying ECR for latest image tag: %s", repositoryName)

	// List images and sort by push timestamp
	input := &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repositoryName),
	}

	if registryID != "" {
		input.RegistryId = aws.String(registryID)
	}

	result, err := em.client.DescribeImages(em.ctx, input)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to list ECR images for %s", repositoryName)
	}

	if len(result.ImageDetails) == 0 {
		return "", errors.New(errors.ErrFail, fmt.Sprintf("no images found for %s", repositoryName))
	}

	// Find the most recently pushed image
	var latestImage types.ImageDetail
	for _, img := range result.ImageDetails {
		if img.ImagePushedAt == nil {
			continue
		}
		if latestImage.ImagePushedAt == nil || img.ImagePushedAt.After(*latestImage.ImagePushedAt) {
			latestImage = img
		}
	}

	// Get the first tag of the latest image
	if len(latestImage.ImageTags) == 0 {
		return "", errors.New(errors.ErrFail, fmt.Sprintf("latest image for %s has no tags", repositoryName))
	}

	latestTag := latestImage.ImageTags[0]
	logging.Debug("Found latest tag for %s -> %s", repositoryName, latestTag)

	return latestTag, nil
}

// ListImageTags lists all tags for an image repository.
func (em *ECRManager) ListImageTags(imageURL string) ([]string, error) {
	repositoryName, registryID, err := parseECRImageURL(imageURL)
	if err != nil {
		return nil, err
	}

	logging.Debug("Listing ECR image tags for: %s", repositoryName)

	input := &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repositoryName),
	}

	if registryID != "" {
		input.RegistryId = aws.String(registryID)
	}

	result, err := em.client.DescribeImages(em.ctx, input)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to list ECR images for %s", repositoryName)
	}

	// Collect all tags
	var tags []string
	for _, img := range result.ImageDetails {
		tags = append(tags, img.ImageTags...)
	}

	return tags, nil
}

// ImageExists checks if an image with the specified tag exists.
func (em *ECRManager) ImageExists(imageURL, tag string) (bool, error) {
	repositoryName, registryID, err := parseECRImageURL(imageURL)
	if err != nil {
		return false, err
	}

	logging.Debug("Checking if ECR image exists: %s:%s", repositoryName, tag)

	input := &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repositoryName),
		ImageIds: []types.ImageIdentifier{
			{
				ImageTag: aws.String(tag),
			},
		},
	}

	if registryID != "" {
		input.RegistryId = aws.String(registryID)
	}

	result, err := em.client.DescribeImages(em.ctx, input)
	if err != nil {
		return false, nil // Image doesn't exist
	}

	return len(result.ImageDetails) > 0, nil
}

// parseECRImageURL parses an ECR image URL to extract repository name and registry ID.
// ECR URLs are in format: <registry-id>.dkr.ecr.<region>.amazonaws.com/<repository-name>
func parseECRImageURL(imageURL string) (repositoryName, registryID string, err error) {
	// Example: 1688899028633.dkr.ecr.eu-west-1.amazonaws.com/devops.gitops-tools

	parts := strings.Split(imageURL, "/")
	if len(parts) < 2 {
		return "", "", errors.New(errors.ErrParam, fmt.Sprintf("invalid ECR image URL format: %s", imageURL))
	}

	// Repository name is the last part
	repositoryName = parts[len(parts)-1]

	// Registry ID is extracted from the first part
	hostParts := strings.Split(parts[0], ".")
	if len(hostParts) > 0 && strings.Contains(parts[0], "dkr.ecr") {
		registryID = hostParts[0]
	}

	return repositoryName, registryID, nil
}

// Close closes the ECR manager and releases resources.
func (em *ECRManager) Close() error {
	// No resources to release in this stub implementation
	return nil
}
