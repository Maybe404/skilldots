package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// acmeLibrary is a sandboxed library subscribed to acme/skills (tdd and
// review selected, grill upstream only) plus an app skill linked from
// outside the library.
func acmeLibrary(t *testing.T) (o *libOps, root, home, appDir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, up := sandbox(t)
	repo := filepath.Join(up, "acme", "skills.git")
	write(t, filepath.Join(repo, "LICENSE"), "MIT License\n")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd", "Test first."))
	write(t, filepath.Join(repo, "skills", "eng", "review", "SKILL.md"), skillMD("review", "Review code."))
	write(t, filepath.Join(repo, "skills", "prod", "grill", "SKILL.md"), skillMD("grill", "Grill me."))
	run(t, repo, "init", "-q")
	run(t, repo, "add", "-A")
	run(t, repo, "commit", "-q", "-m", "init")
	root = newLibrary(t, home)
	appDir = filepath.Join(home, "Applications", "Surge.app", "skills", "surge")
	write(t, filepath.Join(appDir, "SKILL.md"), skillMD("surge", "Control Surge."))
	m, err := manifest.Load(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	m.Apps = append(m.Apps, &manifest.App{Name: "surge", Path: appDir})
	if err := m.Save(filepath.Join(root, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
	o = &libOps{root: root}
	if _, err := o.Add("acme/skills", AddRequest{Names: []string{"tdd", "review"}}); err != nil {
		t.Fatal(err)
	}
	return o, root, home, appDir
}

func loadManifest(t *testing.T, root string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestOpsTrashAppAndOwnSkillOnlyUnlink(t *testing.T) {
	o, root, home, appDir := acmeLibrary(t)
	write(t, filepath.Join(root, "skills", "mine", "SKILL.md"), skillMD("mine", "My own."))
	write(t, filepath.Join(root, "private", "secret", "SKILL.md"), skillMD("secret", "Private."))
	if _, err := o.Apply(); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(home, ".agents", "skills")
	for _, n := range []string{"surge", "mine", "secret"} {
		if !isLink(filepath.Join(agents, n)) {
			t.Fatalf("%s not linked before trash", n)
		}
	}
	s, _ := o.Load()
	if len(s.Apps) != 1 || len(s.Mine) != 1 || len(s.Private) != 1 {
		t.Fatalf("snapshot: apps %d mine %d private %d", len(s.Apps), len(s.Mine), len(s.Private))
	}
	for _, sk := range []Skill{s.Apps[0], s.Mine[0], s.Private[0]} {
		msg, err := o.Trash(sk)
		if err != nil {
			t.Fatalf("trash %s: %v", sk.Name, err)
		}
		if !strings.Contains(msg, "unlinked "+sk.LinkName) || !strings.Contains(msg, "files stay") {
			t.Errorf("trash %s: %q", sk.Name, msg)
		}
		if isLink(filepath.Join(agents, sk.LinkName)) {
			t.Errorf("%s still linked", sk.Name)
		}
		if trashed(t, root, sk.Name) {
			t.Errorf("%s went to the trash; only vendored skills may", sk.Name)
		}
	}
	for _, dir := range []string{appDir, filepath.Join(root, "skills", "mine"), filepath.Join(root, "private", "secret")} {
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			t.Errorf("files gone from %s: %v", dir, err)
		}
	}
	m := loadManifest(t, root)
	if len(m.Apps) != 1 || m.Apps[0].Targets == nil || len(*m.Apps[0].Targets) != 0 {
		t.Errorf("app should stay in skills.toml with no targets: %+v", m.Apps[0])
	}
	for _, name := range []string{"mine", "secret"} {
		found := false
		for _, l := range m.Locals {
			if l.Name == name && l.Targets != nil && len(*l.Targets) == 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("no [[local]] %s with targets = []", name)
		}
	}
}

func TestOpsForkApp(t *testing.T) {
	o, root, home, appDir := acmeLibrary(t)
	s, _ := o.Load()
	if len(s.Apps) != 1 {
		t.Fatalf("apps: %+v", s.Apps)
	}
	msg, err := o.Fork(s.Apps[0], false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "forked surge → skills/surge") {
		t.Errorf("message: %q", msg)
	}
	fork := filepath.Join(root, "skills", "surge", "SKILL.md")
	b, err := os.ReadFile(fork)
	if err != nil || !strings.Contains(string(b), "forked by skilldots from") {
		t.Fatalf("fork: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(appDir, "SKILL.md")); err != nil {
		t.Fatal("the app's own files must stay")
	}
	if m := loadManifest(t, root); len(m.Apps) != 0 {
		t.Errorf("app entry kept after fork: %+v", m.Apps)
	}
	s, _ = o.Load()
	if len(s.Apps) != 0 || len(s.Mine) != 1 || s.Mine[0].Name != "surge" || !s.Mine[0].Enabled() {
		t.Fatalf("after fork: apps %+v mine %+v", s.Apps, s.Mine)
	}
	target, err := os.Readlink(filepath.Join(home, ".agents", "skills", "surge"))
	if err != nil || !strings.HasPrefix(target, filepath.Join(root, "skills")) {
		t.Errorf("link should point at the fork: %q %v", target, err)
	}
	// Forking again: the fork is yours now.
	if _, err := o.Fork(s.Mine[0], false, ""); err == nil || !strings.Contains(err.Error(), "already yours") {
		t.Errorf("fork of your own skill: %v", err)
	}
}

func TestOpsSetTargetsOnPack(t *testing.T) {
	o, root, home, _ := acmeLibrary(t)
	agents := filepath.Join(home, ".agents", "skills")
	claude := filepath.Join(home, ".claude", "skills")
	msg, err := o.SetTargets(nil, "acme/skills", []string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "acme/skills → claude") {
		t.Errorf("message: %q", msg)
	}
	for _, n := range []string{"tdd", "review"} {
		if !isLink(filepath.Join(claude, n)) || isLink(filepath.Join(agents, n)) {
			t.Errorf("%s: want linked into claude only", n)
		}
	}
	p := loadManifest(t, root).FindPack("acme/skills")
	if p.Targets == nil || strings.Join(*p.Targets, ",") != "claude" {
		t.Errorf("pack targets: %v", p.Targets)
	}
	s, _ := o.Load()
	if got := findPack(s, "acme/skills").Targets; strings.Join(got, ",") != "claude" {
		t.Errorf("snapshot targets: %v", got)
	}
	// Empty: keep in library, link nowhere; the vendored files stay.
	if _, err := o.SetTargets(nil, "acme/skills", []string{}); err != nil {
		t.Fatal(err)
	}
	if isLink(filepath.Join(claude, "tdd")) || !dirExists(filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd")) {
		t.Error("targets = [] must unlink and keep the files")
	}
	if _, err := o.SetTargets(nil, "nobody/nothing", []string{"claude"}); err == nil {
		t.Error("unknown pack accepted")
	}
}

func TestOpsTrashLastSkillPurgesPack(t *testing.T) {
	o, root, home, _ := acmeLibrary(t)
	s, _ := o.Load()
	p := findPack(s, "acme/skills")
	if _, err := o.Trash(*findSkill(p, "review")); err != nil {
		t.Fatal(err)
	}
	if loadManifest(t, root).FindPack("acme/skills") == nil {
		t.Fatal("pack purged while tdd is still selected")
	}
	s, _ = o.Load()
	msg, err := o.Trash(*findSkill(findPack(s, "acme/skills"), "tdd"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "had no skills left") || !strings.Contains(msg, "purged acme/skills") {
		t.Errorf("message: %q", msg)
	}
	if loadManifest(t, root).FindPack("acme/skills") != nil {
		t.Error("pack still in skills.toml")
	}
	lib, err := library.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Lock.Packs["acme/skills"] != nil {
		t.Error("pack still in skills.lock")
	}
	if dirExists(filepath.Join(root, "vendor", "acme", "skills")) || isLink(filepath.Join(home, ".agents", "skills", "tdd")) {
		t.Error("vendor dir or link left behind")
	}
	if !trashedFrom(t, root, filepath.Join("vendor", "acme", "skills"), filepath.Join("eng", "tdd", "SKILL.md")) {
		t.Error("vendor/acme/skills (with tdd) not in the trash")
	}
}

// trashedFrom reports whether the trash holds a directory that came from
// origin (a path ending) and still contains file.
func trashedFrom(t *testing.T, root, origin, file string) bool {
	t.Helper()
	items, err := (&libOps{root: root}).TrashList()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if strings.HasSuffix(it.Origin, origin) {
			if _, err := os.Stat(filepath.Join(it.Path, file)); err == nil {
				return true
			}
		}
	}
	t.Logf("trash: %+v", items)
	return false
}

func TestOpsSync(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, _ := sandbox(t)
	root := newLibrary(t, home)
	o := &libOps{root: root}

	// No remote: committed, and a warning saying how to add one.
	msg, err := o.Sync()
	var w warning
	if !errors.As(err, &w) || !strings.HasPrefix(err.Error(), "no remote configured") || !strings.Contains(err.Error(), "committed") || msg != "" {
		t.Fatalf("no remote: msg %q err %v", msg, err)
	}
	// With a remote: pushed.
	bare := filepath.Join(home, "backup.git")
	run(t, home, "init", "-q", "--bare", bare)
	run(t, root, "remote", "add", "origin", bare)
	write(t, filepath.Join(root, "skills", "x", "SKILL.md"), skillMD("x", "X."))
	msg, err = o.Sync()
	if err != nil || !strings.Contains(msg, "committed") || !strings.Contains(msg, "pushed") {
		t.Fatalf("with remote: msg %q err %v", msg, err)
	}
	// Nothing new: nothing to commit, still pushed.
	if msg, err = o.Sync(); err != nil || !strings.Contains(msg, "nothing to commit") {
		t.Fatalf("again: msg %q err %v", msg, err)
	}
	// A push that fails is an error.
	run(t, root, "remote", "set-url", "origin", filepath.Join(home, "nowhere.git"))
	if _, err = o.Sync(); err == nil || errors.As(err, &w) {
		t.Fatalf("failed push: %v", err)
	}
}
