//go:build !darwin

package api

// readOnlyVolume only matters for macOS app bundles.
func readOnlyVolume(string) bool { return false }
