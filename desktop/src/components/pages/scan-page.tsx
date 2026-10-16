import * as React from "react"
import { toast } from "sonner"
import { CheckCheck, Loader2, ScanSearch } from "lucide-react"

import { useApp } from "@/lib/app-state"
import { backend, type ScanItem } from "@/lib/backend"
import type { MessageKey } from "@/lib/i18n"
import { Mono, PageHeader } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Empty, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"

const STATE: Record<ScanItem["state"], MessageKey> = {
  copy: "stateCopy",
  link: "stateLink",
  broken: "stateBroken",
  app: "stateApp",
  unrecorded: "stateUnrecorded",
  managed: "stateUnrecorded",
  other: "stateCopy",
}

export function ScanPage() {
  const { t, run, reload, agents } = useApp()
  const [items, setItems] = React.useState<ScanItem[] | null>(null)
  const [scanning, setScanning] = React.useState(false)
  const [picked, setPicked] = React.useState<Set<string>>(new Set())
  const [busy, setBusy] = React.useState(false)
  const label = (n: string) => agents.find((a) => a.name === n)?.label ?? n

  const scan = React.useCallback(async () => {
    setScanning(true)
    const r = await run(() => backend.scan())
    setScanning(false)
    if (!r.ok) return
    setItems(r.value)
    setPicked(new Set(r.value.filter((x) => x.proposal !== "skip").map((x) => x.id)))
  }, [run])

  React.useEffect(() => {
    void scan()
  }, [scan])

  const proposal = (it: ScanItem) => {
    switch (it.proposal) {
      case "pack":
        return t("propPack", { repo: it.repo ?? "" })
      case "own":
        return t("propOwn")
      case "enable":
        return t("propEnable", { agent: label(it.agent) })
      case "drop":
        return t("propDrop")
      case "app":
        return t("propApp")
      default:
        return `${t("propSkip")}: ${it.label}`
    }
  }

  const adopt = async () => {
    setBusy(true)
    const r = await run(() => backend.adopt([...picked]))
    setBusy(false)
    if (!r.ok) return
    toast(t("adoptedToast", { n: r.value.adopted }))
    await reload()
    await scan()
  }

  const byAgent = React.useMemo(() => {
    const m = new Map<string, ScanItem[]>()
    for (const it of items ?? []) m.set(it.agent, [...(m.get(it.agent) ?? []), it])
    return [...m.entries()]
  }, [items])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        title={t("scanTitle")}
        hint={t("scanHint")}
        actions={
          <Button variant="outline" size="sm" onClick={scan} disabled={scanning}>
            <ScanSearch /> {items ? t("rescan") : t("scan")}
          </Button>
        }
      />
      <div className="min-h-0 flex-1 overflow-y-auto px-8 pb-6">
        {scanning && (
          <p className="flex items-center gap-2 text-sm text-muted-foreground" aria-live="polite">
            <Loader2 className="size-4 animate-spin motion-reduce:animate-none" /> {t("scanning")}
          </p>
        )}
        {!scanning && items && items.length === 0 && (
          <Empty className="border border-dashed">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <CheckCheck />
              </EmptyMedia>
              <EmptyTitle>{t("scanEmpty")}</EmptyTitle>
            </EmptyHeader>
          </Empty>
        )}
        {!scanning &&
          byAgent.map(([agent, list]) => (
            <section key={agent} className="mb-6">
              <h2 className="mb-2 text-sm font-semibold">{label(agent)}</h2>
              <ul className="divide-y rounded-xl border">
                {list.map((it) => {
                  const id = `scan-${it.id}`
                  return (
                    <li key={it.id} className="flex items-start gap-3 px-4 py-3">
                      <Checkbox
                        id={id}
                        className="mt-0.5"
                        disabled={it.proposal === "skip"}
                        checked={picked.has(it.id)}
                        onCheckedChange={(v) =>
                          setPicked((s) => {
                            const n = new Set(s)
                            if (v) n.add(it.id)
                            else n.delete(it.id)
                            return n
                          })
                        }
                      />
                      <label htmlFor={id} className="grid min-w-0 flex-1 gap-0.5 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] sm:gap-6">
                        <span className="min-w-0">
                          <span className="font-medium">{it.name}</span>
                          <span className="ml-2 text-[0.8rem] text-muted-foreground">{t(STATE[it.state])}</span>
                          <Mono className="block truncate text-muted-foreground">
                            {it.path}
                            {it.linkTo && ` → ${it.linkTo}`}
                          </Mono>
                        </span>
                        <span className="text-[0.85rem]">{proposal(it)}</span>
                      </label>
                    </li>
                  )
                })}
              </ul>
            </section>
          ))}
      </div>
      {items && items.length > 0 && (
        <div className="flex items-center justify-between gap-4 border-t px-8 py-3">
          <span className="text-[0.8rem] text-muted-foreground">{t("adoptNote")}</span>
          <Button onClick={adopt} disabled={busy || picked.size === 0}>
            {busy && <Loader2 className="animate-spin motion-reduce:animate-none" />}
            {t("adoptSelected", { n: picked.size })}
          </Button>
        </div>
      )}
    </div>
  )
}
