import * as React from "react"

import { useApp, type DialogState } from "@/lib/app-state"
import { DialogClosing } from "@/lib/dialog-presence"
import { AddDialog } from "./add-dialog"
import { ApplyDialog, LibraryDialog, SyncDialog, UpdateDialog } from "./ops-dialogs"
import { ConfirmBox, ConfirmDialog, ForkDialog, NewSkillDialog } from "./skill-dialogs"

/**
 * Renders whichever dialog the app state asks for. When the state closes it,
 * the last dialog stays mounted, told to close (DialogClosing), so its exit
 * animation can play; the next dialog replaces it.
 */
export function Dialogs() {
  const { dialog, closeDialog } = useApp()
  const [shown, setShown] = React.useState<{ d: NonNullable<DialogState>; n: number } | null>(null)
  if (dialog && dialog !== shown?.d) setShown({ d: dialog, n: (shown?.n ?? 0) + 1 })
  if (!shown) return null
  return (
    <DialogClosing.Provider value={!dialog}>
      <React.Fragment key={shown.n}>{render(shown.d, closeDialog)}</React.Fragment>
    </DialogClosing.Provider>
  )
}

function render(d: NonNullable<DialogState>, closeDialog: () => void) {
  switch (d.kind) {
    case "add":
      return <AddDialog initial={d.initial} adjust={d.adjust} onClose={closeDialog} />
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
    case "confirm":
      return <ConfirmBox title={d.title} body={d.body} action={d.action} run={d.run} onClose={closeDialog} />
  }
}
