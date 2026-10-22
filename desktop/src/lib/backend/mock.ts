// In-memory Backend used until the Go bindings exist. It behaves like skd on
// the sample library in mock-data.ts; nothing touches the real file system.
import {
  CATALOG,
  CLAUDE_OVERRIDES,
  DRAFT,
  HOME,
  LIBRARY,
  OWNER,
  REMOTE,
  SAMPLE_PR,
  SHELVED,
  SYNC_REMOTE,
  initialAgents,
  initialFiles,
  initialHistory,
  initialOutdated,
  initialPacks,
  initialPlan,
  initialScan,
  initialSkills,
  initialTrash,
  refKey,
  sha,
  skillDir,
  trashBatch,
  upstreamDescription,
  vendoredFiles,
} from "./mock-data"
import { SHARED_READERS } from "@/lib/agents"
import { parseSource } from "./parse-source"
import type {
  Absent,
  Agent,
  Backend,
  CliStatus,
  Config,
  ConflictBlock,
  ConflictFile,
  ContributeMode,
  ContributeOptions,
  ContributeOutcome,
  Contribution,
  DiffHunk,
  DiffLine,
  FileDiff,
  FileNode,
  HistoryEntry,
  LibraryStatus,
  Outdated,
  Pack,
  PlanItem,
  Ref,
  RepoPreview,
  ScanItem,
  ShelvedSkill,
  Skill,
  SkillDiff,
  TrashEntry,
} from "./types"
import { SYNC_REF } from "./types"
import { shortSha } from "@/lib/utils"

const CONFIG_KEY = "skilldots.desktop.config"
const DEFAULT_CONFIG: Config = {
  editor: "auto",
  editorCommand: "",
  lang: "zh",
  theme: "system",
  checkOnLaunch: true,
}

const wait = (ms = 260) => new Promise((r) => setTimeout(r, ms))
const clone = <T>(x: T): T => structuredClone(x)
const NAME = /^[A-Za-z0-9_-][A-Za-z0-9._-]*$/
const PLACEHOLDER = "What this skill does and when an agent should use it."

interface State {
  root: string | null
  remote?: string
  changes: number
  lastSync?: string
  skills: Skill[]
  packs: Pack[]
  agents: Agent[]
  files: Record<string, Record<string, string>>
  outdated: Outdated | null
  history: HistoryEntry[]
  trash: TrashEntry[]
  plan: PlanItem[]
  scan: ScanItem[]
  /** Vendored skills whose files the user edited ("edit anyway"). */
  touched: Set<string>
  /** Skills made in the person's own repository and not pushed yet (lock draft), by key. */
  drafts: Set<string>
  /** Shelved updates by skill key (lock Shelved), and how far resolving them got. */
  shelved: Map<string, Shelf>
  /** What was sent upstream, by skill key (lock contribution). */
  contributions: Map<string, Contribution>
  /** The library's remote has changes to the same lines as this Mac (SYNC_REMOTE): syncing conflicts until resolved. */
  remoteAhead: boolean
  /** A sync merge being resolved (`skd sync --resolve`): the conflicted files by path in the library, with markers. */
  syncMerge: Record<string, string> | null
}

interface Shelf {
  files: string[]
  /** What resolving writes: the merge, conflicted files with Git's markers. */
  merge: Record<string, string>
  /** Common ancestor of each conflict, by its local side. */
  bases?: Record<string, string>
  /** Set while resolving: the files as they were (restored by abort) and the trash entry holding that copy. */
  backup?: Record<string, string>
  trashId?: string
}

function sampleState(root: string | null): State {
  const now = new Date()
  const skills = initialSkills()
  const files = initialFiles(skills)
  // Vendored skills the user edited, with updates waiting: one edit merges cleanly (a line in the middle), the other
  // adds a note at the end, where upstream adds a section too, so updating shelves it with a conflict.
  const touched = new Set<string>()
  const edit = (name: string, change: (md: string) => string) => {
    const s = skills.find((x) => x.kind === "vendor" && x.name === name)
    if (!s || !files[s.ref.key]?.["SKILL.md"]) return
    files[s.ref.key]["SKILL.md"] = change(files[s.ref.key]["SKILL.md"])
    touched.add(s.ref.key)
  }
  edit("differential-review", (md) => md + "\nMy note: also check the migration files.\n")
  edit("skilldots", (md) => md.replace("1. Read the task.", "1. Read the task and the library's skills.toml."))
  // Your own repository, up to date: both a PR and a direct push are possible.
  edit("ui-arsenal", (md) => md.replace("3. Check the result.", "3. Check the result against the catalogue's licence column."))
  // Edits to two of your own skills on this Mac; another Mac changed the same lines and synced first (SYNC_REMOTE).
  for (const r of SYNC_REMOTE) if (files[r.key]?.[r.file]) files[r.key][r.file] = files[r.key][r.file].replace(r.base, r.local)
  // Yesterday's obsidian-skills update (history h3) merged your obsidian-cli and shelved your obsidian-markdown.
  touched.add("kepano/obsidian-skills:obsidian-cli")
  touched.add(SHELVED.key)
  // A skill made in your own repository (ui-arsenal) and not pushed yet: sent with the ui-arsenal edit by 推送.
  const draft: Skill = {
    ref: { key: refKey("vendor", DRAFT.repo, DRAFT.name) },
    name: DRAFT.name,
    linkName: DRAFT.name,
    kind: "vendor",
    origin: DRAFT.repo,
    category: "",
    description: DRAFT.description,
    dir: skillDir("vendor", DRAFT.repo, "", DRAFT.name),
    targets: ["agents", "claude"],
  }
  skills.push(draft)
  Object.assign(files, initialFiles([draft]))
  const drafts = new Set([draft.ref.key])
  const shelved = new Map<string, Shelf>([[SHELVED.key, { files: ["SKILL.md"], merge: { "SKILL.md": SHELVED.markers }, bases: SHELVED.bases }]])
  // A skill of yours also chosen for Kiro, which this Mac does not have: shown as chosen but not linked.
  skills.find((s) => s.name === "design-review")?.targets.push("kiro")
  // Copies another installer put in agent folders and that were changed since: adopted as your own, not replaced by upstream.
  const differs: ScanItem[] = [
    { id: "claude/write-a-prd", agent: "claude", name: "write-a-prd", path: `${HOME}/.claude/skills/write-a-prd`, state: "copy", source: "mattpocock/skills", proposal: "own", label: "", upstream: "mattpocock/skills", differs: true, as: "write-a-prd" },
    { id: "cursor/ubiquitous-language", agent: "cursor", name: "ubiquitous-language", path: `${HOME}/.cursor/skills/ubiquitous-language`, state: "copy", source: "mattpocock/skills", proposal: "own", label: "", upstream: "mattpocock/skills", differs: true, as: "ubiquitous-language" },
  ]
  return {
    root,
    remote: REMOTE,
    // Not synced since two days ago: yesterday's removal and update, and the edits to subagent and release-notes.
    changes: 4,
    lastSync: new Date(now.getTime() - 2 * 24 * 3600 * 1000).toISOString(),
    skills,
    packs: initialPacks(),
    agents: initialAgents(),
    files,
    outdated: initialOutdated(now),
    history: initialHistory(now),
    trash: initialTrash(now),
    plan: initialPlan(),
    scan: [...initialScan(skills), ...differs],
    touched,
    drafts,
    shelved,
    contributions: new Map([[SAMPLE_PR.key, { ...SAMPLE_PR.contribution }]]),
    remoteAhead: true,
    syncMerge: null,
  }
}

/** A new library: nothing in it yet (initLibrary adds the skilldots skill when asked). */
function emptyState(root: string | null): State {
  const s = sampleState(root)
  return {
    ...s,
    remote: undefined,
    changes: 1,
    lastSync: undefined,
    skills: [],
    packs: [],
    files: {},
    outdated: null,
    history: [],
    trash: [],
    plan: [],
    touched: new Set(),
    drafts: new Set(),
    shelved: new Map(),
    contributions: new Map(),
    remoteAhead: false,
    syncMerge: null,
    // The same machine: what the agents have that differs from upstream is there before any library too.
    scan: [...initialScan([]), ...s.scan.filter((x) => x.differs)],
  }
}

export function createMockBackend(): Backend {
  const fresh = typeof location !== "undefined" && new URLSearchParams(location.search).has("fresh")
  let st: State = fresh ? emptyState(null) : sampleState(LIBRARY)
  // The preview never touches /usr/local/bin: the command is only pretended.
  let cli: CliStatus = { path: "/usr/local/bin/skd", state: "missing", target: "", canInstall: true, reason: "" }
  // Like Session.HasSelfSkill: the skill from GitHub, or the app's own copy as an [[app]] entry.
  const selfSkill = () => st.packs.some((p) => p.repo === "Maybe404/skilldots") || st.skills.some((s) => s.kind === "app" && s.name === "skilldots")

  const find = (ref: Ref): Skill => {
    const s = st.skills.find((x) => x.ref.key === ref.key)
    if (!s) throw new Error(`no skill ${ref.key}`)
    return s
  }
  const changed = () => {
    st.changes += 1
  }
  // Built-in agents not installed on this machine: apply links nothing there (the shared folder is always made).
  const absentAgents = () => st.agents.filter((a) => a.builtin && !a.detected && a.name !== "agents").map((a) => a.name)
  const absentOf = (list: Skill[]): Absent => {
    const away = absentAgents()
    const out: Absent = {}
    for (const s of list) for (const a of s.targets) if (away.includes(a)) out[a] = [...(out[a] ?? []), s.linkName]
    return out
  }
  // Like the note skd keeps beside a removed or purged entry: what restoring puts back.
  const notes = new Map<string, { skills: Skill[]; pack?: Pack }>()
  const toTrash = (name: string, origin: string, reason?: TrashEntry["reason"], note?: { skills: Skill[]; pack?: Pack }) => {
    const batch = trashBatch(new Date().toISOString())
    const id = `${batch}/${name}`
    st.trash.unshift({
      id,
      name,
      origin,
      path: `${st.root}/.trash/${batch}/${name}`,
      when: new Date().toISOString(),
      reason,
      skills: note?.skills.map((s) => s.name),
      repo: note?.pack?.repo,
    })
    if (note) notes.set(id, clone(note))
  }
  // Like the bridge's resolveState: each conflicted file parsed from the markers it has now.
  const conflictsOf = (key: string): { files: ConflictFile[] } => {
    if (key === SYNC_REF.key) {
      // The sync merge: paths relative to the library root.
      const bases = Object.fromEntries(SYNC_REMOTE.map((r) => [r.local, r.base]))
      return clone({ files: Object.entries(st.syncMerge ?? {}).map(([path, text]) => parseConflicts(path, text, bases)) })
    }
    const shelf = st.shelved.get(key)
    const files = (shelf?.files ?? []).map((path): ConflictFile => {
      const f = parseConflicts(path, st.files[key]?.[path] ?? "", shelf?.bases ?? {})
      return shelf?.backup ? f : { ...f, blocks: [] }
    })
    return clone({ files })
  }
  // Like app.ContributeOptions: repositories of the sample person (OWNER) take a push, any GitHub repository a PR
  // (gh permitting). Several skills of one pack go together (contributePack). In the person's own repository
  // (own = true) being behind does not block: sending pulls the new commits first (pull).
  const unpushed = (s: Skill) => s.kind === "vendor" && (st.touched.has(s.ref.key) || st.drafts.has(s.ref.key))
  const contributeOptions = (repo: string, list: Skill[]): ContributeOptions => {
    for (const s of list) if (s.kind !== "vendor") throw new Error(`${s.name} is not from a source; there is no upstream to send it to`)
    if (!list.length || !list.every(unpushed)) throw new Error(list.length === 1 ? `${list[0].name} has no edits to send` : `nothing to send in ${repo}`)
    const zh = readConfig().lang === "zh"
    const asked = typeof location !== "undefined" ? new URLSearchParams(location.search).get("gh") : null
    const gh: ContributeOptions["gh"] = asked === "missing" || asked === "logged-out" ? asked : "ok"
    const own = !!st.packs.find((p) => p.repo === repo)?.own
    const changed = st.outdated?.packs.find((p) => p.repo === repo)?.changed ?? []
    const behind = list.some((s) => st.shelved.has(s.ref.key) || changed.includes(s.name))
    const pull = own && behind
    const pushable = repo.startsWith(OWNER + "/")
    const reasons: string[] = []
    if (behind && !pull) reasons.push(zh ? `上游有更新的版本：先更新 ${repo}（有冲突就先解决），再提交。` : `Upstream has a newer version: update ${repo} (and resolve any conflicts) first.`)
    else {
      if (!pushable) reasons.push(zh ? `你没有 ${repo} 的推送权限，只能提 PR。` : `You cannot push to ${repo}; open a PR instead.`)
      if (gh === "missing") reasons.push(zh ? "提 PR 需要 gh：brew install gh，再 gh auth login。" : "A PR needs gh: brew install gh, then gh auth login.")
      if (gh === "logged-out") reasons.push(zh ? "gh 还没登录：在终端运行 gh auth login。" : "gh is not logged in: run gh auth login in a terminal.")
    }
    const drafts = list.filter((s) => st.drafts.has(s.ref.key))
    const updated = list.filter((s) => !st.drafts.has(s.ref.key)).map((s) => s.name)
    const title = [updated.length ? `Update ${updated.join(", ")}` : "", drafts.length ? `Add ${drafts.map((s) => s.name).join(", ")}` : ""].filter(Boolean).join("; ")
    return {
      repo,
      host: "github",
      canPush: pushable && (!behind || pull),
      canPR: gh === "ok" && (!behind || pull),
      gh,
      behind,
      reason: reasons.join(" "),
      defaultMessage: `${title}\n\nEdited with skilldots`,
      branch: "main",
      own,
      pull,
      skills: list.map((s) => s.linkName),
      drafts: drafts.map((s) => s.linkName),
      paths: Object.fromEntries(drafts.map((s) => [s.linkName, s.name === DRAFT.name ? DRAFT.path : s.category ? `${s.category}/${s.name}` : s.name])),
    }
  }
  // Like Session.Contribute: one commit for every skill listed. In the person's own repository a push is pulled
  // back afterwards (an update of the pack), so the skills match the repository again and the update records
  // them as landed; ?pullback=fail in the preview URL plays that update failing.
  const contribute = (repo: string, list: Skill[], mode: ContributeMode, message: string): ContributeOutcome => {
    const o = contributeOptions(repo, list)
    if ((o.behind && !o.pull) || (mode === "pr" && !o.canPR) || (mode === "push" && !o.canPush)) throw new Error(o.reason || `cannot ${mode} to ${o.repo}`)
    if (!message.trim()) throw new Error("a commit message is needed")
    const pack = st.packs.find((p) => p.repo === repo)!
    const status = st.outdated?.packs.find((p) => p.repo === repo)
    if (o.pull && status) {
      // Pulled first: the repository's new commits merged into your edits.
      pack.commit = status.latest
      Object.assign(status, { commit: status.latest, changed: [], moved: [], new: [], gone: [], pending: [], commits: [] })
    }
    const d = new Date()
    const p = (n: number) => String(n).padStart(2, "0")
    const stamp = `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}`
    const base = { skills: o.skills, pulled: o.pull, pulledBack: false, pullBackError: "" }
    if (mode === "pr") {
      const number = 13 + [...st.contributions.values()].filter((c) => c.kind === "pr").length
      const c: Contribution = { kind: "pr", url: `https://github.com/${repo}/pull/${number}`, number, branch: `skilldots/${list.length === 1 ? list[0].name : "update"}-${stamp}`, state: "open" }
      for (const s of list) st.contributions.set(s.ref.key, { ...c })
      changed()
      return { ...base, mode, url: c.url, number, branch: c.branch, pushed: false }
    }
    const commit = sha(Date.now().toString(16).slice(-7))
    const c: Contribution = { kind: "push", url: `https://github.com/${repo}/commit/${commit}`, number: 0, branch: o.branch, state: "pushed" }
    for (const s of list) st.contributions.set(s.ref.key, { ...c })
    changed()
    if (!o.own) return { ...base, mode, url: c.url, number: 0, branch: c.branch, pushed: true }
    if (typeof location !== "undefined" && new URLSearchParams(location.search).get("pullback") === "fail")
      return { ...base, mode, url: c.url, number: 0, branch: c.branch, pushed: true, pullBackError: `git fetch ${repo}: connection reset by peer` }
    const names = list.map((s) => s.name)
    st.history.unshift({ id: `h${Date.now()}-${repo}`, when: new Date().toISOString(), repo, from: pack.commit, to: commit, changed: [], added: [], gone: [], moved: [], landed: names, commits: [{ id: commit, title: message.split("\n")[0] }], backups: [] })
    pack.commit = commit
    if (status) status.commit = status.latest = commit
    for (const s of list) {
      st.touched.delete(s.ref.key)
      st.drafts.delete(s.ref.key)
    }
    pack.upstreamCount = Math.max(pack.upstreamCount, st.skills.filter((s) => s.kind === "vendor" && s.origin === repo).length)
    return { ...base, mode, url: c.url, number: 0, branch: c.branch, pushed: true, pulledBack: true }
  }
  const status = (): LibraryStatus => ({
    root: st.root,
    remote: st.remote,
    branch: "main",
    changes: st.changes,
    lastSync: st.lastSync,
    unmanaged: st.scan.length,
    selfSkill: selfSkill(),
    // The preview plays the packaged app, which carries the skill.
    bundledSkill: true,
    syncing: !!st.syncMerge,
  })

  const readConfig = (): Config => {
    try {
      const raw = localStorage.getItem(CONFIG_KEY)
      return raw ? { ...DEFAULT_CONFIG, ...JSON.parse(raw) } : DEFAULT_CONFIG
    } catch {
      return DEFAULT_CONFIG
    }
  }

  const addSkill = (s: Omit<Skill, "ref" | "dir" | "linkName">, body?: Record<string, string>) => {
    const skill: Skill = {
      ...s,
      ref: { key: refKey(s.kind, s.origin, s.name) },
      linkName: s.name,
      dir: skillDir(s.kind, s.origin, s.category, s.name),
    }
    st.skills.push(skill)
    st.files[skill.ref.key] = body ?? initialFiles([skill])[skill.ref.key]
    return skill
  }

  // Like InstallSelfSkill in the packaged app: the skill shipped inside the app, as an [[app]] entry
  // (no download), linked into every agent on this machine.
  const installSelf = () => {
    if (selfSkill()) return
    const skill = addSkill({
      name: "skilldots",
      kind: "app",
      origin: "skilldots",
      category: "",
      description: upstreamDescription("Maybe404/skilldots", "skilldots"),
      // What "*" expands to here: agents that read ~/.agents/skills get it from there.
      targets: st.agents.filter((a) => a.detected && !SHARED_READERS.includes(a.name)).map((a) => a.name),
    })
    skill.dir = "/Applications/skilldots.app/Contents/Resources/skills/skilldots"
  }

  return {
    async status() {
      await wait(120)
      return status()
    },
    async initLibrary(dir, withSkill) {
      await wait(500)
      st = emptyState(dir || `${HOME}/.skilldots`)
      if (withSkill) installSelf()
      return status()
    },
    async installSelfSkill() {
      await wait(600)
      installSelf()
      return status()
    },
    async cliStatus() {
      await wait(60)
      return { ...cli }
    },
    async installCli() {
      await wait(700)
      cli = { ...cli, state: "installed", target: "/Applications/skilldots.app/Contents/MacOS/skilldots" }
      return { ...cli }
    },
    async removeCli() {
      await wait(300)
      cli = { ...cli, state: "missing", target: "" }
      return { ...cli }
    },
    async cloneLibrary(url, dir) {
      await wait(1200)
      if (!/github\.com|^git@|\//.test(url)) throw new Error(`cannot clone ${url}`)
      // A repository named just "skills" stands for someone's repository of skills (app.CloneLibrary).
      if (/\/skills(\.git)?$/.test(url))
        throw new Error(`${url} is not a skilldots library but a repository of skills (no skills.toml at its top); nothing was kept. To use its skills, add it as a source: skd add ${url} --all`)
      st = sampleState(dir || `${HOME}/.skilldots`)
      st.remote = url
      st.changes = 0
      return status()
    },
    async useLibrary(dir) {
      await wait(400)
      if (!dir.startsWith("/") && !dir.startsWith("~")) throw new Error("not an absolute path")
      st = sampleState(dir)
      return status()
    },
    async pickFolder() {
      await wait(200)
      return LIBRARY
    },

    async skills() {
      await wait(80)
      const away = absentAgents()
      // Like engine.markViaShared: with Agents a target and not paused, the agents that read ~/.agents/skills get it
      // from there (no link of their own); sharedReach is every installed such agent, chosen or not.
      const shared = st.agents.find((a) => a.name === "agents")
      const readers = st.agents.filter((a) => a.detected && SHARED_READERS.includes(a.name)).map((a) => a.name)
      return clone(
        st.skills.map((s) => {
          const serves = !!shared?.enabled && s.targets.includes("agents")
          const shelf = st.shelved.get(s.ref.key)
          return {
            ...s,
            shelved: !!shelf,
            resolving: !!shelf?.backup,
            conflictFiles: shelf?.files ?? [],
            edited: unpushed(s),
            own: s.kind === "vendor" && !!st.packs.find((p) => p.repo === s.origin)?.own,
            draft: st.drafts.has(s.ref.key),
            contribution: st.contributions.get(s.ref.key) ?? null,
            absent: s.targets.filter((a) => away.includes(a)),
            viaShared: serves ? s.targets.filter((a) => SHARED_READERS.includes(a)) : [],
            sharedReach: serves ? readers : [],
          }
        }),
      )
    },
    async packs() {
      await wait(60)
      return clone(st.packs)
    },
    async agents() {
      await wait(60)
      // Like the bridge: which agents read ~/.agents/skills too, and an agent not here is shown while a skill names it.
      return clone(
        st.agents.map((a) => ({
          ...a,
          readsShared: SHARED_READERS.includes(a.name),
          hidden: a.hidden && !st.skills.some((s) => s.targets.includes(a.name)),
        })),
      )
    },
    async claudeOverrides() {
      return [...CLAUDE_OVERRIDES]
    },

    async setAgentHidden(name, hidden) {
      const agent = st.agents.find((a) => a.name === name)
      if (!agent) throw new Error(`unknown agent ${name}`)
      agent.hidden = hidden
    },
    async setAgentEnabled(name, enabled) {
      await wait(150)
      const a = st.agents.find((x) => x.name === name)
      if (!a) throw new Error(`unknown agent ${name}`)
      a.enabled = enabled
      changed()
    },
    async setDefaultAgents(names) {
      await wait(150)
      if (!names.length) throw new Error("at least one default agent is needed")
      for (const a of st.agents) a.isDefault = names.includes(a.name)
      changed()
    },

    async parseSource(input) {
      await wait(60)
      return parseSource(input)
    },
    async show(src) {
      await wait(900)
      const cat = CATALOG[src.repo]
      if (!cat) throw new Error(`repository ${src.repo} not found (the sample backend knows ${Object.keys(CATALOG).join(", ")})`)
      const pack = st.packs.find((p) => p.repo === src.repo)
      const have = st.skills.filter((s) => s.origin === src.repo).map((s) => s.name)
      // Like Fetched.Preselect: a name, category/name or folder; what matches nothing is reported, not an error.
      const chosen = new Set<string>()
      const missing: string[] = []
      for (const n of src.skills) {
        const hit = cat.skills.find((s) => s.name === n || `${s.category}/${s.name}` === n || n.split("/").pop() === s.name)
        if (hit) chosen.add(hit.name)
        else missing.push(n)
      }
      const preselected = cat.skills.filter((s) => chosen.has(s.name)).map((s) => s.name)
      return clone({ ...cat, source: src, subscribed: !!pack, have, preselected, missing })
    },
    async add(preview: RepoPreview, sel) {
      await wait(800)
      const repo = preview.source.repo
      const names = new Set(sel.skills)
      for (const c of sel.categories) preview.skills.filter((s) => s.category === c).forEach((s) => names.add(s.name))
      if (sel.all) preview.skills.forEach((s) => names.add(s.name))
      for (const h of preview.have) names.delete(h)
      // The person's own repository may have no skill yet: then it is followed whole.
      const emptyOwn = !!sel.own && preview.skills.length === 0
      const known = st.packs.find((p) => p.repo === repo)
      if (!names.size && !emptyOwn) {
        if (known && sel.own) known.own = true
        return { repo, linked: [], renamed: {}, nothing: true, own: !!known?.own }
      }
      if (sel.alias && (sel.alias.includes("/") || sel.alias.includes(":"))) throw new Error(`alias ${sel.alias} may not contain / or :`)
      if (!st.packs.some((p) => p.repo === repo)) {
        st.packs.push({
          repo,
          alias: sel.alias || undefined,
          ref: preview.source.ref,
          commit: preview.commit,
          followAll: sel.all || emptyOwn,
          upstreamCount: preview.skills.length,
          license: preview.license,
          url: preview.source.url,
          own: !!sel.own,
        })
      } else if (sel.own) known!.own = true
      const renamed: Record<string, string> = {}
      const linked: string[] = []
      for (const n of names) {
        const up = preview.skills.find((s) => s.name === n)!
        const taken = st.skills.some((s) => s.linkName.toLowerCase() === n.toLowerCase())
        const skill = addSkill({ name: n, kind: "vendor", origin: repo, category: up.category, description: up.description, targets: [...sel.agents] })
        if (taken) {
          skill.linkName = `${sel.alias || repo.split("/")[1]}-${n}`
          renamed[n] = skill.linkName
        }
        linked.push(skill.linkName)
      }
      st.scan = st.scan.filter((x) => !(x.proposal === "pack" && x.repo === repo && names.has(x.name)))
      changed()
      return { repo, linked, renamed, nothing: false, absent: absentOf(st.skills), own: !!st.packs.find((p) => p.repo === repo)?.own }
    },

    async enable(ref, agents) {
      await wait(150)
      const s = find(ref)
      s.targets = [...new Set([...s.targets, ...agents])]
      changed()
      return { absent: absentOf(st.skills) }
    },
    async disable(ref, agents) {
      await wait(150)
      const s = find(ref)
      s.targets = agents.length ? s.targets.filter((t) => !agents.includes(t)) : []
      changed()
      return { absent: absentOf(st.skills) }
    },
    async setTargets(ref, agents) {
      await wait(150)
      find(ref).targets = [...agents]
      changed()
      return { absent: absentOf(st.skills) }
    },
    async remove(ref) {
      await wait(300)
      const s = find(ref)
      if (s.kind === "app") throw new Error(`${s.name} belongs to an application; disable it instead`)
      st.skills = st.skills.filter((x) => x !== s)
      const pack = st.packs.find((p) => p.repo === s.origin)
      toTrash(s.name, s.dir, "remove", { skills: [s], pack: s.kind === "vendor" ? pack : undefined })
      if (!st.skills.some((x) => x.origin === s.origin && x.kind === "vendor")) st.packs = st.packs.filter((p) => p.repo !== s.origin)
      changed()
    },
    async purge(repo) {
      await wait(400)
      const pack = st.packs.find((p) => p.repo === repo)
      if (!pack) throw new Error(`not subscribed: ${repo}`)
      const gone = st.skills.filter((x) => x.kind === "vendor" && x.origin === repo)
      st.skills = st.skills.filter((x) => !gone.includes(x))
      st.packs = st.packs.filter((p) => p.repo !== repo)
      if (st.outdated) st.outdated.packs = st.outdated.packs.filter((p) => p.repo !== repo)
      toTrash(repo.split("/")[1], `${st.root}/vendor/${repo}`, "purge", { skills: gone, pack })
      changed()
    },
    async setOwn(repo, own) {
      await wait(150)
      const pack = st.packs.find((p) => p.repo === repo)
      if (!pack) throw new Error(`not subscribed: ${repo}`)
      pack.own = own
      changed()
    },
    async looksOwn(url) {
      await wait(300)
      // Like app.LooksOwn: GitHub says the sample person administers their own repositories.
      return url.includes(`github.com/${OWNER}/`)
    },
    async fork(ref, opts) {
      await wait(500)
      const s = find(ref)
      const name = opts.name.trim() || s.name
      if (!NAME.test(name)) throw new Error(`invalid name ${name}`)
      const kind = opts.private ? "private" : "skills"
      if (st.skills.some((x) => x !== s && (x.kind === "skills" || x.kind === "private") && x.name === name))
        throw new Error(`${kind}/${name} already exists`)
      const files = clone(st.files[s.ref.key] ?? {})
      if (files["SKILL.md"]) files["SKILL.md"] = files["SKILL.md"].replace(/^name:.*$/m, `name: ${name}`)
      if (s.kind === "vendor") {
        st.skills = st.skills.filter((x) => x !== s)
        st.touched.delete(s.ref.key)
        st.drafts.delete(s.ref.key)
      }
      const created = addSkill({ name, kind, origin: "", category: "", description: s.description, targets: [...s.targets] }, files)
      changed()
      return clone(created)
    },
    async newSkill(opts) {
      await wait(400)
      if (!NAME.test(opts.name)) throw new Error(`invalid name ${opts.name}`)
      if (opts.pack) {
        // Like Session.NewInPack: a draft in the person's own repository, pushed there later.
        const pack = st.packs.find((p) => p.repo === opts.pack)
        if (!pack) throw new Error(`not subscribed: ${opts.pack}`)
        if (!pack.own) throw new Error(`${opts.pack} is not marked as your own repository`)
        if (opts.private) throw new Error("a skill in your own repository is pushed there; it cannot be private")
        if (st.skills.some((x) => x.linkName === opts.name)) throw new Error(`${opts.name} already exists`)
        const dir = (opts.dir ?? "").trim().replace(/^\/+|\/+$/g, "")
        if (dir.split("/").includes("..")) throw new Error(`${dir}: not a directory in the repository`)
        const description = opts.description.trim() || PLACEHOLDER
        const category = dir && dir !== "." ? dir : ""
        const created = addSkill({ name: opts.name, kind: "vendor", origin: pack.repo, category, description, targets: [...opts.agents] })
        st.drafts.add(created.ref.key)
        changed()
        return clone({ ...created, own: true, draft: true, edited: true })
      }
      const kind = opts.private ? "private" : "skills"
      if (st.skills.some((x) => (x.kind === "skills" || x.kind === "private") && x.name === opts.name)) throw new Error(`${kind}/${opts.name} already exists`)
      const description = opts.description.trim() || PLACEHOLDER
      const created = addSkill({ name: opts.name, kind, origin: "", category: "", description, targets: [...opts.agents] })
      changed()
      return clone(created)
    },
    async reveal(ref) {
      await wait(100)
      find(ref)
    },
    async revealLibrary() {
      await wait(100)
    },
    async openInEditor(ref) {
      await wait(100)
      find(ref)
    },

    async files(ref) {
      await wait(120)
      return tree(Object.keys(st.files[ref.key] ?? {}))
    },
    async openUrl(url) {
      // The browser preview opens a new tab; the app asks the system browser (Service.openUrl).
      window.open(url, "_blank", "noopener,noreferrer")
    },
    async readFile(ref, path) {
      await wait(60)
      // A shelved sync's conflicted file (Service: readFile with @sync), markers and all.
      if (ref.key === SYNC_REF.key) {
        const text = st.syncMerge?.[path]
        if (text === undefined) throw new Error(`${path} is not a conflicted file of this sync`)
        return text
      }
      const f = st.files[ref.key]?.[path]
      if (f === undefined) throw new Error(`no file ${path}`)
      return f
    },
    async writeFile(ref, path, content) {
      await wait(200)
      if (ref.key === SYNC_REF.key) {
        if (st.syncMerge?.[path] === undefined) throw new Error(`${path} is not a conflicted file of this sync`)
        st.syncMerge[path] = content
        return
      }
      const s = find(ref)
      st.files[ref.key][path] = content
      if (path === "SKILL.md") {
        const m = content.match(/^---\n([\s\S]*?)\n---/)
        const d = m?.[1].match(/^description:\s*(.*)$/m)?.[1]
        if (d) s.description = d.startsWith('"') ? safeJson(d) : d
      }
      if (s.kind === "vendor" || s.kind === "app") st.touched.add(s.ref.key)
      if (s.kind !== "private" && s.kind !== "app") changed()
    },

    async plan() {
      await wait(150)
      return clone(st.plan)
    },
    async apply({ force }) {
      await wait(500)
      let linked = 0
      let unlinked = 0
      let trashed = 0
      const left: PlanItem[] = []
      for (const p of st.plan) {
        if (p.action === "link" || p.action === "relink") linked++
        else if (p.action === "unlink") unlinked++
        else if (force) {
          toTrash(p.skill, p.path)
          trashed++
          linked++
        } else left.push(p)
      }
      for (const p of st.plan) {
        const s = st.skills.find((x) => x.linkName === p.skill)
        if (!s) continue
        if (p.action === "link" || (p.action === "blocked" && force)) s.targets = [...new Set([...s.targets, p.agent])]
        if (p.action === "unlink") s.targets = s.targets.filter((t) => t !== p.agent)
      }
      st.plan = left
      return { linked, unlinked, trashed, absent: absentOf(st.skills) }
    },

    async recordedOutdated() {
      await wait(60)
      return clone(st.outdated)
    },
    async outdated(repos) {
      await wait(1400)
      const now = new Date().toISOString()
      if (!st.outdated) st.outdated = { checked: now, packs: [], failed: [] }
      for (const p of st.packs) {
        if (repos?.length && !repos.includes(p.repo)) continue
        let s = st.outdated.packs.find((x) => x.repo === p.repo)
        if (!s) {
          s = { repo: p.repo, commit: p.commit, latest: p.commit, changed: [], moved: [], new: [], gone: [], pending: [], commits: [], checked: now }
          st.outdated.packs.push(s)
        }
        s.checked = now
      }
      st.outdated.checked = now
      return clone(st.outdated)
    },
    async update(repos, opts) {
      await wait(1500)
      const out: HistoryEntry[] = []
      for (const repo of repos) {
        const s = st.outdated?.packs.find((x) => x.repo === repo)
        const pack = st.packs.find((p) => p.repo === repo)
        if (!s || !pack) continue
        const merged: string[] = []
        const shelvedNow: ShelvedSkill[] = []
        const title = s.commits[0]?.title ?? "Update"
        for (const name of s.changed) {
          const sk = st.skills.find((x) => x.origin === repo && x.name === name)
          if (!sk) continue
          const base = vendoredFiles(sk)
          const mine = st.files[sk.ref.key] ?? base
          if (!st.touched.has(sk.ref.key)) {
            st.files[sk.ref.key] = upstreamVersion(base, title)
            continue
          }
          // Like engine.Sync: your edits merged with upstream's. Upstream's sample change adds a section at the end of
          // SKILL.md, so an edit that added lines there too conflicts: the update is shelved and the folder left alone.
          const md = base["SKILL.md"] ?? ""
          const added = mine["SKILL.md"]?.startsWith(md) ? mine["SKILL.md"].slice(md.length).trim() : ""
          if (added) {
            const short = shortSha(s.latest)
            const merge = upstreamVersion(base, title)
            merge["SKILL.md"] = withUpstreamDescription(md).trimEnd() + `\n\n<<<<<<< local\n${added}\n=======\n${upstreamSection(title).trim()}\n>>>>>>> upstream ${short}\n`
            st.shelved.set(sk.ref.key, { files: ["SKILL.md"], merge, bases: { [added]: "" } })
            shelvedNow.push({ name, files: ["SKILL.md"] })
            continue
          }
          // Clean: the folder as it was goes to the trash first. It stays edited, so the next update merges again.
          toTrash(name, sk.dir, "merged")
          st.files[sk.ref.key] = upstreamVersion(mine, title)
          merged.push(name)
        }
        for (const mv of s.moved) {
          const [name, cats] = mv.split(": ")
          const sk = st.skills.find((x) => x.origin === repo && x.name === name)
          const next = cats?.split(" → ")[1]
          if (sk && next !== undefined) {
            sk.category = next
            sk.dir = skillDir("vendor", repo, next, name)
          }
        }
        const added = opts?.addNew?.[repo] ?? []
        for (const n of added) {
          addSkill({ name: n, kind: "vendor", origin: repo, category: "", description: "", targets: st.agents.filter((a) => a.isDefault).map((a) => a.name) })
        }
        const entry: HistoryEntry = {
          id: `h${Date.now()}-${repo}`,
          when: new Date().toISOString(),
          repo,
          from: s.commit,
          to: s.latest,
          changed: s.changed,
          added,
          gone: s.gone,
          moved: s.moved,
          commits: s.commits,
          backups: [],
          merged,
          shelved: shelvedNow,
        }
        pack.commit = s.latest
        Object.assign(s, { commit: s.latest, changed: [], moved: [], new: [], gone: [], pending: [], commits: [] })
        st.history.unshift(entry)
        out.push(entry)
      }
      if (out.length) changed()
      return clone(out)
    },
    async updateDiff(repo) {
      await wait(700)
      const s = st.outdated?.packs.find((x) => x.repo === repo)
      if (!s || !st.packs.some((p) => p.repo === repo)) throw new Error(`no pack "${repo}"`)
      const skills: SkillDiff[] = []
      const title = s.commits[0]?.title ?? "Update"
      for (const name of s.changed) {
        const sk = st.skills.find((x) => x.origin === repo && x.name === name)
        if (!sk) continue
        const vendored = vendoredFiles(sk)
        const mine = st.files[sk.ref.key] ?? vendored
        const upstream = upstreamVersion(vendored, title)
        const localFiles = changedPaths(vendored, mine)
        const local = new Set(localFiles)
        skills.push({ name, status: "changed", local: localFiles.length > 0, localFiles, files: diffFiles(mine, upstream, local) })
      }
      for (const name of s.pending) skills.push({ name, status: "added", local: false, localFiles: [], files: diffFiles({}, { "SKILL.md": `---\nname: ${name}\n---\n` }, new Set()) })
      for (const name of s.landed ?? []) skills.push({ name, status: "landed", local: false, localFiles: [], files: [] })
      for (const mv of s.moved) {
        const name = mv.split(": ")[0]
        if (!skills.some((x) => x.name === name)) skills.push({ name, status: "moved", local: false, localFiles: [], files: [] })
      }
      return clone({ repo, commit: s.commit, latest: s.latest, skills })
    },
    async history() {
      await wait(100)
      return clone(st.history)
    },
    async historyDiff(repo, time) {
      await wait(500)
      const h = st.history.find((x) => x.repo === repo && x.when === time)
      if (!h) throw new Error(`no update of ${repo} at ${time}`)
      // Like app.HistoryDiff between the upstream snapshots before and after: upstream's changes, not yours.
      const title = h.commits[0]?.title ?? "Update"
      const before = (name: string): Record<string, string> => {
        const sk = st.skills.find((x) => x.origin === repo && x.name === name)
        return sk ? vendoredFiles(sk) : { "SKILL.md": `---\nname: ${name}\n---\n\n# ${name}\n` }
      }
      const skills: SkillDiff[] = []
      for (const name of h.changed) {
        const old = before(name)
        const next = repo === SHELVED.repo && name === SHELVED.name ? { "SKILL.md": SHELVED.theirs } : upstreamVersion(old, title)
        skills.push({ name, status: "changed", local: false, localFiles: [], files: diffFiles(old, next, new Set()) })
      }
      for (const name of h.added) skills.push({ name, status: "added", local: false, localFiles: [], files: diffFiles({}, before(name), new Set()) })
      for (const name of h.landed ?? []) skills.push({ name, status: "landed", local: false, localFiles: [], files: [] })
      for (const mv of h.moved) {
        const name = mv.split(": ")[0]
        if (!skills.some((x) => x.name === name)) skills.push({ name, status: "moved", local: false, localFiles: [], files: [] })
      }
      return clone({ skills })
    },

    async resolveStart(ref) {
      await wait(500)
      if (ref.key === SYNC_REF.key) {
        // Like skd sync --resolve: the merge is redone and left in the library with Git's markers.
        if (!st.remoteAhead) throw new Error("no shelved sync to resolve; sync first")
        if (!st.syncMerge) {
          st.syncMerge = {}
          for (const r of SYNC_REMOTE) {
            const text = st.files[r.key]?.[r.file] ?? ""
            const block = `<<<<<<< HEAD\n${r.local}\n=======\n${r.remote}\n>>>>>>> origin/main`
            st.syncMerge[r.path] = text.includes(r.local) ? text.replace(r.local, block) : `${text.trimEnd()}\n${block}\n`
          }
        }
        return conflictsOf(ref.key)
      }
      const s = find(ref)
      const shelf = st.shelved.get(ref.key)
      if (!shelf) throw new Error(`${s.name} has no shelved update`)
      if (!shelf.backup) {
        // Like skd resolve: the folder as it is goes to the trash (abort brings it back), the merge with markers is written.
        shelf.backup = clone(st.files[ref.key] ?? {})
        toTrash(s.name, s.dir, "merged")
        shelf.trashId = st.trash[0]?.id
        st.files[ref.key] = { ...st.files[ref.key], ...shelf.merge }
        changed()
      }
      return conflictsOf(ref.key)
    },
    async resolveState(ref) {
      await wait(120)
      if (ref.key !== SYNC_REF.key) find(ref)
      return conflictsOf(ref.key)
    },
    async resolvePick(ref, path, block, choice) {
      await wait(160)
      if (ref.key === SYNC_REF.key) {
        const text = st.syncMerge?.[path]
        if (text === undefined) throw new Error(`${path} is not being resolved`)
        st.syncMerge![path] = pickBlock(text, block, choice, path)
        return
      }
      const shelf = st.shelved.get(ref.key)
      const text = st.files[ref.key]?.[path]
      if (!shelf?.backup || text === undefined) throw new Error(`${path} is not being resolved`)
      st.files[ref.key][path] = pickBlock(text, block, choice, path)
    },
    async resolveContinue(ref) {
      await wait(500)
      if (ref.key === SYNC_REF.key) {
        // Like skd sync --continue: refused while markers remain; then the merge is committed, pushed and applied.
        if (!st.syncMerge) throw new Error("no sync merge is being resolved")
        for (const [path, text] of Object.entries(st.syncMerge)) if (MARKER.test(text)) throw new Error(`${path} still has conflict markers`)
        for (const r of SYNC_REMOTE) if (st.syncMerge[r.path] !== undefined && st.files[r.key]) st.files[r.key][r.file] = st.syncMerge[r.path]
        st.syncMerge = null
        st.remoteAhead = false
        st.changes = 0
        st.lastSync = new Date().toISOString()
        return {}
      }
      const s = find(ref)
      const shelf = st.shelved.get(ref.key)
      if (!shelf?.backup) throw new Error(`${s.name} is not being resolved`)
      for (const [path, text] of Object.entries(st.files[ref.key] ?? {})) {
        if (MARKER.test(text)) throw new Error(`${path} still has conflict markers`)
      }
      st.shelved.delete(ref.key)
      st.touched.add(ref.key)
      // The update's record now counts it as merged.
      const h = st.history.find((x) => x.repo === s.origin && (x.shelved ?? []).some((y) => (typeof y === "string" ? y : y.name) === s.name))
      if (h) {
        h.shelved = (h.shelved ?? []).filter((y) => (typeof y === "string" ? y : y.name) !== s.name)
        h.merged = [...(h.merged ?? []), s.name]
      }
      changed()
      return { absent: absentOf([s]) }
    },
    async resolveAbort(ref) {
      await wait(300)
      if (ref.key === SYNC_REF.key) {
        // Like skd sync --abort (git merge --abort): the library is as it was; the sync stays shelved.
        if (!st.syncMerge) throw new Error("no sync merge is being resolved")
        st.syncMerge = null
        return
      }
      const shelf = st.shelved.get(ref.key)
      if (!shelf?.backup) throw new Error(`${find(ref).name} is not being resolved`)
      st.files[ref.key] = shelf.backup
      st.trash = st.trash.filter((x) => x.id !== shelf.trashId)
      shelf.backup = undefined
      shelf.trashId = undefined
    },

    async localChanges(ref) {
      await wait(300)
      const s = find(ref)
      if (s.kind !== "vendor") throw new Error(`${s.name} is not from a source`)
      // Like app.LocalChanges: the upstream version your edits started from against the folder as it is (a draft has none).
      const base = st.drafts.has(ref.key) ? {} : vendoredFiles(s)
      const mine = st.files[ref.key] ?? base
      const localFiles = changedPaths(base, mine)
      return clone({ name: s.name, status: "changed" as const, local: localFiles.length > 0, localFiles, base: true, files: diffFiles(base, mine, new Set()) })
    },
    async contributeOptions(ref) {
      await wait(450)
      const s = find(ref)
      return clone(contributeOptions(s.origin, [s]))
    },
    async contribute(ref, mode, message) {
      await wait(1400)
      const s = find(ref)
      return clone(contribute(s.origin, [s], mode, message))
    },
    async contributePackOptions(repo) {
      await wait(450)
      return clone(contributeOptions(repo, st.skills.filter((s) => s.origin === repo && unpushed(s))))
    },
    async contributePack(repo, mode, message) {
      await wait(1600)
      return clone(contribute(repo, st.skills.filter((s) => s.origin === repo && unpushed(s)), mode, message))
    },
    async refreshContributions() {
      await wait(700)
      // The sample's PR from last week was merged upstream since.
      const c = st.contributions.get(SAMPLE_PR.key)
      if (c?.number === SAMPLE_PR.contribution.number) c.state = "merged"
    },

    async scan() {
      await wait(900)
      return clone(st.scan)
    },
    async adopt(picks) {
      await wait(900)
      let adopted = 0
      // Like the bridge: every row must still be what the confirmation showed, or nothing is adopted.
      for (const { id, kind } of picks) if (st.scan.find((x) => x.id === id)?.proposal !== kind) throw new Error(`scan item "${id}" changed; scan again`)
      for (const { id } of picks) {
        const it = st.scan.find((x) => x.id === id)
        if (!it || it.proposal === "skip") continue
        if (it.proposal === "pack" && it.repo) {
          if (!st.packs.some((p) => p.repo === it.repo)) {
            const cat = CATALOG[it.repo]
            st.packs.push({ repo: it.repo, commit: cat?.commit ?? sha("0000000"), followAll: false, upstreamCount: cat?.skills.length ?? 1, license: cat?.license, url: `https://github.com/${it.repo}` })
          }
          const have = st.skills.find((s) => s.kind === "vendor" && s.origin === it.repo && s.name === it.name)
          if (have) have.targets = [...new Set([...have.targets, it.agent])]
          else {
            const up = CATALOG[it.repo]?.skills.find((s) => s.name === it.name)
            addSkill({ name: it.name, kind: "vendor", origin: it.repo, category: up?.category ?? "", description: up?.description ?? "", targets: [it.agent] })
          }
          toTrash(it.name, it.path, "adopt")
        } else if (it.proposal === "own") {
          // The name the core chose (app.Proposal.As); a clash on both names is never proposed.
          const name = it.as ?? it.name
          addSkill({ name, kind: "skills", origin: "", category: "", description: "", targets: [it.agent] })
          toTrash(it.name, it.path, "adopt")
        } else if (it.proposal === "enable" && it.sharedCopy) {
          // The agent already reads the library's copy through Agents: this second copy only goes to the trash.
          toTrash(it.name, it.path, "in-the-way")
        } else if (it.proposal === "enable") {
          const s = st.skills.find((x) => x.linkName === it.name)
          if (s) s.targets = [...new Set([...s.targets, it.agent])]
          const a = st.agents.find((x) => x.name === it.agent)
          if (a) a.enabled = true
          // The copy makes way for the link, like any adopted copy.
          if (it.state === "copy") toTrash(it.name, it.path, "adopt")
        } else if (it.proposal === "drop") {
          toTrash(it.name, it.path, "broken")
        }
        st.scan = st.scan.filter((x) => x.id !== id)
        adopted++
      }
      if (adopted) changed()
      return { adopted }
    },

    async trash() {
      await wait(100)
      return clone(st.trash.map((e) => ({ ...e, size: e.size ?? sampleSize(e.id) })))
    },
    async trashUsage() {
      await wait(80)
      return { count: st.trash.length, bytes: st.trash.reduce((n, e) => n + (e.size ?? sampleSize(e.id)), 0) }
    },
    async emptyTrash({ ids }) {
      await wait(400)
      if (ids.length !== st.trash.length || st.trash.some((e) => !ids.includes(e.id))) throw new Error("the trash changed since it was shown; look at it again")
      const deleted = st.trash.length
      st.trash = []
      return { deleted }
    },
    async deleteTrash(id) {
      await wait(200)
      if (!st.trash.some((x) => x.id === id)) throw new Error(`no trash entry ${id}`)
      st.trash = st.trash.filter((x) => x.id !== id)
    },
    async restore(id) {
      await wait(300)
      const e = st.trash.find((x) => x.id === id)
      if (!e) throw new Error(`no trash entry ${id}`)
      if (st.skills.some((s) => s.name === e.name && s.dir === e.origin)) throw new Error(`${e.origin} exists again; move it away first`)
      st.trash = st.trash.filter((x) => x !== e)
      // Like RestoreEntry: what remove or purge took out goes back into the library, selected and linked again.
      const note = notes.get(id)
      notes.delete(id)
      if (note) {
        if (note.pack && !st.packs.some((p) => p.repo === note.pack!.repo)) st.packs.push(note.pack)
        for (const s of note.skills) if (!st.skills.some((x) => x.ref.key === s.ref.key)) st.skills.push(s)
        changed()
        return { skills: note.skills.map((s) => s.name), repo: note.pack?.repo, selected: true, linked: true, absent: absentOf(note.skills) }
      }
      const inLibrary = !!st.root && e.origin.startsWith(st.root)
      return { skills: [e.name], linked: inLibrary, unused: inLibrary && e.origin.includes("/vendor/") }
    },
    async sync({ remote, message }) {
      await wait(1200)
      if (remote) st.remote = remote
      if (!st.remote) throw new Error("no remote: give a GitHub repository URL")
      void message
      if (st.syncMerge) throw new Error("a sync merge is being resolved: finish it or shelve it first")
      const committed = st.changes > 0
      // Like skd sync: commit, then fetch and merge before pushing. Another Mac changed the same lines: the merge is
      // abandoned (the library stays as it was) and nothing is pushed until it is resolved.
      if (st.remoteAhead) return { committed, pushed: false, remote: st.remote, pulled: false, conflicts: SYNC_REMOTE.map((r) => r.path) }
      st.changes = 0
      st.lastSync = new Date().toISOString()
      return { committed, pushed: true, remote: st.remote, pulled: false, conflicts: [] }
    },

    async config() {
      return readConfig()
    },
    async setConfig(patch) {
      const next = { ...readConfig(), ...patch }
      try {
        localStorage.setItem(CONFIG_KEY, JSON.stringify(next))
      } catch {
        // storage unavailable: keep the change for this session only
      }
      return next
    },
  }
}

/** A file's conflicts, parsed from its markers like the bridge's resolveState (base: the ancestor by local side). */
function parseConflicts(path: string, text: string, bases: Record<string, string>): ConflictFile {
  const lines = text.split("\n")
  const blocks = regions(lines).map((r, index): ConflictBlock => {
    const local = lines.slice(r.start + 1, r.mid).join("\n")
    return { index, local, upstream: lines.slice(r.mid + 1, r.end).join("\n"), base: bases[local] }
  })
  return { path, binary: false, blocks, text }
}

const MARKER = /^(<<<<<<<|=======|>>>>>>>)( |$)/m

/** Replaces one conflict of text with one side or both (resolvePick). */
function pickBlock(text: string, block: number, choice: "local" | "upstream" | "both", path: string): string {
  const lines = text.split("\n")
  const r = regions(lines)[block]
  if (!r) throw new Error(`${path} has no conflict ${block + 1}`)
  const local = lines.slice(r.start + 1, r.mid)
  const upstream = lines.slice(r.mid + 1, r.end)
  lines.splice(r.start, r.end - r.start + 1, ...(choice === "local" ? local : choice === "upstream" ? upstream : [...local, ...upstream]))
  return lines.join("\n")
}

/** Where Git's conflict markers are in a file's lines: <<<<<<< start, ======= mid, >>>>>>> end. */
function regions(lines: string[]): { start: number; mid: number; end: number }[] {
  const out: { start: number; mid: number; end: number }[] = []
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].startsWith("<<<<<<<")) continue
    const mid = lines.findIndex((l, k) => k > i && l.startsWith("======="))
    const end = lines.findIndex((l, k) => k > mid && l.startsWith(">>>>>>>"))
    if (mid < 0 || end < 0) break
    out.push({ start: i, mid, end })
    i = end
  }
  return out
}

/** A stable, plausible size for a sample trash entry. */
function sampleSize(id: string): number {
  let h = 0
  for (const c of id) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return 2_000 + (h % 60_000)
}

const withUpstreamDescription = (md: string) => md.replace(/^(description:.*?)\s*$/m, "$1 Also covers follow-up checks.")
const upstreamSection = (title: string) => `\n## What changed upstream\n\n- ${title}\n`

/** The sample's "upstream" version of a skill: a changed description and a new section at the end. */
function upstreamVersion(files: Record<string, string>, title: string): Record<string, string> {
  const out = { ...files }
  out["SKILL.md"] = withUpstreamDescription(out["SKILL.md"] ?? "") + upstreamSection(title)
  out["references/checklist.md"] = "# Checklist\n\n- Read the diff once without commenting.\n- Note every assumption.\n"
  return out
}

function changedPaths(a: Record<string, string>, b: Record<string, string>): string[] {
  return [...new Set([...Object.keys(a), ...Object.keys(b)])].filter((p) => a[p] !== b[p]).sort()
}

function diffFiles(old: Record<string, string>, next: Record<string, string>, local: Set<string>): FileDiff[] {
  return changedPaths(old, next).map((path) => {
    const status: FileDiff["status"] = !(path in old) ? "added" : !(path in next) ? "removed" : "modified"
    const { hunks, added, removed } = lineDiff(lines(old[path]), lines(next[path]))
    return { path, status, local: local.has(path), added, removed, hunks }
  })
}

const lines = (s?: string) => (s ? s.replace(/\n$/, "").split("\n") : [])

/** A small LCS line diff with three lines of context, as app.lineDiff. */
function lineDiff(a: string[], b: string[]): { hunks: DiffHunk[]; added: number; removed: number } {
  const n = a.length
  const m = b.length
  const lcs: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
  const ops: { op: DiffLine["op"]; text: string; o: number; n: number }[] = []
  let i = 0
  let j = 0
  while (i < n || j < m) {
    if (i < n && j < m && a[i] === b[j]) ops.push({ op: " ", text: a[i], o: i++, n: j++ })
    else if (i < n && (j === m || lcs[i + 1][j] >= lcs[i][j + 1])) ops.push({ op: "-", text: a[i], o: i++, n: j })
    else ops.push({ op: "+", text: b[j], o: i, n: j++ })
  }
  const hunks: DiffHunk[] = []
  for (let k = 0; k < ops.length; ) {
    if (ops[k].op === " ") {
      k++
      continue
    }
    const start = Math.max(0, k - 3)
    let end = k
    while (end < ops.length) {
      if (ops[end].op !== " ") {
        end++
        continue
      }
      let run = end
      while (run < ops.length && ops[run].op === " ") run++
      if (run === ops.length || run - end > 6) {
        end = Math.min(ops.length, end + 3)
        break
      }
      end = run
    }
    const part = ops.slice(start, end)
    const oldLines = part.filter((x) => x.op !== "+").length
    const newLines = part.filter((x) => x.op !== "-").length
    hunks.push({ oldStart: ops[start].o + (oldLines ? 1 : 0), oldLines, newStart: ops[start].n + (newLines ? 1 : 0), newLines, lines: part.map(({ op, text }) => ({ op, text })) })
    k = end
  }
  return { hunks, added: ops.filter((x) => x.op === "+").length, removed: ops.filter((x) => x.op === "-").length }
}

function safeJson(s: string): string {
  try {
    return JSON.parse(s)
  } catch {
    return s
  }
}

/** Builds a sorted tree (folders first, SKILL.md on top) from file paths. */
export function tree(paths: string[]): FileNode[] {
  const root: FileNode[] = []
  for (const p of paths) {
    const parts = p.split("/")
    let level = root
    parts.forEach((name, i) => {
      const path = parts.slice(0, i + 1).join("/")
      const dir = i < parts.length - 1
      let node = level.find((n) => n.name === name && n.dir === dir)
      if (!node) {
        node = { name, path, dir, children: dir ? [] : undefined }
        level.push(node)
      }
      if (dir) level = node.children!
    })
  }
  const sort = (nodes: FileNode[]) => {
    nodes.sort((a, b) => {
      if (a.name === "SKILL.md") return -1
      if (b.name === "SKILL.md") return 1
      if (a.dir !== b.dir) return a.dir ? -1 : 1
      return a.name.localeCompare(b.name)
    })
    nodes.forEach((n) => n.children && sort(n.children))
  }
  sort(root)
  return root
}
