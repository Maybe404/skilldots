import * as React from "react"
import { MotionConfig } from "motion/react"
import { isMyGo } from "mygo-runtime"

import { AppProvider, useApp } from "@/lib/app-state"
import { CommandPalette } from "@/components/app/command-palette"
import { AppSidebar, PendingBanner, Toolbar, searchInputId } from "@/components/app/shell"
import { Dialogs } from "@/components/dialogs/dialogs"
import { AgentsPage } from "@/components/pages/agents-page"
import { HistoryPage } from "@/components/pages/history-page"
import { Onboarding } from "@/components/pages/onboarding"
import { ScanPage } from "@/components/pages/scan-page"
import { SettingsPage } from "@/components/pages/settings-page"
import { SkillsPage } from "@/components/pages/skills-page"
import { SourcesPage } from "@/components/pages/sources-page"
import { TrashPage } from "@/components/pages/trash-page"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { Button } from "@/components/ui/button"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"

// CodeMirror and the Markdown renderer load only when a skill is opened.
const EditorPage = React.lazy(() => import("@/components/editor/editor-page").then((m) => ({ default: m.EditorPage })))

export default function App() {
  return (
    <AppProvider>
      <MotionConfig reducedMotion="user">
        <TooltipProvider delay={400}>
          <Root />
          <Toaster position="bottom-right" />
        </TooltipProvider>
      </MotionConfig>
    </AppProvider>
  )
}

function Root() {
  const { ready, status, loadError, t } = useApp()
  if (loadError) return <main className="flex min-h-svh flex-col items-center justify-center gap-4 p-8"><h1>{t("loadFailed")}</h1><p className="max-w-prose text-center text-sm text-muted-foreground" role="alert">{loadError}</p><Button onClick={() => window.location.reload()}>{t("retry")}</Button></main>
  if (!ready) return <main className="flex min-h-svh items-center justify-center p-8"><div className="w-full max-w-xl space-y-4" role="status" aria-live="polite"><div className="h-8 w-40 animate-pulse rounded-md bg-muted motion-reduce:animate-none" /><div className="h-24 animate-pulse rounded-xl border bg-muted/50 motion-reduce:animate-none" /><p className="text-sm text-muted-foreground">{t("loadingLibrary")}</p></div></main>
  if (!status?.root) return <Onboarding />
  return <>{!isMyGo() && <div className="border-b bg-muted/70 px-8 py-1.5 text-[0.78rem] text-muted-foreground" role="status">{t("exampleMode")}</div>}<Shell /></>
}

function Shell() {
  const { view, editing, setPaletteOpen, paletteOpen, openDialog, navigate, dialog } = useApp()

  // Global shortcuts: ⌘K palette, ⌘N new skill, ⌘, settings, ⌘F search.
  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey) return
      const k = e.key.toLowerCase()
      if (k === "k") {
        e.preventDefault()
        setPaletteOpen(!paletteOpen)
      } else if (k === "n" && !dialog) {
        e.preventDefault()
        openDialog({ kind: "new" })
      } else if (k === "," && !dialog) {
        e.preventDefault()
        navigate("settings")
      } else if (k === "f" && view !== "editor" && !dialog) {
        e.preventDefault()
        document.getElementById(searchInputId)?.focus()
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [paletteOpen, setPaletteOpen, openDialog, navigate, view, dialog])

  const page = (() => {
    switch (view) {
      case "skills":
        return <SkillsPage />
      case "sources":
        return <SourcesPage />
      case "agents":
        return <AgentsPage />
      case "history":
        return <HistoryPage />
      case "scan":
        return <ScanPage />
      case "trash":
        return <TrashPage />
      case "settings":
        return <SettingsPage />
      default:
        return null
    }
  })()

  return (
    <SidebarProvider className="h-svh">
      <AppSidebar />
      <SidebarInset className="h-[calc(100svh-1rem)] min-h-0 overflow-hidden">
        {view === "editor" && editing ? (
          <React.Suspense fallback={null}>
            <EditorPage skillRef={editing} />
          </React.Suspense>
        ) : (
          <>
            <Toolbar />
            <PendingBanner />
            <div className="flex min-h-0 flex-1 flex-col">{page}</div>
          </>
        )}
      </SidebarInset>
      <Dialogs />
      <CommandPalette />
    </SidebarProvider>
  )
}
