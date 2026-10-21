package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// syncFileOperation reads and writes the files of a sync being resolved
// (readFile and writeFile with the ref {key: "@sync"}), so a sync's
// conflicts can be edited by hand as a skill's are. Only the files the
// merge conflicts on, relative to the library's root, under the same
// rules as a skill's files: no symbolic link on the way, nothing outside
// the library, UTF-8 regular files below 4 MiB. A file written is added
// (git add), as the editor stages what it saves.
func syncFileOperation(ses *app.Session, method string, args []json.RawMessage) (any, error) {
	if method != "readFile" && method != "writeFile" {
		return nil, errors.New("a sync's conflicts are read and written file by file")
	}
	name, err := argument[string](args, 1)
	if err != nil {
		return nil, err
	}
	if name == "." || !fs.ValidPath(name) || strings.Contains(name, "\\") {
		return nil, errors.New("invalid relative file path")
	}
	if !slices.Contains(ses.SyncConflictPaths(), name) {
		return nil, errors.New("not a file the sync being resolved conflicts on")
	}
	lib := ses.Lib.Root
	if fsutil.LinkOnPath(lib, filepath.Join(lib, filepath.FromSlash(name))) != "" {
		return nil, errors.New("symbolic links are not editable")
	}
	root, err := os.OpenRoot(lib)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	st, err := root.Stat(name)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxTextSize {
		return nil, errors.New("choose a text file smaller than 4 MiB")
	}
	if method == "readFile" {
		b, err := root.ReadFile(name)
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(b) || strings.ContainsRune(string(b), 0) {
			return nil, errors.New("file is not UTF-8 text")
		}
		return string(b), nil
	}
	content, err := argument[string](args, 2)
	if err != nil {
		return nil, err
	}
	if len(content) > maxTextSize || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return nil, errors.New("text must be UTF-8 and smaller than 4 MiB")
	}
	tmp, err := root.OpenFile(name+".skd-save", os.O_WRONLY|os.O_CREATE|os.O_EXCL, st.Mode().Perm())
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.Remove(name + ".skd-save")
		}
	}()
	_, werr := tmp.WriteString(content)
	cerr := tmp.Close()
	if werr != nil {
		return nil, werr
	}
	if cerr != nil {
		return nil, cerr
	}
	if err := root.Rename(name+".skd-save", name); err != nil {
		return nil, err
	}
	cleanup = false
	if _, err := ses.Lib.Git("add", "--", ":(literal)"+name); err != nil {
		return object{"staged": false, "warning": err.Error()}, nil
	}
	return object{"staged": true}, nil
}

// openURL opens an https:// address in the system's browser; anything
// else (another scheme, a local file, text that is not a URL) is
// refused.
func openURL(u string) error {
	if err := checkURL(u); err != nil {
		return err
	}
	return launchURL(u)
}

// launchURL hands an address to the system (macOS: open). Replaced in
// tests.
var launchURL = func(u string) error {
	cmd := "open"
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
	default:
		cmd = "xdg-open"
	}
	return exec.Command(cmd, u).Start()
}

// checkURL accepts only an https:// address with a host, and nothing that
// a terminal or a command line could take for something else.
func checkURL(u string) error {
	if !strings.HasPrefix(u, "https://") || safetext.Has(u) || strings.ContainsAny(u, " \t\r\n") {
		return errors.New("only https:// addresses are opened")
	}
	p, err := url.Parse(u)
	if err != nil || p.Scheme != "https" || p.Host == "" || p.User != nil {
		return errors.New("only https:// addresses are opened")
	}
	return nil
}
