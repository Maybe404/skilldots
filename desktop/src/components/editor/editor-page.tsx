import * as React from "react"
import { toast } from "sonner"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { useReducedMotion } from "motion/react"
import { ArrowLeft, ChevronDown, FileCode2, FileText, FolderOpen, PanelRightClose, PanelRightOpen, Save } from "lucide-react"

import { cn } from "@/lib/utils"
import { PLACEHOLDER_DESC, hasConflicts, kindLabel, useApp, type View } from "@/lib/app-state"
import { backend, type FileNode, type Ref, type Skill } from "@/lib/backend"
import { parseSkillMd, serializeSkillMd } from "@/lib/frontmatter"
import { useSkillActions } from "@/lib/use-actions"
import { CodeEditor, languageName } from "@/components/editor/code-editor"
import { RefreshIcon } from "@/components/app/bits"
import { Files, FileItem, FolderItem, FolderPanel, FolderTrigger } from "@/components/animate-ui/components/base/files"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Collapsible, CollapsiblePanel, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Kbd } from "@/components/ui/kbd"
import { Textarea } from "@/components/ui/textarea"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

interface Buffer {
  saved: string
  current: string
}

const isMarkdown = (p: string) => /\.(md|markdown)$/i.test(p)

export function EditorPage({ skillRef, file = "SKILL.md", back = "skills" }: { skillRef: Ref; file?: string; back?: View }) {
  const { t, skills, run, reload, navigate, setLeaveGuard, openDialog } = useApp()
  const { reveal } = useSkillActions()
  const skill = skills.find((s) => s.ref.key === skillRef.key)
  const [tree, setTree] = React.useState<FileNode[] | null>(null)
  const [buffers, setBuffers] = React.useState<Record<string, Buffer>>({})
  const [active, setActive] = React.useState("SKILL.md")
  const [preview, setPreview] = React.useState(() => typeof window === "undefined" || window.innerWidth >= 1100)
  const [metadataOpen, setMetadataOpen] = React.useState(false)
  const [cursor, setCursor] = React.useState({ line: 1, col: 1 })
  const [saving, setSaving] = React.useState(false)
  // Resolving a merge edits the files with conflict markers: the vendored copy is already yours to change.
  const needsAck = (skill?.kind === "vendor" || skill?.kind === "app") && !hasConflicts(skill)
  const [acked, setAcked] = React.useState(false)
  const [leaving, setLeaving] = React.useState<(() => void) | null>(null)

  // Load the file list and SKILL.md.
  React.useEffect(() => {
    let live = true
    setTree(null)
    setBuffers({})
    setActive(file)
    setAcked(false)
    ;(async () => {
      const r = await run(() => backend.files(skillRef))
      if (!live || !r.ok) return
      setTree(r.value)
      const loaded: Record<string, Buffer> = {}
      for (const path of new Set(["SKILL.md", file])) {
        const f = await run(() => backend.readFile(skillRef, path))
        if (!live) return
        if (f.ok) loaded[path] = { saved: f.value, current: f.value }
      }
      setBuffers(loaded)
    })()
    return () => {
      live = false
    }
  }, [skillRef, file, run])

  const open = async (path: string) => {
    setActive(path)
    if (buffers[path]) return
    const r = await run(() => backend.readFile(skillRef, path))
    if (r.ok) setBuffers((b) => ({ ...b, [path]: { saved: r.value, current: r.value } }))
  }

  const dirty = Object.entries(buffers).filter(([, b]) => b.current !== b.saved)
  const dirtySet = new Set(dirty.map(([p]) => p))
  const buf = buffers[active]
  const readOnly = needsAck && !acked

  const setCurrent = React.useCallback((path: string, current: string) => {
    setBuffers((b) => (b[path] ? { ...b, [path]: { ...b[path], current } } : b))
  }, [])

  const save = React.useCallback(async () => {
    const list = Object.entries(buffers).filter(([, b]) => b.current !== b.saved)
    if (!list.length) return true
    setSaving(true)
    for (const [path, b] of list) {
      const r = await run(() => backend.writeFile(skillRef, path, b.current))
      if (!r.ok) {
        setSaving(false)
        return false
      }
    }
    setBuffers((all) => {
      const next = { ...all }
      for (const [path, b] of list) next[path] = { saved: b.current, current: next[path]?.current ?? b.current }
      return next
    })
    setSaving(false)
    await reload()
    toast(t("saved"))
    return true
  }, [buffers, run, skillRef, reload, t])

  // ⌘S anywhere on the page.
  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault()
        if (!readOnly) void save()
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [save, readOnly])

  // Ask before leaving with unsaved changes: in-app navigation and window close.
  const hasDirty = dirty.length > 0
  React.useEffect(() => {
    setLeaveGuard(hasDirty ? (go) => (setLeaving(() => go), false) : null)
    const onUnload = (e: BeforeUnloadEvent) => {
      if (hasDirty) {
        e.preventDefault()
        e.returnValue = ""
      }
    }
    window.addEventListener("beforeunload", onUnload)
    return () => window.removeEventListener("beforeunload", onUnload)
  }, [hasDirty, setLeaveGuard])
  React.useEffect(() => () => setLeaveGuard(null), [setLeaveGuard])

  if (!skill) return null

  const doc = active === "SKILL.md" && buf ? parseSkillMd(buf.current) : null
  const editorValue = doc ? doc.body : (buf?.current ?? "")
  const onEditorChange = (v: string) => {
    if (!buf) return
    if (doc) setCurrent(active, serializeSkillMd({ ...parseSkillMd(buf.current), body: v }))
    else setCurrent(active, v)
  }
  const onDescription = (v: string) => {
    if (!buf || !doc) return
    setCurrent("SKILL.md", serializeSkillMd({ ...doc, description: v }))
  }
  const md = isMarkdown(active)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-[46px] shrink-0 items-center gap-2 px-10 font-body fs-body max-[760px]:px-5">
        <Button variant="ghost" onClick={() => navigate(back)} className="-ml-2.5">
          <ArrowLeft /> {t("back")}
        </Button>
        <div className="ml-1 flex min-w-0 items-baseline gap-2">
          <h1 className="truncate font-heading fs-name font-semibold text-ink">{t("editorTitle", { name: skill.linkName })}</h1>
          <span className="truncate fs-meta text-mut">{kindLabel(t, skill)}</span>
          {hasDirty && (
            <span className="flex items-center gap-1.5 fs-meta text-ink" role="status">
              <span className="size-1.5 rounded-full bg-ac" aria-hidden />
              {dirty.length > 1 ? t("unsavedFiles", { n: dirty.length }) : t("unsaved")}
            </span>
          )}
        </div>
        <div className="ml-auto flex items-center gap-1.5">
          <Button variant="ghost" onClick={() => reveal(skill.ref)}>
            <FolderOpen /> {t("revealInFinder")}
          </Button>
          <Button variant="ghost" aria-pressed={preview} onClick={() => setPreview((p) => !p)}>
            {preview ? <PanelRightClose /> : <PanelRightOpen />}
            {preview ? t("hidePreview") : t("showPreview")}
          </Button>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button onClick={() => void save()} disabled={!hasDirty || saving || readOnly} className="px-3">
                  {saving ? <RefreshIcon spinning /> : <Save />}
                  {t("save")}
                </Button>
              }
            />
            <TooltipContent>
              {t("save")} <Kbd>⌘S</Kbd>
            </TooltipContent>
          </Tooltip>
        </div>
      </div>

      {needsAck && acked && (
        <div className="flex items-center gap-2 border-b-row bg-row-hv px-10 py-1.5 fs-meta text-ink [border-left:3px_solid_var(--ac)]" role="note">
          {t("vendorBanner")}
        </div>
      )}

      <div className="flex min-h-0 flex-1 [border-top:var(--tblHd)]">
        <nav aria-label={t("files")} className="w-56 shrink-0 overflow-y-auto py-2 pl-7 [border-right:var(--tblRow)] lg:w-64">
          <p className="px-2 pt-1 pb-1 fs-meta text-mut">{t("files")}</p>
          {tree ? <FileTree nodes={tree} active={active} dirty={dirtySet} onOpen={open} /> : <p className="px-4 fs-body text-mut">{t("loading")}</p>}
        </nav>

        <section className="flex min-w-0 flex-1 flex-col lg:min-w-[30rem]">
          {doc && doc.hasFrontmatter && (
            <FrontmatterForm
              skill={skill}
              name={doc.name}
              description={doc.description}
              onDescription={onDescription}
              readOnly={readOnly}
              open={metadataOpen}
              onOpenChange={setMetadataOpen}
            />
          )}
          <div className="min-h-0 flex-1">
            {buf ? (
              <CodeEditor
                docId={`${skill.ref.key}/${active}`}
                path={active}
                value={editorValue}
                onChange={onEditorChange}
                onCursor={(line, col) => setCursor({ line, col })}
                readOnly={readOnly}
                dark={false}
                label={active}
              />
            ) : (
              <p className="p-4 fs-body text-mut">{t("loading")}</p>
            )}
          </div>
          <footer className="flex h-7 shrink-0 items-center gap-4 [border-top:var(--tblRow)] px-4 fs-meta text-mut">
            <span className="font-mono">{active}</span>
            <span>{languageName(active) ?? t("plainText")}</span>
            <span className="ml-auto tabular-nums">{t("lineCol", cursor)}</span>
          </footer>
        </section>

        {preview && (
          <aside aria-label={t("preview")} className="w-[36%] max-w-[38rem] min-w-[20rem] shrink-0 overflow-y-auto px-8 py-6 [border-left:var(--tblRow)]">
            {md && buf ? (
              <>
                {doc?.hasFrontmatter && (
                  <div className="mb-5 border-b-row pb-4">
                    <p className="fs-meta font-medium text-mut">{doc.name}</p>
                    <p className="mt-1 fs-body leading-relaxed text-ink">{doc.description}</p>
                  </div>
                )}
                <article className="prose-skill">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{doc ? doc.body : buf.current}</ReactMarkdown>
                </article>
              </>
            ) : (
              <p className="fs-body text-mut">{t("previewOnlyMd")}</p>
            )}
          </aside>
        )}
      </div>

      {/* vendor / app: warn before editing */}
      <AlertDialog open={needsAck && !acked && !!tree} onOpenChange={(o) => !o && !acked && navigate("skills")}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {skill.kind === "app" ? t("appWarnTitle", { name: skill.linkName, origin: skill.origin }) : t("vendorWarnTitle", { name: skill.linkName, origin: skill.origin })}
            </AlertDialogTitle>
            <AlertDialogDescription>{skill.kind === "app" ? t("appWarnBody") : t("vendorWarnBody")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => navigate("skills")}>{t("cancel")}</AlertDialogCancel>
            <Button variant="outline" size="lg" onClick={() => setAcked(true)}>
              {t("editAnyway")}
            </Button>
            <Button
              autoFocus
              variant="ink"
              size="lg"
              onClick={() => {
                navigate("skills")
                openDialog({ kind: "fork", ref: skill.ref, thenEdit: true })
              }}
            >
              {t("forkThenEdit")}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* unsaved changes */}
      <AlertDialog open={!!leaving} onOpenChange={(o) => !o && setLeaving(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("leaveTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("leaveBody")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
            <Button
              variant="outline"
              size="lg"
              onClick={() => {
                const go = leaving
                setLeaving(null)
                setLeaveGuard(null)
                go?.()
              }}
            >
              {t("discard")}
            </Button>
            <Button
              autoFocus
              variant="ink"
              size="lg"
              onClick={async () => {
                const go = leaving
                if (await save()) {
                  setLeaving(null)
                  setLeaveGuard(null)
                  go?.()
                }
              }}
            >
              {t("saveAndLeave")}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function FrontmatterForm({
  skill,
  name,
  description,
  onDescription,
  readOnly,
  open,
  onOpenChange,
}: {
  skill: Skill
  name: string
  description: string
  onDescription: (v: string) => void
  readOnly: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useApp()
  const placeholder = description.trim() === PLACEHOLDER_DESC
  const len = [...description].length
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} className="shrink-0 border-b-row">
      <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 px-5 py-2.5 text-left hover:bg-row-hv">
        <span className="min-w-0">
          <span className="block fs-meta text-mut">{t("frontmatter")}</span>
          <span className="mt-0.5 block truncate fs-body text-ink">{name || skill.name}</span>
        </span>
        <ChevronDown className={cn("size-4 shrink-0 transition-transform motion-reduce:transition-none", open && "rotate-180")} aria-hidden />
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="grid gap-3 [border-top:var(--tblRow)] px-5 py-4 lg:grid-cols-[14rem_1fr]">
          <Field>
            <FieldLabel htmlFor="fm-name">{t("nameLabel")}</FieldLabel>
            <Input id="fm-name" value={name || skill.name} readOnly className="bg-nav fs-body" aria-describedby="fm-name-hint" />
            <FieldDescription id="fm-name-hint" className="fs-meta">
              {t("nameReadonlyHint")}
            </FieldDescription>
          </Field>
          <Field data-invalid={placeholder || len > 1024 || undefined}>
            <div className="flex items-baseline justify-between">
              <FieldLabel htmlFor="fm-desc">{t("descLabel")}</FieldLabel>
              <span className={cn("fs-meta tabular-nums", len > 1024 ? "font-semibold text-ink" : "text-mut")}>{t("descCounter", { n: len })}</span>
            </div>
            <Textarea
              id="fm-desc"
              value={description}
              onChange={(e) => onDescription(e.target.value)}
              readOnly={readOnly}
              rows={3}
              placeholder={t("descPlaceholder")}
              className="min-h-[4.5rem] resize-y fs-body"
              aria-describedby="fm-desc-hint"
            />
            <FieldDescription id="fm-desc-hint" className="fs-meta">
              {placeholder ? t("placeholderWarn", { name: skill.name }) : len > 1024 ? t("descTooLong") : t("descHint")}
            </FieldDescription>
          </Field>
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}

function FileTree({ nodes, active, dirty, onOpen }: { nodes: FileNode[]; active: string; dirty: Set<string>; onOpen: (p: string) => void }) {
  const { t } = useApp()
  const reduce = useReducedMotion()
  const folders = React.useMemo(() => {
    const out: string[] = []
    const walk = (ns: FileNode[]) => ns.forEach((n) => n.dir && (out.push(n.path), walk(n.children ?? [])))
    walk(nodes)
    return out
  }, [nodes])

  const render = (ns: FileNode[]): React.ReactNode =>
    ns.map((n) =>
      n.dir ? (
        <FolderItem key={n.path} value={n.path}>
          <FolderTrigger>{n.name}</FolderTrigger>
          <FolderPanel transition={reduce ? { duration: 0 } : undefined}>{render(n.children ?? [])}</FolderPanel>
        </FolderItem>
      ) : (
        <button
          key={n.path}
          type="button"
          onClick={() => onOpen(n.path)}
          aria-current={active === n.path ? "true" : undefined}
          className={cn("block w-full rounded-ctl text-left", active === n.path && "bg-foreground/[0.07] font-medium")}
        >
          <FileItem icon={isMarkdown(n.path) ? FileText : FileCode2} title={n.name}>
            {n.name}
            {dirty.has(n.path) && <span className="ml-1.5 inline-block size-1.5 rounded-full bg-ac align-middle" aria-label={t("unsaved")} title={t("unsaved")} />}
          </FileItem>
        </button>
      ),
    )

  return (
    <Files defaultOpen={folders} className="p-1.5 fs-body">
      {render(nodes)}
    </Files>
  )
}
