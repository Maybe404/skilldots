import * as React from "react"

import { cn } from "@/lib/utils"
import { isMine, kindLabel, useApp } from "@/lib/app-state"
import { agentLabel, tildify } from "@/lib/agents"
import type { Agent, Skill } from "@/lib/backend"
import { cellState, isOn } from "@/lib/pending"
import { AgentMono, OffRing, Tag, UpdateDot } from "@/components/app/bits"
import { AGENT_H, AGENTS_TOP, GROUP_H, LIST_TOP, ROW_H, type ListItem, type useSkillList } from "./model"

/*
 * The wire view: skills on the left, agents on the right, one curve per
 * installed link. Written for this app (no library has this layout); the
 * numbers come from the design file. Keyboard users get the same actions
 * from the "装上" buttons, the number keys and the detail panel; dragging a
 * port is a pointer shortcut only.
 */

type List = ReturnType<typeof useSkillList>

export function wireGeometry(width: number) {
  const padX = width < 900 ? 20 : 40
  let listW = 400
  let agentsW = 360
  const gap = width - 2 * padX - listW - agentsW
  if (gap < 160) {
    const avail = Math.max(420, width - 2 * padX - 140)
    listW = Math.round(avail * 0.52)
    agentsW = avail - listW
  }
  const x1 = padX + listW
  const x2 = Math.max(x1 + 120, width - padX - agentsW)
  return { padX, listW, agentsW, x1, x2, width: Math.max(width, x2 + agentsW + padX) }
}

const curve = (x1: number, y1: number, x2: number, y2: number, bend = 190) =>
  `M${x1} ${y1} C${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`

interface Wire {
  d: string
  stroke: string
  width: number
  opacity: number
  dash?: string
  z: number
}

export function WireView({
  list,
  width,
  scrollTop,
  hover,
  setHover,
  rowId,
}: {
  list: List
  width: number
  scrollTop: number
  hover: string | null
  setHover: (key: string | null) => void
  rowId: (key: string) => string
}) {
  const { t, skills, selected, select, openDetail, pending, togglePending, agentFocus, toggleCollapsed, updatable, overrides } = useApp()
  const { items, columns, count, listHeight } = list
  const g = wireGeometry(width)
  const contentRef = React.useRef<HTMLDivElement>(null)
  const [drag, setDrag] = React.useState<{ key: string; x: number; y: number } | null>(null)

  const focusKey = hover ?? selected
  const focusSkill = skills.find((s) => s.ref.key === focusKey) ?? null
  const selSkill = skills.find((s) => s.ref.key === selected) ?? null
  const agentsH = AGENTS_TOP + columns.length * AGENT_H
  const contentH = Math.max(listHeight + LIST_TOP + 30, agentsH + 20)
  const agentOffset = Math.min(Math.max(0, scrollTop), Math.max(0, contentH - agentsH))
  const ay = (k: number) => AGENTS_TOP + k * AGENT_H + AGENT_H / 2 + agentOffset

  // Dragging a port: follow the pointer, drop on an agent row to toggle it.
  React.useEffect(() => {
    if (!drag) return
    const move = (e: PointerEvent) => {
      const r = contentRef.current?.getBoundingClientRect()
      if (r) setDrag((d) => (d ? { ...d, x: e.clientX - r.left, y: e.clientY - r.top } : d))
    }
    const up = (e: PointerEvent) => {
      const el = document.elementFromPoint(e.clientX, e.clientY)?.closest<HTMLElement>("[data-agent]")
      const s = skills.find((x) => x.ref.key === drag.key)
      if (el && s) togglePending(s, el.dataset.agent!)
      setDrag(null)
    }
    const cancel = (e: KeyboardEvent) => e.key === "Escape" && setDrag(null)
    window.addEventListener("pointermove", move)
    window.addEventListener("pointerup", up, { once: true })
    window.addEventListener("keydown", cancel)
    return () => {
      window.removeEventListener("pointermove", move)
      window.removeEventListener("pointerup", up)
      window.removeEventListener("keydown", cancel)
    }
  }, [drag, skills, togglePending])

  const wires: Wire[] = []
  const labels: { x: number; y: number; text: number }[] = []
  for (const it of items) {
    if (it.type === "skill") {
      const s = it.skill
      const y1 = it.y + LIST_TOP + ROW_H / 2
      columns.forEach((a, k) => {
        const st = cellState(s, a.name, pending)
        const d = curve(g.x1, y1, g.x2, ay(k))
        if (st === "on" || st === "adding") {
          const pend = st === "adding"
          const f = s.ref.key === focusKey
          const af = agentFocus === a.name
          wires.push({
            d,
            stroke: f || pend ? "var(--ac)" : "var(--wire)",
            width: f ? 2.4 : af ? 1.8 : 0,
            opacity: !a.enabled ? 0.08 : f || pend ? 1 : af ? 0.7 : agentFocus ? 0.06 : -1,
            dash: pend || !a.enabled ? "6 5" : undefined,
            z: f || pend ? 2 : af ? 1 : 0,
          })
        } else if (st === "removing") {
          wires.push({ d, stroke: "var(--mut)", width: 1.4, opacity: 0.6, dash: "3 5", z: 2 })
        }
      })
    } else if (it.collapsed) {
      const y1 = it.y + LIST_TOP + GROUP_H / 2
      columns.forEach((a, k) => {
        const n = it.skills.filter((s) => isOn(s, a.name, pending)).length
        if (!n) return
        const y2 = ay(k)
        wires.push({ d: curve(g.x1, y1, g.x2, y2), stroke: "var(--wire)", width: 1 + n * 0.9, opacity: agentFocus && agentFocus !== a.name ? 0.08 : 0.4, z: 0 })
        labels.push({ x: g.x1 + 60, y: y1 - 6 + (y2 - y1) * 0.08, text: n })
      })
    }
  }
  wires.sort((a, b) => a.z - b.z)

  let dragPath = ""
  if (drag) {
    const it = items.find((x) => x.type === "skill" && x.skill.ref.key === drag.key)
    if (it) {
      const y1 = it.y + LIST_TOP + ROW_H / 2
      dragPath = `M${g.x1} ${y1} C${g.x1 + 120} ${y1}, ${drag.x - 120} ${drag.y}, ${drag.x} ${drag.y}`
    }
  }

  const focusText = focusSkill
    ? `${focusSkill.linkName} → ${columns.filter((a) => isOn(focusSkill, a.name, pending)).map(agentLabel).join(" · ") || "—"}`
    : ""

  return (
    <div ref={contentRef} className={cn("relative", drag && "cursor-grabbing select-none")} style={{ minHeight: contentH, width: g.width }}>
      {/* Agents: sticky on the right so the curves always have somewhere to land. */}
      <section
        aria-label={t("agentsTitle")}
        className="sticky top-0 z-[2] pt-3"
        style={{ marginLeft: g.x2, width: g.agentsW }}
      >
        <div className="mb-1.5 flex h-[25px] items-baseline gap-2.5 border-b-row px-3.5 font-body fs-meta text-mut">
          <span className="font-semibold text-ink">Agent</span>
          <span className="font-mono">{columns.length}</span>
          <span className="flex-1" />
          <span className="truncate">{selSkill ? t("wireAgentHintSel") : t("wireAgentHint")}</span>
        </div>
        {columns.map((a) => (
          <AgentRow key={a.name} agent={a} skill={selSkill} dragging={!!drag} claudeOff={!!selSkill && a.name === "claude" && overrides.has(selSkill.name)} />
        ))}
      </section>

      <svg aria-hidden className="pointer-events-none absolute top-0 left-0" width={g.width} height={contentH} fill="none" style={{ filter: "var(--wireFilter)" }}>
        {wires.map((w, i) => (
          <path
            key={i}
            d={w.d}
            stroke={w.stroke}
            strokeWidth={w.width || "var(--wireW)"}
            strokeOpacity={w.opacity < 0 ? "var(--wireOp)" : w.opacity}
            strokeDasharray={w.dash}
            strokeLinecap="round"
          />
        ))}
        {labels.map((l, i) => (
          <text key={i} x={l.x} y={l.y} fill="var(--hueInk)" fontSize={10.5} fontFamily="var(--fm)" textAnchor="middle">
            {l.text}
          </text>
        ))}
        {dragPath && <path d={dragPath} stroke="var(--ac)" strokeWidth={2.2} strokeDasharray="6 5" strokeLinecap="round" />}
      </svg>

      <section aria-label="Skill" className="absolute top-0" style={{ left: g.padX, width: g.listW }}>
        <div className="mt-3 mb-1.5 flex h-[25px] items-baseline gap-2.5 border-b-row px-3.5 font-body fs-meta text-mut">
          <span className="font-semibold text-ink">Skill</span>
          <span className="font-mono">{count}</span>
          <span className="flex-1" />
          <span className="max-w-[260px] truncate" aria-live="polite">
            {focusText}
          </span>
        </div>
        <div role="list" className="relative" style={{ height: listHeight }}>
          {items.map((it) =>
            it.type === "group" ? (
              <GroupRow key={"g:" + it.name} item={it} onToggle={() => toggleCollapsed(it.name)} />
            ) : (
              <SkillRow
                key={it.skill.ref.key}
                id={rowId(it.skill.ref.key)}
                skill={it.skill}
                y={it.y}
                dim={it.dim}
                count={columns.filter((a) => isOn(it.skill, a.name, pending)).length}
                total={columns.length}
                focused={it.skill.ref.key === focusKey}
                selected={it.skill.ref.key === selected}
                update={updatable.has(it.skill.ref.key)}
                sourceText={isMine(it.skill) ? t("kindMine") : kindLabel(t, it.skill)}
                onSelect={() => select(it.skill.ref.key)}
                onOpen={() => openDetail(it.skill.ref.key)}
                onEnter={() => setHover(it.skill.ref.key)}
                onLeave={() => setHover(null)}
                onPortDown={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  const r = contentRef.current?.getBoundingClientRect()
                  select(it.skill.ref.key)
                  if (r) setDrag({ key: it.skill.ref.key, x: e.clientX - r.left, y: e.clientY - r.top })
                }}
              />
            ),
          )}
        </div>
      </section>
    </div>
  )
}

function AgentRow({ agent: a, skill, dragging, claudeOff }: { agent: Agent; skill: Skill | null; dragging: boolean; claudeOff: boolean }) {
  const { t, skills, pending, togglePending, agentFocus, setAgentFocus } = useApp()
  const count = skills.filter((s) => isOn(s, a.name, pending)).length
  const selA = agentFocus === a.name
  const st = skill ? cellState(skill, a.name, pending) : "off"
  const on = st === "on" || st === "adding"
  const pend = st === "adding" || st === "removing"
  const label = agentLabel(a)
  const btnText = st === "adding" ? t("wireAdding") : st === "on" ? t("wireOn") : st === "removing" ? t("wireRemoving") : t("wireOff")
  return (
    <div
      data-agent={a.name}
      className={cn(
        "group/agent relative flex h-[72px] items-center gap-3 border-b-row pr-3.5 pl-[22px] transition-opacity",
        (selA || dragging) && "rounded-ctl bg-row-on",
        selA && "shadow-row",
        ((agentFocus && !selA) || !a.enabled) && "opacity-55",
      )}
    >
      <span
        aria-hidden
        className="absolute top-1/2 -left-[7px] -mt-1.5 size-3 rounded-full transition-shadow"
        style={{ background: on || selA ? "var(--ac)" : "var(--hue)", boxShadow: dragging ? "0 0 0 4px var(--acSoft)" : "none" }}
      />
      <AgentMono agent={a} size={34} />
      <button
        type="button"
        aria-pressed={selA}
        title={t("agentFocusHint", { agent: label })}
        onClick={() => setAgentFocus(selA ? null : a.name)}
        className="min-w-0 flex-1 rounded-ctl text-left after:absolute after:inset-0 after:content-[''] focus-visible:outline-none focus-visible:after:rounded-ctl focus-visible:after:outline-2 focus-visible:after:outline-ring"
      >
        <span className="flex items-center gap-1.5 font-heading fs-name leading-[1.1] font-semibold whitespace-nowrap text-ink">
          <span className="truncate">{label}</span>
          {!a.enabled && <Tag>{t("pausedTag")}</Tag>}
          {claudeOff && <OffRing />}
        </span>
        <span className="mt-[3px] block truncate font-mono fs-meta text-mut">{tildify(a.path)}</span>
      </button>
      {skill && (
        <button
          type="button"
          aria-pressed={on}
          aria-label={`${skill.linkName} · ${label} (${a.name}) · ${btnText}`}
          onClick={() => togglePending(skill, a.name)}
          className={cn(
            "relative z-[1] flex h-[26px] shrink-0 items-center rounded-ctl px-2.5 font-body fs-body whitespace-nowrap",
            on && !pend ? "border-[1.5px] border-hue bg-hue-soft text-hue-ink" : "border-[1.5px] border-dashed bg-transparent",
            pend ? "border-ac text-ac" : !on && "border-mut text-ink",
          )}
        >
          {btnText}
        </button>
      )}
      <span className="min-w-[30px] flex-none text-right font-mono text-[18px] leading-none font-medium text-hue-ink">{count}</span>
    </div>
  )
}

function GroupRow({ item, onToggle }: { item: Extract<ListItem, { type: "group" }>; onToggle: () => void }) {
  const { t } = useApp()
  return (
    <div role="listitem" className="absolute inset-x-0" style={{ top: item.y, height: GROUP_H }}>
      <button
        type="button"
        aria-expanded={!item.collapsed}
        onClick={onToggle}
        className="relative flex size-full items-center gap-2 rounded-ctl px-4 text-left"
      >
        <span aria-hidden className={cn("inline-block w-3 text-sm leading-none text-mut transition-transform", !item.collapsed && "rotate-90")}>
          ›
        </span>
        <span className="font-mono fs-meta text-ink">{item.name}</span>
        <span className="font-body fs-meta text-mut">{t("groupCount", { n: item.skills.length })}</span>
        {item.collapsed && <span aria-hidden className="absolute top-1/2 -right-1.5 -mt-1.5 size-3 rounded-full bg-[var(--wire)]" />}
      </button>
    </div>
  )
}

function SkillRow({
  id,
  skill,
  y,
  dim,
  count,
  total,
  focused,
  selected,
  update,
  sourceText,
  onSelect,
  onOpen,
  onEnter,
  onLeave,
  onPortDown,
}: {
  id: string
  skill: Skill
  y: number
  dim: boolean
  count: number
  total: number
  focused: boolean
  selected: boolean
  update: boolean
  sourceText: string
  onSelect: () => void
  onOpen: () => void
  onEnter: () => void
  onLeave: () => void
  onPortDown: (e: React.PointerEvent) => void
}) {
  const { t } = useApp()
  return (
    <div role="listitem" className="absolute inset-x-0" style={{ top: y, height: ROW_H }} onMouseEnter={onEnter} onMouseLeave={onLeave}>
      <button
        id={id}
        type="button"
        tabIndex={selected ? 0 : -1}
        aria-current={selected ? "true" : undefined}
        aria-label={`${skill.linkName} · ${sourceText} · ${t("installedCount", { n: count, total })}`}
        onClick={onSelect}
        onDoubleClick={onOpen}
        className={cn(
          "relative flex size-full items-center gap-3 rounded-card pr-[18px] pl-4 text-left transition-opacity",
          focused && "bg-row-on shadow-row",
          dim && "opacity-40",
          selected && !focused && "outline-[1.5px] -outline-offset-1 outline-dashed outline-ac",
        )}
      >
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-[7px] font-heading fs-name leading-[1.2] font-semibold text-ink">
            <span className="truncate">{skill.linkName}</span>
            {update && <UpdateDot />}
            {isMine(skill) && <Tag>{t("kindMine")}</Tag>}
          </span>
          <span className="mt-0.5 block truncate font-mono fs-meta text-mut">{sourceText}</span>
        </span>
        <span className="font-mono fs-meta whitespace-nowrap text-hue-ink" title={t("installedOfTotal")}>
          {count}/{total}
        </span>
        <span
          aria-hidden
          title={t("dragToInstall")}
          onPointerDown={onPortDown}
          className="absolute top-1/2 -right-1.5 -mt-1.5 box-border size-3 cursor-grab rounded-full border-2"
          style={{ background: focused ? "var(--ac)" : "var(--bg)", borderColor: focused ? "var(--ac)" : "var(--hue)" }}
        />
      </button>
    </div>
  )
}
