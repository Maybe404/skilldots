import * as React from "react"

/**
 * False while a dialog the app state has already closed is still on screen,
 * playing its exit animation (components/dialogs/dialogs.tsx keeps the last
 * one mounted for that). ui/dialog and ui/alert-dialog read it and close.
 */
export const DialogClosing = React.createContext(false)
