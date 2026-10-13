// Package targets knows where each agent tool reads skills from.
package targets

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Target is one agent skill directory.
type Target struct {
	Name string
	Path string
	// Mode is "link" or "copy".
	Mode string
	// Builtin is false for targets declared in skills.toml.
	Builtin bool
}

// builtin lists known agents by their global skill directory relative to $HOME.
// "agents" is the cross-agent convention many tools read; it comes first.
var builtin = []struct{ name, rel string }{
	{"agents", ".agents/skills"},
	{"claude", ".claude/skills"},
	{"codex", ".codex/skills"},
	{"gemini", ".gemini/skills"},
	{"cursor", ".cursor/skills"},
	{"opencode", ".config/opencode/skills"},
	{"copilot", ".copilot/skills"},
	{"windsurf", ".codeium/windsurf/skills"},
	{"grok", ".grok/skills"},
	{"kiro", ".kiro/skills"},
	{"qwen", ".qwen/skills"},
	{"trae", ".trae/skills"},
	{"goose", ".config/goose/skills"},
	{"antigravity", ".gemini/antigravity/skills"},
	{"roo", ".roo/skills"},
	{"kilocode", ".kilocode/skills"},
	{"continue", ".continue/skills"},
	{"augment", ".augment/skills"},
	{"junie", ".junie/skills"},
	{"iflow", ".iflow/skills"},
	{"qoder", ".qoder/skills"},
	{"codebuddy", ".codebuddy/skills"},
	{"openhands", ".openhands/skills"},
	{"crush", ".config/crush/skills"},
	{"factory", ".factory/skills"},
	{"pi", ".pi/agent/skills"},
}

// Registry resolves target names to directories.
type Registry struct {
	home    string
	targets map[string]Target
}

// NewRegistry builds the registry from built-ins plus custom entries
// (name -> path, name -> mode).
func NewRegistry(custom map[string]struct{ Path, Mode string }) (*Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	r := &Registry{home: home, targets: map[string]Target{}}
	for _, b := range builtin {
		r.targets[b.name] = Target{Name: b.name, Path: filepath.Join(home, filepath.FromSlash(b.rel)), Mode: "link", Builtin: true}
	}
	for name, c := range custom {
		mode := c.Mode
		if mode == "" {
			mode = "link"
		}
		r.targets[name] = Target{Name: name, Path: Expand(c.Path, home), Mode: mode}
	}
	return r, nil
}

// Get resolves one target name.
func (r *Registry) Get(name string) (Target, error) {
	t, ok := r.targets[name]
	if !ok {
		return Target{}, fmt.Errorf("unknown target %q (see `skd targets`)", name)
	}
	return t, nil
}

// Resolve resolves many names, failing on the first unknown.
func (r *Registry) Resolve(names []string) ([]Target, error) {
	out := make([]Target, 0, len(names))
	for _, n := range names {
		t, err := r.Get(n)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// All returns every target sorted by name.
func (r *Registry) All() []Target {
	out := make([]Target, 0, len(r.targets))
	for _, t := range r.targets {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Present returns targets whose directory (or its parent agent dir) exists,
// i.e. agents that look installed on this machine.
func (r *Registry) Present() []Target {
	var out []Target
	for _, t := range r.All() {
		if _, err := os.Stat(t.Path); err == nil {
			out = append(out, t)
			continue
		}
		if _, err := os.Stat(filepath.Dir(t.Path)); err == nil {
			out = append(out, t)
		}
	}
	return out
}

// Expand replaces a leading ~ with home.
func Expand(p, home string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(p, `~\`)) {
		return filepath.Join(home, p[1:])
	}
	return filepath.Clean(p)
}
