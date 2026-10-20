import * as React from "react"

import { useApp, useColumnAgents } from "@/lib/app-state"
import { ago } from "@/lib/i18n"
import { Effect } from "@/components/animate-ui/primitives/effects/effect"
import { RefreshIcon, RollingText } from "@/components/app/bits"
import { Button } from "@/components/ui/button"

/** Seconds since an operation started, ticking once a second. */
export function useElapsed(startedAt: number | undefined) {
  const [elapsed, setElapsed] = React.useState(0)
  React.useEffect(() => {
    if (!startedAt) {
      setElapsed(0)
      return
    }
    const tick = () => setElapsed(Math.max(0, Math.floor((Date.now() - startedAt) / 1000)))
    tick()
    const timer = window.setInterval(tick, 1000)
    return () => window.clearInterval(timer)
  }, [startedAt])
  return elapsed
}

/** "正在更新… 12 秒": a running operation with its spinner and rolling seconds. */
export function OperationText({ label, startedAt, mutedClassName = "text-mut" }: { label: string; startedAt: number; mutedClassName?: string }) {
  const { t } = useApp()
  const elapsed = useElapsed(startedAt)
  return (
    <>
      <RefreshIcon spinning />
      {label}
      <RollingText className={mutedClassName} value={elapsed} message={(s) => t("elapsed", { s })} />
    </>
  )
}

/**
 * The Skill page's bottom bar (design: footer, 48px). Pending link changes
 * take it over with 应用 / 撤销; otherwise it shows a running operation or the
 * library's state, and the keyboard hints on the right. Each state eases in
 * (Animate UI effect: fade and a short rise; fade only with reduced motion),
 * and the pending count and the seconds roll (Animate UI sliding number).
 */
export function StatusBar() {
  const { t, pendingCount, applying, applyPending, revertPending, operation, outdated, checking, updatablePacks, status, plan, openDialog, navigate } = useApp()
  const columns = useColumnAgents()
  const keys = Math.min(9, columns.length)

  let state: string
  let left: React.ReactNode
  if (operation && (applying || pendingCount === 0)) {
    // A running operation; while the pending changes are being written it replaces the count and the greyed buttons.
    state = "operation"
    left = (
      <span className="flex items-center gap-2 text-ink" role="status" aria-live="polite">
        <OperationText label={operation.label} startedAt={operation.startedAt} />
      </span>
    )
  } else if (pendingCount > 0) {
    state = "pending"
    left = (
      <>
        <span className="flex items-center gap-2 text-ink" role="status">
          <span aria-hidden className="size-[7px] rounded-full bg-ac" />
          <RollingText value={pendingCount} message={(n) => t("pendingN", { n })} />
        </span>
        <Button variant="accent" className="px-3.5" disabled={!!operation} onClick={() => void applyPending()}>
          {t("apply")}
        </Button>
        <Button variant="quiet" size="inline" className="no-underline" disabled={!!operation} onClick={revertPending}>
          {t("revert")}
        </Button>
      </>
    )
  } else {
    state = "idle"
    const sep = <span aria-hidden> · </span>
    left = (
      <span className="flex min-w-0 items-center truncate whitespace-pre">
        <span>{checking ? t("checking") : outdated ? t("checkedAgoShort", { ago: ago(outdated.checked, t) }) : t("neverChecked")}</span>
        {/* Up to date only says something once the upstreams were checked. */}
        {updatablePacks.length > 0 && (
          <>
            {sep}
            <Button variant="quiet" size="inline" className="no-underline hover:underline" onClick={() => navigate("sources")}>
              {t("updatesAvailable", { n: updatablePacks.length })}
            </Button>
          </>
        )}
        {!updatablePacks.length && outdated && !checking && (
          <>
            {sep}
            <span>{t("allUpToDate")}</span>
          </>
        )}
        {sep}
        <span>{status?.changes ? t("unsynced", { n: status.changes }) : t("synced")}</span>
        {plan.length > 0 && (
          <>
            {sep}
            <Button variant="link" size="inline" className="fs-body" onClick={() => openDialog({ kind: "apply" })}>
              {t("driftN", { n: plan.length })}
            </Button>
          </>
        )}
      </span>
    )
  }

  return (
    <footer className="relative z-[4] flex h-[49px] shrink-0 items-center gap-3 border-t-hd bg-bg px-10 font-body fs-body text-mut doodle:pr-[150px] max-[760px]:px-5">
      <Effect key={state} fade slide={{ direction: "up", offset: 6 }} transition={{ type: "spring", stiffness: 420, damping: 34 }} className="flex min-w-0 items-center gap-3">
        {left}
      </Effect>
      <span className="flex-1" />
      <span aria-hidden className="font-mono fs-mono-xs whitespace-nowrap max-[1080px]:hidden">
        {t("keysHint", { n: keys })}
      </span>
    </footer>
  )
}
