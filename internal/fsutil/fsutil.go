// Package fsutil has the small file-system helpers shared across skilldots.
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/i18n"
)

// Skip lists directory names never copied or hashed.
var Skip = map[string]bool{".git": true, ".DS_Store": true, "node_modules": true}

// HashTree returns a stable sha256 over every regular file under dir
// (relative path + content), ignoring Skip entries.
func HashTree(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if Skip[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, p := range files {
		rel, _ := filepath.Rel(dir, p)
		io.WriteString(h, filepath.ToSlash(rel))
		h.Write([]byte{0})
		f, err := os.Open(p)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", err
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// CopyTree copies src to dst (regular files and dirs only, Skip entries
// left out). dst must not exist: CopyTree never removes anything, callers
// decide what happens to whatever was there.
func CopyTree(src, dst string) error {
	if Lexists(dst) {
		return i18n.Errorf("copy %s: %s already exists", src, dst)
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if Skip[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(p, target)
	})
}

// CopyAll copies src to dst exactly: every file including .git and
// node_modules, symlinks as symlinks, permissions kept. dst must not exist.
// Anything it cannot reproduce (sockets, devices) is an error, so a caller
// about to remove src never loses data silently.
func CopyAll(src, dst string) error {
	if Lexists(dst) {
		return i18n.Errorf("copy %s: %s already exists", src, dst)
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			return copyFile(p, target)
		}
		return i18n.Errorf("copy %s: cannot copy special file %s", src, p)
	})
}

// CopyFile copies one regular file to dst, replacing a regular file there.
func CopyFile(src, dst string) error {
	if st, err := os.Lstat(dst); err == nil && !st.Mode().IsRegular() {
		return i18n.Errorf("copy %s: %s exists and is not a regular file", src, dst)
	}
	return copyFile(src, dst)
}

// Within reports whether p is strictly inside dir, lexically.
func Within(dir, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	if err != nil || rel == "." || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Pristine reports whether dir holds exactly what a CopyTree with the
// given HashTree hash produced: same files, and nothing HashTree ignores
// (.git, node_modules, symlinks, special files). Only a pristine copy may
// be discarded; anything else could hold someone's work.
func Pristine(dir, hash string) bool {
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || hash == "" {
		return false
	}
	extra := false
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		// Finder's .DS_Store file is noise; anything else by that name
		// (a directory, a link) can hold data and counts as extra.
		if d.Name() == ".DS_Store" && d.Type().IsRegular() {
			return nil
		}
		if Skip[d.Name()] || !(d.IsDir() || d.Type().IsRegular()) {
			extra = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil || extra {
		return false
	}
	h, err := HashTree(dir)
	return err == nil && h == hash
}

func copyFile(src, dst string) error {
	// Never follow a symlink: its target may be anywhere on the machine.
	if st, err := os.Lstat(src); err != nil {
		return err
	} else if !st.Mode().IsRegular() {
		return i18n.Errorf("copy %s: not a regular file", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Exists reports whether p exists (following symlinks).
func Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Lexists reports whether p exists without following a final symlink.
func Lexists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// IsLicenseName reports whether a file name looks like a license.
func IsLicenseName(name string) bool {
	n := strings.ToUpper(name)
	return strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "LICENCE") || strings.HasPrefix(n, "COPYING")
}

// FindLicense returns the first LICENSE-like regular file directly under
// dir. Symlinks are ignored: in an untrusted repository they could point
// at any file on this machine.
func FindLicense(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.Type().IsRegular() && IsLicenseName(e.Name()) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// LicenseLinks lists LICENSE-like entries directly under dir that are not
// regular files (symlinks and the like), which FindLicense ignores.
func LicenseLinks(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && !e.Type().IsRegular() && IsLicenseName(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// staleTemp is how old a leftover temporary file must be before a save
// removes it. A save takes milliseconds; a younger file may belong to a
// save running in another skd process right now.
const staleTemp = time.Minute

// removeStaleTemps deletes regular files in dir matching pattern (the
// temporary-file pattern of WriteAtomic) that a killed save left behind.
func removeStaleTemps(dir, pattern string) {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	for _, m := range matches {
		if st, err := os.Lstat(m); err == nil && st.Mode().IsRegular() && time.Since(st.ModTime()) > staleTemp {
			os.Remove(m)
		}
	}
}

// WriteAtomic replaces path with data so a crash or kill mid-write leaves
// either the old file or the new one, never a torn one: write a temporary
// file next to it, then rename over it. A symlinked file is written
// through (the link stays), and an existing file keeps its permissions.
//
// A rename replaces a file whatever its own mode, so a file the user made
// read-only (no owner write bit) is refused here, as an in-place write
// would be.
func WriteAtomic(path string, data []byte) error {
	if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return os.WriteFile(path, data, 0o644)
		}
		path = real
	}
	perm := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
		if perm&0o200 == 0 {
			return &fs.PathError{Op: "write", Path: path, Err: fs.ErrPermission}
		}
	}
	pattern := "." + filepath.Base(path) + ".tmp-*"
	removeStaleTemps(filepath.Dir(path), pattern)
	f, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, perm)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp) // our own temporary file, never the user's
	}
	return err
}

// Resolve is p with every symbolic link resolved: of its longest part
// that exists, with the rest appended as it is.
func Resolve(p string) string {
	var rest []string
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(append([]string{r}, rest...)...)
		}
		d := filepath.Dir(p)
		if d == p {
			return filepath.Join(append([]string{p}, rest...)...)
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = d
	}
}

// SameDir reports whether a and b are the same place: the same path, or
// both exist and are the same file (two spellings of one directory on a
// file system that ignores case or Unicode normalisation, as macOS and
// Windows do by default). Loose also counts paths that differ only in
// case: use it only to refuse, never to allow.
func SameDir(a, b string, loose bool) bool {
	if a == b || (loose && strings.EqualFold(a, b)) {
		return true
	}
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

// Inside reports whether p is dir or inside it, comparing p and each of
// its parents with dir by SameDir. Both should be clean, absolute and
// have their links resolved (Resolve).
func Inside(dir, p string, loose bool) bool {
	for {
		if SameDir(p, dir, loose) {
			return true
		}
		d := filepath.Dir(p)
		if d == p {
			return false
		}
		p = d
	}
}

// LinkOnPath returns the first symbolic link on the way from root (not
// included: the root itself may be reached through links) down to p
// (included), or "" when there is none. p must be inside root lexically;
// when it is not, p itself is returned. Parts that do not exist end the
// walk: nothing below them can be a link yet.
func LinkOnPath(root, p string) string {
	root, p = filepath.Clean(root), filepath.Clean(p)
	if !Within(root, p) {
		return p
	}
	rel, _ := filepath.Rel(root, p)
	q := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		q = filepath.Join(q, part)
		st, err := os.Lstat(q)
		if err != nil {
			return ""
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return q
		}
	}
	return ""
}
