package tplutil

import (
	"bytes"
	"fmt"
	"log"
	"maps"
	"os"
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
// If tpl starts with "@" char, treat it (the rest part after @) as a file name
// and read template contents from it instead.
func GetTemplate(tpl string, strict bool) (*Template, error) {
	return GetTemplateWithFuncs(tpl, strict, nil)
}

// GetTemplateWithFuncs parses a template and registers extra dynamic functions before compilation.
func GetTemplateWithFuncs(tpl string, strict bool, extraFuncs map[string]any) (*Template, error) {
	if strings.HasPrefix(tpl, "@") {
		contents, err := os.ReadFile(tpl[1:])
		if err != nil {
			return nil, err
		}
		tpl = string(contents)
	}
	templateInstance := template.New("template").Funcs(templateFuncs)
	if extraFuncs != nil {
		templateInstance = templateInstance.Funcs(extraFuncs)
	}
	if strict {
		templateInstance = templateInstance.Option("missingkey=error")
	}
	t, err := templateInstance.Parse(tpl)
	var jsvm *goja.Runtime
	if err != nil {
		if strings.Contains(err.Error(), ` function "eval" not defined`) {
			jsvm = goja.New()
			new(require.Registry).Enable(jsvm)
			console.Enable(jsvm)
			evalFuncMap := template.FuncMap{
				"eval": func(input any) any {
					v, e := Eval(jsvm, input)
					if e != nil {
						log.Printf("eval error: %v", e)
					}
					return v
				},
			}
			templateInstance.Funcs(evalFuncMap)
			t, err = templateInstance.Parse(tpl)
		}
	}
	if err != nil {
		return nil, err
	}
	return &Template{Template: t, jsvm: jsvm}, nil
}

func RenderTemplate(tplStr string, data any) (string, error) {
	tpl, err := GetTemplate(tplStr, false)
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
