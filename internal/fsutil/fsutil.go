// Package fsutil has the small file-system helpers shared across skilldots.
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		return fmt.Errorf("copy %s: %s already exists", src, dst)
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
		return fmt.Errorf("copy %s: %s already exists", src, dst)
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
		return fmt.Errorf("copy %s: cannot copy special file %s", src, p)
	})
}

// CopyFile copies one regular file to dst, replacing a regular file there.
func CopyFile(src, dst string) error {
	if st, err := os.Lstat(dst); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("copy %s: %s exists and is not a regular file", src, dst)
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
		if d.Name() == ".DS_Store" {
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

// FindLicense returns the first LICENSE-like file directly under dir.
func FindLicense(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := strings.ToUpper(e.Name())
		if strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "LICENCE") || strings.HasPrefix(n, "COPYING") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}
