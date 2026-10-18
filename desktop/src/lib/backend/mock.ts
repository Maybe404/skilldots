// In-memory Backend used until the Go bindings exist. It behaves like skd on
// the sample library in mock-data.ts; nothing touches the real file system.
import {
  CATALOG,
  CLAUDE_OVERRIDES,
  LIBRARY,
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
  skillDir,
} from "./mock-data"
import { parseSource } from "./parse-source"
import type {
  Agent,
  Backend,
  Config,
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
  return {
    root,
    remote: "git@github.com:Maybe404/agent-skills.git",
    changes: 2,
    lastSync: new Date(now.getTime() - 3 * 3600 * 1000).toISOString(),
    skills,
    packs: initialPacks(),
    agents: initialAgents(),
    files: initialFiles(skills),
    outdated: initialOutdated(now),
    history: initialHistory(now),
    trash: initialTrash(now),
    plan: initialPlan(),
    scan: initialScan(),
    touched: new Set(),
  }
}

function emptyState(root: string): State {
  const s = sampleState(root)
  const keep = s.skills.filter((x) => x.name === "skilldots")
  return {
    ...s,
    remote: undefined,
    changes: 1,
    lastSync: undefined,
    skills: keep,
    packs: s.packs.filter((p) => p.repo === "Maybe404/skilldots"),
    files: initialFiles(keep),
    outdated: null,
    history: [],
    trash: [],
    plan: [],
  }
}

export function createMockBackend(): Backend {
  const fresh = typeof location !== "undefined" && new URLSearchParams(location.search).has("fresh")
  let st: State = fresh ? { ...sampleState(null), root: null } : sampleState(LIBRARY)

  const find = (ref: Ref): Skill => {
    const s = st.skills.find((x) => x.ref.key === ref.key)
    if (!s) throw new Error(`no skill ${ref.key}`)
    return s
  }
  const changed = () => {
    st.changes += 1
  }
  const toTrash = (name: string, origin: string, reason?: TrashEntry["reason"]) => {
    const batch = new Date().toISOString().replace(/[-:]/g, "").replace("T", "-").slice(0, 15)
    st.trash.unshift({
      id: `${batch}/${name}`,
      name,
      origin,
      path: `${st.root}/.trash/${batch}/${name}`,
      when: new Date().toISOString(),
      reason,
    })
  }
  const status = (): LibraryStatus => ({
    root: st.root,
    remote: st.remote,
    branch: "main",
    changes: st.changes,
    lastSync: st.lastSync,
    unmanaged: 47,
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

  return {
    async status() {
      await wait(120)
      return status()
    },
    async initLibrary(dir) {
      await wait(500)
      st = emptyState(dir || `${"/Users/maybe"}/.skilldots`)
      return status()
    },
    async cloneLibrary(url, dir) {
      await wait(1200)
      if (!/github\.com|^git@|\//.test(url)) throw new Error(`cannot clone ${url}`)
      st = sampleState(dir || "/Users/maybe/.skilldots")
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
      return clone(st.skills)
    },
    async packs() {
      await wait(60)
      return clone(st.packs)
    },
    async agents() {
      await wait(60)
      return clone(st.agents)
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
      return clone({ ...cat, source: src, subscribed: !!pack, have })
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
      return { repo, linked, renamed, nothing: false }
    },

    async enable(ref, agents) {
      await wait(150)
      const s = find(ref)
      s.targets = [...new Set([...s.targets, ...agents])]
      changed()
    },
    async disable(ref, agents) {
      await wait(150)
      const s = find(ref)
      s.targets = agents.length ? s.targets.filter((t) => !agents.includes(t)) : []
      changed()
    },
    async setTargets(ref, agents) {
      await wait(150)
      find(ref).targets = [...agents]
      changed()
    },
    async remove(ref) {
      await wait(300)
      const s = find(ref)
      if (s.kind === "app") throw new Error(`${s.name} belongs to an application; disable it instead`)
      st.skills = st.skills.filter((x) => x !== s)
      toTrash(s.name, s.dir, "removed")
      if (!st.skills.some((x) => x.origin === s.origin && x.kind === "vendor")) st.packs = st.packs.filter((p) => p.repo !== s.origin)
      changed()
    },
    async purge(repo) {
      await wait(400)
      if (!st.packs.some((p) => p.repo === repo)) throw new Error(`not subscribed: ${repo}`)
      st.skills = st.skills.filter((x) => !(x.kind === "vendor" && x.origin === repo))
      st.packs = st.packs.filter((p) => p.repo !== repo)
      if (st.outdated) st.outdated.packs = st.outdated.packs.filter((p) => p.repo !== repo)
      toTrash(repo.split("/")[1], `${st.root}/vendor/${repo}`, "purge")
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
      return { linked, unlinked, trashed }
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
    async history() {
      await wait(100)
      return clone(st.history)
    },

    async scan() {
      await wait(900)
      return clone(st.scan)
    },
    async adopt(ids) {
      await wait(900)
      let adopted = 0
      for (const id of ids) {
        const it = st.scan.find((x) => x.id === id)
        if (!it || it.proposal === "skip") continue
        if (it.proposal === "pack" && it.repo) {
          if (!st.packs.some((p) => p.repo === it.repo)) {
            const cat = CATALOG[it.repo]
            st.packs.push({ repo: it.repo, commit: cat?.commit ?? "0000000", followAll: false, upstreamCount: cat?.skills.length ?? 1, license: cat?.license, url: `https://github.com/${it.repo}` })
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
        }
        st.scan = st.scan.filter((x) => x.id !== id)
        adopted++
      }
      if (adopted) changed()
      return { adopted }
    },

    async trash() {
      await wait(100)
      return clone(st.trash)
    },
    async restore(id) {
      await wait(300)
      const e = st.trash.find((x) => x.id === id)
      if (!e) throw new Error(`no trash entry ${id}`)
      if (st.skills.some((s) => s.name === e.name && s.dir === e.origin)) throw new Error(`${e.origin} exists again; move it away first`)
      st.trash = st.trash.filter((x) => x !== e)
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
