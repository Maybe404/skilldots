import * as React from "react"
import { toast } from "sonner"

import {
  backend,
  shelvedNames,
  SYNC_REF,
  type Absent,
  type Agent,
  type Config,
  type HistoryEntry,
  type LibraryStatus,
  type Outdated,
  type Pack,
  type PackStatus,
  type PlanItem,
  type Ref,
  type Skill,
} from "@/lib/backend"
import { agentLabel, isAbsentAgent } from "@/lib/agents"
import { translator, type T } from "@/lib/i18n"
import { changes, pendingKey, prune, readsViaShared, setMany, toggle as togglePendingCell, type Pending } from "@/lib/pending"

export type View = "skills" | "sources" | "agents" | "history" | "scan" | "trash" | "settings" | "editor" | "resolve"

/** Where the editor opens and what its 返回 goes back to (the resolve page opens a conflicted file). */
export interface EditorTarget {
  ref: Ref
  file?: string
  back?: View
}
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
  /** own: "这是我的仓库" ticked from the start (importing your own skills repository). */
  | { kind: "add"; initial?: string; adjust?: boolean; own?: boolean }
  /** pack: made in this own repository. */
  | { kind: "new"; pack?: string }
  | { kind: "fork"; ref: Ref; thenEdit?: boolean }
  | { kind: "update"; repos?: string[] }
  | { kind: "apply" }
  | { kind: "sync" }
  | { kind: "contribute"; ref: Ref }
  /** Every skill of the source you changed or made, at once (推送 N 项). */
  | { kind: "contribute"; repo: string }
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
  editing: EditorTarget | null
  openEditor: (ref: Ref, opts?: { file?: string; back?: View }) => void
  /** The skill whose shelved update the resolve page is resolving, or SYNC_REF for the library's sync merge. */
  resolving: Ref | null
  openResolve: (ref: Ref) => void
  /** skd sync (commit, pull and merge, push), with its toasts; "shelved" when the remote changed the same files. */
  syncLibrary: (opts?: { remote?: string; message?: string }) => Promise<"done" | "shelved" | "failed">
  /** The files of the sync shelved in this window, until it is resolved or synced. */
  syncShelved: string[] | null
  setSyncShelved: (files: string[] | null) => void
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
  /** Wants every one of skills on or off for agent (a group header's switch). */
  setPendingMany: (skills: Skill[], agent: string, want: boolean) => void
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
  // skd clone (import) of a repository without a skills.toml: app.CloneLibrary's error.
  const notLibrary = /^(.+?) is not a skilldots library( but a repository of skills)?/.exec(detail)
  if (notLibrary && !lower.includes("removing the clone failed")) return t(notLibrary[2] ? "errorCloneSkillsRepo" : "errorCloneNotLibrary", { url: notLibrary[1] })
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
  const [editing, setEditing] = React.useState<EditorTarget | null>(null)
  const [resolving, setResolving] = React.useState<Ref | null>(null)
  const [syncShelved, setSyncShelved] = React.useState<string[] | null>(null)
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
    // The bridge remembers a shelved sync across launches; the preview keeps it in this window only.
    if (st.syncShelved) setSyncShelved(st.syncShelved.length ? st.syncShelved : null)
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
        // Open PRs are looked at again too (best effort: offline or without gh nothing is said).
        if (skills.some((s) => s.contribution?.kind === "pr" && s.contribution.state === "open")) {
          void backend.refreshContributions().then(reload, () => undefined)
        }
      } catch (e) {
        toast.error(t("checkFailed"), { description: errorText(t, e) })
      } finally {
        setChecking(false)
        endOperation(id)
      }
    },
    [beginOperation, endOperation, operation, skills, reload, t],
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
      // The resolve page keeps its skill while a conflicted file is open in the editor.
      if (v !== "resolve" && v !== "editor") setResolving(null)
    }
    if (guard.current && !guard.current(go)) return
    go()
  }, [])

  const openEditor = React.useCallback((ref: Ref, opts?: { file?: string; back?: View }) => {
    const go = () => {
      guard.current = null
      setDetail(null)
      setEditing({ ref, ...opts })
      setView("editor")
    }
    if (guard.current && !guard.current(go)) return
    go()
  }, [])

  const openResolve = React.useCallback((ref: Ref) => {
    const go = () => {
      guard.current = null
      setDetail(null)
      setEditing(null)
      setResolving(ref)
      setView("resolve")
    }
    if (guard.current && !guard.current(go)) return
    go()
  }, [])

  const syncLibrary = React.useCallback(
    async (opts: { remote?: string; message?: string } = {}): Promise<"done" | "shelved" | "failed"> => {
      const r = await runTask(t("syncing"), () => backend.sync(opts))
      if (!r.ok) return "failed"
      await reload().catch(() => undefined)
      const o = r.value
      if (o.conflicts?.length) {
        // Both sides changed the same files: nothing was pushed and the library is as it was.
        setSyncShelved(o.conflicts)
        toast(t("syncShelvedToast", { n: o.conflicts.length }), {
          description: t("syncShelvedNote"),
          duration: 10000,
          action: { label: t("resolveConflicts"), onClick: () => openResolve(SYNC_REF) },
        })
        return "shelved"
      }
      setSyncShelved(null)
      toast(o.pulled ? t("pulledToast", { remote: o.remote }) : o.committed ? t("syncedToast", { remote: o.remote }) : t("nothingToSync"))
      return "done"
    },
    [runTask, reload, openResolve, t],
  )

  // While applying, the snapshot being written is the truth: edits would only disagree with it.
  // An agent this Mac does not have gets nothing linked: turning a skill on for it is refused here, for every
  // way of doing so (cell, button, number key, drag, group switch); taking a choice off it still works.
  const refuseAbsent = React.useCallback(
    (agent: string) => {
      const a = agents.find((x) => x.name === agent)
      if (!a || !isAbsentAgent(a)) return false
      toast(t("absentCannotInstall", { agent: agentLabel(a) }))
      return true
    },
    [agents, t],
  )

  const label = React.useCallback((name: string) => {
    const a = agents.find((x) => x.name === name)
    return a ? agentLabel(a) : name
  }, [agents])

  // Taking a skill off Agents: the agents that only read it there lose it; those chosen for it as well (in its
  // targets) get a link of their own instead, as the engine does. Said when the change is marked.
  const warnSharedLost = React.useCallback(
    (list: Skill[]) => {
      const lost = new Set<string>()
      const relinked = new Set<string>()
      for (const s of list) {
        for (const a of s.sharedReach ?? []) (s.targets.includes(a) ? relinked : lost).add(a)
        for (const a of s.viaShared ?? []) relinked.add(a)
      }
      const skill = list.length === 1 ? list[0].linkName : t("groupCount", { n: list.length })
      const names = (set: Set<string>) => [...set].map(label).join(t("listSep"))
      if (lost.size) toast(t("viaSharedLost", { agents: names(lost), skill }))
      if (relinked.size) toast(t("viaSharedRelink", { agents: names(relinked), skill }))
    },
    [label, t],
  )

  const togglePending = React.useCallback(
    (skill: Skill, agent: string) => {
      if (applyingRef.current) return
      const k = pendingKey(skill.ref.key, agent)
      if (!(k in pending) && readsViaShared(skill, agent, pending)) {
        // Read from the shared folder. Not chosen for it: turning it on would change nothing it can see (and the
        // choice can still be made when Agents loses the skill: the cell then offers it). Chosen for it as well:
        // taking that choice off is allowed, it still reads the skill through Agents.
        if (!skill.targets.includes(agent)) {
          toast(t("viaSharedAlready", { agent: label(agent), skill: skill.linkName }))
          return
        }
        toast(t("viaCancelOwn", { agent: label(agent) }))
        setPending((p) => togglePendingCell(p, skill, agent))
        return
      }
      const turningOn = !(k in pending) && !skill.targets.includes(agent)
      if (turningOn && refuseAbsent(agent)) return
      if (agent === "agents" && !(k in pending) && skill.targets.includes(agent)) warnSharedLost([skill])
      setPending((p) => togglePendingCell(p, skill, agent))
    },
    [pending, refuseAbsent, label, t, warnSharedLost],
  )

  const setPendingMany = React.useCallback(
    (list: Skill[], agent: string, want: boolean) => {
      if (applyingRef.current) return
      if (want && refuseAbsent(agent)) return
      // Turning on leaves alone the skills the agent already reads from the shared folder; turning off still takes
      // off the agent's own choice (setMany only touches skills whose targets have it).
      const own = want ? list.filter((s) => !readsViaShared(s, agent, pending)) : list
      if (!own.length) {
        toast(t("viaSharedAlready", { agent: label(agent), skill: t("groupCount", { n: list.length }) }))
        return
      }
      if (agent === "agents" && !want) warnSharedLost(own.filter((s) => s.targets.includes("agents")))
      setPending((p) => setMany(p, own, agent, want))
    },
    [pending, refuseAbsent, label, t, warnSharedLost],
  )

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
        // Agents not installed here that a skill was just turned on for: linked nowhere, said once.
        const absent: Absent = {}
        for (const c of list) {
          try {
            if (c.enable.length) {
              const note = await backend.enable(c.skill.ref, c.enable)
              for (const a of c.enable) {
                if (note?.absent?.[a]?.includes(c.skill.linkName)) absent[a] = [...(absent[a] ?? []), c.skill.linkName]
              }
              done += c.enableKeys.length
            }
            if (c.disable.length) {
              await backend.disable(c.skill.ref, c.disable)
              done += c.disableKeys.length
            }
          } catch (error) {
            return { done, absent, failed: { name: c.skill.name, error } }
          }
        }
        return { done, absent, failed: undefined }
      })
      // Applied links drop out of `pending` once the reload shows their new targets;
      // whatever failed or was not reached stays.
      await reload().catch(() => undefined)
      if (!r.ok) return
      const { done, failed, absent } = r.value
      toastAbsent(t, agents, absent)
      if (failed) toast.error(t("applyFailedAt", { name: failed.name, done, rest: n - done }), { description: errorText(t, failed.error) })
      else toast(t("appliedN", { n }))
    } finally {
      applyingRef.current = false
      setApplying(false)
    }
  }, [pending, skills, agents, operation, runTask, reload, t])

  const revertPending = React.useCallback(() => {
    if (applyingRef.current) return
    setPending({})
  }, [])

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
    resolving,
    openResolve,
    syncLibrary,
    syncShelved,
    setSyncShelved,
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
    setPendingMany,
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

/**
 * One toast per agent an apply left out because it is not installed on
 * this Mac ("本机没有安装 Claude Code：有 1 个 skill 没有链接到那里").
 */
export function toastAbsent(t: T, agents: Agent[], ...notes: (Absent | undefined)[]) {
  const merged = new Map<string, Set<string>>()
  for (const absent of notes) {
    for (const [agent, skills] of Object.entries(absent ?? {})) {
      const set = merged.get(agent) ?? new Set<string>()
      skills.forEach((s) => set.add(s))
      merged.set(agent, set)
    }
  }
  for (const [name, skills] of merged) {
    const a = agents.find((x) => x.name === name)
    toast(t("absentToast", { agent: a ? agentLabel(a) : name, n: skills.size }))
  }
}

/** The part of an apply's absent list about names (an apply reports the whole library's). */
export function absentFor(absent: Absent | undefined, names: string[]): Absent {
  const out: Absent = {}
  for (const [agent, list] of Object.entries(absent ?? {})) {
    const hit = list.filter((n) => names.includes(n))
    if (hit.length) out[agent] = hit
  }
  return out
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
/** Where a skill lives, shown under its name next to the 本地 tag: the library folder of your own skills, the repository of the rest. */
export function sourceText(t: T, s: Skill): string {
  return s.kind === "skills" ? "skills/" : s.kind === "private" ? "private/" : kindLabel(t, s)
}

export function sourceGroup(t: T, s: Skill): string {
  return s.kind === "skills" || s.kind === "private" ? t("kindMine") : s.origin
}

export function isMine(s: Skill): boolean {
  return s.kind === "skills" || s.kind === "private"
}

/** A vendored skill with something to send to its repository: edited, or made there and not pushed (a draft). */
export function isUnpushed(s: Skill): boolean {
  return s.kind === "vendor" && !!(s.edited || s.draft)
}

/** An update of this skill waits for its conflicts to be resolved (shelved, or being resolved). */
export function hasConflicts(s: Skill): boolean {
  return !!(s.shelved || s.resolving)
}

/**
 * Says what an update did with the skills you had edited: merged, or shelved because of conflicts,
 * with a way to resolve them ("2 个 skill 已与你的改动合并；1 个有冲突，已搁置").
 */
export function toastMergeOutcome(t: T, entries: HistoryEntry[], skills: Skill[], openResolve: (ref: Ref) => void) {
  const merged = entries.reduce((n, h) => n + (h.merged?.length ?? 0), 0)
  const shelved = entries.flatMap((h) => shelvedNames(h).map((name) => skills.find((s) => s.origin === h.repo && s.name === name)))
  if (!merged && !shelved.length) return
  const text = merged
    ? shelved.length
      ? t("updateMergedN", { n: merged }) + t("mergeSep") + t("updateShelvedTail", { n: shelved.length })
      : t("updateMergedN", { n: merged })
    : t("updateShelvedN", { n: shelved.length })
  const first = shelved.find((s) => !!s)
  toast(text, {
    description: shelved.length ? t("shelvedKeepNote") : undefined,
    duration: shelved.length ? 10000 : undefined,
    action: first ? { label: t("resolveConflicts"), onClick: () => openResolve(first.ref) } : undefined,
  })
}

export const PLACEHOLDER_DESC = "What this skill does and when an agent should use it."
