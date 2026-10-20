import * as React from "react"
import { cn, shortSha } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import { agentMono } from "@/lib/agents"
import type { Agent } from "@/lib/backend"
import { RotateCwIcon } from "lucide-react"
import { useReducedMotion } from "motion/react"
import { SlidingNumber } from "@/components/animate-ui/primitives/texts/sliding-number"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

/** Accent dot: an update is available, or something waits for you. */
export function UpdateDot({ className, label, decorative }: { className?: string; label?: string; decorative?: boolean }) {
  const { t } = useApp()
  const cls = cn("inline-block size-1.5 shrink-0 rounded-full bg-ac", className)
  if (decorative) return <span aria-hidden className={cls} />
  return <span role="img" aria-label={label ?? t("updateMarker")} title={label ?? t("updateMarker")} className={cls} />
}

/** Hollow ring: turned off inside Claude Code (read-only). */
export function OffRing({ className, label }: { className?: string; label?: string }) {
  const { t } = useApp()
  return (
    <span
      role="img"
      aria-label={label ?? t("claudeOffMarker")}
      title={label ?? t("claudeOffMarker")}
      className={cn("inline-block size-[7px] shrink-0 rounded-full border-[1.5px] border-ac bg-bg", className)}
    />
  )
}

/** Small tag next to a name ("本地", "已暂停", "示例"). */
export function Tag({ children, className, tone }: { children: React.ReactNode; className?: string; tone?: "accent" }) {
  return <span className={cn("skd-tag", tone === "accent" && "bg-ac text-ac-ink", className)}>{children}</span>
}

export function ExampleTag() {
  const { t } = useApp()
  return <Tag>{t("example")}</Tag>
}

/** Agent monogram tile ("CC"). */
export function AgentMono({
  agent,
  size = 34,
  className,
}: {
  agent: Pick<Agent, "name">
  /** Square size, or [width, height]. */
  size?: number | [number, number]
  className?: string
}) {
  const [w, h] = Array.isArray(size) ? size : [size, size]
  const font = h >= 34 ? 11.5 : h >= 28 ? 11 : h >= 22 ? 10.5 : 9.5
  return (
    <span aria-hidden className={cn("skd-mono", className)} style={{ width: w, height: h, fontSize: font }}>
      {agentMono(agent)}
    </span>
  )
}

/** A commit id: 7 digits on screen, the full id as the tooltip. */
export function Sha({ id, className }: { id: string; className?: string }) {
  return (
    <Mono className={className} title={id}>
      {shortSha(id)}
    </Mono>
  )
}

export function Mono({ children, className, title }: { children: React.ReactNode; className?: string; title?: string }) {
  return (
    <span className={cn("font-mono fs-meta", className)} title={title}>
      {children}
    </span>
  )
}

/** Tooltip on a control. wide: for a paragraph rather than a label. */
export function Hint({ label, children, wide }: { label: string; children: React.ReactElement; wide?: boolean }) {
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent className={wide ? "max-w-md" : undefined}>{label}</TooltipContent>
    </Tooltip>
  )
}

const SLOT = "\u0000"

/**
 * A message whose number rolls to its new value (Animate UI sliding number),
 * e.g. "3 处改动待应用" or "12 秒". `message` gets a placeholder for the number
 * and returns the translated text. Screen readers get the plain number; with
 * reduced motion the number just changes.
 */
export function RollingText({ message, value, className }: { message: (slot: string) => string; value: number; className?: string }) {
  const reduce = useReducedMotion()
  const [before, after = ""] = message(SLOT).split(SLOT)
  return (
    <span className={className}>
      {before}
      <span className="sr-only">{value}</span>
      {reduce ? <span aria-hidden>{value}</span> : <SlidingNumber aria-hidden number={value} className="tabular-nums" />}
      {after}
    </span>
  )
}

/**
 * The design's circular arrow (Lucide rotate-cw): the shadcn spinner while
 * `spinning`, a still icon otherwise. Always next to a text label, so hidden
 * from assistive technology.
 */
export function RefreshIcon({ spinning, className }: { spinning?: boolean; className?: string }) {
  if (spinning) return <Spinner aria-hidden role={undefined} aria-label={undefined} className={className} />
  return <RotateCwIcon aria-hidden strokeWidth={1.8} className={cn("size-[13px] shrink-0", className)} />
}

/**
 * Page frame below the top bar: big title, a one-line summary and actions on
 * the right (the design's sources / agent / history / trash / settings pages).
 */
export function Page({
  title,
  summary,
  actions,
  children,
  footer,
  className,
}: {
  title: string
  summary?: React.ReactNode
  actions?: React.ReactNode
  children: React.ReactNode
  footer?: React.ReactNode
  className?: string
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <main className={cn("min-h-0 flex-1 overflow-y-auto px-10 pt-[26px] pb-10 doodle:relative doodle:z-[6] doodle:pb-14 max-[760px]:px-5", className)} aria-label={title}>
        <div className="mb-[18px] flex flex-wrap items-end gap-x-3.5 gap-y-2">
          <h1 className="m-0 font-heading fs-h1 leading-none font-semibold tracking-[-0.01em] text-ink">{title}</h1>
          {summary && <div className="flex min-w-0 items-center gap-1.5 pb-0.5 fs-body text-mut">{summary}</div>}
          {actions && <div className="ml-auto flex shrink-0 items-center gap-2">{actions}</div>}
        </div>
        {children}
      </main>
      {footer}
    </div>
  )
}

/** Header row of a page table: 30px, strong rule, small muted labels. */
export function TableHead({ className, children, style }: { className?: string; children: React.ReactNode; style?: React.CSSProperties }) {
  return (
    <div role="row" className={cn("grid h-[31px] items-center border-b-head px-2.5 fs-meta text-mut", className)} style={style}>
      {children}
    </div>
  )
}

export function PageError({ onRetry }: { onRetry: () => void }) {
  const { t } = useApp()
  return (
    <div role="alert" className="py-10 fs-body text-mut">
      <p className="mb-1 font-heading fs-h3 text-ink">{t("pageLoadFailed")}</p>
      <Button variant="outline" onClick={onRetry} className="mt-3">
        {t("retry")}
      </Button>
    </div>
  )
}

/** The design's empty state (shadcn Empty, restyled): a title in the heading font and one line of help. */
export function EmptyNote({ title, children, action }: { title: string; children?: React.ReactNode; action?: React.ReactNode }) {
  return (
    <Empty>
      <EmptyHeader>
        <EmptyTitle>{title}</EmptyTitle>
        {children && <EmptyDescription>{children}</EmptyDescription>}
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  )
}
