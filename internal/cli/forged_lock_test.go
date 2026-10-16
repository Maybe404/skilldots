package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/fsutil"
)

// keptOrTrashed fails the test unless path was moved into the library
// trash, recorded as coming from path, and is the same there (same link
// target, or same file contents, checked by same), or is still in place
// as it was. Either way nothing was deleted for good.
func keptOrTrashed(t *testing.T, lib, path string, same func(p string) bool, what string) (trashed string) {
	t.Helper()
	records, _ := filepath.Glob(filepath.Join(lib, ".trash", "*", "*.origin"))
	for _, r := range records {
		b, err := os.ReadFile(r)
		if err != nil || strings.TrimSpace(string(b)) != path {
			continue
		}
		p := strings.TrimSuffix(r, ".origin")
		if !same(p) {
			t.Errorf("%s: %s is in the trash at %s, but not as it was", what, path, p)
		}
		return p
	}
	if _, err := os.Lstat(path); err != nil {
		t.Errorf("%s: %s was deleted: neither in place nor in the trash", what, path)
	} else if !same(path) {
		t.Errorf("%s: %s was changed in place", what, path)
	}
	return ""
}

// A forged skills.lock (someone else's library, run by an agent with no
// terminal) never makes skd delete anything for good: a copy record whose
// hash matches the user's own directory, a link record whose source
// leaves the library (to the user's own link's target, or not), an
// absolute one, or one under a root that looks like a previous library
// location. Without --force and with it, every
// one of the user's entries is either still there or in the trash; a
// real directory moved aside can be restored.
func TestForgedLockNeverDeletes(t *testing.T) {
	for _, force := range []bool{false, true} {
		for _, wanted := range []bool{false, true} {
			name := "orphan"
			if wanted {
				name = "wanted"
			}
			if force {
				name += " --force"
			}
			t.Run(name, func(t *testing.T) {
				e := newEnv(t)
				lib := e.p("lib")
				e.write("lib/skills.toml", "[targets]\ndefault = [\"claude\"]\n")
				// The user's own entries in a built-in agent directory.
				e.write(".claude/skills/mine/SKILL.md", "---\nname: mine\ndescription: mine\n---\n")
				e.write(".claude/skills/mine/notes.txt", "precious")
				e.write(".claude/skills/dot/SKILL.md", "---\nname: dot\ndescription: dot\n---\n")
				e.write(".claude/skills/dot/notes.txt", "precious too")
				for _, l := range []struct{ name, to string }{
					{"mine3", "dot"}, {"mine4", e.p(".claude/skills/dot")}, {"mine5", "dot"}, {"mine6", "dot"},
				} {
					if err := os.Symlink(l.to, e.p(".claude/skills/"+l.name)); err != nil {
						t.Fatal(err)
					}
				}
				if wanted {
					// The library also has skills by those names, so the
					// records are of entries it wants.
					for _, n := range []string{"mine", "mine3", "mine4", "mine5", "mine6"} {
						e.skill("lib/skills/"+n, n)
					}
				}
				h, err := fsutil.HashTree(e.p(".claude/skills/mine"))
				if err != nil {
					t.Fatal(err)
				}
				e.write("lib/skills.lock", `version = 1

[links.'claude/mine']
mode = 'copy'
source = 'skills/mine'
hash = '`+h+`'
path = '`+e.p(".claude/skills/mine")+`'

[links.'claude/mine3']
mode = 'link'
source = '../dot'
path = '`+e.p(".claude/skills/mine3")+`'

[links.'claude/mine4']
mode = 'link'
source = '`+e.p(".claude/skills/dot")+`'
path = '`+e.p(".claude/skills/mine4")+`'

[links.'claude/mine5']
mode = 'link'
source = 'dot'
path = '`+e.p(".claude/skills/mine5")+`'

[links.'claude/mine6']
mode = 'link'
source = '../.claude/skills/dot'
path = '`+e.p(".claude/skills/mine6")+`'
`)
				args := []string{"apply"}
				if force {
					args = append(args, "--force")
				}
				out, errOut, _ := e.skd(lib, args...)
				log := out + errOut
				mine := keptOrTrashed(t, lib, e.p(".claude/skills/mine"), func(p string) bool {
					b, err := os.ReadFile(filepath.Join(p, "notes.txt"))
					return err == nil && string(b) == "precious"
				}, log)
				for _, l := range []struct{ name, to string }{
					{"mine3", "dot"}, {"mine4", e.p(".claude/skills/dot")}, {"mine5", "dot"}, {"mine6", "dot"},
				} {
					keptOrTrashed(t, lib, e.p(".claude/skills/"+l.name), func(p string) bool {
						got, err := os.Readlink(p)
						return err == nil && got == l.to
					}, log)
				}
				if b, err := os.ReadFile(e.p(".claude/skills/dot/notes.txt")); err != nil || string(b) != "precious too" {
					t.Errorf("~/.claude/skills/dot was taken: %v\n%s", err, log)
				}
				// A directory moved aside comes back with skd trash restore,
				// once the place is free again.
				if mine != "" {
					id := filepath.Base(filepath.Dir(mine)) + "/" + filepath.Base(mine)
					if fsutil.Lexists(e.p(".claude/skills/mine")) {
						if err := os.Rename(e.p(".claude/skills/mine"), e.p("elsewhere")); err != nil {
							t.Fatal(err)
						}
					}
					e.ok(lib, "trash", "restore", id)
					if b, err := os.ReadFile(e.p(".claude/skills/mine/notes.txt")); err != nil || string(b) != "precious" {
						t.Errorf("restore: %v", err)
					}
				}
			})
		}
	}
}

// A library like most people's, built-in targets and links only, behaves
// as before: links made by an older skd (no record of what it placed)
// are kept, disabling removes them outright, and after the library moves
// they are re-pointed. Nothing lands in the trash.
func TestOwnLinksBehaveAsBefore(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.write("lib/skills.toml", "[targets]\ndefault = [\"agents\", \"claude\"]\n")
	for _, n := range []string{"a", "b", "c"} {
		e.skill("lib/skills/"+n, n)
	}
	e.ok(lib, "apply")
	placed := filepath.Join(filepath.Dir(e.configPath()), "placed.json")
	if !fsutil.Lexists(placed) {
		t.Fatal("no record of the links placed")
	}
	trash := func(what string) {
		t.Helper()
		if m, _ := filepath.Glob(filepath.Join(e.p("*"), ".trash", "*", "*")); len(m) != 0 {
			t.Fatalf("%s: trashed %v", what, m)
		}
	}
	isLinkTo := func(name, want string) {
		t.Helper()
		for _, d := range []string{".agents/skills", ".claude/skills"} {
			got, err := os.Readlink(e.p(d + "/" + name))
			if err != nil || got != want {
				t.Fatalf("%s/%s: %q %v, want a link to %s", d, name, got, err, want)
			}
		}
	}
	// Links made by an older skd, which kept no record.
	os.Remove(placed)
	if out := e.ok(lib, "apply"); !strings.Contains(out, "up to date, 6 linked") {
		t.Fatalf("apply: %s", out)
	}
	e.ok(lib, "disable", "a")
	for _, d := range []string{".agents/skills/a", ".claude/skills/a"} {
		if fsutil.Lexists(e.p(d)) {
			t.Fatalf("%s still there", d)
		}
	}
	trash("disable")
	// The library moves.
	if err := os.Rename(lib, e.p("lib2")); err != nil {
		t.Fatal(err)
	}
	e.ok("", "use", e.p("lib2"))
	out := e.ok(e.p("lib2"), "apply")
	if !strings.Contains(out, "previous library location") {
		t.Fatalf("apply after the move: %s", out)
	}
	isLinkTo("b", e.p("lib2/skills/b"))
	isLinkTo("c", e.p("lib2/skills/c"))
	trash("move")
	e.ok(e.p("lib2"), "disable", "b")
	trash("disable after the move")
}

// Links skd made in a custom target that is then removed from skills.toml
// stay where they are (skd no longer manages that directory), and skd
// keeps saying so: every apply names them and `skd scan` lists them as
// left, until they are removed. A forged record naming something that
// is not such a link is not listed.
func TestLinksOfARemovedTargetAreListedUntilGone(t *testing.T) {
	e := newEnv(t)
	lib := customLib(e, "[targets.custom.box]\npath = \"~/box/skills\"\n")
	os.MkdirAll(e.p("box/skills"), 0o755)
	rel, _ := filepath.Rel(e.home, e.approvals())
	e.write(filepath.ToSlash(rel), "[[target]]\nname = 'box'\npath = '"+e.p("box/skills")+"'\nmode = 'link'\napproved = 2026-01-01T00:00:00Z\n")
	e.write("lib/skills.toml", "[targets]\ndefault = [\"agents\", \"box\"]\n\n[targets.custom.box]\npath = \"~/box/skills\"\n")
	e.ok(lib, "apply")
	left := e.p("box/skills/tdd")
	if _, err := os.Readlink(left); err != nil {
		t.Fatalf("not linked into box: %v", err)
	}
	if js := e.ok(lib, "scan", "--json"); strings.Contains(js, `"left"`) {
		t.Fatalf("links of targets in use listed as left:\n%s", js)
	}
	// The target goes; a forged record names the user's own directory.
	e.write("lib/skills.toml", "[targets]\ndefault = [\"agents\"]\n")
	e.write("Documents/x/notes.txt", "mine")
	lockb, _ := os.ReadFile(filepath.Join(lib, "skills.lock"))
	os.WriteFile(filepath.Join(lib, "skills.lock"), append(lockb, []byte("\n[links.'gone/x']\nmode = 'link'\nsource = 'skills/tdd'\npath = '"+e.p("Documents/x")+"'\n")...), 0o644)
	for i := 0; i < 2; i++ {
		out := e.ok(lib, "apply")
		if !strings.Contains(out, left) || !strings.Contains(out, "no longer manages it") {
			t.Fatalf("apply %d does not name the left link:\n%s", i, out)
		}
		if _, err := os.Readlink(left); err != nil {
			t.Fatalf("the left link was removed: %v", err)
		}
	}
	scan := e.ok(lib, "scan")
	if !strings.Contains(scan, "left") || !strings.Contains(scan, "tdd") || strings.Contains(scan, "Documents") {
		t.Fatalf("scan:\n%s", scan)
	}
	js := e.ok(lib, "scan", "--json")
	if !strings.Contains(js, `"state": "left"`) || !strings.Contains(js, left) {
		t.Fatalf("scan --json:\n%s", js)
	}
	if b, err := os.ReadFile(e.p("Documents/x/notes.txt")); err != nil || string(b) != "mine" {
		t.Fatalf("~/Documents/x touched: %v", err)
	}
	// Removed by the user: no longer said.
	os.Remove(left)
	e.ok(lib, "apply")
	if scan := e.ok(lib, "scan", "--json"); strings.Contains(scan, `"left"`) {
		t.Fatalf("still listed:\n%s", scan)
	}
}

// A cloned library's own files may be links to the user's files: skd
// reads through them but never writes, so nothing there is overwritten
// (skills.lock -> ../.codex/config.toml would otherwise be replaced by
// the lock), and nothing is linked before it says so.
func TestLibraryFilesLinkedOutsideAreNotWritten(t *testing.T) {
	for _, f := range []string{"skills.lock", "skills.toml"} {
		t.Run(f, func(t *testing.T) {
			e := newEnv(t)
			e.write(".codex/config.toml", "model = \"o3\"\n\n[targets]\ndefault = [\"agents\"]\n")
			e.write("lib/skills.toml", "[targets]\ndefault = [\"agents\"]\n")
			e.skill("lib/skills/a", "a")
			os.Remove(e.p("lib/" + f))
			if err := os.Symlink("../.codex/config.toml", e.p("lib/"+f)); err != nil {
				t.Fatal(err)
			}
			out, errOut, code := e.skd(e.p("lib"), "apply")
			if code == 0 || !strings.Contains(errOut, "outside the library") {
				t.Fatalf("apply: exit %d\n%s%s", code, out, errOut)
			}
			if b, _ := os.ReadFile(e.p(".codex/config.toml")); string(b) != "model = \"o3\"\n\n[targets]\ndefault = [\"agents\"]\n" {
				t.Fatalf("overwritten:\n%s", b)
			}
			if fsutil.Lexists(e.p(".agents/skills/a")) {
				t.Fatal("linked before refusing")
			}
		})
	}
}

// `skd init --from` a repository whose skills.lock or .gitignore is a
// link to one of the user's files refuses before writing anything.
func TestInitDoesNotWriteThroughLinks(t *testing.T) {
	for _, f := range []string{"skills.lock", ".gitignore"} {
		t.Run(f, func(t *testing.T) {
			e := newEnv(t)
			e.write(".codex/config.toml", "model = \"o3\"\n")
			e.write("repo/README.md", "hi")
			if err := os.Symlink("../.codex/config.toml", e.p("repo/"+f)); err != nil {
				t.Fatal(err)
			}
			_, errOut, code := e.skd("", "init", "--from", e.p("repo"))
			if code == 0 || !strings.Contains(errOut, "outside the library") {
				t.Fatalf("init: exit %d\n%s", code, errOut)
			}
			if b, _ := os.ReadFile(e.p(".codex/config.toml")); string(b) != "model = \"o3\"\n" {
				t.Fatalf("overwritten:\n%s", b)
			}
		})
	}
}
