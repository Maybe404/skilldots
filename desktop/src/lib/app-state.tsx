import * as React from "react"
import { toast } from "sonner"

import {
  backend,
  type Agent,
  type Config,
  type LibraryStatus,
  type Outdated,
  type Pack,
  type PackStatus,
  type PlanItem,
  type Ref,
  type Skill,
} from "@/lib/backend"
import { translator, type T } from "@/lib/i18n"
import { changes, prune, toggle as togglePendingCell, type Pending } from "@/lib/pending"

export type View = "skills" | "sources" | "agents" | "history" | "scan" | "trash" | "settings" | "editor"
/** The two interface styles of the design ("简洁" and "涂鸦"). */
export type Style = "clean" | "doodle"
export type SkillView = "wire" | "table"

export type Result<R> = { ok: true; value: R } | { ok: false }
export type LoadStage = "config" | "library" | "checking" | "ready"
export interface LoadError {
  stage: LoadStage
  detail: string
}
export interface ActiveOperation {
  label: string
  startedAt: number
  id: number
}

export type SkillFilter = "all" | "mine" | "vendor" | "updates"
export type TableSort = { by: "name" | "source"; dir: "asc" | "desc" } | null

export interface ConfirmRequest {
  title: string
  body: string
  action: string
  /** Runs when confirmed; resolve false to keep the dialog open. */
  run: () => Promise<boolean | void> | boolean | void
}

export type DialogState =
  | { kind: "add"; initial?: string }
  | { kind: "new" }
  | { kind: "fork"; ref: Ref; thenEdit?: boolean }
  | { kind: "update"; repos?: string[] }
  | { kind: "apply" }
  | { kind: "sync" }
  | { kind: "library"; mode: "use" | "clone" | "init" }
  | { kind: "remove"; ref: Ref }
  | { kind: "unlink"; ref: Ref }
  | { kind: "purge"; repo: string }
  | ({ kind: "confirm" } & ConfirmRequest)
  | null

/** A pack counts as updatable like app.PackStatus.HasUpdates (moves alone do not count). */
export function hasUpdates(p: PackStatus): boolean {
  return p.changed.length + p.new.length + p.gone.length + p.pending.length > 0
}

interface AppContextValue {
  t: T
  config: Config
  setConfig: (patch: Partial<Config>) => Promise<void>
  style: Style
  setStyle: (s: Style) => void

  loadError: LoadError | null
  loadStage: LoadStage
  retryLoad: () => void
  ready: boolean
  status: LibraryStatus | null
  skills: Skill[]
  packs: Pack[]
  agents: Agent[]
  overrides: Set<string>
  outdated: Outdated | null
  plan: PlanItem[]
  trashCount: number
  checking: boolean
  operation: ActiveOperation | null
  runTask: <R>(label: string, fn: () => Promise<R>) => Promise<Result<R>>
  /** Keys of skills with a pending update. */
  updatable: Set<string>
  updatablePacks: PackStatus[]
  reload: () => Promise<void>
  check: (repos?: string[]) => Promise<void>

  view: View
  navigate: (view: View) => void
  editing: Ref | null
  openEditor: (ref: Ref) => void
  selected: string | null
  select: (key: string | null) => void
  detail: string | null
  openDetail: (key: string) => void
  closeDetail: () => void
  dialog: DialogState
  openDialog: (d: DialogState) => void
  closeDialog: () => void
  confirm: (req: ConfirmRequest) => void
  query: string
  setQuery: (q: string) => void
  filter: SkillFilter
  setFilter: (f: SkillFilter) => void
  skillView: SkillView
  setSkillView: (v: SkillView) => void
  grouped: boolean
  setGrouped: (g: boolean) => void
  collapsed: Set<string>
  toggleCollapsed: (group: string) => void
  agentFocus: string | null
  setAgentFocus: (agent: string | null) => void
  tableSort: TableSort
  setTableSort: (s: TableSort) => void
  /** Source row to open on the sources page (from a skill's update notice). */
  sourceFocus: string | null
  showSource: (repo: string | null) => void
  paletteOpen: boolean
  setPaletteOpen: (open: boolean) => void
  /** The editor registers a guard; navigation asks it before leaving. */
  setLeaveGuard: (guard: ((go: () => void) => boolean) | null) => void

  // Link changes waiting for 应用
  pending: Pending
  pendingCount: number
  /** True while applyPending is writing links (the status bar shows progress). */
  applying: boolean
  togglePending: (skill: Skill, agent: string) => void
  revertPending: () => void
  applyPending: () => Promise<void>

  /** Runs a backend call, toasting its error. Refused while another operation runs. */
  run: <R>(fn: () => Promise<R>) => Promise<Result<R>>
  /** Runs a read-only backend call, toasting its error. Not blocked by a running operation. */
  read: <R>(fn: () => Promise<R>) => Promise<Result<R>>
}

const AppContext = React.createContext<AppContextValue | null>(null)

export function useApp() {
  const ctx = React.useContext(AppContext)
  if (!ctx) throw new Error("useApp must be used inside AppProvider")
  return ctx
}

export function errorText(t: T, error: unknown): string {
  const detail = error instanceof Error ? error.message : String(error)
  const lower = detail.toLowerCase()
  if (lower.includes("folder picker")) return t("errorFolderPicker")
  if (lower.includes("no skilldots library") || lower.includes("cannot find the library")) return t("errorLibraryMissing")
  return detail
}

const DEFAULT_CONFIG: Config = { editor: "auto", editorCommand: "", lang: "zh", theme: "system", checkOnLaunch: true }
const STYLE_KEY = "skilldots.desktop.style"

function readStyle(): Style {
  try {
    const s = localStorage.getItem(STYLE_KEY)
    if (s === "doodle" || s === "clean") return s
  } catch {
    // storage unavailable: fall back to the default style
  }
  return document.documentElement.dataset.style === "doodle" ? "doodle" : "clean"
}

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [config, setConfigState] = React.useState<Config>(DEFAULT_CONFIG)
  const [style, setStyleState] = React.useState<Style>(readStyle)
  const [loadError, setLoadError] = React.useState<LoadError | null>(null)
  const [loadStage, setLoadStage] = React.useState<LoadStage>("config")
  const [ready, setReady] = React.useState(false)
  const [status, setStatus] = React.useState<LibraryStatus | null>(null)
  const [skills, setSkills] = React.useState<Skill[]>([])
  const [packs, setPacks] = React.useState<Pack[]>([])
  const [agents, setAgents] = React.useState<Agent[]>([])
  const [overrides, setOverrides] = React.useState<Set<string>>(new Set())
  const [outdated, setOutdated] = React.useState<Outdated | null>(null)
  const [plan, setPlan] = React.useState<PlanItem[]>([])
  const [trashCount, setTrashCount] = React.useState(0)
  const [checking, setChecking] = React.useState(false)
  const [operation, setOperation] = React.useState<ActiveOperation | null>(null)
  const operationId = React.useRef(0)
  const operations = React.useRef(new Map<number, ActiveOperation>())
  const [view, setView] = React.useState<View>("skills")
  const [editing, setEditing] = React.useState<Ref | null>(null)
  const [selected, setSelected] = React.useState<string | null>(null)
  const [detail, setDetail] = React.useState<string | null>(null)
  const [dialog, setDialog] = React.useState<DialogState>(null)
  const [paletteOpen, setPaletteOpen] = React.useState(false)
  const [query, setQuery] = React.useState("")
  const [filter, setFilter] = React.useState<SkillFilter>("all")
  const [skillView, setSkillView] = React.useState<SkillView>("wire")
  const [grouped, setGrouped] = React.useState(false)
  const [collapsed, setCollapsed] = React.useState<Set<string>>(new Set())
  const [agentFocus, setAgentFocus] = React.useState<string | null>(null)
  const [tableSort, setTableSort] = React.useState<TableSort>(null)
  const [sourceFocus, setSourceFocus] = React.useState<string | null>(null)
  const [pending, setPending] = React.useState<Pending>({})
  const [applying, setApplying] = React.useState(false)
  const applyingRef = React.useRef(false)
  const guard = React.useRef<((go: () => void) => boolean) | null>(null)

  const t = React.useMemo(() => translator(config.lang), [config.lang])

  React.useLayoutEffect(() => {
    const root = document.documentElement
    root.dataset.style = style
    // Light only for now; the dark variant of both styles is still being designed.
    root.classList.remove("dark")
    root.style.colorScheme = "light"
  }, [style])
  React.useEffect(() => {
    document.documentElement.lang = config.lang === "zh" ? "zh-CN" : "en"
  }, [config.lang])

  const setStyle = React.useCallback((s: Style) => {
    setStyleState(s)
    try {
      localStorage.setItem(STYLE_KEY, s)
    } catch {
      // storage unavailable: keep the choice for this session only
    }
  }, [])

  const execute = React.useCallback(
    async <R,>(fn: () => Promise<R>): Promise<Result<R>> => {
      try {
        return { ok: true, value: await fn() }
      } catch (e) {
        toast.error(t("errorPrefix"), { description: errorText(t, e) })
        return { ok: false }
      }
    },
    [t],
  )

  const run = React.useCallback(
    async <R,>(fn: () => Promise<R>): Promise<Result<R>> => {
      if (operation) {
        toast(t("operationWait"))
        return { ok: false }
      }
      return execute(fn)
    },
    [execute, operation, t],
  )

  const read = execute

  const beginOperation = React.useCallback((label: string) => {
    const op = { id: ++operationId.current, label, startedAt: Date.now() }
    operations.current.set(op.id, op)
    setOperation(op)
    return op.id
  }, [])

  const endOperation = React.useCallback((id: number) => {
    operations.current.delete(id)
    const latest = [...operations.current.values()].sort((a, b) => a.id - b.id).at(-1) ?? null
    setOperation(latest)
  }, [])

  const runTask = React.useCallback(
    async <R,>(label: string, fn: () => Promise<R>): Promise<Result<R>> => {
      const id = beginOperation(label)
      try {
        return await execute(fn)
      } finally {
        endOperation(id)
      }
    },
    [beginOperation, endOperation, execute],
  )

  const reload = React.useCallback(async () => {
    const st = await backend.status()
    setStatus(st)
    if (!st.root) return
    const [s, p, a, o, out, pl, tr] = await Promise.all([
      backend.skills(),
      backend.packs(),
      backend.agents(),
      backend.claudeOverrides(),
      backend.recordedOutdated(),
      backend.plan(),
      backend.trash(),
    ])
    setSkills(s)
    setPacks(p)
    setAgents(a)
    // Focusing an agent only makes sense while its column is on the page.
    setAgentFocus((f) => (f && a.find((x) => x.name === f && !x.hidden) ? f : null))
    setOverrides(new Set(o))
    setOutdated(out)
    setPlan(pl)
    setTrashCount(tr.length)
  }, [])

  // Pending changes that the backend now matches (or whose skill or column is gone) are done.
  React.useEffect(() => {
    setPending((p) => prune(p, skills, agents))
  }, [skills, agents])

  const check = React.useCallback(
    async (repos?: string[]) => {
      if (operation) {
        toast(t("operationWait"))
        return
      }
      setChecking(true)
      const id = beginOperation(t("checking"))
      try {
        const out = await backend.outdated(repos)
        setOutdated(out)
        const n = out.packs.filter(hasUpdates).length
        toast(n ? t("checkedToast", { n }) : t("checkedNone"))
        if (out.failed.length) toast.error(t("checkFailed"), { description: out.failed.map((f) => `${f.repo}: ${f.error}`).join("\n") })
      } catch (e) {
        toast.error(t("checkFailed"), { description: errorText(t, e) })
      } finally {
        setChecking(false)
        endOperation(id)
      }
    },
    [beginOperation, endOperation, operation, t],
  )

  // Load config and library once.
  const launched = React.useRef(false)
  const load = React.useCallback(async () => {
    let stage: LoadStage = "config"
    setLoadError(null)
    setReady(false)
    setLoadStage(stage)
    try {
      const c = await backend.config()
      setConfigState(c)
      stage = "library"
      setLoadStage(stage)
      await reload()
      setReady(true)
      const st = await backend.status()
      if (st.root && c.checkOnLaunch) {
        stage = "checking"
        setLoadStage(stage)
        setChecking(true)
        const id = beginOperation(translator(c.lang)("checking"))
        backend
          .outdated()
          .then(setOutdated, () => undefined)
          .finally(() => {
            setChecking(false)
            endOperation(id)
          })
      }
      setLoadStage("ready")
    } catch (error: unknown) {
      setLoadError({ stage, detail: error instanceof Error ? error.message : String(error) })
    }
  }, [reload, beginOperation, endOperation])

  React.useEffect(() => {
    if (launched.current) return
    launched.current = true
    void load()
  }, [load])

  const retryLoad = React.useCallback(() => {
    void load()
  }, [load])

  const setConfig = React.useCallback(
    async (patch: Partial<Config>) => {
      const result = await run(() => backend.setConfig(patch))
      if (result.ok) setConfigState(result.value)
    },
    [run],
  )

  const updatablePacks = React.useMemo(() => (outdated?.packs ?? []).filter(hasUpdates), [outdated])
  const updatable = React.useMemo(() => {
    const keys = new Set<string>()
    for (const p of updatablePacks) {
      for (const n of [...p.changed, ...p.pending]) {
        const s = skills.find((x) => x.origin === p.repo && x.name === n)
        if (s) keys.add(s.ref.key)
      }
    }
    return keys
  }, [updatablePacks, skills])

  const navigate = React.useCallback((v: View) => {
    const go = () => {
      guard.current = null
      setView(v)
      setDetail(null)
      if (v !== "editor") setEditing(null)
    }
    if (guard.current && !guard.current(go)) return
    go()
  }, [])

  const openEditor = React.useCallback((ref: Ref) => {
    const go = () => {
      guard.current = null
      setDetail(null)
      setEditing(ref)
      setView("editor")
    }
    if (guard.current && !guard.current(go)) return
    go()
  }, [])

  const togglePending = React.useCallback((skill: Skill, agent: string) => {
    setPending((p) => togglePendingCell(p, skill, agent))
  }, [])

  const pendingCount = Object.keys(pending).length

  const applyPending = React.useCallback(async () => {
    const list = changes(pending, skills)
    if (!list.length) return
    if (operation || applyingRef.current) {
      toast(t("operationWait"))
      return
    }
    const n = list.reduce((sum, c) => sum + c.enableKeys.length + c.disableKeys.length, 0)
    applyingRef.current = true
    setApplying(true)
    try {
      // One enable / disable per skill (see changes()); it stops at the first skill that fails.
      const r = await runTask(t("applyingN", { n }), async () => {
        let done = 0
        for (const c of list) {
          try {
            if (c.enable.length) {
              await backend.enable(c.skill.ref, c.enable)
              done += c.enableKeys.length
            }
            if (c.disable.length) {
              await backend.disable(c.skill.ref, c.disable)
              done += c.disableKeys.length
            }
          } catch (error) {
            return { done, failed: { name: c.skill.name, error } }
          }
        }
        return { done, failed: undefined }
      })
      // Applied links drop out of `pending` once the reload shows their new targets;
      // whatever failed or was not reached stays.
      await reload().catch(() => undefined)
      if (!r.ok) return
      const { done, failed } = r.value
      if (failed) toast.error(t("applyFailedAt", { name: failed.name, done, rest: n - done }), { description: errorText(t, failed.error) })
      else toast(t("appliedN", { n }))
    } finally {
      applyingRef.current = false
      setApplying(false)
    }
  }, [pending, skills, operation, runTask, reload, t])

  const revertPending = React.useCallback(() => setPending({}), [])

  // Pending changes live only in this window: ask before it closes.
  React.useEffect(() => {
    if (!pendingCount) return
    const onUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ""
    }
    window.addEventListener("beforeunload", onUnload)
    return () => window.removeEventListener("beforeunload", onUnload)
  }, [pendingCount])

  const openDialog = React.useCallback(
    (next: DialogState) => {
      if (!operation) setDialog(next)
      else toast(t("operationWait"))
    },
    [operation, t],
  )

  const value: AppContextValue = {
    t,
    config,
    setConfig,
    style,
    setStyle,
    loadError,
    loadStage,
    retryLoad,
    ready,
    status,
    skills,
    packs,
    agents,
    overrides,
    outdated,
    plan,
    trashCount,
    checking,
    operation,
    updatable,
    updatablePacks,
    reload,
    check,
    view,
    navigate,
    editing,
    openEditor,
    selected,
    select: setSelected,
    detail,
    openDetail: React.useCallback((key: string) => {
      setSelected(key)
      setDetail(key)
    }, []),
    closeDetail: React.useCallback(() => setDetail(null), []),
    dialog,
    openDialog,
    closeDialog: React.useCallback(() => setDialog(null), []),
    confirm: React.useCallback((req: ConfirmRequest) => openDialog({ kind: "confirm", ...req }), [openDialog]),
    query,
    setQuery,
    filter,
    setFilter,
    skillView,
    setSkillView,
    grouped,
    setGrouped,
    collapsed,
    toggleCollapsed: React.useCallback((g: string) => {
      setCollapsed((c) => {
        const n = new Set(c)
        if (n.has(g)) n.delete(g)
        else n.add(g)
        return n
      })
    }, []),
    agentFocus,
    setAgentFocus,
    tableSort,
    setTableSort,
    sourceFocus,
    showSource: React.useCallback(
      (repo: string | null) => {
        setSourceFocus(repo)
        if (repo) navigate("sources")
      },
      [navigate],
    ),
    paletteOpen,
    setPaletteOpen,
    setLeaveGuard: React.useCallback((g: ((go: () => void) => boolean) | null) => {
      guard.current = g
    }, []),
    pending,
    pendingCount,
    applying,
    togglePending,
    revertPending,
    applyPending,
    run,
    read,
    runTask,
  }

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}

/** Agents shown as columns on the Skill page, in registry order (paused ones included, dimmed). */
export function useColumnAgents() {
  const { agents } = useApp()
  return React.useMemo(() => agents.filter((a) => !a.hidden), [agents])
}

/**
 * Agents a new skill can be installed to: every enabled one. A hidden column
 * only affects the Skill page; the defaults that pre-check a box still apply.
 */
export function useEnabledAgents() {
  const { agents } = useApp()
  return React.useMemo(() => agents.filter((a) => a.enabled), [agents])
}

export function kindLabel(t: T, s: Skill): string {
  switch (s.kind) {
    case "skills":
      return t("kindMine")
    case "private":
      return t("kindPrivate")
    case "app":
      return s.origin || t("kindApp")
    default:
      return s.origin
  }
}

/** The group a skill belongs to when the list is grouped by source. */
export function sourceGroup(t: T, s: Skill): string {
  return s.kind === "skills" || s.kind === "private" ? t("kindMine") : s.origin
}

export function isMine(s: Skill): boolean {
  return s.kind === "skills" || s.kind === "private"
}

export const PLACEHOLDER_DESC = "What this skill does and when an agent should use it."
