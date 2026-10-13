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
	Dropped       []string // vendored but no longer selected (removed from vendor)
	NoLicense     bool
	NoLicenseList []string
	Upstream      []pack.Skill
}

// Sync fetches a pack's upstream and makes vendor/ match the selection.
// It updates the lock but does not save or link; callers do that.
func Sync(lib *library.Library, p *manifest.Pack) (*SyncReport, error) {
	src, err := pack.ParseSource(p.Repo)
	if err != nil {
		return nil, err
	}
	src.Ref = p.Ref
	clone, commit, err := pack.Fetch(src)
	if err != nil {
		return nil, err
	}
	upstream, err := pack.Scan(clone)
	if err != nil {
		return nil, err
	}
	if len(upstream) == 0 {
		return nil, fmt.Errorf("%s: no SKILL.md found", p.Repo)
	}

	rep := &SyncReport{Repo: p.Repo, NewCommit: commit, Upstream: upstream}
	lp := lib.Lock.Packs[p.Repo]
	if lp == nil {
		lp = &lock.Pack{Skills: map[string]*lock.Skill{}}
		lib.Lock.Packs[p.Repo] = lp
	}
	rep.OldCommit = lp.Commit
	known := map[string]bool{}
	for n := range lp.Skills {
		known[n] = true
	}

	packDir := lib.PackDir(p.Repo)
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
		dest := filepath.Join(packDir, filepath.FromSlash(s.Category), s.Name)
		newHash, err := fsutil.HashTree(s.Dir)
		if err != nil {
			return nil, err
		}
		prev := lp.Skills[s.Name]
		switch {
		case prev == nil:
			rep.Added = append(rep.Added, s.Name)
		case prev.Hash == newHash && fsutil.Exists(dest):
			rep.Unchanged = append(rep.Unchanged, s.Name)
			continue
		default:
			rep.Changed = append(rep.Changed, s.Name)
		}
		if prev != nil && prev.Category != s.Category {
			os.RemoveAll(filepath.Join(packDir, filepath.FromSlash(prev.Category), s.Name))
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
		os.RemoveAll(filepath.Join(packDir, filepath.FromSlash(ls.Category), n))
		delete(lp.Skills, n)
	}
	pruneEmpty(packDir)

	if repoLicense != "" && len(lp.Skills) > 0 {
		if err := os.MkdirAll(packDir, 0o755); err != nil {
			return nil, err
		}
		fsutil.CopyTree(repoLicense, filepath.Join(packDir, filepath.Base(repoLicense)))
	}
	lp.Commit, lp.Ref = commit, p.Ref
	for _, l := range [][]string{rep.Changed, rep.Added, rep.Unchanged, rep.UpstreamNew, rep.UpstreamGone, rep.Dropped} {
		sort.Strings(l)
	}
	return rep, nil
}

// RemovePack deletes a pack's vendored files and lock entry.
func RemovePack(lib *library.Library, repo string) error {
	if err := os.RemoveAll(lib.PackDir(repo)); err != nil {
		return err
	}
	delete(lib.Lock.Packs, repo)
	pruneEmpty(filepath.Dir(lib.PackDir(repo)))
	return nil
}

// pruneEmpty removes dir and empty parents up to vendor/.
func pruneEmpty(dir string) {
	for strings.Contains(dir, library.DirVendor) && filepath.Base(dir) != library.DirVendor {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}
