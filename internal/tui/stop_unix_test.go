//go:build unix

package tui

import (
	"bufio"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// stopDescendants ends a whole tree below a process: a child, a
// grandchild, and one that ignores SIGTERM. The process itself is left.
func TestStopDescendants(t *testing.T) {
	sh := exec.Command("sh", "-c", `
sleep 300 & echo $!
sh -c 'sleep 300 & echo $!; wait' &
(trap '' TERM; exec sleep 300) & echo $!
wait`)
	out, err := sh.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := sh.Start(); err != nil {
		t.Fatal(err)
	}
	defer sh.Process.Kill()
	var pids []int
	sc := bufio.NewScanner(out)
	for len(pids) < 3 && sc.Scan() {
		pid, err := strconv.Atoi(sc.Text())
		if err != nil {
			t.Fatal(err)
		}
		pids = append(pids, pid)
	}
	if len(pids) != 3 {
		t.Fatalf("pids: %v", pids)
	}
	for _, p := range pids {
		defer syscall.Kill(p, syscall.SIGKILL)
	}
	time.Sleep(100 * time.Millisecond) // let the TERM trap be set
	start := time.Now()
	if err := stopDescendants(sh.Process.Pid, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 400*time.Millisecond {
		t.Error("the TERM-ignoring process was killed before the grace period")
	}
	done := make(chan error, 1)
	go func() { done <- sh.Wait() }()
	select {
	case <-done: // its children are gone, so its wait returned
	case <-time.After(5 * time.Second):
		t.Fatal("the shell is still waiting for children")
	}
	tab, err := processTable()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pids {
		if _, ok := tab[p]; ok {
			t.Errorf("process %d survived", p)
		}
	}
}

func TestDescendants(t *testing.T) {
	ps := []byte(`    1     0 Fri Oct 16 10:00:00 2026
  100     1 Fri Oct 16 10:00:01 2026
  200   100 Fri Oct 16 10:00:02 2026
  300   200 Fri Oct 16 10:00:03 2026
  301   200 Fri Oct 16 10:00:03 2026
  400     1 Fri Oct 16 10:00:04 2026
  999   100 Fri Oct 16 10:00:05 2026
garbage
`)
	tab := parsePS(ps, 999)
	if _, ok := tab[999]; ok {
		t.Error("ps itself listed")
	}
	if tab[300].start != "Fri Oct 16 10:00:03 2026" || tab[300].ppid != 200 {
		t.Errorf("row: %+v", tab[300])
	}
	d := descendants(tab, 100)
	if len(d) != 3 {
		t.Errorf("descendants of 100: %v", d)
	}
	for _, pid := range []int{200, 300, 301} {
		if _, ok := d[pid]; !ok {
			t.Errorf("%d missing", pid)
		}
	}
	if len(descendants(tab, 300)) != 0 {
		t.Error("a leaf has descendants")
	}
}
