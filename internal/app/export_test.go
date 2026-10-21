package app

// SetFilesRead sets the hook told each directory the update diff reads
// (nil clears it).
func SetFilesRead(f func(dir string)) { filesRead = f }
