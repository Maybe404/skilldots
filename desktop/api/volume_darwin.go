package api

import "golang.org/x/sys/unix"

// readOnlyVolume reports whether path is on a read-only volume: a mounted
// disk image, or where macOS runs a translocated app from.
func readOnlyVolume(path string) bool {
	var st unix.Statfs_t
	return unix.Statfs(path, &st) == nil && st.Flags&unix.MNT_RDONLY != 0
}
