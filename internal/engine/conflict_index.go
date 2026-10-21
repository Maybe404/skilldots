package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
)

// While a skill is being resolved, its conflicted files are in the
// library's index as a git merge leaves them: unmerged, with the common
// version (stage 1), the local one (2) and upstream's (3). `git status`
// shows them as both modified (or deleted by one side), `git diff` shows
// the combined diff, `git checkout --ours/--theirs <file>` takes a side
// and `git add <file>` marks it resolved. The entries they replace are
// kept in the repository's git directory (.git/skilldots/), so that
// --abort puts the index back as it was.

// savedIndexPath is where the index entries of the skill at rel were kept.
func savedIndexPath(lib *library.Library, rel string) (string, error) {
	sum := sha256.Sum256([]byte(rel))
	return library.Git(lib.Root, "rev-parse", "--git-path", "skilldots/resolve-"+hex.EncodeToString(sum[:8]))
}

func indexPath(rel, p string) string { return rel + "/" + p }

// StageConflicts puts the conflicted files of the skill at dir into the
// library's index as unmerged entries (stages 1, 2 and 3 from base, ours
// and theirs, a stage left out where that side has no such file). It
// does nothing when the library is not a git repository of its own.
func StageConflicts(lib *library.Library, dir string, conflicts []Conflict, base, ours, theirs FileSet) error {
	if !KeepsSnapshots(lib) || len(conflicts) == 0 {
		return nil
	}
	rel := lib.Rel(dir)
	saved, err := savedIndexPath(lib, rel)
	if err != nil {
		return err
	}
	saved = lib.Abs(saved)
	var paths []string
	for _, c := range conflicts {
		paths = append(paths, indexPath(rel, c.Path))
	}
	// What the index has for these paths now, to put back on --abort.
	before, err := gitInput(lib.Root, nil, nil, append([]string{"ls-files", "-s", "-z", "--"}, literal(paths)...)...)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(saved), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(saved, []byte(before), 0o644); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "skd-stages-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	type stage struct {
		n    int
		path string
		f    *File
	}
	var stages []stage
	var srcs bytes.Buffer
	for _, c := range conflicts {
		for n, side := range []FileSet{base, ours, theirs} {
			f := side[c.Path]
			if f == nil {
				continue
			}
			src := filepath.Join(tmp, strconv.Itoa(len(stages)))
			if err := os.WriteFile(src, f.Data, 0o600); err != nil {
				return err
			}
			srcs.WriteString(src + "\n")
			stages = append(stages, stage{n + 1, indexPath(rel, c.Path), f})
		}
	}
	out, err := gitInput(lib.Root, nil, srcs.Bytes(), "hash-object", "-w", "--no-filters", "--stdin-paths")
	if err != nil {
		return err
	}
	shas := strings.Fields(out)
	if len(shas) != len(stages) {
		return i18n.Errorf("git hash-object answered %d objects for %d files", len(shas), len(stages))
	}
	zero := strings.Repeat("0", len(shas[0]))
	var info bytes.Buffer
	for _, p := range paths {
		info.WriteString("0 " + zero + "\t" + p + "\x00")
	}
	for i, st := range stages {
		mode := "100644"
		if st.f.Exec {
			mode = "100755"
		}
		info.WriteString(mode + " " + shas[i] + " " + strconv.Itoa(st.n) + "\t" + st.path + "\x00")
	}
	_, err = gitInput(lib.Root, nil, info.Bytes(), "update-index", "-z", "--index-info")
	return err
}

// UnstageConflicts takes the unmerged entries of the skill at dir's
// conflicted files out of the library's index and puts back what was
// there before StageConflicts (when that was not kept, the paths are as
// in HEAD). It does nothing when the library is not a git repository of
// its own.
func UnstageConflicts(lib *library.Library, dir string, conflicts []string) error {
	if !KeepsSnapshots(lib) || len(conflicts) == 0 {
		return nil
	}
	rel := lib.Rel(dir)
	var paths []string
	for _, c := range conflicts {
		paths = append(paths, indexPath(rel, c))
	}
	saved, err := savedIndexPath(lib, rel)
	if err != nil {
		return err
	}
	saved = lib.Abs(saved)
	before, rerr := os.ReadFile(saved)
	if _, err := gitInput(lib.Root, nil, nil, append([]string{"update-index", "--force-remove", "--"}, paths...)...); err != nil {
		return err
	}
	if rerr != nil {
		// Nothing kept: as in HEAD (nothing, on a branch with no commit).
		if _, err := library.Git(lib.Root, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
			if _, err := gitInput(lib.Root, nil, nil, append([]string{"reset", "-q", "--"}, literal(paths)...)...); err != nil {
				return err
			}
		}
		return nil
	}
	if len(before) > 0 {
		if _, err := gitInput(lib.Root, nil, before, "update-index", "-z", "--index-info"); err != nil {
			return err
		}
	}
	os.Remove(saved)
	return nil
}

// ForgetStagedConflicts drops what StageConflicts kept for the skill at
// dir (its resolve finished).
func ForgetStagedConflicts(lib *library.Library, dir string) {
	if !KeepsSnapshots(lib) {
		return
	}
	if saved, err := savedIndexPath(lib, lib.Rel(dir)); err == nil {
		os.Remove(lib.Abs(saved))
	}
}

// literal makes paths git pathspecs that match only themselves.
func literal(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = ":(literal)" + p
	}
	return out
}
