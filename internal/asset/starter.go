package asset

import (
	"bytes"
	"embed"
	"text/template"

	"github.com/hexworks/agentfiles/internal/config"
	"github.com/hexworks/agentfiles/internal/errs"
)

// starterTemplates holds the embedded starter bodies seeded into a freshly
// scaffolded asset. Each file is named "<base>.tmpl" and executed against the
// Manifest (see renderStarter). Shipping them inside the binary keeps the
// scaffolding self-contained — no external files to locate at runtime.
//
//go:embed templates
var starterTemplates embed.FS

// starterTmpl is the pre-parsed set of every embedded starter template, keyed
// by base filename (e.g. "skill.md.tmpl"). Parsed once at init: a parse failure
// is a build-time bug in the embedded files, so template.Must is appropriate.
var starterTmpl = template.Must(template.ParseFS(starterTemplates, "templates/*.tmpl"))

// starter describes how a scaffolded asset of a Type seeds its initial content:
// the filename written into the asset directory and the embedded template that
// produces its body.
type starter struct {
	filename string // written into the asset dir (a config.*StarterFileName)
	template string // base name inside templates/, e.g. "skill.md.tmpl"
}

// starters is the single source of truth for the per-Type starter contract:
// Type -> (starter filename, template). Both writeStarter and
// RequiredContentFile derive from it, so the written filename and the
// required-content filename can never drift apart. The generic types
// (TypeMCP, TypeRule, TypeHook) have no entry: their content is user-authored,
// so they emit no starter file.
var starters = map[Type]starter{
	TypeSkill:     {config.SkillStarterFileName, "skill.md.tmpl"},
	TypeAgentsDoc: {config.AgentsDocStarterFileName, "agents_doc.md.tmpl"},
	TypeSettings:  {config.SettingsStarterFileName, "settings.toml.tmpl"},
}

// renderStarter executes s's template against manifest and returns the starter
// body bytes. A template-execution failure is wrapped in StarterRenderError so
// the caller can report which type failed.
func renderStarter(s starter, manifest Manifest) ([]byte, errs.DomainError) {
	var buf bytes.Buffer
	if err := starterTmpl.ExecuteTemplate(&buf, s.template, manifest); err != nil {
		return nil, StarterRenderError{Type: manifest.Type, Err: err}
	}
	return buf.Bytes(), nil
}
