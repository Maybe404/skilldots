package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
)

const maxTextSize = 4 << 20

type fileNode struct {
	Path     string     `json:"path"`
	Name     string     `json:"name"`
	Dir      bool       `json:"dir"`
	Children []fileNode `json:"children,omitempty"`
}

func (s *Service) fileOperation(ses *app.Session, method string, args []json.RawMessage) (any, error) {
	r, err := argument[ref](args, 0)
	if err != nil {
		return nil, err
	}
	if r.Key == app.SyncRef {
		return syncFileOperation(ses, method, args)
	}
	entries, err := ses.Resolve()
	if err != nil {
		return nil, err
	}
	e, err := ses.Entry(entries, r.app())
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errors.New("skill no longer exists")
	}
	if e.LinkedVia != "" || e.Refused != "" || e.Missing {
		return nil, errors.New("skill directory is unavailable or unsafe")
	}
	if method == "reveal" {
		if s.native.Reveal == nil {
			return nil, errors.New("Finder unavailable")
		}
		return nil, s.native.Reveal(e.Dir)
	}
	if method == "openInEditor" {
		o, err := ses.Edit(r.app(), app.EditOptions{Force: true, NoTerminal: true})
		if err != nil {
			return nil, err
		}
		return nil, o.Editor.Start(o.Dir)
	}
	root, err := os.OpenRoot(e.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if method == "files" {
		count := 0
		return fileTree(root, ".", &count)
	}
	name, err := argument[string](args, 1)
	if err != nil {
		return nil, err
	}
	if name == "." || !fs.ValidPath(name) || strings.Contains(name, "\\") {
		return nil, errors.New("invalid relative file path")
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return nil, errors.New("hidden files are not editable")
		}
	}
	if fsutil.LinkOnPath(e.Dir, filepath.Join(e.Dir, filepath.FromSlash(name))) != "" {
		return nil, errors.New("symbolic links are not editable")
	}
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
	if e.Kind == engine.KindApp {
		return nil, errors.New("fork an application skill before editing it")
	}
	content, err := argument[string](args, 2)
	if err != nil {
		return nil, err
	}
	if len(content) > maxTextSize || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return nil, errors.New("text must be UTF-8 and smaller than 4 MiB")
	}
	// The directory handle confines writes even if a parent is replaced. Refuse
	// links above, and replace atomically so interrupted saves keep the old file.
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
	// Saving is staging (git add); skd sync, or the sync button, commits.
	// A file that could not be staged is saved all the same.
	if err := ses.StageFile(filepath.Join(e.Dir, filepath.FromSlash(name))); err != nil {
		return object{"staged": false, "warning": err.Error()}, nil
	}
	return object{"staged": true}, nil
}

func fileTree(root *os.Root, dir string, count *int) ([]fileNode, error) {
	f, err := root.Open(dir)
	if err != nil {
		return nil, err
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	out := []fileNode{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		*count++
		if *count > 10000 {
			return nil, fmt.Errorf("skill contains too many files")
		}
		p := e.Name()
		if dir != "." {
			p = dir + "/" + p
		}
		n := fileNode{Path: p, Name: e.Name(), Dir: e.IsDir()}
		if e.IsDir() {
			n.Children, err = fileTree(root, p, count)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, n)
	}
	return out, nil
}
