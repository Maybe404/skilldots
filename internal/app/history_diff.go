package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/pack"
)

// RecordDiff is what one recorded update changed upstream, skill by
// skill: the upstream version vendored before against the one after.
type RecordDiff struct {
	Record UpdateRecord `json:"record"`
	Skills []SkillDiff  `json:"skills"`
	// Source says where the versions were read: "snapshots" (the
	// library's upstream snapshots, no network) or "upstream" (the pack's
	// cache clone, fetching what it lacked).
	Source string `json:"source"`
}

// HistoryRecord is the n-th newest record (from 1) of a pack, or of every
// pack when packName is "".
func (s *Session) HistoryRecord(packName string, n int) (*UpdateRecord, error) {
	recs, _, err := s.History(packName, 0)
	if err != nil {
		return nil, err
	}
	if n < 1 || n > len(recs) {
		if len(recs) == 0 {
			return nil, i18n.New("no updates recorded yet")
		}
		return nil, i18n.Errorf("there are %d records; pick one from 1 (the newest) to %d", len(recs), len(recs))
	}
	return &recs[n-1], nil
}

// FindRecord is the record of repo's update at time at. Of two records
// of the same second (an update, and a resolve finished right after it)
// the one that names skills upstream changed is taken.
func (s *Session) FindRecord(repo string, at time.Time) (*UpdateRecord, error) {
	recs, _, err := s.History(repo, 0)
	if err != nil {
		return nil, err
	}
	var found *UpdateRecord
	for i := range recs {
		if !recs[i].Time.Equal(at) {
			continue
		}
		r := &recs[i]
		if len(r.Changed)+len(r.Merged)+len(r.Added)+len(r.Moved)+len(r.Shelved) > 0 {
			return r, nil
		}
		if found == nil {
			found = r
		}
	}
	if found != nil {
		return found, nil
	}
	return nil, i18n.Errorf("no update of %s recorded at %s", repo, at.UTC().Format(time.RFC3339))
}

// HistoryDiff is the upstream diff of record r, per skill it names
// (changed, merged, added, moved or shelved; only skill when set). The
// record of a finished resolve goes from the version the local changes
// were made on to the one merged in. It reads the
// upstream snapshots the record names when the library has them;
// otherwise the pack's cache clone, fetching the record's commits when
// the clone lacks them (that needs the network, and fails without it).
func (s *Session) HistoryDiff(ctx context.Context, r *UpdateRecord, skill string) (*RecordDiff, error) {
	out := &RecordDiff{Record: *r, Skills: []SkillDiff{}}
	status := map[string]string{}
	var names []string
	for _, l := range []struct {
		names  []string
		status string
	}{{r.Changed, "changed"}, {r.Merged, "changed"}, {r.Shelved, "changed"}, {r.Added, "added"}, {r.Moved, "moved"}} {
		for _, n := range l.names {
			if status[n] == "" && (skill == "" || n == skill) {
				status[n] = l.status
				names = append(names, n)
			}
		}
	}
	if skill != "" && len(names) == 0 {
		return nil, i18n.Errorf("the update of %s at %s did not change %s", r.Repo, r.Time.Local().Format("2006-01-02 15:04"), skill)
	}
	if r.SnapTo != "" && engine.KeepsSnapshots(s.Lib) && hasCommit(ctx, s.Lib.Root, r.SnapTo) && (r.SnapFrom == "" || hasCommit(ctx, s.Lib.Root, r.SnapFrom)) {
		out.Source = "snapshots"
		for _, n := range names {
			sd, err := diffInTrees(ctx, s.Lib.Root, r.SnapFrom, r.SnapTo, n, nil)
			if err != nil {
				return nil, err
			}
			if sd != nil {
				out.Skills = append(out.Skills, *sd)
			}
		}
		return out, nil
	}
	if r.To == "" {
		return out, nil
	}
	out.Source = "upstream"
	src, err := pack.ParseSource(r.Repo)
	if err != nil {
		return nil, err
	}
	unlock, err := pack.HoldContext(ctx, r.Repo)
	defer unlock()
	if err != nil {
		return nil, err
	}
	clone := pack.CachePath(r.Repo)
	if !fsutil.Exists(filepath.Join(clone, ".git")) {
		if clone, _, err = pack.FetchContext(ctx, src); err != nil {
			return nil, err
		}
	}
	for _, c := range []string{r.From, r.To} {
		if c == "" || hasCommit(ctx, clone, c) {
			continue
		}
		if _, err := library.GitContext(ctx, clone, "fetch", "-q", "--depth=1", "--", "origin", c); err != nil || !hasCommit(ctx, clone, c) {
			return nil, i18n.Errorf("the upstream commit %s of this update is not in the cache and could not be fetched (it needs the network, and upstream must still have it): %v", short7(c), err)
		}
	}
	// Where each skill is upstream now: where to look first.
	dirs := map[string]string{}
	if up, _, err := pack.Scan(clone); err == nil {
		for _, sk := range up {
			if rel, err := filepath.Rel(clone, sk.Dir); err == nil {
				dirs[sk.Name] = filepath.ToSlash(rel)
			}
		}
	}
	for _, n := range names {
		sd, err := diffInTrees(ctx, clone, r.From, r.To, n, dirs)
		if err != nil {
			return nil, err
		}
		if sd != nil {
			out.Skills = append(out.Skills, *sd)
		}
	}
	return out, nil
}

func short7(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

func hasCommit(ctx context.Context, root, c string) bool {
	_, err := library.GitContext(ctx, root, "cat-file", "-e", c+"^{commit}")
	return err == nil
}

// diffInTrees compares skill name between commits from and to of the
// repository at root ("" from: it did not exist). dirs names where to
// look for a skill first (by name); nil: category/name as in a snapshot.
// nil when the skill is the same in both.
func diffInTrees(ctx context.Context, root, from, to, name string, dirs map[string]string) (*SkillDiff, error) {
	locate := func(rev string) (string, error) {
		if rev == "" {
			return "", nil
		}
		prefer := dirs[name]
		if prefer == "." {
			return ".", nil
		}
		return engine.FindSkillDir(root, rev, name, prefer)
	}
	read := func(rev, dir string) (map[string][]byte, error) {
		if dir == "" {
			return map[string][]byte{}, nil
		}
		if dir == "." {
			dir = ""
		}
		return gitTreeFiles(ctx, root, rev, dir)
	}
	newDir, err := locate(to)
	if err != nil {
		return nil, err
	}
	oldDir, err := locate(from)
	if err != nil {
		return nil, err
	}
	oldFiles, err := read(from, oldDir)
	if err != nil {
		return nil, err
	}
	newFiles, err := read(to, newDir)
	if err != nil {
		return nil, err
	}
	sd := SkillDiff{Name: name, Status: "changed", LocalFiles: []string{}}
	switch {
	case oldDir == "":
		sd.Status = "added"
	case newDir != oldDir && len(changedPaths(oldFiles, newFiles)) == 0:
		sd.Status = "moved"
	}
	sd.Files, sd.MoreFiles = diffFiles(oldFiles, newFiles, nil)
	if len(sd.Files) == 0 && sd.Status == "changed" {
		return nil, nil
	}
	return &sd, nil
}
