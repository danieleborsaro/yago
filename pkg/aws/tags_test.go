package aws

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func exampleTagConfiguration() TagConfiguration {
	return TagConfiguration{
		Account: "123456789012",
		AccountsCoding: map[string]AccountCoding{
			"123456789012": {Class: "management", Name: "management", NameCanonical: "Management", NameEncoded: "mgmt"},
		},
		CompanyNameShort:   "foo",
		InfraEnvironment:   "management",
		ProjectNameLong:    "example project",
		ResourceSetLong:    "example resource set",
		Role:               "app",
		Owner:              "team@example.com",
		CostCentre:         "cc-1234",
		Compliance:         "internal",
		CustomTags:         map[string]string{"data classification": "internal"},
		CustomTagsVerbatim: map[string]string{"ManagedBy": "terraform"},
	}
}

// The expected tags are what the awstagging provider's own code generates for the same inputs.
func Test_Tags_MatchTheProvider(t *testing.T) {
	config := exampleTagConfiguration()
	config.RoleExtend = "secrets"
	config.TerraformModule = "tf-root-example"
	config.TerraformWorkspace = "default"

	tags, dropped, err := Tags(config, ResourceTypeKmsKey, "")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"ManagedBy":                         "terraform",
		"Foo:Automation:TerraformModule":    "tf-root-example",
		"Foo:Automation:TerraformWorkspace": "default",
		"Foo:Business:CostCentre":           "cc-1234",
		"Foo:Business:Owner":                "team@example.com",
		"Foo:Business:Project":              "Example Project",
		"Foo:Custom:Data Classification":    "internal",
		"Foo:Environment:Account":           "Management",
		"Foo:Environment:AppEnvironment":    "Management",
		"Foo:Environment:InfraEnvironment":  "Management",
		"Foo:Environment:ResourceSet":       "Example Resource Set",
		"Foo:Environment:ResourceType":      ":AWS::KMS::Key",
		"Foo:Environment:Role":              "App-Secrets",
		"Foo:Security:Compliance":           "internal",
	}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags differ from the provider's:\ngot:\n%s\nwant:\n%s", FormatTags(tags), FormatTags(want))
	}
	if len(dropped) != 0 {
		t.Fatalf("dropped tags: %v", dropped)
	}
}

func Test_Tags_SecretWithoutTerraform(t *testing.T) {
	config := exampleTagConfiguration()
	config.Role = "secrets"
	config.CustomTagsVerbatim = map[string]string{"ManagedBy": "yago"}

	tags, _, err := Tags(config, ResourceTypeSecret, "example/app/database")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"Name":                             "example/app/database",
		"ManagedBy":                        "yago",
		"Foo:Business:CostCentre":          "cc-1234",
		"Foo:Business:Owner":               "team@example.com",
		"Foo:Business:Project":             "Example Project",
		"Foo:Custom:Data Classification":   "internal",
		"Foo:Environment:Account":          "Management",
		"Foo:Environment:AppEnvironment":   "Management",
		"Foo:Environment:InfraEnvironment": "Management",
		"Foo:Environment:ResourceSet":      "Example Resource Set",
		"Foo:Environment:ResourceType":     ":AWS::SecretsManager::Secret",
		"Foo:Environment:Role":             "Secrets",
		"Foo:Security:Compliance":          "internal",
	}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("got:\n%s\nwant:\n%s", FormatTags(tags), FormatTags(want))
	}
}

func Test_Tags_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*TagConfiguration)
		want   string
	}{
		{"missing properties", func(c *TagConfiguration) { c.Owner = ""; c.CompanyNameShort = " " },
			"missing or empty tagging properties: company_name_short, owner"},
		{"no account", func(c *TagConfiguration) { c.Account = "" }, "missing AWS account ID"},
		{"no accounts_coding", func(c *TagConfiguration) { c.AccountsCoding = nil }, "accounts_coding"},
		{"account not coded", func(c *TagConfiguration) { c.Account = "210987654321" },
			"account ID '210987654321' is not in accounts_coding"},
		{"unknown class", func(c *TagConfiguration) {
			c.AccountsCoding["123456789012"] = AccountCoding{Class: "sandbox", NameCanonical: "Management"}
		}, "has class 'sandbox'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := exampleTagConfiguration()
			tc.change(&config)
			_, _, err := Tags(config, ResourceTypeSecret, "example")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func Test_Tags_EscapesTheAwsPrefix(t *testing.T) {
	config := exampleTagConfiguration()
	config.CustomTagsVerbatim = map[string]string{"aws:owner": "aws:team"}

	tags, _, err := Tags(config, ResourceTypeSecret, "example")
	if err != nil {
		t.Fatal(err)
	}
	if tags[":aws:owner"] != ":aws:team" {
		t.Fatalf("aws: prefix not escaped: %s", FormatTags(tags))
	}
	if _, ok := tags["aws:owner"]; ok {
		t.Fatalf("unescaped aws: key kept: %s", FormatTags(tags))
	}
}

func Test_Tags_KeepsFiftyTags(t *testing.T) {
	config := exampleTagConfiguration()
	config.CustomTagsVerbatim = map[string]string{}
	for i := 0; i < 60; i++ {
		config.CustomTagsVerbatim[fmt.Sprintf("Verbatim%02d", i)] = "x"
	}

	tags, dropped, err := Tags(config, ResourceTypeSecret, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != maxTags {
		t.Fatalf("got %d tags, want %d", len(tags), maxTags)
	}
	if tags["Name"] != "example" || tags["Foo:Business:Owner"] == "" {
		t.Fatalf("generated tags were dropped: %s", FormatTags(tags))
	}
	for _, key := range dropped {
		if !strings.HasPrefix(key, "Verbatim") {
			t.Fatalf("dropped a generated tag: %s", key)
		}
	}
}
