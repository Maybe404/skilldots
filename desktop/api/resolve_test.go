package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type bridgeSkill struct {
	Ref           ref
	Name          string
	Shelved       bool
	Resolving     bool
	ConflictFiles []string
}

func findBridgeSkill(t *testing.T, s *Service, name string) bridgeSkill {
	t.Helper()
	for _, sk := range roundTrip[[]bridgeSkill](t, invoke(t, s, "skills")) {
		if sk.Name == name {
			return sk
		}
	}
	t.Fatalf("no skill %s", name)
	return bridgeSkill{}
}

type conflictFile struct {
	Path   string
	Binary bool
	Text   string
	Blocks []struct {
		Index           int
		Local, Upstream string
	}
}

// The whole resolve flow through the desktop's calls: an update shelves
// tdd, the skill says so, resolveStart writes the markers, resolvePick
// settles them, resolveContinue finishes; history and its diff carry the
// merge.
func TestResolveThroughTheBridge(t *testing.T) {
	s := setup(t)
	repo := fakeGitHub(t)
	parsed := invoke(t, s, "parseSource", "npx skills add acme/skills --skill tdd")
	p := roundTrip[previewOut](t, invoke(t, s, "show", parsed))
	invoke(t, s, "add", object{"Source": p.Source, "Commit": p.Commit}, object{"skills": []string{"tdd"}, "agents": []string{"agents"}})
	lib := roundTrip[struct{ Root string }](t, invoke(t, s, "status")).Root
	tdd := filepath.Join(lib, "vendor", "acme", "skills", "eng", "tdd")

	sk := findBridgeSkill(t, s, "tdd")
	if sk.Shelved || sk.Resolving || sk.ConflictFiles == nil {
		t.Fatalf("before: %+v", sk)
	}
	// Saving in the editor stages the file.
	out := roundTrip[struct{ Staged bool }](t, invoke(t, s, "writeFile", sk.Ref, "SKILL.md", "---\nname: tdd\ndescription: tdd skill\n---\nmine\n"))
	if !out.Staged {
		t.Fatal("not staged")
	}
	st, _ := exec.Command("git", "-C", lib, "status", "--porcelain", "--", "vendor/acme/skills/eng/tdd/SKILL.md").Output()
	if !strings.HasPrefix(string(st), "A ") && !strings.HasPrefix(string(st), "M ") {
		t.Fatalf("status after save: %q", st)
	}

	writeFile(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), "---\nname: tdd\ndescription: tdd skill\n---\nupstream\n")
	runGit(t, repo, "commit", "-q", "-am", "upstream")
	rows := roundTrip[[]struct {
		When    time.Time
		Repo    string
		Merged  []string
		Shelved []struct {
			Name  string
			Files []string
		}
	}](t, invoke(t, s, "update", []string{"acme/skills"}))
	if len(rows) != 1 || len(rows[0].Shelved) != 1 || rows[0].Shelved[0].Name != "tdd" || !slices.Equal(rows[0].Shelved[0].Files, []string{"SKILL.md"}) || rows[0].Merged == nil {
		t.Fatalf("update: %+v", rows)
	}
	sk = findBridgeSkill(t, s, "tdd")
	if !sk.Shelved || sk.Resolving || !slices.Equal(sk.ConflictFiles, []string{"SKILL.md"}) {
		t.Fatalf("shelved: %+v", sk)
	}

	state := roundTrip[struct{ Files []conflictFile }](t, invoke(t, s, "resolveStart", sk.Ref))
	if len(state.Files) != 1 || len(state.Files[0].Blocks) != 1 || state.Files[0].Blocks[0].Local != "mine\n" || state.Files[0].Blocks[0].Upstream != "upstream\n" {
		t.Fatalf("start: %+v", state)
	}
	if sk = findBridgeSkill(t, s, "tdd"); !sk.Resolving {
		t.Fatalf("resolving: %+v", sk)
	}
	if _, err := s.Invoke(context.Background(), "resolveContinue", mustArgs(t, sk.Ref)); err == nil {
		t.Fatal("continued with markers")
	}
	invoke(t, s, "resolvePick", sk.Ref, "SKILL.md", 0, "upstream")
	state = roundTrip[struct{ Files []conflictFile }](t, invoke(t, s, "resolveState", sk.Ref))
	if len(state.Files) != 0 {
		t.Fatalf("left: %+v", state)
	}
	if b, _ := os.ReadFile(filepath.Join(tdd, "SKILL.md")); !strings.HasSuffix(string(b), "---\nupstream\n") {
		t.Fatalf("picked: %q", b)
	}
	cont := roundTrip[struct{ Absent map[string][]string }](t, invoke(t, s, "resolveContinue", sk.Ref))
	if cont.Absent == nil {
		t.Fatal("no absent map")
	}
	if sk = findBridgeSkill(t, s, "tdd"); sk.Shelved || sk.Resolving {
		t.Fatalf("after: %+v", sk)
	}

	hist := roundTrip[[]struct {
		When            time.Time
		Repo, From, To  string
		Merged, Changed []string
	}](t, invoke(t, s, "history"))
	if len(hist) != 2 || !slices.Equal(hist[0].Merged, []string{"tdd"}) || len(hist[0].Changed) != 0 || hist[0].From == hist[0].To {
		t.Fatalf("history: %+v", hist)
	}
	// The diff of the update that shelved it: upstream's change.
	d := roundTrip[struct {
		Skills []struct {
			Name  string
			Files []struct{ Path string }
		}
	}](t, invoke(t, s, "historyDiff", rows[0].Repo, rows[0].When))
	if len(d.Skills) != 1 || d.Skills[0].Name != "tdd" || len(d.Skills[0].Files) != 1 {
		t.Fatalf("historyDiff: %+v", d)
	}
}

// Abort through the bridge puts the skill back; a whole-file conflict is
// picked with block -1.
func TestResolveAbortAndWholeFileThroughTheBridge(t *testing.T) {
	s := setup(t)
	repo := fakeGitHub(t)
	writeFile(t, filepath.Join(repo, "skills", "eng", "tdd", "logo.bin"), "a\x00")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "logo")
	parsed := invoke(t, s, "parseSource", "npx skills add acme/skills --skill tdd")
	p := roundTrip[previewOut](t, invoke(t, s, "show", parsed))
	invoke(t, s, "add", object{"Source": p.Source, "Commit": p.Commit}, object{"skills": []string{"tdd"}, "agents": []string{"agents"}})
	lib := roundTrip[struct{ Root string }](t, invoke(t, s, "status")).Root
	tdd := filepath.Join(lib, "vendor", "acme", "skills", "eng", "tdd")
	writeFile(t, filepath.Join(tdd, "logo.bin"), "mine\x00")
	writeFile(t, filepath.Join(repo, "skills", "eng", "tdd", "logo.bin"), "theirs\x00")
	runGit(t, repo, "commit", "-q", "-am", "logo 2")
	invoke(t, s, "update", []string{"acme/skills"})
	sk := findBridgeSkill(t, s, "tdd")

	state := roundTrip[struct{ Files []conflictFile }](t, invoke(t, s, "resolveStart", sk.Ref))
	if len(state.Files) != 1 || !state.Files[0].Binary {
		t.Fatalf("start: %+v", state)
	}
	invoke(t, s, "resolveAbort", sk.Ref)
	if b, _ := os.ReadFile(filepath.Join(tdd, "logo.bin")); string(b) != "mine\x00" {
		t.Fatalf("abort: %q", b)
	}
	if _, err := os.Stat(filepath.Join(tdd, "logo.bin.upstream")); err == nil {
		t.Fatal("the .upstream file stayed")
	}
	if sk = findBridgeSkill(t, s, "tdd"); !sk.Shelved || sk.Resolving {
		t.Fatalf("after abort: %+v", sk)
	}

	invoke(t, s, "resolveStart", sk.Ref)
	invoke(t, s, "resolvePick", sk.Ref, "logo.bin", -1, "upstream")
	if b, _ := os.ReadFile(filepath.Join(tdd, "logo.bin")); string(b) != "theirs\x00" {
		t.Fatalf("picked: %q", b)
	}
	if _, err := os.Stat(filepath.Join(tdd, "logo.bin.upstream")); err == nil {
		t.Fatal("the .upstream file stayed")
	}
	invoke(t, s, "resolveContinue", sk.Ref)
	trash := roundTrip[[]struct{ Reason string }](t, invoke(t, s, "trash"))
	merged := 0
	for _, e := range trash {
		if e.Reason == "merged" {
			merged++
		}
	}
	if merged == 0 {
		t.Fatalf("trash: %+v", trash)
	}
}
