// Package manifest reads and writes skills.toml, the declarative source of
// truth for a skilldots library.
package manifest

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/safetext"
)

const FileName = "skills.toml"

// Manifest is the whole skills.toml.
type Manifest struct {
	Targets Targets  `toml:"targets"`
	Packs   []*Pack  `toml:"pack,omitempty"`
	Apps    []*App   `toml:"app,omitempty"`
	Locals  []*Local `toml:"local,omitempty"`
}

// Targets configures where skills get linked.
type Targets struct {
	// Default is the target set used when nothing more specific is given.
	Default []string `toml:"default"`
	// Disabled suspends distribution while preserving target selections.
	Disabled []string `toml:"disabled,omitempty"`
	// Custom adds user-defined targets on top of the built-in registry.
	Custom map[string]CustomTarget `toml:"custom,omitempty"`
}

type CustomTarget struct {
	Path string `toml:"path"`
	// Mode is "link" (default) or "copy".
	Mode string `toml:"mode,omitempty"`
}

// Pack is one upstream repository and the skills selected from it.
type Pack struct {
	Repo  string `toml:"repo"`
	Ref   string `toml:"ref,omitempty"`
	Alias string `toml:"alias,omitempty"`
	// Use lists selected skills by name, or ["*"] for everything.
	Use []string `toml:"use"`
	// Skip excludes skills (name or category/* glob) when Use is "*".
	Skip    []string  `toml:"skip,omitempty"`
	Targets *[]string `toml:"targets,omitempty"`
	// Skill holds per-skill overrides keyed by upstream skill name.
	Skill map[string]*SkillOverride `toml:"skill,omitempty"`
}

// SkillOverride tunes one skill inside a pack.
type SkillOverride struct {
	// As renames the link in target directories (used for conflicts).
	As      string    `toml:"as,omitempty"`
	Targets *[]string `toml:"targets,omitempty"`
}

// App is a skill shipped inside an installed application; linked, never copied.
type App struct {
	Name    string    `toml:"name"`
	Path    string    `toml:"path"`
	Targets *[]string `toml:"targets,omitempty"`
}

// Local overrides a skill under skills/ or private/. Directories there are
// managed automatically; an entry is only needed to change targets.
type Local struct {
	Name    string    `toml:"name"`
	Targets *[]string `toml:"targets,omitempty"`
}

// Load reads path. A missing file is an error; use New for a fresh library.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := toml.Unmarshal(b, m); err != nil {
		return nil, i18n.Errorf("parse %s: %w", path, err)
	}
	if m.Targets.Custom == nil {
		m.Targets.Custom = map[string]CustomTarget{}
	}
	if err := checkText(b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// known are the places of the manifest skd reads: names, paths and
// settings that commands print or pass on.
var known = []string{
	"targets.default", "targets.disabled", "targets.custom.*", "targets.custom.*.path", "targets.custom.*.mode",
	"pack.repo", "pack.ref", "pack.alias", "pack.use", "pack.skip", "pack.targets",
	"pack.skill.*", "pack.skill.*.as", "pack.skill.*.targets",
	"app.name", "app.path", "app.targets", "local.name", "local.targets",
}

// checkText refuses a manifest that holds control characters, bidi
// controls or invalid UTF-8 in a place skd reads (known: repo, ref,
// alias, use, skip, targets, skill overrides and their as, app and
// local entries, custom targets with their path and mode, and the keys
// naming them). Nothing skd writes has them there, so the file was
// written to attack a terminal. Tables and keys this version does not
// know are refused only for what could never be meant as text (an
// escape, BEL, DEL, C1, a bidi control, invalid UTF-8): a multi-line
// string with tabs is someone's note, not an attack. The error names
// each place, quoted so it prints safely.
func checkText(b []byte) error {
	raw := map[string]any{}
	if err := toml.Unmarshal(b, &raw); err != nil {
		return err
	}
	var errs []error
	for _, p := range safetext.DocSchema(raw, known) {
		errs = append(errs, i18n.Errorf("%s has control characters; fix it by hand", p))
	}
	return errors.Join(errs...)
}

// New returns a manifest with sensible defaults.
func New(defaultTargets []string) *Manifest {
	return &Manifest{Targets: Targets{Default: defaultTargets, Custom: map[string]CustomTarget{}}}
}

// Save writes the manifest with stable ordering.
func (m *Manifest) Save(path string) error {
	sort.SliceStable(m.Packs, func(i, j int) bool { return m.Packs[i].Repo < m.Packs[j].Repo })
	sort.SliceStable(m.Apps, func(i, j int) bool { return m.Apps[i].Name < m.Apps[j].Name })
	sort.SliceStable(m.Locals, func(i, j int) bool { return m.Locals[i].Name < m.Locals[j].Name })
	b, err := toml.Marshal(m)
	if err != nil {
		return err
	}
	// Never write a file Load would refuse: a name with control
	// characters that came in some other way (an option, an old file)
	// is an error now, not a library that no longer opens.
	if err := checkText(b); err != nil {
		return i18n.Errorf("not saving %s: %w", FileName, err)
	}
	header := "# skilldots manifest — edit by hand or with `skd`.\n# Docs: https://github.com/Maybe404/skilldots\n\n"
	return fsutil.WriteAtomic(path, []byte(header+string(b)))
}

// FindPack returns the pack matching repo or alias. An exact repo match
// wins, so an alias that spells another pack's repo cannot shadow it.
func (m *Manifest) FindPack(key string) *Pack {
	for _, p := range m.Packs {
		if p.Repo == key {
			return p
		}
	}
	for _, p := range m.Packs {
		if p.Repo == key || (p.Alias != "" && p.Alias == key) || strings.EqualFold(p.DisplayName(), key) {
			return p
		}
	}
	return nil
}

// CheckAlias reports why alias cannot name the pack whose repo is self
// ("" for a pack not subscribed yet): it may not contain "/" (it would
// read as a repo) or ":" (it would split a qualified "<pack>:<skill>"
// key), nor spell a subscribed repo or another pack's alias.
func (m *Manifest) CheckAlias(alias, self string) error {
	if alias == "" {
		return nil
	}
	if safetext.Has(alias) {
		return i18n.Errorf("--alias %q has control characters; pick a short name such as \"acme\"", alias)
	}
	if strings.ContainsAny(alias, `/\:`) {
		return i18n.Errorf(`--alias %q may not contain "/", "\" or ":"; pick a short name such as "acme"`, alias)
	}
	for _, p := range m.Packs {
		if strings.EqualFold(p.Repo, alias) {
			return i18n.Errorf("--alias %q is the repository of a subscribed pack; pick another", alias)
		}
		if p.Repo != self && p.Alias != "" && strings.EqualFold(p.Alias, alias) {
			return i18n.Errorf("--alias %q is already the alias of %s; pick another", alias, p.Repo)
		}
	}
	return nil
}

// DisplayName is the alias when set, else the repo.
func (p *Pack) DisplayName() string {
	if p.Alias != "" {
		return p.Alias
	}
	return p.Repo
}

// Prefix is the string used to namespace conflicting skill names: the
// alias, else the last segment of the repo ("owner/repo" → "repo",
// "gitlab.com/g/sub/repo" → "repo", "/path/to/repo.git" → "repo"),
// reduced to characters valid in a skill name.
func (p *Pack) Prefix() string {
	base := p.Alias
	if base == "" {
		r := strings.TrimRight(strings.ReplaceAll(p.Repo, `\`, "/"), "/")
		base = strings.TrimSuffix(path.Base(r), ".git")
	}
	var b strings.Builder
	for _, c := range base {
		if c < 0x80 && (c == '-' || c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-")
	if CheckName(out) != nil {
		return "pack"
	}
	return out
}

// UsesAll reports whether the pack follows every upstream skill.
func (p *Pack) UsesAll() bool {
	return len(p.Use) == 1 && p.Use[0] == "*"
}

// Override returns the per-skill override, creating it when create is true.
func (p *Pack) Override(name string, create bool) *SkillOverride {
	if p.Skill == nil {
		p.Skill = map[string]*SkillOverride{}
	}
	o := p.Skill[name]
	if o == nil && create {
		o = &SkillOverride{}
		p.Skill[name] = o
	}
	return o
}

// Selected reports whether a skill (by name and category path) is chosen.
func (p *Pack) Selected(name, category string) bool {
	if p.UsesAll() {
		for _, s := range p.Skip {
			if matchSkip(s, name, category) {
				return false
			}
		}
		return true
	}
	for _, u := range p.Use {
		if UseNames(u, name) {
			return true
		}
	}
	return false
}

// UseNames reports whether a `use` entry ("name" or "category/name")
// selects the skill called name. Names are unique within a pack, so the
// category part only documents where the skill lives; it is not required
// to match, and a skill that moves to another category stays selected.
func UseNames(u, name string) bool {
	return u == name || strings.HasSuffix(u, "/"+name)
}

// SkipPattern returns the skip entry that excludes a skill of a pack
// following "*", or "" when none does.
func (p *Pack) SkipPattern(name, category string) string {
	if !p.UsesAll() {
		return ""
	}
	for _, s := range p.Skip {
		if matchSkip(s, name, category) {
			return s
		}
	}
	return ""
}

func matchSkip(pat, name, category string) bool {
	if pat == name {
		return true
	}
	if strings.HasSuffix(pat, "/*") {
		return category == strings.TrimSuffix(pat, "/*") || strings.HasPrefix(category, strings.TrimSuffix(pat, "/*")+"/")
	}
	return pat == category+"/"+name
}

// ErrNotFound is returned by lookups that miss.
var ErrNotFound = i18n.New("not found")
