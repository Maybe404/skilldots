package cli_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oneJSON checks that stdout holds exactly one JSON document and decodes it.
func oneJSON(t *testing.T, args []string, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("skd %v: stdout is not JSON: %v\n%s", args, err, stdout)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("skd %v: more than one JSON document on stdout:\n%s", args, stdout)
	}
	m, _ := v.(map[string]any)
	return m
}

// js runs a command with --json, requires exit 0 and a single JSON document.
func (e *env) js(dir string, args ...string) map[string]any {
	e.t.Helper()
	args = append(args, "--json")
	out, errOut, code := e.skd(dir, args...)
	if code != 0 {
		e.t.Fatalf("skd %v: exit %d\nstdout: %s\nstderr: %s", args, code, out, errOut)
	}
	return oneJSON(e.t, args, out)
}

// jsFail runs a command with --json and requires a non-zero exit with
// stdout either empty or a single JSON document.
func (e *env) jsFail(dir string, args ...string) map[string]any {
	e.t.Helper()
	args = append(args, "--json")
	out, _, code := e.skd(dir, args...)
	if code == 0 {
		e.t.Fatalf("skd %v: want failure, got exit 0\n%s", args, out)
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	return oneJSON(e.t, args, out)
}

func TestJSONContract(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	// init installing the skilldots skill from a fake "GitHub" in the cache.
	e.skill("selfsrc/skills/skilldots", "skilldots")
	e.git(e.home, "clone", "-q", e.repo("selfsrc"), e.p("cache/Maybe404__skilldots"))
	if r := e.js("", "init", lib); r["skill"] != true {
		t.Fatalf("init: %v", r)
	}

	e.skill("up/skills/x/a", "a")
	e.skill("up/skills/x/b", "b")
	e.skill("up/skills/y/c", "c")
	up := e.repo("up")

	e.jsFail("", "add", up, "--category", "nosuch")
	if b, _ := os.ReadFile(filepath.Join(lib, "skills.toml")); strings.Contains(string(b), up) {
		t.Fatal("failed add left the pack in skills.toml")
	}
	r := e.js("", "add", up, "--skill", "a")
	if skills, _ := r["skills"].([]any); len(skills) != 1 {
		t.Fatalf("add: %v", r)
	}
	e.js("", "add", up, "--category", "x")
	e.js("", "list")
	e.js("", "list", "--flat")
	e.js("", "apply")
	e.js("", "apply", "-n")
	e.js("", "targets")
	e.js("", "show", up)
	e.js("", "scan")
	e.js("", "path", "a")
	e.js("", "new", "mine")
	e.js("", "enable", "a", "--agent", "claude")
	e.js("", "disable", "a", "--agent", "claude")
	e.js("", "fork", "b")
	e.js("", "use", lib)
	e.js("", "sync")
	e.js("", "adopt", "--dry-run")

	// update: lock written, links rebuilt, single document.
	e.skill("up/skills/y/d", "d")
	e.write("up/skills/x/a/extra.md", "more\n")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "c2")
	head := e.git(up, "rev-parse", "HEAD")
	os.Remove(e.p(".agents/skills/a"))
	r = e.js("", "update")
	if lockb, _ := os.ReadFile(filepath.Join(lib, "skills.lock")); !strings.Contains(string(lockb), head) {
		t.Fatalf("update --json did not write the lock (want commit %s)", head)
	}
	if _, err := os.Lstat(e.p(".agents/skills/a")); err != nil {
		t.Fatal("update --json did not re-link")
	}
	if packs, _ := r["packs"].([]any); len(packs) != 2 {
		t.Fatalf("update packs: %v", r["packs"])
	}

	// A pack that cannot be fetched: non-zero exit, failure in the JSON,
	// and the other packs still updated.
	f, _ := os.OpenFile(filepath.Join(lib, "skills.toml"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\n[[pack]]\nrepo = \"" + e.p("does-not-exist") + "\"\nuse = [\"x\"]\n")
	f.Close()
	r = e.jsFail("", "update")
	if failed, _ := r["failed"].([]any); len(failed) != 1 || !strings.Contains(failed[0].(map[string]any)["repo"].(string), "does-not-exist") {
		t.Fatalf("update failure not reported: %v", r)
	}
	e.ok("", "purge", e.p("does-not-exist"), "--yes")

	// purge needs --yes without a terminal.
	if r := e.jsFail("", "purge", up); r != nil {
		t.Fatalf("purge without --yes printed %v", r)
	}

	// remove until the pack is empty: the last one drops the pack.
	e.js("", "add", up, "--skill", "c")
	if r := e.js("", "remove", "c"); r["apply"] == nil {
		t.Fatalf("remove: %v", r)
	}
	r = e.js("", "remove", "a")
	if r["purged"] == nil {
		t.Fatalf("removing the last skill did not drop the pack: %v", r)
	}
	out, _, _ := e.skd("", "trash", "--json")
	oneJSON(t, []string{"trash"}, out)
	var items []map[string]any
	json.Unmarshal([]byte(out), &items)
	if len(items) == 0 {
		t.Fatal("nothing in the trash after remove")
	}
	e.js("", "trash", "restore", items[0]["id"].(string))
	e.jsFail("", "trash", "empty")

	// clone into a fresh place.
	e.git(lib, "add", "-A")
	e.git(lib, "commit", "-qm", "snap")
	e.git(e.home, "clone", "-q", "--bare", lib, e.p("backup.git"))
	e.js("", "clone", e.p("backup.git"), e.p("lib2"))
}

// update --json when linking fails after the update was saved: stdout
// still has the one document, with the error and the history record that
// says linking failed; a later skd apply that links everything clears it.
func TestUpdateJSONWhenApplyFails(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--all")
	e.skill("up/skills/eng/fresh", "fresh")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "fresh")
	dir := e.p(".agents/skills")
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	out, errOut, code := e.skd("", "update", "--json")
	if code == 0 {
		t.Skip("linking did not fail (running as root?)")
	}
	r := oneJSON(t, []string{"update"}, out)
	if !strings.Contains(errOut, "error:") {
		t.Errorf("no error on stderr: %s", errOut)
	}
	for _, k := range []string{"packs", "failed", "apply", "committed", "history", "error"} {
		if _, ok := r[k]; !ok {
			t.Errorf("no %q in %v", k, r)
		}
	}
	h, _ := r["history"].([]any)
	if len(h) != 1 || h[0].(map[string]any)["applyError"] == nil {
		t.Fatalf("history: %v", r["history"])
	}
	os.Chmod(dir, 0o755)
	e.ok("", "apply")
	recs := e.js("", "history")["records"].([]any)
	if len(recs) != 1 || recs[0].(map[string]any)["applyError"] != nil {
		t.Fatalf("after apply: %v", recs)
	}
}
