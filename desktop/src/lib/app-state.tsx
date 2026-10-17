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

export type View = "skills" | "sources" | "agents" | "history" | "scan" | "trash" | "settings" | "editor"

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
  | null

/** A pack counts as updatable like app.PackStatus.HasUpdates (moves alone do not count). */
export function hasUpdates(p: PackStatus): boolean {
  return p.changed.length + p.new.length + p.gone.length + p.pending.length > 0
}

interface AppContextValue {
  t: T
  config: Config
  setConfig: (patch: Partial<Config>) => Promise<void>
  resolvedTheme: "light" | "dark"

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
  dialog: DialogState
  openDialog: (d: DialogState) => void
  closeDialog: () => void
  query: string
  setQuery: (q: string) => void
  filter: SkillFilter
  setFilter: (f: SkillFilter) => void
  paletteOpen: boolean
  setPaletteOpen: (open: boolean) => void
  /** The editor registers a guard; navigation asks it before leaving. */
  setLeaveGuard: (guard: ((go: () => void) => boolean) | null) => void

  /** Runs a backend call, toasting its error. */
  run: <R>(fn: () => Promise<R>) => Promise<Result<R>>
}

const AppContext = React.createContext<AppContextValue | null>(null)

export function useApp() {
  const ctx = React.useContext(AppContext)
  if (!ctx) throw new Error("useApp must be used inside AppProvider")
  return ctx
}

function errorText(t: T, error: unknown): string {
  const detail = error instanceof Error ? error.message : String(error)
  const lower = detail.toLowerCase()
  if (lower.includes("folder picker")) return t("errorFolderPicker")
  if (lower.includes("no skilldots library") || lower.includes("cannot find the library")) return t("errorLibraryMissing")
  return detail
}

const DEFAULT_CONFIG: Config = { editor: "auto", editorCommand: "", lang: "zh", theme: "system", checkOnLaunch: true }

function useSystemDark() {
  const query = "(prefers-color-scheme: dark)"
  const [dark, setDark] = React.useState(() => window.matchMedia(query).matches)
  React.useEffect(() => {
    const mq = window.matchMedia(query)
    const on = () => setDark(mq.matches)
    mq.addEventListener("change", on)
    return () => mq.removeEventListener("change", on)
  }, [])
  return dark
}

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [config, setConfigState] = React.useState<Config>(DEFAULT_CONFIG)
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
  const [dialog, setDialog] = React.useState<DialogState>(null)
  const [paletteOpen, setPaletteOpen] = React.useState(false)
  const [query, setQuery] = React.useState("")
  const [filter, setFilter] = React.useState<SkillFilter>("all")
  const guard = React.useRef<((go: () => void) => boolean) | null>(null)

  const systemDark = useSystemDark()
  const resolvedTheme = config.theme === "system" ? (systemDark ? "dark" : "light") : config.theme
  const t = React.useMemo(() => translator(config.lang), [config.lang])

  React.useLayoutEffect(() => {
    const root = document.documentElement
    root.classList.toggle("dark", resolvedTheme === "dark")
    root.style.colorScheme = resolvedTheme
  }, [resolvedTheme])
  React.useEffect(() => {
    document.documentElement.lang = config.lang === "zh" ? "zh-CN" : "en"
  }, [config.lang])

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
    setOverrides(new Set(o))
    setOutdated(out)
    setPlan(pl)
    setTrashCount(tr.length)
  }, [])

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
        return
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
        void check()
      }
      setLoadStage("ready")
    } catch (error: unknown) {
      setLoadError({ stage, detail: error instanceof Error ? error.message : String(error) })
    }
  }, [check, reload])

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
      if (v !== "editor") setEditing(null)
    }
    if (guard.current && !guard.current(go)) return
    go()
  }, [])

  const openEditor = React.useCallback((ref: Ref) => {
    const go = () => {
      guard.current = null
      setEditing(ref)
      setView("editor")
    }
    if (guard.current && !guard.current(go)) return
    go()
  }, [])

  const value: AppContextValue = {
    t,
    config,
    setConfig,
    resolvedTheme,
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
    dialog,
    openDialog: React.useCallback((next) => {
      if (!operation) setDialog(next)
    }, [operation]),
    closeDialog: () => setDialog(null),
    query,
    setQuery,
    filter,
    setFilter,
    paletteOpen,
    setPaletteOpen,
    setLeaveGuard: (g) => {
      guard.current = g
    },
    run,
    runTask,
  }

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}

/** Agents shown as columns, in registry order. */
export function useEnabledAgents(columnsOnly = false) {
  const { agents } = useApp()
  return React.useMemo(() => agents.filter((a) => a.enabled && (!columnsOnly || !a.hidden)), [agents, columnsOnly])
}

export function kindLabel(t: T, s: Skill): string {
  switch (s.kind) {
    case "skills":
      return t("kindMine")
    case "private":
      return t("kindPrivate")
    case "app":
      return t("kindApp")
    default:
      return s.origin
  }
}

export const PLACEHOLDER_DESC = "What this skill does and when an agent should use it."
