package api

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

/*
 * The command line tool. The app's own executable is skd when it runs under
 * that name (desktop/main.go), so installing the tool is a symbolic link on
 * the PATH: /usr/local/bin is on every macOS PATH (/etc/paths), the shells
 * agents start included. An update of the app updates skd with it.
 */

// CLIDir is where installCLI links skd.
const CLIDir = "/usr/local/bin"

// CLI states.
const (
	cliInstalled = "installed" // links to this app
	cliStale     = "stale"     // links to another copy of skilldots, or to nothing (the app moved)
	cliOther     = "other"     // something else is called skd there; never replaced
	cliMissing   = "missing"
)

// Why the running app cannot install skd (cliStatus's "reason").
const (
	reasonDev  = "dev"  // not a packaged skilldots.app (`mygo dev`)
	reasonMove = "move" // run from the disk image, or moved aside by macOS (App Translocation): the link would break
)

// appExecutable is the executable of the running app when it is a packaged
// skilldots.app in a place of its own, resolved; otherwise the reason it
// cannot be linked to.
func appExecutable() (exe, reason string, err error) {
	if exe, err = os.Executable(); err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return "", reasonDev, err
	}
	if !isAppExecutable(exe) {
		return "", reasonDev, errors.New("only the packaged app can install the skd command")
	}
	// The disk image, and where macOS runs a quarantined app from (App
	// Translocation), are read-only and go away: a link there would break.
	// An app installed on an external drive is fine.
	if readOnlyVolume(exe) || strings.Contains(exe, "/AppTranslocation/") {
		return "", reasonMove, errors.New("move skilldots to the Applications folder first, then install the skd command")
	}
	return exe, "", nil
}

// isAppExecutable reports whether path is the executable of a skilldots.app
// bundle (wherever it is or was).
func isAppExecutable(path string) bool {
	return filepath.Base(path) == "skilldots" && strings.HasSuffix(filepath.Dir(path), ".app/Contents/MacOS")
}

// cliState describes dir/skd against exe.
func cliState(dir, exe string) (state, target string) {
	link := filepath.Join(dir, "skd")
	fi, err := os.Lstat(link)
	if err != nil {
		return cliMissing, ""
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return cliOther, ""
	}
	target, _ = os.Readlink(link)
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	if !isAppExecutable(target) {
		return cliOther, target
	}
	real, err := filepath.EvalSymlinks(target)
	if err == nil && exe != "" && real == exe {
		return cliInstalled, target
	}
	return cliStale, target
}

func cliStatus() object {
	exe, reason, _ := appExecutable()
	state, target := cliState(CLIDir, exe)
	return object{"path": filepath.Join(CLIDir, "skd"), "state": state, "target": target, "canInstall": reason == "", "reason": reason}
}

// installCLI links CLIDir/skd to this app, replacing only a link to some
// skilldots.app; elevate runs a shell command as an administrator when the
// directory is not writable.
func installCLI(dir, exe string, elevate func(string) error) error {
	switch state, target := cliState(dir, exe); state {
	case cliInstalled:
		return nil
	case cliOther:
		if target == "" {
			target = filepath.Join(dir, "skd")
		}
		return fmt.Errorf("%s is another program; skd was not replaced", target)
	}
	link := filepath.Join(dir, "skd")
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		// A new link renamed over the old one, so skd never goes missing.
		tmp := link + ".skd-new"
		_ = os.Remove(tmp)
		if err = os.Symlink(exe, tmp); err == nil {
			if err = os.Rename(tmp, link); err != nil {
				_ = os.Remove(tmp)
			}
		}
	}
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrPermission) {
		return err
	}
	return elevate("/bin/mkdir -p " + shellQuote(dir) + " && /bin/ln -sfh " + shellQuote(exe) + " " + shellQuote(link))
}

// removeCLI removes CLIDir/skd when it links to a skilldots.app.
func removeCLI(dir, exe string, elevate func(string) error) error {
	switch state, _ := cliState(dir, exe); state {
	case cliMissing:
		return nil
	case cliOther:
		return fmt.Errorf("%s is another program; it was left in place", filepath.Join(dir, "skd"))
	}
	link := filepath.Join(dir, "skd")
	err := os.Remove(link)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		return err
	}
	return elevate("/bin/rm -f " + shellQuote(link))
}

func installCLIHere() (any, error) {
	exe, _, err := appExecutable()
	if err != nil {
		return nil, err
	}
	if err = installCLI(CLIDir, exe, asAdministrator); err != nil {
		return nil, err
	}
	return cliStatus(), nil
}

func removeCLIHere() (any, error) {
	exe, _, _ := appExecutable()
	if err := removeCLI(CLIDir, exe, asAdministrator); err != nil {
		return nil, err
	}
	return cliStatus(), nil
}

// BundleVersion is the version in the Info.plist of the app bundle holding
// the running executable, or "". Read from the file: as skd the app must not
// start mygo's backend (AppKit) for it.
func BundleVersion() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "..", "Info.plist"))
	if err != nil {
		return ""
	}
	return plistString(string(b), "CFBundleShortVersionString")
}

// plistString is the <string> after <key>key</key> in an XML property list.
func plistString(plist, key string) string {
	_, rest, ok := strings.Cut(plist, "<key>"+key+"</key>")
	if !ok {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t\r\n")
	v, ok := strings.CutPrefix(rest, "<string>")
	if !ok {
		return ""
	}
	v, _, ok = strings.Cut(v, "</string>")
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// errCancelled is a password prompt the user dismissed.
var errCancelled = errors.New("cancelled")

// asAdministrator runs a shell command with macOS's administrator prompt.
func asAdministrator(command string) error {
	script := `do shell script "` + appleScriptString(command) + `" with administrator privileges`
	out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(string(out), "-128") {
		return errCancelled
	}
	return fmt.Errorf("%s", strings.TrimSpace(string(out)))
}

// shellQuote quotes s for /bin/sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// appleScriptString escapes s for a double-quoted AppleScript string.
func appleScriptString(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`)
}
