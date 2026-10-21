// In-memory Backend used until the Go bindings exist. It behaves like skd on
// the sample library in mock-data.ts; nothing touches the real file system.
import {
  CATALOG,
  CLAUDE_OVERRIDES,
  HOME,
  LIBRARY,
  REMOTE,
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
} from "./mock-data"
import { SHARED_READERS } from "@/lib/agents"
import { parseSource } from "./parse-source"
import type {
  Absent,
  Agent,
  Backend,
  CliStatus,
  Config,
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
  Skill,
  SkillDiff,
  TrashEntry,
} from "./types"

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
}

function sampleState(root: string | null): State {
  const now = new Date()
  const skills = initialSkills()
  const files = initialFiles(skills)
  // One vendored skill the user edited, so the update review can show what moves to the trash.
  const touched = new Set<string>()
  const edited = skills.find((s) => s.kind === "vendor" && s.name === "differential-review")
  if (edited && files[edited.ref.key]?.["SKILL.md"]) {
    files[edited.ref.key]["SKILL.md"] += "\nMy note: also check the migration files.\n"
    touched.add(edited.ref.key)
  }
  // A skill of yours also chosen for Kiro, which this Mac does not have: shown as chosen but not linked.
  skills.find((s) => s.name === "design-review")?.targets.push("kiro")
  // Copies another installer put in agent folders and that were changed since: adopted as your own, not replaced by upstream.
  const differs: ScanItem[] = [
    { id: "claude/tdd", agent: "claude", name: "tdd", path: `${HOME}/.claude/skills/tdd`, state: "copy", source: "mattpocock/skills", proposal: "own", label: "", upstream: "mattpocock/skills", differs: true },
    { id: "cursor/code-review", agent: "cursor", name: "code-review", path: `${HOME}/.cursor/skills/code-review`, state: "copy", source: "mattpocock/skills", proposal: "own", label: "", upstream: "mattpocock/skills", differs: true },
  ]
  return {
    root,
    remote: REMOTE,
    // Not backed up since two days ago: yesterday's removal and update.
    changes: 2,
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
    scan: initialScan([]),
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
          return {
            ...s,
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
      if (!names.size) return { repo, linked: [], renamed: {}, nothing: true }
      if (sel.alias && (sel.alias.includes("/") || sel.alias.includes(":"))) throw new Error(`alias ${sel.alias} may not contain / or :`)
      if (!st.packs.some((p) => p.repo === repo)) {
        st.packs.push({
          repo,
          alias: sel.alias || undefined,
          ref: preview.source.ref,
          commit: preview.commit,
          followAll: sel.all,
          upstreamCount: preview.skills.length,
          license: preview.license,
          url: preview.source.url,
        })
      }
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
      return { repo, linked, renamed, nothing: false, absent: absentOf(st.skills) }
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
      }
      const created = addSkill({ name, kind, origin: "", category: "", description: s.description, targets: [...s.targets] }, files)
      changed()
      return clone(created)
    },
    async newSkill(opts) {
      await wait(400)
      if (!NAME.test(opts.name)) throw new Error(`invalid name ${opts.name}`)
      const kind = opts.private ? "private" : "skills"
      if (st.skills.some((x) => (x.kind === "skills" || x.kind === "private") && x.name === opts.name)) throw new Error(`${kind}/${opts.name} already exists`)
      const description = opts.description.trim() || "What this skill does and when an agent should use it."
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
    async readFile(ref, path) {
      await wait(60)
      const f = st.files[ref.key]?.[path]
      if (f === undefined) throw new Error(`no file ${path}`)
      return f
    },
    async writeFile(ref, path, content) {
      await wait(200)
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
        const backups: string[] = []
        for (const name of s.changed) {
          const sk = st.skills.find((x) => x.origin === repo && x.name === name)
          if (sk && st.touched.has(sk.ref.key)) {
            backups.push(`${name}/SKILL.md`)
            toTrash(name, sk.dir, "update")
            st.touched.delete(sk.ref.key)
            st.files[sk.ref.key] = initialFiles([sk])[sk.ref.key]
          }
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
          backups,
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
        const vendored = initialFiles([sk])[sk.ref.key]
        const mine = st.files[sk.ref.key] ?? vendored
        const upstream = upstreamVersion(vendored, title)
        const localFiles = changedPaths(vendored, mine)
        const local = new Set(localFiles)
        skills.push({ name, status: "changed", local: localFiles.length > 0, localFiles, files: diffFiles(mine, upstream, local) })
      }
      for (const name of s.pending) skills.push({ name, status: "added", local: false, localFiles: [], files: diffFiles({}, { "SKILL.md": `---\nname: ${name}\n---\n` }, new Set()) })
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
          const taken = st.skills.some((s) => s.linkName === it.name)
          const name = taken ? `${it.name}-${it.agent}` : it.name
          addSkill({ name, kind: "skills", origin: "", category: "", description: "", targets: [it.agent] })
          toTrash(it.name, it.path, "adopt")
        } else if (it.proposal === "enable") {
          const s = st.skills.find((x) => x.linkName === it.name)
          if (s) s.targets = [...new Set([...s.targets, it.agent])]
          const a = st.agents.find((x) => x.name === it.agent)
          if (a) a.enabled = true
          // The copy makes way for the link, like any adopted copy.
          if (it.state === "copy") toTrash(it.name, it.path, "adopt")
        } else if (it.proposal === "drop") {
          toTrash(it.name, it.path, "adopt")
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
      const committed = st.changes > 0
      st.changes = 0
      st.lastSync = new Date().toISOString()
      return { committed, pushed: true, remote: st.remote }
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

/** A stable, plausible size for a sample trash entry. */
function sampleSize(id: string): number {
  let h = 0
  for (const c of id) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return 2_000 + (h % 60_000)
}

/** The sample's "upstream" version of a skill: a changed description and a new section. */
function upstreamVersion(files: Record<string, string>, title: string): Record<string, string> {
  const out = { ...files }
  const md = out["SKILL.md"] ?? ""
  out["SKILL.md"] = md.replace(/^(description:.*?)\s*$/m, "$1 Also covers follow-up checks.") + `\n## What changed upstream\n\n- ${title}\n`
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
