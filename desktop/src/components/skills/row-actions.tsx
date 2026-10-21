import { GitMerge, Info, PencilLine } from "lucide-react"

import { cn } from "@/lib/utils"
import { hasConflicts, useApp } from "@/lib/app-state"
import type { Skill } from "@/lib/backend"
import { Hint } from "@/components/app/bits"
import { Button } from "@/components/ui/button"

/**
 * Details and Edit on a skill row of the Skill page (wire and table views),
 * so editing is one click from the list instead of behind the detail sheet.
 * A skill whose update is shelved offers 解决冲突 there instead (editing
 * stays in the detail sheet). Shown while the row is hovered, holds focus,
 * or is the selected one; the parent row carries `group/row`. Clicks stay
 * here: they neither select nor open the row.
 */
export function RowActions({ skill, shown, tinted, className }: { skill: Skill; shown: boolean; tinted?: boolean; className?: string }) {
  const { t, openEditor, openResolve, openDetail, select } = useApp()
  const conflicts = hasConflicts(skill)
  return (
    <span
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
      className={cn(
        // Opaque under the focused row's tint (a see-through hatch in the doodle style), so a long name or a tag
        // under the buttons does not show through.
        "bg-bg transition-opacity",
        shown ? "opacity-100" : "pointer-events-none opacity-0 group-hover/row:pointer-events-auto group-hover/row:opacity-100 group-focus-within/row:pointer-events-auto group-focus-within/row:opacity-100",
        className,
      )}
    >
      <span className={cn("flex items-center gap-1 rounded-[inherit] py-1 pl-2", tinted && "bg-row-on")}>
        <Hint label={t("details")}>
          <Button
            variant="ghost"
            size="icon-sm"
            tabIndex={shown ? 0 : -1}
            aria-label={`${skill.linkName} · ${t("details")}`}
            onClick={() => {
              select(skill.ref.key)
              openDetail(skill.ref.key)
            }}
          >
            <Info />
          </Button>
        </Hint>
        {conflicts ? (
          <Button
            variant="outline"
            size="sm"
            tabIndex={shown ? 0 : -1}
            aria-label={`${skill.linkName} · ${t("resolveConflicts")}`}
            onClick={() => {
              select(skill.ref.key)
              openResolve(skill.ref)
            }}
          >
            <GitMerge /> {t("resolveConflicts")}
          </Button>
        ) : (
          <Button
            variant="outline"
            size="sm"
            tabIndex={shown ? 0 : -1}
            aria-label={`${skill.linkName} · ${t("edit")}`}
            onClick={() => {
              select(skill.ref.key)
              openEditor(skill.ref)
            }}
          >
            <PencilLine /> {t("edit")}
          </Button>
        )}
      </span>
    </span>
  )
}
