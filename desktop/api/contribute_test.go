package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// fakeGH stands for GitHub in these tests: nothing reaches it.
type fakeGH struct{}

func (fakeGH) Status(context.Context) string                         { return app.GHOK }
func (fakeGH) CanPush(context.Context, string, string) (bool, error) { return true, nil }
func (fakeGH) Fork(_ context.Context, o, r string) (string, string, error) {
	return "me", r, nil
}
func (fakeGH) CreatePR(_ context.Context, o, r, base, head, title, body string) (string, int, error) {
	return "https://github.com/" + o + "/" + r + "/pull/3", 3, nil
}
func (fakeGH) PRState(context.Context, string) (string, error) { return "open", nil }

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// The edited mark, the user's edits and a push upstream through the
// desktop's calls.
func TestContributeThroughTheBridge(t *testing.T) {
	s := setup(t)
	repo := fakeGitHub(t)
	// A bare upstream takes pushes.
	runGit(t, filepath.Dir(repo), "clone", "-q", "--bare", repo, repo+".bare")
	os.RemoveAll(repo)
	os.Rename(repo+".bare", repo)
	t.Cleanup(app.SetGitHub(fakeGH{}))
	parsed := invoke(t, s, "parseSource", "npx skills add acme/skills --skill tdd")
	p := roundTrip[previewOut](t, invoke(t, s, "show", parsed))
	invoke(t, s, "add", object{"Source": p.Source, "Commit": p.Commit}, object{"skills": []string{"tdd"}, "agents": []string{"agents"}})

	type row struct {
		Ref          ref
		Name         string
		Edited       bool
		Contribution *struct{ Kind, URL, Branch, State string }
	}
	find := func() row {
		for _, r := range roundTrip[[]row](t, invoke(t, s, "skills")) {
			if r.Name == "tdd" {
				return r
			}
		}
		t.Fatal("no tdd")
		return row{}
	}
	sk := find()
	if sk.Edited || sk.Contribution != nil {
		t.Fatalf("before: %+v", sk)
	}
	invoke(t, s, "writeFile", sk.Ref, "SKILL.md", "---\nname: tdd\ndescription: tdd skill\n---\nbody\nmine\n")
	if sk = find(); !sk.Edited {
		t.Fatal("not marked edited")
	}
	d := roundTrip[struct {
		Base  bool
		Files []struct{ Path string }
	}](t, invoke(t, s, "localChanges", sk.Ref))
	if !d.Base || len(d.Files) != 1 {
		t.Fatalf("localChanges: %+v", d)
	}
	o := roundTrip[map[string]any](t, invoke(t, s, "contributeOptions", sk.Ref))
	for _, k := range []string{"repo", "host", "canPush", "canPR", "gh", "behind", "reason", "defaultMessage", "branch"} {
		if _, ok := o[k]; !ok {
			t.Errorf("contributeOptions has no %s: %v", k, o)
		}
	}
	if o["repo"] != "acme/skills" || o["host"] != "github" || o["canPush"] != true || o["behind"] != false {
		t.Fatalf("contributeOptions: %v", o)
	}
	out := roundTrip[map[string]any](t, invoke(t, s, "contribute", sk.Ref, "push", "Mine"))
	if out["pushed"] != true || out["mode"] != "push" || out["branch"] != o["branch"] {
		t.Fatalf("contribute: %v", out)
	}
	if got := gitOutput(t, repo, "log", "-1", "--format=%s", o["branch"].(string)); got != "Mine" {
		t.Errorf("upstream: %q", got)
	}
	if sk = find(); sk.Contribution == nil || sk.Contribution.Kind != "push" || sk.Contribution.State != "pushed" {
		t.Fatalf("after: %+v", sk.Contribution)
	}
	if res, err := s.Invoke(context.Background(), "refreshContributions", nil); err != nil || res != nil {
		t.Fatalf("refresh: %v %v", res, err)
	}
}

// A sync that conflicts, resolved through the resolve calls with @sync.
func TestSyncConflictThroughTheBridge(t *testing.T) {
	s := setup(t)
	lib := roundTrip[struct{ Root string }](t, invoke(t, s, "status")).Root
	invoke(t, s, "newSkill", object{"Name": "mine", "Description": "mine"})
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, filepath.Dir(remote), "init", "-q", "--bare", remote)
	first := roundTrip[map[string]any](t, invoke(t, s, "sync", object{"Remote": remote}))
	if first["pushed"] != true || first["pulled"] != false {
		t.Fatalf("first sync: %v", first)
	}
	// Another machine changes the same file.
	other := filepath.Join(t.TempDir(), "other")
	runGit(t, filepath.Dir(other), "clone", "-q", remote, other)
	writeFile(t, filepath.Join(other, "skills", "mine", "SKILL.md"), "---\nname: mine\ndescription: from the other machine\n---\n")
	runGit(t, other, "commit", "-qam", "other")
	runGit(t, other, "push", "-q")
	writeFile(t, filepath.Join(lib, "skills", "mine", "SKILL.md"), "---\nname: mine\ndescription: from here\n---\n")

	res := roundTrip[map[string]any](t, invoke(t, s, "sync", object{}))
	if c, _ := res["conflicts"].([]any); len(c) != 1 || c[0] != "skills/mine/SKILL.md" || res["pushed"] != false {
		t.Fatalf("sync: %v", res)
	}
	// Shelved, and still known by the app opened again.
	for _, svc := range []*Service{s, New("", Native{})} {
		st := roundTrip[map[string]any](t, invoke(t, svc, "status"))
		if sh, _ := st["syncShelved"].([]any); len(sh) != 1 || sh[0] != "skills/mine/SKILL.md" || st["syncing"] != false {
			t.Fatalf("status: %v", st)
		}
	}
	syncRef := ref{Key: "@sync"}
	state := roundTrip[struct {
		Files []struct {
			Path   string
			Blocks []struct{ Local, Upstream string }
		}
	}](t, invoke(t, s, "resolveStart", syncRef))
	if len(state.Files) != 1 || state.Files[0].Path != "skills/mine/SKILL.md" || len(state.Files[0].Blocks) != 1 {
		t.Fatalf("resolveStart: %+v", state)
	}
	if st := roundTrip[map[string]any](t, invoke(t, s, "status")); st["syncing"] != true || len(st["syncShelved"].([]any)) != 0 {
		t.Fatalf("status: %v", st)
	}
	// The conflicted file, read and written by hand; nothing else.
	if text := invoke(t, s, "readFile", syncRef, "skills/mine/SKILL.md").(string); !strings.Contains(text, "<<<<<<<") {
		t.Fatalf("readFile: %q", text)
	}
	for _, p := range []string{"skills.toml", "../outside", "skills/mine/../../skills.toml", "/etc/passwd"} {
		if _, err := s.Invoke(context.Background(), "readFile", mustArgs(t, syncRef, p)); err == nil {
			t.Errorf("readFile %s", p)
		}
		if _, err := s.Invoke(context.Background(), "writeFile", mustArgs(t, syncRef, p, "x")); err == nil {
			t.Errorf("writeFile %s", p)
		}
	}
	w := roundTrip[map[string]any](t, invoke(t, s, "writeFile", syncRef, "skills/mine/SKILL.md", "---\nname: mine\ndescription: from here, by hand\n---\n"))
	if w["staged"] != true || strings.Contains(gitOutput(t, lib, "status", "--porcelain"), "UU ") {
		t.Fatalf("writeFile: %v\n%s", w, gitOutput(t, lib, "status", "--porcelain"))
	}
	if st := roundTrip[struct{ Files []any }](t, invoke(t, s, "resolveState", syncRef)); len(st.Files) != 0 {
		t.Fatalf("left: %+v", st)
	}
	cont := roundTrip[map[string]any](t, invoke(t, s, "resolveContinue", syncRef))
	if _, ok := cont["absent"]; !ok {
		t.Fatalf("continue: %v", cont)
	}
	if st := roundTrip[map[string]any](t, invoke(t, s, "status")); st["syncing"] != false {
		t.Fatalf("status after: %v", st)
	}
	if got := gitOutput(t, remote, "show", "HEAD:skills/mine/SKILL.md"); !strings.Contains(got, "from here, by hand") {
		t.Errorf("remote:\n%s", got)
	}
	if st := roundTrip[map[string]any](t, invoke(t, s, "status")); len(st["syncShelved"].([]any)) != 0 {
		t.Errorf("status after: %v", st)
	}
}

// openUrl opens https:// addresses only.
func TestOpenURL(t *testing.T) {
	var opened []string
	prev := launchURL
	launchURL = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { launchURL = prev })
	s := New("", Native{})
	if _, err := s.Invoke(context.Background(), "openUrl", mustArgs(t, "https://github.com/acme/skills/pull/3")); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"http://example.com", "file:///etc/passwd", "javascript:alert(1)", "-a Calculator", "https://", "https://x/\x1b[31m", "https://a b", "https://user@evil.example/", "ssh://github.com/x"} {
		if _, err := s.Invoke(context.Background(), "openUrl", mustArgs(t, u)); err == nil {
			t.Errorf("opened %q", u)
		}
	}
	if len(opened) != 1 {
		t.Errorf("opened %v", opened)
	}
}

// The user's own repository through the desktop's calls: added with
// own, marked in packs, a skill made in it (newSkill with a pack), sent
// with an edit by contributePack, and nothing left marked after.
func TestOwnRepositoryThroughTheBridge(t *testing.T) {
	s := setup(t)
	repo := fakeGitHub(t)
	runGit(t, filepath.Dir(repo), "clone", "-q", "--bare", repo, repo+".bare")
	os.RemoveAll(repo)
	os.Rename(repo+".bare", repo)
	t.Cleanup(app.SetGitHub(fakeGH{}))
	parsed := invoke(t, s, "parseSource", "acme/skills")
	p := roundTrip[previewOut](t, invoke(t, s, "show", parsed))
	invoke(t, s, "add", object{"Source": p.Source, "Commit": p.Commit}, object{"all": true, "own": true, "agents": []string{"agents"}})
	packs := roundTrip[[]struct {
		Repo string
		Own  bool
	}](t, invoke(t, s, "packs"))
	if len(packs) != 1 || !packs[0].Own {
		t.Fatalf("packs: %+v", packs)
	}

	type row struct {
		Ref                ref
		Name               string
		Edited, Own, Draft bool
		Origin             string
	}
	find := func(name string) row {
		for _, r := range roundTrip[[]row](t, invoke(t, s, "skills")) {
			if r.Name == name {
				return r
			}
		}
		t.Fatalf("no %s", name)
		return row{}
	}
	made := roundTrip[row](t, invoke(t, s, "newSkill", object{"Name": "notes", "Description": "Take notes.", "Pack": "acme/skills", "Agents": []string{"agents"}}))
	if made.Origin != "acme/skills" || !made.Draft || !made.Own {
		t.Fatalf("newSkill: %+v", made)
	}
	if _, err := s.Invoke(context.Background(), "newSkill", mustArgs(t, object{"Name": "x", "Pack": "acme/skills", "Private": true})); err == nil {
		t.Error("a private skill in the own repository")
	}
	tdd := find("tdd")
	invoke(t, s, "writeFile", tdd.Ref, "SKILL.md", "---\nname: tdd\ndescription: tdd skill\n---\nbody\nmine\n")
	o := roundTrip[map[string]any](t, invoke(t, s, "contributePackOptions", "acme/skills"))
	if o["own"] != true || o["pull"] != false || len(o["skills"].([]any)) != 2 || len(o["drafts"].([]any)) != 1 {
		t.Fatalf("contributePackOptions: %v", o)
	}
	out := roundTrip[map[string]any](t, invoke(t, s, "contributePack", "acme/skills", "push", ""))
	if out["pushed"] != true || out["pulledBack"] != true {
		t.Fatalf("contributePack: %v", out)
	}
	if got := gitOutput(t, repo, "show", "HEAD:skills/eng/notes/SKILL.md"); !strings.Contains(got, "Take notes.") {
		t.Errorf("upstream:\n%s", got)
	}
	for _, n := range []string{"notes", "tdd"} {
		if r := find(n); r.Edited || r.Draft {
			t.Errorf("%s still marked: %+v", n, r)
		}
	}
	invoke(t, s, "setOwn", "acme/skills", false)
	if packs := roundTrip[[]struct{ Own bool }](t, invoke(t, s, "packs")); packs[0].Own {
		t.Error("setOwn false")
	}
	if res, err := s.Invoke(context.Background(), "looksOwn", mustArgs(t, "https://github.com/acme/skills")); err != nil || res != false {
		t.Errorf("looksOwn without Owns: %v %v", res, err)
	}
}
