import * as React from "react"

import { useApp, useColumnAgents } from "@/lib/app-state"
import { ago } from "@/lib/i18n"
import { RefreshIcon } from "@/components/app/bits"
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

/**
 * The Skill page's bottom bar (design: footer, 48px). Pending link changes
 * take it over with 应用 / 撤销; otherwise it shows a running operation or the
 * library's state, and the keyboard hints on the right.
 */
export function StatusBar() {
  const { t, pendingCount, applyPending, revertPending, operation, outdated, checking, updatablePacks, status, plan, openDialog, navigate } = useApp()
  const columns = useColumnAgents()
  const elapsed = useElapsed(operation?.startedAt)
  const keys = Math.min(9, columns.length)

  let left: React.ReactNode
  if (pendingCount > 0) {
    left = (
      <>
        <span className="flex items-center gap-2 text-ink" role="status">
          <span aria-hidden className="size-[7px] rounded-full bg-ac" />
          {t("pendingN", { n: pendingCount })}
        </span>
        <Button variant="accent" className="px-3.5" disabled={!!operation} onClick={() => void applyPending()}>
          {t("apply")}
        </Button>
        <Button variant="quiet" size="inline" className="no-underline" disabled={!!operation} onClick={revertPending}>
          {t("revert")}
        </Button>
      </>
    )
  } else if (operation) {
    left = (
      <span className="flex items-center gap-2 text-ink" role="status" aria-live="polite">
        <RefreshIcon spinning />
        {operation.label}
        <span className="text-mut">{t("elapsed", { s: elapsed })}</span>
      </span>
    )
  } else {
    const sep = <span aria-hidden> · </span>
    left = (
      <span className="flex min-w-0 items-center truncate whitespace-pre">
        <span>{checking ? t("checking") : outdated ? t("checkedAgoShort", { ago: ago(outdated.checked, t) }) : t("neverChecked")}</span>
        {sep}
        {updatablePacks.length ? (
          <button type="button" className="rounded-seg hover:text-ink hover:underline hover:underline-offset-[3px]" onClick={() => navigate("sources")}>
            {t("updatesAvailable", { n: updatablePacks.length })}
          </button>
        ) : (
          <span>{t("allUpToDate")}</span>
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
    <footer className="relative z-[4] flex h-[49px] shrink-0 items-center gap-3 border-t-hd bg-bg px-10 font-body fs-body text-mut max-[760px]:px-5">
      {left}
      <span className="flex-1" />
      <span aria-hidden className="font-mono fs-mono-xs whitespace-nowrap max-[1080px]:hidden">
        {t("keysHint", { n: keys })}
      </span>
    </footer>
  )
}
