import { Info, PencilLine } from "lucide-react"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import type { Skill } from "@/lib/backend"
import { Hint } from "@/components/app/bits"
import { Button } from "@/components/ui/button"

/**
 * Details and Edit on a skill row of the Skill page (wire and table views),
 * so editing is one click from the list instead of behind the detail sheet.
 * Shown while the row is hovered, holds focus, or is the selected one; the
 * parent row carries `group/row`. Clicks stay here: they neither select nor
 * open the row.
 */
export function RowActions({ skill, shown, className }: { skill: Skill; shown: boolean; className?: string }) {
  const { t, openEditor, openDetail, select } = useApp()
  return (
    <span
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
      className={cn(
        "flex items-center gap-1 transition-opacity",
        shown ? "opacity-100" : "pointer-events-none opacity-0 group-hover/row:pointer-events-auto group-hover/row:opacity-100 group-focus-within/row:pointer-events-auto group-focus-within/row:opacity-100",
        className,
      )}
    >
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
    </span>
  )
}
