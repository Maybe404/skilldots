import * as React from "react"
import { toast } from "sonner"
import { ArrowRight } from "lucide-react"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import { agentLabel, tildify } from "@/lib/agents"
import { backend, type ScanItem } from "@/lib/backend"
import type { MessageKey } from "@/lib/i18n"
import { AgentMono, EmptyNote, Page, PageError, RefreshIcon, TableHead } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Skeleton } from "@/components/ui/skeleton"

const COLS = "var(--cols)"

const STATE: Record<ScanItem["state"], MessageKey> = {
  copy: "stateCopy",
  link: "stateLink",
  broken: "stateBroken",
  app: "stateApp",
  unrecorded: "stateUnrecorded",
  managed: "stateUnrecorded",
  left: "stateUnrecorded",
  other: "stateCopy",
}

/** 收编: scan the agent folders and adopt what skilldots does not manage yet. */
export function ScanPage() {
  const { t, runTask, reload, agents, skills, confirm, navigate } = useApp()
  const [items, setItems] = React.useState<ScanItem[] | null>(null)
  const [failed, setFailed] = React.useState(false)
  const [scanning, setScanning] = React.useState(false)
  const [picked, setPicked] = React.useState<Set<string>>(new Set())
  /** How many the last adopt took in: the page then offers the way back to the Skill page. */
  const [done, setDone] = React.useState<number | null>(null)

  const scan = React.useCallback(
    async (announce: boolean) => {
      setScanning(true)
      setFailed(false)
      const r = await runTask(t("scanning"), () => backend.scan())
      setScanning(false)
      if (!r.ok) {
        setFailed(true)
        return
      }
      setItems(r.value)
      setPicked(new Set(r.value.filter((x) => x.proposal !== "skip").map((x) => x.id)))
      if (announce) setDone(null)
      if (announce) toast(r.value.length ? t("scanDone", { n: r.value.length }) : t("scanDoneNone"))
    },
    [runTask, t],
  )

  React.useEffect(() => {
    void scan(false)
  }, [scan])

  const label = (n: string) => {
    const a = agents.find((x) => x.name === n)
    return a ? agentLabel(a) : n
  }
  const proposal = (it: ScanItem) => {
    switch (it.proposal) {
      case "pack":
        return t("propPack", { repo: it.repo ?? "", name: it.name })
      case "own":
        // Changed since it was installed from upstream: kept as yours; the upstream version is an explicit add.
        if (it.differs && it.upstream) return t("propOwnDiffers", { repo: it.upstream })
        return skills.some((s) => s.linkName === it.name) ? t("propOwnRename") : t("propOwn")
      case "enable":
        // The agent already reads the library's copy from ~/.agents/skills: this one is a second copy it would load.
        if (it.sharedCopy) return t("propSharedCopy", { agent: label(it.agent), name: it.name })
        return t("propEnable", { agent: label(it.agent), name: it.name })
      case "drop":
        return t("propDrop")
      case "app":
        return t("propApp")
      case "skip":
        return `${t("propSkip")}: ${it.label}`
      default:
        // A kind this app does not know yet: the core's own words.
        return it.label
    }
  }

  // Grouped by agent, in registry order.
  const groups = React.useMemo(() => {
    const order = agents.map((a) => a.name)
    const m = new Map<string, ScanItem[]>()
    for (const it of items ?? []) m.set(it.agent, [...(m.get(it.agent) ?? []), it])
    return [...m.entries()].sort(([a], [b]) => order.indexOf(a) - order.indexOf(b))
  }, [items, agents])

  const selectable = (items ?? []).filter((x) => x.proposal !== "skip")
  const n = picked.size
  const all = selectable.length > 0 && selectable.every((x) => picked.has(x.id))
  const some = selectable.some((x) => picked.has(x.id))
  const toggle = (id: string, on: boolean) =>
    setPicked((s) => {
      const next = new Set(s)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })

  const adopt = () => {
    const chosen = (items ?? []).filter((x) => picked.has(x.id))
    // The table already says what happens to each one: the confirmation only counts them by kind.
    const kindOf = (x: ScanItem) => (x.proposal === "enable" && x.sharedCopy ? "second" : x.proposal)
    const parts = (
      [
        ["pack", "adoptPartPack"],
        ["own", "adoptPartOwn"],
        ["enable", "adoptPartEnable"],
        ["second", "adoptPartSecond"],
        ["drop", "adoptPartDrop"],
        ["app", "adoptPartApp"],
      ] as const
    )
      .map(([kind, key]) => [chosen.filter((x) => kindOf(x) === kind).length, key] as const)
      .filter(([n]) => n > 0)
      .map(([n, key]) => t(key, { n }))
    // Kinds this app does not know yet still count.
    const other = chosen.filter((x) => !["pack", "own", "enable", "drop", "app", "skip"].includes(x.proposal)).length
    if (other) parts.push(t("adoptPartOther", { n: other }))
    confirm({
      title: t("adoptTitle", { n: chosen.length }),
      body: t("adoptConfirmBody", { parts: parts.join(t("listSep")) }),
      action: t("adoptAction"),
      run: async () => {
        // Each row with the kind shown here: the backend refuses a row whose proposal changed since.
        const r = await runTask(t("adopting"), () => backend.adopt(chosen.map((x) => ({ id: x.id, kind: x.proposal }))))
        if (!r.ok) return false
        toast(t("adoptedToast", { n: r.value.adopted }))
        await reload()
        await scan(false)
        setDone(r.value.adopted)
      },
    })
  }

  const summary = scanning
    ? t("scanning")
    : items === null
      ? ""
      : items.length
        ? t("scanSummary", { agents: groups.length, n: items.length })
        : t("scanSummaryNone")

  return (
    <Page
      title={t("scanTitle")}
      summary={summary}
      className="pb-5"
      actions={
        <Button variant="outline" onClick={() => void scan(true)} disabled={scanning}>
          <RefreshIcon spinning={scanning} />
          {scanning ? t("scanningShort") : t("rescan")}
        </Button>
      }
      footer={
        <footer className="relative z-[4] flex h-[57px] shrink-0 items-center gap-3 border-t-hd bg-bg px-10 font-body fs-body text-mut doodle:pr-[150px] max-[760px]:px-5">
          <span className="min-w-0 truncate">{t("adoptNote")}</span>
          <span className="flex-1" />
          {/* Nothing left to adopt: no button at all, not a greyed one. */}
          {selectable.length > 0 && (
            <Button size="lg" className="px-4" disabled={!n || scanning} variant={n ? "default" : "secondary"} onClick={adopt}>
              {n ? t("adoptSelected", { n }) : t("adoptPickFirst")}
            </Button>
          )}
        </footer>
      }
    >
      {done !== null && (
        <div role="status" className="mb-5 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-card bg-row-hv px-4 py-3">
          <div className="min-w-0 flex-1">
            <p className="font-heading fs-name font-semibold text-ink">{t("adoptDoneTitle")}</p>
            <p className="mt-0.5 fs-meta text-mut">{t("adoptDoneBody", { n: done })}</p>
          </div>
          <Button onClick={() => navigate("skills")}>
            {t("backToSkills")}
            <ArrowRight aria-hidden />
          </Button>
        </div>
      )}
      {failed ? (
        <PageError onRetry={() => void scan(false)} />
      ) : items === null ? (
        <div className="space-y-2 pt-1" role="status" aria-label={t("scanning")}>
          {/* Scanning fetches each upstream to compare: it can take a while. */}
          <p className="pb-1 fs-meta text-mut">{t("scanningHint")}</p>
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="h-[52px]" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyNote title={t("scanEmpty")}>{t("scanEmptyBody")}</EmptyNote>
      ) : (
        <div role="table" aria-label={t("scanTitle")} aria-busy={scanning} className="skd-cols-adopt">
          <TableHead style={{ gridTemplateColumns: COLS }}>
            <div role="columnheader">
              <Checkbox
                aria-label={t("selectAll")}
                checked={all}
                indeterminate={some && !all}
                onCheckedChange={() => setPicked(all ? new Set() : new Set(selectable.map((x) => x.id)))}
              />
            </div>
            <div role="columnheader">{t("colFound")}</div>
            <div role="columnheader">{t("colState")}</div>
            <div role="columnheader">{t("colAfter")}</div>
          </TableHead>
          {groups.map(([agent, list]) => {
            const a = agents.find((x) => x.name === agent) ?? { name: agent, label: agent, path: "" }
            return (
              <div role="rowgroup" key={agent} aria-label={label(agent)}>
                <div role="row" className="flex h-[35px] items-center gap-2 border-b-row px-2.5">
                  <AgentMono agent={a} size={20} className="rounded-[5px]" />
                  <span className="font-heading fs-body font-semibold text-ink">{label(agent)}</span>
                  <span className="truncate font-mono fs-meta text-mut">{tildify(a.path)}</span>
                </div>
                {list.map((it) => {
                  const id = `scan-${it.id}`
                  const on = picked.has(it.id)
                  const skip = it.proposal === "skip"
                  return (
                    <div
                      role="row"
                      key={it.id}
                      onClick={() => !skip && toggle(it.id, !on)}
                      className={cn("grid min-h-[59px] cursor-pointer items-center border-b-row px-2.5 py-2 hover:bg-row-hv", (!on || skip) && "opacity-55", skip && "cursor-default")}
                      style={{ gridTemplateColumns: COLS }}
                    >
                      <div role="cell" onClick={(e) => e.stopPropagation()}>
                        <Checkbox id={id} checked={on} disabled={skip} onCheckedChange={(v) => toggle(it.id, !!v)} />
                      </div>
                      <div role="cell" className="min-w-0 pr-3">
                        <label htmlFor={id} className="block cursor-pointer font-heading fs-name font-semibold text-ink" onClick={(e) => e.stopPropagation()}>
                          {it.name}
                        </label>
                        <div className="mt-0.5 truncate font-mono fs-meta text-mut">
                          {tildify(it.path)}
                          {it.linkTo && ` → ${tildify(it.linkTo)}`}
                        </div>
                      </div>
                      <div role="cell" className={cn("fs-meta", it.state === "broken" ? "font-semibold text-ac" : "text-mut")}>
                        {it.sharedCopy ? t("stateSecondCopy") : it.proposal === "enable" ? t("stateInLibrary") : it.differs ? t("stateDiffers") : (STATE[it.state] ? t(STATE[it.state]) : it.state)}
                      </div>
                      <div role="cell" className="line-clamp-3 min-w-0 fs-body leading-snug text-ink" title={proposal(it)}>
                        {proposal(it)}
                      </div>
                    </div>
                  )
                })}
              </div>
            )
          })}
        </div>
      )}
    </Page>
  )
}
