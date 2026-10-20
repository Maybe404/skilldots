import * as React from "react"
import { toast } from "sonner"
import { Trash2, Undo2 } from "lucide-react"

import { useApp } from "@/lib/app-state"
import { backend, type TrashEntry } from "@/lib/backend"
import { ago, formatBytes, type MessageKey } from "@/lib/i18n"
import { EmptyNote, ExampleTag, Hint, Page, PageError, TableHead } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

const COLS = "var(--cols)"

const REASON: Record<NonNullable<TrashEntry["reason"]>, MessageKey> = {
  removed: "reasonRemoved",
  update: "reasonUpdate",
  adopt: "reasonAdopt",
  purge: "reasonPurge",
}

/**
 * Trash: restore an entry, or delete it for good. Deleting (one entry or
 * all) is the app's only permanent deletion: it always asks first, saying
 * how many entries and how much space, and the backend refuses when the
 * trash changed after that was shown.
 */
export function TrashPage() {
  const { t, run, read, reload, trashCount, status, confirm } = useApp()
  const [items, setItems] = React.useState<TrashEntry[] | null>(null)
  const [bytes, setBytes] = React.useState(0)
  const [failed, setFailed] = React.useState(false)

  const load = React.useCallback(async () => {
    setFailed(false)
    const r = await read(() => Promise.all([backend.trash(), backend.trashUsage()]))
    if (r.ok) {
      setItems(r.value[0])
      setBytes(r.value[1].bytes)
    } else setFailed(true)
  }, [read])
  React.useEffect(() => {
    void load()
  }, [load, trashCount])

  const restore = async (e: TrashEntry) => {
    if (!(await run(() => backend.restore(e.id))).ok) return
    toast(t("restored", { name: e.name }))
    await reload()
  }

  const emptyAll = async () => {
    // Counted now, not from the list on screen: the confirmation names exactly what goes.
    const u = await read(() => backend.trashUsage())
    if (!u.ok || !u.value.count) return
    const { count, bytes } = u.value
    confirm({
      title: t("emptyTrashTitle", { n: count }),
      body: t("emptyTrashBody", { size: formatBytes(bytes) }),
      action: t("deleteForever"),
      run: async () => {
        const r = await run(() => backend.emptyTrash({ count }))
        if (!r.ok) return false
        toast(t("emptiedToast", { n: r.value.deleted }))
        await reload()
        await load()
      },
    })
  }

  const deleteOne = (e: TrashEntry) =>
    confirm({
      title: t("deleteEntryTitle", { name: e.name }),
      body: t("deleteEntryBody", { size: formatBytes(e.size ?? 0) }),
      action: t("deleteForever"),
      run: async () => {
        if (!(await run(() => backend.deleteTrash(e.id))).ok) return false
        toast(t("deletedToast", { name: e.name }))
        await reload()
        await load()
      },
    })

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
      summary={items?.length ? t("trashSummary", { n: items.length, size: formatBytes(bytes) }) : undefined}
      actions={
        items?.length ? (
          <Button variant="outline" onClick={() => void emptyAll()}>
            <Trash2 aria-hidden strokeWidth={1.8} />
            {t("emptyTrash")}
          </Button>
        ) : undefined
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
        <div role="table" aria-label={t("trashTitle")} className="skd-cols-trash">
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
              <div role="cell" className="flex justify-end gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={e.restorable === false}
                  title={e.problem}
                  onClick={() => restore(e)}
                  aria-label={`${t("restore")} ${e.name}`}
                  className="max-[999px]:w-[26px] max-[999px]:px-0"
                >
                  <Undo2 aria-hidden strokeWidth={1.8} />
                  <span className="max-[999px]:hidden">{t("restore")}</span>
                </Button>
                <Hint label={t("deleteForever")}>
                  <Button variant="ghost" size="icon-sm" disabled={e.unsafe} onClick={() => deleteOne(e)} aria-label={`${t("deleteForever")} ${e.name}`}>
                    <Trash2 aria-hidden strokeWidth={1.8} />
                  </Button>
                </Hint>
              </div>
            </div>
          ))}
        </div>
      )}
    </Page>
  )
}
