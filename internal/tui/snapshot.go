package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Snapshot is everything the dashboard shows, read in one pass. It is a
// value: operations never mutate it, they produce a new one.
type Snapshot struct {
	Root   string
	Name   string // library directory name
	Branch string

	Pending int  // link changes `skd apply` would make
	Dirty   bool // uncommitted changes in the library
	Remote  bool // origin configured
	Tracked bool // the branch has an upstream
	Ahead   int  // commits not pushed (only meaningful when Tracked)

	Defaults []string
	Agents   []targets.Target // agent directories present on this machine

	Packs   []Pack
	Mine    []Skill
	Private []Skill
	Apps    []Skill

	// Warnings are the engine's per-entry problems (name clashes, unknown
	// targets, packs never fetched…). Affected entries are skipped or
	// marked; the dashboard keeps them on screen until they are fixed.
	Warnings []string
}

// Pack is one subscribed repository.
type Pack struct {
	Repo, Alias string
	Commit      string
	When        string // "3 days ago", relative to now
	License     string
	NoLicense   bool
	Targets     []string // nil: manifest default
	UsesAll     bool
	Selected    int
	Total       int  // upstream skills known (from the cache clone, else the lock)
	Upstream    bool // Total comes from a cached upstream scan
	Skills      []Skill
}

// Skill is one row: a library entry, or an upstream skill not installed.
type Skill struct {
	Name, LinkName string
	Kind           engine.Kind
	Origin         string
	Category       string
	Description    string
	Dir            string
	Installed      bool
	Missing        bool
	Conflict       string   // set when another entry wants the same link name
	Agents         []string // targets it is linked into (resolved)
}

// Key identifies a row across reloads.
func (s Skill) Key() string { return string(s.Kind) + ":" + s.Origin + ":" + s.Name }

// Enabled reports whether the skill is linked anywhere.
func (s Skill) Enabled() bool { return len(s.Agents) > 0 }

// openLibrary loads the library and the target registry the same way the
// CLI does.
func openLibrary(root string) (*library.Library, *targets.Registry, error) {
	lib, err := library.Open(root)
	if err != nil {
		return nil, nil, err
	}
	custom := map[string]struct{ Path, Mode string }{}
	for n, c := range lib.Manifest.Targets.Custom {
		custom[n] = struct{ Path, Mode string }{c.Path, c.Mode}
	}
	reg, err := targets.NewRegistry(custom)
	if err != nil {
		return nil, nil, err
	}
	engine.SetRegistry(reg)
	return lib, reg, nil
}

// LoadSnapshot reads the library at root. It never writes and never
// touches the network: upstream skill lists come from the cache clones
// left by earlier fetches.
func LoadSnapshot(root string) (*Snapshot, error) {
	lib, reg, err := openLibrary(root)
	if err != nil {
		return nil, err
	}
	m := lib.Manifest
	s := &Snapshot{Root: root, Name: filepath.Base(root), Defaults: append([]string(nil), m.Targets.Default...),
		Agents: reg.Present()}

	entries, warnings, err := engine.Resolve(lib, reg)
	s.Warnings = warnings
	if err != nil {
		s.Warnings = append(s.Warnings, err.Error())
	} else {
		s.Pending = pendingChanges(engine.MakePlan(lib, entries, false))
	}
	gitStatus(lib, s)

	byPack := map[string][]engine.Entry{}
	for _, e := range entries {
		switch e.Kind {
		case engine.KindVendor:
			byPack[e.Origin] = append(byPack[e.Origin], e)
		case engine.KindSkill:
			s.Mine = append(s.Mine, toSkill(e))
		case engine.KindPrivate:
			s.Private = append(s.Private, toSkill(e))
		case engine.KindApp:
			s.Apps = append(s.Apps, toSkill(e))
		}
	}
	for _, p := range m.Packs {
		lp := lib.Lock.Packs[p.Repo]
		pk := Pack{Repo: p.Repo, Alias: p.Alias, UsesAll: p.UsesAll()}
		if p.Targets != nil {
			pk.Targets = append([]string{}, *p.Targets...)
		}
		installed := map[string]bool{}
		for _, e := range byPack[p.Repo] {
			pk.Skills = append(pk.Skills, toSkill(e))
			installed[e.Name] = true
		}
		pk.Selected = len(pk.Skills)
		var rest []Skill
		cache := pack.CachePath(p.Repo)
		if fsutil.Exists(filepath.Join(cache, ".git")) {
			if up, _, err := pack.Scan(cache); err == nil {
				pk.Upstream = true
				for _, u := range up {
					if installed[u.Name] {
						continue
					}
					rest = append(rest, Skill{Name: u.Name, LinkName: u.Name, Kind: engine.KindVendor, Origin: p.Repo,
						Category: u.Category, Description: u.Description, Dir: u.Dir})
				}
			}
		}
		sort.SliceStable(rest, func(i, j int) bool {
			if rest[i].Category != rest[j].Category {
				return rest[i].Category < rest[j].Category
			}
			return rest[i].Name < rest[j].Name
		})
		pk.Skills = append(pk.Skills, rest...)
		pk.Total = len(pk.Skills)
		if !pk.Upstream && lp != nil && len(lp.Skills) > pk.Total {
			pk.Total = len(lp.Skills)
		}
		packMeta(lib, lp, &pk)
		s.Packs = append(s.Packs, pk)
	}
	return s, nil
}

func toSkill(e engine.Entry) Skill {
	s := Skill{Name: e.Name, LinkName: e.LinkName, Kind: e.Kind, Origin: e.Origin, Category: e.Category,
		Description: e.Description, Dir: e.Dir, Installed: true, Missing: e.Missing, Conflict: e.Conflict}
	for _, t := range e.Targets {
		s.Agents = append(s.Agents, t.Name)
	}
	return s
}

func pendingChanges(p engine.Plan) int {
	n := 0
	for _, c := range p.ChangeList() {
		if c.Action != "skip" {
			n++
		}
	}
	return n
}

// gitStatus fills branch, dirty and push state. Every git failure (no
// commits yet, no remote) just leaves the zero value.
func gitStatus(lib *library.Library, s *Snapshot) {
	if b, err := lib.Git("symbolic-ref", "--short", "HEAD"); err == nil {
		s.Branch = b
	} else if b, err := lib.Git("rev-parse", "--short", "HEAD"); err == nil {
		s.Branch = b
	}
	if out, err := lib.Git("status", "--porcelain"); err == nil && out != "" {
		s.Dirty = true
	}
	if out, err := lib.Git("remote"); err == nil && out != "" {
		s.Remote = true
	}
	if _, err := lib.Git("rev-parse", "--abbrev-ref", "@{u}"); err == nil {
		s.Tracked = true
		if out, err := lib.Git("rev-list", "--count", "@{u}..HEAD"); err == nil {
			s.Ahead, _ = strconv.Atoi(out)
		}
	}
}

// packMeta fills commit, age and license for a pack.
func packMeta(lib *library.Library, lp *lock.Pack, pk *Pack) {
	if lp != nil {
		pk.Commit = lp.Commit
	}
	cache := pack.CachePath(pk.Repo)
	// Unix times, formatted here: git's own %cr follows the user's locale
	// and would mix languages in the dashboard.
	if pk.Commit != "" && fsutil.Exists(filepath.Join(cache, ".git")) {
		if out, err := library.Git(cache, "log", "-1", "--format=%ct", pk.Commit); err == nil {
			pk.When = ago(out, time.Now())
		}
	}
	if pk.When == "" {
		if out, err := lib.Git("log", "-1", "--format=%ct", "--", "vendor/"+pk.Repo); err == nil && out != "" {
			pk.When = "vendored " + ago(out, time.Now())
		}
	}
	dir := lib.PackDir(pk.Repo)
	if f := fsutil.FindLicense(dir); f != "" {
		pk.License = licenseName(f)
	} else if f := fsutil.FindLicense(cache); f != "" {
		pk.License = licenseName(f)
	} else {
		pk.NoLicense = true
	}
}

// ago renders a unix timestamp as "3 days ago".
func ago(unix string, now time.Time) string {
	sec, err := strconv.ParseInt(strings.TrimSpace(unix), 10, 64)
	if err != nil {
		return ""
	}
	d := now.Sub(time.Unix(sec, 0))
	unit := func(n int, w string) string {
		if n == 1 {
			return "1 " + w + " ago"
		}
		return strconv.Itoa(n) + " " + w + "s ago"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return unit(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return unit(int(d.Hours()), "hour")
	case d < 30*24*time.Hour:
		return unit(int(d.Hours()/24), "day")
	case d < 365*24*time.Hour:
		return unit(int(d.Hours()/24/30), "month")
	}
	return unit(int(d.Hours()/24/365), "year")
}

// licenseName recognises the common licenses by their text.
func licenseName(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return filepath.Base(path)
	}
	t := strings.ToLower(string(b))
	switch {
	case strings.Contains(t, "mit license") || strings.Contains(t, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "apache license"):
		return "Apache-2.0"
	case strings.Contains(t, "mozilla public license"):
		return "MPL-2.0"
	case strings.Contains(t, "gnu affero general public license"):
		return "AGPL"
	case strings.Contains(t, "gnu lesser general public license"):
		return "LGPL"
	case strings.Contains(t, "gnu general public license"):
		return "GPL"
	case strings.Contains(t, "creative commons") || strings.Contains(t, "cc-by") || strings.Contains(t, "cc by"):
		return "CC"
	case strings.Contains(t, "isc license"):
		return "ISC"
	case strings.Contains(t, "unlicense"):
		return "Unlicense"
	case strings.Contains(t, "redistribution and use in source and binary forms"):
		return "BSD"
	}
	return filepath.Base(path)
}
