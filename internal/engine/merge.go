package engine

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// What kind of conflict a file has (Conflict.Kind).
const (
	// ConflictContent: a text file both sides changed; the merged file
	// holds Git's conflict markers.
	ConflictContent = "content"
	// ConflictAdded: a text file both sides added with different content
	// (merged against an empty file; it holds conflict markers too).
	ConflictAdded = "added"
	// ConflictBinary: a file that is not text, changed on both sides. The
	// skill keeps the local file; the upstream one is written beside it as
	// <file>.upstream while resolving.
	ConflictBinary = "binary"
	// ConflictDeletedLocal: removed locally, changed upstream. The file
	// stays removed; the upstream one is written as <file>.upstream while
	// resolving.
	ConflictDeletedLocal = "deleted-local"
	// ConflictDeletedUpstream: changed locally, removed upstream. The skill
	// keeps the local file; an empty <file>.upstream-deleted marks it while
	// resolving.
	ConflictDeletedUpstream = "deleted-upstream"
)

// Sidecar suffixes written next to a conflicted file that cannot hold
// conflict markers (see the Conflict kinds).
const (
	UpstreamSuffix = ".upstream"
	DeletedSuffix  = ".upstream-deleted"
)

// LocalLabel is the name of the local side in conflict markers; the
// upstream side is UpstreamLabel(commit).
const LocalLabel = "local"

// UpstreamLabel names the upstream side of a merge in conflict markers:
// "upstream <short commit>".
func UpstreamLabel(commit string) string {
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if commit == "" {
		return "upstream"
	}
	return "upstream " + commit
}

// Conflict is one file of a skill that did not merge.
type Conflict struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// MergeResult is a three-way merge of a skill's files.
type MergeResult struct {
	// Files is the merged skill. A text conflict is there with conflict
	// markers; the other kinds keep the local side (or its absence).
	Files FileSet
	// Conflicts are the files that did not merge, sorted by path.
	Conflicts []Conflict
	// Sidecars are what resolving writes beside the conflicted files that
	// cannot hold markers: <file>.upstream with upstream's version,
	// <file>.upstream-deleted (empty) where upstream removed it.
	Sidecars map[string][]byte
}

// Clean reports whether the merge had no conflicts.
func (m *MergeResult) Clean() bool { return len(m.Conflicts) == 0 }

// ConflictPaths are the conflicted paths.
func (m *MergeResult) ConflictPaths() []string {
	out := []string{}
	for _, c := range m.Conflicts {
		out = append(out, c.Path)
	}
	return out
}

func sameFile(a, b *File) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return bytes.Equal(a.Data, b.Data)
}

func isText(f *File) bool {
	return f == nil || (bytes.IndexByte(f.Data, 0) < 0 && utf8.Valid(f.Data))
}

// Merge merges ours (the local copy) and theirs (upstream's new version)
// against base (the version both started from; empty when not known),
// file by file over the union of their paths: a file only one side
// changed takes that side; text both changed goes through `git
// merge-file`; anything else both changed is a conflict. upstream is the
// upstream commit, for the conflict markers' label.
func Merge(base, ours, theirs FileSet, upstream string) (*MergeResult, error) {
	res := &MergeResult{Files: FileSet{}, Conflicts: []Conflict{}, Sidecars: map[string][]byte{}}
	paths := map[string]bool{}
	for _, s := range []FileSet{base, ours, theirs} {
		for p := range s {
			paths[p] = true
		}
	}
	all := make([]string, 0, len(paths))
	for p := range paths {
		all = append(all, p)
	}
	sort.Strings(all)
	for _, p := range all {
		b, o, t := base[p], ours[p], theirs[p]
		switch {
		case sameFile(o, t):
			if o != nil {
				res.Files[p] = o
			}
			continue
		case sameFile(o, b):
			if t != nil {
				res.Files[p] = t
			}
			continue
		case sameFile(t, b):
			if o != nil {
				res.Files[p] = o
			}
			continue
		}
		// Both sides changed p, differently.
		switch {
		case o == nil:
			res.Conflicts = append(res.Conflicts, Conflict{p, ConflictDeletedLocal})
			res.Sidecars[p+UpstreamSuffix] = t.Data
		case t == nil:
			res.Files[p] = o
			res.Conflicts = append(res.Conflicts, Conflict{p, ConflictDeletedUpstream})
			res.Sidecars[p+DeletedSuffix] = []byte{}
		case !isText(o) || !isText(t) || !isText(b):
			res.Files[p] = o
			res.Conflicts = append(res.Conflicts, Conflict{p, ConflictBinary})
			res.Sidecars[p+UpstreamSuffix] = t.Data
		default:
			var bd []byte
			if b != nil {
				bd = b.Data
			}
			merged, conflicts, err := mergeText(o.Data, bd, t.Data, UpstreamLabel(upstream))
			if err != nil {
				return nil, i18n.Errorf("merging %s: %w", p, err)
			}
			res.Files[p] = &File{Data: merged, Exec: o.Exec}
			if conflicts {
				kind := ConflictContent
				if b == nil {
					kind = ConflictAdded
				}
				res.Conflicts = append(res.Conflicts, Conflict{p, kind})
			}
		}
	}
	// A sidecar never replaces a file of the skill.
	for sc := range res.Sidecars {
		if res.Files[sc] != nil {
			return nil, i18n.Errorf("cannot mark the conflict in %s: the skill already has a file %s", strings.TrimSuffix(strings.TrimSuffix(sc, UpstreamSuffix), DeletedSuffix), sc)
		}
	}
	return res, nil
}

// mergeText is `git merge-file -p` of ours, base and theirs: the merged
// text, and whether it holds conflicts.
func mergeText(ours, base, theirs []byte, upstream string) ([]byte, bool, error) {
	dir, err := os.MkdirTemp("", "skd-merge-")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)
	names := []string{"local", "base", "upstream"}
	for i, b := range [][]byte{ours, base, theirs} {
		if err := os.WriteFile(filepath.Join(dir, names[i]), b, 0o600); err != nil {
			return nil, false, err
		}
	}
	cmd := exec.Command("git", "merge-file", "-p", "-L", LocalLabel, "-L", "base", "-L", upstream, "local", "base", "upstream")
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out.Bytes(), false, nil
	case errors.As(err, &exit) && exit.ExitCode() > 0 && exit.ExitCode() < 128:
		// The number of conflicts.
		return out.Bytes(), true, nil
	}
	msg := strings.TrimSpace(errb.String())
	if msg == "" {
		msg = err.Error()
	}
	return nil, false, i18n.Errorf("git merge-file: %s", msg)
}

// WriteFileSet makes dir hold exactly files, plus extra (sidecars): a
// file that is already as wanted is left alone, others are written,
// files no longer wanted are removed, and directories left empty go.
// What fsutil.Skip names and what is not a regular file are left as
// they are.
func WriteFileSet(dir string, files FileSet, extra map[string][]byte) error {
	have := FileSet{}
	if fsutil.Lexists(dir) {
		var err error
		if have, err = ReadFileSet(dir); err != nil {
			return err
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	want := FileSet{}
	for p, f := range files {
		want[p] = f
	}
	for p, b := range extra {
		want[p] = &File{Data: b}
	}
	for _, p := range have.Paths() {
		if want[p] == nil {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
				return err
			}
			pruneUp(dir, filepath.Dir(filepath.Join(dir, filepath.FromSlash(p))))
		}
	}
	for _, p := range want.Paths() {
		f := want[p]
		dest := filepath.Join(dir, filepath.FromSlash(p))
		if !fsutil.Within(dir, dest) {
			return i18n.Errorf("%s is outside %s", dest, dir)
		}
		if h := have[p]; h != nil && bytes.Equal(h.Data, f.Data) && h.Exec == f.Exec {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Exec {
			mode = 0o755
		}
		if err := fsutil.WriteAtomic(dest, f.Data); err != nil {
			return err
		}
		if err := os.Chmod(dest, mode); err != nil {
			return err
		}
	}
	return nil
}

// pruneUp removes d and its empty parents up to (not including) top.
func pruneUp(top, d string) {
	for fsutil.Within(top, d) {
		if err := os.Remove(d); err != nil {
			return
		}
		d = filepath.Dir(d)
	}
}

// markerLine reports whether line is a conflict marker as Git writes
// them: seven '<', '|', '=' or '>' alone or followed by a space.
func markerLine(line string) bool {
	line = strings.TrimSuffix(line, "\r")
	if len(line) < 7 {
		return false
	}
	c := line[0]
	if c != '<' && c != '>' && c != '=' && c != '|' {
		return false
	}
	for i := 1; i < 7; i++ {
		if line[i] != c {
			return false
		}
	}
	return len(line) == 7 || line[7] == ' '
}

// HasMarkers reports whether text holds a conflict marker line. With
// all unset only the lines that open and close a conflict count
// (<<<<<<< and >>>>>>>): seven '=' alone is also a Markdown heading's
// underline, so it is taken for a marker only in a file that had a
// conflict.
func HasMarkers(text []byte, all bool) bool {
	for _, l := range strings.Split(string(text), "\n") {
		if markerLine(l) && (all || l[0] == '<' || l[0] == '>') {
			return true
		}
	}
	return false
}

// Unresolved lists what still keeps a skill being resolved from being
// finished: files under dir holding conflict markers, and the sidecars
// skd wrote for the conflicts (conflicts) that are still there.
func Unresolved(dir string, conflicts []string) ([]string, error) {
	files, err := ReadFileSet(dir)
	if err != nil {
		return nil, err
	}
	conflicted := map[string]bool{}
	for _, c := range conflicts {
		conflicted[c] = true
	}
	var out []string
	for _, p := range files.Paths() {
		if isText(files[p]) && HasMarkers(files[p].Data, conflicted[p]) {
			out = append(out, p)
		}
	}
	for _, c := range conflicts {
		for _, sc := range []string{c + UpstreamSuffix, c + DeletedSuffix} {
			if fsutil.Lexists(filepath.Join(dir, filepath.FromSlash(sc))) {
				out = append(out, sc)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}
