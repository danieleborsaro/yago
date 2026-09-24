package aws

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// Ported from terraform-provider-awstagging (pkg/tagging/core at 04f0301), which needs Go 1.27.1 so can't be imported.

const (
	ResourceTypeSecret = "AWS::SecretsManager::Secret"
	ResourceTypeKmsKey = "AWS::KMS::Key"
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

var environmentClasses = []string{"development", "preproduction", "production", "management", "undefined"}

type AccountCoding struct {
	Class         string `json:"class" yaml:"class"`
	Name          string `json:"name" yaml:"name"`
	NameCanonical string `json:"name_canonical" yaml:"name_canonical"`
	NameEncoded   string `json:"name_encoded" yaml:"name_encoded"`
}

type TagConfiguration struct {
	Account            string
	AccountsCoding     map[string]AccountCoding
	CompanyNameShort   string
	AppEcosystem       string
	AppEnvironment     string
	InfraEnvironment   string
	ProjectNameLong    string
	ResourceSetLong    string
	Role               string
	RoleExtend         string
	Owner              string
	CostCentre         string
	Compliance         string
	Description        string
	CustomTags         map[string]string
	CustomTagsVerbatim map[string]string
	TerraformModule    string
	TerraformWorkspace string
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

func Tags(c TagConfiguration, resourceType, name string) (map[string]string, []string, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}

	// NB: the provider removes a leading zero because of https://github.com/hashicorp/terraform/issues/28619
	accountID := strings.TrimPrefix(c.Account, "0")
	account, ok := c.AccountsCoding[accountID]
	if !ok {
		return nil, nil, errors.Newf(errors.ErrParam, "account ID '%s' is not in accounts_coding", c.Account)
	}
	if !slices.Contains(environmentClasses, account.Class) {
		return nil, nil, errors.Newf(errors.ErrParam,
			"account ID '%s' has class '%s' in accounts_coding. Use one of %s",
			c.Account, account.Class, strings.Join(environmentClasses, ", "))
	}

	infraEnvironment := c.InfraEnvironment
	if infraEnvironment == "" {
		infraEnvironment = c.AppEnvironment
	}
	appEnvironment := c.AppEnvironment
	if appEnvironment == "" {
		appEnvironment = c.InfraEnvironment
	}

	caser := cases.Title(language.English, cases.NoLower)
	title := caser.String

	canonicalRole := title(strings.Trim(strings.TrimPrefix(strings.ReplaceAll(c.Role, " ", ""), nameComponentSeparator), " "))
	canonicalRoleExtend := title(strings.Trim(strings.TrimPrefix(strings.ReplaceAll(c.RoleExtend, " ", ""), nameComponentSeparator), " "))
	if canonicalRoleExtend != "" {
		canonicalRole = canonicalRole + nameComponentSeparator + canonicalRoleExtend
	}

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

	tags := map[string]string{}
	dropped := []string{}
	for i, key := range orderedKeys {
		if i < maxTags {
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
