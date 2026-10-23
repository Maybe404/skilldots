package app_test

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// isolate gives the test a home of its own, as on a new machine.
func isolate(t *testing.T) string {
	t.Helper()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	gc := filepath.Join(home, "gitconfig")
	write(t, gc, "[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755)
	return home
}

// backedUpLibrary makes a library with a skill, a private skill (each
// committed in its own repository), an update history, something in the
// trash and the skilldots skill as an [[app]] entry, applies it and backs
// it up. It returns the library and the backup.
func backedUpLibrary(t *testing.T) (root, zipPath string) {
	t.Helper()
	home := isolate(t)
	root = filepath.Join(home, ".skilldots")
	if err := app.InitLibrary(root, []string{"agents"}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "skills", "mine", "SKILL.md"), skillMD("mine"))
	write(t, filepath.Join(root, "skills", "mine", "scripts", "run.sh"), "#!/bin/sh\necho hi\n")
	os.Chmod(filepath.Join(root, "skills", "mine", "scripts", "run.sh"), 0o755)
	write(t, filepath.Join(root, "private", "secret", "SKILL.md"), skillMD("secret"))
	write(t, filepath.Join(root, "skills.history.jsonl"), `{"skill":"mine"}`+"\n")
	write(t, filepath.Join(root, ".trash", "old", "SKILL.md"), skillMD("old"))
	bundle := filepath.Join(home, "Applications", "skilldots.app", "Contents", "Resources", "skills", "skilldots")
	write(t, filepath.Join(bundle, "SKILL.md"), skillMD("skilldots"))
	m, err := manifest.Load(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	m.Apps = append(m.Apps, &manifest.App{Name: "skilldots", Path: bundle})
	if err := m.Save(filepath.Join(root, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "library")
	git(t, filepath.Join(root, "private"), "add", "-A")
	git(t, filepath.Join(root, "private"), "commit", "-q", "-m", "private secret")
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(app.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	res, err := s.BackupZip(filepath.Join(home, "backups"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(res.Path), "skilldots-backup-") || filepath.Ext(res.Path) != ".zip" {
		t.Fatalf("backup name %s", res.Path)
	}
	// Taken away from the home it was made in, as when carried to
	// another machine.
	moved := filepath.Join(t.TempDir(), filepath.Base(res.Path))
	if err := os.Rename(res.Path, moved); err != nil {
		t.Fatal(err)
	}
	return root, moved
}

// hashes is the SHA-256 of every file under root and the target of every
// link, by path, without the trash.
func hashes(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		if rel == ".trash" {
			return filepath.SkipDir
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			out[rel] = "-> " + l
		case d.Type().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(b)
			info, _ := d.Info()
			out[rel] = hex.EncodeToString(sum[:]) + " " + info.Mode().Perm().String()
		}
		return nil
	})
	return out
}

func gitLog(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "log", "--format=%s").CombinedOutput()
	if err != nil {
		t.Fatalf("git log in %s: %v\n%s", dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// A backup restored on another machine is the same library, file for file,
// with the history of both repositories, and its skills are linked.
func TestBackupRestoresOnAnotherMachine(t *testing.T) {
	root, zipPath := backedUpLibrary(t)
	want := hashes(t, root)

	z, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if strings.Contains(f.Name, ".trash") || strings.Contains(f.Name, "Applications") {
			t.Errorf("%s is in the backup", f.Name)
		}
	}
	z.Close()
	info, err := app.ReadBackup(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range info.Skills {
		names[s.Name] = true
	}
	if !names["mine"] || !names["secret"] || names["skilldots"] || info.Version != "test" {
		t.Fatalf("manifest skills %v, version %q", info.Skills, info.Version)
	}

	home := isolate(t)
	dest := filepath.Join(home, ".skilldots")
	res, err := app.RestoreBackup(zipPath, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 0 {
		t.Fatalf("skipped %v", res.Skipped)
	}
	got := hashes(t, dest)
	for p, h := range want {
		if got[p] != h {
			t.Errorf("%s: %q, want %q", p, got[p], h)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			t.Errorf("%s was not in the library", p)
		}
	}
	if l := gitLog(t, filepath.Join(dest, "private")); l != "private secret" {
		t.Fatalf("private history %q", l)
	}
	if l := gitLog(t, dest); l != "library" {
		t.Fatalf("library history %q", l)
	}
	// Uncommitted changes come along as they were.
	before, _ := exec.Command("git", "-C", root, "status", "--porcelain").CombinedOutput()
	if after, err := exec.Command("git", "-C", dest, "status", "--porcelain").CombinedOutput(); err != nil || string(after) != string(before) {
		t.Fatalf("git status after restore: %v\n%s\nwant\n%s", err, after, before)
	}
	if loc, err := library.Locate(""); err != nil || loc != dest {
		t.Fatalf("current library %q, %v", loc, err)
	}

	s, err := app.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(app.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, sk := range []string{"skills/mine", "private/secret"} {
		link := filepath.Join(home, ".agents", "skills", filepath.Base(sk))
		target, err := filepath.EvalSymlinks(link)
		if err != nil {
			t.Fatalf("%s not linked: %v", link, err)
		}
		if target != filepath.Join(dest, sk) {
			t.Fatalf("%s -> %s, want %s", link, target, filepath.Join(dest, sk))
		}
	}
}

// Restore never writes into a directory that has something in it.
func TestRestoreRefusesALibraryThatIsThere(t *testing.T) {
	_, zipPath := backedUpLibrary(t)
	home := isolate(t)
	dest := filepath.Join(home, ".skilldots")
	write(t, filepath.Join(dest, "skills.toml"), "mine\n")
	if _, err := app.RestoreBackup(zipPath, dest); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("restore over a library: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "skills.toml")); string(b) != "mine\n" {
		t.Fatalf("skills.toml changed: %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(home, ".*.restore-*")); len(left) != 0 {
		t.Fatalf("left behind %v", left)
	}
	// A file in the way counts too; an empty directory does not.
	write(t, filepath.Join(home, "file"), "x")
	if _, err := app.RestoreBackup(zipPath, filepath.Join(home, "file")); err == nil {
		t.Fatal("restored over a file")
	}
	empty := filepath.Join(home, "empty")
	os.MkdirAll(empty, 0o755)
	if _, err := app.RestoreBackup(zipPath, empty); err != nil {
		t.Fatalf("restore into an empty directory: %v", err)
	}
}

// rewrite copies the backup at src to a new file, changing what edit says.
func rewrite(t *testing.T, src string, edit func(name string, body []byte) []byte) string {
	t.Helper()
	z, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	dst := filepath.Join(t.TempDir(), "edited.zip")
	f, _ := os.Create(dst)
	w := zip.NewWriter(f)
	for _, e := range z.File {
		r, _ := e.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		if b = edit(e.Name, b); b == nil {
			continue
		}
		h := e.FileHeader
		hw, err := w.CreateHeader(&h)
		if err != nil {
			t.Fatal(err)
		}
		hw.Write(b)
	}
	w.Close()
	f.Close()
	return dst
}

// A damaged backup is refused before anything is written.
func TestRestoreRefusesADamagedBackup(t *testing.T) {
	_, zipPath := backedUpLibrary(t)
	raw, _ := os.ReadFile(zipPath)
	truncated := filepath.Join(t.TempDir(), "truncated.zip")
	os.WriteFile(truncated, raw[:len(raw)/2], 0o644)
	cases := map[string]string{
		"truncated": truncated,
		"changed file": rewrite(t, zipPath, func(n string, b []byte) []byte {
			if n == "library/skills/mine/SKILL.md" {
				return append(b, "tampered\n"...)
			}
			return b
		}),
		"missing file": rewrite(t, zipPath, func(n string, b []byte) []byte {
			if n == "library/private/secret/SKILL.md" {
				return nil
			}
			return b
		}),
		"no manifest": rewrite(t, zipPath, func(n string, b []byte) []byte {
			if n == app.BackupManifest {
				return nil
			}
			return b
		}),
	}
	// An entry the manifest does not list.
	cases["extra file"] = func() string {
		dst := filepath.Join(t.TempDir(), "extra.zip")
		z, _ := zip.OpenReader(zipPath)
		defer z.Close()
		f, _ := os.Create(dst)
		w := zip.NewWriter(f)
		for _, e := range z.File {
			w.Copy(e)
		}
		hw, _ := w.Create("library/skills/mine/extra.md")
		hw.Write([]byte("x"))
		w.Close()
		f.Close()
		return dst
	}()
	for name, p := range cases {
		home := isolate(t)
		dest := filepath.Join(home, ".skilldots")
		_, err := app.RestoreBackup(p, dest)
		if !errors.Is(err, app.ErrBadBackup) {
			t.Errorf("%s: %v", name, err)
		}
		if _, err := os.Lstat(dest); !os.IsNotExist(err) {
			t.Errorf("%s: %s was created", name, dest)
		}
		if left, _ := filepath.Glob(filepath.Join(home, ".*.restore-*")); len(left) != 0 {
			t.Errorf("%s: left behind %v", name, left)
		}
	}
}

// A backup is never written inside the library it backs up.
func TestBackupRefusesADirectoryInsideTheLibrary(t *testing.T) {
	root, _ := backedUpLibrary(t)
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BackupZip(filepath.Join(root, "skills"), "test"); err == nil {
		t.Fatal("backed up into the library")
	}
}

// Links in a backup are restored only when they stay in the library, also
// when one sits behind another link.
func TestRestoreKeepsLinksInTheLibrary(t *testing.T) {
	_, zipPath := backedUpLibrary(t)
	links := []app.BackupFile{
		{Path: "skills/inside", Type: "symlink", Mode: 0o777, Target: "mine"},
		{Path: "skills/abs", Type: "symlink", Mode: 0o777, Target: "/etc"},
		{Path: "skills/up", Type: "symlink", Mode: 0o777, Target: "../../.."},
		// skills/hop is skills/../c, one level up: a link behind it that
		// climbs as far as it seems to sit below the top would leave it.
		{Path: "skills/hop", Type: "symlink", Mode: 0o777, Target: "../c"},
		{Path: "skills/hop/out", Type: "symlink", Mode: 0o777, Target: "../.."},
	}
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "links.zip")
	f, _ := os.Create(dst)
	w := zip.NewWriter(f)
	for _, e := range z.File {
		if e.Name != app.BackupManifest {
			w.Copy(e)
			continue
		}
		r, _ := e.Open()
		var info app.BackupInfo
		if err := json.NewDecoder(r).Decode(&info); err != nil {
			t.Fatal(err)
		}
		r.Close()
		info.Files = append(info.Files, links...)
		hw, _ := w.Create(app.BackupManifest)
		json.NewEncoder(hw).Encode(info)
	}
	z.Close()
	for _, l := range links {
		h := &zip.FileHeader{Name: "library/" + l.Path}
		h.SetMode(fs.ModeSymlink | 0o777)
		hw, _ := w.CreateHeader(h)
		hw.Write([]byte(l.Target))
	}
	w.Close()
	f.Close()

	home := isolate(t)
	dest := filepath.Join(home, ".skilldots")
	os.MkdirAll(filepath.Join(home, "c"), 0o755)
	res, err := app.RestoreBackup(dst, dest)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Skipped, ","); got != "skills/abs,skills/up,skills/hop/out" {
		t.Fatalf("skipped %s", got)
	}
	if l, err := os.Readlink(filepath.Join(dest, "skills", "inside")); err != nil || l != "mine" {
		t.Fatalf("skills/inside: %q %v", l, err)
	}
	if fsutil.Lexists(filepath.Join(home, "c", "out")) {
		t.Fatal("a link was made outside the library")
	}
}
