import * as React from "react"
import { ArrowRight, History } from "lucide-react"

import { useApp } from "@/lib/app-state"
import { backend, type HistoryEntry } from "@/lib/backend"
import { formatDate, type MessageKey } from "@/lib/i18n"
import { ExampleTag, Mono, PageHeader } from "@/components/app/bits"
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"

export function HistoryPage() {
  const { t, config, run, skills } = useApp()
  const [items, setItems] = React.useState<HistoryEntry[] | null>(null)

  // reload whenever the library changes (skills is a cheap proxy)
  React.useEffect(() => {
    let live = true
    run(() => backend.history()).then((r) => live && r.ok && setItems(r.value))
    return () => {
      live = false
    }
  }, [run, skills])

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <PageHeader title={t("historyTitle")} hint={t("historyHint")} />
      <div className="px-8 pb-10">
        {items === null ? (
          <div className="space-y-3">
            <Skeleton className="h-20" />
            <Skeleton className="h-20" />
          </div>
        ) : items.length === 0 ? (
          <Empty className="border border-dashed">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <History />
              </EmptyMedia>
              <EmptyTitle>{t("historyEmpty")}</EmptyTitle>
            </EmptyHeader>
          </Empty>
        ) : (
          <ol className="relative ml-1.5 border-l">
            {items.map((h) => (
              <li key={h.id} className="relative pb-7 pl-6 last:pb-0">
                <span className="absolute top-1.5 -left-[4.5px] size-2 rounded-full bg-foreground ring-4 ring-background" aria-hidden />
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <time dateTime={h.when} className="text-[0.8rem] text-muted-foreground">
                    {formatDate(h.when, config.lang)}
                  </time>
                  <span className="font-medium">{h.repo}</span>
                  <span className="inline-flex items-center gap-1 text-[0.78rem] text-muted-foreground">
                    <Mono>{h.from}</Mono>
                    <ArrowRight className="size-3" aria-hidden />
                    <Mono>{h.to}</Mono>
                  </span>
                  {h.example && <ExampleTag />}
                </div>
                <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-[0.82rem]">
                  {(
                    [
                      ["skillChanged", h.changed],
                      ["added", h.added],
                      ["skillGone", h.gone],
                      ["skillMoved", h.moved],
                      ["dropped", h.dropped ?? []],
                      ["skillNew", h.new ?? []],
                    ] as [MessageKey, string[]][]
                  )
                    .filter(([, list]) => list.length)
                    .map(([k, list]) => (
                      <React.Fragment key={k}>
                        <dt className="text-muted-foreground">{t(k)}</dt>
                        <dd>
                          <Mono>{list.join(", ")}</Mono>
                        </dd>
                      </React.Fragment>
                    ))}
                </dl>
                {h.commits.length > 0 && (
                  <ul className="mt-2 space-y-0.5 text-[0.8rem] text-muted-foreground">
                    {h.commits.map((c) => (
                      <li key={c.id} className="truncate">
                        <Mono className="mr-1.5">{c.id}</Mono>
                        {c.title}
                      </li>
                    ))}
                  </ul>
                )}
                {h.moreCommits && <p className="mt-2 text-[0.8rem] text-muted-foreground">{t("historyMoreCommits")}</p>}
                {h.commitsUnknown && <p className="mt-2 text-[0.8rem] text-muted-foreground">{t("historyCommitsUnknown")}</p>}
                {h.applyError && <p className="mt-2 text-[0.8rem] text-destructive">{t("historyApplyError", { error: h.applyError })}</p>}
                {!!h.omitted && <p className="mt-2 text-[0.8rem] text-muted-foreground">{t("historyOmitted", { n: h.omitted })}</p>}
                {h.backups.map((b) => (
                  <p key={b} className="mt-2 text-[0.8rem]">
                    {t("backupNote", { file: b })}
                  </p>
                ))}
              </li>
            ))}
          </ol>
        )}
      </div>
    </div>
  )
}
