package tplutil

import (
	"strings"
	"testing"
)

func TestRenderTemplateWithHooks(t *testing.T) {
	tpl := `ip={{ .ip }}
{{- range .hooks }}
{{ include . $ }}
{{- end }}`
	hooks := map[string]string{
		"hook_a_0": `name={{ toUpper .name }}`,
		"hook_a_1": `sum={{ eval "1 + 2" }} / {{ include "hook_a_0" $ }}-again`,
	}
	data := map[string]any{
		"ip":    "10.0.0.1",
		"name":  "node1",
		"hooks": []string{"hook_a_0", "hook_a_1"},
	}

	out, err := RenderTemplateWithHooks(tpl, data, hooks)
	if err != nil {
		t.Fatalf("RenderTemplateWithHooks failed: %v", err)
	}
	expected := "ip=10.0.0.1\nname=NODE1\nsum=3 / name=NODE1-again"
	if out != expected {
		t.Errorf("Unexpected output:\n%s\nexpected:\n%s", out, expected)
	}
}

func TestRenderTemplateWithHooksNil(t *testing.T) {
	out, err := RenderTemplateWithHooks("hello {{ .name }}", map[string]any{"name": "world"}, nil)
	if err != nil {
		t.Fatalf("RenderTemplateWithHooks failed: %v", err)
	}
	if out != "hello world" {
		t.Errorf("Unexpected output: %s", out)
	}
}

func TestRenderTemplateWithHooksIncludeUnknown(t *testing.T) {
	_, err := RenderTemplateWithHooks(`{{ include "missing" $ }}`, map[string]any{}, nil)
	if err == nil {
		t.Fatal("Expected error for unknown include template")
	}
	if !strings.Contains(err.Error(), `include "missing"`) {
		t.Errorf("Expected include error, got: %v", err)
	}
}

func TestGetTemplateWithHooksInvalidHookSyntax(t *testing.T) {
	_, err := GetTemplateWithHooks("ok", false, map[string]string{
		"hook_bad_0": `{{ .ip`,
	})
	if err == nil {
		t.Fatal("Expected parse error for invalid hook template")
	}
	if !strings.Contains(err.Error(), `hook template "hook_bad_0"`) {
		t.Errorf("Expected hook template error, got: %v", err)
	}
}
