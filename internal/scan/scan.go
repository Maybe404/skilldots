// Package scan inventories every agent skill directory on the machine and
// works out where each entry came from, so skilldots can take them over.
package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/skillmd"
	"github.com/Maybe404/skilldots/internal/targets"
)

// State classifies one entry in a target directory.
type State string

const (
	Managed   State = "managed"    // link recorded in skills.lock
	Unlinked  State = "unrecorded" // resolves into the library (possibly via other links) but not in the lock
	AppLink   State = "app"        // link into an installed application bundle
	ExtLink   State = "link"       // link somewhere else
	Copy      State = "copy"       // real directory with a SKILL.md
	Broken    State = "broken"     // dangling link
	NotASkill State = "other"      // directory without SKILL.md, or a file
	// Left is a link into the library that skills.lock records for a
	// target no longer in use (removed from skills.toml, not approved,
	// or refused): skd made it and does not manage it any more.
	Left State = "left"
)

// Item is one directory entry in one target.
type Item struct {
	Target string `json:"target"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	State  State  `json:"state"`
	LinkTo string `json:"linkTo,omitempty"`
	Hash   string `json:"hash,omitempty"`
	// Source is the upstream "owner/repo" when another installer recorded it.
	Source     string `json:"source,omitempty"`
	SourcePath string `json:"sourcePath,omitempty"`
	// Duplicate is set when the same name appears elsewhere with different content.
	Duplicate bool `json:"duplicate,omitempty"`
}

// Report is the whole inventory.
type Report struct {
	Items []Item `json:"items"`
}

// Unmanaged returns the items skilldots does not control yet.
func (r Report) Unmanaged() []Item {
	var out []Item
	for _, it := range r.Items {
		switch it.State {
		case Copy, ExtLink, AppLink, Broken, Unlinked:
			out = append(out, it)
		}
	}
	return out
}

// Run inventories every present target.
func Run(lib *library.Library, reg *targets.Registry) (Report, error) {
	npx := readNpxLock()
	var rep Report
	byName := map[string][]int{}
	for _, t := range reg.Present() {
		entries, err := os.ReadDir(t.Path)
		if err != nil {
			continue
		}
		for _, de := range entries {
			name := de.Name()
			if strings.HasPrefix(name, ".") || name == "synced" {
				continue
			}
			p := filepath.Join(t.Path, name)
			it := Item{Target: t.Name, Name: name, Path: p}
			st, err := os.Lstat(p)
			if err != nil {
				continue
			}
			switch {
			case st.Mode()&os.ModeSymlink != 0:
				dest, _ := os.Readlink(p)
				if !filepath.IsAbs(dest) {
					dest = filepath.Join(t.Path, dest)
				}
				dest = filepath.Clean(dest)
				it.LinkTo = dest
				final := dest
				if r, err := filepath.EvalSymlinks(p); err == nil {
					final = r
				}
				libRoot := lib.Root
				if r, err := filepath.EvalSymlinks(lib.Root); err == nil {
					libRoot = r
				}
				switch {
				case !fsutil.Exists(dest):
					it.State = Broken
				case lib.Lock.Links[t.Name+"/"+name] != nil:
					it.State = Managed
				case strings.HasPrefix(final, libRoot+string(filepath.Separator)):
					it.State = Unlinked
				case strings.Contains(dest, ".app"+string(filepath.Separator)):
					it.State = AppLink
				default:
					it.State = ExtLink
				}
			case st.IsDir() && skillmd.IsSkillDir(p):
				it.State = Copy
				if lib.Lock.Links[t.Name+"/"+name] != nil {
					it.State = Managed
				}
				it.Hash, _ = fsutil.HashTree(p)
				if n, ok := npx[name]; ok {
					it.Source, it.SourcePath = n.Source, n.SkillPath
				}
			default:
				it.State = NotASkill
			}
			if it.State == Copy || it.State == ExtLink {
				byName[name] = append(byName[name], len(rep.Items))
			}
			rep.Items = append(rep.Items, it)
		}
	}
	// Links left in the directories of targets no longer in use. The
	// lock may be forged, so only a record whose path is a link into the
	// library, as skd makes them, is listed.
	for _, key := range lib.Lock.LinkKeys() {
		tname, name, _ := strings.Cut(key, "/")
		if _, err := reg.Get(tname); err == nil {
			continue
		}
		prev := lib.Lock.Links[key]
		if !engine.LeftLink(prev, lib) {
			continue
		}
		dest, _ := os.Readlink(prev.Path)
		rep.Items = append(rep.Items, Item{Target: tname, Name: name, Path: prev.Path, State: Left, LinkTo: dest})
	}
	// Same name, different content in two places: flag both.
	for _, idx := range byName {
		if len(idx) < 2 {
			continue
		}
		first := rep.Items[idx[0]]
		for _, i := range idx[1:] {
			if rep.Items[i].Hash != first.Hash || rep.Items[i].LinkTo != first.LinkTo {
				rep.Items[idx[0]].Duplicate = true
				rep.Items[i].Duplicate = true
			}
		}
	}
	sort.SliceStable(rep.Items, func(i, j int) bool {
		if rep.Items[i].Target != rep.Items[j].Target {
			return rep.Items[i].Target < rep.Items[j].Target
		}
		return rep.Items[i].Name < rep.Items[j].Name
	})
	return rep, nil
}

type npxEntry struct {
	Source    string `json:"source"`
	SkillPath string `json:"skillPath"`
}

// readNpxLock reads the lock written by `npx skills` so adopted copies keep
// their upstream.
func readNpxLock() map[string]npxEntry {
	out := map[string]npxEntry{}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	candidates := []string{filepath.Join(home, ".agents", ".skill-lock.json")}
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		candidates = append(candidates, filepath.Join(x, "skills", ".skill-lock.json"))
	}
	candidates = append(candidates, filepath.Join(home, ".local", "state", "skills", ".skill-lock.json"))
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if err != nil {
			continue
		}
		var doc struct {
			Skills map[string]npxEntry `json:"skills"`
		}
		if json.Unmarshal(b, &doc) == nil {
			for k, v := range doc.Skills {
				if _, seen := out[k]; !seen {
					out[k] = v
				}
			}
		}
	}
	return out
}
