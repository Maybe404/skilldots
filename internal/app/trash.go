package app

import "github.com/Maybe404/skilldots/internal/engine"

// Trash lists what skd has set aside, newest first.
func (s *Session) Trash() ([]engine.TrashEntry, error) { return engine.ListTrash(s.Lib) }

// Restore moves a trash entry back where it came from and returns that path.
func (s *Session) Restore(id string) (string, error) { return engine.Restore(s.Lib, id) }

// EmptyTrash permanently deletes everything in the trash. It is the one
// irreversible operation: callers offer it only to a person at a terminal,
// after asking.
func (s *Session) EmptyTrash() (int, error) { return engine.EmptyTrash(s.Lib) }
