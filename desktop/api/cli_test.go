package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeApp makes <dir>/<name>.app/Contents/MacOS/skilldots and returns it.
func fakeApp(t *testing.T, dir, name string) string {
	t.Helper()
	exe := filepath.Join(dir, name+".app", "Contents", "MacOS", "skilldots")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(exe)
	return real
}

func noElevate(t *testing.T) func(string) error {
	return func(cmd string) error { t.Fatalf("unexpected elevation: %s", cmd); return nil }
}

func TestCLIInstallStates(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	exe := fakeApp(t, root, "skilldots")
	if st, _ := cliState(bin, exe); st != cliMissing {
		t.Fatalf("state = %s, want missing", st)
	}
	if err := installCLI(bin, exe, noElevate(t)); err != nil {
		t.Fatal(err)
	}
	if st, target := cliState(bin, exe); st != cliInstalled || target != exe {
		t.Fatalf("state = %s %s, want installed", st, target)
	}
	// Installing again is a no-op.
	if err := installCLI(bin, exe, noElevate(t)); err != nil {
		t.Fatal(err)
	}

	// The app moved: the old link is stale and is replaced.
	moved := fakeApp(t, root, "moved/skilldots")
	if st, _ := cliState(bin, moved); st != cliStale {
		t.Fatalf("state = %s, want stale", st)
	}
	if err := installCLI(bin, moved, noElevate(t)); err != nil {
		t.Fatal(err)
	}
	if st, _ := cliState(bin, moved); st != cliInstalled {
		t.Fatalf("state = %s, want installed after repair", st)
	}
	if _, err := os.Lstat(filepath.Join(bin, "skd.skd-new")); !os.IsNotExist(err) {
		t.Fatal("temporary link left behind")
	}

	if err := removeCLI(bin, moved, noElevate(t)); err != nil {
		t.Fatal(err)
	}
	if st, _ := cliState(bin, moved); st != cliMissing {
		t.Fatalf("state = %s, want missing after remove", st)
	}
}

func TestCLIKeepsOtherPrograms(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	exe := fakeApp(t, root, "skilldots")
	os.MkdirAll(bin, 0o755)

	// A go-installed skd (a file) and a link to some other program are not ours.
	// Another app's executable is not ours either, though it sits in an .app.
	other := filepath.Join(root, "Foo.app", "Contents", "MacOS", "foo")
	for _, make := range []func(string) error{
		func(p string) error { return os.WriteFile(p, []byte("other"), 0o755) },
		func(p string) error { return os.Symlink(filepath.Join(root, "go", "bin", "skd"), p) },
		func(p string) error { return os.Symlink(other, p) },
	} {
		link := filepath.Join(bin, "skd")
		os.Remove(link)
		if err := make(link); err != nil {
			t.Fatal(err)
		}
		if st, _ := cliState(bin, exe); st != cliOther {
			t.Fatalf("state = %s, want other", st)
		}
		if err := installCLI(bin, exe, noElevate(t)); err == nil || !strings.Contains(err.Error(), "another program") {
			t.Fatalf("install over another program: %v", err)
		}
		if err := removeCLI(bin, exe, noElevate(t)); err == nil {
			t.Fatal("removed another program")
		}
		if _, err := os.Lstat(link); err != nil {
			t.Fatal("another program was touched")
		}
	}
}

func TestCLIElevatesWhenNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	exe := fakeApp(t, root, "it's \"here\"")
	os.MkdirAll(bin, 0o555)
	t.Cleanup(func() { os.Chmod(bin, 0o755) })
	var got string
	if err := installCLI(bin, exe, func(cmd string) error { got = cmd; return nil }); err != nil {
		t.Fatal(err)
	}
	want := "/bin/mkdir -p " + shellQuote(bin) + " && /bin/ln -sfh " + shellQuote(exe) + " " + shellQuote(filepath.Join(bin, "skd"))
	if got != want {
		t.Fatalf("elevated command\n got %s\nwant %s", got, want)
	}
	if !strings.Contains(shellQuote(exe), `'\''`) {
		t.Fatal("single quote not escaped")
	}
	if appleScriptString(`a "b" \c`) != `a \"b\" \\c` {
		t.Fatal("AppleScript escaping")
	}
}

func TestPlistString(t *testing.T) {
	plist := "<dict>\n\t<key>CFBundleName</key>\n\t<string>skilldots</string>\n\t<key>CFBundleShortVersionString</key>\n\t<string>0.1.0</string>\n</dict>"
	if v := plistString(plist, "CFBundleShortVersionString"); v != "0.1.0" {
		t.Fatalf("version = %q", v)
	}
	if v := plistString(plist, "Missing"); v != "" {
		t.Fatalf("missing key = %q", v)
	}
	if v := plistString("<key>CFBundleShortVersionString</key><integer>1</integer>", "CFBundleShortVersionString"); v != "" {
		t.Fatalf("non-string value = %q", v)
	}
}
