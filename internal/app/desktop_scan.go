package app

import (
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/scan"
	"github.com/Maybe404/skilldots/internal/targets"
)

// noLibrary stands in for the library when there is none yet: a root no
// directory can have (it holds a NUL byte), so nothing on disk is ever
// inside it, and a lock that records no link, so nothing is managed.
const noLibrary = "\x00no library"

// ScanWithoutLibrary inventories the agents' skill directories of this
// machine before any library exists (the first-run screen counts what
// could be adopted). Like Scan it only reads; with no library nothing is
// managed, so every skill found is unmanaged.
func ScanWithoutLibrary() (scan.Report, error) {
	reg, err := targets.NewRegistry("", nil)
	if err != nil {
		return scan.Report{}, err
	}
	lib := &library.Library{Root: noLibrary, Lock: &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}}
	return scan.Run(lib, reg)
}
