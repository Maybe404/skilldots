package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
	NoLicense     bool
	NoLicenseList []string
	// Rejected lists upstream SKILL.md files that were not used (e.g. a
	// name that is not a safe directory name).
	Rejected []pack.Rejected
	// Backups lists vendored directories that held something other than
	// what skilldots wrote and were moved to the trash before replacing.
	Backups  []string
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

	repoLicense := fsutil.FindLicense(clone)
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
