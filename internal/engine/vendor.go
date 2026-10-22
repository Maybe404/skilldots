package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
)

// SyncReport describes what one Sync did to a pack.
type SyncReport struct {
	Repo          string
	OldCommit     string
	NewCommit     string
	Changed       []string // selected skills whose content changed
	Added         []string // selected skills vendored for the first time
	Unchanged     []string
	UpstreamNew   []string // not selected, new upstream since the last update (set by app.Update, not Sync)
	UpstreamGone  []string // selected but no longer upstream (kept locally)
	Dropped       []string // vendored but no longer selected (moved to the trash)
	Moved         []string // "name: old category → new category" (directory renamed)
	NoLicense     bool
	NoLicenseList []string
	// Rejected lists upstream SKILL.md files that were not used (e.g. a
	// name that is not a safe directory name).
	Rejected []pack.Rejected
	// Backups lists vendored directories that held something other than
	// what skilldots wrote and were moved to the trash before replacing.
	Backups []string
	// Renamed maps newly vendored skills whose name was already taken to
	// the `as` link name assigned (and written into the manifest).
	Renamed map[string]string `json:",omitempty"`
	// Warnings are other things the user should know (e.g. a LICENSE
	// that is a symlink and was not copied).
	Warnings []string     `json:",omitempty"`
	Upstream []pack.Skill `json:"-"`
	// Trashed maps each Dropped skill to where in the trash its
	// directory went.
	Trashed map[string]string `json:"-"`
	// Merged are Changed skills the user had changed: upstream's changes
	// were merged into the local copy (the copy as it was went to the
	// trash first).
	Merged []string `json:",omitempty"`
	// Shelved are skills upstream changed whose local changes conflict
	// with upstream's: they were left as they are, waiting for `skd
	// resolve`. They are not in Changed.
	Shelved []ShelvedSkill `json:",omitempty"`
	// SnapFrom and SnapTo are the upstream snapshots (commits of the
	// library repository) before and after the update; empty when the
	// library keeps none.
	SnapFrom string `json:"-"`
	SnapTo   string `json:"-"`
}

// ShelvedSkill is a skill whose update waits for its conflicts to be
// resolved.
type ShelvedSkill struct {
	Name string
	// Files are the conflicted files, relative to the skill's directory.
	Files []string
}

// PackDir returns where a pack is vendored, refusing repo keys that would
// place it outside vendor/.
func PackDir(lib *library.Library, repo string) (string, error) {
	d := lib.PackDir(repo)
	if !fsutil.Within(lib.VendorDir(), d) {
		return "", i18n.Errorf("pack %q would be vendored at %s, outside %s", repo, d, lib.VendorDir())
	}
	return d, nil
}

// throughLink is an error when p, a place in the library's vendor/, is
// reached through a symbolic link inside the library (vendor/ itself, a
// pack's directory, a category): skd would write, move or remove
// wherever that link points. The library root itself may be a link.
func throughLink(lib *library.Library, p string) error {
	if l := fsutil.LinkOnPath(lib.Root, p); l != "" {
		return i18n.Errorf("%s goes through %s, a symbolic link inside the library; skd does not follow it (make it a directory of its own)", lib.Rel(p), lib.Rel(l))
	}
	return nil
}

// skillDir is where one vendored skill lives, validated to stay inside packDir.
func skillDir(packDir, category, name string) (string, error) {
	if err := manifest.CheckName(name); err != nil {
		return "", err
	}
	if err := manifest.CheckCategory(category); err != nil {
		return "", err
	}
	d := filepath.Join(packDir, filepath.FromSlash(category), name)
	if !fsutil.Within(packDir, d) {
		return "", i18n.Errorf("%s is outside %s", d, packDir)
	}
	return d, nil
}

// Sync fetches a pack's upstream and makes vendor/ match the selection.
// It updates the lock but does not save or link; callers do that.
func Sync(lib *library.Library, p *manifest.Pack) (*SyncReport, error) {
	packDir, err := PackDir(lib, p.Repo)
	if err != nil {
		return nil, err
	}
	// Nothing below is written, moved or removed through a link inside
	// the library (vendor/, the pack's directory, a category): not in the
	// directory of any skill the lock records (renamed, replaced or
	// dropped below), nor in that of any skill vendored (checked as it
	// comes).
	if lp := lib.Lock.Packs[p.Repo]; lp != nil {
		for _, n := range sortedKeys(lp.Skills) {
			if d, err := skillDir(packDir, lp.Skills[n].Category, n); err == nil {
				if err := throughLink(lib, d); err != nil {
					return nil, err
				}
			}
		}
	}
	src, err := pack.ParseSource(p.Repo)
	if err != nil {
		return nil, err
	}
	src.Ref = p.Ref
	defer pack.Hold(p.Repo)()
	clone, commit, err := pack.Fetch(src)
	if err != nil {
		return nil, err
	}
	upstream, rejected, err := pack.Scan(clone)
	if err != nil {
		return nil, err
	}
	if len(upstream) == 0 {
		if len(rejected) > 0 {
			return nil, i18n.Errorf("%s: no usable SKILL.md (%s)", p.Repo, rejected[0])
		}
		return nil, i18n.Errorf("%s: no SKILL.md found", p.Repo)
	}

	rep := &SyncReport{Repo: p.Repo, NewCommit: commit, Upstream: upstream, Rejected: rejected}
	lp := lib.Lock.Packs[p.Repo]
	if lp == nil {
		lp = &lock.Pack{Skills: map[string]*lock.Skill{}}
		lib.Lock.Packs[p.Repo] = lp
	}
	if lp.Skills == nil {
		lp.Skills = map[string]*lock.Skill{}
	}
	rep.OldCommit = lp.Commit

	// Upstream renamed a skill only in case (Foo → foo): carry the lock
	// entry, the vendored directory, the manifest references and, where
	// the file system folds case, the link records over to the new name,
	// instead of treating it as one skill gone and another added.
	upExact := map[string]bool{}
	for _, s := range upstream {
		upExact[s.Name] = true
	}
	for _, s := range upstream {
		if lp.Skills[s.Name] != nil {
			continue
		}
		for _, k := range sortedKeys(lp.Skills) {
			if k == s.Name || upExact[k] || !strings.EqualFold(k, s.Name) {
				continue
			}
			if err := renameCase(lib, p, lp, packDir, k, s.Name); err != nil {
				return nil, err
			}
			rep.Moved = append(rep.Moved, i18n.Sprintf("%s: renamed from %s upstream", s.Name, k))
			break
		}
	}

	// moveAside clears dest for a new copy. A pristine copy of what the
	// lock recorded is discarded (it is reproducible); anything else is
	// moved to the trash.
	moveAside := func(dest string, prev *lock.Skill) error {
		if !fsutil.Lexists(dest) {
			return nil
		}
		if prev != nil && fsutil.Pristine(dest, prev.Hash) {
			return os.RemoveAll(dest)
		}
		to, err := Trash(lib, dest)
		if err != nil {
			return i18n.Errorf("moving %s to trash: %w", dest, err)
		}
		if to != "" {
			rep.Backups = append(rep.Backups, lib.Rel(dest)+" → "+lib.Rel(to))
			if err := SetTrashNote(to, &TrashNote{Reason: ReasonReplaced, Skills: []string{filepath.Base(dest)}, Repo: p.Repo}); err != nil {
				rep.Warnings = append(rep.Warnings, err.Error())
			}
		}
		return nil
	}

	// Link names used by everything else in the library, so a skill this
	// pack vendors for the first time can be given a free one.
	taken := map[string]string{}
	others, _ := collect(lib)
	for _, e := range others {
		if !(e.Kind == KindVendor && e.Origin == p.Repo) {
			taken[strings.ToLower(e.LinkName)] = Describe(e) + " (" + e.Dir + ")"
		}
	}
	for _, s := range upstream {
		if o := p.Override(s.Name, false); o != nil && o.As != "" {
			taken[strings.ToLower(o.As)] = "vendor:" + p.Repo
		} else if lp.Skills[s.Name] != nil {
			taken[strings.ToLower(s.Name)] = "vendor:" + p.Repo
		}
	}

	repoLicense := fsutil.FindLicense(clone)
	for _, n := range fsutil.LicenseLinks(clone) {
		rep.Warnings = append(rep.Warnings, i18n.Sprintf("%s is a symlink, not a file; it was not copied into vendor/", n))
	}
	wanted := map[string]bool{}
	contributions := map[string]*lock.Contribution{}
	for n, ls := range lp.Skills {
		if ls != nil && ls.Contribution != nil {
			contributions[n] = ls.Contribution
		}
	}
	// oldCategory is the category a skill had before upstream moved it.
	oldCategory := map[string]string{}
	var selected []pack.Skill
	for _, s := range upstream {
		if p.Selected(s.Name, s.Category) {
			selected = append(selected, s)
		}
	}
	for _, s := range upstream {
		if !p.Selected(s.Name, s.Category) {
			continue
		}
		wanted[s.Name] = true
		if repoLicense == "" && fsutil.FindLicense(s.Dir) == "" && s.License == "" {
			rep.NoLicenseList = append(rep.NoLicenseList, s.Name)
		}
		dest, err := skillDir(packDir, s.Category, s.Name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		if err := throughLink(lib, dest); err != nil {
			return nil, err
		}
		newHash, err := fsutil.HashTree(s.Dir)
		if err != nil {
			return nil, err
		}
		prev := lp.Skills[s.Name]
		if prev != nil && prev.Category != s.Category {
			// The skill's category changed (upstream moved it, or the
			// categorisation rule did): rename the vendored directory.
			old, err := skillDir(packDir, prev.Category, s.Name)
			if err == nil && fsutil.Lexists(old) && (!fsutil.Lexists(dest) || enclosing(old, dest) != "") {
				if err := moveSkillDir(old, dest); errors.As(err, new(strandedError)) {
					return nil, i18n.Errorf("upstream moved %s from %s to %s; moving its directory failed: %w", s.Name, lib.Rel(old), lib.Rel(dest), err)
				} else if err != nil {
					return nil, i18n.Errorf("upstream moved %s from %s to %s, but moving its directory failed: %w; it was left where it was",
						s.Name, lib.Rel(old), lib.Rel(dest), err)
				}
				pruneEmpty(lib, filepath.Dir(old))
				rep.Moved = append(rep.Moved, fmt.Sprintf("%s: %q → %q", s.Name, prev.Category, s.Category))
				oldCategory[s.Name] = prev.Category
				moved := *prev
				moved.Category = s.Category
				prev = &moved
				lp.Skills[s.Name] = prev
			}
		}
		if prev == nil {
			if o := p.Override(s.Name, false); (o == nil || o.As == "") && taken[strings.ToLower(s.Name)] != "" {
				as := freeName(func(n string) bool { return taken[strings.ToLower(n)] != "" }, s.Name, p.Prefix())
				p.Override(s.Name, true).As = as
				taken[strings.ToLower(as)] = "vendor:" + p.Repo
				if rep.Renamed == nil {
					rep.Renamed = map[string]string{}
				}
				rep.Renamed[s.Name] = as
			}
		}
		switch {
		case prev == nil && fsutil.Pristine(dest, newHash):
			// Already there (e.g. the lock was lost): just record it.
			rep.Added = append(rep.Added, s.Name)
			lp.Skills[s.Name] = &lock.Skill{Category: s.Category, Hash: newHash}
			continue
		case prev == nil:
			rep.Added = append(rep.Added, s.Name)
		case prev.Resolving && fsutil.Exists(dest):
			// Being resolved (skd resolve): the directory holds the merge
			// with its conflict markers; it is left to finish.
			rep.Shelved = append(rep.Shelved, ShelvedSkill{Name: s.Name, Files: nonNil(prev.Conflicts)})
			continue
		case prev.Hash == newHash && prev.Category == s.Category && fsutil.Exists(dest):
			if prev.Shelved != "" {
				// Upstream went back to the version the local changes
				// were made on: nothing waits any more.
				lp.Skills[s.Name] = &lock.Skill{Category: prev.Category, Hash: prev.Hash}
			}
			rep.Unchanged = append(rep.Unchanged, s.Name)
			continue
		case fsutil.Exists(dest) && !fsutil.Pristine(dest, prev.Hash) && filesChanged(dest, prev.Hash):
			// The user changed its files: merge upstream's changes in.
			// (A copy that only gained what is not a file of it, a .git
			// or a link, is replaced below, that set aside.)
			cats := []string{s.Category, oldCategory[s.Name]}
			m, err := mergeSkill(lib, p.Repo, s, dest, prev, cats, commit)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.Name, err)
			}
			if !m.Clean() {
				rep.Shelved = append(rep.Shelved, ShelvedSkill{Name: s.Name, Files: m.ConflictPaths()})
				lp.Skills[s.Name] = &lock.Skill{Category: prev.Category, Hash: prev.Hash,
					Shelved: commit, Upstream: newHash, Conflicts: LockPaths(m.ConflictPaths())}
				continue
			}
			to, err := SetAsideMerged(lib, p.Repo, dest, s.Name, lp, prev)
			if err != nil {
				return nil, err
			}
			rep.Backups = append(rep.Backups, lib.Rel(dest)+" → "+lib.Rel(to))
			if err := WriteFileSet(dest, m.Files, nil); err != nil {
				return nil, err
			}
			rep.Changed = append(rep.Changed, s.Name)
			rep.Merged = append(rep.Merged, s.Name)
			lp.Skills[s.Name] = &lock.Skill{Category: s.Category, Hash: newHash}
			continue
		default:
			rep.Changed = append(rep.Changed, s.Name)
		}
		if prev != nil && prev.Category != s.Category {
			if old, err := skillDir(packDir, prev.Category, s.Name); err == nil {
				if err := moveAside(old, prev); err != nil {
					return nil, err
				}
			}
		}
		if err := moveAside(dest, prev); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := fsutil.CopyTree(s.Dir, dest); err != nil {
			return nil, err
		}
		lp.Skills[s.Name] = &lock.Skill{Category: s.Category, Hash: newHash}
	}
	rep.NoLicense = len(rep.NoLicenseList) > 0

	// Selected in the manifest but absent upstream: keep what we have.
	upNames := map[string]bool{}
	for _, s := range upstream {
		upNames[s.Name] = true
	}
	for n, ls := range lp.Skills {
		if wanted[n] {
			continue
		}
		if !upNames[n] && p.Selected(n, ls.Category) {
			rep.UpstreamGone = append(rep.UpstreamGone, n)
			continue
		}
		rep.Dropped = append(rep.Dropped, n)
		if old, err := skillDir(packDir, ls.Category, n); err == nil && ls.Resolving {
			// Its conflicts go with it.
			if err := UnstageConflicts(lib, old, ls.Conflicts); err != nil {
				rep.Warnings = append(rep.Warnings, err.Error())
			}
		}
		if old, err := skillDir(packDir, ls.Category, n); err == nil && fsutil.Lexists(old) {
			to, err := Trash(lib, old)
			if err != nil {
				return nil, err
			}
			if rep.Trashed == nil {
				rep.Trashed = map[string]string{}
			}
			rep.Trashed[n] = to
			// What restoring it needs to select it again.
			was := &lock.Pack{Commit: lp.Commit, Ref: lp.Ref, Upstream: append([]string(nil), lp.Upstream...),
				Skills: map[string]*lock.Skill{n: {Category: ls.Category, Hash: ls.Hash}}}
			if err := SetTrashNote(to, &TrashNote{Reason: ReasonDeselected, Skills: []string{n}, Repo: p.Repo, Lock: was}); err != nil {
				rep.Warnings = append(rep.Warnings, err.Error())
			}
		}
		delete(lp.Skills, n)
	}
	pruneEmpty(lib, packDir)

	// A change sent upstream (skd contribute) is remembered while the
	// skill still differs from upstream; once an update brought upstream
	// to what the user has, it landed and is forgotten.
	for n, c := range contributions {
		ls := lp.Skills[n]
		if ls == nil {
			continue
		}
		ls.Contribution = nil
		if d, err := skillDir(packDir, ls.Category, n); err == nil && fsutil.Exists(d) && !fsutil.Pristine(d, ls.Hash) {
			ls.Contribution = c
		}
	}

	if repoLicense != "" && len(lp.Skills) > 0 {
		if err := os.MkdirAll(packDir, 0o755); err != nil {
			return nil, err
		}
		if err := fsutil.CopyFile(repoLicense, filepath.Join(packDir, filepath.Base(repoLicense))); err != nil {
			return nil, err
		}
	}
	lp.Commit, lp.Ref = commit, p.Ref
	for _, l := range [][]string{rep.Changed, rep.Added, rep.Unchanged, rep.UpstreamGone, rep.Dropped, rep.Merged} {
		sort.Strings(l)
	}
	sort.Slice(rep.Shelved, func(i, j int) bool { return rep.Shelved[i].Name < rep.Shelved[j].Name })

	// The upstream version vendored, as the next merge's base.
	if KeepsSnapshots(lib) {
		from, to, err := writeSnapshot(lib, p.Repo, commit, commitDate(clone, commit), selected, repoLicense)
		if err != nil {
			rep.Warnings = append(rep.Warnings, i18n.Sprintf("could not record upstream's version in the library's git repository (%v); the next update of a skill you changed may report more conflicts", err))
		} else {
			rep.SnapFrom, rep.SnapTo = from, to
		}
	} else if len(rep.Merged)+len(rep.Shelved) > 0 {
		rep.Warnings = append(rep.Warnings, i18n.T("the library is not a git repository, so upstream's versions are not recorded: merging a skill you changed finds no common version and reports every file both sides changed as a conflict (skd init or git init makes it one)"))
	}
	return rep, nil
}

// enclosing returns the spelling, on old's path, of dest when dest is a
// directory that holds old (upstream moved x/x/ to x/): "" otherwise.
// The file system decides, so a category differing only in case counts
// where it folds case.
func enclosing(old, dest string) string {
	if len(dest) >= len(old) || old[len(dest)] != filepath.Separator || !strings.EqualFold(old[:len(dest)], dest) {
		return ""
	}
	a, err1 := os.Lstat(old[:len(dest)])
	b, err2 := os.Lstat(dest)
	if err1 != nil || err2 != nil || !os.SameFile(a, b) {
		return ""
	}
	return old[:len(dest)]
}

// moveSkillDir renames the vendored directory old to dest, which does
// not exist, or holds old and nothing else but the directories leading
// to it. When one lies inside the other (upstream moved x/ to x/x/, or
// x/x/ to x/), old first goes to a free name beside them, so it is never
// renamed into itself. On failure everything is put back as it was.
func moveSkillDir(old, dest string) error {
	outer := enclosing(old, dest)
	if outer == "" && !(len(dest) > len(old) && dest[len(old)] == filepath.Separator && strings.EqualFold(dest[:len(old)], old)) {
		undo, err := mkdirs(filepath.Dir(dest))
		if err != nil {
			return err
		}
		if err := rename(old, dest); err != nil {
			undo()
			return err
		}
		return nil
	}
	aside := old
	if outer != "" {
		aside = outer
		// Only the directories leading to old may be in the way.
		var in []string
		filepath.WalkDir(outer, func(p string, d fs.DirEntry, err error) error {
			switch {
			case p == old:
				return filepath.SkipDir
			case err != nil || !d.IsDir() || !fsutil.Within(p, old):
				in = append(in, p)
				return filepath.SkipDir
			}
			return nil
		})
		if len(in) > 0 {
			return i18n.Errorf("%s is in the way", in[0])
		}
	}
	tmp := aside + ".skd-move"
	for i := 2; fsutil.Lexists(tmp); i++ {
		tmp = fmt.Sprintf("%s.skd-move-%d", aside, i)
	}
	if err := rename(old, tmp); err != nil {
		return err
	}
	putBack := func(err error) error {
		_, perr := mkdirs(filepath.Dir(old))
		if perr == nil {
			perr = rename(tmp, old)
		}
		if perr != nil {
			return strandedError{i18n.Errorf("%w, and putting %s back failed too (%v): it is at %s; move it back by hand", err, old, perr, tmp)}
		}
		return err
	}
	if outer != "" {
		// Now only empty directories: remove them, innermost first.
		for d := filepath.Dir(old); ; d = filepath.Dir(d) {
			if err := os.Remove(d); err != nil {
				return putBack(err)
			}
			if d == outer {
				break
			}
		}
	}
	undo, err := mkdirs(filepath.Dir(dest))
	if err != nil {
		return putBack(err)
	}
	if err := rename(tmp, dest); err != nil {
		undo()
		return putBack(err)
	}
	return nil
}

// strandedError is a failed moveSkillDir that could not put the
// directory back either.
type strandedError struct{ error }

func (e strandedError) Unwrap() error { return e.error }

// mkdirs is os.MkdirAll that also returns how to remove again the
// directories it created (those still empty).
func mkdirs(dir string) (undo func(), err error) {
	var made []string
	for d := dir; !fsutil.Lexists(d); d = filepath.Dir(d) {
		made = append(made, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	undo = func() {
		for _, d := range made {
			os.Remove(d)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		undo()
		return nil, err
	}
	return undo, nil
}

// renameCase moves a vendored skill from name old to name new (which
// differ only in case) everywhere skilldots records it.
func renameCase(lib *library.Library, p *manifest.Pack, lp *lock.Pack, packDir, old, new string) error {
	ls := lp.Skills[old]
	oldDir, err := skillDir(packDir, ls.Category, old)
	if err != nil {
		return err
	}
	newDir, err := skillDir(packDir, ls.Category, new)
	if err != nil {
		return err
	}

	if fsutil.Lexists(oldDir) {
		fail := func(err error) error {
			return i18n.Errorf("upstream renamed %s to %s, but renaming %s failed: %w; nothing was changed", old, new, lib.Rel(oldDir), err)
		}
		// Two steps, so a case-only rename also works where the file
		// system folds case. Nothing is overwritten, and a failed second
		// step is rolled back before the lock or manifest are touched.
		tmp := oldDir + ".skd-rename"
		if fsutil.Lexists(tmp) {
			return fail(i18n.Errorf("%s already exists", lib.Rel(tmp)))
		}
		if a, err := os.Lstat(oldDir); err == nil {
			if b, err := os.Lstat(newDir); err == nil && !os.SameFile(a, b) {
				return fail(i18n.Errorf("%s already exists", lib.Rel(newDir)))
			}
		}
		if err := rename(oldDir, tmp); err != nil {
			return fail(err)
		}
		if err := rename(tmp, newDir); err != nil {
			if rerr := rename(tmp, oldDir); rerr != nil {
				return i18n.Errorf("upstream renamed %s to %s; renaming failed (%v) and restoring %s failed too (%v): move %s back by hand",
					old, new, err, lib.Rel(oldDir), rerr, lib.Rel(tmp))
			}
			return fail(err)
		}
	}
	lp.Skills[new] = ls
	delete(lp.Skills, old)

	rename := func(u string) string {
		if u == old {
			return new
		}
		if strings.HasSuffix(u, "/"+old) {
			return strings.TrimSuffix(u, old) + new
		}
		return u
	}
	for i, u := range p.Use {
		p.Use[i] = rename(u)
	}
	for i, u := range p.Skip {
		p.Skip[i] = rename(u)
	}
	hasAs := false
	if o := p.Skill[old]; o != nil {
		if p.Skill[new] == nil {
			p.Skill[new] = o
			delete(p.Skill, old)
		}
		hasAs = o.As != ""
	}
	if hasAs {
		return nil // the link name does not change
	}
	// Link records "<target>/<old>": where the target directory folds case
	// the existing link already is "<target>/<new>", so re-key the record;
	// elsewhere the old link is a separate entry that apply removes.
	for key, l := range lib.Lock.Links {
		t, n, _ := splitKey(key)
		if n != old || l.Path == "" || l.Source != lib.Rel(oldDir) {
			continue
		}
		newPath := filepath.Join(filepath.Dir(l.Path), new)
		a, err1 := os.Lstat(l.Path)
		b, err2 := os.Lstat(newPath)
		if err1 != nil || err2 != nil || !os.SameFile(a, b) {
			continue
		}
		l.Path = newPath
		lib.Lock.Links[t+"/"+new] = l
		delete(lib.Lock.Links, key)
	}
	return nil
}

// RemovePack moves a pack's vendored files to the trash and drops its lock entry.
func RemovePack(lib *library.Library, repo string) error {
	_, err := RemovePackTo(lib, repo)
	return err
}

// RemovePackTo is RemovePack that also says where in the trash the pack's
// directory went ("" when nothing was vendored). The entry is named after
// the repository ("owner-repo"), so `skd trash` tells packs apart.
func RemovePackTo(lib *library.Library, repo string) (string, error) {
	dir, err := PackDir(lib, repo)
	if err != nil {
		return "", err
	}
	if err := throughLink(lib, dir); err != nil {
		return "", err
	}
	to := ""
	if fsutil.Lexists(dir) {
		if to, err = trashAs(lib, dir, packTrashName(repo)); err != nil {
			return "", err
		}
	}
	delete(lib.Lock.Packs, repo)
	pruneEmpty(lib, filepath.Dir(dir))
	return to, nil
}

// packTrashName is the name a pack's directory gets in the trash:
// "owner-repo" for owner/repo, else the repository's last segment (a URL
// or a path would make a long name of no use).
func packTrashName(repo string) string {
	parts := strings.FieldsFunc(repo, func(r rune) bool { return r == '/' || r == '\\' || r == ':' })
	switch {
	case len(parts) == 2:
		return parts[0] + "-" + parts[1]
	case len(parts) > 0:
		return parts[len(parts)-1]
	}
	return repo
}

// pruneEmpty removes dir and empty parents up to vendor/.
func pruneEmpty(lib *library.Library, dir string) {
	for fsutil.Within(lib.VendorDir(), dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}
