package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/lock"
)

// longSkill is a SKILL.md with lines far enough apart that changes at
// the top and at the bottom merge cleanly.
func longSkill(name string, top, bottom string) string {
	return "---\nname: " + name + "\ndescription: " + name + " skill\n---\n" + top + "\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n" + bottom + "\n"
}

// mergeFixture is a library following tdd and review of acme/skills, both
// with long SKILL.md files, updated once so both are vendored.
func mergeFixture(t *testing.T) (repo, root, tdd string) {
	t.Helper()
	up := upstream(t)
	repo = filepath.Join(up, "acme", "skills.git")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), longSkill("tdd", "top", "bottom"))
	write(t, filepath.Join(repo, "skills", "eng", "review", "SKILL.md"), longSkill("review", "top", "bottom"))
	commit(t, repo, "long skills")
	_, root = side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd,review")
	return repo, root, filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd")
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func lockSkill(t *testing.T, root, name string) *lock.Skill {
	t.Helper()
	l, err := lock.Load(filepath.Join(root, lock.FileName))
	if err != nil {
		t.Fatal(err)
	}
	ls := l.Packs["acme/skills"].Skills[name]
	if ls == nil {
		t.Fatalf("%s not in the lock", name)
	}
	return ls
}

func update(t *testing.T, root string) *engine.SyncReport {
	t.Helper()
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Update(nil, nil)
	if err != nil || len(out.Reports) != 1 {
		t.Fatalf("update: %v %+v", err, out)
	}
	return out.Reports[0]
}

func upstreamHash(t *testing.T, repo, rel string) string {
	t.Helper()
	h, err := engine.ReadFileSet(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return h.Hash()
}

// The upstream snapshot holds exactly what a pristine vendor/<pack> holds.
func TestSnapshotMatchesVendor(t *testing.T) {
	_, root, _ := mergeFixture(t)
	ref := "refs/skilldots/upstream/acme/skills"
	snap, err := engine.ReadGitFileSet(context.Background(), root, ref, "")
	if err != nil {
		t.Fatal(err)
	}
	vendored, err := engine.ReadFileSet(filepath.Join(root, "vendor", "acme", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(snap.Paths(), vendored.Paths()) || snap.Hash() != vendored.Hash() {
		t.Errorf("snapshot %v\nvendor %v", snap.Paths(), vendored.Paths())
	}
	if msg := gitOut(t, root, "log", "-1", "--format=%s", ref); !strings.HasPrefix(msg, "upstream acme/skills ") {
		t.Errorf("message %q", msg)
	}
	// The library's index and work tree are not touched.
	if st := gitOut(t, root, "status", "--porcelain", "--", "."); strings.Contains(st, "A ") {
		t.Errorf("status:\n%s", st)
	}
}

// A clean merge keeps the local change and takes upstream's.
func TestUpdateMergesLocalChanges(t *testing.T) {
	repo, root, tdd := mergeFixture(t)
	write(t, filepath.Join(tdd, "SKILL.md"), longSkill("tdd", "top mine", "bottom"))
	write(t, filepath.Join(tdd, "notes.md"), "my notes\n")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), longSkill("tdd", "top", "bottom upstream"))
	commit(t, repo, "tdd: bottom")

	rep := update(t, root)
	if !slices.Equal(rep.Merged, []string{"tdd"}) || len(rep.Shelved) != 0 {
		t.Fatalf("merged %v shelved %v", rep.Merged, rep.Shelved)
	}
	if got := read(t, filepath.Join(tdd, "SKILL.md")); got != longSkill("tdd", "top mine", "bottom upstream") {
		t.Errorf("merged SKILL.md:\n%s", got)
	}
	if read(t, filepath.Join(tdd, "notes.md")) != "my notes\n" {
		t.Error("a local file was lost")
	}
	if ls := lockSkill(t, root, "tdd"); ls.Hash != upstreamHash(t, repo, "skills/eng/tdd") || ls.Shelved != "" {
		t.Errorf("lock %+v", ls)
	}
	// The record lists it once: merged, not changed as well.
	if s, _ := app.Open(root); s != nil {
		recs, _, _ := s.History("acme/skills", 1)
		if len(recs) != 1 || !slices.Equal(recs[0].Merged, []string{"tdd"}) || len(recs[0].Changed) != 0 || len(recs[0].Commits) != 1 {
			t.Errorf("record %+v", recs)
		}
	}
	// The copy before the merge is in the trash, and restoring it swaps
	// it back (the lock goes back too, so the next update merges again).
	s, _ := app.Open(root)
	items, _ := s.Trash()
	var id string
	for _, it := range items {
		if it.Reason == engine.ReasonMerged && slices.Contains(it.Skills, "tdd") {
			id = it.ID
			if !it.Restorable {
				t.Errorf("not restorable: %+v", it)
			}
		}
	}
	if id == "" {
		t.Fatalf("no merge backup in the trash: %+v", items)
	}
	before := lockSkill(t, root, "tdd").Hash
	if _, err := s.RestoreEntry(id); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(tdd, "SKILL.md")); got != longSkill("tdd", "top mine", "bottom") {
		t.Errorf("restored SKILL.md:\n%s", got)
	}
	if ls := lockSkill(t, root, "tdd"); ls.Hash == before {
		t.Errorf("lock not put back: %+v", ls)
	}
	rep = update(t, root)
	if !slices.Equal(rep.Merged, []string{"tdd"}) {
		t.Errorf("merged again: %v", rep.Merged)
	}
}

// shelve makes tdd conflict: both sides change its top line. review only
// changes upstream.
func shelve(t *testing.T) (repo, root, tdd string, rep *engine.SyncReport) {
	t.Helper()
	repo, root, tdd = mergeFixture(t)
	write(t, filepath.Join(tdd, "SKILL.md"), longSkill("tdd", "top mine", "bottom"))
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), longSkill("tdd", "top upstream", "bottom"))
	write(t, filepath.Join(repo, "skills", "eng", "review", "SKILL.md"), longSkill("review", "top", "bottom new"))
	commit(t, repo, "tdd and review")
	return repo, root, tdd, update(t, root)
}

func TestConflictShelves(t *testing.T) {
	repo, root, tdd, rep := shelve(t)
	if len(rep.Shelved) != 1 || rep.Shelved[0].Name != "tdd" || !slices.Equal(rep.Shelved[0].Files, []string{"SKILL.md"}) {
		t.Fatalf("shelved %+v", rep.Shelved)
	}
	if slices.Contains(rep.Changed, "tdd") || !slices.Contains(rep.Changed, "review") {
		t.Errorf("changed %v", rep.Changed)
	}
	// The directory and its lock entry are as they were; the pack moved on.
	if got := read(t, filepath.Join(tdd, "SKILL.md")); got != longSkill("tdd", "top mine", "bottom") {
		t.Errorf("tdd touched:\n%s", got)
	}
	head := gitOut(t, repo, "rev-parse", "HEAD")
	ls := lockSkill(t, root, "tdd")
	if ls.Shelved != head || ls.Upstream != upstreamHash(t, repo, "skills/eng/tdd") || ls.Hash == ls.Upstream || !slices.Equal(ls.Conflicts, []string{"SKILL.md"}) {
		t.Errorf("lock %+v", ls)
	}
	l, _ := lock.Load(filepath.Join(root, lock.FileName))
	if l.Packs["acme/skills"].Commit != head {
		t.Error("the pack's commit did not advance")
	}
	if got := read(t, filepath.Join(root, "vendor", "acme", "skills", "eng", "review", "SKILL.md")); !strings.Contains(got, "bottom new") {
		t.Error("review was not updated")
	}
	// Listed, counted as still having an update, and recorded.
	s, _ := app.Open(root)
	sh, err := s.Shelved()
	if err != nil || len(sh) != 1 || sh[0].Name != "tdd" || sh[0].Resolving {
		t.Errorf("shelved %+v %v", sh, err)
	}
	if rec := s.RecordedOutdated(); rec != nil {
		if st := rec.Find("acme/skills"); st == nil || !st.HasUpdates() || !slices.Equal(st.Shelved, []string{"tdd"}) {
			t.Errorf("recorded %+v", st)
		}
	}
	o, err := s.Outdated(nil, nil)
	if err != nil || o.Updates() != 1 || !slices.Equal(o.Packs[0].Shelved, []string{"tdd"}) {
		t.Errorf("outdated %+v %v", o, err)
	}
	recs, _, _ := s.History("acme/skills", 1)
	if len(recs) != 1 || !slices.Equal(recs[0].Shelved, []string{"tdd"}) || recs[0].SnapTo == "" {
		t.Errorf("history %+v", recs)
	}
	// Updating again keeps it shelved, still at the old hash.
	update(t, root)
	if ls2 := lockSkill(t, root, "tdd"); ls2.Hash != ls.Hash || ls2.Shelved != head {
		t.Errorf("after a second update %+v", ls2)
	}
}

func TestResolveFlow(t *testing.T) {
	repo, root, tdd, _ := shelve(t)
	s, _ := app.Open(root)
	ref := app.KeyRef("tdd")
	out, err := s.ResolveStart(ref)
	if err != nil {
		t.Fatal(err)
	}
	if out.Merged || len(out.Conflicts) != 1 || out.Conflicts[0].Path != "SKILL.md" || out.Backup == "" {
		t.Fatalf("start %+v", out)
	}
	text := read(t, filepath.Join(tdd, "SKILL.md"))
	if !strings.Contains(text, "<<<<<<< local\ntop mine\n=======\ntop upstream\n>>>>>>> upstream ") {
		t.Fatalf("markers:\n%s", text)
	}
	if st := gitOut(t, root, "status", "--porcelain"); !strings.Contains(st, "UU vendor/acme/skills/eng/tdd/SKILL.md") {
		t.Errorf("status:\n%s", st)
	}
	files, err := s.ConflictFiles(ref)
	if err != nil || len(files) != 1 || len(files[0].Blocks) != 1 || files[0].Blocks[0].Local != "top mine\n" || files[0].Blocks[0].Upstream != "top upstream\n" {
		t.Fatalf("files %+v %v", files, err)
	}
	// Still conflicted: continue refuses.
	_, err = s.ResolveContinue(app.Ref{})
	var ue *app.UnresolvedError
	if !errors.As(err, &ue) || !slices.Equal(ue.Files, []string{"SKILL.md"}) {
		t.Fatalf("continue with markers: %v", err)
	}
	// Committing waits too.
	if _, err := s.Commit("x", false); err == nil {
		t.Error("committed conflict markers")
	}
	// git checkout --theirs works on the stages skd wrote.
	git(t, root, "checkout", "--theirs", "--", "vendor/acme/skills/eng/tdd/SKILL.md")
	if read(t, filepath.Join(tdd, "SKILL.md")) != longSkill("tdd", "top upstream", "bottom") {
		t.Error("checkout --theirs")
	}
	// Back to the conflict, then pick both sides.
	git(t, root, "checkout", "-m", "--", "vendor/acme/skills/eng/tdd/SKILL.md")
	if err := s.ResolvePick(ref, "SKILL.md", 0, app.PickBoth); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(tdd, "SKILL.md")); !strings.Contains(got, "top mine\ntop upstream\nl1") || engine.HasMarkers([]byte(got), true) {
		t.Errorf("picked:\n%s", got)
	}
	s, _ = app.Open(root)
	if _, err := s.ResolveContinue(ref); err != nil {
		t.Fatal(err)
	}
	ls := lockSkill(t, root, "tdd")
	if ls.Hash != upstreamHash(t, repo, "skills/eng/tdd") || ls.Shelved != "" || ls.Resolving || ls.Backup != "" {
		t.Errorf("lock after continue %+v", ls)
	}
	if st := gitOut(t, root, "status", "--porcelain"); !strings.Contains(st, "M  vendor/acme/skills/eng/tdd/SKILL.md") && !strings.Contains(st, "A  vendor/acme/skills/eng/tdd/SKILL.md") {
		t.Errorf("not staged:\n%s", st)
	}
	recs, _, _ := s.History("acme/skills", 1)
	if len(recs) != 1 || !slices.Equal(recs[0].Merged, []string{"tdd"}) {
		t.Fatalf("history %+v", recs)
	}
	// The record of the resolve goes from the version the local changes
	// were made on to the one merged in, and its diff is that change.
	r := recs[0]
	head := gitOut(t, repo, "rev-parse", "HEAD")
	if r.SnapFrom == "" || r.SnapTo == "" || r.SnapFrom == r.SnapTo || r.To != head || r.From == r.To || r.From != gitOut(t, repo, "rev-parse", "HEAD~1") {
		t.Errorf("resolve record %+v (upstream head %s)", r, head)
	}
	d, err := s.HistoryDiff(context.Background(), &r, "")
	if err != nil || len(d.Skills) != 1 || d.Skills[0].Name != "tdd" || len(d.Skills[0].Files) != 1 {
		t.Fatalf("resolve diff %+v %v", d, err)
	}
	var lines []string
	for _, l := range d.Skills[0].Files[0].Hunks[0].Lines {
		lines = append(lines, l.Op+l.Text)
	}
	if got := strings.Join(lines, "\n"); !strings.Contains(got, "-top\n+top upstream") {
		t.Errorf("resolve diff:\n%s", got)
	}
	// Nothing waits any more.
	if sh, _ := s.Shelved(); len(sh) != 0 {
		t.Errorf("still shelved %+v", sh)
	}
}

func TestResolveAbort(t *testing.T) {
	_, root, tdd, _ := shelve(t)
	before := read(t, filepath.Join(tdd, "SKILL.md"))
	// Something of the user's staged before: kept through the abort.
	write(t, filepath.Join(tdd, "SKILL.md"), before)
	git(t, root, "add", "-A")
	indexBefore := gitOut(t, root, "ls-files", "-s")
	s, _ := app.Open(root)
	if _, err := s.ResolveStart(app.KeyRef("tdd")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tdd, "half.md"), "half done\n")
	out, err := s.ResolveAbort(app.Ref{})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(tdd, "SKILL.md")) != before {
		t.Error("not restored")
	}
	if _, err := os.Stat(filepath.Join(tdd, "half.md")); err == nil {
		t.Error("the aborted files are still there")
	}
	if out.Trash == "" {
		t.Error("the aborted files did not go to the trash")
	}
	if got := gitOut(t, root, "ls-files", "-s"); got != indexBefore {
		t.Errorf("index:\n%s\nwant\n%s", got, indexBefore)
	}
	ls := lockSkill(t, root, "tdd")
	if ls.Resolving || ls.Backup != "" || ls.Shelved == "" {
		t.Errorf("lock %+v", ls)
	}
	// It can be started again.
	s, _ = app.Open(root)
	if _, err := s.ResolveStart(app.KeyRef("tdd")); err != nil {
		t.Fatal(err)
	}
}

// A skill being resolved is left alone by an update.
func TestUpdateLeavesResolvingAlone(t *testing.T) {
	repo, root, tdd, _ := shelve(t)
	s, _ := app.Open(root)
	if _, err := s.ResolveStart(app.KeyRef("tdd")); err != nil {
		t.Fatal(err)
	}
	marked := read(t, filepath.Join(tdd, "SKILL.md"))
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "more.md"), "more\n")
	commit(t, repo, "tdd: more")
	rep := update(t, root)
	if len(rep.Shelved) != 1 || read(t, filepath.Join(tdd, "SKILL.md")) != marked {
		t.Errorf("shelved %+v", rep.Shelved)
	}
	if ls := lockSkill(t, root, "tdd"); !ls.Resolving {
		t.Errorf("lock %+v", ls)
	}
}

// A file removed on one side and changed on the other: the upstream copy
// is written beside it, and git sees it deleted by us.
func TestResolveDeleteModify(t *testing.T) {
	repo, root, tdd := mergeFixture(t)
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "extra.md"), "extra\n")
	commit(t, repo, "extra")
	update(t, root)
	os.Remove(filepath.Join(tdd, "extra.md"))
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "extra.md"), "extra changed\n")
	commit(t, repo, "extra changed")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "lib")
	rep := update(t, root)
	if len(rep.Shelved) != 1 || !slices.Equal(rep.Shelved[0].Files, []string{"extra.md"}) {
		t.Fatalf("shelved %+v", rep.Shelved)
	}
	s, _ := app.Open(root)
	out, err := s.ResolveStart(app.KeyRef("tdd"))
	if err != nil || len(out.Conflicts) != 1 || out.Conflicts[0].Kind != engine.ConflictDeletedLocal {
		t.Fatalf("start %+v %v", out, err)
	}
	if read(t, filepath.Join(tdd, "extra.md.upstream")) != "extra changed\n" {
		t.Error("no .upstream file")
	}
	if st := gitOut(t, root, "status", "--porcelain"); !strings.Contains(st, "DU vendor/acme/skills/eng/tdd/extra.md") {
		t.Errorf("status:\n%s", st)
	}
	if _, err := s.ResolveContinue(app.KeyRef("tdd")); err == nil {
		t.Error("continued with the .upstream file left")
	}
	if err := s.ResolvePick(app.KeyRef("tdd"), "extra.md", 0, app.PickUpstream); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(tdd, "extra.md")) != "extra changed\n" {
		t.Error("upstream not taken")
	}
	if _, err := s.ResolveContinue(app.KeyRef("tdd")); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryDiff(t *testing.T) {
	repo, root, _ := mergeFixture(t)
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), longSkill("tdd", "top", "bottom upstream"))
	commit(t, repo, "tdd: bottom")
	update(t, root)
	s, _ := app.Open(root)
	r, err := s.HistoryRecord("acme/skills", 1)
	if err != nil {
		t.Fatal(err)
	}
	check := func(d *app.RecordDiff, source string) {
		t.Helper()
		if d.Source != source || len(d.Skills) != 1 || d.Skills[0].Name != "tdd" || len(d.Skills[0].Files) != 1 {
			t.Fatalf("%s diff %+v", source, d)
		}
		f := d.Skills[0].Files[0]
		if f.Path != "SKILL.md" || f.Added != 1 || f.Removed != 1 {
			t.Errorf("%s file %+v", source, f)
		}
	}
	d, err := s.HistoryDiff(context.Background(), r, "")
	if err != nil {
		t.Fatal(err)
	}
	check(d, "snapshots")
	// A record from before snapshots: read from the cache clone, the old
	// commit fetched by its hash (the clone is shallow).
	old := *r
	old.SnapFrom, old.SnapTo = "", ""
	cache := os.Getenv("SKILLDOTS_CACHE")
	os.RemoveAll(cache)
	d, err = s.HistoryDiff(context.Background(), &old, "")
	if err != nil {
		t.Fatal(err)
	}
	check(d, "upstream")
	if _, err := s.HistoryDiff(context.Background(), r, "review"); err == nil {
		t.Error("a skill the update did not change")
	}
	// Found by repository and time, as the desktop asks.
	if got, err := s.FindRecord("acme/skills", r.Time); err != nil || got.To != r.To {
		t.Errorf("find %+v %v", got, err)
	}
}

// A lock written before these fields reads and updates as before.
func TestOldLockWithoutMergeFields(t *testing.T) {
	repo, root, _ := mergeFixture(t)
	b := read(t, filepath.Join(root, lock.FileName))
	for _, k := range []string{"shelved", "resolving", "conflicts", "backup"} {
		if strings.Contains(b, k+" =") {
			t.Fatalf("the lock has %s:\n%s", k, b)
		}
	}
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), longSkill("tdd", "top", "bottom 2"))
	commit(t, repo, "tdd")
	if rep := update(t, root); !slices.Equal(rep.Changed, []string{"tdd"}) {
		t.Errorf("changed %v", rep.Changed)
	}
}

// A library that is not a git repository still merges and shelves; it
// keeps no snapshots and says so, without failing.
func TestMergeWithoutGit(t *testing.T) {
	repo, root, tdd := mergeFixture(t)
	os.RemoveAll(filepath.Join(root, ".git"))
	write(t, filepath.Join(tdd, "SKILL.md"), longSkill("tdd", "top mine", "bottom"))
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), longSkill("tdd", "top", "bottom upstream"))
	commit(t, repo, "tdd: bottom")
	rep := update(t, root)
	// No common version: SKILL.md changed on both sides conflicts.
	if len(rep.Shelved) != 1 || rep.SnapTo != "" {
		t.Fatalf("shelved %+v snap %q", rep.Shelved, rep.SnapTo)
	}
	warned := false
	for _, w := range rep.Warnings {
		warned = warned || strings.Contains(w, "not a git repository")
	}
	if !warned {
		t.Errorf("warnings %v", rep.Warnings)
	}
	s, _ := app.Open(root)
	out, err := s.ResolveStart(app.KeyRef("tdd"))
	if err != nil || len(out.Conflicts) != 1 {
		t.Fatalf("start %+v %v", out, err)
	}
	// Without a base each run of differing lines is a conflict of its own.
	for i := 0; ; i++ {
		files, err := s.ConflictFiles(app.KeyRef("tdd"))
		if err != nil || i > 5 {
			t.Fatalf("%+v %v", files, err)
		}
		if len(files[0].Blocks) == 0 {
			break
		}
		if err := s.ResolvePick(app.KeyRef("tdd"), "SKILL.md", 0, app.PickUpstream); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, filepath.Join(tdd, "SKILL.md")); got != longSkill("tdd", "top", "bottom upstream") {
		t.Errorf("picked upstream:\n%s", got)
	}
	if _, err := s.ResolveContinue(app.KeyRef("tdd")); err != nil {
		t.Fatal(err)
	}
}

// A record written when a merged skill was listed under changed too
// reads with it once, under merged.
func TestHistoryListsMergedOnce(t *testing.T) {
	_, root, _ := mergeFixture(t)
	line := `{"time":"2026-01-02T03:04:05Z","repo":"acme/skills","from":"aaaaaaa","to":"bbbbbbb","changed":["tdd","review"],"added":[],"moved":[],"gone":[],"dropped":[],"new":[],"commits":[],"merged":["tdd"]}` + "\n"
	f, err := os.OpenFile(filepath.Join(root, app.HistoryFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(line)
	f.Close()
	s, _ := app.Open(root)
	recs, _, _ := s.History("acme/skills", 0)
	var got *app.UpdateRecord
	for i := range recs {
		if recs[i].From == "aaaaaaa" {
			got = &recs[i]
		}
	}
	if got == nil || !slices.Equal(got.Changed, []string{"review"}) || !slices.Equal(got.Merged, []string{"tdd"}) {
		t.Fatalf("record %+v", got)
	}
	if rows := app.RecordSummary(*got); strings.Count(rows, "1") != 2 {
		t.Errorf("summary %q", rows)
	}
}
