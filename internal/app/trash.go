package app

import "github.com/Maybe404/skilldots/internal/engine"

// Trash lists what skd has set aside, newest first.
func (s *Session) Trash() ([]engine.TrashEntry, error) { return engine.ListTrash(s.Lib, s.Reg) }

// Restore moves a trash entry back where it came from and returns that
// path. It refuses an entry skd could not have made, or one recorded as
// coming from anywhere but an agent's skill directory or the library's
// vendor/ (engine.Restore).
func (s *Session) Restore(id string) (string, error) { return engine.Restore(s.Lib, s.Reg, id) }

// EmptyTrash permanently deletes everything in the trash. It is the one
// irreversible operation: callers offer it only to a person at a terminal,
// after asking.
func (s *Session) EmptyTrash() (int, error) { return engine.EmptyTrash(s.Lib) }
