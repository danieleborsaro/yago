package wrapper

import (
	"strings"
	"testing"
)

func TestUsableConfigRepoLocators_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "List the configuration locators a desiredstate has, in the order wrappers should try them",
		CurrentImpl:     "UsableConfigRepoLocators filters ConfigRepoLocatorsFromMeta by what the desiredstate contains",
		ExpectedOutcome: "A 2.0.0 desiredstate yields its configuration.<env>.<wrapper> map, then the .git repository inside it",
		Rationale:       "The first locator that exists may not be the repository, so wrappers need every candidate to try",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	t.Run("a_2.0.0_desiredstate_yields_the_map_then_its_repository", func(t *testing.T) {
		// Given: a desiredstate with its terraform configuration under configuration.all.terraform.git
		dsContent := map[string]interface{}{
			"desiredstate": map[string]interface{}{
				"configuration": map[string]interface{}{
					"all": map[string]interface{}{
						"terraform": map[string]interface{}{
							"git": map[string]interface{}{"url": "git@example.com:example/configurations.git"},
						},
					},
				},
			},
		}

		// When: the usable locators are resolved for terraform in environment all
		locators, err := UsableConfigRepoLocators("terraform", "all", dsContent, nil, "")

		// Then: the map comes first and the repository second
		if err != nil {
			t.Fatal(err)
		}
		want := "desiredstate.configuration.all.terraform,desiredstate.configuration.all.terraform.git"
		if got := strings.Join(locators, ","); got != want {
			t.Fatalf("got locators %s, want %s", got, want)
		}
	})

	t.Run("a_desiredstate_without_the_configuration_fails", func(t *testing.T) {
		// Given: a desiredstate without a configuration for the wrapper
		dsContent := map[string]interface{}{"desiredstate": map[string]interface{}{}}

		// When: the usable locators are resolved
		_, err := UsableConfigRepoLocators("terraform", "all", dsContent, nil, "")

		// Then: the error names the wrapper, the environment and the locators tried
		if err == nil || !strings.Contains(err.Error(), "wrapper 'terraform' and environment 'all'") ||
			!strings.Contains(err.Error(), "desiredstate.configuration.all.terraform.git") {
			t.Fatalf("expected an error listing the tried locators, got %v", err)
		}
	})
}
