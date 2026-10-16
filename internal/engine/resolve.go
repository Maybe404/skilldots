// Package engine turns the manifest plus library contents into a desired
// state, diffs it against the lock and the file system, and applies it.
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/skillmd"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Kind classifies where an entry comes from.
type Kind string

const (
	KindVendor  Kind = "vendor"
	KindSkill   Kind = "skills"
	KindPrivate Kind = "private"
	KindApp     Kind = "app"
)

// Entry is one skill the library wants present somewhere.
type Entry struct {
	// Name is the upstream/original skill name.
	Name string
	// LinkName is the name used inside target directories.
	LinkName string
	Kind     Kind
	// Origin is the pack repo for vendor entries, the app name for apps,
	// else "".
	Origin   string
	Category string
	// Dir is the absolute source directory.
	Dir         string
	Description string
	Targets     []targets.Target
	// TargetNames is the resolved target list before registry lookup.
	TargetNames []string
	// UnknownTargets are names in TargetNames the registry does not know.
	// They are skipped; links already recorded for them are left alone.
	UnknownTargets []string `json:",omitempty"`
	// Missing is set when the source dir is absent (e.g. not vendored yet).
	Missing bool
	// LinkedVia is the symbolic link inside the library on the way to Dir,
	// if any: skd neither links nor copies such a skill (its files are
	// wherever the link points, outside the library).
	LinkedVia string `json:",omitempty"`
	// Refused says why skd neither links nor copies an app's skill
	// (CheckAppDir), "" when it does.
	Refused string `json:",omitempty"`
	// Conflict is set when another entry wants the same LinkName. Neither
	// is linked until the clash is resolved; existing links stay as they are.
	Conflict string `json:",omitempty"`
	// ClaudeDisabled is set when the skill is turned off inside Claude Code
	// (skillOverrides in its settings); skd still links it. Filled in by
	// app.Session.Resolve.
	ClaudeDisabled bool
}

// Resolve computes every entry declared by the library. Problems confined
// to one entry (an unknown target, a name clash, an invalid name) do not
// fail the whole library: the entry is skipped or marked, and a
// human-readable warning is returned for each problem.
func Resolve(lib *library.Library, reg *targets.Registry) ([]Entry, []string, error) {
	out, warnings := collect(lib)

	unknown, withheld, linked := map[string][]string{}, map[string][]string{}, map[string][]string{}
	for i := range out {
		e := &out[i]
		if e.Kind != KindApp {
			if l := sourceLink(lib, e.Dir); l != "" {
				e.LinkedVia = l
				linked[l] = append(linked[l], e.LinkName)
			}
		}
		if _, err := os.Stat(e.Dir); err != nil {
			e.Missing = true
		} else if e.Kind == KindApp && CheckAppDir(e.Dir) != nil {
			e.Refused = CheckAppDir(e.Dir).Error()
			warnings = append(warnings, fmt.Sprintf("[[app]] %s: %s; skipped", e.Name, e.Refused))
		} else if e.Description == "" {
			if meta, err := skillmd.Read(e.Dir); err == nil {
				e.Description = meta.Description
			}
		}
		for _, n := range e.TargetNames {
			t, err := reg.Get(n)
			if err != nil {
				e.UnknownTargets = append(e.UnknownTargets, n)
				if _, held := reg.WithheldTarget(n); held {
					withheld[n] = append(withheld[n], e.LinkName)
				} else {
					unknown[n] = append(unknown[n], e.LinkName)
				}
				continue
			}
			e.Targets = append(e.Targets, t)
		}
	}
	for _, l := range sortedKeys(linked) {
		warnings = append(warnings, fmt.Sprintf("%s in the library is a symbolic link; skd does not link or copy skills through it (skipped: %s); make it a directory of its own",
			lib.Rel(l), strings.Join(linked[l], ", ")))
	}
	for _, n := range sortedKeys(unknown) {
		warnings = append(warnings, fmt.Sprintf("unknown target %q (used by %s) is skipped; define it under [targets.custom] in skills.toml or fix the name",
			n, strings.Join(unknown[n], ", ")))
	}
	// A custom target skd does not use (not approved here, in a place it
	// never writes, or named like a built-in one) is said every time,
	// used or not: it may be how someone else's library tried to write
	// outside the agents' directories.
	for _, w := range reg.Withheld() {
		msg := fmt.Sprintf("custom target %q in skills.toml is not used: %s", w.Name, w.Problem)
		if used := withheld[w.Name]; len(used) > 0 {
			msg += fmt.Sprintf(" (skipped for %s)", strings.Join(used, ", "))
		}
		warnings = append(warnings, msg)
	}

	warnings = append(warnings, markConflicts(out)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].LinkName < out[j].LinkName })
	return out, warnings, nil
}

// collect gathers entries from the manifest and the library directories
// without touching the target registry.
func collect(lib *library.Library) ([]Entry, []string) {
	m := lib.Manifest
	var out []Entry
	var warnings []string
	for _, l := range m.Locals {
		if err := manifest.CheckName(l.Name); err != nil {
			warnings = append(warnings, fmt.Sprintf("[[local]] %v; ignored", err))
		}
	}

	for _, p := range m.Packs {
		packTargets := inherit(m.Targets.Default, p.Targets)
		dir, err := PackDir(lib, p.Repo)
		if err != nil {
			warnings = append(warnings, err.Error()+"; skipped")
			continue
		}
		lp := lib.Lock.Packs[p.Repo]
		if lp == nil {
			warnings = append(warnings, fmt.Sprintf("pack %s is in skills.toml but has not been fetched; run `skd update %s`", p.Repo, p.Repo))
			continue
		}
		names := make([]string, 0, len(lp.Skills))
		for n := range lp.Skills {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			ls := lp.Skills[name]
			if !p.Selected(name, ls.Category) {
				continue
			}
			sd, err := skillDir(dir, ls.Category, name)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("pack %s: skill %q in skills.lock: %v; skipped", p.Repo, name, err))
				continue
			}
			e := Entry{Name: name, LinkName: name, Kind: KindVendor, Origin: p.Repo, Category: ls.Category,
				Dir: sd, TargetNames: packTargets}
			if o := p.Override(name, false); o != nil {
				if o.As != "" {
					if err := manifest.CheckName(o.As); err != nil {
						warnings = append(warnings, fmt.Sprintf("pack %s: as for %s: %v; skipped", p.Repo, name, err))
						continue
					}
					e.LinkName = o.As
				}
				e.TargetNames = inherit(packTargets, o.Targets)
			}
			out = append(out, e)
		}
	}

	for _, a := range m.Apps {
		if err := manifest.CheckName(a.Name); err != nil {
			warnings = append(warnings, fmt.Sprintf("[[app]] %v; skipped", err))
			continue
		}
		home, _ := os.UserHomeDir()
		out = append(out, Entry{Name: a.Name, LinkName: a.Name, Kind: KindApp, Origin: a.Name,
			Dir: targets.Expand(a.Path, home), TargetNames: inherit(m.Targets.Default, a.Targets)})
	}

	for _, kind := range []struct {
		kind Kind
		dir  string
	}{{KindSkill, lib.SkillsDir()}, {KindPrivate, lib.PrivateDir()}} {
		entries, _ := os.ReadDir(kind.dir)
		for _, de := range entries {
			if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
				continue
			}
			d := filepath.Join(kind.dir, de.Name())
			if !skillmd.IsSkillDir(d) {
				continue
			}
			meta, _ := skillmd.Read(d)
			if err := manifest.CheckName(meta.Name); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: SKILL.md %v; skipped", lib.Rel(d), err))
				continue
			}
			e := Entry{Name: meta.Name, LinkName: meta.Name, Kind: kind.kind, Dir: d, Description: meta.Description,
				TargetNames: m.Targets.Default}
			for _, l := range m.Locals {
				if l.Name == meta.Name || l.Name == de.Name() {
					e.TargetNames = inherit(m.Targets.Default, l.Targets)
				}
			}
			out = append(out, e)
		}
	}
	return out, warnings
}

func inherit(parent []string, child *[]string) []string {
	if child == nil {
		return parent
	}
	return *child
}

// markConflicts flags every entry whose LinkName another entry also wants.
func markConflicts(entries []Entry) []string {
	// Names that differ only in case clash too: on a case-insensitive
	// file system they are the same directory entry.
	byName := map[string][]int{}
	for i, e := range entries {
		k := strings.ToLower(e.LinkName)
		byName[k] = append(byName[k], i)
	}
	var warnings []string
	for _, name := range sortedKeys(byName) {
		all := byName[name]
		if len(all) < 2 {
			continue
		}
		// Same name is only a clash where both would land in the same
		// target directory; entries linked to disjoint agents coexist.
		var idx []int
		for _, i := range all {
			for _, j := range all {
				if i != j && sharesTarget(entries[i], entries[j]) {
					idx = append(idx, i)
					break
				}
			}
		}
		if len(idx) < 2 {
			continue
		}
		var who []string
		for _, i := range idx {
			who = append(who, fmt.Sprintf("%s (%s)", Describe(entries[i]), entries[i].Dir))
		}
		msg := fmt.Sprintf("%q is provided by %s", entries[idx[0]].LinkName, strings.Join(who, " and "))
		for _, i := range idx {
			entries[i].Conflict = msg
		}
		warnings = append(warnings, msg+"; neither is linked until one is renamed (set `as` under [pack.skill.<name>] in skills.toml, or rename your own skill)")
	}
	return warnings
}

// sharesTarget reports whether two entries link into a common directory.
func sharesTarget(a, b Entry) bool {
	for _, x := range a.Targets {
		for _, y := range b.Targets {
			if x.Name == y.Name || evalClean(x.Path) == evalClean(y.Path) {
				return true
			}
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Describe names an entry's origin for messages, e.g. "vendor:owner/repo".
func Describe(e Entry) string {
	if e.Origin != "" {
		return string(e.Kind) + ":" + e.Origin
	}
	return string(e.Kind) + "/"
}

// Find returns the entry whose name or link name matches. Link names win
// over upstream names so an aliased skill is found under the name shown.
func Find(entries []Entry, key string) *Entry {
	for i := range entries {
		if entries[i].LinkName == key {
			return &entries[i]
		}
	}
	for i := range entries {
		if entries[i].Name == key {
			return &entries[i]
		}
	}
	return nil
}

// FreeName returns name if no entry uses it, else prefix-name.
func FreeName(entries []Entry, name, prefix string) string {
	return freeName(func(n string) bool { return Find(entries, n) != nil }, name, prefix)
}

func freeName(taken func(string) bool, name, prefix string) string {
	if !taken(name) {
		return name
	}
	alt := prefix + "-" + name
	for i := 2; taken(alt); i++ {
		alt = fmt.Sprintf("%s-%s-%d", prefix, name, i)
	}
	return alt
}

// Qualified names an entry unambiguously: "owner/repo:name" for vendored
// skills, "skills:name", "private:name" or "app:name" otherwise.
func Qualified(e Entry) string {
	if e.Kind == KindVendor {
		return e.Origin + ":" + e.LinkName
	}
	return string(e.Kind) + ":" + e.LinkName
}

// ReservedQualifier reports whether q names a kind of local entry in a
// qualified key ("skills:foo"), so it can never stand for a pack alias.
func ReservedQualifier(q string) bool {
	switch Kind(q) {
	case KindSkill, KindPrivate, KindApp, KindVendor:
		return true
	}
	return false
}

// Lookup finds the one entry a user means by key: a link name, an
// upstream name, or a qualified "<owner/repo|skills|private|app>:<name>".
// Link names win over upstream names. When several entries match (same
// name, disjoint targets) it fails and lists the qualified candidates.
// It returns (nil, nil) when nothing matches.
func Lookup(entries []Entry, key string) (*Entry, error) {
	qual, name := "", key
	if i := strings.LastIndex(key, ":"); i > 0 {
		qual, name = key[:i], key[i+1:]
	}
	match := func(e *Entry) bool {
		if qual == "" {
			return true
		}
		if e.Kind == KindVendor {
			return e.Origin == qual
		}
		return string(e.Kind) == qual
	}
	for _, byLink := range []bool{true, false} {
		var hits []*Entry
		for i := range entries {
			e := &entries[i]
			n := e.Name
			if byLink {
				n = e.LinkName
			}
			if n == name && match(e) {
				hits = append(hits, e)
			}
		}
		switch {
		case len(hits) == 1:
			return hits[0], nil
		case len(hits) > 1:
			var c []string
			for _, e := range hits {
				c = append(c, Qualified(*e))
			}
			return nil, fmt.Errorf("%q matches several skills; name one of: %s", key, strings.Join(c, ", "))
		}
	}
	return nil, nil
}

// CheckAppDir reports why skd does not link or copy the app skill at
// dir. skills.toml names an app's directory and may come from someone
// else (skd clone): an [[app]] path = "~/.ssh" would have skd link the
// keys into every agent's directory, or copy them, 0600 files and all,
// into a copy target. So the directory must hold SKILL.md as a regular
// file (not a link: one could lead anywhere), as every skill does, and
// must not be, be inside or hold a place of keys and credentials
// (targets.SecretDirs). Links on the way are resolved first.
func CheckAppDir(dir string) error {
	st, err := os.Lstat(filepath.Join(dir, "SKILL.md"))
	switch {
	case os.IsNotExist(err):
		return fmt.Errorf("%s has no SKILL.md, so it is not a skill", dir)
	case err != nil:
		return err
	case !st.Mode().IsRegular():
		return fmt.Errorf("%s/SKILL.md is not a regular file", dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	real := fsutil.Resolve(dir)
	for _, s := range targets.SecretDirs(home) {
		s = fsutil.Resolve(s)
		if fsutil.Inside(s, real, true) || fsutil.Inside(real, s, true) {
			return fmt.Errorf("%s is, is inside or holds %s, a place of keys and credentials", dir, s)
		}
	}
	return nil
}
