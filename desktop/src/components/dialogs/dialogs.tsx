import { useApp } from "@/lib/app-state"
import { AddDialog } from "./add-dialog"
import { ApplyDialog, EmptyTrashDialog, LibraryDialog, SyncDialog, UpdateDialog } from "./ops-dialogs"
import { ConfirmDialog, ForkDialog, NewSkillDialog } from "./skill-dialogs"

/** Renders whichever dialog the app state asks for. */
export function Dialogs() {
  const { dialog: d, closeDialog } = useApp()
  if (!d) return null
  switch (d.kind) {
    case "add":
      return <AddDialog initial={d.initial} onClose={closeDialog} />
    case "new":
      return <NewSkillDialog onClose={closeDialog} />
    case "fork":
      return <ForkDialog skillRef={d.ref} thenEdit={d.thenEdit} onClose={closeDialog} />
    case "update":
      return <UpdateDialog repos={d.repos} onClose={closeDialog} />
    case "apply":
      return <ApplyDialog onClose={closeDialog} />
    case "sync":
      return <SyncDialog onClose={closeDialog} />
    case "library":
      return <LibraryDialog mode={d.mode} onClose={closeDialog} />
    case "remove":
      return <ConfirmDialog kind="remove" skillRef={d.ref} onClose={closeDialog} />
    case "unlink":
      return <ConfirmDialog kind="unlink" skillRef={d.ref} onClose={closeDialog} />
    case "purge":
      return <ConfirmDialog kind="purge" repo={d.repo} onClose={closeDialog} />
    case "emptyTrash":
      return <EmptyTrashDialog onClose={closeDialog} />
  }
}
