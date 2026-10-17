import * as React from "react"
import { cn } from "@/lib/utils"
import { kindLabel, useApp } from "@/lib/app-state"
import type { Skill } from "@/lib/backend"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

/** Filled dot: an update is available. Shape, not colour. */
export function UpdateDot({ className, label, decorative }: { className?: string; label?: string; decorative?: boolean }) {
  const { t } = useApp()
  if (decorative) return <span aria-hidden className={cn("inline-block size-1.5 shrink-0 rounded-full bg-foreground", className)} />
  return (
    <span role="img" aria-label={label ?? t("updateMarker")} title={label ?? t("updateMarker")} className={cn("inline-block size-1.5 shrink-0 rounded-full bg-foreground", className)} />
  )
}

/** Hollow ring: turned off inside Claude Code (read-only). */
export function OffRing({ className, label }: { className?: string; label?: string }) {
  const { t } = useApp()
  return (
    <span
      role="img"
      aria-label={label ?? t("claudeOffMarker")}
      title={label ?? t("claudeOffMarker")}
      className={cn("inline-block size-2 shrink-0 rounded-full border-[1.5px] border-foreground/70", className)}
    />
  )
}

export function KindBadge({ skill, className }: { skill: Skill; className?: string }) {
  const { t } = useApp()
  if (skill.kind === "vendor") return <span className={cn("truncate text-muted-foreground", className)}>{skill.origin}</span>
  return (
    <Badge variant={skill.kind === "skills" ? "secondary" : "outline"} className={cn("font-normal", className)}>
      {kindLabel(t, skill)}
    </Badge>
  )
}

export function Mono({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={cn("font-mono text-[0.8rem] tracking-tight", className)}>{children}</span>
}

/** Small "示例" tag for fictional sample entries. */
export function ExampleTag() {
  const { t } = useApp()
  return (
    <Badge variant="outline" className="h-4.5 px-1.5 text-[0.7rem] font-normal text-muted-foreground">
      {t("example")}
    </Badge>
  )
}

export function Hint({ label, children }: { label: string; children: React.ReactElement }) {
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

/** Page header inside the content panel. */
export function PageHeader({ title, hint, actions }: { title: string; hint?: string; actions?: React.ReactNode }) {
  return (
    <div className="flex items-end justify-between gap-6 px-8 pt-7 pb-5">
      <div className="min-w-0">
        <h1 className="text-[1.35rem] leading-tight font-semibold tracking-tight">{title}</h1>
        {hint && <p className="mt-1.5 max-w-prose text-sm text-muted-foreground">{hint}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  )
}

export function Section({ title, hint, children, className }: { title: string; hint?: string; children: React.ReactNode; className?: string }) {
  return (
    <section className={cn("px-8 pb-8", className)}>
      <div className="mb-3">
        <h2 className="text-sm font-semibold">{title}</h2>
        {hint && <p className="mt-0.5 text-[0.8rem] text-muted-foreground">{hint}</p>}
      </div>
      {children}
    </section>
  )
}
