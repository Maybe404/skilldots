import * as React from "react"

import { useApp } from "@/lib/app-state"
import { backend, type HistoryEntry } from "@/lib/backend"
import { formatDate, type MessageKey } from "@/lib/i18n"
import { shortSha } from "@/lib/utils"
import { EmptyNote, ExampleTag, Page, PageError, Sha, TableHead, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

const COLS = "var(--cols)"

/** Update history kept in the library (design: one row per update). */
export function HistoryPage() {
  const { t, read, skills } = useApp()
  const [items, setItems] = React.useState<HistoryEntry[] | null>(null)
  const [failed, setFailed] = React.useState(false)

  const load = React.useCallback(async () => {
    setFailed(false)
    const r = await read(() => backend.history())
    if (r.ok) setItems(r.value)
    else setFailed(true)
  }, [read])

  // Reload whenever the library changes (skills is a cheap proxy).
  React.useEffect(() => {
    void load()
  }, [load, skills])

  return (
    <Page title={t("historyTitle")} summary={t("historyHint")}>
      {failed ? (
        <PageError onRetry={() => void load()} />
      ) : items === null ? (
        <div className="space-y-3 pt-2" role="status" aria-label={t("loading")}>
          <Skeleton className="h-16" />
          <Skeleton className="h-16" />
        </div>
      ) : items.length === 0 ? (
        <EmptyNote title={t("historyEmpty")}>{t("historyEmptyBody")}</EmptyNote>
      ) : (
        // One grid for the whole table (rows are subgrids), so the source column fits its longest name.
        <div role="table" aria-label={t("historyTitle")} className="skd-cols-history grid" style={{ gridTemplateColumns: COLS }}>
          <TableHead className="col-span-full grid-cols-subgrid">
            <div role="columnheader">{t("colTime")}</div>
            <div role="columnheader">{t("colSource")}</div>
            <div role="columnheader">{t("colVersion")}</div>
            <div role="columnheader">{t("colWhat")}</div>
          </TableHead>
          {items.map((h) => (
            <Row key={h.id} h={h} />
          ))}
        </div>
      )}
    </Page>
  )
}

function Row({ h }: { h: HistoryEntry }) {
  const { t, config, navigate } = useApp()
  const fresh = Date.now() - new Date(h.when).getTime() < 15 * 60 * 1000
  const kinds = (
    [
      ["skillChanged", h.changed],
      ["added", h.added],
      ["skillNew", h.new ?? []],
      ["skillGone", h.gone],
      ["skillMoved", h.moved],
      ["dropped", h.dropped ?? []],
    ] as [MessageKey, string[]][]
  ).filter(([, l]) => l.length)

  return (
    <div role="row" className="col-span-full grid grid-cols-subgrid items-start border-b-row px-2.5 py-3 hover:bg-row-hv">
      <div role="cell" className="flex items-center gap-2 pt-0.5 pr-3 fs-meta text-mut tabular-nums">
        <time dateTime={h.when}>{formatDate(h.when, config.lang)}</time>
        {fresh && <UpdateDot label={t("fresh")} />}
      </div>
      <div role="cell" className="flex min-w-0 items-center gap-2 pr-3">
        <span className="truncate font-heading fs-name font-semibold text-ink">{h.repo}</span>
        {h.example && <ExampleTag />}
      </div>
      <div role="cell" className="min-w-0 pt-0.5 font-mono fs-meta text-mut" title={`${h.from} → ${h.to}`}>
        {shortSha(h.from)} → {shortSha(h.to)}
      </div>
      <div role="cell" className="min-w-0">
        {kinds.map(([k, list]) => (
          <div key={k} className="flex gap-2 fs-body">
            <span className="flex-none text-mut">{t(k)}</span>
            <span className="pt-px fs-meta text-hue-ink">{list.join(", ")}</span>
          </div>
        ))}
        {h.commits.length > 0 && (
          <div className="mt-1.5 flex flex-col gap-[3px]">
            {h.commits.map((c) => (
              <div key={c.id} className="flex gap-3 fs-meta">
                <Sha id={c.id} className="w-16 flex-none text-mut" />
                <span className="min-w-0 text-ink">{c.title}</span>
              </div>
            ))}
          </div>
        )}
        {h.moreCommits && <p className="mt-1.5 fs-meta text-mut">{t("historyMoreCommits")}</p>}
        {h.commitsUnknown && <p className="mt-1.5 fs-meta text-mut">{t("historyCommitsUnknown")}</p>}
        {!!h.omitted && <p className="mt-1.5 fs-meta text-mut">{t("historyOmitted", { n: h.omitted })}</p>}
        {h.applyError && (
          <p className="mt-1.5 flex items-center gap-2 fs-meta text-ink" role="note">
            <UpdateDot decorative />
            {t("historyApplyError", { error: h.applyError })}
          </p>
        )}
        {h.backups.map((b) => (
          <p key={b} className="mt-1.5 flex flex-wrap items-center gap-x-2 fs-meta text-ink">
            <UpdateDot decorative />
            {t("backupNote", { file: b })}
            <Button variant="link" size="inline" className="fs-meta text-ink" onClick={() => navigate("trash")}>
              {t("goTrash")}
            </Button>
          </p>
        ))}
      </div>
    </div>
  )
}
