package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// File is one regular file of a skill: its content and whether it is
// executable.
type File struct {
	Data []byte
	Exec bool
}

// FileSet is a skill's files as fsutil.HashTree sees them (regular files,
// fsutil.Skip names left out), keyed by slash-separated path relative to
// the skill's directory.
type FileSet map[string]*File

// Paths are the set's paths, sorted.
func (s FileSet) Paths() []string {
	out := make([]string, 0, len(s))
	for p := range s {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Hash is fsutil.HashTree of a directory holding exactly these files.
func (s FileSet) Hash() string {
	h := sha256.New()
	for _, p := range s.Paths() {
		io.WriteString(h, p)
		h.Write([]byte{0})
		h.Write(s[p].Data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ReadFileSet reads every regular file under dir as fsutil.HashTree does.
func ReadFileSet(dir string) (FileSet, error) {
	out := FileSet{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if fsutil.Skip[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = &File{Data: b, Exec: info.Mode().Perm()&0o111 != 0}
		return nil
	})
	return out, err
}

// GitBlob is one file of a git tree.
type GitBlob struct {
	// Path is relative to the directory read, with forward slashes.
	Path string
	SHA  string
	Exec bool
}

// GitTreeBlobs lists the regular files under rel (a directory, with
// forward slashes; "" for the whole tree) in the tree-ish rev of the
// repository at root, leaving out fsutil.Skip names as HashTree does.
// Symbolic links and submodules are left out like HashTree leaves them.
func GitTreeBlobs(ctx context.Context, root, rev, rel string) ([]GitBlob, error) {
	args := []string{"-C", root, "ls-tree", "-r", "-z", "--full-tree", rev}
	if rel != "" {
		args = append(args, "--", rel)
	}
	ls, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return nil, i18n.Errorf("git ls-tree %s: %w", rev, err)
	}
	var out []GitBlob
	for _, rec := range bytes.Split(ls, []byte{0}) {
		meta, path, ok := strings.Cut(string(rec), "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		if len(f) != 3 || f[1] != "blob" || (f[0] != "100644" && f[0] != "100755") {
			continue
		}
		sub := path
		if rel != "" {
			if sub, ok = strings.CutPrefix(path, rel+"/"); !ok {
				continue
			}
		}
		skip := false
		for _, part := range strings.Split(sub, "/") {
			skip = skip || fsutil.Skip[part]
		}
		if !skip {
			out = append(out, GitBlob{Path: sub, SHA: f[2], Exec: f[0] == "100755"})
		}
	}
	return out, nil
}

// ReadGitBlobs reads blobs from the repository at root, as they come
// (git cat-file --batch). each is given every blob's content; with big
// > 0 a blob larger than that is not read into memory: each gets its
// SHA-256 instead, with tooBig set.
func ReadGitBlobs(ctx context.Context, root string, blobs []GitBlob, big int, each func(b GitBlob, data []byte, sum []byte) error) error {
	if len(blobs) == 0 {
		return nil
	}
	var in bytes.Buffer
	for _, b := range blobs {
		in.WriteString(b.SHA + "\n")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "cat-file", "--batch")
	cmd.Stdin = &in
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() { cancel(); cmd.Wait() }()
	r := bufio.NewReader(pipe)
	for _, b := range blobs {
		head, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		f := strings.Fields(head)
		if len(f) != 3 || f[1] != "blob" {
			return i18n.Errorf("git cat-file answered %q, not a file's contents", strings.TrimSpace(head))
		}
		n, err := strconv.Atoi(f[2])
		if err != nil || n < 0 {
			return i18n.Errorf("git cat-file answered %q, not a file's contents", strings.TrimSpace(head))
		}
		if big > 0 && n > big {
			h := sha256.New()
			if _, err := io.CopyN(h, r, int64(n)); err != nil {
				return err
			}
			if _, err := r.ReadByte(); err != nil { // git's newline after it
				return err
			}
			if err := each(b, nil, h.Sum(nil)); err != nil {
				return err
			}
			continue
		}
		body := make([]byte, n+1) // the content and git's newline after it
		if _, err := io.ReadFull(r, body); err != nil {
			return err
		}
		if err := each(b, body[:n], nil); err != nil {
			return err
		}
	}
	return nil
}

// ReadGitFileSet reads the files under rel in the tree-ish rev of the
// repository at root, whole.
func ReadGitFileSet(ctx context.Context, root, rev, rel string) (FileSet, error) {
	blobs, err := GitTreeBlobs(ctx, root, rev, rel)
	if err != nil {
		return nil, err
	}
	out := FileSet{}
	err = ReadGitBlobs(ctx, root, blobs, 0, func(b GitBlob, data, _ []byte) error {
		out[b.Path] = &File{Data: data, Exec: b.Exec}
		return nil
	})
	return out, err
}
