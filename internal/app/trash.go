package app

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// Trash lists what skd has set aside, newest first.
func (s *Session) Trash() ([]engine.TrashEntry, error) { return engine.ListTrash(s.Lib, s.Reg) }

// RestoreOutcome is the result of RestoreEntry; its JSON form is what
// `skd trash restore --json` prints.
type RestoreOutcome struct {
	ID string `json:"id"`
	// Path is where the entry was put back.
	Path string `json:"restored"`
	// Skills are the skills it holds, and Repo the pack they come from,
	// as recorded when skd set it aside.
	Skills []string `json:"skills,omitempty"`
	Repo   string   `json:"repo,omitempty"`
	// Selected is set when skd selected it in skills.toml again: what
	// `skd remove` or `skd purge` took out is back as it was.
	Selected bool `json:"selected"`
	// Apply is the apply that linked it again (nil for an entry that went
	// back into an agent's directory, which needs no linking).
	Apply *ApplyOutcome `json:"apply,omitempty"`
	// Unused is set when it is back in the library's vendor/ but nothing
	// in skills.toml uses it (an entry set aside before skd recorded what
	// restoring needs); Hint then says what to run.
	Unused bool   `json:"unused,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

// Restore moves a trash entry back where it came from and returns that
// path; see RestoreEntry.
func (s *Session) Restore(key string) (string, error) {
	out, err := s.RestoreEntry(key)
	if out == nil {
		return "", err
	}
	return out.Path, err
}

// RestoreEntry moves a trash entry back where it came from. key is the
// entry's ID, or the name of the skill it holds when one entry does.
// It refuses an entry skd could not have made, or one recorded as coming
// from anywhere but an agent's skill directory, the library's vendor/ or
// a skill of yours skd removed here (engine.Restore).
//
// What `skd remove` or `skd purge` set aside comes back whole: the pack's
// entry in skills.toml and skills.lock is put back as it was (from the
// note skd kept on this machine, engine.TrashNote), and the skill is
// linked again; a skill of yours (skills/, private/) is linked again.
func (s *Session) RestoreEntry(key string) (*RestoreOutcome, error) {
	id, err := s.trashKey(key)
	if err != nil {
		return nil, err
	}
	var note *engine.TrashNote
	if p, err := engine.TrashEntryPath(s.Lib, id); err == nil {
		note = engine.TrashNoteFor(p)
	}
	if err := s.Lib.Writable(); err != nil {
		return nil, err
	}
	p, _ := engine.TrashEntryPath(s.Lib, id)
	dest, err := engine.Restore(s.Lib, s.Reg, id)
	if err != nil {
		return nil, err
	}
	s.noteWarn(engine.DropTrashNote(p))
	out := &RestoreOutcome{ID: id, Path: dest}
	if note != nil {
		out.Skills, out.Repo = note.Skills, note.Repo
	}
	if note != nil && note.Repo != "" {
		if pd, err := engine.PackDir(s.Lib, note.Repo); err == nil && (dest == pd || fsutil.Within(pd, dest)) {
			out.Selected = s.reinstate(note, dest == pd)
		}
	}
	if !fsutil.Within(s.Lib.Root, dest) {
		// Back in an agent's directory: nothing to link.
		return out, nil
	}
	if out.Apply, err = s.saveAndApply(); err != nil {
		return out, err
	}
	if fsutil.Within(s.Lib.VendorDir(), dest) && !s.uses(dest) {
		out.Unused = true
		out.Hint = i18n.T("skills.toml does not select it, so no agent gets it: `skd add <repository> --skill <name>` follows it again (the files restored are kept when they are what upstream has)")
		if out.Repo != "" && len(out.Skills) > 0 {
			out.Hint = i18n.Sprintf("skills.toml does not select it, so no agent gets it: `skd add %s --skill %s` follows it again", out.Repo, strings.Join(out.Skills, ","))
		}
	}
	return out, nil
}

// uses reports whether an entry of the library lives at dir, inside it
// or holds it.
func (s *Session) uses(dir string) bool {
	entries, _ := s.Resolve()
	for _, e := range entries {
		if e.Dir == dir || fsutil.Within(dir, e.Dir) || fsutil.Within(e.Dir, dir) {
			return true
		}
	}
	return false
}

// trashKey is the ID of the trash entry key names: key itself when it is
// an ID ("<batch>/<name>"), else the one entry holding the skill key.
func (s *Session) trashKey(key string) (string, error) {
	if batchID.MatchString(key) {
		return key, nil
	}
	items, err := s.Trash()
	if err != nil {
		return "", err
	}
	// A pack's repository names the entry of its whole directory (what
	// skd purge, or removing its last skill, set aside); any other key a
	// skill the entry holds.
	pack := s.Lib.Manifest.FindPack(key) == nil && strings.Contains(key, "/")
	var ids []string
	for _, it := range items {
		if it.Unsafe {
			continue
		}
		hit := false
		if pack {
			if pd, err := engine.PackDir(s.Lib, key); err == nil && it.Repo == key && it.Origin == pd {
				hit = true
			}
		} else {
			hit = it.Name == key
			for _, n := range it.Skills {
				hit = hit || n == key
			}
		}
		if hit {
			ids = append(ids, it.ID)
		}
	}
	switch len(ids) {
	case 0:
		if pack {
			return "", i18n.Errorf("no trash entry holds the pack %q (give an id as `skd trash` lists it, the repository of a purged pack, or the name of a skill in the trash)", key)
		}
		return "", i18n.Errorf("no trash entry %q (give an id as `skd trash` lists it, or the name of a skill in the trash)", key)
	case 1:
		return ids[0], nil
	}
	return "", i18n.Errorf("%d trash entries hold %q: %s; restore one by its id", len(ids), key, strings.Join(ids, ", "))
}

// batchID is how an ID `skd trash` lists starts: the batch, then "/".
var batchID = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}/`)

// reinstate puts back into skills.toml and skills.lock what n recorded
// for the vendored skills it holds: the whole pack as it was when whole
// (its directory came back) and the pack is not in skills.toml, else
// each skill selected again. It reports whether the skills are selected.
func (s *Session) reinstate(n *engine.TrashNote, whole bool) bool {
	m := s.Lib.Manifest
	var p *manifest.Pack
	for _, q := range m.Packs {
		if q.Repo == n.Repo {
			p = q
		}
	}
	category := func(name string) string {
		if n.Lock != nil && n.Lock.Skills[name] != nil {
			return n.Lock.Skills[name].Category
		}
		return ""
	}
	if p == nil {
		np := snapshotPack(n.Pack)
		if np == nil {
			np = &manifest.Pack{Repo: n.Repo}
			if n.Lock != nil {
				np.Ref = n.Lock.Ref
			}
			whole = false
		}
		if !whole {
			// Only what came back is selected.
			was := np.Skill
			np.Use, np.Skip, np.Skill = append([]string(nil), n.Skills...), nil, nil
			for _, name := range n.Skills {
				if o := was[name]; o != nil {
					np.Override(name, true).As, np.Override(name, true).Targets = o.As, o.Targets
				}
			}
		}
		if np.Alias != "" && s.checkAlias(np.Alias, np.Repo) != nil {
			s.logf(Warn, "the alias %q of %s is taken now; the pack is restored without it", np.Alias, np.Repo)
			np.Alias = ""
		}
		m.Packs = append(m.Packs, np)
		p = np
	} else {
		for _, name := range n.Skills {
			var was []string
			if n.Pack != nil {
				was = n.Pack.Use
			}
			selectAgain(p, name, category(name), was)
			if n.Pack != nil && n.Pack.Skill[name] != nil && p.Override(name, false) == nil {
				o := *n.Pack.Skill[name]
				p.Override(name, true).As, p.Override(name, true).Targets = o.As, o.Targets
			}
		}
	}
	if n.Lock != nil {
		lp := s.Lib.Lock.Packs[n.Repo]
		if lp == nil {
			lp = snapshotLock(n.Lock)
			if !whole || lp.Skills == nil {
				lp.Skills = map[string]*lock.Skill{}
			}
			s.Lib.Lock.Packs[n.Repo] = lp
		}
		if lp.Skills == nil {
			lp.Skills = map[string]*lock.Skill{}
		}
		for _, name := range n.Skills {
			if ls := n.Lock.Skills[name]; ls != nil && lp.Skills[name] == nil {
				c := *ls
				lp.Skills[name] = &c
			}
		}
	}
	selected := true
	for _, name := range n.Skills {
		if !p.Selected(name, category(name)) {
			selected = false
			s.logf(Warn, "%s stays out of %s: skip = [%q] in skills.toml excludes it", name, p.Repo, p.SkipPattern(name, category(name)))
		}
	}
	return selected
}

// selectAgain selects a vendored skill of p again: out of skip when p
// follows everything, else back in use, where it was in was (the use
// list before it went) when it was there.
func selectAgain(p *manifest.Pack, name, category string, was []string) {
	if p.Selected(name, category) {
		return
	}
	if p.UsesAll() {
		var skip []string
		for _, sk := range p.Skip {
			if sk != name && sk != filepath.ToSlash(filepath.Join(category, name)) {
				skip = append(skip, sk)
			}
		}
		p.Skip = skip
		return
	}
	entry, at := name, len(p.Use)
	for i, u := range was {
		if manifest.UseNames(u, name) {
			// Before the entries that followed it.
			entry, at = u, len(p.Use)
			for j, v := range p.Use {
				if indexOf(was, v) > i {
					at = j
					break
				}
			}
			break
		}
	}
	use := append(append(append([]string{}, p.Use[:at]...), entry), p.Use[at:]...)
	p.Use = dedupe(use)
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// noteWarn says, as a warning, why a trash note could not be kept as it
// should (engine.SetTrashNote): the entry itself is in the trash either way.
func (s *Session) noteWarn(err error) {
	if err != nil {
		s.logf(Warn, "%v", err)
	}
}

// TrashReason says, for a person, why a trash entry was set aside ("" when
// skd cannot tell).
func TrashReason(e engine.TrashEntry) string {
	switch e.Reason {
	case engine.ReasonRemove:
		return i18n.T("skd remove")
	case engine.ReasonPurge:
		return i18n.T("skd purge")
	case engine.ReasonDeselected:
		return i18n.T("no longer selected")
	case engine.ReasonReplaced:
		return i18n.T("had changes; an update replaced it")
	case engine.ReasonInTheWay:
		return i18n.T("in the way in an agent directory")
	}
	return ""
}

// EmptyTrash permanently deletes everything in the trash. It is the one
// irreversible operation: callers offer it only to a person at a terminal,
// after asking.
func (s *Session) EmptyTrash() (int, error) { return engine.EmptyTrash(s.Lib) }
