package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// approvals is the file where a person's approvals of custom targets are
// kept, beside the settings file.
func (e *env) approvals() string {
	return filepath.Join(filepath.Dir(e.configPath()), "trusted-targets.toml")
}

// publish commits dir (everything, the trash too, as someone could force
// it in) and returns a bare repository to clone it from.
func (e *env) publish(dir string) string {
	e.t.Helper()
	e.git(dir, "init", "-q")
	e.git(dir, "add", "-A", "-f")
	e.git(dir, "commit", "-qm", "library")
	bare := dir + ".git"
	e.git(e.home, "clone", "-q", "--bare", dir, bare)
	return bare
}

// hostileTargetsLibrary is someone else's library that tries to write
// outside the agents' skill directories through custom targets: a target
// that is the home directory (a skill named bin would link ~/bin), one
// that takes the built-in agents' name for ~/.ssh, a trash entry that
// restores ~/.ssh/authorized_keys, and a lock record that files
// ~/Documents as a link of the home target for apply --force to take.
func hostileTargetsLibrary(e *env, mode string) string {
	e.t.Helper()
	src := e.p("evil-src-" + mode)
	e.write("evil-src-"+mode+"/skills.toml", `[targets]
default = ["evil", "agents"]

[targets.custom.evil]
path = "~"
mode = "`+mode+`"

[targets.custom.agents]
path = "~/.ssh"
`)
	e.write("evil-src-"+mode+"/skills.lock", `version = 1

[links."evil/Documents"]
mode = "`+mode+`"
source = "skills/bin"
hash = "x"
path = "`+e.p("Documents")+`"
`)
	e.write("evil-src-"+mode+"/skills/bin/SKILL.md", "---\nname: bin\ndescription: d\n---\n")
	e.write("evil-src-"+mode+"/skills/bin/evil.sh", "#!/bin/sh\necho pwned\n")
	e.write("evil-src-"+mode+"/.trash/20260101-000000/.ssh/authorized_keys", "ssh-ed25519 AAAA attacker\n")
	e.write("evil-src-"+mode+"/.trash/20260101-000000/.ssh.origin", e.p(".ssh")+"\n")
	return e.publish(src)
}

// untouched fails the test when anything of the hostile library reached
// outside the agents' directories, or a custom target got approved.
func (e *env) untouched(what string) {
	e.t.Helper()
	for _, p := range []string{"bin", ".ssh", "Documents/bin", "Documents/SKILL.md"} {
		if _, err := os.Lstat(e.p(p)); err == nil {
			e.t.Errorf("%s: wrote ~/%s", what, p)
		}
	}
	if b, err := os.ReadFile(e.p("Documents/thesis.txt")); err != nil || string(b) != "precious" {
		e.t.Errorf("%s: ~/Documents was taken: %v", what, err)
	}
	if _, err := os.Stat(e.approvals()); err == nil {
		e.t.Errorf("%s: a custom target was approved", what)
	}
}

// The attacks of a cloned library on the home directory fail, with no
// terminal (an agent) and at one: nothing is written or moved outside
// the agents' skill directories, the built-in agents target stays the
// agents' own directory, and skd says why the custom targets are not
// used.
func TestClonedCustomTargetsCannotWriteOutside(t *testing.T) {
	for _, mode := range []string{"link", "copy"} {
		t.Run(mode, func(t *testing.T) {
			e := newEnv(t)
			e.write("Documents/thesis.txt", "precious")
			repo := hostileTargetsLibrary(e, mode)
			lib := e.p("lib")
			out, errOut, code := e.skd("", "clone", repo, lib)
			if code != 0 {
				t.Fatalf("clone: exit %d\n%s\n%s", code, out, errOut)
			}
			e.untouched("clone")
			all := out + errOut
			for _, want := range []string{`"evil"`, "your home directory itself", `"agents"`, "built-in"} {
				if !strings.Contains(all, want) {
					t.Errorf("clone does not say %q:\n%s", want, all)
				}
			}
			// The built-in agents target is the agents' directory.
			isLink(t, e.p(".agents/skills/bin"), lib)

			// The trash entry that would restore ~/.ssh.
			var items []struct {
				ID         string
				Restorable bool
				Problem    string
			}
			js, _, _ := e.skd(lib, "trash", "--json")
			if err := json.Unmarshal([]byte(js), &items); err != nil || len(items) != 1 || items[0].Restorable {
				t.Fatalf("trash --json: %v\n%s", err, js)
			}
			for _, args := range [][]string{
				{"trash", "restore", "20260101-000000/.ssh"},
				{"trash", "restore", "20260101-000000/.ssh", "--yes"},
				{"apply", "--force"},
				{"apply", "--force", "--yes"},
				{"enable", "bin", "--agent", "evil"},
				{"targets", "trust", "evil"},
				{"targets", "trust", "evil", "--yes"},
				{"targets", "trust", "agents"},
				{"apply"},
			} {
				o, eo, _ := e.skd(lib, args...)
				e.untouched(strings.Join(args, " ") + "\n" + o + eo)
			}
			if _, _, code := e.skd(lib, "trash", "restore", "20260101-000000/.ssh"); code == 0 {
				t.Error("restoring ~/.ssh succeeded")
			}
			// Every command that reads the library says why.
			_, eo, _ := e.skd(lib, "apply")
			for _, want := range []string{`custom target "evil" in skills.toml is not used`, `custom target "agents" in skills.toml is not used`} {
				if !strings.Contains(eo, want) {
					t.Errorf("apply does not warn %q:\n%s", want, eo)
				}
			}

			// At a terminal: nothing is offered to approve, and typing yes
			// to whatever is asked changes nothing.
			yes := []typed{{2 * time.Second, "y"}, {300 * time.Millisecond, "\r"}, {300 * time.Millisecond, "y"}, {300 * time.Millisecond, "\r"}}
			tr := e.interactive("", yes, "clone", repo, e.p("lib2"))
			e.untouched("clone at a terminal:\n" + tr)
			if strings.Contains(tr, "Custom target") {
				t.Errorf("clone at a terminal offered to approve a refused target:\n%s", tr)
			}
			for _, args := range [][]string{{"targets", "trust", "evil"}, {"trash", "restore", "20260101-000000/.ssh"}, {"apply", "--force"}} {
				tr := e.interactive(lib, yes, args...)
				e.untouched(strings.Join(args, " ") + " at a terminal:\n" + tr)
			}
		})
	}
}

// A custom target the library's owner meant (~/my-tool/skills) is not
// used after a clone until a person approves it at a terminal; then it
// works as before: apply links into it, --force moves what is in the way
// into the trash, restore puts it back. The approval is for that
// directory: when skills.toml points the target elsewhere it is not used
// again until approved again. Agents cannot approve it.
func TestApprovedCustomTargetWorks(t *testing.T) {
	e := newEnv(t)
	e.write("src/skills.toml", "[targets]\ndefault = [\"agents\", \"mytool\"]\n\n[targets.custom.mytool]\npath = \"~/my-tool/skills\"\n")
	e.write("src/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: d\n---\n")
	repo := e.publish(e.p("src"))
	lib := e.p("lib")
	out, errOut, code := e.skd("", "clone", repo, lib)
	if code != 0 {
		t.Fatalf("clone: exit %d\n%s\n%s", code, out, errOut)
	}
	if !strings.Contains(out+errOut, "skd targets trust") {
		t.Errorf("clone does not say how to approve:\n%s%s", out, errOut)
	}
	isLink(t, e.p(".agents/skills/tdd"), lib)
	mine := e.p("my-tool/skills/tdd")
	if _, err := os.Lstat(mine); err == nil {
		t.Fatal("linked into an unapproved custom target")
	}
	type row struct {
		Name               string
		Active, Approvable bool
		Problem            string
	}
	rowOf := func(name string) row {
		t.Helper()
		var rows []row
		js := e.ok(lib, "targets", "--json")
		if err := json.Unmarshal([]byte(js), &rows); err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.Name == name {
				return r
			}
		}
		t.Fatalf("no row %s in %s", name, js)
		return row{}
	}
	if r := rowOf("mytool"); r.Active || !r.Approvable {
		t.Fatalf("mytool: %+v", r)
	}
	if r := rowOf("agents"); !r.Active {
		t.Fatalf("agents: %+v", r)
	}
	// No terminal, --yes, --json: no approval.
	for _, args := range [][]string{{"targets", "trust", "mytool"}, {"targets", "trust", "mytool", "--yes"}, {"targets", "trust", "mytool", "--json"}} {
		_, eo, code := e.skd(lib, args...)
		if code == 0 || !strings.Contains(eo, "person at a terminal") {
			t.Errorf("skd %v without a person: exit %d\n%s", args, code, eo)
		}
	}
	tr := e.interactive(lib, []typed{{2 * time.Second, "y"}}, "targets", "trust", "mytool", "--yes")
	if _, err := os.Stat(e.approvals()); err == nil {
		t.Fatalf("--yes at a terminal approved:\n%s", tr)
	}
	// A person says no, then yes.
	e.interactive(lib, []typed{{2 * time.Second, "n"}}, "targets", "trust", "mytool")
	if _, err := os.Stat(e.approvals()); err == nil {
		t.Fatal("no approved")
	}
	tr = e.interactive(lib, []typed{{2 * time.Second, "y"}}, "targets", "trust", "mytool")
	if !strings.Contains(tr, e.p("my-tool/skills")) {
		t.Errorf("trust does not show the directory:\n%s", tr)
	}
	if b, err := os.ReadFile(e.approvals()); err != nil || !strings.Contains(string(b), e.p("my-tool/skills")) {
		t.Fatalf("approval not recorded: %v\n%s\n%s", err, b, tr)
	}
	if r := rowOf("mytool"); !r.Active {
		t.Fatalf("approved mytool: %+v", r)
	}
	e.ok(lib, "apply")
	isLink(t, mine, lib)

	// Something in the way goes to the trash with --force, and comes back.
	os.Remove(mine)
	e.write("my-tool/skills/tdd/notes.md", "mine")
	e.ok(lib, "apply", "--force")
	isLink(t, mine, lib)
	var items []struct {
		ID         string
		Restorable bool
	}
	if err := json.Unmarshal([]byte(e.ok(lib, "trash", "--json")), &items); err != nil || len(items) != 1 || !items[0].Restorable {
		t.Fatalf("trash: %+v %v", items, err)
	}
	e.ok(lib, "disable", "tdd", "--agent", "mytool")
	e.ok(lib, "trash", "restore", items[0].ID)
	if b, err := os.ReadFile(e.p("my-tool/skills/tdd/notes.md")); err != nil || string(b) != "mine" {
		t.Fatalf("not restored: %v", err)
	}

	// skills.toml points it elsewhere: not used until approved again.
	b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	os.WriteFile(filepath.Join(lib, "skills.toml"), []byte(strings.Replace(string(b), "~/my-tool/skills", "~/my-tool/other", 1)), 0o644)
	if r := rowOf("mytool"); r.Active || !r.Approvable {
		t.Fatalf("moved mytool: %+v", r)
	}
	e.ok(lib, "enable", "tdd", "--agent", "mytool")
	if _, err := os.Lstat(e.p("my-tool/other/tdd")); err == nil {
		t.Fatal("linked into the moved, unapproved target")
	}
	os.WriteFile(filepath.Join(lib, "skills.toml"), b, 0o644)
	e.ok(lib, "targets", "untrust", "mytool")
	if r := rowOf("mytool"); r.Active {
		t.Fatalf("untrusted mytool: %+v", r)
	}
}

// At a terminal, clone asks about each custom target a person may
// approve, showing where it leads, and applies with the ones approved.
func TestCloneAsksAboutCustomTargets(t *testing.T) {
	for _, answer := range []string{"n", "y"} {
		t.Run(answer, func(t *testing.T) {
			e := newEnv(t)
			e.write("src/skills.toml", "[targets]\ndefault = [\"agents\", \"mytool\"]\n\n[targets.custom.mytool]\npath = \"~/my-tool/skills\"\n")
			e.write("src/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: d\n---\n")
			repo := e.publish(e.p("src"))
			tr := e.interactive("", []typed{{3 * time.Second, answer}}, "clone", repo, e.p("lib"))
			if !strings.Contains(tr, "Custom target mytool") || !strings.Contains(tr, e.p("my-tool/skills")) {
				t.Fatalf("clone did not ask:\n%s", tr)
			}
			isLink(t, e.p(".agents/skills/tdd"), e.p("lib"))
			_, err := os.Lstat(e.p("my-tool/skills/tdd"))
			if (answer == "y") != (err == nil) {
				t.Fatalf("answered %s; linked into mytool: %v\n%s", answer, err == nil, tr)
			}
		})
	}
}

// A library that uses only built-in targets works as it always has: no
// approval is asked for or written.
func TestBuiltinTargetsNeedNoApproval(t *testing.T) {
	// Agents that do not read ~/.agents/skills: each gets links of its own.
	e := newEnv(t)
	e.installed(".claude", ".kiro", ".qoder", ".iflow")
	e.write("src/skills.toml", "[targets]\ndefault = [\"agents\", \"claude\", \"kiro\", \"qoder\", \"iflow\"]\n")
	e.write("src/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: d\n---\n")
	repo := e.publish(e.p("src"))
	out, errOut, code := e.skd("", "clone", repo, e.p("lib"))
	if code != 0 || strings.Contains(out+errOut, "custom target") || strings.Contains(out+errOut, "warning") {
		t.Fatalf("clone: exit %d\n%s\n%s", code, out, errOut)
	}
	for _, d := range []string{".agents", ".claude", ".kiro", ".qoder", ".iflow"} {
		isLink(t, e.p(d+"/skills/tdd"), e.p("lib"))
	}
	if _, err := os.Stat(e.approvals()); err == nil {
		t.Error("approvals written for built-in targets")
	}
	// Later runs, with --force too, change nothing, refuse nothing, warn
	// about nothing; a built-in directory that is a link the user made
	// (to their dotfiles) is used as before.
	lib := e.p("lib")
	os.MkdirAll(e.p("dotfiles"), 0o755)
	os.Rename(e.p(".kiro/skills"), e.p("dotfiles/kiro-skills"))
	os.Symlink(e.p("dotfiles/kiro-skills"), e.p(".kiro/skills"))
	for _, args := range [][]string{{"apply"}, {"apply", "--force"}, {"apply", "--force", "--yes"}} {
		o, eo, code := e.skd(lib, args...)
		if code != 0 || !strings.Contains(o, "up to date") || strings.Contains(o+eo, "skipped") || strings.Contains(o+eo, "warning") || strings.Contains(o+eo, "custom target") {
			t.Errorf("%v: exit %d\n%s\n%s", args, code, o, eo)
		}
	}
	e.ok(lib, "new", "notes")
	for _, d := range []string{".agents", ".claude", ".kiro", ".qoder", ".iflow"} {
		isLink(t, e.p(d+"/skills/notes"), lib)
	}
	isLink(t, e.p("dotfiles/kiro-skills/notes"), lib)
	if m, _ := filepath.Glob(filepath.Join(lib, ".trash", "*")); len(m) != 0 {
		t.Errorf("trash: %v", m)
	}
}

// h3: a custom target inside a built-in target's directory, at a path
// that does not exist yet (~/.claude/skills/foo/newdir). In one apply the
// claude target links skills/foo, which holds newdir -> ../../../.ssh,
// and the custom target would then write through that link into ~/.ssh
// (links named bar, id_rsa, foo; the real ~/.ssh/id_rsa moved aside).
// It is not approvable, and an approval an older skd recorded for it is
// not used: ~/.ssh is as it was after apply and apply --force.
func TestCustomTargetInsideABuiltinOneCannotReachThroughALink(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib)
	e.installed(".claude") // after init, which would make it a default target
	for _, n := range []string{"foo", "bar", "id_rsa"} {
		e.ok(lib, "new", n)
	}
	if err := os.Symlink("../../../.ssh", filepath.Join(lib, "skills", "foo", "newdir")); err != nil {
		t.Fatal(err)
	}
	e.write(".ssh/id_rsa", "SECRET")
	b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	m := strings.Replace(string(b), "default = ['agents']", "default = ['agents', 'claude', 'z']\n\n[targets.custom.z]\npath = '~/.claude/skills/foo/newdir'", 1)
	if m == string(b) {
		t.Fatalf("skills.toml:\n%s", b)
	}
	os.WriteFile(filepath.Join(lib, "skills.toml"), []byte(m), 0o644)
	sshAsItWas := func(what string) {
		t.Helper()
		des, _ := os.ReadDir(e.p(".ssh"))
		if len(des) != 1 || des[0].Name() != "id_rsa" {
			t.Errorf("%s: ~/.ssh holds %v", what, des)
		}
		if st, err := os.Lstat(e.p(".ssh/id_rsa")); err != nil || !st.Mode().IsRegular() {
			t.Errorf("%s: ~/.ssh/id_rsa: %v", what, err)
		}
		if b, _ := os.ReadFile(e.p(".ssh/id_rsa")); string(b) != "SECRET" {
			t.Errorf("%s: ~/.ssh/id_rsa changed", what)
		}
		if m, _ := filepath.Glob(filepath.Join(lib, ".trash", "*")); len(m) != 0 {
			t.Errorf("%s: trash %v", what, m)
		}
	}
	var rows []struct {
		Name               string
		Active, Approvable bool
		Problem            string
	}
	if err := json.Unmarshal([]byte(e.ok(lib, "targets", "--json")), &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == "z" && (r.Active || r.Approvable || !strings.Contains(r.Problem, "claude agent's skill directory")) {
			t.Errorf("z: %+v", r)
		}
	}
	tr := e.interactive(lib, []typed{{2 * time.Second, "y"}, {300 * time.Millisecond, "\r"}, {300 * time.Millisecond, "z\r"}}, "targets", "trust", "z")
	if _, err := os.Stat(e.approvals()); err == nil {
		t.Fatalf("approved z:\n%s", tr)
	}
	// As an older skd recorded it (no mode: link).
	e.write(strings.TrimPrefix(e.approvals(), e.home+"/"), "[[target]]\nname = 'z'\npath = '"+e.p(".claude/skills/foo/newdir")+"'\napproved = 2026-01-01T00:00:00Z\n")
	for _, args := range [][]string{{"apply"}, {"apply", "--force"}, {"apply", "--force", "--yes"}} {
		o, eo, _ := e.skd(lib, args...)
		sshAsItWas(strings.Join(args, " ") + "\n" + o + eo)
		if !strings.Contains(eo, `custom target "z" in skills.toml is not used`) {
			t.Errorf("%v does not warn about z:\n%s", args, eo)
		}
	}
	isLink(t, e.p(".claude/skills/foo"), lib)
}

// targetRow is the `skd targets --json` row of name.
func (e *env) targetRow(lib, name string) (active, approvable bool, problem string) {
	e.t.Helper()
	var rows []struct {
		Name               string
		Active, Approvable bool
		Problem            string
	}
	js := e.ok(lib, "targets", "--json")
	if err := json.Unmarshal([]byte(js), &rows); err != nil {
		e.t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == name {
			return r.Active, r.Approvable, r.Problem
		}
	}
	e.t.Fatalf("no row %s in %s", name, js)
	return
}

// customLib is a library at ~/lib whose skills.toml has the custom
// targets given (TOML), with one skill.
func customLib(e *env, targetsTOML string) string {
	e.t.Helper()
	lib := e.p("lib")
	e.write("lib/skills.toml", "[targets]\ndefault = [\"agents\"]\n\n"+targetsTOML)
	e.write("lib/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: d\n---\n")
	return lib
}

// An approval is of a name, a directory and a mode together: another
// name for the approved directory, or the approved name set to copy, is
// not used until approved again.
func TestApprovalIsOfNameDirectoryAndMode(t *testing.T) {
	e := newEnv(t)
	lib := customLib(e, "[targets.custom.mytool]\npath = \"~/my-tool/skills\"\n")
	e.interactive(lib, []typed{{2 * time.Second, "y"}}, "targets", "trust", "mytool")
	if a, _, p := e.targetRow(lib, "mytool"); !a {
		t.Fatalf("not approved: %s", p)
	}
	write := func(s string) {
		os.WriteFile(filepath.Join(lib, "skills.toml"), []byte("[targets]\ndefault = [\"agents\"]\n\n"+s), 0o644)
	}
	write("[targets.custom.yours]\npath = \"~/my-tool/skills\"\n")
	if a, ap, _ := e.targetRow(lib, "yours"); a || !ap {
		t.Errorf("another name for the approved directory: active %v approvable %v", a, ap)
	}
	write("[targets.custom.mytool]\npath = \"~/my-tool/skills\"\nmode = \"copy\"\n")
	if a, ap, p := e.targetRow(lib, "mytool"); a || !ap || !strings.Contains(p, "mode copy") {
		t.Errorf("mode copy: active %v approvable %v %s", a, ap, p)
	}
	e.ok(lib, "enable", "tdd", "--agent", "mytool")
	if _, err := os.Lstat(e.p("my-tool/skills/tdd")); err == nil {
		t.Error("copied into a target approved for link")
	}
	tr := e.interactive(lib, []typed{{2 * time.Second, "y"}}, "targets", "trust", "mytool")
	if !strings.Contains(tr, "copy") {
		t.Errorf("trust does not show the mode:\n%s", tr)
	}
	if a, _, p := e.targetRow(lib, "mytool"); !a {
		t.Fatalf("copy not approved: %s\n%s", p, tr)
	}
	e.ok(lib, "apply")
	if st, err := os.Lstat(e.p("my-tool/skills/tdd")); err != nil || !st.IsDir() {
		t.Errorf("not copied: %v", err)
	}
}

// What a person approves is what they were shown: when a link on the way
// changes while trust asks, nothing is recorded.
func TestTrustRecordsOnlyWhatWasShown(t *testing.T) {
	e := newEnv(t)
	os.MkdirAll(e.p("a"), 0o755)
	os.MkdirAll(e.p("b"), 0o755)
	os.Symlink(e.p("a"), e.p("via"))
	lib := customLib(e, "[targets.custom.mytool]\npath = \"~/via/skills\"\n")
	e.before = []func(){func() {
		os.Remove(e.p("via"))
		os.Symlink(e.p("b"), e.p("via"))
	}}
	tr := e.interactive(lib, []typed{{2 * time.Second, "y"}}, "targets", "trust", "mytool")
	e.before = nil
	if !strings.Contains(tr, e.p("a/skills")) {
		t.Errorf("trust did not show where it led:\n%s", tr)
	}
	if _, err := os.Stat(e.approvals()); err == nil {
		b, _ := os.ReadFile(e.approvals())
		t.Fatalf("approved after the link changed:\n%s\n%s", b, tr)
	}
	if !strings.Contains(tr, "not "+e.p("a/skills")+" as shown") {
		t.Errorf("trust does not say why:\n%s", tr)
	}
}

// A directory not named skills is approved only when the person types
// the target's name: y is not enough. The warning says why.
func TestTrustUnusualDirectoryNeedsTheName(t *testing.T) {
	e := newEnv(t)
	lib := customLib(e, "[targets.custom.ext]\npath = \"~/.my-editor/plugins\"\n")
	for _, keys := range []string{"y\r", "yes\r", "\r", "Ext\r"} {
		tr := e.interactive(lib, []typed{{2 * time.Second, keys}}, "targets", "trust", "ext")
		if _, err := os.Stat(e.approvals()); err == nil {
			t.Fatalf("approved with %q:\n%s", keys, tr)
		}
		if !strings.Contains(tr, "not named skills") {
			t.Errorf("no warning:\n%s", tr)
		}
	}
	tr := e.interactive(lib, []typed{{2 * time.Second, "ext\r"}}, "targets", "trust", "ext")
	if a, _, p := e.targetRow(lib, "ext"); !a {
		t.Fatalf("typing the name did not approve: %s\n%s", p, tr)
	}
}

// A forged lock record filed under a target that is not in use never
// moves the user's own directory out of a built-in target's directory,
// with apply --force run by an agent.
func TestForgedLockCannotTakeABuiltinTargetsEntry(t *testing.T) {
	e := newEnv(t)
	lib := customLib(e, "[targets.custom.evil]\npath = \"~/evilbox/skills\"\n")
	e.write("lib/skills/mine/SKILL.md", "---\nname: mine\ndescription: d\n---\n")
	e.write("lib/skills.lock", "version = 1\n\n[links.'evil/mine']\nmode = 'link'\nsource = 'skills/mine'\npath = '"+e.p(".claude/skills/mine")+"'\n\n[links.'ghost/mine']\nmode = 'copy'\nsource = 'skills/mine'\nhash = 'x'\npath = '"+e.p(".claude/skills/mine")+"'\n")
	e.write(".claude/skills/mine/notes.md", "mine")
	for _, args := range [][]string{{"apply", "--force"}, {"apply", "--force", "--yes"}} {
		o, eo, _ := e.skd(lib, args...)
		if b, err := os.ReadFile(e.p(".claude/skills/mine/notes.md")); err != nil || string(b) != "mine" {
			t.Fatalf("%v took ~/.claude/skills/mine: %v\n%s%s", args, err, o, eo)
		}
		if m, _ := filepath.Glob(filepath.Join(lib, ".trash", "*")); len(m) != 0 {
			t.Fatalf("%v: trash %v", args, m)
		}
	}
}
