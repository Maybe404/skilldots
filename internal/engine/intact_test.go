package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/targets"
)

// sshUntouched fails the test unless ~/.ssh holds exactly id_rsa, as it
// was, and nothing went to the trash.
func sshUntouched(t *testing.T, lib string, home, what string) {
	t.Helper()
	ssh := filepath.Join(home, ".ssh")
	var names []string
	des, _ := os.ReadDir(ssh)
	for _, de := range des {
		names = append(names, de.Name())
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "id_rsa" {
		t.Errorf("%s: ~/.ssh now holds %v", what, names)
	}
	if st, err := os.Lstat(filepath.Join(ssh, "id_rsa")); err != nil || !st.Mode().IsRegular() {
		t.Errorf("%s: ~/.ssh/id_rsa is no longer the file: %v", what, err)
	} else if b, _ := os.ReadFile(filepath.Join(ssh, "id_rsa")); string(b) != "SECRET" {
		t.Errorf("%s: ~/.ssh/id_rsa changed", what)
	}
	if m, _ := filepath.Glob(filepath.Join(lib, TrashDir, "*")); len(m) != 0 {
		t.Errorf("%s: trash: %v", what, m)
	}
}

// A target is written into only while it leads to the directory found
// when the registry was built, even when a link on the way appears in
// the same run: here the claude target first links skills/foo, which
// holds newdir -> ../../../.ssh, and the custom target z (approved as
// ~/.claude/skills/foo/newdir when that did not exist) would then lead
// into ~/.ssh: links there, and id_rsa moved aside.
func TestTargetThroughALinkMadeInTheSameRunIsRefused(t *testing.T) {
	for _, mode := range []string{"link", "copy"} {
		for _, force := range []bool{false, true} {
			lib, reg, home := restoreLib(t)
			lib.Root = filepath.Join(home, "lib")
			foo := filepath.Join(lib.Root, "skills", "foo")
			writeSkill(t, foo, "foo")
			if err := os.Symlink("../../../.ssh", filepath.Join(foo, "newdir")); err != nil {
				t.Fatal(err)
			}
			writeSkill(t, filepath.Join(lib.Root, "skills", "bar"), "bar")
			writeSkill(t, filepath.Join(lib.Root, "skills", "id_rsa"), "id_rsa")
			os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
			os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), []byte("SECRET"), 0o600)

			claude, err := reg.Get("claude")
			if err != nil {
				t.Fatal(err)
			}
			zp := filepath.Join(home, ".claude", "skills", "foo", "newdir")
			z := targets.Target{Name: "z", Path: zp, Mode: mode, Real: fsutil.Resolve(zp)}
			// Deeper: creating its directory would make ~/.ssh/sub.
			zp2 := filepath.Join(zp, "sub")
			z2 := targets.Target{Name: "z2", Path: zp2, Mode: mode, Real: fsutil.Resolve(zp2)}
			entry := func(name string, tt ...targets.Target) Entry {
				return Entry{Name: name, LinkName: name, Kind: KindSkill, Dir: filepath.Join(lib.Root, "skills", name), Targets: tt}
			}
			entries := []Entry{entry("foo", claude), entry("bar", z, z2), entry("id_rsa", z)}
			plan := MakePlan(lib, entries, force)
			res, err := Apply(lib, plan)
			if err != nil {
				t.Fatalf("%s force=%v: %v", mode, force, err)
			}
			if !isLinkTo(filepath.Join(claude.Path, "foo"), foo) {
				t.Errorf("%s force=%v: foo not linked for claude", mode, force)
			}
			sshUntouched(t, lib.Root, home, mode)
			if len(res.Refused) != 3 || !strings.Contains(res.Refused[0], "now leads to") {
				t.Errorf("%s force=%v: refused %q", mode, force, res.Refused)
			}
			for _, k := range []string{"z/bar", "z/id_rsa", "z2/bar"} {
				if lib.Lock.Links[k] != nil {
					t.Errorf("%s force=%v: recorded %s", mode, force, k)
				}
			}
			// A later run plans the same and is refused the same way.
			res, _ = Apply(lib, MakePlan(lib, entries, true))
			sshUntouched(t, lib.Root, home, mode+" again")
		}
	}
}

// A built-in target whose directory is replaced by a link after the
// registry was built (here before apply; in a real run, by anything
// earlier in it) gets nothing written, moved aside or removed.
func TestBuiltinTargetReplacedByALinkIsRefused(t *testing.T) {
	lib, reg, home := restoreLib(t)
	writeSkill(t, filepath.Join(lib.Root, "skills", "id_rsa"), "id_rsa")
	writeSkill(t, filepath.Join(lib.Root, "skills", "gone"), "gone")
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), []byte("SECRET"), 0o600)
	claude, _ := reg.Get("claude")
	entries := []Entry{{Name: "id_rsa", LinkName: "id_rsa", Kind: KindSkill, Dir: filepath.Join(lib.Root, "skills", "id_rsa"), Targets: []targets.Target{claude}}}
	plan := MakePlan(lib, entries, true)
	if err := os.MkdirAll(filepath.Dir(claude.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".ssh"), claude.Path); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(lib, plan)
	if err != nil {
		t.Fatal(err)
	}
	sshUntouched(t, lib.Root, home, "create")
	if len(res.Refused) != 1 {
		t.Errorf("refused %q", res.Refused)
	}
	// A recorded entry no longer wanted is not removed through it either.
	os.Symlink(filepath.Join(lib.Root, "skills", "gone"), filepath.Join(home, ".ssh", "gone"))
	lib.Lock.Links["claude/gone"] = &lock.Link{Mode: "link", Source: "skills/gone", Path: filepath.Join(claude.Path, "gone")}
	res, _ = Apply(lib, MakePlan(lib, nil, true))
	if _, err := os.Lstat(filepath.Join(home, ".ssh", "gone")); err != nil || len(res.Refused) != 1 || lib.Lock.Links["claude/gone"] == nil {
		t.Errorf("removed through the link: %v %q", err, res.Refused)
	}
}

// A trash entry from a built-in target goes back only while the target
// leads where it did: not through a link that replaced its directory.
func TestRestoreRefusesATargetReplacedByALink(t *testing.T) {
	lib, reg, home := restoreLib(t)
	claude, _ := reg.Get("claude")
	os.MkdirAll(claude.Path, 0o755)
	park(t, filepath.Join(lib.Root, TrashDir, "20260101-000000"), "x", filepath.Join(claude.Path, "x"))
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.Remove(claude.Path)
	if err := os.Symlink(filepath.Join(home, ".ssh"), claude.Path); err != nil {
		t.Fatal(err)
	}
	if dest, err := Restore(lib, reg, "20260101-000000/x"); err == nil {
		t.Fatalf("restored to %s", dest)
	}
	if fsutil.Lexists(filepath.Join(home, ".ssh", "x")) {
		t.Fatal("restored into ~/.ssh")
	}
}

// The target is checked again after its directory is made: should the
// directory be replaced by a link meanwhile (here by the stand-in for
// os.MkdirAll; in a real run, by anything racing it), nothing is
// written through it.
func TestTargetCheckedAgainAfterMakingItsDirectory(t *testing.T) {
	lib, reg, home := restoreLib(t)
	writeSkill(t, filepath.Join(lib.Root, "skills", "id_rsa"), "id_rsa")
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), []byte("SECRET"), 0o600)
	claude, _ := reg.Get("claude")
	prev := mkdirAll
	defer func() { mkdirAll = prev }()
	mkdirAll = func(p string, perm os.FileMode) error {
		if err := os.MkdirAll(filepath.Dir(p), perm); err != nil {
			return err
		}
		if p == claude.Path && !fsutil.Lexists(p) {
			return os.Symlink(filepath.Join(home, ".ssh"), p)
		}
		return os.MkdirAll(p, perm)
	}
	entries := []Entry{{Name: "id_rsa", LinkName: "id_rsa", Kind: KindSkill, Dir: filepath.Join(lib.Root, "skills", "id_rsa"), Targets: []targets.Target{claude}}}
	res, err := Apply(lib, MakePlan(lib, entries, true))
	if err != nil {
		t.Fatal(err)
	}
	sshUntouched(t, lib.Root, home, "made then replaced")
	if len(res.Refused) != 1 || lib.Lock.Links["claude/id_rsa"] != nil {
		t.Errorf("refused %q, lock %v", res.Refused, lib.Lock.LinkKeys())
	}
}
