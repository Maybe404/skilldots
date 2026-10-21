package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
)

// fakeGH stands for GitHub: nothing a test runs reaches it.
type fakeGH struct {
	status    string
	canPush   bool
	forkOwner string
	onCanPush func()
	prs       []fakePR
	states    map[string]string
}

type fakePR struct{ owner, repo, base, head, title, body string }

func (f *fakeGH) Status(context.Context) string { return f.status }
func (f *fakeGH) CanPush(context.Context, string, string) (bool, error) {
	if f.onCanPush != nil {
		f.onCanPush()
	}
	return f.canPush, nil
}
func (f *fakeGH) Fork(_ context.Context, owner, repo string) (string, string, error) {
	return f.forkOwner, repo, nil
}
func (f *fakeGH) CreatePR(_ context.Context, owner, repo, base, head, title, body string) (string, int, error) {
	f.prs = append(f.prs, fakePR{owner, repo, base, head, title, body})
	return "https://github.com/" + owner + "/" + repo + "/pull/7", 7, nil
}
func (f *fakeGH) PRState(_ context.Context, url string) (string, error) { return f.states[url], nil }

// bareUpstream turns the fake GitHub's acme/skills into a bare
// repository (so it takes pushes) and returns it.
func bareUpstream(t *testing.T, repo string) string {
	t.Helper()
	tmp := repo + ".tmp"
	git(t, filepath.Dir(repo), "clone", "-q", "--bare", repo, tmp)
	os.RemoveAll(repo)
	if err := os.Rename(tmp, repo); err != nil {
		t.Fatal(err)
	}
	return repo
}

// upstreamWrite commits a file into a bare upstream through a clone.
func upstreamWrite(t *testing.T, bare, rel, body string) {
	t.Helper()
	w := t.TempDir()
	git(t, w, "clone", "-q", bare, "w")
	write(t, filepath.Join(w, "w", filepath.FromSlash(rel)), body)
	commit(t, filepath.Join(w, "w"), "upstream: "+rel)
	git(t, filepath.Join(w, "w"), "push", "-q", "origin", "HEAD")
}

func upstreamFile(t *testing.T, bare, ref, rel string) string {
	t.Helper()
	return gitOut(t, bare, "show", ref+":"+rel)
}

// contribFixture: acme/skills is a bare repository on the fake GitHub, the
// library follows tdd and review, and tdd has an edit of the user's.
func contribFixture(t *testing.T) (up, bare, root, tdd string, f *fakeGH) {
	t.Helper()
	up = upstream(t)
	bare = bareUpstream(t, filepath.Join(up, "acme", "skills.git"))
	_, root = side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd,review")
	tdd = filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd")
	write(t, filepath.Join(tdd, "SKILL.md"), skillMD("tdd")+"Write the test first.\n")
	f = &fakeGH{status: app.GHOK, canPush: true, forkOwner: "me", states: map[string]string{}}
	t.Cleanup(app.SetGitHub(f))
	return up, bare, root, tdd, f
}

func TestLocalChanges(t *testing.T) {
	_, _, root, _, _ := contribFixture(t)
	s, _ := app.Open(root)
	d, err := s.LocalChanges(context.Background(), app.KeyRef("tdd"))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Local || !d.Base || len(d.Files) != 1 || d.Files[0].Path != "SKILL.md" || d.Files[0].Added != 1 || d.Files[0].Removed != 0 {
		t.Fatalf("%+v", d)
	}
	entries, _ := s.Resolve()
	s.MarkEdited(entries)
	for _, e := range entries {
		if want := e.Name == "tdd"; e.Edited != want {
			t.Errorf("%s edited %v", e.Name, e.Edited)
		}
	}
	// Without the version it is based on, nothing is guessed.
	git(t, root, "update-ref", "-d", "refs/skilldots/upstream/acme/skills")
	d, err = s.LocalChanges(context.Background(), app.KeyRef("tdd"))
	if err != nil || d.Base || !d.LocalUnknown || len(d.Files) != 0 || !d.Local {
		t.Fatalf("without base: %+v %v", d, err)
	}
	// An unchanged skill has no changes.
	if d, _ := s.LocalChanges(context.Background(), app.KeyRef("review")); d.Local || len(d.Files) != 0 {
		t.Errorf("review: %+v", d)
	}
}

// Pushed, recorded; once an update brings upstream to the user's version
// the record and the edited mark go.
func TestContributePush(t *testing.T) {
	_, bare, root, tdd, _ := contribFixture(t)
	s, _ := app.Open(root)
	o, err := s.ContributeOptions(context.Background(), app.KeyRef("tdd"))
	if err != nil || o.Repo != "acme/skills" || o.Host != app.HostGitHub || o.Branch != mainOf(t, bare) || !o.CanPush || !o.CanPR || o.Behind || o.GH != app.GHOK {
		t.Fatalf("options %+v %v", o, err)
	}
	out, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, "")
	if err != nil || !out.Pushed || out.Branch != mainOf(t, bare) {
		t.Fatalf("push %+v %v", out, err)
	}
	if got := upstreamFile(t, bare, mainOf(t, bare), "skills/eng/tdd/SKILL.md"); !strings.Contains(got, "Write the test first.") {
		t.Fatalf("upstream:\n%s", got)
	}
	if msg := gitOut(t, bare, "log", "-1", "--format=%B", mainOf(t, bare)); !strings.Contains(msg, "Update tdd") || !strings.Contains(msg, "Edited with skilldots") {
		t.Errorf("message %q", msg)
	}
	if ls := lockSkill(t, root, "tdd"); ls.Contribution == nil || ls.Contribution.Kind != "push" || ls.Contribution.State != "pushed" {
		t.Fatalf("lock %+v", ls)
	}
	if cs := s.Contributions(); len(cs) != 1 || cs[0].Name != "tdd" {
		t.Errorf("contributions %+v", cs)
	}
	update(t, root)
	if ls := lockSkill(t, root, "tdd"); ls.Contribution != nil {
		t.Errorf("record kept after the change landed: %+v", ls.Contribution)
	}
	s, _ = app.Open(root)
	entries, _ := s.Resolve()
	s.MarkEdited(entries)
	for _, e := range entries {
		if e.Edited {
			t.Errorf("%s still edited", e.Name)
		}
	}
	_ = tdd
}

// A branch the user may not push to (a protected branch): refused, with
// the pull request suggested; nothing recorded.
func TestContributePushRejected(t *testing.T) {
	_, bare, root, _, _ := contribFixture(t)
	write(t, filepath.Join(bare, "hooks", "pre-receive"), "#!/bin/sh\necho protected branch >&2\nexit 1\n")
	os.Chmod(filepath.Join(bare, "hooks", "pre-receive"), 0o755)
	s, _ := app.Open(root)
	_, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, "")
	if err == nil || !strings.Contains(err.Error(), "--pr") {
		t.Fatalf("push to a protected branch: %v", err)
	}
	if ls := lockSkill(t, root, "tdd"); ls.Contribution != nil {
		t.Errorf("recorded %+v", ls.Contribution)
	}
}

// Upstream moved on while the change was being made: not a fast-forward.
func TestContributePushNonFastForward(t *testing.T) {
	_, bare, root, _, f := contribFixture(t)
	f.onCanPush = func() { upstreamWrite(t, bare, "README.md", "moved on\n") }
	s, _ := app.Open(root)
	_, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, "")
	if err == nil || !strings.Contains(err.Error(), "--pr") {
		t.Fatalf("non-fast-forward: %v", err)
	}
}

// With push rights the pull request's branch goes to upstream itself.
func TestContributePRWithPushRights(t *testing.T) {
	_, bare, root, _, f := contribFixture(t)
	s, _ := app.Open(root)
	out, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePR, "Test first\n\nBecause.")
	if err != nil || out.Number != 7 || !strings.HasPrefix(out.Branch, "skilldots/tdd-") {
		t.Fatalf("pr %+v %v", out, err)
	}
	if len(f.prs) != 1 || f.prs[0].head != out.Branch || f.prs[0].base != mainOf(t, bare) || f.prs[0].title != "Test first" || !strings.HasPrefix(f.prs[0].body, "Because.") {
		t.Fatalf("prs %+v", f.prs)
	}
	if got := upstreamFile(t, bare, out.Branch, "skills/eng/tdd/SKILL.md"); !strings.Contains(got, "Write the test first.") {
		t.Errorf("branch:\n%s", got)
	}
	if got := upstreamFile(t, bare, mainOf(t, bare), "skills/eng/tdd/SKILL.md"); strings.Contains(got, "Write the test first.") {
		t.Error("main was changed")
	}
	if ls := lockSkill(t, root, "tdd"); ls.Contribution == nil || ls.Contribution.State != "open" || ls.Contribution.Number != 7 {
		t.Fatalf("lock %+v", ls.Contribution)
	}
	// Refreshed from GitHub.
	f.states[out.URL] = "merged"
	s, _ = app.Open(root)
	if err := s.RefreshContributions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ls := lockSkill(t, root, "tdd"); ls.Contribution.State != "merged" {
		t.Errorf("refreshed %+v", ls.Contribution)
	}
}

// Without push rights: forked first, the branch pushed to the fork.
func TestContributePRFromFork(t *testing.T) {
	up, bare, root, _, f := contribFixture(t)
	f.canPush = false
	fork := filepath.Join(up, "me", "skills.git")
	os.MkdirAll(filepath.Dir(fork), 0o755)
	git(t, filepath.Dir(fork), "clone", "-q", "--bare", bare, fork)
	s, _ := app.Open(root)
	o, _ := s.ContributeOptions(context.Background(), app.KeyRef("tdd"))
	if o.CanPush || !o.CanPR || !strings.Contains(o.Reason, "cannot push") {
		t.Fatalf("options %+v", o)
	}
	if _, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, ""); err == nil {
		t.Fatal("pushed without rights")
	}
	out, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePR, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.prs) != 1 || f.prs[0].head != "me:"+out.Branch {
		t.Fatalf("prs %+v", f.prs)
	}
	if got := upstreamFile(t, fork, out.Branch, "skills/eng/tdd/SKILL.md"); !strings.Contains(got, "Write the test first.") {
		t.Errorf("fork:\n%s", got)
	}
	if b := gitOut(t, bare, "branch", "--list", "skilldots/*"); b != "" {
		t.Errorf("pushed to upstream: %s", b)
	}
}

func TestContributeNeedsGH(t *testing.T) {
	_, _, root, _, f := contribFixture(t)
	for status, want := range map[string]string{app.GHMissing: "not installed", app.GHLoggedOut: "gh auth login"} {
		f.status = status
		s, _ := app.Open(root)
		o, err := s.ContributeOptions(context.Background(), app.KeyRef("tdd"))
		if err != nil || o.CanPR || o.GH != status || !strings.Contains(o.Reason, want) {
			t.Errorf("%s: %+v %v", status, o, err)
		}
		if _, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePR, ""); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: pr: %v", status, err)
		}
	}
}

// Not based on upstream's current version: update first.
func TestContributeBehind(t *testing.T) {
	_, bare, root, _, _ := contribFixture(t)
	upstreamWrite(t, bare, "skills/eng/tdd/notes.md", "new upstream\n")
	s, _ := app.Open(root)
	o, err := s.ContributeOptions(context.Background(), app.KeyRef("tdd"))
	if err != nil || !o.Behind || !strings.Contains(o.Reason, "skd update") {
		t.Fatalf("options %+v %v", o, err)
	}
	if _, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, ""); err == nil || !strings.Contains(err.Error(), "skd update") {
		t.Fatalf("contribute: %v", err)
	}
	// An unedited skill has nothing to send.
	if _, err := s.ContributeOptions(context.Background(), app.KeyRef("review")); err == nil {
		t.Error("review has no changes")
	}
}

// A repository not on GitHub: no pull requests; git says whether a push
// goes through.
func TestContributeOtherHost(t *testing.T) {
	up := upstream(t)
	bare := bareUpstream(t, filepath.Join(up, "acme", "skills.git"))
	_, root := side(t, up)
	t.Cleanup(app.SetGitHub(&fakeGH{status: app.GHOK, canPush: true}))
	runCLI(t, root, "add", "file://"+bare, "--skill", "tdd")
	s, _ := app.Open(root)
	entries, _ := s.Resolve()
	var tdd engine.Entry
	for _, e := range entries {
		if e.Name == "tdd" {
			tdd = e
		}
	}
	write(t, filepath.Join(tdd.Dir, "SKILL.md"), skillMD("tdd")+"mine\n")
	o, err := s.ContributeOptions(context.Background(), app.KeyRef("tdd"))
	if err != nil || o.Host != app.HostOther || o.CanPR || !o.CanPush || !strings.Contains(o.Reason, "GitHub only") {
		t.Fatalf("options %+v %v", o, err)
	}
	if _, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePR, ""); err == nil {
		t.Fatal("a pull request elsewhere than GitHub")
	}
	if _, err := s.Contribute(context.Background(), app.KeyRef("tdd"), app.ModePush, ""); err != nil {
		t.Fatal(err)
	}
	if got := upstreamFile(t, bare, mainOf(t, bare), "skills/eng/tdd/SKILL.md"); !strings.Contains(got, "mine") {
		t.Errorf("upstream:\n%s", got)
	}
}

// mainOf is the branch HEAD names in a bare repository.
func mainOf(t *testing.T, bare string) string {
	t.Helper()
	return gitOut(t, bare, "symbolic-ref", "--short", "HEAD")
}
