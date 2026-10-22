import * as React from "react"

import { cn } from "@/lib/utils"
import { hasConflicts, isMine, sourceText, useApp } from "@/lib/app-state"
import { agentLabel, isAbsentAgent, tildify } from "@/lib/agents"
import type { Agent, Skill } from "@/lib/backend"
import { cellState, isOn, keepsViaShared } from "@/lib/pending"
import { AgentMono, ConflictMark, ContributionMark, EditedMark, Hint, OffRing, Tag, UpdateDot } from "@/components/app/bits"
import { DisclosureChevron } from "@/components/ui/collapsible"
import { Switch } from "@/components/ui/switch"
import { GroupBatchMenu } from "./group-batch"
import { RowActions } from "./row-actions"
import { traceCubic, wireSeed } from "./wobble"
import { AGENT_H, AGENTS_TOP, GROUP_H, LIST_TOP, ROW_H, type ListItem, type useSkillList } from "./model"

/*
 * The wire view: skills on the left, agents on the right, one curve per
 * installed link. Written for this app (no library has this layout); the
 * numbers come from the design file. Keyboard users get the same actions
 * from the "装上" buttons, the number keys and the detail panel; dragging a
 * port is a pointer shortcut only.
 *
 * Scrolling: a curve joins a skill row, which scrolls, to an agent row, which
 * stays put, so the curves are redrawn on every scroll. WebKit scrolls (and
 * pins sticky elements) off the main thread, ahead of any script, so a native
 * sticky agent column ran a frame or two ahead of the curves and they jumped
 * on a fast display. Instead the page scrolls an empty spacer; one pinned
 * frame shows the columns, moved by script, and a canvas of curves, redrawn
 * in the same scroll event, so all three always move together.
 */

type List = ReturnType<typeof useSkillList>

export function wireGeometry(width: number) {
  const padX = width < 900 ? 20 : 40
  let listW = 400
  let agentsW = 360
  const gap = width - 2 * padX - listW - agentsW
  if (gap < 180) {
    const avail = Math.max(420, width - 2 * padX - 180)
    listW = Math.round(avail * 0.52)
    agentsW = avail - listW
  }
  const x1 = padX + listW
  const x2 = Math.max(x1 + 120, width - padX - agentsW)
  return { padX, listW, agentsW, x1, x2, width: Math.max(width, x2 + agentsW + padX) }
}

// The design bends each curve by 190px; narrower windows bend less so curves never loop.
const bendOf = (x1: number, x2: number) => Math.min(190, (x2 - x1) * 0.43)

type Ink = "ac" | "wire" | "mut"

interface Wire {
  /** Skill end, in content coordinates (before scrolling). */
  y1: number
  /** Agent index (the agent end follows the agent column). */
  k: number
  seed: number
  ink: Ink
  /** 0: the style's --wireW. */
  width: number
  /** -1: the style's --wireOp. */
  opacity: number
  dash?: number[]
  z: number
}

interface Scene {
  wires: Wire[]
  labels: { y1: number; k: number; text: number }[]
  drag: { y1: number; x: number; y: number } | null
  x1: number
  x2: number
  width: number
  contentH: number
  agentsH: number
  viewH: number
  wobble: boolean
}

/** The style's wire colours, read from its CSS variables (canvas cannot use var()). */
function readInks() {
  const cs = getComputedStyle(document.documentElement)
  const v = (name: string) => cs.getPropertyValue(name).trim()
  return {
    style: document.documentElement.dataset.style,
    ac: v("--ac"),
    wire: v("--wire"),
    mut: v("--mut"),
    hueInk: v("--hueInk"),
    wireW: parseFloat(v("--wireW")) || 1.2,
    wireOp: parseFloat(v("--wireOp")) || 0.12,
    font: v("--fb"),
  }
}

export function WireView({
  list,
  width,
  height,
  scroller,
  hover,
  setHover,
  rowId,
}: {
  list: List
  width: number
  /** Height of the scroller's viewport. */
  height: number
  scroller: React.RefObject<HTMLDivElement | null>
  hover: string | null
  setHover: (key: string | null) => void
  rowId: (key: string) => string
}) {
  const { t, style, skills, selected, select, openDetail, pending, togglePending, agentFocus, toggleCollapsed, updatable, overrides } = useApp()
  const { items, order, columns, count, listHeight } = list
  const g = wireGeometry(width)
  const wobble = style === "doodle"
  const layerRef = React.useRef<HTMLDivElement>(null)
  const agentsRef = React.useRef<HTMLElement>(null)
  const canvasRef = React.useRef<HTMLCanvasElement>(null)
  const scene = React.useRef<Scene | null>(null)
  const inks = React.useRef<ReturnType<typeof readInks> | null>(null)
  /** True while the list scrolls: rows passing under the pointer take no hover. */
  const scrolling = React.useRef(false)
  const [drag, setDrag] = React.useState<{ key: string; x: number; y: number } | null>(null)

  const focusKey = hover ?? selected
  const focusSkill = skills.find((s) => s.ref.key === focusKey) ?? null
  const selSkill = skills.find((s) => s.ref.key === selected) ?? null
  // Without a selection the first row takes Tab, so the list stays reachable.
  const tabKey = selSkill && order.includes(selSkill) ? selSkill.ref.key : order[0]?.ref.key
  const agentsH = AGENTS_TOP + columns.length * AGENT_H
  const contentH = Math.max(listHeight + LIST_TOP + 30, agentsH + 20)
  const viewH = Math.min(height, contentH)

  // Dragging a port: follow the pointer, drop on an agent row to toggle it.
  React.useEffect(() => {
    if (!drag) return
    const move = (e: PointerEvent) => {
      const r = layerRef.current?.getBoundingClientRect()
      if (r) setDrag((d) => (d ? { ...d, x: e.clientX - r.left, y: e.clientY - r.top } : d))
    }
    const up = (e: PointerEvent) => {
      const el = document.elementFromPoint(e.clientX, e.clientY)?.closest<HTMLElement>("[data-agent]")
      const s = skills.find((x) => x.ref.key === drag.key)
      if (el && s) togglePending(s, el.dataset.agent!)
      setDrag(null)
    }
    // Esc only cancels the drag: caught first and marked handled, so the page's esc (clear the selection) skips it.
    const cancel = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return
      e.preventDefault()
      setDrag(null)
    }
    window.addEventListener("pointermove", move)
    window.addEventListener("pointerup", up, { once: true })
    window.addEventListener("keydown", cancel, true)
    return () => {
      window.removeEventListener("pointermove", move)
      window.removeEventListener("pointerup", up)
      window.removeEventListener("keydown", cancel, true)
    }
  }, [drag, skills, togglePending])

  const wires: Wire[] = []
  const labels: Scene["labels"] = []
  for (const it of items) {
    if (it.type === "skill") {
      const s = it.skill
      const y1 = it.y + LIST_TOP + ROW_H / 2
      columns.forEach((a, k) => {
        const st = cellState(s, a.name, pending)
        const seed = wireSeed(s.ref.key + ">" + a.name)
        if (st === "on" || st === "adding") {
          const pend = st === "adding"
          const f = s.ref.key === focusKey
          const af = agentFocus === a.name
          wires.push({
            y1,
            k,
            seed,
            ink: f || pend ? "ac" : "wire",
            width: f ? 2.4 : af ? 1.8 : 0,
            opacity: !a.enabled ? 0.08 : f || pend ? 1 : af ? 0.7 : agentFocus ? 0.06 : -1,
            dash: pend || !a.enabled ? [6, 5] : undefined,
            z: f || pend ? 2 : af ? 1 : 0,
          })
        } else if (st === "absent") {
          // Chosen, but this Mac has no such agent: a faint dotted line, nothing is linked.
          wires.push({ y1, k, seed, ink: "mut", width: 1.2, opacity: 0.35, dash: [2, 4], z: 0 })
        } else if (st === "removing") {
          wires.push({ y1, k, seed, ink: "mut", width: 1.4, opacity: 0.6, dash: [3, 5], z: 2 })
        }
      })
    } else if (it.collapsed) {
      const y1 = it.y + LIST_TOP + GROUP_H / 2
      columns.forEach((a, k) => {
        // A collapsed group's curve stands for its own links: what an agent reads through Agents draws none.
        const n = it.skills.filter((s) => ["on", "adding"].includes(cellState(s, a.name, pending))).length
        if (!n) return
        wires.push({ y1, k, seed: wireSeed(it.name + ">" + a.name), ink: "wire", width: 1 + n * 0.9, opacity: agentFocus && agentFocus !== a.name ? 0.08 : 0.4, z: 0 })
        labels.push({ y1, k, text: n })
      })
    }
  }
  wires.sort((a, b) => a.z - b.z)

  let dragLine: Scene["drag"] = null
  if (drag) {
    const it = items.find((x) => x.type === "skill" && x.skill.ref.key === drag.key)
    if (it) dragLine = { y1: it.y + LIST_TOP + ROW_H / 2, x: drag.x, y: drag.y }
  }

  // Moves the columns to the scroll position and redraws the curves; called on every scroll and render.
  const draw = React.useCallback(() => {
    const sc = scene.current
    const el = scroller.current
    const layer = layerRef.current
    const canvas = canvasRef.current
    if (!sc || !el || !layer || !canvas) return
    const top = Math.min(Math.max(el.scrollTop, 0), Math.max(0, sc.contentH - sc.viewH))
    // The agent column sticks to the top until the end of the list pushes it up (like position: sticky).
    const agentOffset = Math.min(top, Math.max(0, sc.contentH - sc.agentsH))
    layer.style.transform = `translateY(${-top}px)`
    if (agentsRef.current) agentsRef.current.style.transform = `translateY(${agentOffset}px)`

    const dpr = window.devicePixelRatio || 1
    const w = Math.round(sc.width * dpr)
    const h = Math.round(sc.viewH * dpr)
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w
      canvas.height = h
    }
    const ctx = canvas.getContext("2d")
    if (!ctx) return
    if (inks.current?.style !== document.documentElement.dataset.style) inks.current = readInks()
    const ink = inks.current!
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, sc.width, sc.viewH)
    ctx.lineCap = "round"
    ctx.lineJoin = "round"

    const { x1, x2 } = sc
    const bend = bendOf(x1, x2)
    const ay = (k: number) => AGENTS_TOP + k * AGENT_H + AGENT_H / 2 + agentOffset - top
    for (const wire of sc.wires) {
      const y1 = wire.y1 - top
      const y2 = ay(wire.k)
      // A curve stays between its ends' heights (plus the wobble): skip the ones outside the view.
      if (Math.max(y1, y2) < -8 || Math.min(y1, y2) > sc.viewH + 8) continue
      ctx.beginPath()
      traceCubic(ctx, sc.wobble, wire.seed, x1, y1, x1 + bend, y1, x2 - bend, y2, x2, y2)
      ctx.strokeStyle = ink[wire.ink]
      ctx.lineWidth = wire.width || ink.wireW
      ctx.globalAlpha = wire.opacity < 0 ? ink.wireOp : wire.opacity
      ctx.setLineDash(wire.dash ?? [])
      ctx.stroke()
    }
    ctx.globalAlpha = 1
    ctx.setLineDash([])
    if (sc.labels.length) {
      ctx.fillStyle = ink.hueInk
      ctx.font = `10.5px ${ink.font}`
      ctx.textAlign = "center"
      for (const l of sc.labels) {
        const y1 = l.y1 - top
        ctx.fillText(String(l.text), x1 + 60, y1 - 6 + (ay(l.k) - y1) * 0.08)
      }
    }
    if (sc.drag) {
      const { y1, x, y } = sc.drag
      ctx.beginPath()
      traceCubic(ctx, sc.wobble, 0, x1, y1 - top, x1 + 120, y1 - top, x - 120, y - top, x, y - top)
      ctx.strokeStyle = ink.ac
      ctx.lineWidth = 2.2
      ctx.setLineDash([6, 5])
      ctx.stroke()
    }
  }, [scroller])

  React.useLayoutEffect(() => {
    scene.current = { wires, labels, drag: dragLine, x1: g.x1, x2: g.x2, width: g.width, contentH, agentsH, viewH, wobble }
    draw()
  })
  // The style's colours land on <html> in the provider's layout effect, after this view's: draw again once they have.
  React.useEffect(() => {
    draw()
    void document.fonts?.ready.then(draw)
  }, [style, draw])

  // Moving the window to a screen with another pixel ratio resizes nothing, so redraw at the new ratio.
  React.useEffect(() => {
    let mq: MediaQueryList
    const watch = () => {
      mq = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
      mq.addEventListener("change", onChange, { once: true })
    }
    const onChange = () => {
      draw()
      watch()
    }
    watch()
    return () => mq.removeEventListener("change", onChange)
  }, [draw])

  React.useEffect(() => {
    const el = scroller.current
    if (!el) return
    let timer = 0
    const onScroll = () => {
      draw()
      // Rows slide under a still pointer while scrolling: drop the hover (and its orange curves) and take no new one
      // until the scroll stops. Only the hover: a click right after a scroll still selects its row (switching pointer
      // events off swallowed it, and space then opened the skill selected before).
      if (scene.current?.drag) return
      if (!timer) setHover(null)
      scrolling.current = true
      clearTimeout(timer)
      timer = window.setTimeout(() => {
        timer = 0
        scrolling.current = false
      }, 120)
    }
    el.addEventListener("scroll", onScroll, { passive: true })
    return () => {
      el.removeEventListener("scroll", onScroll)
      clearTimeout(timer)
      scrolling.current = false
    }
  }, [scroller, draw, setHover])

  const focusText = focusSkill
    ? `${focusSkill.linkName} → ${columns.filter((a) => isOn(focusSkill, a.name, pending)).map(agentLabel).join(" · ") || "—"}`
    : ""

  return (
    // The spacer gives the scroller its height; the pinned frame inside shows the view.
    <div style={{ height: contentH, width: g.width }}>
      <div className="sticky top-0 overflow-clip" style={{ height: viewH }}>
        <canvas ref={canvasRef} aria-hidden className="pointer-events-none absolute top-0 left-0" style={{ width: g.width, height: viewH }} />
        <div ref={layerRef} className={cn("relative", drag && "cursor-grabbing select-none")} style={{ height: contentH }}>
          {/* Agents: held at the top of the view so the curves always have somewhere to land. */}
          <section ref={agentsRef} aria-label={t("agentsTitle")} className="absolute top-0 z-[2] pt-3" style={{ left: g.x2, width: g.agentsW }}>
            <div className="mb-1.5 flex h-[25px] items-baseline gap-2.5 border-b-row px-3.5 font-body fs-meta text-mut">
              <span className="font-semibold text-ink">Agent</span>
              <span className="tabular-nums">{columns.length}</span>
              <span className="flex-1" />
              <span className="truncate" title={selSkill ? t("wireAgentHintSel") : t("wireAgentHint")}>
                {selSkill ? t("wireAgentHintSel") : t("wireAgentHint")}
              </span>
            </div>
            {columns.map((a) => (
              <AgentRow key={a.name} agent={a} skill={selSkill} dragging={!!drag} compact={g.agentsW < 340} claudeOff={!!selSkill && a.name === "claude" && overrides.has(selSkill.name)} />
            ))}
          </section>

          <section aria-label="Skill" className="absolute top-0" style={{ left: g.padX, width: g.listW }}>
            <div className="mt-3 mb-1.5 flex h-[25px] items-baseline gap-2.5 border-b-row px-3.5 font-body fs-meta text-mut">
              <span className="font-semibold text-ink">Skill</span>
              <span className="tabular-nums">{count}</span>
              <span className="flex-1" />
              <span className="min-w-0 truncate" aria-live="polite" title={focusText}>
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
                    tabStop={it.skill.ref.key === tabKey}
                    update={updatable.has(it.skill.ref.key)}
                    sourceText={sourceText(t, it.skill)}
                    // A second click on the selected skill clears the selection (and its orange curves).
                    onSelect={(e) => e.detail < 2 && select(it.skill.ref.key === selected ? null : it.skill.ref.key)}
                    onOpen={() => {
                      select(it.skill.ref.key)
                      openDetail(it.skill.ref.key)
                    }}
                    onEnter={() => {
                      if (!scrolling.current) setHover(it.skill.ref.key)
                    }}
                    onLeave={() => setHover(null)}
                    onPortDown={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      const r = layerRef.current?.getBoundingClientRect()
                      select(it.skill.ref.key)
                      if (r) setDrag({ key: it.skill.ref.key, x: e.clientX - r.left, y: e.clientY - r.top })
                    }}
                  />
                ),
              )}
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}

function AgentRow({ agent: a, skill, dragging, claudeOff, compact }: { agent: Agent; skill: Skill | null; dragging: boolean; claudeOff: boolean; compact: boolean }) {
  const { t, skills, pending, togglePending, agentFocus, setAgentFocus, applying } = useApp()
  const count = skills.filter((s) => isOn(s, a.name, pending)).length
  const selA = agentFocus === a.name
  const st = skill ? cellState(skill, a.name, pending) : "off"
  const on = st === "on" || st === "adding" || st === "absent" || st === "via"
  // Read from ~/.agents/skills: no link of its own, no curve, and the button says where it comes from.
  const via = st === "via"
  const away = isAbsentAgent(a)
  // Off for an agent this Mac does not have: shown as such and refused when pressed (app-state says why).
  const locked = st === "off" && away
  const pend = st === "adding" || st === "removing"
  const label = agentLabel(a)
  const btnText = via ? t(skill?.targets.includes(a.name) ? "cellViaOwn" : "cellVia", { agent: label }) : locked ? t("absentCannotInstall", { agent: label }) : st === "adding" ? t("wireAdding") : st === "absent" ? t("cellAbsent", { agent: label }) : st === "on" ? (a.enabled ? t("wireOn") : t("cellPaused", { agent: label })) : st === "removing" ? (skill && keepsViaShared(skill, a.name, pending) ? t("cellViaCancel", { agent: label }) : t("wireRemoving")) : t("wireOff")
  return (
    <div
      data-agent={a.name}
      className={cn(
        "group/agent relative flex h-[72px] items-center gap-3 border-b-row pr-3.5 pl-[22px] transition-opacity",
        (selA || dragging) && "rounded-ctl bg-row-on",
        selA && "shadow-row",
        ((agentFocus && !selA) || !a.enabled || away) && "opacity-55",
      )}
    >
      <span
        aria-hidden
        className="absolute top-1/2 -left-[7px] -mt-1.5 size-3 rounded-full transition-shadow"
        style={{ background: (on && !via) || selA ? "var(--ac)" : "var(--hue)", boxShadow: dragging ? "0 0 0 4px var(--acSoft)" : "none" }}
      />
      <AgentMono agent={a} size={34} />
      {/* The shared folder says who reads it (several agents, not Claude Code). */}
      <Hint label={a.name === "agents" ? `${t("sharedDirNote")} · ${t("agentFocusHint", { agent: label })}` : t("agentFocusHint", { agent: label })}>
        <button
          type="button"
          aria-pressed={selA}
          onClick={() => setAgentFocus(selA ? null : a.name)}
          className="min-w-0 flex-1 rounded-ctl text-left after:absolute after:inset-0 after:content-[''] focus-visible:outline-none focus-visible:after:rounded-ctl focus-visible:after:outline-2 focus-visible:after:outline-ring"
        >
          <span className="flex items-center gap-1.5 font-heading fs-name leading-[1.1] font-semibold whitespace-nowrap text-ink">
            <span className="truncate">{label}</span>
            {!a.enabled && <Tag>{t("pausedTag")}</Tag>}
            {away && <Tag>{t("notInstalled")}</Tag>}
            {claudeOff && <OffRing />}
          </span>
          <span className="mt-[3px] block truncate font-mono fs-meta text-mut">{tildify(a.path)}</span>
        </button>
      </Hint>
      {skill &&
        (() => {
          // The same switch as the detail sheet: on is "this agent gets it", whatever the way (its own link, or
          // the Agents folder it reads). Read through Agents it stays on and cannot be switched off here, unless
          // the agent was also chosen itself (then off takes back that choice). Pending changes say so in orange.
          const checked = skill ? isOn(skill, a.name, pending) : false
          const fixed = locked || (via && !skill.targets.includes(a.name))
          const note = st === "adding" ? t("wireAdding") : st === "removing" ? (keepsViaShared(skill, a.name, pending) ? t("viaCancelShort") : t("wireRemoving")) : via ? t("viaState") : null
          const explain = via || locked || st === "absent" || st === "removing" || (st === "on" && !a.enabled)
          const control = (
            <span className="relative z-[1] flex items-center gap-2">
              {note && !compact && <span className={cn("fs-meta whitespace-nowrap", pend ? "text-ac" : "text-mut")}>{note}</span>}
              <Switch
                checked={checked}
                // Toggles are ignored while pending changes are being applied: say so by greying the switch.
                disabled={fixed || applying}
                aria-label={`${skill.linkName} · ${label} (${a.name}) · ${btnText}`}
                onCheckedChange={() => togglePending(skill, a.name)}
                className={cn(pend && "ring-2 ring-ac/45 ring-offset-1 ring-offset-bg")}
              />
            </span>
          )
          return explain || compact ? <Hint label={btnText}>{control}</Hint> : control
        })()}
      <span className="min-w-[30px] flex-none text-right text-[18px] tabular-nums leading-none font-medium text-hue-ink">{count}</span>
    </div>
  )
}

function GroupRow({ item, onToggle }: { item: Extract<ListItem, { type: "group" }>; onToggle: () => void }) {
  const { t } = useApp()
  return (
    <div role="listitem" className="absolute inset-x-0 flex items-center" style={{ top: item.y, height: GROUP_H }}>
      <button
        type="button"
        aria-expanded={!item.collapsed}
        onClick={onToggle}
        className="relative flex size-full items-center gap-2 rounded-ctl px-4 text-left"
      >
        <DisclosureChevron open={!item.collapsed} />
        <span className="fs-meta font-medium text-ink">{item.name}</span>
        <span className="font-body fs-meta text-mut">{t("groupCount", { n: item.skills.length })}</span>
        {item.collapsed && <span aria-hidden className="absolute top-1/2 -right-1.5 -mt-1.5 size-3 rounded-full bg-[var(--wire)]" />}
      </button>
      <span className="absolute right-6">
        <GroupBatchMenu group={item.name} skills={item.skills} />
      </span>
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
  tabStop,
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
  tabStop: boolean
  update: boolean
  sourceText: string
  onSelect: (e: React.MouseEvent) => void
  onOpen: () => void
  onEnter: () => void
  onLeave: () => void
  onPortDown: (e: React.PointerEvent) => void
}) {
  const { t } = useApp()
  return (
    <div role="listitem" className="group/row absolute inset-x-0" style={{ top: y, height: ROW_H }} onMouseEnter={onEnter} onMouseLeave={onLeave}>
      <button
        id={id}
        type="button"
        tabIndex={tabStop ? 0 : -1}
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
            {hasConflicts(skill) && <ConflictMark />}
            {skill.edited && <EditedMark skill={skill} />}
            {skill.contribution && <ContributionMark contribution={skill.contribution} own={skill.own} />}
          </span>
          <span className="mt-0.5 block truncate fs-meta text-mut">{sourceText}</span>
        </span>
        <Hint label={t("installedOfTotal")}>
          <span className="fs-meta whitespace-nowrap text-hue-ink tabular-nums">
            {count}/{total}
          </span>
        </Hint>
        <Hint label={t("dragToInstall")}>
          <span
            aria-hidden
            onPointerDown={onPortDown}
            // The press already selected the skill; its click must not clear it again.
            onClick={(e) => e.stopPropagation()}
            className="absolute top-1/2 -right-1.5 -mt-1.5 box-border size-3 cursor-grab rounded-full border-2"
            style={{ background: focused ? "var(--ac)" : "var(--bg)", borderColor: focused ? "var(--ac)" : "var(--hue)" }}
          />
        </Hint>
      </button>
      {/* Over the row's right end, left of the count; the row's own background keeps a long name from showing through. */}
      <RowActions skill={skill} shown={selected || focused} tinted={focused} className={cn("absolute top-1/2 right-[62px] -translate-y-1/2 rounded-ctl", dim && "opacity-40")} />
    </div>
  )
}
