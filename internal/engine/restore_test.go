package engine

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/safetext"
	"github.com/Maybe404/skilldots/internal/targets"
)

// park writes a trash entry by hand, as someone else's library could
// hold it: dir/name with a SKILL.md, and name.origin recording origin.
func park(t *testing.T, dir, name, origin string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	writeSkill(t, p, "s")
	if err := os.WriteFile(p+".origin", []byte(origin+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func realTemp(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// restoreLib is a library and a home whose paths have no symbolic links
// in them, so the places the tests compare are the places on disk.
func restoreLib(t *testing.T) (*library.Library, *targets.Registry, string) {
	t.Helper()
	lib, _, _ := newTestLib(t)
	lib.Root = realTemp(t)
	home := realTemp(t)
	t.Setenv("HOME", home)
	reg, err := targets.NewRegistry("", nil)
	if err != nil {
		t.Fatal(err)
	}
	SetRegistry(reg)
	return lib, reg, home
}

// An id names an entry exactly two levels down in the library's trash:
// "..", absolute paths, more or fewer parts, and links (the trash, a
// batch or an entry) reaching elsewhere are refused, and nothing moves.
func TestRestoreRefusesIDsOutsideTheTrash(t *testing.T) {
	lib, reg, home := restoreLib(t)
	agents := filepath.Join(home, ".agents", "skills")
	trash := filepath.Join(lib.Root, TrashDir)
	// A directory outside the library with a record next to it that
	// names a place restore would otherwise accept.
	victim := filepath.Join(home, "victim")
	dirA := park(t, victim, "dirA", filepath.Join(agents, "dirA"))
	os.MkdirAll(filepath.Join(trash, "b1"), 0o755)
	up, _ := filepath.Rel(trash, dirA)
	if !strings.HasPrefix(up, "..") {
		t.Fatalf("victim inside the trash: %s", up)
	}
	// A batch that is a link to the victim's directory, an entry that is
	// a link to the victim, and a trash that is a link.
	os.Symlink(victim, filepath.Join(trash, "b2"))
	os.MkdirAll(filepath.Join(trash, "b3"), 0o755)
	os.Symlink(dirA, filepath.Join(trash, "b3", "dirA"))
	os.WriteFile(filepath.Join(trash, "b3", "dirA.origin"), []byte(filepath.Join(agents, "dirA")+"\n"), 0o644)
	for _, id := range []string{
		filepath.ToSlash(up), "../victim/dirA", "b1/../../victim/dirA", "b1/..", "../b1",
		dirA, "/b1/dirA", "b1/dirA/x", "b1", "dirA", "b1/", "/", "", "b1//dirA", "./b1/dirA", "b1/.",
		"b2/dirA", "b3/dirA", `b1\..\..\victim\dirA`, "b1/dirA\x00",
	} {
		if dest, err := Restore(lib, reg, id); err == nil {
			t.Errorf("Restore(%q) moved it to %s", id, dest)
		} else if safetext.Has(err.Error()) {
			t.Errorf("Restore(%q): the error has control characters: %q", id, err)
		}
		if !dirExists(dirA) || fileExists(filepath.Join(agents, "dirA")) {
			t.Fatalf("Restore(%q) moved the victim", id)
		}
	}
	// The listing does not follow the links either.
	items, err := ListTrash(lib, reg)
	if err != nil || len(items) != 0 {
		t.Errorf("trash lists %+v %v", items, err)
	}

	// The trash itself a link: refused, listed as an error.
	os.RemoveAll(trash)
	os.Symlink(victim, trash)
	if _, err := Restore(lib, reg, "dirA"); err == nil {
		t.Error("restored through a linked trash")
	}
	os.Remove(trash)
	os.MkdirAll(filepath.Join(lib.Root, "elsewhere"), 0o755)
	os.Symlink(home, trash) // home/victim/dirA would be the entry victim/dirA
	if _, err := Restore(lib, reg, "victim/dirA"); err == nil || !dirExists(dirA) {
		t.Errorf("restored through a linked trash: %v", err)
	}
	if _, err := ListTrash(lib, reg); err == nil {
		t.Error("a linked trash is listed")
	}
}

// Where an entry goes back to must be where skd takes directories from:
// an entry of a target's directory, or inside the library's vendor/. An
// absolute, clean path without control characters. Anything else is
// listed as not restorable, with a reason, and refused.
func TestRestoreRefusesOriginsSkdNeverWrites(t *testing.T) {
	lib, reg, home := restoreLib(t)
	agents := filepath.Join(home, ".agents", "skills")
	outside := filepath.Join(filepath.Dir(home), "not-home-"+filepath.Base(home), "x")
	os.MkdirAll(filepath.Join(home, "Library"), 0o755)
	// vendor/o is a link to ~/Library: vendor/o/x is not inside vendor/.
	os.MkdirAll(lib.VendorDir(), 0o755)
	os.Symlink(filepath.Join(home, "Library"), filepath.Join(lib.VendorDir(), "o"))
	bad := map[string]string{
		"outside home":         outside,
		"home, not a target":   filepath.Join(home, "Library", "x", "target"),
		"a target itself":      agents,
		"below a target entry": filepath.Join(agents, "a", "b"),
		"relative":             filepath.Join(".agents", "skills", "rel"),
		"dot dot":              agents + "/../../Library/dd", // not filepath.Join: it cleans
		"dot dot, inside":      agents + "/../skills/dd",
		"dot":                  agents + "/./dot",
		"trailing separator":   agents + "/ts/",
		"doubled separator":    agents + "//ds",
		"control characters":   filepath.Join(agents, "e\x1b]0;x\x07"),
		"library skills/":      filepath.Join(lib.Root, "skills", "mine"),
		"library private/":     filepath.Join(lib.Root, "private", "mine"),
		"library root":         filepath.Join(lib.Root, "skills.toml"),
		"the trash":            filepath.Join(lib.Root, TrashDir, "x"),
		"vendor itself":        lib.VendorDir(),
		"vendor link out":      filepath.Join(lib.VendorDir(), "o", "x"),
		"root":                 "/",
		"empty":                "",
	}
	batch := filepath.Join(lib.Root, TrashDir, "20240102-030405")
	ids := map[string]string{}
	for what, origin := range bad {
		name := strings.NewReplacer(" ", "-", ",", "", "/", "").Replace(what)
		park(t, batch, name, origin)
		ids["20240102-030405/"+name] = what
	}
	// A record that is a link (to a file that would never end), and none.
	p := filepath.Join(batch, "zero")
	writeSkill(t, p, "s")
	os.Symlink("/dev/zero", p+".origin")
	ids["20240102-030405/zero"] = "record is a link"
	p = filepath.Join(batch, "fifo")
	writeSkill(t, p, "s")
	if err := syscall.Mkfifo(p+".origin", 0o644); err == nil {
		ids["20240102-030405/fifo"] = "record is a FIFO"
	} else {
		os.RemoveAll(p)
	}
	writeSkill(t, filepath.Join(batch, "none"), "s")
	ids["20240102-030405/none"] = "no record"

	items, err := ListTrash(lib, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(ids) {
		t.Fatalf("%d listed, want %d: %+v", len(items), len(ids), items)
	}
	// The reason names what is wrong.
	reason := map[string]string{"relative": "not an absolute path", "dot dot": "not a clean path", "doubled separator": "not a clean path",
		"control characters": "control characters", "home, not a target": "neither", "record is a link": "not a file skd wrote",
		"record is a FIFO": "not a file skd wrote", "no record": "not recorded"}
	for _, it := range items {
		if it.Restorable || it.Problem == "" || safetext.Has(it.Problem) || !strings.Contains(it.Problem, reason[ids[it.ID]]) {
			t.Errorf("%s (%s): restorable %v, problem %q", it.ID, ids[it.ID], it.Restorable, it.Problem)
		}
	}
	for id, what := range ids {
		dest, err := Restore(lib, reg, id)
		if err == nil {
			t.Errorf("%s: restored to %q", what, dest)
			continue
		}
		if safetext.Has(err.Error()) || !strings.Contains(err.Error(), "cannot be restored") {
			t.Errorf("%s: %q", what, err)
		}
		if !dirExists(filepath.Join(lib.Root, TrashDir, filepath.FromSlash(id))) {
			t.Errorf("%s: the entry moved", what)
		}
	}
	for _, p := range []string{outside, filepath.Join(home, "Library", "x"), filepath.Join(home, "Library", "dd"), filepath.Join(home, "Library", "x", "target"),
		filepath.Join(lib.Root, "skills"), filepath.Join(lib.Root, "private"), filepath.Join(agents, "a")} {
		if fileExists(p) {
			t.Errorf("%s was created", p)
		}
	}
}

// vendor/ itself a link: nothing is "inside vendor/".
func TestRestoreRefusesALinkedVendor(t *testing.T) {
	lib, reg, home := restoreLib(t)
	elsewhere := filepath.Join(home, "Library", "x")
	os.MkdirAll(elsewhere, 0o755)
	os.Symlink(elsewhere, lib.VendorDir())
	park(t, filepath.Join(lib.Root, TrashDir, "b"), "s", filepath.Join(lib.VendorDir(), "o", "r", "s"))
	if dest, err := Restore(lib, reg, "b/s"); err == nil {
		t.Errorf("restored to %s", dest)
	}
	if fileExists(filepath.Join(elsewhere, "o")) {
		t.Error("restored through the link")
	}
}

// What skd parks comes back: a target's entry (also a custom target, and
// one reached through a link to the target), a vendored skill and a
// whole pack.
func TestRestoreAcceptsWhatSkdParks(t *testing.T) {
	lib, _, home := restoreLib(t)
	custom := filepath.Join(home, "box")
	reg := customRegistry(t, map[string]struct{ Path, Mode string }{"box": {Path: custom, Mode: "copy"}})
	SetRegistry(reg)
	agents := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(agents, 0o755)
	os.Symlink(agents, filepath.Join(home, "agents-link"))
	origins := []string{
		filepath.Join(agents, "a"),
		filepath.Join(custom, "c"), // the target directory need not exist
		filepath.Join(home, "agents-link", "l"),
		filepath.Join(lib.VendorDir(), "o", "r", "eng", "s"),
		filepath.Join(lib.VendorDir(), "o", "p"),
	}
	for _, o := range origins {
		writeSkill(t, o, filepath.Base(o))
		if _, err := Trash(lib, o); err != nil {
			t.Fatal(err)
		}
	}
	items, err := ListTrash(lib, reg)
	if err != nil || len(items) != len(origins) {
		t.Fatalf("%+v %v", items, err)
	}
	for _, it := range items {
		if !it.Restorable {
			t.Errorf("%s from %s: %s", it.ID, it.Origin, it.Problem)
			continue
		}
		dest, err := Restore(lib, reg, it.ID)
		if err != nil || dest != it.Origin || !fileExists(filepath.Join(dest, "SKILL.md")) {
			t.Errorf("restore %s: %q %v", it.ID, dest, err)
		}
	}
	// A target named by a relative path is recorded absolute.
	t.Chdir(home)
	rel := filepath.Join(".agents", "skills", "r")
	writeSkill(t, rel, "r")
	if _, err := Trash(lib, rel); err != nil {
		t.Fatal(err)
	}
	items, _ = ListTrash(lib, reg)
	if len(items) != 1 || items[0].Origin != filepath.Join(agents, "r") || !items[0].Restorable {
		t.Fatalf("%+v", items)
	}
}

func dirExists(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.IsDir()
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// The library named by another spelling of its path (another case, or
// another Unicode normalisation, where the file system ignores them, as
// macOS does: skd --library ~/Code/lib after ~/code/lib) still has its
// vendor/ entries restorable; places outside vendor/ are still refused.
func TestRestoreVendorThroughAnotherSpelling(t *testing.T) {
	lib, reg, home := restoreLib(t)
	base := realTemp(t)
	for _, c := range []struct{ made, used string }{
		{"Lib", "lib"},
		{"caf\u00e9", "cafe\u0301"}, // NFC, NFD
	} {
		root := filepath.Join(base, c.made)
		os.MkdirAll(root, 0o755)
		if _, err := os.Stat(filepath.Join(base, c.used)); err != nil {
			t.Logf("%q and %q are different directories here", c.made, c.used)
			continue
		}
		lib.Root = root
		origins := map[string]bool{
			filepath.Join(root, "vendor", "o", "r", "s"): true,
			filepath.Join(root, "vendor", "o", "p"):      true,
			filepath.Join(root, "skills", "x"):           false,
			filepath.Join(root, "vendorx", "y"):          false,
			filepath.Join(home, "elsewhere", "z"):        false,
		}
		for o := range origins {
			writeSkill(t, o, filepath.Base(o))
			if _, err := Trash(lib, o); err != nil {
				t.Fatal(err)
			}
		}
		// vendor/ itself is not a place skd parks anything from.
		park(t, filepath.Join(root, TrashDir, "b9"), "vendor", filepath.Join(root, "vendor"))
		origins[filepath.Join(root, "vendor")] = false
		// Now the library is named the other way.
		lib.Root = filepath.Join(base, c.used)
		items, err := ListTrash(lib, reg)
		if err != nil || len(items) != len(origins) {
			t.Fatalf("%+v %v", items, err)
		}
		for _, it := range items {
			want := origins[it.Origin]
			if it.Restorable != want {
				t.Errorf("%s from %s: restorable %v (%s)", c.used, it.Origin, it.Restorable, it.Problem)
			}
			if _, err := Restore(lib, reg, it.ID); (err == nil) != want {
				t.Errorf("%s: restore %s: %v", c.used, it.ID, err)
			}
		}
		os.RemoveAll(filepath.Join(root, TrashDir))
	}
}

// An agent's directory named by another spelling of its path, where the
// file system ignores case, is still that agent's directory.
func TestRestoreTargetThroughAnotherSpelling(t *testing.T) {
	lib, reg, home := restoreLib(t)
	agents := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(agents, 0o755)
	other := filepath.Join(home, ".AGENTS", "Skills")
	if _, err := os.Stat(other); err != nil {
		t.Skip("this file system tells case apart")
	}
	p := park(t, filepath.Join(lib.Root, TrashDir, "b1"), "a", filepath.Join(other, "a"))
	items, err := ListTrash(lib, reg)
	if err != nil || len(items) != 1 || !items[0].Restorable {
		t.Fatalf("%+v %v", items, err)
	}
	if _, err := Restore(lib, reg, "b1/a"); err != nil || fileExists(p) || !fileExists(filepath.Join(agents, "a", "SKILL.md")) {
		t.Fatalf("restore: %v", err)
	}
}

// A record longer than skd ever writes (16 KiB) is not read, even when
// it is a plain file naming a place restore would accept; one of exactly
// 16 KiB still is.
func TestRestoreRefusesAnOversizedRecord(t *testing.T) {
	lib, reg, home := restoreLib(t)
	agents := filepath.Join(home, ".agents", "skills")
	batch := filepath.Join(lib.Root, TrashDir, "b1")
	for name, size := range map[string]int{"fits": maxOrigin, "over": maxOrigin + 1} {
		// A clean absolute path in the agents' directory, the record's
		// line break included, size bytes long.
		origin := filepath.Join(agents, name+"-"+strings.Repeat("a", size-len(agents)-len(name)-3))
		p := park(t, batch, name, origin)
		if st, err := os.Stat(p + ".origin"); err != nil || st.Size() != int64(size) {
			t.Fatalf("%s: record is %d bytes, want %d", name, st.Size(), size)
		}
	}
	items, err := ListTrash(lib, reg)
	if err != nil || len(items) != 2 {
		t.Fatalf("%+v %v", items, err)
	}
	for _, it := range items {
		over := it.Name == "over"
		if it.Restorable == over || over != strings.Contains(it.Problem, "not a file skd wrote") {
			t.Errorf("%s: restorable %v, problem %q", it.Name, it.Restorable, it.Problem)
		}
	}
	if _, err := Restore(lib, reg, "b1/over"); err == nil || !strings.Contains(err.Error(), "not a file skd wrote") {
		t.Errorf("restore of the oversized record: %v", err)
	}
}
