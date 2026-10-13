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

// CopyTree replaces dst with a copy of src (regular files and dirs only).
func CopyTree(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
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
