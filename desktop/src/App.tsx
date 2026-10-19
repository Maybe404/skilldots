import * as React from "react"
import { AnimatePresence, MotionConfig } from "motion/react"

import { AppProvider, useApp } from "@/lib/app-state"
import { Effect } from "@/components/animate-ui/primitives/effects/effect"
import { CommandPalette } from "@/components/app/command-palette"
import { DoodleDefs, Doodles } from "@/components/app/doodles"
import { Header, searchInputId } from "@/components/app/header"
import { RefreshIcon } from "@/components/app/bits"
import { OperationText } from "@/components/app/status-bar"
import { Dialogs } from "@/components/dialogs/dialogs"
import { AgentsPage } from "@/components/pages/agents-page"
import { HistoryPage } from "@/components/pages/history-page"
import { Onboarding } from "@/components/pages/onboarding"
import { ScanPage } from "@/components/pages/scan-page"
import { SettingsPage } from "@/components/pages/settings-page"
import { SourcesPage } from "@/components/pages/sources-page"
import { TrashPage } from "@/components/pages/trash-page"
import { rowId } from "@/components/skills/model"
import { SkillSheet } from "@/components/skills/skill-sheet"
import { SkillsPage } from "@/components/skills/skills-page"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"

// CodeMirror and the Markdown renderer load only when a skill is opened.
const EditorPage = React.lazy(() => import("@/components/editor/editor-page").then((m) => ({ default: m.EditorPage })))

export default function App() {
  return (
    <AppProvider>
      <MotionConfig reducedMotion="user">
        <TooltipProvider delay={400}>
          <DoodleDefs />
          <Root />
          <Toaster />
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
      <main className="flex min-h-svh items-center justify-center p-8">
        {/* No top bar here: a strip as tall as the window's title bar still drags the window. */}
        <div aria-hidden className="skd-titlebar fixed inset-x-0 top-0 h-[var(--mygo-titlebar-height,0px)]" />
        <div className="w-full max-w-xl rounded-card border-card bg-dialog px-6 py-[22px] shadow-pop" role="alert">
          <h1 className="font-heading fs-h3 font-bold text-ink">{t("loadFailed")}</h1>
          <p className="mt-1 fs-meta text-mut">{stageLabel}</p>
          <p className="mt-3 fs-body leading-relaxed text-mut">{t("loadErrorHint")}</p>
          <details className="mt-4 rounded-ctl bg-nav px-3 py-2 fs-meta">
            <summary className="cursor-pointer font-medium text-ink">{t("showDetails")}</summary>
            <pre className="mt-2 max-h-40 overflow-auto font-mono break-words whitespace-pre-wrap text-mut">{loadError.detail}</pre>
          </details>
          <div className="mt-[22px] flex justify-end">
            <Button variant="ink" size="lg" onClick={retryLoad}>
              {t("retry")}
            </Button>
          </div>
        </div>
      </main>
    )
  }
  if (!ready) {
    const message = loadStage === "config" ? t("loadingConfig") : loadStage === "checking" ? t("loadingCheck") : t("loadingLibrary")
    return (
      <main className="flex min-h-svh flex-col">
        <div className="skd-titlebar flex h-[54px] items-center gap-2.5 pr-6">
          <span className="font-heading fs-brand leading-none font-semibold text-ink">skilldots</span>
        </div>
        <div className="px-10 pt-[26px]" role="status" aria-live="polite">
          <p className="mb-5 flex items-center gap-2 fs-body text-mut">
            <RefreshIcon spinning />
            {message}
          </p>
          <div className="w-[400px] max-w-full space-y-2">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-[50px] rounded-card" />
            ))}
          </div>
        </div>
      </main>
    )
  }
  if (!status?.root) return <Onboarding />
  return <Shell />
}

/**
 * Shows a long operation on pages without the Skill page's status bar. It
 * pops up from the bottom and sinks away when the operation ends (Animate UI
 * effect under AnimatePresence; fade only with reduced motion).
 */
function OperationPill() {
  const { operation: current, view } = useApp()
  // The pill keeps the last label while it sinks away after the operation ended.
  const [operation, setOperation] = React.useState(current)
  if (current && current !== operation) setOperation(current)
  const show = !!current && view !== "skills"
  return (
    <AnimatePresence>
      {show && operation && (
        <Effect
          key="operation"
          role="status"
          aria-live="polite"
          fade
          zoom={{ initialScale: 0.92 }}
          slide={{ direction: "up", offset: 14 }}
          transition={{ type: "spring", stiffness: 420, damping: 32 }}
          className="fixed bottom-4 left-1/2 z-30 flex h-9 -translate-x-1/2 items-center gap-2.5 rounded-[18px] bg-ink px-4 font-body fs-body whitespace-nowrap text-bg shadow-toast"
        >
          <OperationText label={operation.label} startedAt={operation.startedAt} mutedClassName="text-bg/70" />
        </Effect>
      )}
    </AnimatePresence>
  )
}

function Shell() {
  const { view, editing, setPaletteOpen, paletteOpen, openDialog, navigate, dialog, selected } = useApp()

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
      } else if (k === "f" && !dialog && view !== "editor") {
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
      case "editor":
        return editing ? (
          <React.Suspense fallback={null}>
            <EditorPage skillRef={editing} />
          </React.Suspense>
        ) : null
    }
  })()

  return (
    <div className="relative flex h-svh min-w-0 flex-col overflow-hidden">
      <Doodles />
      <Header />
      <div className="flex min-h-0 flex-1 flex-col">{page}</div>
      <OperationPill />
      {/* Skill details open over any page (the Skill page, or a skill link on Sources); closing returns focus to the skill's row when it is on screen. */}
      <SkillSheet returnFocus={() => (selected ? document.getElementById(rowId(selected)) : null)} />
      <Dialogs />
      <CommandPalette />
    </div>
  )
}
