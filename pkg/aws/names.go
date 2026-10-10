package aws

import (
	"crypto/sha1" //nolint:gosec // the provider versions names with sha1, a stronger hash would give a different name
	"encoding/hex"
	"regexp"
	"slices"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// ported from terraform-provider-awstagging, pkg/tagging/core/namer.go and resources.go at 04f0301

const (
	bucketNameMaxLength      = 63
	environmentNameMaxLength = 20
	// four characters of the hash and the hyphen before them
	nameVersionMinLength = 5
)

// the name awstagging_aws_s3_bucket gives a bucket in that region
func BucketName(c TagConfiguration, region string) (string, error) {
	parts, err := bucketNameParts(c)
	if err != nil {
		return "", err
	}
	regionCode, err := regionCode(region)
	if err != nil {
		return "", err
	}
	// trimmed once and only here, the provider keeps a hyphen that ends one part and so do we
	name := strings.Trim(joinNameParts(append(parts, regionCode)), nameComponentSeparator)

	if c.IsCreateBeforeDestroy != nil && !*c.IsCreateBeforeDestroy {
		return trimName(name), nil
	}

	hash := sha1.Sum([]byte(strings.Join(append(slices.Clone(c.VersioningSources), name), nameComponentSeparator))) //nolint:gosec // see the import
	version := hex.EncodeToString(hash[:])

	// the hash fills whatever is left, and a name with no room for it is cut back until four characters of it fit
	prefixLength, versionLength := len(name), min(len(version), bucketNameMaxLength-len(name))
	if bucketNameMaxLength-len(name) < nameVersionMinLength {
		prefixLength, versionLength = bucketNameMaxLength-nameVersionMinLength, nameVersionMinLength
	}
	prefix := strings.Trim(name[:prefixLength], nameComponentSeparator)
	return trimName(prefix + nameComponentSeparator + version[:versionLength]), nil
}

// the prefix every other bucket of the same project, resource set and role starts with, and the shape the whole name
// has to have to be one of them in that region, only role_extend may tell them apart so anything else sitting
// between the prefix and the region code makes it a bucket of something else
func siblingBucketNames(c TagConfiguration, region string) (prefix string, shape *regexp.Regexp, err error) {
	c.RoleExtend = ""
	parts, err := bucketNameParts(c)
	if err != nil {
		return "", nil, err
	}
	regionCode, err := regionCode(region)
	if err != nil {
		return "", nil, err
	}
	stem := joinNameParts(parts)

	// how an extended name carries on after the stem depends on what the provider did with the role
	role := strings.ToLower(c.canonicalRole())
	extend := "(-.+?)?"
	switch {
	case role == "":
		// it still writes the hyphen that would have joined role and extend, hence the double one
		extend = "(--.+?)?"
	case strings.EqualFold(role, c.ResourceSetShort):
		// a role left out for repeating the resource set comes back once it is extended
		extend = "(-" + regexp.QuoteMeta(role) + "-.+?)?"
	}
	shape = regexp.MustCompile("^" + regexp.QuoteMeta(stem) + extend + "-" + regexp.QuoteMeta(regionCode) + "(-[0-9a-f]+)?$")
	return stem + nameComponentSeparator, shape, nil
}

func bucketNameParts(c TagConfiguration) ([]string, error) {
	if c.CompanyNameShort == "" || c.ProjectNameShort == "" {
		return nil, errors.New(errors.ErrParam, "a bucket name needs company_name_short and project_name_short")
	}
	_, classCode, err := c.accountCoding()
	if err != nil {
		return nil, err
	}

	// a part that only repeats its neighbour is left out to save length
	infraEnvironment, appEnvironment := c.environments()
	infraEnvironment, appEnvironment = shortEnvironment(infraEnvironment), shortEnvironment(appEnvironment)
	if strings.EqualFold(appEnvironment, infraEnvironment) {
		appEnvironment = ""
	}
	resourceSet := c.ResourceSetShort
	if strings.EqualFold(resourceSet, c.ProjectNameShort) {
		resourceSet = ""
	}
	role := c.canonicalRole()
	if strings.EqualFold(role, c.ResourceSetShort) {
		role = ""
	}

	parts := []string{}
	for _, part := range []string{
		c.CompanyNameShort, classCode, infraEnvironment, c.ProjectNameShort, resourceSet, appEnvironment, role,
	} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts, nil
}

func joinNameParts(parts []string) string {
	return strings.ToLower(strings.Join(parts, nameComponentSeparator))
}

func trimName(name string) string {
	return strings.Trim(name[:min(len(name), bucketNameMaxLength)], nameComponentSeparator)
}

func shortEnvironment(environment string) string {
	environment = strings.ToLower(environment)
	return environment[:min(len(environment), environmentNameMaxLength)]
}

// eu-west-1 becomes euw1
func regionCode(region string) (string, error) {
	parts := strings.Split(strings.ToLower(region), "-")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", errors.Newf(errors.ErrParam, "region '%s' is not in the form eu-west-1", region)
	}
	return parts[0] + parts[1][:1] + parts[2], nil
}
