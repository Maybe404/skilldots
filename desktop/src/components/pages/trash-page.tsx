import * as React from "react"
import { toast } from "sonner"

import { useApp } from "@/lib/app-state"
import { backend, type TrashEntry } from "@/lib/backend"
import { ago, type MessageKey } from "@/lib/i18n"
import { EmptyNote, ExampleTag, Page, PageError, TableHead } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

const COLS = "200px minmax(0,1fr) 110px 220px 120px"

const REASON: Record<NonNullable<TrashEntry["reason"]>, MessageKey> = {
  removed: "reasonRemoved",
  update: "reasonUpdate",
  adopt: "reasonAdopt",
  purge: "reasonPurge",
}

/** Trash: restore only. Emptying it is a terminal command (skd trash empty). */
export function TrashPage() {
  const { t, run, reload, trashCount, status } = useApp()
  const [items, setItems] = React.useState<TrashEntry[] | null>(null)
  const [failed, setFailed] = React.useState(false)

  const load = React.useCallback(async () => {
    setFailed(false)
    const r = await run(() => backend.trash())
    if (r.ok) setItems(r.value)
    else setFailed(true)
  }, [run])
  React.useEffect(() => {
    void load()
  }, [load, trashCount])

  const restore = async (e: TrashEntry) => {
    if (!(await run(() => backend.restore(e.id))).ok) return
    toast(t("restored", { name: e.name }))
    await reload()
  }

  // Older entries carry no reason: say where they came from instead.
  const why = (e: TrashEntry) => {
    if (e.reason === "update") {
      const name = e.origin.match(/\/vendor\/[^/]+\/[^/]+\/([^/]+)\//)?.[1]
      if (name) return t("reasonUpdateOf", { name })
    }
    if (e.reason) return t(REASON[e.reason])
    return status?.root && e.origin.startsWith(status.root) ? t("reasonLibrary") : t("reasonAgentDir")
  }

  return (
    <Page
      title={t("trashTitle")}
      summary={
        <>
          {t("trashRestoreOnly")}
          <code className="rounded-[4px] bg-nav px-1.5 py-px font-mono fs-meta text-ink">skd trash empty</code>
        </>
      }
    >
      {failed ? (
        <PageError onRetry={() => void load()} />
      ) : items === null ? (
        <div className="space-y-2 pt-1" role="status" aria-label={t("loading")}>
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
        </div>
      ) : items.length === 0 ? (
        <EmptyNote title={t("trashEmpty")}>{t("trashEmptyBody")}</EmptyNote>
      ) : (
        <div role="table" aria-label={t("trashTitle")}>
          <TableHead style={{ gridTemplateColumns: COLS }}>
            <div role="columnheader">{t("colName")}</div>
            <div role="columnheader">{t("from")}</div>
            <div role="columnheader">{t("colTime")}</div>
            <div role="columnheader">{t("colWhy")}</div>
            <div role="columnheader" />
          </TableHead>
          {items.map((e) => (
            <div role="row" key={e.id} className="grid h-[57px] items-center border-b-row px-2.5 hover:bg-row-hv" style={{ gridTemplateColumns: COLS }}>
              <div role="cell" className="flex min-w-0 items-center gap-2 pr-3">
                <span className="truncate font-heading fs-name font-semibold text-ink">{e.name}</span>
                {e.example && <ExampleTag />}
              </div>
              <div role="cell" className="truncate pr-3 text-left font-mono fs-meta text-mut [direction:rtl]" title={e.origin}>
                <bdi>{e.origin}</bdi>
              </div>
              <div role="cell" className="fs-meta text-mut">
                <time dateTime={e.when}>{ago(e.when, t)}</time>
              </div>
              <div role="cell" className="truncate fs-meta text-mut" title={e.problem ?? why(e)}>
                {e.restorable === false && e.problem ? e.problem : why(e)}
              </div>
              <div role="cell" className="flex justify-end">
                <Button variant="outline" size="sm" disabled={e.restorable === false} title={e.problem} onClick={() => restore(e)} aria-label={`${t("restore")} ${e.name}`}>
                  <svg aria-hidden width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M9 14 4 9l5-5" />
                    <path d="M4 9h11a5 5 0 0 1 0 10h-3" />
                  </svg>
                  {t("restore")}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </Page>
  )
}
