package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The first command after upgrading from an skd that kept no record of
// what it placed (no placed.json) treats the app's links as that skd
// did: disabling the app, for every agent or one, removes them
// outright, never to the trash and never left behind. So does taking it
// out of skills.toml by hand once any skd command (skd list here) ran
// since the upgrade; taken out before, nothing names the app any more,
// and its links are left in place, as for any link skd cannot tell it
// made.
func TestOlderAppLinksGoOnTheFirstCommand(t *testing.T) {
	for _, first := range [][]string{{"disable", "x"}, {"disable", "x", "--agent", "claude"}, {"list", "apply"}, {"apply"}} {
		e := newEnv(t)
		e.installed(".claude")
		lib := e.p("lib")
		e.ok("", "init", "--no-skill", "--targets", "agents,claude", lib)
		app := e.p("Apps/X.app/Skills/x")
		e.skill("Apps/X.app/Skills/x", "x")
		e.skill("lib/skills/mine", "mine")
		b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
		os.WriteFile(filepath.Join(lib, "skills.toml"), append(b, []byte("\n[[app]]\nname = \"x\"\npath = \""+app+"\"\n")...), 0o644)
		e.ok(lib, "apply")
		// What the older skd left: the same links, no record of them.
		placed := filepath.Join(filepath.Dir(e.configPath()), "placed.json")
		if err := os.Remove(placed); err != nil {
			t.Fatal(err)
		}
		// Taken out of skills.toml by hand before any command: left.
		left := len(first) == 1 && first[0] == "apply"
		if first[0] == "list" {
			e.ok(lib, "list")
			first = first[1:]
			// Read-only as it is, it writes placed.json (README).
			if _, err := os.Lstat(placed); err != nil {
				t.Fatalf("skd list wrote no placed.json: %v", err)
			}
		}
		if first[0] == "apply" {
			b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
			s := string(b)
			os.WriteFile(filepath.Join(lib, "skills.toml"), []byte(s[:strings.Index(s, "[[app]]")]), 0o644)
		}
		out := e.ok(lib, first...)
		gone := []string{"agents", "claude"}
		if len(first) == 4 {
			gone = []string{"claude"}
			isLink(t, e.p(".agents/skills/x"), app)
		}
		for _, tn := range gone {
			_, err := os.Lstat(e.p("." + tn + "/skills/x"))
			if left {
				isLink(t, e.p("."+tn+"/skills/x"), app)
			} else if err == nil {
				t.Fatalf("%v: the link in %s is still there\n%s", first, tn, out)
			}
		}
		if left != strings.Contains(out, "cannot tell") || strings.Contains(out, "app's") {
			t.Fatalf("%v:\n%s", first, out)
		}
		if _, err := os.Stat(filepath.Join(lib, ".trash")); err == nil {
			t.Fatalf("%v: something went to the trash\n%s", first, out)
		}
		isLink(t, e.p(".claude/skills/mine"), lib)
		if _, err := os.Stat(filepath.Join(app, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

// Someone else's library, cloned and applied by an agent as the first
// command of this skd (no placed.json yet): its skills.lock records the
// user's own links as apps' links, `ln -s ~/projects/my-skill
// ~/.claude/skills/my-skill` and a link from one agent's directory to
// skd's own link in another, and its skills.toml names them as apps or
// not. Neither the clone nor a later change of that skills.toml (the
// [[app]] gone, then apply, or skd disable) removes them or moves them
// to the trash. Nor when the library is cloned by hand and used from
// its directory, or with skd use.
func TestForgedAppRecordsNeverTakeTheUsersLinks(t *testing.T) {
	for _, how := range []string{"skd clone", "skd clone where the recorded library was", "git clone, used from its directory", "git clone, skd use"} {
		for _, declared := range []bool{false, true} {
			e := newEnv(t)
			own := e.p("own")
			e.ok("", "init", "--no-skill", "--targets", "agents", own)
			e.skill("own/skills/via", "via")
			e.ok(own, "apply")
			os.Remove(filepath.Join(filepath.Dir(e.configPath()), "placed.json"))
			e.skill("projects/my-skill", "my-skill")
			os.MkdirAll(e.p(".claude/skills"), 0o755)
			os.Symlink(e.p("projects/my-skill"), e.p(".claude/skills/my-skill"))
			os.Symlink(e.p(".agents/skills/via"), e.p(".claude/skills/via"))
			apps := ""
			if declared {
				apps = "\n[[app]]\nname = \"my-skill\"\npath = \"" + e.p("projects/my-skill") + "\"\n" +
					"\n[[app]]\nname = \"via\"\npath = \"" + e.p(".agents/skills/via") + "\"\n"
			}
			e.write("evil/skills.toml", "[targets]\ndefault = [\"claude\"]\n"+apps)
			e.write("evil/skills.lock", `version = 1

[links.'claude/my-skill']
mode = 'link'
source = '`+e.p("projects/my-skill")+`'
path = '`+e.p(".claude/skills/my-skill")+`'

[links.'claude/via']
mode = 'link'
source = '`+e.p(".agents/skills/via")+`'
path = '`+e.p(".claude/skills/via")+`'
`)
			repo := e.repo("evil")
			lib := e.p("evil-lib")
			var log string
			switch how {
			case "skd clone", "skd clone where the recorded library was":
				if how != "skd clone" {
					// The settings name a library that is gone since.
					lib = e.p("gone")
					e.write("gone/skills.toml", "[targets]\n")
					e.ok("", "use", lib)
					os.Remove(filepath.Join(filepath.Dir(e.configPath()), "placed.json"))
					os.RemoveAll(lib)
				}
				o, er, _ := e.skd("", "clone", repo, lib)
				log = o + er
			default:
				e.git(e.home, "clone", "-q", repo, lib)
				if how == "git clone, skd use" {
					e.ok("", "use", lib)
				}
				o, er, _ := e.skd(lib, "apply")
				log = o + er
			}
			// The attacker's next commit: no [[app]] any more.
			os.WriteFile(filepath.Join(lib, "skills.toml"), []byte("[targets]\ndefault = [\"claude\"]\n"), 0o644)
			o, er, _ := e.skd(lib, "apply", "--force")
			log += o + er
			label := how
			if declared {
				label += ", declared"
			}
			isLink(t, e.p(".claude/skills/my-skill"), e.p("projects/my-skill"))
			if got, err := os.Readlink(e.p(".claude/skills/via")); err != nil || got != e.p(".agents/skills/via") {
				t.Fatalf("%s: the user's link changed: %q %v\n%s", label, got, err, log)
			}
			if _, err := os.Stat(filepath.Join(lib, ".trash")); err == nil {
				t.Fatalf("%s: something went to the trash\n%s", label, log)
			}
			if strings.Contains(log, "app's link") {
				t.Fatalf("%s: called an app's link\n%s", label, log)
			}
		}
	}
}

// On a machine where skd never recorded a library (no config.toml, no
// placed.json), someone's library cloned into ~/.skilldots names the
// user's own `~/.claude/skills/my-skill -> ~/projects/my-skill` as an
// app's link in skills.toml and skills.lock. ~/.skilldots is where skd
// looks then, but it is not the library an older skd made links from
// (that skd recorded every library it used): the first command takes
// nothing for skd's and writes no placed.json, and neither the [[app]]
// taken out and apply, nor disabling it, removes the link.
func TestAnUnrecordedDefaultLibraryTakesNoLinks(t *testing.T) {
	for _, first := range [][]string{{"list"}, {"disable", "my-skill"}} {
		e := newEnv(t)
		e.skill("projects/my-skill", "my-skill")
		user := e.p("projects/my-skill")
		os.MkdirAll(e.p(".claude/skills"), 0o755)
		if err := os.Symlink(user, e.p(".claude/skills/my-skill")); err != nil {
			t.Fatal(err)
		}
		e.write("evil/skills.toml", "[targets]\ndefault = [\"claude\"]\n\n[[app]]\nname = \"my-skill\"\npath = \""+user+"\"\n")
		e.write("evil/skills.lock", "version = 1\n\n[links.'claude/my-skill']\nmode = 'link'\nsource = '"+user+"'\npath = '"+e.p(".claude/skills/my-skill")+"'\n")
		lib := e.p(".skilldots")
		e.git(e.home, "clone", "-q", e.repo("evil"), lib)
		if _, err := os.Stat(e.configPath()); !os.IsNotExist(err) {
			t.Fatalf("a settings file to begin with: %v", err)
		}
		placed := filepath.Join(filepath.Dir(e.configPath()), "placed.json")
		o, er, _ := e.skd("", first...)
		log := o + er
		if first[0] == "list" {
			if _, err := os.Lstat(placed); !os.IsNotExist(err) {
				t.Fatalf("placed.json written: %v\n%s", err, log)
			}
			if !strings.Contains(o, "my-skill") {
				t.Fatalf("not the library at ~/.skilldots:\n%s", log)
			}
			// The attacker's next commit: no [[app]] any more.
			os.WriteFile(filepath.Join(lib, "skills.toml"), []byte("[targets]\ndefault = [\"claude\"]\n"), 0o644)
			o, er, _ = e.skd("", "apply", "-y")
			log += o + er
			if !strings.Contains(o, "cannot tell") {
				t.Fatalf("not skipped as the user's own:\n%s", log)
			}
		}
		isLink(t, e.p(".claude/skills/my-skill"), user)
		if _, err := os.Stat(filepath.Join(lib, ".trash")); err == nil {
			t.Fatalf("%v: something went to the trash\n%s", first, log)
		}
		if strings.Contains(log, "app's link") {
			t.Fatalf("%v: called an app's link\n%s", first, log)
		}
		if _, err := os.Stat(filepath.Join(user, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

// When placed.json cannot be written (the settings directory read-only),
// the app's links an older skd made are still taken for skd's in that
// run: the first `skd disable` removes them from every agent, as that
// skd did, and says once on stderr that it could not record them. With
// nothing written, the next skd looks for them again.
func TestOlderAppLinksWhenTheRecordCannotBeWritten(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", "--targets", "agents,claude", lib)
	app := e.p("Apps/X.app/Skills/x")
	e.skill("Apps/X.app/Skills/x", "x")
	b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	os.WriteFile(filepath.Join(lib, "skills.toml"), append(b, []byte("\n[[app]]\nname = \"x\"\npath = \""+app+"\"\n")...), 0o644)
	e.ok(lib, "apply")
	dir := filepath.Dir(e.configPath())
	placed := filepath.Join(dir, "placed.json")
	if err := os.Remove(placed); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	o, er, code := e.skd(lib, "disable", "x")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, o, er)
	}
	for _, tn := range []string{"agents", "claude"} {
		if _, err := os.Lstat(e.p("." + tn + "/skills/x")); err == nil {
			t.Fatalf("the link in %s is still there\n%s%s", tn, o, er)
		}
	}
	if n := strings.Count(er, "placed.json"); n != 1 || strings.Contains(o, "placed.json") {
		t.Fatalf("warned %d times on stderr\nstdout: %s\nstderr: %s", n, o, er)
	}
	if strings.Contains(o, "cannot tell") {
		t.Fatalf("not removed as skd's:\n%s", o)
	}
	if _, err := os.Stat(filepath.Join(lib, ".trash")); err == nil {
		t.Fatalf("something went to the trash\n%s", o)
	}
	if _, err := os.Lstat(placed); !os.IsNotExist(err) {
		t.Fatalf("placed.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(app, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
