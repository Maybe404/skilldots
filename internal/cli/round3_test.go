package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Adopting copies named otherwise than in the library: one that holds
// exactly a subscribed skill is that skill (installed for the agent, the
// copy to the trash); one whose SKILL.md name is taken is copied under
// its directory's name, which the dry run says. Neither leaves a name
// clash for apply to skip ever after.
func TestAdoptRenamedCopies(t *testing.T) {
	e := newEnv(t)
	e.installed(".claude")
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok(lib, "targets", "default", "agents")
	e.write("toolkit/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: test first\n---\nbody\n")
	up := e.repo("toolkit")
	e.ok(lib, "add", up, "--skill", "tdd")
	e.write(".claude/skills/tdd-copy/SKILL.md", "---\nname: tdd\ndescription: test first\n---\nbody\n")
	e.write(".claude/skills/other/SKILL.md", "---\nname: tdd\ndescription: something else\n---\nmine\n")

	out := e.ok(lib, "adopt", "-n")
	if !strings.Contains(out, "already in the library as tdd") || !strings.Contains(out, "as other: its SKILL.md name tdd is taken") {
		t.Fatalf("adopt -n:\n%s", out)
	}
	out = e.ok(lib, "adopt", "--yes")
	if strings.Contains(out, "name conflict") || strings.Contains(out, "both") {
		t.Fatalf("adopt:\n%s", out)
	}
	isLink(t, e.p(".claude/skills/tdd"), lib)
	isLink(t, e.p(".claude/skills/other"), lib)
	if _, err := os.Lstat(e.p(".claude/skills/tdd-copy")); !os.IsNotExist(err) {
		t.Fatalf("the copy of tdd stayed: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(lib, "skills", "other", "SKILL.md")); !strings.Contains(string(b), "name: other") || !strings.Contains(string(b), "mine") {
		t.Fatalf("skills/other:\n%s", b)
	}
	if out, errOut, _ := e.skd(lib, "apply"); !strings.Contains(out, "up to date") || strings.Contains(out+errOut, "skipped") || strings.Contains(out+errOut, "conflict") {
		t.Fatalf("apply after adopt:\n%s%s", out, errOut)
	}
}

// A copy that differs from the library's skill, in an agent that reads
// the library's from ~/.agents/skills, is the user's to settle: adopt
// says so in every mode, and scan says so instead of "skd adopt".
func TestAdoptSaysWhatItLeaves(t *testing.T) {
	e := newEnv(t)
	e.installed(".gemini")
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok(lib, "targets", "default", "agents")
	e.ok(lib, "new", "gamma")
	e.write(".gemini/skills/gamma/SKILL.md", "---\nname: gamma\ndescription: mine\n---\nother\n")
	const why = "differs from the library's gamma, which gemini already reads from ~/.agents/skills"
	if out := e.ok(lib, "adopt", "--yes"); !strings.Contains(out, why) {
		t.Errorf("adopt --yes:\n%s", out)
	}
	out := e.ok(lib, "scan")
	if !strings.Contains(out, why) || !strings.Contains(out, "1 entry needs you") || strings.Contains(out, "take these over with") {
		t.Errorf("scan:\n%s", out)
	}
	if _, err := os.Stat(e.p(".gemini/skills/gamma/SKILL.md")); err != nil {
		t.Fatal("the copy was touched")
	}
}

// A pack added from a file:// URL is keyed by its path, and the name
// shown, the URL, the path and the directory's name all find it: list,
// update and purge take any of them. An older skd's key ("file////…")
// is found by the URL too.
func TestFileURLPackByAnyName(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill("demo-skills/skills/alpha", "alpha")
	dir := e.repo("demo-skills")
	url := "file://" + dir
	e.ok(lib, "add", url, "--all")
	if b, _ := os.ReadFile(filepath.Join(lib, "skills.toml")); !strings.Contains(string(b), "repo = '"+dir+"'") {
		t.Fatalf("skills.toml:\n%s", b)
	}
	for _, name := range []string{dir, url, "demo-skills"} {
		if out := e.ok(lib, "list", name); !strings.Contains(out, "alpha") {
			t.Errorf("list %s:\n%s", name, out)
		}
	}
	e.ok(lib, "update", url, "--yes")
	e.ok(lib, "purge", url, "--yes")
	if b, _ := os.ReadFile(filepath.Join(lib, "skills.toml")); strings.Contains(string(b), "[[pack]]") {
		t.Fatalf("not purged:\n%s", b)
	}
	// The key an older skd wrote.
	e.ok(lib, "trash", "restore", dir)
	b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	old := strings.Replace(string(b), "repo = '"+dir+"'", "repo = 'file///"+dir+"'", 1)
	os.WriteFile(filepath.Join(lib, "skills.toml"), []byte(old), 0o644)
	if out := e.ok(lib, "list", url); !strings.Contains(out, "file///") {
		t.Errorf("list by URL of an older key:\n%s", out)
	}
	e.ok(lib, "purge", "file///"+dir, "--yes")
}

// Paths in the home directory are ~/… in scan's headings, adopt's
// progress, name-clash warnings and skd new; list --agent shows where a
// skill comes from, not skd's internal prefix; a broken link is 断链 and
// goes to the trash as one.
func TestRoundThreeWording(t *testing.T) {
	e := newEnv(t)
	e.installed(".claude")
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok(lib, "targets", "default", "agents", "claude")
	if out := e.ok(lib, "new", "x"); !strings.Contains(out, "created ~/lib/skills/x") {
		t.Errorf("new:\n%s", out)
	}
	e.skill("up/skills/tdd", "tdd")
	up := e.repo("up")
	e.ok(lib, "add", up, "--skill", "tdd")
	if out := e.ok(lib, "list", "--agent", "agents"); strings.Contains(out, "vendor:") || !strings.Contains(out, "skills/") {
		t.Errorf("list --agent:\n%s", out)
	}
	os.Symlink(e.p("gone"), e.p(".claude/skills/old-notes"))
	if out := e.ok(lib, "scan"); !strings.Contains(out, "claude ~/.claude/skills") || strings.Contains(out, e.home) {
		t.Errorf("scan:\n%s", out)
	}
	e.setLang("zh")
	if out := e.ok(lib, "scan"); !strings.Contains(out, "断链") || strings.Contains(out, "失效") {
		t.Errorf("scan zh:\n%s", out)
	}
	e.setLang("en")
	out := e.ok(lib, "adopt", "--yes")
	if !strings.Contains(out, "moved broken link ~/.claude/skills/old-notes") {
		t.Errorf("adopt:\n%s", out)
	}
	if out := e.ok(lib, "trash"); !strings.Contains(out, "broken link in an agent directory") {
		t.Errorf("trash:\n%s", out)
	}
	// A name clash: the warning names both directories as ~/….
	e.skill("lib/private/x", "x")
	_, errOut, _ := e.skd(lib, "apply")
	if !strings.Contains(errOut, "~/lib/skills/x") || strings.Contains(errOut, e.home) {
		t.Errorf("clash warning:\n%s", errOut)
	}
}

// skd targets refuses a subcommand it does not have, naming those it has;
// its note on agents that read ~/.agents/skills covers a skill installed
// for one by name, not only "*".
func TestTargetsSubcommandsAndNote(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	out, errOut, code := e.skd(lib, "targets", "disable", "agents")
	if code == 0 || !strings.Contains(errOut, `unknown subcommand "disable" of skd targets`) || !strings.Contains(errOut, "pause") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if out := e.ok(lib, "targets"); !strings.Contains(out, "by name") || !strings.Contains(out, "paused") {
		t.Errorf("targets:\n%s", out)
	}
}

// A broken trash-notes.json is set aside the first time skd moves
// something to the trash, and the command says so: the warning is not
// lost on the way.
func TestBrokenTrashNotesAreSaid(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok(lib, "new", "foo")
	notes := filepath.Join(filepath.Dir(e.configPath()), "trash-notes.json")
	os.MkdirAll(filepath.Dir(notes), 0o755)
	os.WriteFile(notes, []byte("{broken"), 0o644)
	out, errOut, code := e.skd(lib, "remove", "foo")
	if code != 0 || !strings.Contains(out+errOut, "trash-notes.json was not valid JSON") {
		t.Fatalf("exit %d:\n%s%s", code, out, errOut)
	}
	backups, _ := filepath.Glob(notes + ".broken-*")
	if len(backups) != 1 {
		t.Fatalf("backups %v", backups)
	}
	if b, _ := os.ReadFile(backups[0]); string(b) != "{broken" {
		t.Fatalf("backup holds %q", b)
	}
}
