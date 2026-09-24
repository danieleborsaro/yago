package wrapper

import (
	"fmt"
	"strings"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
)

func appendUniqueLocator(target []string, candidate string) []string {
	if candidate == "" {
		return target
	}
	for _, existing := range target {
		if existing == candidate {
			return target
		}
	}
	return append(target, candidate)
}

// ConfigRepoLocatorsFromMeta returns ordered candidate desiredstate locator paths
// for the configuration repository, preferring schema-defined x-gitops-paths when available.
func ConfigRepoLocatorsFromMeta(wrapperName, environment string, dsMeta map[string]interface{}, schemaVersion string) []string {
	locators := make([]string, 0, 8)

	if schemaVersion != "" {
		namespace := NamespaceFromMeta(dsMeta)
		schemaManager, err := schema.GetOrCreateSchemaManagerAuto()
		if err == nil {
			paths, err := schemaManager.DiscoverPropertyPaths(namespace, schema.SchemaVersion(schemaVersion), true)
			if err == nil {
				pathKeys := []string{
					"configRepoLocatorPrimary",
					"configRepoLocatorPrimaryGit",
					"configRepoLocatorLegacy",
					"configRepoLocatorLegacyGit",
				}
				for _, key := range pathKeys {
					if locatorTemplate, pathErr := paths.Get(key); pathErr == nil && locatorTemplate != "" {
						locator := strings.ReplaceAll(locatorTemplate, "{environment}", environment)
						locator = strings.ReplaceAll(locator, "{wrapper}", wrapperName)
						locators = appendUniqueLocator(locators, locator)
					}
				}
			}
		}
	}

	locators = appendUniqueLocator(locators, fmt.Sprintf("desiredstate.content.environments.%s.configurations.%s", environment, wrapperName))
	locators = appendUniqueLocator(locators, fmt.Sprintf("desiredstate.content.environments.%s.configurations.%s.git", environment, wrapperName))
	locators = appendUniqueLocator(locators, fmt.Sprintf("desiredstate.configuration.%s.%s", environment, wrapperName))
	locators = appendUniqueLocator(locators, fmt.Sprintf("desiredstate.configuration.%s.%s.git", environment, wrapperName))

	return locators
}

// UsableConfigRepoLocators returns the candidate locators the desiredstate has, in order.
// A locator can exist without being the repository itself, so callers try each until one clones.
func UsableConfigRepoLocators(wrapperName, environment string, dsContent, dsMeta map[string]interface{}, schemaVersion string) ([]string, error) {
	return filterUsableLocators(dsContent, wrapperName, environment, ConfigRepoLocatorsFromMeta(wrapperName, environment, dsMeta, schemaVersion))
}

func filterUsableLocators(dsContent map[string]interface{}, wrapperName, environment string, locators []string) ([]string, error) {
	h := parser.NewYAMLHandler("")
	usable := make([]string, 0, len(locators))
	for _, locator := range locators {
		if _, err := h.GetValue(dsContent, locator); err == nil {
			usable = append(usable, locator)
		}
	}
	if len(usable) == 0 {
		return nil, errors.Newf(errors.ErrParse,
			"no configuration repository locator found for wrapper '%s' and environment '%s' (tried locators: %v)",
			wrapperName, environment, locators)
	}
	return usable, nil
}
