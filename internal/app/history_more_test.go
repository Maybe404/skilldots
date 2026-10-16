package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Maybe404/skilldots/internal/app"
)

// commitFile commits with the message read from a file: a message too long
// for the command line, or with bytes the shell would mangle.
func commitFile(t *testing.T, repo, msg string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "msg")
	if err := os.WriteFile(f, []byte(msg), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--cleanup=verbatim", "-F", f)
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) })
}

// Upstream commit subjects are someone else's text: control characters
// never reach the record (nor a terminal printing it), and a huge subject
// is cut when written, so the file stays small and readable.
func TestHistoryCleansUpstreamSubjects(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "a.md"), "a\n")
	commitFile(t, repo, "tdd: \x1b]0;pwned\x07title \x1b[31mred\x1b[0m‮evil\x9b2J")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "b.md"), "b\n")
	commitFile(t, repo, strings.Repeat("x", 5<<20))

	s, _ := app.Open(root)
	out, err := s.Update(nil, nil)
	if err != nil || len(out.History) != 1 {
		t.Fatalf("update: %v %+v", err, out)
	}
	fi, err := os.Stat(filepath.Join(root, app.HistoryFile))
	if err != nil || fi.Size() > 4096 {
		t.Fatalf("history file: %v, %d bytes", err, fi.Size())
	}
	// Cleaned when written, not only when read: the file is committed and
	// read by other tools (and older skd versions).
	raw, _ := os.ReadFile(filepath.Join(root, app.HistoryFile))
	var onDisk app.UpdateRecord
	if err := json.Unmarshal(raw, &onDisk); err != nil || len(onDisk.Commits) != 2 {
		t.Fatalf("on disk: %v %s", err, raw)
	}
	for _, c := range onDisk.Commits {
		if hasControl(c.Subject) || strings.ContainsRune(c.Subject, '‮') || utf8.RuneCountInString(c.Subject) > app.MaxSubject+1 {
			t.Errorf("written uncleaned: %q", c.Subject)
		}
	}
	s, _ = app.Open(root)
	recs, skipped, err := s.History("", 0)
	if err != nil || skipped != 0 || len(recs) != 1 || len(recs[0].Commits) != 2 {
		t.Fatalf("History: %v %d %+v", err, skipped, recs)
	}
	long, evil := recs[0].Commits[0].Subject, recs[0].Commits[1].Subject
	if n := utf8.RuneCountInString(long); n > app.MaxSubject+1 || !strings.HasSuffix(long, "…") {
		t.Errorf("long subject kept %d characters", n)
	}
	if hasControl(evil) || strings.ContainsRune(evil, '‮') || !strings.Contains(evil, "title") || !strings.Contains(evil, "red") {
		t.Errorf("subject not cleaned: %q", evil)
	}
}

// What is already in the file is cleaned when read too (written by an
// older skd, or merged in by git), and a line longer than any record skd
// writes is skipped, not fatal.
func TestHistoryReadsBadFiles(t *testing.T) {
	up := upstream(t)
	_, root := side(t, up)
	rec := func(repo, when, subject string) string {
		b, _ := json.Marshal(map[string]any{"time": when, "repo": repo, "from": "a", "to": "b",
			"changed": []string{"tdd\x1b[2J"}, "commits": []map[string]string{{"hash": "c", "subject": subject}}})
		return string(b)
	}
	// Out of order, as two machines' lines merged by git can be.
	lines := []string{
		rec("acme/skills", "2024-01-01T00:00:00Z", "old \x1b[31mred"),
		`{"repo":"` + strings.Repeat("y", 5<<20) + `"}`,
		rec("acme/skills", "2026-01-01T00:00:00Z", "new"),
		rec("acme/skills", "2025-01-01T00:00:00Z", "middle"),
		"",
	}
	write(t, filepath.Join(root, app.HistoryFile), strings.Join(lines, "\n"))
	s, _ := app.Open(root)
	for _, pack := range []string{"", "acme/skills"} {
		recs, skipped, err := s.History(pack, 0)
		if err != nil || skipped != 1 || len(recs) != 3 {
			t.Fatalf("History(%q): %v skipped %d %+v", pack, err, skipped, recs)
		}
		// Newest first, whatever the order in the file.
		if got := []string{recs[0].Commits[0].Subject, recs[1].Commits[0].Subject, recs[2].Commits[0].Subject[:3]}; !slices.Equal(got, []string{"new", "middle", "old"}) {
			t.Errorf("not newest first: %q", got)
		}
		for _, r := range recs {
			if hasControl(r.Commits[0].Subject) || hasControl(r.Changed[0]) {
				t.Errorf("not cleaned on read: %+v", r)
			}
		}
	}
}

// Every update adds its record to the ones before (never replaces the
// file), newest first; a file that lost its last line break (edited by
// hand) does not glue the next record onto the last one.
func TestHistoryAccumulates(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	var tos []string
	for i, f := range []string{"a.md", "b.md"} {
		write(t, filepath.Join(repo, "skills", "eng", "tdd", f), f)
		commit(t, repo, "tdd: "+f)
		s, _ := app.Open(root)
		out, err := s.Update(nil, nil)
		if err != nil || len(out.History) != 1 {
			t.Fatalf("update %d: %v %+v", i, err, out)
		}
		tos = append(tos, out.History[0].To)
	}
	p := filepath.Join(root, app.HistoryFile)
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.TrimRight(string(b), "\n")), 0o644)
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "c.md"), "c")
	commit(t, repo, "tdd: c.md")
	s, _ := app.Open(root)
	out, err := s.Update(nil, nil)
	if err != nil || len(out.History) != 1 {
		t.Fatalf("third update: %v %+v", err, out)
	}
	tos = append(tos, out.History[0].To)
	s, _ = app.Open(root)
	recs, skipped, err := s.History("acme/skills", 0)
	if err != nil || skipped != 0 || len(recs) != 3 {
		t.Fatalf("History: %v skipped %d %+v", err, skipped, recs)
	}
	for i, r := range recs {
		if want := tos[len(tos)-1-i]; r.To != want {
			t.Errorf("record %d is %s, want %s (newest first)", i, r.To, want)
		}
	}
}

// When the lock cannot be saved the update did not happen (the lock still
// names the old commit): nothing is recorded, nothing linked; the next
// update records it.
func TestHistoryNotRecordedWhenLockSaveFails(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "a.md"), "a\n")
	commit(t, repo, "tdd: a")
	s, _ := app.Open(root)
	// A directory where the lock goes: saving it fails.
	lockFile := filepath.Join(root, "skills.lock")
	os.Remove(lockFile)
	write(t, filepath.Join(lockFile, "keep"), "x")
	out, err := s.Update(nil, nil)
	if err == nil {
		t.Fatal("update saved a lock over a directory")
	}
	if _, serr := os.Stat(filepath.Join(root, app.HistoryFile)); serr == nil || len(out.History) != 0 || out.Apply != nil {
		t.Fatalf("recorded or applied an update whose lock was not saved: %v %+v", serr, out)
	}
}

// A record that says linking failed while everything is linked (merged
// in from another machine, or linked by hand): an apply with nothing to
// do settles it too.
func TestApplyWithNothingToDoSettlesApplyErrors(t *testing.T) {
	up := upstream(t)
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	p := filepath.Join(root, app.HistoryFile)
	other := `{"time":"2026-01-01T00:00:00Z","repo":"acme/skills","from":"a","to":"b","applyError":"permission denied"}`
	write(t, p, other+"\nnot json \"applyError\"\n")
	s, _ := app.Open(root)
	out, err := s.Apply(app.ApplyOptions{})
	if err != nil || out.Applied {
		t.Fatalf("apply had something to do: %v %+v", err, out)
	}
	b, _ := os.ReadFile(p)
	if got := string(b); strings.Contains(strings.SplitN(got, "\n", 2)[0], "applyError") || !strings.HasSuffix(got, "not json \"applyError\"\n") {
		t.Fatalf("history after apply:\n%s", got)
	}
}

// Linking can fail after the update is saved (vendor/ and the lock are at
// the new commit): the update happened, so it is recorded, marked.
func TestHistoryRecordsApplyFailure(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	home, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--all")
	write(t, filepath.Join(repo, "skills", "eng", "fresh", "SKILL.md"), skillMD("fresh"))
	commit(t, repo, "fresh")
	dir := filepath.Join(home, ".agents", "skills")
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	s, _ := app.Open(root)
	out, err := s.Update(nil, nil)
	if err == nil {
		t.Skip("the target directory is writable anyway (running as root?)")
	}
	if len(out.History) != 1 || out.History[0].ApplyError == "" {
		t.Fatalf("history: %+v", out.History)
	}
	s, _ = app.Open(root)
	recs, _, _ := s.History("", 0)
	if len(recs) != 1 || recs[0].ApplyError == "" || recs[0].To != s.Lib.Lock.Packs["acme/skills"].Commit {
		t.Fatalf("recorded: %+v", recs)
	}
	// Once an apply links everything the record stops saying it failed;
	// the rest of the line (an unknown field too) is kept.
	p := filepath.Join(root, app.HistoryFile)
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(b), `{"time"`, `{"future":1,"time"`, 1)), 0o644)
	os.Chmod(dir, 0o755)
	if _, err := s.Apply(app.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	recs, _, _ = s.History("", 0)
	b, _ = os.ReadFile(p)
	if len(recs) != 1 || recs[0].ApplyError != "" || strings.Contains(string(b), "applyError") || !strings.Contains(string(b), `"future":1`) || recs[0].Changed == nil {
		t.Fatalf("after apply: %+v\n%s", recs, b)
	}
}
