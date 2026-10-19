import * as React from "react"
import { toast } from "sonner"
import { ListTree, Table, X } from "lucide-react"

import { useApp, type SkillFilter } from "@/lib/app-state"
import { agentLabel } from "@/lib/agents"
import { isOn } from "@/lib/pending"
import { EmptyNote } from "@/components/app/bits"
import { StatusBar } from "@/components/app/status-bar"
import { Button } from "@/components/ui/button"
import { Toggle } from "@/components/ui/toggle"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { matchesFilter, useSkillList } from "./model"
import { SkillSheet } from "./skill-sheet"
import { TableView } from "./table-view"
import { WireView } from "./wire-view"

const rowId = (key: string) => "skill-row-" + key.replace(/[^A-Za-z0-9_-]/g, "_")

// The first skill is picked once per launch; after that the selection is the user's, including none.
let autoPicked = false

function isTyping(el: Element | null) {
  if (!el) return false
  const h = el as HTMLElement
  return h.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(h.tagName) || !!h.closest("[role=dialog],[role=alertdialog],[role=menu],[role=listbox],.cm-editor")
}

export function SkillsPage() {
  const app = useApp()
  const { t, skills, updatable, filter, setFilter, query, setQuery, grouped, setGrouped, agentFocus, setAgentFocus, skillView, setSkillView, selected, select, openDetail, pending, pendingCount, applying, togglePending, applyPending, revertPending, detail, dialog, paletteOpen } = app
  const list = useSkillList()
  const { order, columns } = list
  const scrollRef = React.useRef<HTMLDivElement>(null)
  const [width, setWidth] = React.useState(1280)
  const [height, setHeight] = React.useState(0)
  const [hover, setHover] = React.useState<string | null>(null)

  React.useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const measure = () => {
      setWidth(el.clientWidth)
      setHeight(el.clientHeight)
    }
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    measure()
    return () => ro.disconnect()
  }, [])

  // Start on the first visible skill, like the design, and move to it when the selected one disappears
  // from the library. A cleared selection (a second click, esc, moving the skill to the trash) stays cleared.
  React.useEffect(() => {
    if (!order.length) return
    if (selected ? !skills.some((s) => s.ref.key === selected) : !autoPicked) select(order[0].ref.key)
    autoPicked = true
  }, [order, selected, skills, select])

  const moveTo = React.useCallback(
    (key: string, focus: boolean) => {
      select(key)
      requestAnimationFrame(() => {
        const el = document.getElementById(rowId(key))
        el?.scrollIntoView({ block: "nearest" })
        if (focus) el?.focus({ preventScroll: true })
      })
    },
    [select],
  )

  // The design's keyboard: ↑↓ select, 1–n toggle an agent, ⏎ apply, v view, esc clear.
  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return
      if (detail || dialog || paletteOpen || isTyping(document.activeElement)) return
      const target = e.target as HTMLElement
      const onRow = !!target.id && target.id.startsWith("skill-row-")
      const onControl = !onRow && !!target.closest("button,a,[role=button]")
      const idx = order.findIndex((s) => s.ref.key === selected)
      const current = idx >= 0 ? order[idx] : null
      const inList = onRow || target === document.body || (!onControl && !!target.closest("[data-skill-page]"))
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        if (!inList || !order.length) return
        e.preventDefault()
        const next = order[Math.max(0, Math.min(order.length - 1, idx + (e.key === "ArrowDown" ? 1 : -1)))]
        if (next) moveTo(next.ref.key, true)
      } else if (/^[1-9]$/.test(e.key)) {
        const a = columns[Number(e.key) - 1]
        if (!a || !current) return
        e.preventDefault()
        togglePending(current, a.name)
      } else if (e.key === "Enter") {
        if (onControl) return
        e.preventDefault()
        if (pendingCount) void applyPending()
        else if (current) openDetail(current.ref.key)
      } else if (e.key === " ") {
        if (onControl || !current) return
        e.preventDefault()
        openDetail(current.ref.key)
      } else if (e.key === "v" || e.key === "V") {
        setSkillView(skillView === "wire" ? "table" : "wire")
      } else if (e.key === "Escape") {
        if (agentFocus) setAgentFocus(null)
        else if (selected) select(null)
        else if (pendingCount && !applying) {
          revertPending()
          toast(t("revertedN", { n: pendingCount }))
        }
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [order, selected, select, columns, detail, dialog, paletteOpen, pendingCount, applyPending, openDetail, togglePending, moveTo, skillView, setSkillView, agentFocus, setAgentFocus, revertPending, applying, t])

  const filters: [SkillFilter, string][] = [
    ["all", t("filterAll")],
    ["mine", t("filterMine")],
    ["updates", t("filterUpdates")],
  ]
  const focusAgent = columns.find((a) => a.name === agentFocus)

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-skill-page>
      <div className="flex h-[46px] shrink-0 items-center gap-2 px-10 font-body fs-body max-[760px]:px-5">
        <ToggleGroup
          aria-label={t("filterLabel")}
          variant="chip"
          size="none"
          value={filter === "vendor" ? [] : [filter]}
          onValueChange={(v) => v[0] && setFilter(v[0] as SkillFilter)}
          spacing={8}
        >
          {filters.map(([value, label]) => (
            <ToggleGroupItem key={value} value={value}>
              {label}
              <span className="font-mono fs-meta opacity-70">{skills.filter((s) => matchesFilter(s, value, updatable)).length}</span>
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        {filter === "vendor" && (
          <Button variant="secondary" size="sm" className="rounded-[13px] pr-1.5" onClick={() => setFilter("all")}>
            {t("filterVendor")} <X className="size-3" aria-label={t("clearFilter")} />
          </Button>
        )}
        <span aria-hidden className="mx-1 h-4 w-px bg-ln" />
        <Toggle variant="chip" size="none" pressed={grouped} onPressedChange={setGrouped}>
          <ListTree aria-hidden strokeWidth={2} />
          {t("groupBySource")}
        </Toggle>
        {focusAgent && (
          <Button
            variant="hue"
            onClick={() => setAgentFocus(null)}
            aria-label={t("clearAgentFocus", { agent: agentLabel(focusAgent) })}
            className="h-[26px] gap-2 rounded-[13px] pr-1.5 pl-2.5"
          >
            {t("agentFocusChip", { agent: agentLabel(focusAgent), n: skills.filter((s) => isOn(s, focusAgent.name, pending)).length })}
            <span aria-hidden className="grid size-4 place-items-center rounded-full bg-bg/90 text-hue">
              <X className="size-2.5" strokeWidth={3} />
            </span>
          </Button>
        )}
        <span className="flex-1" />
        <ToggleGroup aria-label={t("viewLabel")} variant="seg" size="none" value={[skillView]} onValueChange={(v) => v[0] && setSkillView(v[0] as "wire" | "table")} className="skd-seg h-[34px]" spacing={2}>
          <ToggleGroupItem value="wire" className="h-7">
            {/* The design's own wire glyph (two crossing curves); Lucide has none like it. */}
            <svg aria-hidden width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M4 6c8 0 8 12 16 12M4 18c8 0 8-12 16-12" />
            </svg>
            {t("viewWire")}
          </ToggleGroupItem>
          <ToggleGroupItem value="table" className="h-7">
            <Table aria-hidden strokeWidth={2} />
            {t("viewTable")}
          </ToggleGroupItem>
        </ToggleGroup>
      </div>

      <div ref={scrollRef} className="relative min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
        {order.length === 0 && list.items.length === 0 ? (
          <div className="px-10 max-[760px]:px-5">
            {skills.length === 0 ? (
              <EmptyNote
                title={t("noSkillsTitle")}
                action={
                  <>
                    <Button onClick={() => app.openDialog({ kind: "add" })}>{t("addSkill")}</Button>
                    <Button variant="outline" onClick={() => app.openDialog({ kind: "new" })}>
                      {t("newSkill")}
                    </Button>
                    <Button variant="outline" onClick={() => app.navigate("scan")}>
                      {t("goScan")}
                    </Button>
                  </>
                }
              >
                {t("noSkillsBody")}
              </EmptyNote>
            ) : (
              <EmptyNote
                title={t("noMatch")}
                action={
                  <Button
                    variant="outline"
                    onClick={() => {
                      setFilter("all")
                      setQuery("")
                      setAgentFocus(null)
                    }}
                  >
                    {t("clearFilters")}
                  </Button>
                }
              >
                {query ? t("noMatchQuery", { query }) : t("noMatchFilter")}
              </EmptyNote>
            )}
          </div>
        ) : skillView === "wire" ? (
          <WireView list={list} width={width} height={height} scroller={scrollRef} hover={hover} setHover={setHover} rowId={rowId} />
        ) : (
          <TableView list={list} hover={hover} setHover={setHover} rowId={rowId} />
        )}
      </div>

      <StatusBar />
      <SkillSheet returnFocus={() => (selected ? document.getElementById(rowId(selected)) : null)} />
    </div>
  )
}
