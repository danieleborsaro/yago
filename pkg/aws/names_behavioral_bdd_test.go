package aws

import "testing"

func exampleNameConfiguration() TagConfiguration {
	config := exampleTagConfiguration()
	config.AccountsCoding["210987654321"] = AccountCoding{Class: "production", Name: "production", NameCanonical: "Production", NameEncoded: "prod"}
	config.AccountsCoding["111111111111"] = AccountCoding{Class: "development", Name: "development", NameCanonical: "Development", NameEncoded: "dev"}
	config.AccountsCoding["222222222222"] = AccountCoding{Class: "preproduction", Name: "preproduction", NameCanonical: "Preproduction", NameEncoded: "pre"}
	config.AccountsCoding["333333333333"] = AccountCoding{Class: "undefined", Name: "sandbox", NameCanonical: "Sandbox", NameEncoded: "sbx"}
	config.ProjectNameShort = "inception"
	config.ResourceSetShort = "state"
	config.Role = ""
	return config
}

// the expected names are what the awstagging provider's own code generates for the same inputs
func TestBucketName_MatchesTheProvider_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Name a state bucket exactly as awstagging_aws_s3_bucket would",
		CurrentImpl:     "BucketName joins the provider's S3 components and versions the name with the provider's sha1 suffix",
		ExpectedOutcome: "The same name as the provider for every case, quirks included",
		Rationale:       "A bucket can't be renamed, so one made by yago has to carry the name Terraform would have given it",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	notVersioned := false
	cases := []struct {
		name   string
		region string
		tweak  func(*TagConfiguration)
		want   string
	}{
		{name: "base", want: "foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8"},
		{name: "role extend without a role keeps the provider's double hyphen",
			tweak: func(c *TagConfiguration) { c.RoleExtend = "dev" },
			want:  "foo-m-management-inception-state--dev-euw1-b74d7cd3c2d40d0494c3"},
		{name: "role and extend",
			tweak: func(c *TagConfiguration) { c.Role = "app"; c.RoleExtend = "blue team" },
			want:  "foo-m-management-inception-state-app-blueteam-euw1-f2a57599d333"},
		{name: "a name too long for the hash loses its tail, the region with it",
			tweak: func(c *TagConfiguration) { c.RoleExtend = "averyveryverylongroleextendthatoverflows" },
			want:  "foo-m-management-inception-state--averyveryverylongroleext-0189"},
		{name: "one character short of room for the hash",
			tweak: func(c *TagConfiguration) { c.RoleExtend = "abcdefghijklmnopqrstuvw" },
			want:  "foo-m-management-inception-state--abcdefghijklmnopqrstuvw-a0cee"},
		{name: "versioning sources change the hash only",
			tweak: func(c *TagConfiguration) { c.VersioningSources = []string{"v2", "blue"} },
			want:  "foo-m-management-inception-state-euw1-272655d0e52f307fbbca09987"},
		{name: "not versioned has no hash",
			tweak: func(c *TagConfiguration) { c.IsCreateBeforeDestroy = &notVersioned },
			want:  "foo-m-management-inception-state-euw1"},
		{name: "app environment is added when it differs",
			tweak: func(c *TagConfiguration) { c.AppEnvironment = "Staging" },
			want:  "foo-m-management-inception-state-staging-euw1-7acce6b79878b616f"},
		{name: "resource set that repeats the project is dropped",
			tweak: func(c *TagConfiguration) { c.ResourceSetShort = "Inception" },
			want:  "foo-m-management-inception-euw1-e837d026f9c34e15d22b0b2e1752157"},
		{name: "role that repeats the resource set is dropped",
			tweak: func(c *TagConfiguration) { c.Role = "state" },
			want:  "foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8"},
		{name: "production account in us-east-1", region: "us-east-1",
			tweak: func(c *TagConfiguration) { c.Account = "210987654321"; c.InfraEnvironment = "production" },
			want:  "foo-p-production-inception-state-use1-d47fe11ad710607722363dc66"},
		{name: "two word direction", region: "ap-southeast-2",
			want: "foo-m-management-inception-state-aps2-658d6fe3270edb4fb1d4c73fa"},
		{name: "upper case company is lowered",
			tweak: func(c *TagConfiguration) { c.CompanyNameShort = "Foo" },
			want:  "foo-m-management-inception-state-euw1-c4bfe936cc80a6ec1e47d97f8"},
		{name: "long environment is cut to twenty",
			tweak: func(c *TagConfiguration) { c.InfraEnvironment = "a-very-long-environment-name-here" },
			want:  "foo-m-a-very-long-environm-inception-state-euw1-d4b82d7ed1f6fe9"},
		{name: "an environment cut on a hyphen keeps it, the provider only trims the ends of the whole name",
			tweak: func(c *TagConfiguration) { c.AppEnvironment = "customer-acceptance-test" },
			want:  "foo-m-management-inception-state-customer-acceptance--euw1-4314"},
		{name: "a role extend typed with a hyphen at its end keeps it too",
			tweak: func(c *TagConfiguration) { c.RoleExtend = "dev-" },
			want:  "foo-m-management-inception-state--dev--euw1-740f7cf85a37f0cbd98"},
		{name: "a role that repeats the resource set comes back once it is extended",
			tweak: func(c *TagConfiguration) { c.Role = "state"; c.RoleExtend = "dev" },
			want:  "foo-m-management-inception-state-state-dev-euw1-eec0aecf083163f"},
		{name: "only the app environment set",
			tweak: func(c *TagConfiguration) { c.InfraEnvironment = ""; c.AppEnvironment = "staging" },
			want:  "foo-m-staging-inception-state-euw1-3a97182e00dcff9789c8b5fbd06c"},
		{name: "not versioned and too long is simply cut",
			tweak: func(c *TagConfiguration) {
				c.IsCreateBeforeDestroy = &notVersioned
				c.RoleExtend = "averyveryverylongroleextendthatoverflows"
			},
			want: "foo-m-management-inception-state--averyveryverylongroleextendth"},
		{name: "development account",
			tweak: func(c *TagConfiguration) { c.Account = "111111111111"; c.InfraEnvironment = "development" },
			want:  "foo-d-development-inception-state-euw1-872d894805f143b7a214d716"},
		{name: "preproduction account",
			tweak: func(c *TagConfiguration) { c.Account = "222222222222"; c.InfraEnvironment = "staging" },
			want:  "foo-e-staging-inception-state-euw1-1367d8dec650a280f216c1bf700d"},
		{name: "undefined account",
			tweak: func(c *TagConfiguration) { c.Account = "333333333333"; c.InfraEnvironment = "sandbox" },
			want:  "foo-u-sandbox-inception-state-euw1-373733ada490b733e06c8317dbcf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: the provider's inputs for that case
			config := exampleNameConfiguration()
			if tc.tweak != nil {
				tc.tweak(&config)
			}
			region := tc.region
			if region == "" {
				region = "eu-west-1"
			}

			// When: the bucket is named
			got, err := BucketName(config, region)

			// Then: it is the provider's name
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("name = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBucketName_ReadsWhetherItIsVersionedFromTheProperties_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "is_create_before_destroy in the project properties decides whether the name gets the provider's hash, and a quoted value reads like a bare one",
		CurrentImpl:     "TagConfigurationFromProperties reads the property by hand, anything but false means versioned, as does leaving it out",
		ExpectedOutcome: "false and \"false\" give the name without a hash, true, \"true\" and no property give it with one",
		Rationale:       "Roots feed the provider this property and Terraform takes a quoted bool, so a configuration that works there has to name the same bucket here",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := []struct {
		name      string
		property  interface{}
		versioned bool
	}{
		{name: "left out", versioned: true},
		{name: "true", property: true, versioned: true},
		{name: "quoted true", property: "true", versioned: true},
		{name: "false", property: false},
		{name: "quoted false", property: "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: the properties with that value
			properties := exampleStateBucketProperties()
			if tc.property != nil {
				properties["is_create_before_destroy"] = tc.property
			}

			// When: they are read and the bucket is named
			config, err := TagConfigurationFromProperties(properties, "123456789012")
			if err != nil {
				t.Fatal(err)
			}
			name, err := BucketName(config, "eu-west-1")

			// Then: the hash is there only when the name is versioned
			if err != nil {
				t.Fatal(err)
			}
			want := "foo-m-management-inception-state-euw1"
			if tc.versioned {
				want = exampleStateBucketName
			}
			if name != want {
				t.Errorf("name = %q, want %q", name, want)
			}
		})
	}
}

func TestBucketName_StopsWithoutWhatTheNameIsMadeOf_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Refuse to name a bucket when the project, the account or the region can't be worked out",
		CurrentImpl:     "BucketName checks project_name_short, accounts_coding and the shape of the region before joining anything",
		ExpectedOutcome: "An error and no name",
		Rationale:       "A name built around a missing part would still be a valid bucket name, just not ours, and it can't be renamed",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := map[string]struct {
		region string
		tweak  func(*TagConfiguration)
	}{
		"no project short name": {region: "eu-west-1", tweak: func(c *TagConfiguration) { c.ProjectNameShort = "" }},
		"account not coded":     {region: "eu-west-1", tweak: func(c *TagConfiguration) { c.Account = "999999999999" }},
		"region without a zone": {region: "europe", tweak: func(c *TagConfiguration) {}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Given: a configuration with that part missing
			config := exampleNameConfiguration()
			tc.tweak(&config)

			// When: the bucket is named
			got, err := BucketName(config, tc.region)

			// Then: it stops
			if err == nil || got != "" {
				t.Errorf("name = %q, err = %v", got, err)
			}
		})
	}
}
