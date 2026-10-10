package aws

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// Ported from terraform-provider-awstagging (pkg/tagging/core at 04f0301), which needs Go 1.27.1 so can't be imported.

const (
	ResourceTypeSecret = "AWS::SecretsManager::Secret" //nolint:gosec // a resource type, not a credential
	ResourceTypeKmsKey = "AWS::KMS::Key"
	ResourceTypeBucket = "AWS::S3::Bucket"
)

const (
	tagComponentSeparator  = ":"
	nameComponentSeparator = "-"
	reservedTagKeyPrefix   = ":"
	tagValueReservedPrefix = "aws:"
	tagKeyMaxLength        = 127
	tagValueMaxLength      = 255
	nameTagKey             = "Name"
	customTagSection       = "Custom"
	maxTags                = 50
)

// the code is the letter the provider puts in a name for an account of that class
var environmentClasses = []struct{ name, code string }{
	{"development", "d"}, {"preproduction", "e"}, {"production", "p"}, {"management", "m"}, {"undefined", "u"},
}

type AccountCoding struct {
	Class         string `json:"class" yaml:"class"`
	Name          string `json:"name" yaml:"name"`
	NameCanonical string `json:"name_canonical" yaml:"name_canonical"`
	NameEncoded   string `json:"name_encoded" yaml:"name_encoded"`
}

type TagConfiguration struct {
	Account            string                   `yaml:"-"`
	AccountsCoding     map[string]AccountCoding `yaml:"accounts_coding"`
	CompanyNameShort   string                   `yaml:"company_name_short"`
	AppEcosystem       string                   `yaml:"app_ecosystem"`
	AppEnvironment     string                   `yaml:"app_environment"`
	InfraEnvironment   string                   `yaml:"infra_environment"`
	ProjectNameLong    string                   `yaml:"project_name_long"`
	ProjectNameShort   string                   `yaml:"project_name_short"`
	ResourceSetLong    string                   `yaml:"resource_set_long"`
	ResourceSetShort   string                   `yaml:"resource_set_short"`
	Role               string                   `yaml:"role"`
	RoleExtend         string                   `yaml:"-"`
	Owner              string                   `yaml:"owner"`
	CostCentre         string                   `yaml:"cost_centre"`
	Compliance         string                   `yaml:"compliance"`
	Description        string                   `yaml:"description"`
	CustomTags         map[string]string        `yaml:"custom_tags"`
	CustomTagsVerbatim map[string]string        `yaml:"custom_tags_verbatim"`
	TerraformModule    string                   `yaml:"-"`
	TerraformWorkspace string                   `yaml:"-"`
	// unset means true, as in the provider
	IsCreateBeforeDestroy *bool    `yaml:"-"`
	VersioningSources     []string `yaml:"-"`
}

// properties are an assembled configuration's project properties, with their scopes already merged into one map
func TagConfigurationFromProperties(properties interface{}, account string) (TagConfiguration, error) {
	config := TagConfiguration{}
	content, err := yaml.Marshal(properties)
	if err == nil {
		err = yaml.Unmarshal(content, &config)
	}
	if err != nil {
		return TagConfiguration{}, errors.Wrap(errors.ErrParse, "failed to read project_properties", err)
	}
	config.Account = account

	// terraform takes "false" for a bool as happily as false, so a quoted one must not fail the whole decode
	if scalars, ok := properties.(map[string]interface{}); ok {
		if raw, set := scalars["is_create_before_destroy"]; set {
			isCreateBeforeDestroy := fmt.Sprint(raw) != "false"
			config.IsCreateBeforeDestroy = &isCreateBeforeDestroy
		}
	}
	return config, nil
}

func kmsTags(tags map[string]string) []kmstypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	kmsTags := make([]kmstypes.Tag, 0, len(tags))
	for _, key := range slices.Sorted(maps.Keys(tags)) {
		kmsTags = append(kmsTags, kmstypes.Tag{TagKey: aws.String(key), TagValue: aws.String(tags[key])})
	}
	return kmsTags
}

func (c TagConfiguration) Validate() error {
	required := []struct {
		name  string
		value string
	}{
		{"company_name_short", c.CompanyNameShort},
		{"infra_environment", c.InfraEnvironment},
		{"owner", c.Owner},
		{"cost_centre", c.CostCentre},
		{"compliance", c.Compliance},
		{"project_name_long", c.ProjectNameLong},
		{"resource_set_long", c.ResourceSetLong},
	}

	missing := []string{}
	for _, property := range required {
		if strings.TrimSpace(property.value) == "" {
			missing = append(missing, property.name)
		}
	}
	if len(missing) > 0 {
		return errors.Newf(errors.ErrParam, "missing or empty tagging properties: %s", strings.Join(missing, ", "))
	}

	if c.Account == "" {
		return errors.New(errors.ErrParam, "missing AWS account ID for tagging")
	}
	if len(c.AccountsCoding) == 0 {
		return errors.New(errors.ErrParam, "missing or empty tagging property: accounts_coding")
	}

	return nil
}

// a caser keeps state between calls and isn't safe to share, so each call gets its own
func title(text string) string {
	return cases.Title(language.English, cases.NoLower).String(text)
}

func (c TagConfiguration) accountCoding() (account AccountCoding, classCode string, err error) {
	// NB: the provider removes a leading zero because of https://github.com/hashicorp/terraform/issues/28619
	account, ok := c.AccountsCoding[strings.TrimPrefix(c.Account, "0")]
	if !ok {
		return AccountCoding{}, "", errors.Newf(errors.ErrParam, "account ID '%s' is not in accounts_coding", c.Account)
	}
	names := make([]string, 0, len(environmentClasses))
	for _, class := range environmentClasses {
		if class.name == account.Class {
			return account, class.code, nil
		}
		names = append(names, class.name)
	}
	return AccountCoding{}, "", errors.Newf(errors.ErrParam,
		"account ID '%s' has class '%s' in accounts_coding. Use one of %s",
		c.Account, account.Class, strings.Join(names, ", "))
}

// each environment falls back to the other when only one is set
func (c TagConfiguration) environments() (infra, app string) {
	infra, app = c.InfraEnvironment, c.AppEnvironment
	if infra == "" {
		infra = app
	}
	if app == "" {
		app = infra
	}
	return infra, app
}

func (c TagConfiguration) canonicalRole() string {
	canonical := func(role string) string {
		return title(strings.Trim(strings.TrimPrefix(strings.ReplaceAll(role, " ", ""), nameComponentSeparator), " "))
	}
	role := canonical(c.Role)
	if extend := canonical(c.RoleExtend); extend != "" {
		role = role + nameComponentSeparator + extend
	}
	return role
}

func Tags(c TagConfiguration, resourceType, name string) (map[string]string, []string, error) {
	return tagsWithRoom(c, resourceType, name, 0)
}

// keeps that many of the tags AWS allows free, for the ones a caller adds on top
func tagsWithRoom(c TagConfiguration, resourceType, name string, room int) (map[string]string, []string, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}

	account, _, err := c.accountCoding()
	if err != nil {
		return nil, nil, err
	}
	infraEnvironment, appEnvironment := c.environments()
	canonicalRole := c.canonicalRole()

	sections := map[string]map[string]string{
		"business": {
			"CostCentre": strings.Trim(c.CostCentre, " "),
			"Owner":      strings.Trim(c.Owner, " "),
			"Project":    title(c.ProjectNameLong),
		},
		"security": {
			"Compliance": strings.Trim(c.Compliance, " "),
		},
		"environment": {
			"Account":          account.NameCanonical,
			"AppEcosystem":     title(strings.ToLower(c.AppEcosystem)),
			"AppEnvironment":   title(strings.ToLower(appEnvironment)),
			"Description":      strings.Trim(c.Description, " "),
			"InfraEnvironment": title(strings.ToLower(infraEnvironment)),
			"Name":             name,
			"ResourceSet":      title(c.ResourceSetLong),
			"Role":             title(strings.Trim(strings.TrimPrefix(canonicalRole, nameComponentSeparator), " ")),
			"ResourceType":     resourceType,
		},
	}
	if c.TerraformModule != "" || c.TerraformWorkspace != "" {
		terraformModule := ""
		if c.TerraformModule != "" {
			moduleAbs, _ := filepath.Abs(c.TerraformModule)
			terraformModule = filepath.Base(moduleAbs)
		}
		sections["automation"] = map[string]string{
			"TerraformModule":    terraformModule,
			"TerraformWorkspace": c.TerraformWorkspace,
		}
	}

	rawTags := map[string]string{}
	for key, value := range c.CustomTags {
		rawTags[customTagSection+tagComponentSeparator+title(key)] = value
	}
	for section, fields := range sections {
		for field, value := range fields {
			if field == nameTagKey {
				rawTags[field] = value
			} else {
				rawTags[section+tagComponentSeparator+title(field)] = value
			}
		}
	}

	prefixedTags := map[string]string{}
	for key, value := range rawTags {
		if value == "" {
			continue
		}
		key = title(key)
		if key == nameTagKey {
			prefixedTags[title(nameTagKey)] = value
		} else {
			prefixedTags[title(c.CompanyNameShort+tagComponentSeparator+key)] = value
		}
	}

	allTags := map[string]string{}
	for key, value := range prefixedTags {
		allTags[key] = value
	}
	for key, value := range c.CustomTagsVerbatim {
		allTags[key] = value
	}

	// AWS reserves the aws: prefix, so keys and values starting with it get a leading ':'.
	escape := func(s string) string {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(tagValueReservedPrefix)) {
			return reservedTagKeyPrefix + s
		}
		return s
	}
	escapedTags := map[string]string{}
	for key, value := range allTags {
		escapedKey := escape(key)
		if _, exists := allTags[escapedKey]; exists && escapedKey != key {
			continue
		}
		escapedTags[escapedKey] = escape(value)
	}

	limitedTags := map[string]string{}
	for key, value := range escapedTags {
		limitedTags[key[:min(len(key), tagKeyMaxLength)]] = value[:min(len(value), tagValueMaxLength)]
	}

	generatedPrefix := title(c.CompanyNameShort + tagComponentSeparator)
	customPrefix := generatedPrefix + customTagSection + tagComponentSeparator
	rank := func(key string) int {
		switch {
		case key == nameTagKey:
			return 0
		case strings.HasPrefix(key, customPrefix):
			return 2
		case strings.HasPrefix(key, generatedPrefix):
			return 1
		default:
			return 3
		}
	}
	orderedKeys := make([]string, 0, len(limitedTags))
	for key := range limitedTags {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Slice(orderedKeys, func(i, j int) bool {
		if rank(orderedKeys[i]) != rank(orderedKeys[j]) {
			return rank(orderedKeys[i]) < rank(orderedKeys[j])
		}
		return orderedKeys[i] < orderedKeys[j]
	})

	limit := maxTags - room
	if resourceType == ResourceTypeSecret {
		limit-- // Leaves room for the isCreatedHere tag that Create adds, which destroy needs.
	}

	tags := map[string]string{}
	dropped := []string{}
	for i, key := range orderedKeys {
		if i < limit {
			tags[key] = limitedTags[key]
		} else {
			dropped = append(dropped, key)
		}
	}

	return tags, dropped, nil
}

func FormatTags(tags map[string]string) string {
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s = %s", key, tags[key]))
	}
	return strings.Join(lines, "\n")
}
