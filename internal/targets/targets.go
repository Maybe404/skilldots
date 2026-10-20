// Package targets knows where each agent tool reads skills from.
package targets

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// Target is one agent skill directory.
type Target struct {
	Name string
	Path string
	// Mode is "link" or "copy".
	Mode string
	// Builtin is false for targets declared in skills.toml.
	Builtin bool
	// Real is where Path led when the registry was built (links
	// resolved): for a custom target, the directory a person approved.
	// Writes into the target check that Path still leads there (Intact).
	Real string `json:"-"`
}

// Intact reports why Path no longer leads to Real, the directory the
// registry found (and, for a custom target, a person approved): a
// symbolic link on the way was made or changed since, possibly by skd
// itself earlier in the same run (a link it created in another target,
// or a link inside a skill it linked). Nothing is written into a target
// that is not intact.
func (t Target) Intact() error {
	if t.Real == "" {
		return i18n.Errorf("the directory of target %q was not checked", t.Name)
	}
	// The same string, not the same file: Real itself may have become a
	// link to elsewhere, and a file comparison would follow it.
	if now := fsutil.Resolve(t.Path); now != t.Real {
		return i18n.Errorf("target %q (%s) now leads to %s, not %s", t.Name, t.Path, now, t.Real)
	}
	return nil
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

// Installed reports whether the agent whose skill directory is path looks
// installed on this machine: the directory, or the one holding it, exists
// (~/.claude for ~/.claude/skills).
func Installed(path string) bool {
	for _, p := range []string{path, filepath.Dir(path)} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// Registry resolves target names to directories. It holds the built-in
// targets and the custom targets of skills.toml that may be used on this
// machine: approved by a person (Approve) and in an allowed place
// (CheckPath). The other custom targets are withheld: unknown to Get,
// All and Present, as if skills.toml did not have them.
type Registry struct {
	home     string
	lib      string
	custom   map[string]struct{ Path, Mode string }
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
// (name -> path, name -> mode) of the library at lib. A custom entry is
// used only when its name is not a built-in one, its path is allowed
// (checkPath: in the home directory, away from sensitive places, the
// library, the built-in targets and the other custom targets), its mode
// is link or copy, and a person approved that name with that directory
// and that mode on this machine (Approve); the others are withheld
// (Withheld), never an error: one bad entry does not stop the library.
func NewRegistry(lib string, custom map[string]struct{ Path, Mode string }) (*Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	r := &Registry{home: home, lib: lib, custom: map[string]struct{ Path, Mode string }{}, targets: map[string]Target{}, withheld: map[string]Withheld{}}
	for _, b := range builtin {
		p := filepath.Join(home, filepath.FromSlash(b.rel))
		r.targets[b.name] = Target{Name: b.name, Path: p, Mode: "link", Builtin: true, Real: fsutil.Resolve(p)}
	}
	for name, c := range custom {
		if c.Mode == "" {
			c.Mode = "link"
		}
		r.custom[name] = c
	}
	var approved []Approval
	var trustErr error
	if len(custom) > 0 {
		approved, trustErr = Approvals()
	}
	for name, c := range r.custom {
		w := Withheld{Name: name, Path: c.Path, Mode: c.Mode}
		real, err := r.inspect(name, approved)
		w.Real = real
		switch {
		case err != nil:
			w.Problem = err.Error()
		case trustErr != nil:
			w.Problem = i18n.Sprintf("the approvals file cannot be read (%v); fix or remove it, then approve again", trustErr)
		case !isApproved(approved, name, real, c.Mode):
			w.Approvable = true
			w.Problem = i18n.Sprintf("it is not approved on this machine (with this directory and mode %s); a person approves it at a terminal with `skd targets trust %s`", c.Mode, name)
		default:
			r.targets[name] = Target{Name: name, Path: Expand(c.Path, home), Mode: c.Mode, Real: real}
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
		return Target{}, i18n.Errorf("custom target %q is not used: %s", name, w.Problem)
	}
	if !ok {
		return Target{}, i18n.Errorf("unknown target %q (see `skd targets`)", name)
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
		if Installed(t.Path) {
			out = append(out, t)
		}
	}
	return out
}

// AllInstalled in a targets list stands for the default targets and every
// built-in agent installed on this machine, worked out on each machine when
// the library is resolved: an agent installed later gets the skill at the
// next apply, and a machine without an agent gets no directory for it.
// An older skd ignores it with a warning, so it is written after the
// default targets, which every skd links.
const AllInstalled = "*"

// ExpandNames replaces AllInstalled in names by defaults and the built-in
// agents among present (Registry.Present, in its order), without
// duplicates. Lists without it are returned as they are.
func ExpandNames(names, defaults []string, present []Target) []string {
	if !slices.Contains(names, AllInstalled) {
		return names
	}
	out := []string{}
	add := func(n string) {
		if n != AllInstalled && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	for _, n := range names {
		if n != AllInstalled {
			add(n)
			continue
		}
		for _, d := range defaults {
			add(d)
		}
		for _, t := range present {
			if t.Builtin {
				add(t.Name)
			}
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
