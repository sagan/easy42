package compiler

import (
	"fmt"
)

// hookTemplatesKey is the reserved template context key under which the
// Build*Context functions store hook snippets destined for registration as
// named templates. It is removed from the context before rendering (see
// popHookTemplates) so it never leaks into template data.
const hookTemplatesKey = "__hook_templates__"

// addHookTemplate stores a hook snippet for registration as a named template
// and returns the template name that renders it. The main config templates
// invoke hooks with the "include" template function:
//
//	{{- range .nft_hooks_pre }}
//	{{ include . $ }}
//	{{- end }}
func addHookTemplate(ctx map[string]any, category, content string) string {
	hooks, _ := ctx[hookTemplatesKey].(map[string]string)
	if hooks == nil {
		hooks = make(map[string]string)
		ctx[hookTemplatesKey] = hooks
	}
	name := fmt.Sprintf("hook_%s_%d", category, len(hooks))
	hooks[name] = content
	return name
}

// popHookTemplates extracts (and removes) the hook templates from the context
// so they can be registered on the Go template before rendering.
func popHookTemplates(ctx map[string]any) map[string]string {
	hooks, _ := ctx[hookTemplatesKey].(map[string]string)
	delete(ctx, hookTemplatesKey)
	return hooks
}
