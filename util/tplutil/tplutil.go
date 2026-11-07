package tplutil

import (
	"bytes"
	"fmt"
	"log"
	"maps"
	"strings"
	"sync"
	"text/template"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/all"
)

var handler *sprout.DefaultHandler

// sprout provided template funcs
var templateFuncs map[string]any

func init() {
	handler = sprout.New()
	handler.AddGroups(all.RegistryGroup())
	templateFuncs = handler.Build()
}

// Simple wrapper on Go text template.Template.
// Add JavaScript exection (eval) ability.
type Template struct {
	*template.Template
	jsvm *goja.Runtime
	mu   sync.Mutex
}

// Execute Go text template and return rendered string.
// It supports a special "eval" function.
// The result string is trim spaced.
func (t *Template) Exec(data any) (string, error) {
	var buf bytes.Buffer
	if t.jsvm != nil && data != nil {
		t.mu.Lock()
		// allow data sharing between Go text template runtime and JavaScript runtime
		if m, ok := data.(map[string]any); ok {
			data = maps.Clone(m)
		} else if m, ok := data.(map[string]string); ok {
			newdata := map[string]any{}
			for k, v := range m {
				newdata[k] = v
			}
			data = newdata
		}
		t.jsvm.Set("global", data)
		defer t.mu.Unlock()
	}
	if err := t.Template.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// Get a Go text template instance from tpl string.
func GetTemplate(tpl string, strict bool) (*Template, error) {
	return GetTemplateWithFuncs(tpl, strict, nil)
}

// GetTemplateWithFuncs parses a template and registers extra dynamic functions before compilation.
func GetTemplateWithFuncs(tpl string, strict bool, extraFuncs map[string]any) (*Template, error) {
	return getTemplate(tpl, strict, nil, extraFuncs)
}

// GetTemplateWithHooks parses a template and registers hook snippets as
// associated named templates on the same template tree (sharing the same
// function map and options). Hooks can be invoked from the main template or
// from other hooks with the "include" function: {{ include "hook_name" $ }}.
func GetTemplateWithHooks(tpl string, strict bool, hooks map[string]string) (*Template, error) {
	return getTemplate(tpl, strict, hooks, nil)
}

// getTemplate is the core template builder. It parses the main template, then
// registers each hook as an associated named template. Both share the sprout
// function map, extraFuncs, an "include" function, and an on-demand "eval"
// function backed by a JavaScript runtime.
func getTemplate(tpl string, strict bool, hooks map[string]string, extraFuncs map[string]any) (*Template, error) {
	var root *template.Template
	funcs := template.FuncMap{
		"include": func(name string, data any) (string, error) {
			var buf bytes.Buffer
			if err := root.ExecuteTemplate(&buf, name, data); err != nil {
				return "", fmt.Errorf("include %q: %w", name, err)
			}
			return buf.String(), nil
		},
	}
	for name, fn := range extraFuncs {
		funcs[name] = fn
	}
	templateInstance := template.New("template").Funcs(templateFuncs).Funcs(funcs)
	if strict {
		templateInstance = templateInstance.Option("missingkey=error")
	}

	var jsvm *goja.Runtime
	// parseInto parses src into dst. If the source uses the "eval" function
	// which is registered on demand, the JavaScript runtime is created lazily
	// and the source is re-parsed with the "eval" function available.
	parseInto := func(dst *template.Template, src string) error {
		_, err := dst.Parse(src)
		if err != nil && strings.Contains(err.Error(), ` function "eval" not defined`) {
			if jsvm == nil {
				jsvm = goja.New()
				new(require.Registry).Enable(jsvm)
				console.Enable(jsvm)
				templateInstance.Funcs(template.FuncMap{
					"eval": func(input any) any {
						v, e := Eval(jsvm, input)
						if e != nil {
							log.Printf("eval error: %v", e)
						}
						return v
					},
				})
			}
			_, err = dst.Parse(src)
		}
		return err
	}

	if err := parseInto(templateInstance, tpl); err != nil {
		return nil, err
	}
	root = templateInstance
	for name, content := range hooks {
		if err := parseInto(templateInstance.New(name), content); err != nil {
			return nil, fmt.Errorf("hook template %q: %w", name, err)
		}
	}
	return &Template{Template: templateInstance, jsvm: jsvm}, nil
}

func RenderTemplate(tplStr string, data any) (string, error) {
	tpl, err := GetTemplate(tplStr, false)
	if err != nil {
		return "", err
	}
	return tpl.Exec(data)
}

// RenderTemplateWithHooks renders a template with hook snippets registered as
// associated named templates (invokable via the "include" function).
func RenderTemplateWithHooks(tplStr string, data any, hooks map[string]string) (string, error) {
	tpl, err := GetTemplateWithHooks(tplStr, false, hooks)
	if err != nil {
		return "", err
	}
	return tpl.Exec(data)
}

// Convert input to string.
// If input is nil, return empty string.
// If input is string or []byte, return as it.
// If input is a goja Promise, use it's resolved value.
// Otherwise return fmt.Sprint(input).
func ToString(input any) string {
	if input == nil {
		return ""
	}
	if gp, ok := input.(*goja.Promise); ok {
		input, _ = ResolveGojaPromise(gp)
		if input == nil {
			return ""
		}
	}
	switch value := input.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(input)
	}
}

func ResolveGojaPromise(p *goja.Promise) (any, error) {
	switch p.State() {
	case goja.PromiseStateRejected:
		return nil, fmt.Errorf("promise rejected: %v", p.Result().Export())
	case goja.PromiseStateFulfilled:
		return p.Result().Export(), nil
	default:
		return nil, fmt.Errorf("invalid promise")
	}
}

func Eval(vm *goja.Runtime, input any) (any, error) {
	value, err := vm.RunString(ToString(input))
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	v := value.Export()
	if v == nil {
		return nil, nil
	}
	if p, ok := v.(*goja.Promise); ok {
		return ResolveGojaPromise(p)
	}
	return v, nil
}
