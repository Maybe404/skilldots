package app_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// ownFixture: acme/skills is a bare repository on the fake GitHub that
// the library follows whole, marked as the user's own.
func ownFixture(t *testing.T) (bare, root string) {
	t.Helper()
	up := upstream(t)
	bare = bareUpstream(t, filepath.Join(up, "acme", "skills.git"))
	_, root = side(t, up)
	t.Cleanup(app.SetGitHub(&fakeGH{status: app.GHOK, canPush: true, states: map[string]string{}}))
	runCLI(t, root, "add", "acme/skills", "--all", "--own")
	return bare, root
}

func entry(t *testing.T, s *app.Session, name string) engine.Entry {
	t.Helper()
	entries, err := s.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	s.MarkEdited(entries)
	for _, e := range entries {
		if e.LinkName == name {
			return e
		}
	}
	t.Fatalf("no entry %s", name)
	return engine.Entry{}
}

// A change pushed upstream comes back with the next update as "landed":
// not changed, not merged, nothing set aside in the trash, and a check
// before it does not count it as an update.
func TestLandedAfterPush(t *testing.T) {
	_, _, root, tdd, _ := contribFixture(t)
	s, _ := app.Open(root)
	if _, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, ""); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	o, err := s.Outdated(nil, nil)
	if err != nil || len(o.Packs) != 1 {
		t.Fatalf("outdated %+v %v", o, err)
	}
	if st := o.Packs[0]; !slices.Equal(st.Landed, []string{"tdd"}) || len(st.Changed) != 0 || st.HasUpdates() {
		t.Fatalf("status %+v", st)
	}
	rep := update(t, root)
	if !slices.Equal(rep.Landed, []string{"tdd"}) || len(rep.Changed)+len(rep.Merged)+len(rep.Backups) != 0 {
		t.Fatalf("report %+v", rep)
	}
	h, _ := fsutil.HashTree(tdd)
	if ls := lockSkill(t, root, "tdd"); ls.Hash != h || ls.Contribution != nil {
		t.Errorf("lock %+v", ls)
	}
	s, _ = app.Open(root)
	recs, _, _ := s.History("acme/skills", 1)
	if len(recs) != 1 || !slices.Equal(recs[0].Landed, []string{"tdd"}) || len(recs[0].Changed) != 0 {
		t.Errorf("history %+v", recs)
	}
}

// skd new --in makes a draft in the own repository; contribute <pack>
// --push sends it with an edit in one commit and pulls them back, after
// which nothing is changed or new.
func TestOwnNewAndPushPack(t *testing.T) {
	bare, root := ownFixture(t)
	s, _ := app.Open(root)
	if !s.Lib.Manifest.FindPack("acme/skills").Own {
		t.Fatal("not marked own")
	}
	out, err := s.NewInPack("notes", "acme/skills", app.NewInPackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Beside most of the skills: skills/eng and skills/prod have two
	// each, the first wins.
	if out.RepoPath != "skills/eng/notes" || out.Path != filepath.Join(root, "vendor", "acme", "skills", "eng", "notes") {
		t.Fatalf("new %+v", out)
	}
	if ls := lockSkill(t, root, "notes"); !ls.Draft || ls.Hash != "" || ls.Path != "skills/eng/notes" || ls.Category != "eng" {
		t.Fatalf("lock %+v", ls)
	}
	if l, err := os.Readlink(filepath.Join(os.Getenv("HOME"), ".agents", "skills", "notes")); err != nil || !strings.HasSuffix(l, filepath.Join("eng", "notes")) {
		t.Errorf("link %q %v", l, err)
	}
	s, _ = app.Open(root)
	if e := entry(t, s, "notes"); !e.Draft || !e.Own || !e.Edited {
		t.Errorf("entry %+v", e)
	}
	d, err := s.LocalChanges(context.Background(), app.KeyRef("notes"))
	if err != nil || !d.Base || len(d.Files) != 1 || d.Files[0].Removed != 0 {
		t.Errorf("diff %+v %v", d, err)
	}
	// An update keeps it, and says nothing about it.
	rep := update(t, root)
	if len(rep.UpstreamGone)+len(rep.Dropped) != 0 || !fsutil.Exists(out.Path) {
		t.Fatalf("update %+v", rep)
	}

	tdd := filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd")
	write(t, filepath.Join(tdd, "SKILL.md"), skillMD("tdd")+"Write the test first.\n")
	write(t, filepath.Join(out.Path, "SKILL.md"), skillMD("notes")+"Take notes.\n")
	before := gitOut(t, bare, "rev-list", "--count", "HEAD")
	s, _ = app.Open(root)
	o, err := s.ContributePackOptions(context.Background(), "acme/skills")
	if err != nil || !o.Own || o.Pull || o.Behind || !slices.Equal(o.Skills, []string{"notes", "tdd"}) || !slices.Equal(o.Drafts, []string{"notes"}) || o.Paths["notes"] != "skills/eng/notes" {
		t.Fatalf("options %+v %v", o, err)
	}
	if !strings.HasPrefix(o.DefaultMessage, "Add notes; update tdd") {
		t.Errorf("message %q", o.DefaultMessage)
	}
	res, err := s.ContributePack(context.Background(), "acme/skills", app.ModePush, "")
	if err != nil || !res.Pushed || res.PulledBack == nil || res.Pulled != nil {
		t.Fatalf("contribute %+v %v", res, err)
	}
	if after := gitOut(t, bare, "rev-list", "--count", "HEAD"); after == before {
		t.Errorf("nothing pushed: %s commits", after)
	}
	if files := gitOut(t, bare, "show", "--name-only", "--format=", "HEAD"); files != "skills/eng/notes/SKILL.md\nskills/eng/tdd/SKILL.md" {
		t.Errorf("one commit with both, got:\n%s", files)
	}
	for rel, want := range map[string]string{"skills/eng/notes/SKILL.md": "Take notes.", "skills/eng/tdd/SKILL.md": "Write the test first."} {
		if got := upstreamFile(t, bare, "HEAD", rel); !strings.Contains(got, want) {
			t.Errorf("%s:\n%s", rel, got)
		}
	}
	if !slices.Contains(res.PulledBack.Landed, "notes") || !slices.Contains(res.PulledBack.Landed, "tdd") {
		t.Errorf("pulled back %+v", res.PulledBack)
	}
	if ls := lockSkill(t, root, "notes"); ls.Draft || ls.Hash == "" || ls.Path != "" {
		t.Errorf("notes still a draft: %+v", ls)
	}
	s, _ = app.Open(root)
	for _, n := range []string{"notes", "tdd"} {
		if e := entry(t, s, n); e.Edited || e.Draft {
			t.Errorf("%s still changed: %+v", n, e)
		}
	}
	if _, err := s.ContributePackOptions(context.Background(), "acme/skills"); err == nil {
		t.Error("nothing left to send, yet options")
	}
	if rep := update(t, root); len(rep.Changed)+len(rep.Landed)+len(rep.Merged) != 0 {
		t.Errorf("a later update reports %+v", rep)
	}
}

// The own repository moved on: contribute pulls first, merging, then
// pushes both; a conflict stops it before anything is sent.
func TestOwnPullsFirst(t *testing.T) {
	bare, root := ownFixture(t)
	tdd := filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd")
	write(t, filepath.Join(tdd, "SKILL.md"), skillMD("tdd")+"Write the test first.\n")
	upstreamWrite(t, bare, "skills/eng/tdd/notes.md", "from elsewhere\n")
	s, _ := app.Open(root)
	o, err := s.ContributePackOptions(context.Background(), "acme/skills")
	if err != nil || !o.Behind || !o.Pull || strings.Contains(o.Reason, "skd update") {
		t.Fatalf("options %+v %v", o, err)
	}
	res, err := s.ContributePack(context.Background(), "acme/skills", app.ModePush, "")
	if err != nil || res.Pulled == nil || !slices.Contains(res.Pulled.Merged, "tdd") || !res.Pushed {
		t.Fatalf("contribute %+v %v", res, err)
	}
	if got := upstreamFile(t, bare, "HEAD", "skills/eng/tdd/SKILL.md"); !strings.Contains(got, "Write the test first.") {
		t.Errorf("upstream:\n%s", got)
	}
	if !fsutil.Exists(filepath.Join(tdd, "notes.md")) {
		t.Error("the pull did not bring notes.md")
	}

	// Both sides change the same line: shelved, nothing sent.
	write(t, filepath.Join(tdd, "SKILL.md"), skillMD("tdd")+"Mine.\n")
	upstreamWrite(t, bare, "skills/eng/tdd/SKILL.md", skillMD("tdd")+"Theirs.\n")
	head := gitOut(t, bare, "rev-parse", "HEAD")
	s, _ = app.Open(root)
	res, err = s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, "")
	if err == nil || !strings.Contains(err.Error(), "skd resolve") || res == nil || res.Pulled == nil || res.Pushed {
		t.Fatalf("conflict: %+v %v", res, err)
	}
	if gitOut(t, bare, "rev-parse", "HEAD") != head {
		t.Error("pushed despite the conflict")
	}
}

// A pack that is not the user's: no skills made in it, no pull before a
// push (update first, as before).
func TestNotOwn(t *testing.T) {
	_, bare, root, _, _ := contribFixture(t)
	s, _ := app.Open(root)
	if _, err := s.NewInPack("notes", "acme/skills", app.NewInPackOptions{}); err == nil || !strings.Contains(err.Error(), "skd own") {
		t.Fatalf("new in a pack not marked own: %v", err)
	}
	upstreamWrite(t, bare, "skills/eng/tdd/notes.md", "new upstream\n")
	s, _ = app.Open(root)
	o, err := s.ContributePackOptions(context.Background(), "acme/skills")
	if err != nil || !o.Behind || o.Pull || o.Own || !strings.Contains(o.Reason, "skd update") {
		t.Fatalf("options %+v %v", o, err)
	}
	if _, err := s.ContributePack(context.Background(), "acme/skills", app.ModePush, ""); err == nil {
		t.Fatal("pushed while behind")
	}
	if p, err := s.SetOwn("skills", true); err != nil || !p.Own {
		t.Fatalf("own %+v %v", p, err)
	}
	m, _ := manifest.Load(filepath.Join(root, manifest.FileName))
	if !m.FindPack("acme/skills").Own {
		t.Error("own not saved")
	}
	if _, err := s.SetOwn("acme/skills", false); err != nil {
		t.Fatal(err)
	}
	if m, _ := manifest.Load(filepath.Join(root, manifest.FileName)); m.FindPack("acme/skills").Own {
		t.Error("own not taken away")
	}
}

// --path: the top, a directory of the user's choosing; refused where the
// scan never looks, and next to a name the library has.
func TestNewInPackPlaces(t *testing.T) {
	_, root := ownFixture(t)
	s, _ := app.Open(root)
	for _, bad := range []string{".claude/skills", "tests", "../out", "/abs"} {
		if _, err := s.NewInPack("x", "acme/skills", app.NewInPackOptions{Dir: bad}); err == nil {
			t.Errorf("--path %s accepted", bad)
		}
	}
	if _, err := s.NewInPack("tdd", "acme/skills", app.NewInPackOptions{}); err == nil {
		t.Error("a name the library has")
	}
	out, err := s.NewInPack("top", "acme/skills", app.NewInPackOptions{Dir: "."})
	if err != nil || out.RepoPath != "top" || out.Path != filepath.Join(root, "vendor", "acme", "skills", "top") {
		t.Fatalf("top %+v %v", out, err)
	}
	s, _ = app.Open(root)
	out, err = s.NewInPack("deep", "acme/skills", app.NewInPackOptions{Dir: "skills/ops/infra"})
	if err != nil || out.RepoPath != "skills/ops/infra/deep" || lockSkill(t, root, "deep").Category != "ops/infra" {
		t.Fatalf("deep %+v %v", out, err)
	}
	// Nothing left behind by a refused one.
	if fsutil.Exists(filepath.Join(root, "vendor", "acme", "skills", "x")) {
		t.Error("left a directory")
	}
}

// The user's own new repository may have no skill yet: added with --own,
// a first skill made and pushed. No license is asked for.
func TestOwnEmptyRepository(t *testing.T) {
	up := upstream(t)
	repo := filepath.Join(up, "me", "mine.git")
	write(t, filepath.Join(repo, "README.md"), "mine\n")
	git(t, repo, "init", "-q")
	commit(t, repo, "init")
	bare := bareUpstream(t, repo)
	_, root := side(t, up)
	t.Cleanup(app.SetGitHub(&fakeGH{status: app.GHOK, canPush: true}))
	s, _ := app.Open(root)
	src, err := s.ParseAdd("me/mine", "")
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Fetch(src)
	if err != nil || len(f.Skills) != 0 {
		t.Fatalf("fetch %+v %v", f, err)
	}
	if _, err := s.Add(f, app.Selection{All: true}); err == nil {
		t.Error("an empty repository that is not the user's was added")
	}
	s, _ = app.Open(root)
	f, _ = s.Fetch(src)
	added, err := s.Add(f, app.Selection{Own: true})
	if err != nil || !added.Report.Own {
		t.Fatalf("add %+v %v", added, err)
	}
	if p := s.Lib.Manifest.FindPack("me/mine"); p == nil || !p.Own || !p.UsesAll() {
		t.Fatalf("pack %+v", p)
	}
	out, err := s.NewInPack("first", "me/mine", app.NewInPackOptions{Description: "The first one."})
	if err != nil || out.RepoPath != "skills/first" {
		t.Fatalf("new %+v %v", out, err)
	}
	s, _ = app.Open(root)
	if _, err := s.Contribute(context.Background(), app.KeyRef("first"), app.ModePush, ""); err != nil {
		t.Fatal(err)
	}
	if got := upstreamFile(t, bare, "HEAD", "skills/first/SKILL.md"); !strings.Contains(got, "The first one.") {
		t.Errorf("upstream:\n%s", got)
	}
	if msg := gitOut(t, bare, "log", "-1", "--format=%s"); msg != "Add first" {
		t.Errorf("message %q", msg)
	}
}

// Pushed from elsewhere before this machine pushed it: a draft identical
// upstream lands, a different one is merged against an empty base and
// shelved where both have the same file.
func TestDraftMetUpstream(t *testing.T) {
	bare, root := ownFixture(t)
	s, _ := app.Open(root)
	a, _ := s.NewInPack("same", "acme/skills", app.NewInPackOptions{})
	s, _ = app.Open(root)
	b, _ := s.NewInPack("other", "acme/skills", app.NewInPackOptions{})
	same, _ := os.ReadFile(filepath.Join(a.Path, "SKILL.md"))
	upstreamWrite(t, bare, "skills/eng/same/SKILL.md", string(same))
	upstreamWrite(t, bare, "skills/eng/other/SKILL.md", skillMD("other")+"theirs\n")
	rep := update(t, root)
	if !slices.Equal(rep.Landed, []string{"same"}) || len(rep.Shelved) != 1 || rep.Shelved[0].Name != "other" {
		t.Fatalf("update %+v", rep)
	}
	if ls := lockSkill(t, root, "same"); ls.IsDraft() {
		t.Errorf("same still a draft %+v", ls)
	}
	if ls := lockSkill(t, root, "other"); ls.Shelved == "" {
		t.Errorf("other %+v", ls)
	}
	_ = b
}

// An older skd drops draft and path from skills.lock when it saves it: a
// skill with no hash is a draft all the same, kept by updates, and its
// place in the repository worked out again when it is pushed.
func TestDraftWithoutItsFields(t *testing.T) {
	bare, root := ownFixture(t)
	s, _ := app.Open(root)
	if _, err := s.NewInPack("notes", "acme/skills", app.NewInPackOptions{Dir: "skills/prod"}); err != nil {
		t.Fatal(err)
	}
	lp := filepath.Join(root, "skills.lock")
	b, _ := os.ReadFile(lp)
	var kept []string
	for _, l := range strings.Split(string(b), "\n") {
		if t := strings.TrimSpace(l); !strings.HasPrefix(t, "draft") && !strings.HasPrefix(t, "path") {
			kept = append(kept, l)
		}
	}
	os.WriteFile(lp, []byte(strings.Join(kept, "\n")), 0o644)
	if ls := lockSkill(t, root, "notes"); ls.Draft || ls.Path != "" || !ls.IsDraft() {
		t.Fatalf("lock %+v", ls)
	}
	if rep := update(t, root); len(rep.UpstreamGone)+len(rep.Dropped) != 0 {
		t.Fatalf("update %+v", rep)
	}
	s, _ = app.Open(root)
	if _, err := s.Contribute(context.Background(), app.KeyRef("notes"), app.ModePush, ""); err != nil {
		t.Fatal(err)
	}
	// Beside most of the skills, now that its own place was forgotten.
	if got := upstreamFile(t, bare, "HEAD", "skills/eng/notes/SKILL.md"); !strings.Contains(got, "name: notes") {
		t.Errorf("upstream:\n%s", got)
	}
}
