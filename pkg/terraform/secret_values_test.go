package terraform

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type secretReadCall struct {
	name, version, stage string
}

type fakeSecretValueReader struct {
	values map[secretReadCall]string
	err    error
	calls  []secretReadCall
}

func (r *fakeSecretValueReader) Read(name, version, stage string) (string, error) {
	call := secretReadCall{name, version, stage}
	r.calls = append(r.calls, call)
	return r.values[call], r.err
}

func TestValidateSecretBindings(t *testing.T) {
	for _, name := range []string{"password", "_private", "Token123", "_"} {
		if err := validateSecretBindings(map[string]SecretVariableBinding{name: {SecretID: "existing-secret"}}); err != nil {
			t.Errorf("valid name %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", "123token", "with-dash", "with.dot", "with space", "ümlaut", "x=y", "x\x00y"} {
		if err := validateSecretBindings(map[string]SecretVariableBinding{name: {SecretID: "existing-secret"}}); err == nil {
			t.Errorf("invalid name %q accepted", name)
		}
	}
	for _, binding := range []SecretVariableBinding{
		{}, {SecretID: " \t\n"}, {SecretID: "existing-secret", VersionID: "version", VersionStage: "AWSCURRENT"},
	} {
		if err := validateSecretBindings(map[string]SecretVariableBinding{"password": binding}); err == nil {
			t.Errorf("invalid binding accepted: %#v", binding)
		}
	}
}

func TestResolveSecretVariablesCachesVersionsAndOrdersReads(t *testing.T) {
	reader := &fakeSecretValueReader{values: map[secretReadCall]string{
		{name: "shared"}:                    `{"password":"first-password","username":"service-user"}`,
		{name: "shared", version: "pinned"}: "pinned-password",
		{name: "shared", stage: "previous"}: "previous-password",
	}}
	bindings := map[string]SecretVariableBinding{
		"z_previous": {SecretID: "shared", VersionStage: "previous"},
		"username":   {SecretID: "shared", JSONKey: "username"},
		"pinned":     {SecretID: "shared", VersionID: "pinned"},
		"password":   {SecretID: "shared", JSONKey: "password"},
	}
	got, err := resolveSecretVariables(bindings, reader)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"TF_VAR_password": "first-password", "TF_VAR_username": "service-user",
		"TF_VAR_pinned": "pinned-password", "TF_VAR_z_previous": "previous-password",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unexpected resolved environment: got %v, want %v", got, want)
	}
	wantCalls := []secretReadCall{{name: "shared"}, {name: "shared", version: "pinned"}, {name: "shared", stage: "previous"}}
	if !reflect.DeepEqual(reader.calls, wantCalls) {
		t.Errorf("unexpected read order/cache behavior: got %v, want %v", reader.calls, wantCalls)
	}
	if _, err := resolveSecretVariables(bindings, reader); err != nil {
		t.Fatal(err)
	}
	if len(reader.calls) != 2*len(wantCalls) {
		t.Error("cache must not persist across invocations")
	}
}

func TestResolveSecretVariablesJSONTypes(t *testing.T) {
	for _, tc := range []struct {
		name, payload, key, want string
	}{
		{"whole string", "plain-password", "", "plain-password"},
		{"whole JSON", ` {"password": "secret"} `, "", ` {"password": "secret"} `},
		{"string", `{"value":"line1\nline2"}`, "value", "line1\nline2"},
		{"empty string", `{"value":""}`, "value", ""},
		{"boolean", `{"value":false}`, "value", "false"},
		{"number", `{"value":9007199254740993}`, "value", "9007199254740993"},
		{"object", `{"value": { "nested": "secret", "n": 1 }}`, "value", `{"nested":"secret","n":1}`},
		{"array", `{"value": ["secret", true, 2]}`, "value", `["secret",true,2]`},
		{"literal top-level key", `{"db.password":"secret","db":{"password":"other"}}`, "db.password", "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeSecretValueReader{values: map[secretReadCall]string{{name: "secret"}: tc.payload}}
			env, err := resolveSecretVariables(map[string]SecretVariableBinding{"value": {SecretID: "secret", JSONKey: tc.key}}, reader)
			if err != nil {
				t.Fatal(err)
			}
			if env["TF_VAR_value"] != tc.want {
				t.Errorf("got %q, want %q", env["TF_VAR_value"], tc.want)
			}
		})
	}
}

func TestResolveSecretVariablesFailsClosed(t *testing.T) {
	for _, payload := range []string{
		"payload-marker", `{"value":"payload-marker"`, `{"other":"payload-marker"}`,
		`{"value":null,"other":"payload-marker"}`, `[{"value":"payload-marker"}]`, `null`,
		`{"value":"payload-marker\u0000"}`, `{"value":"payload-marker"} trailing`,
	} {
		reader := &fakeSecretValueReader{values: map[secretReadCall]string{
			{name: "valid"}: "already-resolved-value", {name: "invalid"}: payload,
		}}
		env, err := resolveSecretVariables(map[string]SecretVariableBinding{
			"a_valid": {SecretID: "valid"}, "b_invalid": {SecretID: "invalid", JSONKey: "value"},
		}, reader)
		if err == nil || env != nil {
			t.Fatalf("invalid payload must return an error and no partial environment")
		}
		if strings.Contains(err.Error(), "payload-marker") || strings.Contains(err.Error(), "already-resolved-value") {
			t.Errorf("secret leaked in error: %v", err)
		}
	}
	reader := &fakeSecretValueReader{err: errors.New("AccessDeniedException")}
	if _, err := resolveSecretVariables(map[string]SecretVariableBinding{"password": {SecretID: "secret"}}, reader); err == nil ||
		!strings.Contains(err.Error(), `"password"`) || !strings.Contains(err.Error(), "AccessDeniedException") {
		t.Fatalf("reader errors must name the variable and keep their cause: %v", err)
	}
	if _, err := resolveSecretVariables(map[string]SecretVariableBinding{"password": {}}, reader); err == nil || len(reader.calls) != 1 {
		t.Fatal("invalid bindings must be rejected before reading any secret")
	}
	if _, err := resolveSecretVariables(map[string]SecretVariableBinding{"password": {SecretID: "secret"}}, nil); err == nil {
		t.Fatal("missing reader must fail")
	}
	if env, err := resolveSecretVariables(nil, nil); err != nil || len(env) != 0 {
		t.Fatalf("empty bindings should require no reader: %v", err)
	}
}

func TestSecretRedactor(t *testing.T) {
	env := map[string]string{
		"TF_VAR_password": "overlapping-secret",
		"TF_VAR_prefix":   "overlapping",
		"TF_VAR_object":   `{"nested":{"password":"nested-password"},"array":["array-password",{"escaped":"line1\nline2"}]}`,
		"TF_VAR_empty":    "",
	}
	output := "overlapping-secret overlapping nested-password array-password line1\nline2 line1\\nline2 " + env["TF_VAR_object"]
	want := "[REDACTED] [REDACTED] [REDACTED] [REDACTED] [REDACTED] [REDACTED] [REDACTED]"
	if got := newSecretRedactor(env).redact(output); got != want {
		t.Errorf("unexpected redacted output: %q", got)
	}
	if got := newSecretRedactor(nil).redact("ordinary output"); got != "ordinary output" {
		t.Errorf("empty secrets changed output: %q", got)
	}
	// Replacements are performed once, so a secret matching the marker cannot corrupt it.
	if got := newSecretRedactor(map[string]string{"a": "password", "b": "REDACTED"}).redact("password REDACTED"); got != "[REDACTED] [REDACTED]" {
		t.Errorf("replacement markers were processed again: %q", got)
	}

	escaped := newSecretRedactor(map[string]string{"a": `pa"ss&w<o>rd\1`})
	if got := escaped.redact(`password = "pa\"ss&w<o>rd\\1"`); got != `password = "[REDACTED]"` {
		t.Errorf("secret as a plan prints it wasn't redacted: %q", got)
	}
	if got := escaped.redact(`{"password":"pa\"ss&w<o>rd\\1"}`); got != `{"password":"[REDACTED]"}` {
		t.Errorf("secret as JSON output prints it wasn't redacted: %q", got)
	}

	multiline := newSecretRedactor(map[string]string{
		"a": "-----BEGIN EXAMPLE-----\nZXhhbXBsZQ==\n-----END EXAMPLE-----\n",
		"b": "pin\n123",
	})
	heredoc := "<<-EOT\n    -----BEGIN EXAMPLE-----\n    ZXhhbXBsZQ==\n    -----END EXAMPLE-----\nEOT"
	if got := multiline.redact(heredoc); got != "<<-EOT\n    [REDACTED]\n    [REDACTED]\n    [REDACTED]\nEOT" {
		t.Errorf("heredoc lines of a secret weren't redacted: %q", got)
	}
	for output, want := range map[string]string{
		"pin\n123\n":                    "[REDACTED]\n",
		"pin\n":                         "[REDACTED]\n",
		"123":                           "[REDACTED]",
		"    pin\n    123\n":            "    [REDACTED]\n    [REDACTED]\n",
		"      - pin\n      + 123\r\n":  "      - [REDACTED]\n      + [REDACTED]\r\n",
		"spinning 1234\nid = \"pin\"\n": "spinning 1234\nid = \"pin\"\n",
	} {
		if got := multiline.redact(output); got != want {
			t.Errorf("redact(%q) = %q, want %q", output, got, want)
		}
	}
	// Output lines that are just a short secret line, like a lone brace, get redacted too
	braces := newSecretRedactor(map[string]string{"a": "{\n  \"id\": 5\n}"})
	if got := braces.redact("tags = {}\n  }\n"); got != "tags = {}\n  [REDACTED]\n" {
		t.Errorf("short secret lines not redacted as whole lines only: %q", got)
	}

	marked := newSecretRedactor(map[string]string{"a": "+ a\n- b"})
	for output, want := range map[string]string{
		"password = <<-EOT\n      + a\n      - b\nEOT\n": "password = <<-EOT\n      [REDACTED]\n      [REDACTED]\nEOT\n",
		"      + + a\n      - - b\n":                     "      + [REDACTED]\n      - [REDACTED]\n",
		"+ create\n":                                     "+ create\n",
	} {
		if got := marked.redact(output); got != want {
			t.Errorf("redact(%q) = %q, want %q", output, got, want)
		}
	}
}
