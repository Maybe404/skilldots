package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/pack"
)

// HistoryFile is the update history, at the library root. It is part of
// the library: committed with it and pushed by `skd sync`, so a clone on
// another machine has it too.
const HistoryFile = "skills.history.jsonl"

// MaxCommits is how many upstream commit subjects one record keeps.
const MaxCommits = 20

// historyDepth is how many commits an update fetches beyond the shallow
// cache clone to find the subjects since the previous commit.
const historyDepth = 200

// UpdateRecord is one pack's update, as `skd history` shows it.
type UpdateRecord struct {
	Time time.Time `json:"time"`
	Repo string    `json:"repo"`
	// From and To are the vendored commits before and after ("" From: the
	// pack had never been fetched).
	From string `json:"from"`
	To   string `json:"to"`
	// Changed were re-vendored with new content; Added were vendored for
	// the first time; Moved only changed directory upstream; Gone left
	// upstream (the local copy is kept); Dropped are no longer selected
	// (moved to the trash); New appeared upstream and are not selected.
	Changed []string `json:"changed"`
	Added   []string `json:"added"`
	Moved   []string `json:"moved"`
	Gone    []string `json:"gone"`
	Dropped []string `json:"dropped"`
	New     []string `json:"new"`
	// Commits are the upstream commits between From and To that touch
	// those skills' directories, newest first, at most MaxCommits.
	Commits []UpstreamCommit `json:"commits"`
	// MoreCommits is set when there were more than MaxCommits.
	MoreCommits bool `json:"moreCommits,omitempty"`
	// CommitsUnknown is set when upstream's history could not be read
	// (From is not in the fetched history, or git failed).
	CommitsUnknown bool `json:"commitsUnknown,omitempty"`
}

// UpstreamCommit is one upstream commit.
type UpstreamCommit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// HistoryPath is the history file of this library.
func (s *Session) HistoryPath() string { return filepath.Join(s.Lib.Root, HistoryFile) }

// record turns a finished Sync into a history record, or nil when the
// update changed nothing worth recording.
func newRecord(rep *engine.SyncReport, now time.Time) *UpdateRecord {
	r := &UpdateRecord{Time: now.UTC().Truncate(time.Second), Repo: rep.Repo, From: rep.OldCommit, To: rep.NewCommit,
		Changed: nonNilStrings(rep.Changed), Added: nonNilStrings(rep.Added), Gone: nonNilStrings(rep.UpstreamGone),
		Dropped: nonNilStrings(rep.Dropped), New: nonNilStrings(rep.UpstreamNew), Moved: movedNames(rep.Moved), Commits: []UpstreamCommit{}}
	if r.From == r.To && len(r.Changed)+len(r.Added)+len(r.Moved)+len(r.Dropped) == 0 {
		return nil
	}
	return r
}

// movedNames takes the skill names out of SyncReport.Moved
// ("name: …").
func movedNames(moved []string) []string {
	out := []string{}
	for _, m := range moved {
		if n, _, ok := strings.Cut(m, ":"); ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return nonNilStrings(dedupe(out))
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return append([]string{}, s...)
}

// fillCommits lists the upstream commits between r.From and r.To that
// touch the skills r names. The cache clone is shallow, so history is
// fetched (historyDepth commits) when r.From is not in it; a failure only
// marks the commits unknown. Callers Hold the repository.
func fillCommits(ctx context.Context, r *UpdateRecord, ref string, upstream []pack.Skill) {
	if r.From == "" || r.From == r.To {
		return
	}
	clone := pack.CachePath(r.Repo)
	dirs := map[string]string{}
	for _, sk := range upstream {
		if rel, err := filepath.Rel(clone, sk.Dir); err == nil {
			dirs[sk.Name] = filepath.ToSlash(rel)
		}
	}
	var specs []string
	add := func(names []string) {
		for _, n := range names {
			if d, ok := dirs[n]; ok && d != "." {
				specs = append(specs, ":(literal)"+d)
			} else {
				// No longer upstream (gone): any directory of that name.
				specs = append(specs, ":(glob)**/"+globEscape(n)+"/**")
			}
		}
	}
	add(r.Changed)
	add(r.Added)
	add(r.Moved)
	add(r.Gone)
	if len(specs) == 0 {
		return
	}
	// In a shallow clone the previous commit may be there as an object
	// and still not be reachable from the new one: the history between
	// them was not fetched.
	reachable := func() bool {
		_, err := library.GitContext(ctx, clone, "merge-base", "--is-ancestor", r.From, r.To)
		return err == nil
	}
	if !reachable() {
		if ref == "" {
			ref = "HEAD"
		}
		library.GitContext(ctx, clone, "fetch", "-q", fmt.Sprintf("--deepen=%d", historyDepth), "origin", ref)
		if !reachable() {
			r.CommitsUnknown = true
			return
		}
	}
	args := append([]string{"log", "--no-merges", "--format=%H%x1f%s", fmt.Sprintf("-n%d", MaxCommits+1), r.From + ".." + r.To, "--"}, specs...)
	out, err := library.GitContext(ctx, clone, args...)
	if err != nil {
		r.CommitsUnknown = true
		return
	}
	for _, line := range strings.Split(out, "\n") {
		h, subj, ok := strings.Cut(line, "\x1f")
		if !ok {
			continue
		}
		if len(r.Commits) == MaxCommits {
			r.MoreCommits = true
			break
		}
		r.Commits = append(r.Commits, UpstreamCommit{Hash: h, Subject: subj})
	}
}

func globEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`)
	return r.Replace(s)
}

// appendHistory adds records to the history file, one JSON line each.
func (s *Session) appendHistory(recs []*UpdateRecord) error {
	if len(recs) == 0 {
		return nil
	}
	var b []byte
	for _, r := range recs {
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		b = append(append(b, line...), '\n')
	}
	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// History returns the recorded updates, newest first: of one pack (repo
// or alias, or a repository no longer subscribed) when pack is set, at
// most n when n > 0. It reads only the library, never the network. Lines
// that cannot be read (a merge gone wrong) are skipped and counted in
// skipped.
func (s *Session) History(packName string, n int) (recs []UpdateRecord, skipped int, err error) {
	repo := packName
	if p := s.Lib.Manifest.FindPack(packName); p != nil {
		repo = p.Repo
	}
	f, err := os.Open(s.HistoryPath())
	if errors.Is(err, os.ErrNotExist) {
		return []UpdateRecord{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r UpdateRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil || r.Repo == "" {
			skipped++
			continue
		}
		if repo == "" || r.Repo == repo {
			recs = append(recs, r)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, skipped, err
	}
	// Newest first; lines from two machines merged by git may be out of
	// order.
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Time.After(recs[j].Time) })
	if n > 0 && len(recs) > n {
		recs = recs[:n]
	}
	if recs == nil {
		recs = []UpdateRecord{}
	}
	return recs, skipped, nil
}

// RecordRows are the labelled skill lists of a record that are not empty,
// in the order front ends show them: {"changed", "tdd, review"}.
func RecordRows(r UpdateRecord) [][2]string {
	var out [][2]string
	for _, row := range []struct {
		label string
		names []string
	}{{"changed", r.Changed}, {"added", r.Added}, {"moved", r.Moved}, {"upstream gone", r.Gone},
		{"dropped", r.Dropped}, {"upstream new", r.New}} {
		if len(row.names) > 0 {
			out = append(out, [2]string{row.label, strings.Join(row.names, ", ")})
		}
	}
	return out
}

// RecordSummary is a one-line count of a record: "2 changed · 1 added".
func RecordSummary(r UpdateRecord) string {
	var parts []string
	for _, c := range []struct {
		n    int
		word string
	}{{len(r.Changed), "changed"}, {len(r.Added), "added"}, {len(r.Moved), "moved"}, {len(r.Gone), "gone"},
		{len(r.Dropped), "dropped"}, {len(r.New), "new upstream"}} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.word))
		}
	}
	if len(parts) == 0 {
		return "new commit, no skill changes"
	}
	return strings.Join(parts, " · ")
}
