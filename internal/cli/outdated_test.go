package cli_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// digest hashes every file under the library except .git: what outdated
// must leave alone.
func digest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, p)
		fmt.Fprintf(h, "%s %v\n", rel, d.Type())
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			h.Write(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// statePath is where skd keeps the last upstream check for this env.
func (e *env) statePath() string {
	if runtime.GOOS == "darwin" {
		return e.p("Library/Application Support/skilldots/outdated.json")
	}
	return e.p(".config/skilldots/outdated.json")
}

type packStatus struct {
	Repo, Commit, Latest        string
	Changed, New, Gone, Pending []string
}

func statuses(t *testing.T, r map[string]any) map[string]packStatus {
	t.Helper()
	b, _ := json.Marshal(r["packs"])
	var ps []packStatus
	if err := json.Unmarshal(b, &ps); err != nil {
		t.Fatal(err)
	}
	out := map[string]packStatus{}
	for _, p := range ps {
		out[p.Repo] = p
	}
	return out
}

func TestOutdated(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	for _, s := range []string{"eng/tdd", "eng/review", "prod/grill", "prod/plan"} {
		e.skill("up/skills/"+s, filepath.Base(s))
	}
	up := e.repo("up")
	e.skill("up2/skills/one", "one")
	up2 := e.repo("up2")
	e.ok("", "add", up, "--skill", "tdd,grill")
	e.ok("", "add", up2, "--skill", "one")
	e.ok("", "update") // records upstream's skill list: the baseline for "new"

	r := e.js("", "outdated")
	if r["updates"] != float64(0) || len(statuses(t, r)) != 2 {
		t.Fatalf("nothing changed upstream, outdated says %v", r)
	}

	// Upstream: tdd changed, fresh added, grill (selected) deleted; plan
	// and review were there before and stay unselected.
	e.write("up/skills/eng/tdd/SKILL.md", "---\nname: tdd\ndescription: d\n---\nnew body\n")
	e.skill("up/skills/eng/fresh", "fresh")
	os.RemoveAll(e.p("up/skills/prod/grill"))
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "c2")
	head := e.git(up, "rev-parse", "HEAD")

	before := digest(t, lib)
	r = e.js("", "outdated")
	if after := digest(t, lib); after != before {
		t.Fatal("outdated changed the library (vendor/, skills.lock or skills.toml)")
	}
	st := statuses(t, r)[up]
	if !slices.Equal(st.Changed, []string{"tdd"}) || !slices.Equal(st.New, []string{"fresh"}) ||
		!slices.Equal(st.Gone, []string{"grill"}) || len(st.Pending) != 0 || st.Latest != head || st.Commit == head {
		t.Fatalf("outdated for %s: %+v", up, st)
	}
	if s2 := statuses(t, r)[up2]; len(s2.Changed)+len(s2.New)+len(s2.Gone)+len(s2.Pending) != 0 {
		t.Fatalf("unchanged pack reported: %+v", s2)
	}
	if r["updates"] != float64(1) {
		t.Fatalf("updates = %v", r["updates"])
	}

	out := e.ok("", "outdated")
	for _, want := range []string{"tdd", "fresh", "grill", "upstream new", "upstream gone", "1 pack has updates · skd update"} {
		if !strings.Contains(out, want) {
			t.Errorf("outdated output lacks %q:\n%s", want, out)
		}
	}
	if lines := strings.Split(out, "\n"); !slices.ContainsFunc(lines, func(l string) bool {
		return strings.HasPrefix(l, up2+"  ") && strings.Contains(l, "up to date")
	}) {
		t.Errorf("unchanged pack is not one line:\n%s", out)
	}

	// The result is recorded; list and the menu read it without fetching.
	b, err := os.ReadFile(e.statePath())
	if err != nil || !strings.Contains(string(b), "fresh") || !strings.Contains(string(b), "checked") {
		t.Fatalf("state file: %v\n%s", err, b)
	}
	if out := e.ok("", "list"); !strings.Contains(out, "↑ 1 update available · checked just now") {
		t.Fatalf("list does not show the recorded check:\n%s", out)
	}

	// Older than a day: stale.
	var state map[string]any
	json.Unmarshal(b, &state)
	old := time.Now().Add(-25 * time.Hour).Format(time.RFC3339)
	for _, l := range state["libraries"].(map[string]any) {
		rec := l.(map[string]any)
		rec["checked"] = old
		for _, p := range rec["packs"].([]any) {
			p.(map[string]any)["checked"] = old
		}
	}
	b, _ = json.Marshal(state)
	os.WriteFile(e.statePath(), b, 0o644)
	if out := e.ok("", "list"); !strings.Contains(out, "checked 25h ago · stale") {
		t.Fatalf("list does not call a day-old check stale:\n%s", out)
	}

	// update brings the pack up to date, and the recorded count with it.
	e.ok("", "update")
	if out := e.ok("", "list"); !strings.Contains(out, "no updates · checked just now") || strings.Contains(out, "↑") {
		t.Fatalf("list after update:\n%s", out)
	}

	// A pack that cannot be fetched: listed as failed, non-zero exit, the
	// others still checked.
	f, _ := os.OpenFile(filepath.Join(lib, "skills.toml"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\n[[pack]]\nrepo = \"" + e.p("does-not-exist") + "\"\nuse = [\"x\"]\n")
	f.Close()
	r = e.jsFail("", "outdated")
	if failed, _ := r["failed"].([]any); len(failed) != 1 || !strings.Contains(failed[0].(map[string]any)["repo"].(string), "does-not-exist") {
		t.Fatalf("failure not reported: %v", r)
	}
	if len(statuses(t, r)) != 2 {
		t.Fatalf("the other packs were not checked: %v", r)
	}
	if out, _, code := e.skd("", "outdated"); code == 0 || !strings.Contains(out, "✗") {
		t.Fatalf("outdated with a failing pack: exit %d\n%s", code, out)
	}
	if out := e.ok("", "list"); !strings.Contains(out, "1 could not be checked") {
		t.Fatalf("list does not mention the failed check:\n%s", out)
	}
}
