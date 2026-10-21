import { ArrowDown, ArrowUp, Check, CornerDownRight, Minus, Plus } from "lucide-react"

import { cn } from "@/lib/utils"
import { hasConflicts, isMine, sourceText, useApp, type TableSort } from "@/lib/app-state"
import { agentLabel, isAbsentAgent } from "@/lib/agents"
import type { Agent, Skill } from "@/lib/backend"
import { cellState, isOn, keepsViaShared } from "@/lib/pending"
import { ConflictMark, Hint, OffRing, Tag, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { DisclosureChevron } from "@/components/ui/collapsible"
import { GroupCell } from "./group-batch"
import { RowActions } from "./row-actions"
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
  // The name keeps room for a long name (differential-review in the doodle face); the source gives way first.
  const cols = `minmax(13rem,1fr) minmax(0,150px) repeat(${columns.length},60px)`
  const focusKey = hover ?? selected
  // Without a selection the first row takes Tab, so the table stays reachable.
  const tabKey = list.order.some((s) => s.ref.key === selected) ? selected : list.order[0]?.ref.key

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
            {/* The agent's own name (two short lines at most), its count under it; the full name, and what the shared folder is, on hover. */}
            <Hint label={a.name === "agents" ? `${agentLabel(a)} · ${t("sharedDirNote")}` : isAbsentAgent(a) ? `${agentLabel(a)} · ${t("notInstalled")}` : agentLabel(a)}>
              <button
                type="button"
                aria-pressed={agentFocus === a.name}
                aria-label={t("agentFocusHint", { agent: agentLabel(a) })}
                onClick={() => setAgentFocus(agentFocus === a.name ? null : a.name)}
                className={cn(
                  "flex h-10 w-full flex-col items-center justify-center gap-px rounded-ctl px-0.5",
                  agentFocus === a.name && "bg-row-on shadow-[inset_0_-2px_0_var(--hue)]",
                  (!a.enabled || isAbsentAgent(a)) && "opacity-50",
                )}
              >
                <span className="line-clamp-2 max-w-full text-center text-[11px] leading-[1.15] font-medium break-words text-ink">{agentLabel(a)}</span>
                <span className="text-[10px] leading-none text-mut tabular-nums">{a.enabled ? skills.filter((s) => isOn(s, a.name, pending)).length : t("pausedTag")}</span>
              </button>
            </Hint>
          </div>
        ))}
      </div>

      {items.map((it) =>
        it.type === "group" ? (
          <GroupRow key={"g:" + it.name} item={it} cols={cols} columns={columns} onToggle={() => toggleCollapsed(it.name)} />
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
            tabStop={it.skill.ref.key === tabKey}
            update={updatable.has(it.skill.ref.key)}
            claudeOff={overrides.has(it.skill.name)}
            onSelect={() => select(it.skill.ref.key)}
            // A second click on the selected row clears the selection.
            onPick={(e) => e.detail < 2 && select(it.skill.ref.key === selected ? null : it.skill.ref.key)}
            onOpen={() => {
              select(it.skill.ref.key)
              openDetail(it.skill.ref.key)
            }}
            onEnter={() => setHover(it.skill.ref.key)}
            onLeave={() => setHover(null)}
            onToggle={(agent) => togglePending(it.skill, agent)}
          />
        ),
      )}
    </div>
  )
}

/** A source's group header: folds the group, and switches the whole group per agent (pending changes). */
function GroupRow({ item, onToggle, cols, columns }: { item: Extract<ListItem, { type: "group" }>; onToggle: () => void; cols: string; columns: Agent[] }) {
  const { t } = useApp()
  return (
    <div role="row" className="grid h-[34px] items-center border-b-row px-3.5" style={{ gridTemplateColumns: cols }}>
      <div role="cell" className="col-span-2 min-w-0">
        <button type="button" aria-expanded={!item.collapsed} onClick={onToggle} className="flex h-[34px] w-full min-w-0 items-center gap-2 text-left">
          <DisclosureChevron open={!item.collapsed} />
          <span className="truncate fs-meta font-medium text-ink">{item.name}</span>
          <span className="shrink-0 font-body fs-meta text-mut">{t("groupCount", { n: item.skills.length })}</span>
        </button>
      </div>
      {columns.map((a) => (
        <div role="cell" key={a.name} className="grid place-items-center">
          <GroupCell group={item.name} skills={item.skills} agent={a} />
        </div>
      ))}
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
  tabStop,
  update,
  claudeOff,
  onSelect,
  onPick,
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
  tabStop: boolean
  update: boolean
  claudeOff: boolean
  onSelect: () => void
  onPick: (e: React.MouseEvent) => void
  onOpen: () => void
  onEnter: () => void
  onLeave: () => void
  onToggle: (agent: string) => void
}) {
  const { t, pending } = useApp()
  const source = sourceText(t, skill)
  return (
    <div
      role="row"
      aria-selected={selected}
      onClick={onPick}
      onDoubleClick={onOpen}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      className={cn("group/row relative grid h-[59px] items-center border-b-row px-3.5 transition-opacity", focused && "bg-row-on", dim && "opacity-40")}
      style={{ gridTemplateColumns: cols }}
    >
      <div role="cell" className="relative min-w-0 pr-4">
        <button
          id={id}
          type="button"
          tabIndex={tabStop ? 0 : -1}
          aria-current={selected ? "true" : undefined}
          onClick={(e) => {
            e.stopPropagation()
            onPick(e)
          }}
          className="block w-full min-w-0 rounded-ctl text-left"
        >
          <span className="flex items-center gap-[7px] font-heading fs-name leading-[1.2] font-semibold text-ink">
            <span className="truncate">{skill.linkName}</span>
            {update && <UpdateDot />}
            {isMine(skill) && <Tag>{t("kindMine")}</Tag>}
            {hasConflicts(skill) && <ConflictMark />}
          </span>
          <span className="mt-0.5 block truncate font-body fs-meta text-mut">{skill.description}</span>
        </button>
        <RowActions skill={skill} shown={selected || focused} tinted={focused} className="absolute top-1/2 right-4 -translate-y-1/2 rounded-ctl" />
      </div>
      <div role="cell" className="truncate pr-2.5 fs-meta text-mut" title={source}>
        {source}
      </div>
      {columns.map((a) => {
        const st = cellState(skill, a.name, pending)
        const on = st === "on" || st === "adding" || st === "absent" || st === "via"
        const label = agentLabel(a)
        // Off for an agent this Mac does not have: it cannot be turned on (app-state refuses and says why).
        const locked = st === "off" && isAbsentAgent(a)
        const title =
          st === "via" ? t(skill.targets.includes(a.name) ? "cellViaOwn" : "cellVia", { agent: label }) : st === "on" ? t(a.enabled ? "cellOn" : "cellPaused", { agent: label }) : st === "absent" ? t("cellAbsent", { agent: label }) : st === "adding" ? t("cellAdding", { agent: label }) : st === "removing" ? t(keepsViaShared(skill, a.name, pending) ? "cellViaCancel" : "cellRemoving", { agent: label }) : locked ? t("absentCannotInstall", { agent: label }) : t("cellOff", { agent: label })
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
                aria-disabled={locked || undefined}
                className={cn("skd-cell", !a.enabled && "opacity-45", locked && "cursor-not-allowed opacity-40 hover:text-transparent")}
              >
                {st === "removing" ? <Minus aria-hidden className="size-3.5" strokeWidth={2.4} /> : st === "via" ? <CornerDownRight aria-hidden className="size-3.5" strokeWidth={2.2} /> : on ? <Check aria-hidden className="size-3.5" strokeWidth={2.6} /> : locked ? null : <Plus aria-hidden className="size-3.5" strokeWidth={2.2} />}
              </button>
            </Hint>
            {claudeOff && a.name === "claude" && <OffRing className="pointer-events-none absolute top-1 right-2" />}
          </div>
        )
      })}
    </div>
  )
}
