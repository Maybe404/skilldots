import * as React from "react"
import { toast } from "sonner"
import { Trash2, Undo2 } from "lucide-react"

import { useApp } from "@/lib/app-state"
import { backend, type TrashEntry } from "@/lib/backend"
import { ago } from "@/lib/i18n"
import { ExampleTag, Mono, PageHeader } from "@/components/app/bits"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"

export function TrashPage() {
  const { t, run, reload, trashCount, openDialog } = useApp()
  const [items, setItems] = React.useState<TrashEntry[]>([])
  const [deleting, setDeleting] = React.useState<TrashEntry | null>(null)

  const load = React.useCallback(async () => {
    const r = await run(() => backend.trash())
    if (r.ok) setItems(r.value)
  }, [run])
  React.useEffect(() => {
    void load()
  }, [load, trashCount])

  const restore = async (e: TrashEntry) => {
    if (!(await run(() => backend.restore(e.id))).ok) return
    toast(t("restored", { name: e.name }))
    await reload()
  }
  const remove = async (e: TrashEntry) => {
    if (!(await run(() => backend.deleteForever(e.id))).ok) return
    toast(t("deletedForever", { name: e.name }))
    setDeleting(null)
    await reload()
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <PageHeader
        title={t("trashTitle")}
        hint={t("trashHint")}
        actions={
          <Button variant="outline" size="sm" disabled={items.length === 0} onClick={() => openDialog({ kind: "emptyTrash" })}>
            <Trash2 /> {t("emptyTrash")}
          </Button>
        }
      />
      <div className="px-8 pb-8">
        {items.length === 0 ? (
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
            {items.map((e) => (
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
                <Button variant="outline" size="sm" onClick={() => restore(e)}>
                  <Undo2 /> {t("restore")}
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setDeleting(e)}>
                  {t("deleteForever")}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <AlertDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("deleteOneTitle", { name: deleting?.name ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("deleteOneBody")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel autoFocus>{t("cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={() => deleting && remove(deleting)}>{t("deleteForever")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
