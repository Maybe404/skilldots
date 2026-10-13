// Package engine turns the manifest plus library contents into a desired
// state, diffs it against the lock and the file system, and applies it.
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	// Missing is set when the source dir is absent (e.g. not vendored yet).
	Missing bool
}

// Resolve computes every entry declared by the library.
func Resolve(lib *library.Library, reg *targets.Registry) ([]Entry, error) {
	m := lib.Manifest
	var out []Entry

	for _, p := range m.Packs {
		packTargets := inherit(m.Targets.Default, p.Targets)
		dir := lib.PackDir(p.Repo)
		lp := lib.Lock.Packs[p.Repo]
		if lp == nil {
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
			e := Entry{Name: name, LinkName: name, Kind: KindVendor, Origin: p.Repo, Category: ls.Category,
				Dir: filepath.Join(dir, filepath.FromSlash(ls.Category), name), TargetNames: packTargets}
			if o := p.Override(name, false); o != nil {
				if o.As != "" {
					e.LinkName = o.As
				}
				e.TargetNames = inherit(packTargets, o.Targets)
			}
			out = append(out, e)
		}
	}

	for _, a := range m.Apps {
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

	for i := range out {
		e := &out[i]
		if _, err := os.Stat(e.Dir); err != nil {
			e.Missing = true
		} else if e.Description == "" {
			if meta, err := skillmd.Read(e.Dir); err == nil {
				e.Description = meta.Description
			}
		}
		ts, err := reg.Resolve(e.TargetNames)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.LinkName, err)
		}
		e.Targets = ts
	}

	if err := checkConflicts(out); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LinkName < out[j].LinkName })
	return out, nil
}

func inherit(parent []string, child *[]string) []string {
	if child == nil {
		return parent
	}
	return *child
}

func checkConflicts(entries []Entry) error {
	seen := map[string]Entry{}
	var errs []string
	for _, e := range entries {
		if prev, ok := seen[e.LinkName]; ok {
			errs = append(errs, fmt.Sprintf("%q is provided by both %s and %s", e.LinkName, Describe(prev), Describe(e)))
			continue
		}
		seen[e.LinkName] = e
	}
	if len(errs) > 0 {
		return fmt.Errorf("skill name conflicts (set `as` in skills.toml):\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// Describe names an entry's origin for messages, e.g. "vendor:owner/repo".
func Describe(e Entry) string {
	if e.Origin != "" {
		return string(e.Kind) + ":" + e.Origin
	}
	return string(e.Kind) + "/"
}

// Find returns the entry whose name or link name matches.
func Find(entries []Entry, key string) *Entry {
	for i := range entries {
		if entries[i].LinkName == key || entries[i].Name == key {
			return &entries[i]
		}
	}
	return nil
}

// FreeName returns name if no entry uses it, else prefix-name.
func FreeName(entries []Entry, name, prefix string) string {
	if Find(entries, name) == nil {
		return name
	}
	alt := prefix + "-" + name
	for i := 2; Find(entries, alt) != nil; i++ {
		alt = fmt.Sprintf("%s-%s-%d", prefix, name, i)
	}
	return alt
}

var _ = manifest.ErrNotFound
