import * as React from "react"
import { toast } from "sonner"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { useReducedMotion } from "motion/react"
import { ArrowLeft, ChevronDown, FileCode2, FileText, FolderOpen, Loader2, PanelRightClose, PanelRightOpen, Save } from "lucide-react"

import { cn } from "@/lib/utils"
import { PLACEHOLDER_DESC, kindLabel, useApp } from "@/lib/app-state"
import { backend, type FileNode, type Ref, type Skill } from "@/lib/backend"
import { parseSkillMd, serializeSkillMd } from "@/lib/frontmatter"
import { useSkillActions } from "@/lib/use-actions"
import { CodeEditor, languageName } from "@/components/editor/code-editor"
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

export function EditorPage({ skillRef }: { skillRef: Ref }) {
  const { t, skills, run, reload, resolvedTheme, navigate, setLeaveGuard, openDialog } = useApp()
  const { reveal } = useSkillActions()
  const skill = skills.find((s) => s.ref.key === skillRef.key)
  const [tree, setTree] = React.useState<FileNode[] | null>(null)
  const [buffers, setBuffers] = React.useState<Record<string, Buffer>>({})
  const [active, setActive] = React.useState("SKILL.md")
  const [preview, setPreview] = React.useState(() => typeof window === "undefined" || window.innerWidth >= 1100)
  const [metadataOpen, setMetadataOpen] = React.useState(false)
  const [cursor, setCursor] = React.useState({ line: 1, col: 1 })
  const [saving, setSaving] = React.useState(false)
  const needsAck = skill?.kind === "vendor" || skill?.kind === "app"
  const [acked, setAcked] = React.useState(false)
  const [leaving, setLeaving] = React.useState<(() => void) | null>(null)

  // Load the file list and SKILL.md.
  React.useEffect(() => {
    let live = true
    setTree(null)
    setBuffers({})
    setActive("SKILL.md")
    setAcked(false)
    ;(async () => {
      const r = await run(() => backend.files(skillRef))
      if (!live || !r.ok) return
      setTree(r.value)
      const md = await run(() => backend.readFile(skillRef, "SKILL.md"))
      if (live && md.ok) setBuffers({ "SKILL.md": { saved: md.value, current: md.value } })
    })()
    return () => {
      live = false
    }
  }, [skillRef, run])

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
      <header className="glass-bar flex h-13 shrink-0 items-center gap-2 rounded-t-xl px-3" data-drag-region>
        <Button variant="ghost" size="sm" onClick={() => navigate("skills")}>
          <ArrowLeft /> {t("back")}
        </Button>
        <div className="ml-1 flex min-w-0 items-baseline gap-2">
          <h1 className="truncate text-[0.95rem] font-semibold">{t("editorTitle", { name: skill.linkName })}</h1>
          <span className="truncate text-[0.78rem] text-muted-foreground">{kindLabel(t, skill)}</span>
          {hasDirty && (
            <span className="flex items-center gap-1.5 text-[0.78rem]" role="status">
              <span className="size-1.5 rounded-full bg-foreground" aria-hidden />
              {dirty.length > 1 ? t("unsavedFiles", { n: dirty.length }) : t("unsaved")}
            </span>
          )}
        </div>
        <div className="ml-auto flex items-center gap-1.5">
          <Button variant="ghost" size="sm" onClick={() => reveal(skill.ref)}>
            <FolderOpen /> {t("revealInFinder")}
          </Button>
          <Button variant="ghost" size="sm" aria-pressed={preview} onClick={() => setPreview((p) => !p)}>
            {preview ? <PanelRightClose /> : <PanelRightOpen />}
            {preview ? t("hidePreview") : t("showPreview")}
          </Button>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button size="sm" onClick={() => void save()} disabled={!hasDirty || saving || readOnly}>
                  {saving ? <Loader2 className="animate-spin motion-reduce:animate-none" /> : <Save />}
                  {t("save")}
                </Button>
              }
            />
            <TooltipContent>
              {t("save")} <Kbd>⌘S</Kbd>
            </TooltipContent>
          </Tooltip>
        </div>
      </header>

      {needsAck && acked && (
        <div className="border-b bg-muted/60 px-5 py-1.5 text-[0.8rem]" role="note">
          {t("vendorBanner")}
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        <nav aria-label={t("files")} className="w-56 shrink-0 overflow-y-auto border-r bg-muted/30 py-2">
          <p className="px-4 pt-1 pb-1 text-[0.72rem] font-medium text-muted-foreground">{t("files")}</p>
          {tree ? <FileTree nodes={tree} active={active} dirty={dirtySet} onOpen={open} /> : <p className="px-4 text-sm text-muted-foreground">{t("loading")}</p>}
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
                dark={resolvedTheme === "dark"}
                label={active}
              />
            ) : (
              <p className="p-4 text-sm text-muted-foreground">{t("loading")}</p>
            )}
          </div>
          <footer className="flex h-7 shrink-0 items-center gap-4 border-t px-4 text-[0.72rem] text-muted-foreground">
            <span className="font-mono">{active}</span>
            <span>{languageName(active) ?? t("plainText")}</span>
            <span className="ml-auto tabular-nums">{t("lineCol", cursor)}</span>
          </footer>
        </section>

        {preview && (
          <aside aria-label={t("preview")} className="w-[36%] max-w-[38rem] min-w-[20rem] shrink-0 overflow-y-auto border-l px-7 py-6">
            {md && buf ? (
              <>
                {doc?.hasFrontmatter && (
                  <div className="mb-5 border-b pb-4">
                    <p className="font-mono text-[0.8rem] text-muted-foreground">{doc.name}</p>
                    <p className="mt-1 text-[0.85rem] leading-relaxed">{doc.description}</p>
                  </div>
                )}
                <article className="prose-skill">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{doc ? doc.body : buf.current}</ReactMarkdown>
                </article>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">{t("previewOnlyMd")}</p>
            )}
          </aside>
        )}
      </div>

      {/* vendor / app: warn before editing */}
      <AlertDialog open={needsAck && !acked && !!tree} onOpenChange={(o) => !o && !acked && navigate("skills")}>
        <AlertDialogContent className="sm:max-w-md">
          <AlertDialogHeader>
            <AlertDialogTitle>
              {skill.kind === "app" ? t("appWarnTitle", { name: skill.linkName, origin: skill.origin }) : t("vendorWarnTitle", { name: skill.linkName, origin: skill.origin })}
            </AlertDialogTitle>
            <AlertDialogDescription>{skill.kind === "app" ? t("appWarnBody") : t("vendorWarnBody")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => navigate("skills")}>{t("cancel")}</AlertDialogCancel>
            <Button variant="outline" onClick={() => setAcked(true)}>
              {t("editAnyway")}
            </Button>
            <Button
              autoFocus
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
    <div className="shrink-0 border-b bg-muted/20">
      <button
        type="button"
        className="flex w-full items-center justify-between gap-3 px-5 py-2.5 text-left hover:bg-muted/50"
        aria-expanded={open}
        onClick={() => onOpenChange(!open)}
      >
        <span className="min-w-0">
          <span className="block text-[0.72rem] font-medium uppercase tracking-[0.08em] text-muted-foreground">{t("frontmatter")}</span>
          <span className="mt-0.5 block truncate text-[0.82rem]">{name || skill.name}</span>
        </span>
        <ChevronDown className={cn("size-4 shrink-0 transition-transform motion-reduce:transition-none", open && "rotate-180")} aria-hidden />
      </button>
      {open && (
        <div className="grid gap-3 border-t px-5 py-4 lg:grid-cols-[14rem_1fr]">
          <Field>
            <FieldLabel htmlFor="fm-name">{t("nameLabel")}</FieldLabel>
            <Input id="fm-name" value={name || skill.name} readOnly className="bg-muted/50 font-mono text-[0.82rem]" aria-describedby="fm-name-hint" />
            <FieldDescription id="fm-name-hint" className="text-[0.75rem]">
              {t("nameReadonlyHint")}
            </FieldDescription>
          </Field>
          <Field data-invalid={placeholder || len > 1024 || undefined}>
            <div className="flex items-baseline justify-between">
              <FieldLabel htmlFor="fm-desc">{t("descLabel")}</FieldLabel>
              <span className={cn("text-[0.72rem] tabular-nums", len > 1024 ? "font-semibold text-foreground" : "text-muted-foreground")}>{t("descCounter", { n: len })}</span>
            </div>
            <Textarea
              id="fm-desc"
              value={description}
              onChange={(e) => onDescription(e.target.value)}
              readOnly={readOnly}
              rows={3}
              placeholder={t("descPlaceholder")}
              className="min-h-[4.5rem] resize-y text-[0.85rem]"
              aria-describedby="fm-desc-hint"
            />
            <FieldDescription id="fm-desc-hint" className="text-[0.75rem]">
              {placeholder ? t("placeholderWarn", { name: skill.name }) : len > 1024 ? t("descTooLong") : t("descHint")}
            </FieldDescription>
          </Field>
        </div>
      )}
    </div>
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
          className={cn("block w-full rounded-lg text-left", active === n.path && "bg-foreground/[0.07] font-medium")}
        >
          <FileItem icon={isMarkdown(n.path) ? FileText : FileCode2}>
            {n.name}
            {dirty.has(n.path) && <span className="ml-1.5 inline-block size-1.5 rounded-full bg-foreground align-middle" aria-label={t("unsaved")} />}
          </FileItem>
        </button>
      ),
    )

  return (
    <Files defaultOpen={folders} className="p-1.5 text-[0.85rem]">
      {render(nodes)}
    </Files>
  )
}
