package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/fsutil"
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
	UpstreamNew   []string // not selected, newly appeared upstream
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
}

// PackDir returns where a pack is vendored, refusing repo keys that would
// place it outside vendor/.
func PackDir(lib *library.Library, repo string) (string, error) {
	d := lib.PackDir(repo)
	if !fsutil.Within(lib.VendorDir(), d) {
		return "", fmt.Errorf("pack %q would be vendored at %s, outside %s", repo, d, lib.VendorDir())
	}
	return d, nil
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
		return "", fmt.Errorf("%s is outside %s", d, packDir)
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
	src, err := pack.ParseSource(p.Repo)
	if err != nil {
		return nil, err
	}
	src.Ref = p.Ref
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
			return nil, fmt.Errorf("%s: no usable SKILL.md (%s)", p.Repo, rejected[0])
		}
		return nil, fmt.Errorf("%s: no SKILL.md found", p.Repo)
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
			rep.Moved = append(rep.Moved, fmt.Sprintf("%s: renamed from %s upstream", s.Name, k))
			break
		}
	}

	known := map[string]bool{}
	for n := range lp.Skills {
		known[n] = true
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
			return fmt.Errorf("moving %s to trash: %w", dest, err)
		}
		if to != "" {
			rep.Backups = append(rep.Backups, lib.Rel(dest)+" → "+lib.Rel(to))
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
		rep.Warnings = append(rep.Warnings, n+" is a symlink, not a file; it was not copied into vendor/")
	}
	wanted := map[string]bool{}
	for _, s := range upstream {
		if !p.Selected(s.Name, s.Category) {
			if !known[s.Name] && lp.Commit != "" {
				rep.UpstreamNew = append(rep.UpstreamNew, s.Name)
			}
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
		newHash, err := fsutil.HashTree(s.Dir)
		if err != nil {
			return nil, err
		}
		prev := lp.Skills[s.Name]
		if prev != nil && prev.Category != s.Category {
			// The skill's category changed (upstream moved it, or the
			// categorisation rule did): rename the vendored directory.
			old, err := skillDir(packDir, prev.Category, s.Name)
			if err == nil && fsutil.Lexists(old) && !fsutil.Lexists(dest) {
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return nil, err
				}
				if err := os.Rename(old, dest); err != nil {
					return nil, err
				}
				pruneEmpty(lib, filepath.Dir(old))
				rep.Moved = append(rep.Moved, fmt.Sprintf("%s: %q → %q", s.Name, prev.Category, s.Category))
				prev = &lock.Skill{Category: s.Category, Hash: prev.Hash}
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
		case prev.Hash == newHash && prev.Category == s.Category && fsutil.Exists(dest):
			rep.Unchanged = append(rep.Unchanged, s.Name)
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
		if old, err := skillDir(packDir, ls.Category, n); err == nil && fsutil.Lexists(old) {
			if _, err := Trash(lib, old); err != nil {
				return nil, err
			}
		}
		delete(lp.Skills, n)
	}
	pruneEmpty(lib, packDir)

	if repoLicense != "" && len(lp.Skills) > 0 {
		if err := os.MkdirAll(packDir, 0o755); err != nil {
			return nil, err
		}
		if err := fsutil.CopyFile(repoLicense, filepath.Join(packDir, filepath.Base(repoLicense))); err != nil {
			return nil, err
		}
	}
	lp.Commit, lp.Ref = commit, p.Ref
	for _, l := range [][]string{rep.Changed, rep.Added, rep.Unchanged, rep.UpstreamNew, rep.UpstreamGone, rep.Dropped} {
		sort.Strings(l)
	}
	return rep, nil
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
			return fmt.Errorf("upstream renamed %s to %s, but renaming %s failed: %w; nothing was changed", old, new, lib.Rel(oldDir), err)
		}
		// Two steps, so a case-only rename also works where the file
		// system folds case. Nothing is overwritten, and a failed second
		// step is rolled back before the lock or manifest are touched.
		tmp := oldDir + ".skd-rename"
		if fsutil.Lexists(tmp) {
			return fail(fmt.Errorf("%s already exists", lib.Rel(tmp)))
		}
		if a, err := os.Lstat(oldDir); err == nil {
			if b, err := os.Lstat(newDir); err == nil && !os.SameFile(a, b) {
				return fail(fmt.Errorf("%s already exists", lib.Rel(newDir)))
			}
		}
		if err := rename(oldDir, tmp); err != nil {
			return fail(err)
		}
		if err := rename(tmp, newDir); err != nil {
			if rerr := rename(tmp, oldDir); rerr != nil {
				return fmt.Errorf("upstream renamed %s to %s; renaming failed (%v) and restoring %s failed too (%v): move %s back by hand",
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
	dir, err := PackDir(lib, repo)
	if err != nil {
		return err
	}
	if fsutil.Lexists(dir) {
		if _, err := Trash(lib, dir); err != nil {
			return err
		}
	}
	delete(lib.Lock.Packs, repo)
	pruneEmpty(lib, filepath.Dir(dir))
	return nil
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
