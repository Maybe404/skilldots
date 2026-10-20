import * as React from "react"
import { FileMinus2, FilePen, FilePlus2 } from "lucide-react"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import type { FileDiff, SkillDiff } from "@/lib/backend"
import type { MessageKey } from "@/lib/i18n"
import { Tag } from "@/components/app/bits"
import { Collapsible, CollapsiblePanel, CollapsibleTrigger, DisclosureChevron } from "@/components/ui/collapsible"

/*
 * What an update would change in one skill, file by file: a unified diff
 * per file in a collapsible (Animate UI's Base UI collapsible), the files
 * you edited marked, and a note when the whole folder goes to the trash
 * first. Written for this app: the one diff component in the collection
 * (Beautiful UI's code block) brings its own red/green/orange tokens and a
 * JavaScript highlighter, neither of which fits SKILL.md or the palette.
 */

const STATUS: Record<FileDiff["status"], [MessageKey, typeof FilePlus2]> = {
  added: ["fileAdded", FilePlus2],
  removed: ["fileRemoved", FileMinus2],
  modified: ["fileModified", FilePen],
}

/** Note shown when update moves the skill's folder to the trash: which files you changed. */
export function LocalEditNote({ diff, className }: { diff: SkillDiff; className?: string }) {
  const { t } = useApp()
  if (!diff.local) return null
  return (
    <div className={cn("rounded-ctl bg-ac/10 px-3 py-2 fs-meta text-ink", className)}>
      <p>{t("localEditedHint")}</p>
      <p className="mt-1 text-mut">
        {diff.localUnknown ? (
          t("localFilesUnknown")
        ) : (
          <>
            {t("localFilesLine", { files: "" })}
            {diff.localFiles.map((f, i) => (
              <React.Fragment key={f}>
                {i > 0 && t("listSep")}
                <span className="font-mono text-ink">{f}</span>
              </React.Fragment>
            ))}
          </>
        )}
      </p>
    </div>
  )
}

export function SkillDiffView({ diff }: { diff: SkillDiff }) {
  const { t } = useApp()
  return (
    <div className="space-y-2">
      <LocalEditNote diff={diff} />
      {diff.files.length === 0 && diff.status === "moved" && <p className="fs-meta text-mut">{t("movedOnlyFiles")}</p>}
      <div className="overflow-hidden rounded-ctl border-line">
        {diff.files.map((f, i) => (
          <FileBlock key={f.path} file={f} defaultOpen={f.path === "SKILL.md" || (i === 0 && diff.files.length <= 2)} first={i === 0} />
        ))}
      </div>
      {!!diff.moreFiles && <p className="fs-meta text-mut">{t("moreFiles", { n: diff.moreFiles })}</p>}
    </div>
  )
}

function FileBlock({ file: f, defaultOpen, first }: { file: FileDiff; defaultOpen: boolean; first: boolean }) {
  const { t } = useApp()
  const [open, setOpen] = React.useState(defaultOpen)
  const [label, Icon] = STATUS[f.status]
  return (
    <Collapsible open={open} onOpenChange={setOpen} className={cn(!first && "[border-top:1.5px_solid_var(--ln)]")}>
      <CollapsibleTrigger className="flex h-8 w-full items-center gap-2 px-2.5 text-left hover:bg-row-hv">
        <DisclosureChevron open={open} />
        <Icon aria-hidden className="size-3.5 shrink-0 text-mut" strokeWidth={1.8} />
        <span className="min-w-0 truncate font-mono fs-meta text-ink">{f.path}</span>
        <span className="sr-only">{t(label)}</span>
        {f.local && <Tag tone="accent">{t("fileLocal")}</Tag>}
        <span className="flex-1" />
        <span className="shrink-0 fs-meta text-mut tabular-nums">
          {f.added > 0 && <span className="text-ink">+{f.added}</span>}
          {f.added > 0 && f.removed > 0 && " "}
          {f.removed > 0 && <span>−{f.removed}</span>}
        </span>
      </CollapsibleTrigger>
      <CollapsiblePanel>
        {f.binary || f.tooLarge ? (
          <p className="px-3 pb-2.5 fs-meta text-mut">{f.binary ? t("fileBinary") : t("fileTooLarge")}</p>
        ) : (
          <Hunks file={f} />
        )}
      </CollapsiblePanel>
    </Collapsible>
  )
}

/** The hunks of one file: old and new line numbers, the sign, the text (in the editor's code font). */
function Hunks({ file }: { file: FileDiff }) {
  return (
    <div className="max-h-[22rem] overflow-auto [border-top:1px_solid_var(--ln)] font-[family-name:var(--fmCode)] text-[11.5px] leading-[1.6]">
      {file.hunks.map((h, hi) => {
        let o = h.oldStart
        let n = h.newStart
        return (
          <div key={hi} role="group" aria-label={`@@ -${h.oldStart},${h.oldLines} +${h.newStart},${h.newLines} @@`}>
            {hi > 0 && <div aria-hidden className="h-2 bg-nav" />}
            {h.lines.map((l, li) => {
              const oldNo = l.op === "+" ? "" : o++
              const newNo = l.op === "-" ? "" : n++
              return (
                <div
                  key={li}
                  className={cn("grid min-w-max grid-cols-[2.5rem_2.5rem_1.25rem_1fr] pr-3", l.op === "+" && "bg-[var(--diffAdd)]", l.op === "-" && "bg-[var(--diffDel)]")}
                >
                  <span className="pr-2 text-right text-mut select-none">{oldNo}</span>
                  <span className="pr-2 text-right text-mut select-none">{newNo}</span>
                  <span aria-label={l.op === "+" ? "+" : l.op === "-" ? "−" : undefined} className={cn("text-center select-none", l.op === "-" ? "text-ac" : "text-ink")}>
                    {l.op === " " ? "" : l.op === "-" ? "−" : "+"}
                  </span>
                  <span className="whitespace-pre text-ink">{l.text || " "}</span>
                </div>
              )
            })}
          </div>
        )
      })}
    </div>
  )
}
