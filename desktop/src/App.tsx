import * as React from "react"
import { MotionConfig } from "motion/react"
import { isMyGo } from "mygo-runtime"

import { AppProvider, useApp } from "@/lib/app-state"
import { CommandPalette } from "@/components/app/command-palette"
import { AppSidebar, OperationBanner, PendingBanner, Toolbar, searchInputId } from "@/components/app/shell"
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
  const { ready, status, loadError, loadStage, retryLoad, t } = useApp()
  if (loadError) {
    const stageLabel = loadError.stage === "config" ? t("loadingConfig") : loadError.stage === "checking" ? t("loadingCheck") : t("loadingLibrary")
    return (
      <main className="flex min-h-svh flex-col items-center justify-center gap-4 p-8">
        <div className="w-full max-w-xl space-y-4 rounded-2xl border bg-background/80 p-7 shadow-sm">
          <div>
            <h1 className="text-xl font-semibold">{t("loadFailed")}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{stageLabel}</p>
          </div>
          <p className="text-sm text-muted-foreground" role="alert">{t("loadErrorHint")}</p>
          <details className="rounded-lg border bg-muted/40 px-3 py-2 text-sm">
            <summary className="cursor-pointer font-medium">{t("showDetails")}</summary>
            <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-words text-xs text-muted-foreground">{loadError.detail}</pre>
          </details>
          <Button onClick={retryLoad}>{t("retry")}</Button>
        </div>
      </main>
    )
  }
  if (!ready) {
    const message = loadStage === "config" ? t("loadingConfig") : loadStage === "checking" ? t("loadingCheck") : t("loadingLibrary")
    return <main className="flex min-h-svh items-center justify-center p-8"><div className="w-full max-w-xl space-y-4" role="status" aria-live="polite"><div className="h-8 w-40 animate-pulse rounded-md bg-muted motion-reduce:animate-none" /><div className="h-24 animate-pulse rounded-xl border bg-muted/50 motion-reduce:animate-none" /><p className="text-sm text-muted-foreground">{message}</p></div></main>
  }
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
      } else if (k === "f" && view === "skills" && !dialog) {
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
            <OperationBanner />
            <div className="flex min-h-0 flex-1 flex-col">{page}</div>
          </>
        )}
      </SidebarInset>
      <Dialogs />
      <CommandPalette />
    </SidebarProvider>
  )
}
