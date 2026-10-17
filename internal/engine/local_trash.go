package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
)

var localTrashMu sync.Mutex

func localTrashPath() string { return filepath.Join(filepath.Dir(config.Path()), "local-trash.json") }
func readLocalTrash() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(localTrashPath())
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]string{}
	}
	return m, err
}

// TrashLocal records local ownership outside the clonable library. A forged
// .origin in a cloned library must not create skills in skills/ or private/.
func TrashLocal(lib *library.Library, src string) (string, error) {
	localTrashMu.Lock()
	defer localTrashMu.Unlock()
	records, err := readLocalTrash()
	if err != nil {
		return "", err
	}
	dest, err := Trash(lib, src)
	if err != nil {
		return dest, err
	}
	records[fsutil.Resolve(dest)] = src
	b, err := json.Marshal(records)
	if err != nil {
		return dest, err
	}
	if err = fsutil.WriteAtomic(localTrashPath(), b); err != nil {
		return dest, i18n.Errorf("%s was moved to %s, but where it came from could not be recorded: %w", src, dest, err)
	}
	return dest, nil
}
func ownsLocalTrash(entry, origin string) bool {
	localTrashMu.Lock()
	defer localTrashMu.Unlock()
	records, err := readLocalTrash()
	return err == nil && records[fsutil.Resolve(entry)] == origin
}
