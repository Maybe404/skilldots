import { ArrowDown, ArrowUp } from "lucide-react"

import { cn } from "@/lib/utils"
import { isMine, kindLabel, useApp, type TableSort } from "@/lib/app-state"
import { agentLabel, agentMono } from "@/lib/agents"
import type { Agent, Skill } from "@/lib/backend"
import { cellState, isOn } from "@/lib/pending"
import { AgentMono, Hint, OffRing, Tag, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { DisclosureChevron } from "@/components/ui/collapsible"
import type { ListItem, useSkillList } from "./model"

/*
 * The table view: one row per skill, one column per agent. Rows and columns
 * follow the design's grid; a cell toggles a pending change. The selected row
 * is the only one in the Tab order (its name, then its cells); the number
 * keys toggle cells of the selected row from anywhere on the page.
 */

type List = ReturnType<typeof useSkillList>

export function TableView({
  list,
  hover,
  setHover,
  rowId,
}: {
  list: List
  hover: string | null
  setHover: (key: string | null) => void
  rowId: (key: string) => string
}) {
  const { t, skills, selected, select, openDetail, pending, togglePending, agentFocus, setAgentFocus, toggleCollapsed, updatable, overrides, tableSort, setTableSort } = useApp()
  const { items, columns } = list
  const cols = `minmax(0,1fr) 170px repeat(${columns.length},52px)`
  const focusKey = hover ?? selected

  const sortButton = (by: NonNullable<TableSort>["by"], label: string) => {
    const dir = tableSort?.by === by ? tableSort.dir : null
    const next: TableSort = dir === null ? { by, dir: "asc" } : dir === "asc" ? { by, dir: "desc" } : null
    return (
      <div role="columnheader" aria-sort={dir === "asc" ? "ascending" : dir === "desc" ? "descending" : "none"}>
        <Button variant="ghost" size="xs" onClick={() => setTableSort(next)} className="-ml-1 gap-1 rounded-seg px-1 font-normal text-[length:inherit] text-mut hover:bg-transparent">
          {label}
          {dir === "asc" ? <ArrowUp className="size-3" /> : dir === "desc" ? <ArrowDown className="size-3" /> : null}
        </Button>
      </div>
    )
  }

  return (
    <div role="table" aria-label="Skill" className="px-10 pt-3 max-[760px]:px-5">
      <div
        role="row"
        className="sticky top-0 z-[2] grid h-[45px] items-center border-b-head bg-bg px-3.5 font-body fs-meta text-mut"
        style={{ gridTemplateColumns: cols }}
      >
        {sortButton("name", t("colName"))}
        {sortButton("source", t("colSource"))}
        {columns.map((a) => (
          <div role="columnheader" key={a.name} className="grid place-items-center">
            <Hint label={agentLabel(a)}>
              <button
                type="button"
                aria-pressed={agentFocus === a.name}
                aria-label={t("agentFocusHint", { agent: agentLabel(a) })}
                onClick={() => setAgentFocus(agentFocus === a.name ? null : a.name)}
                className={cn(
                  "flex h-9 w-full flex-col items-center justify-center gap-0.5 rounded-ctl",
                  agentFocus === a.name && "bg-row-on shadow-[inset_0_-2px_0_var(--hue)]",
                  !a.enabled && "opacity-50",
                )}
              >
                <AgentMono agent={a} size={[26, 22]} className="rounded-[6px]" />
                <span className="font-mono text-[10px] text-mut">{skills.filter((s) => isOn(s, a.name, pending)).length}</span>
              </button>
            </Hint>
          </div>
        ))}
      </div>

      {items.map((it) =>
        it.type === "group" ? (
          <GroupRow key={"g:" + it.name} item={it} onToggle={() => toggleCollapsed(it.name)} />
        ) : (
          <Row
            key={it.skill.ref.key}
            id={rowId(it.skill.ref.key)}
            skill={it.skill}
            cols={cols}
            columns={columns}
            dim={it.dim}
            focused={it.skill.ref.key === focusKey}
            selected={it.skill.ref.key === selected}
            update={updatable.has(it.skill.ref.key)}
            claudeOff={overrides.has(it.skill.name)}
            onSelect={() => select(it.skill.ref.key)}
            onOpen={() => openDetail(it.skill.ref.key)}
            onEnter={() => setHover(it.skill.ref.key)}
            onLeave={() => setHover(null)}
            onToggle={(agent) => togglePending(it.skill, agent)}
          />
        ),
      )}
    </div>
  )
}

function GroupRow({ item, onToggle }: { item: Extract<ListItem, { type: "group" }>; onToggle: () => void }) {
  const { t } = useApp()
  return (
    <div role="row" className="border-b-row">
      <div role="cell">
        <button type="button" aria-expanded={!item.collapsed} onClick={onToggle} className="flex h-[34px] w-full items-center gap-2 px-3.5 text-left">
          <DisclosureChevron open={!item.collapsed} />
          <span className="font-mono fs-meta text-ink">{item.name}</span>
          <span className="font-body fs-meta text-mut">{t("groupCount", { n: item.skills.length })}</span>
        </button>
      </div>
    </div>
  )
}

function Row({
  id,
  skill,
  cols,
  columns,
  dim,
  focused,
  selected,
  update,
  claudeOff,
  onSelect,
  onOpen,
  onEnter,
  onLeave,
  onToggle,
}: {
  id: string
  skill: Skill
  cols: string
  columns: Agent[]
  dim: boolean
  focused: boolean
  selected: boolean
  update: boolean
  claudeOff: boolean
  onSelect: () => void
  onOpen: () => void
  onEnter: () => void
  onLeave: () => void
  onToggle: (agent: string) => void
}) {
  const { t, pending } = useApp()
  const source = isMine(skill) ? t("kindMine") : kindLabel(t, skill)
  return (
    <div
      role="row"
      aria-selected={selected}
      onClick={onSelect}
      onDoubleClick={onOpen}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      className={cn("relative grid h-[59px] items-center border-b-row px-3.5 transition-opacity", focused && "bg-row-on", dim && "opacity-40")}
      style={{ gridTemplateColumns: cols }}
    >
      <div role="cell" className="min-w-0 pr-4">
        <button
          id={id}
          type="button"
          tabIndex={selected ? 0 : -1}
          aria-current={selected ? "true" : undefined}
          onClick={(e) => {
            e.stopPropagation()
            onSelect()
          }}
          className="block w-full min-w-0 rounded-ctl text-left"
        >
          <span className="flex items-center gap-[7px] font-heading fs-name leading-[1.2] font-semibold text-ink">
            <span className="truncate">{skill.linkName}</span>
            {update && <UpdateDot />}
            {isMine(skill) && <Tag>{t("kindMine")}</Tag>}
          </span>
          <span className="mt-0.5 block truncate font-body fs-meta text-mut">{skill.description}</span>
        </button>
      </div>
      <div role="cell" className="truncate pr-2.5 font-mono fs-meta text-mut" title={source}>
        {source}
      </div>
      {columns.map((a) => {
        const st = cellState(skill, a.name, pending)
        const on = st === "on" || st === "adding"
        const label = agentLabel(a)
        const title =
          st === "on" ? t("cellOn", { agent: label }) : st === "adding" ? t("cellAdding", { agent: label }) : st === "removing" ? t("cellRemoving", { agent: label }) : t("cellOff", { agent: label })
        return (
          <div role="cell" key={a.name} className="relative grid place-items-center">
            <Hint label={title}>
              <button
                type="button"
                tabIndex={selected ? 0 : -1}
                aria-pressed={on}
                aria-label={`${skill.linkName} · ${label} (${a.name}) · ${title}`}
                data-state={st}
                onClick={(e) => {
                  e.stopPropagation()
                  onSelect()
                  onToggle(a.name)
                }}
                className={cn("skd-cell", !a.enabled && "opacity-45")}
              >
                {agentMono(a)}
              </button>
            </Hint>
            {claudeOff && a.name === "claude" && <OffRing className="pointer-events-none absolute top-1 right-2" />}
          </div>
        )
      })}
    </div>
  )
}
