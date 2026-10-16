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

// Registry resolves target names to directories. It holds the built-in
// targets and the custom targets of skills.toml that may be used on this
// machine: approved by a person (Approve) and in an allowed place
// (CheckPath). The other custom targets are withheld: unknown to Get,
// All and Present, as if skills.toml did not have them.
type Registry struct {
	home     string
	targets  map[string]Target
	withheld map[string]Withheld
}

// Withheld is a custom target of skills.toml that skd does not use.
type Withheld struct {
	Name string `json:"name"`
	// Path is as skills.toml has it; Real is where that is, ~ expanded
	// and symbolic links resolved ("" when it is not a usable path).
	Path string `json:"path"`
	Real string `json:"real,omitempty"`
	Mode string `json:"mode"`
	// Approvable is set when only a person's approval is missing
	// (`skd targets trust <name>` at a terminal).
	Approvable bool `json:"approvable"`
	// Problem says why it is not used.
	Problem string `json:"problem"`
}

// NewRegistry builds the registry from built-ins plus custom entries
// (name -> path, name -> mode). A custom entry is used only when its name
// is not a built-in one, its path passes CheckPath and a person approved
// that name with that directory on this machine (Approve); the others are
// withheld (Withheld), never an error: one bad entry does not stop the
// library.
func NewRegistry(custom map[string]struct{ Path, Mode string }) (*Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	r := &Registry{home: home, targets: map[string]Target{}, withheld: map[string]Withheld{}}
	for _, b := range builtin {
		r.targets[b.name] = Target{Name: b.name, Path: filepath.Join(home, filepath.FromSlash(b.rel)), Mode: "link", Builtin: true}
	}
	var approved []Approval
	var trustErr error
	if len(custom) > 0 {
		approved, trustErr = Approvals()
	}
	for name, c := range custom {
		mode := c.Mode
		if mode == "" {
			mode = "link"
		}
		w := Withheld{Name: name, Path: c.Path, Mode: mode}
		real, err := inspect(name, c.Path, home)
		w.Real = real
		switch {
		case err != nil:
			w.Problem = err.Error()
		case trustErr != nil:
			w.Problem = fmt.Sprintf("the approvals file cannot be read (%v); fix or remove it, then approve again", trustErr)
		case !isApproved(approved, name, real):
			w.Approvable = true
			w.Problem = fmt.Sprintf("it is not approved on this machine; a person approves it at a terminal with `skd targets trust %s`", name)
		default:
			r.targets[name] = Target{Name: name, Path: Expand(c.Path, home), Mode: mode}
			continue
		}
		r.withheld[name] = w
	}
	return r, nil
}

// Withheld lists the custom targets not in use, sorted by name.
func (r *Registry) Withheld() []Withheld {
	out := make([]Withheld, 0, len(r.withheld))
	for _, w := range r.withheld {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// WithheldTarget is the withheld custom target named name.
func (r *Registry) WithheldTarget(name string) (Withheld, bool) {
	w, ok := r.withheld[name]
	return w, ok
}

// Get resolves one target name.
func (r *Registry) Get(name string) (Target, error) {
	t, ok := r.targets[name]
	if w, held := r.withheld[name]; held && (!ok || !t.Builtin) {
		return Target{}, fmt.Errorf("custom target %q is not used: %s", name, w.Problem)
	}
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
