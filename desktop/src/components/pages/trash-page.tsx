import * as React from "react"
import { toast } from "sonner"
import { Trash2, Undo2 } from "lucide-react"

import { useApp } from "@/lib/app-state"
import { backend, type TrashEntry } from "@/lib/backend"
import { ago } from "@/lib/i18n"
import { ExampleTag, Mono, PageError, PageHeader } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"

export function TrashPage() {
  const { t, run, reload, trashCount } = useApp()
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

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <PageHeader
        title={t("trashTitle")}
        hint={t("trashHint")}
      />
      <div className="px-8 pb-8">
        {failed ? <PageError onRetry={() => void load()} /> : items === null ? (
          <div className="space-y-3">
            <Skeleton className="h-16" />
            <Skeleton className="h-16" />
          </div>
        ) : items.length === 0 ? (
          <Empty className="border border-dashed">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Trash2 />
              </EmptyMedia>
              <EmptyTitle>{t("trashEmpty")}</EmptyTitle>
            </EmptyHeader>
          </Empty>
        ) : (
          <ul className="divide-y rounded-xl border">
            {items?.map((e) => (
              <li key={e.id} className="flex items-center gap-4 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{e.name}</span>
                    {e.example && <ExampleTag />}
                    <span className="text-[0.78rem] text-muted-foreground">{ago(e.when, t)}</span>
                  </div>
                  <p className="mt-0.5 truncate text-[0.8rem] text-muted-foreground">
                    {t("from")} <Mono>{e.origin}</Mono>
                  </p>
                </div>
                <Button variant="outline" size="sm" disabled={e.restorable === false} title={e.problem} onClick={() => restore(e)}>
                  <Undo2 /> {t("restore")}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
