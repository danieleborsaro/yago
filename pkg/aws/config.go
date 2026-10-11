package aws

import (
	"context"
	stderrors "errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// the shortest wait KMS allows before a key scheduled for deletion is gone
const kmsKeyDeletionDays = 7

func callerAccount(ctx context.Context, client stsAPI) (string, error) {
	identity, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get AWS account ID")
	}
	return aws.ToString(identity.Account), nil
}

func isAPIError(err error, code string) bool {
	var apiErr smithy.APIError
	return stderrors.As(err, &apiErr) && apiErr.ErrorCode() == code
}

// an empty profile leaves the choice to the environment, AWS_PROFILE included
func loadConfig(ctx context.Context, region, profile string) (aws.Config, error) {
	options := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if profile != "" {
		options = append(options, config.WithSharedConfigProfile(profile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return aws.Config{}, errors.Wrapf(errors.ErrFail, err, "failed to load AWS config")
	}
	return cfg, nil
}
