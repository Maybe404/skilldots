package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// HistoryFile is the update history, at the library root. It is part of
// the library: committed with it and pushed by `skd sync`, so a clone on
// another machine has it too.
const HistoryFile = "skills.history.jsonl"

// MaxCommits is how many upstream commit subjects one record keeps.
const MaxCommits = 20

// MaxSubject is how many characters of an upstream commit subject a
// record keeps; MaxName is the same for the repository and skill names.
const (
	MaxSubject = 200
	MaxName    = 200
)

// maxLine is the longest history line read; a longer one (nothing skd
// writes comes close) is skipped like an unreadable one.
const maxLine = 1 << 20

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
	// ApplyError is set when the update was saved (vendor/ and the lock
	// moved to To) but linking it into the agents failed: skd apply
	// finishes it, and once an apply succeeds it is taken out of the
	// record (settleApplyErrors).
	ApplyError string `json:"applyError,omitempty"`
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

// sanitize makes everything in r that comes from upstream or the manifest
// safe to print and bounded: control characters (ESC, BEL, …) are taken
// out and long text is cut. It runs when a record is written and again
// when it is read (a line written by an older skd, or merged in by git).
func (r *UpdateRecord) sanitize() {
	r.Repo = CleanText(r.Repo, MaxName)
	r.From, r.To = CleanText(r.From, MaxName), CleanText(r.To, MaxName)
	for _, l := range []*[]string{&r.Changed, &r.Added, &r.Moved, &r.Gone, &r.Dropped, &r.New} {
		out := []string{}
		for _, n := range *l {
			out = append(out, CleanText(n, MaxName))
		}
		*l = out
	}
	if r.Commits == nil {
		r.Commits = []UpstreamCommit{}
	}
	for i := range r.Commits {
		r.Commits[i].Hash = CleanText(r.Commits[i].Hash, MaxName)
		r.Commits[i].Subject = CleanText(r.Commits[i].Subject, MaxSubject)
	}
	r.ApplyError = CleanText(r.ApplyError, MaxSubject)
}

// CleanText is s fit to print on one terminal line: tabs and line breaks
// become spaces, other control characters (C0, DEL, C1) and the bidi
// controls that reorder text are dropped, invalid UTF-8 becomes U+FFFD,
// and with max > 0 the result is cut to max characters with "…". It is
// safetext.Clean, for front ends that already use app.
func CleanText(s string, max int) string { return safetext.Clean(s, max) }

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
		_, err := library.GitContext(ctx, clone, "merge-base", "--is-ancestor", "--end-of-options", r.From, r.To)
		return err == nil
	}
	if !reachable() {
		if ref == "" {
			ref = "HEAD"
		}
		library.GitContext(ctx, clone, "fetch", "-q", fmt.Sprintf("--deepen=%d", historyDepth), "--", "origin", ref)
		if !reachable() {
			r.CommitsUnknown = true
			return
		}
	}
	args := append([]string{"log", "--no-merges", "--format=%H%x1f%s", fmt.Sprintf("-n%d", MaxCommits+1), "--end-of-options", r.From + ".." + r.To, "--"}, specs...)
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
		// Upstream's text: sanitize cleans and cuts it before it is written.
		r.Commits = append(r.Commits, UpstreamCommit{Hash: h, Subject: subj})
	}
}

func globEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`)
	return r.Replace(s)
}

// appendHistory adds records to the history file, one JSON line each. A
// file that does not end in a line break (edited by hand, or a write cut
// short) gets one first, so the new record starts on its own line.
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
	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if fi, err := f.Stat(); err == nil && fi.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, fi.Size()-1); err == nil && last[0] != '\n' {
			b = append([]byte{'\n'}, b...)
		}
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// settleApplyErrors takes applyError out of the history once an apply
// has linked everything: the updates it was about are finished, and the
// history should stop telling the person to run skd apply. Only those
// lines change (their other fields, unknown ones included, are kept);
// every other line stays byte for byte. A failure is only warned about.
func (s *Session) settleApplyErrors() {
	p := s.HistoryPath()
	b, err := os.ReadFile(p)
	key := []byte(`"applyError"`)
	if err != nil || !bytes.Contains(b, key) {
		return
	}
	lines := bytes.SplitAfter(b, []byte("\n"))
	changed := false
	for i, l := range lines {
		if !bytes.Contains(l, key) || len(l) > maxLine {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(l, &m) != nil {
			continue
		}
		if _, ok := m["applyError"]; !ok {
			continue
		}
		delete(m, "applyError")
		nl, err := json.Marshal(m)
		if err != nil {
			continue
		}
		if bytes.HasSuffix(l, []byte("\n")) {
			nl = append(nl, '\n')
		}
		lines[i], changed = nl, true
	}
	if !changed {
		return
	}
	if err := fsutil.WriteAtomic(p, bytes.Join(lines, nil)); err != nil {
		s.logf(Warn, "could not mark the update history linked (%s): %v", HistoryFile, err)
	}
}

// History returns the recorded updates, newest first: of one pack (repo
// or alias, or a repository no longer subscribed) when pack is set, at
// most n when n > 0. It reads only the library, never the network. Lines
// that cannot be read (a merge gone wrong, or longer than any record skd
// writes) are skipped and counted in skipped. Records are sanitized
// (CleanText) as they are read.
func (s *Session) History(packName string, n int) (recs []UpdateRecord, skipped int, err error) {
	repo := packName
	if p := s.Lib.Manifest.FindPack(packName); p != nil {
		repo = p.Repo
	}
	repo = CleanText(repo, MaxName)
	f, err := os.Open(s.HistoryPath())
	if errors.Is(err, os.ErrNotExist) {
		return []UpdateRecord{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 64*1024)
	for {
		raw, tooLong, rerr := readLine(br, maxLine)
		if line := strings.TrimSpace(string(raw)); tooLong || line != "" {
			var r UpdateRecord
			if tooLong || json.Unmarshal([]byte(line), &r) != nil || r.Repo == "" {
				skipped++
			} else if r.sanitize(); repo == "" || r.Repo == repo {
				recs = append(recs, r)
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return nil, skipped, rerr
		}
	}
	// Newest first; lines from two machines merged by git may be out of
	// order. Records of the same second keep the file's order reversed:
	// the later line is the later update.
	slices.Reverse(recs)
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Time.After(recs[j].Time) })
	if n > 0 && len(recs) > n {
		recs = recs[:n]
	}
	if recs == nil {
		recs = []UpdateRecord{}
	}
	return recs, skipped, nil
}

// readLine reads one line of any length. A line longer than max is not
// kept: tooLong is set and the rest of it is read past.
func readLine(br *bufio.Reader, max int) (line []byte, tooLong bool, err error) {
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(line)+len(chunk) > max {
				tooLong, line = true, nil
			} else {
				line = append(line, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, tooLong, err
	}
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
