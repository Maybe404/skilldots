import * as React from "react"
import { toast } from "sonner"
import { Trash2, Undo2 } from "lucide-react"

import { absentFor, toastAbsent, useApp } from "@/lib/app-state"
import { tildify } from "@/lib/agents"
import { backend, type TrashEntry } from "@/lib/backend"
import { ago, formatBytes, type MessageKey } from "@/lib/i18n"
import { EmptyNote, ExampleTag, Hint, Page, PageError, TableHead } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

const COLS = "var(--cols)"

// Why an entry is in the trash: the reasons skd records (remove, purge, …) and the sample's older names.
const REASON: Record<Exclude<NonNullable<TrashEntry["reason"]>, "">, MessageKey> = {
  removed: "reasonRemoved",
  remove: "reasonRemoved",
  update: "reasonUpdate",
  replaced: "reasonUpdate",
  merged: "reasonMerged",
  "resolve-aborted": "reasonResolveAborted",
  adopt: "reasonAdopt",
  "in-the-way": "reasonInTheWay",
  purge: "reasonPurge",
  deselected: "reasonDeselected",
  broken: "reasonBroken",
}

/**
 * Trash: restore an entry, or delete it for good. Deleting (one entry or
 * all) is the app's only permanent deletion: it always asks first, saying
 * how many entries and how much space, and the backend refuses when the
 * trash changed after that was shown.
 */
export function TrashPage() {
  const { t, run, read, reload, trashCount, status, confirm, agents } = useApp()
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

  // The backend puts back what remove or purge took out of the library and links it again in one go.
  const restore = async (e: TrashEntry) => {
    const r = await run(() => backend.restore(e.id))
    if (!r.ok) return
    const o = r.value
    const name = o?.skills?.length ? o.skills.join(t("listSep")) : e.name
    toast(o?.unused ? t("restoredUnused", { name }) : o?.linked ? t("restoredLinked", { name }) : t("restored", { name }))
    toastAbsent(t, agents, absentFor(o?.absent, o?.skills ?? []))
    await reload()
  }

  const emptyAll = async () => {
    // Read now, not from the list on screen: the confirmation names exactly what goes, and the
    // backend deletes only if the trash still holds exactly these entries.
    const u = await read(() => Promise.all([backend.trash(), backend.trashUsage()]))
    if (!u.ok || !u.value[0].length) return
    const ids = u.value[0].map((e) => e.id)
    const { bytes } = u.value[1]
    setItems(u.value[0])
    confirm({
      title: t("emptyTrashTitle", { n: ids.length }),
      body: t("emptyTrashBody", { size: formatBytes(bytes) }),
      action: t("deleteForever"),
      run: async () => {
        const r = await run(() => backend.emptyTrash({ ids }))
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
  const holds = (e: TrashEntry) => (e.skills?.length && !(e.skills.length === 1 && e.skills[0] === e.name) ? t("trashHolds", { names: e.skills.join(t("listSep")) }) : "")

  const why = (e: TrashEntry) => {
    if (e.reason === "update" || e.reason === "replaced") {
      const name = e.skills?.[0] ?? e.origin.match(/\/vendor\/[^/]+\/[^/]+\/([^/]+)\//)?.[1]
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
            <div role="row" key={e.id} className="grid min-h-[57px] items-center border-b-row px-2.5 py-2 hover:bg-row-hv" style={{ gridTemplateColumns: COLS }}>
              <div role="cell" className="min-w-0 pr-3">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-heading fs-name font-semibold text-ink">{e.name}</span>
                  {e.example && <ExampleTag />}
                </div>
                {/* What it holds, when that is more than its folder's name says (a whole source, a renamed folder). */}
                {(holds(e) || e.repo) && (
                  <p className="mt-0.5 truncate fs-meta text-mut" title={[holds(e), e.repo && t("trashFromRepo", { repo: e.repo })].filter(Boolean).join(" · ")}>
                    {[holds(e), e.repo && t("trashFromRepo", { repo: e.repo })].filter(Boolean).join(" · ")}
                  </p>
                )}
              </div>
              <div role="cell" className="truncate pr-3 font-mono fs-meta text-mut" title={tildify(e.origin)}>
                {tildify(e.origin)}
              </div>
              <div role="cell" className="fs-meta text-mut">
                <time dateTime={e.when}>{ago(e.when, t)}</time>
              </div>
              <div role="cell" className="line-clamp-2 pr-2 fs-meta leading-snug text-mut" title={e.problem ?? why(e)}>
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
