//go:build unix

package tui

import (
	"bufio"
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// proc is one row of the process table: its parent and its start time,
// which tells a process apart from a later one that reuses its pid.
type proc struct {
	ppid  int
	start string
}

// listProcs reads the process table; tests replace it to make ps fail.
var listProcs = processTable

// processTable lists every process with `ps`, leaving out ps itself.
func processTable() (map[int]proc, error) {
	cmd := exec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=", "-o", "lstart=")
	cmd.Env = append(cmd.Environ(), "LC_ALL=C")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	self := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return parsePS(out.Bytes(), self), nil
}

func parsePS(b []byte, skip int) map[int]proc {
	t := map[int]proc{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil || pid == skip {
			continue
		}
		t[pid] = proc{ppid: ppid, start: strings.Join(f[2:], " ")}
	}
	return t
}

// descendants returns every process below parent in t.
func descendants(t map[int]proc, parent int) map[int]proc {
	kids := map[int][]int{}
	for pid, p := range t {
		kids[p.ppid] = append(kids[p.ppid], pid)
	}
	out := map[int]proc{}
	queue := []int{parent}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, k := range kids[n] {
			if _, seen := out[k]; !seen && k != parent {
				out[k] = t[k]
				queue = append(queue, k)
			}
		}
	}
	return out
}

// stopDescendants ends every process started, directly or not, by parent:
// the git and `skd sync` processes of a job and whatever they started.
// The tree is frozen first (SIGSTOP) so nothing forks or is reparented
// away while it is collected, then asked to stop (SIGTERM, which lets git
// remove its .lock files), and what is still there after grace is killed.
// SIGKILL only goes to a pid whose start time still matches the one seen
// in the tree, so a process that reused the pid of one that exited is not
// killed. If ps fails part-way, whatever was frozen is still sent SIGTERM
// and SIGCONT before the error is returned: nothing is left stopped.
func stopDescendants(parent int, grace time.Duration) error {
	tree := map[int]proc{}
	release := func() {
		for pid := range tree {
			syscall.Kill(pid, syscall.SIGTERM)
			syscall.Kill(pid, syscall.SIGCONT)
		}
	}
	for range 50 {
		t, err := listProcs()
		if err != nil {
			release()
			return err
		}
		added := false
		for pid, p := range descendants(t, parent) {
			if _, ok := tree[pid]; ok {
				continue
			}
			tree[pid] = p
			syscall.Kill(pid, syscall.SIGSTOP)
			added = true
		}
		if !added {
			break
		}
	}
	if len(tree) == 0 {
		return nil
	}
	release()
	deadline := time.Now().Add(grace)
	for {
		t, err := listProcs()
		if err != nil {
			return err
		}
		left := 0
		for pid, p := range tree {
			if q, ok := t[pid]; ok && q.start == p.start {
				left++
				if time.Now().After(deadline) {
					syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		}
		if left == 0 || time.Now().After(deadline) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}
