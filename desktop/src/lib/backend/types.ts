// The desktop app talks to skilldots only through the Backend interface.
// Each method mirrors an operation of the Go package internal/app (named in
// the comment), so a generated binding (e.g. `mygo generate`) can replace the
// mock implementation without touching the UI.

/** Where a library entry comes from (engine.Kind). */
export type EntryKind = "vendor" | "skills" | "private" | "app"

/** Identifies a skill or a pack (app.Ref). */
export interface Ref {
  /** Qualified key, e.g. "kepano/obsidian-skills:defuddle", "skills:test", "app:surge". */
  key: string
  /** Set when the ref names a whole pack. */
  pack?: string
}

/** Agent skill directory (targets.Target plus what the desktop needs). */
export interface Agent {
  /** Registry name: agents, claude, codex, gemini, grok, … */
  name: string
  /** Human label: "Claude Code". */
  label: string
  /** Absolute directory, e.g. ~/.claude/skills. */
  path: string
  /** The directory (or its parent agent) exists on this machine. */
  detected: boolean
  /** Whether distribution to this agent is active; independent of column visibility. */
  enabled: boolean
  /** Display preference only; does not change distribution. */
  hidden?: boolean
  /** In skills.toml [targets].default: new skills go here. */
  isDefault: boolean
  builtin: boolean
}

/** One resolved library entry (engine.Entry). */
export interface Skill {
  ref: Ref
  name: string
  linkName: string
  kind: EntryKind
  /** Pack repo for vendor entries, app name for apps, else "". */
  origin: string
  category: string
  description: string
  /** Absolute source directory. */
  dir: string
  /** Agents the skill is linked into (resolved targets). */
  targets: string[]
  /** Name clash with another entry; neither is linked. */
  conflict?: string
  /** Source dir absent (not vendored yet). */
  missing?: boolean
}

/** A subscribed repository ([[pack]] in skills.toml plus its lock). */
export interface Pack {
  repo: string
  alias?: string
  ref?: string
  commit: string
  /** Follows every skill ("*"). */
  followAll: boolean
  /** Number of skills upstream had at the last update. */
  upstreamCount: number
  license?: string
  url: string
}

/** Last upstream check of one pack (app.PackStatus). */
export interface PackStatus {
  repo: string
  commit: string
  latest: string
  changed: string[]
  /** "name: old category → new category" */
  moved: string[]
  new: string[]
  gone: string[]
  pending: string[]
  checked: string
  /** Upstream commit titles between commit and latest (newest first). */
  commits: { id: string; title: string }[]
  moreCommits?: boolean
  commitsUnknown?: boolean
}

/** app.Outdated */
export interface Outdated {
  checked: string
  packs: PackStatus[]
  failed: { repo: string; error: string }[]
}

/** One recorded update (history kept in the library, synced with Git). */
export interface HistoryEntry {
  id: string
  when: string
  repo: string
  from: string
  to: string
  changed: string[]
  added: string[]
  gone: string[]
  moved: string[]
  dropped?: string[]
  new?: string[]
  commits: { id: string; title: string }[]
  moreCommits?: boolean
  commitsUnknown?: boolean
  applyError?: string
  omitted?: number
  /** Files the user had changed, moved to the trash before replacing. */
  backups: string[]
  /** Fictional sample shown by the mock. */
  example?: boolean
}

/** What the add box recognised (app.ParseAdd, extended for pasted commands and links). */
export interface ParsedSource {
  repo: string
  url: string
  ref?: string
  /** Skills named by the input (--skill x, /tree/<ref>/<path>, a skills.sh page). */
  skills: string[]
  /** What the input looked like: shorthand | github | tree | blob | npx | skills.sh | git */
  form: "shorthand" | "github" | "tree" | "blob" | "npx" | "skills.sh" | "git"
}

/** One skill offered by a repository (pack.Skill). */
export interface UpstreamSkill {
  name: string
  description: string
  category: string
  license?: string
}

/** app.Fetched: what `skd show` reports. */
export interface RepoPreview {
  source: ParsedSource
  commit: string
  skills: UpstreamSkill[]
  rejected: { name: string; dir: string; reason: string }[]
  subscribed: boolean
  /** Upstream names the library already selects. */
  have: string[]
  license?: string
}

/** app.Selection */
export interface Selection {
  skills: string[]
  categories: string[]
  all: boolean
  alias?: string
  agents: string[]
}

export interface AddOutcome {
  repo: string
  linked: string[]
  /** upstream name → link name assigned because the name was taken */
  renamed: Record<string, string>
  nothing: boolean
}

/** One link change apply would make (engine.Plan item). */
export interface PlanItem {
  action: "link" | "unlink" | "relink" | "blocked"
  skill: string
  agent: string
  path: string
  reason?: string
}

/** scan.Item plus the adoption proposal for it (app.Proposal). */
export interface ScanItem {
  id: string
  agent: string
  name: string
  path: string
  state: "managed" | "unrecorded" | "app" | "link" | "copy" | "broken" | "other" | "left"
  linkTo?: string
  source?: string
  /** pack | app | own | enable | drop | skip */
  proposal: "pack" | "app" | "own" | "enable" | "drop" | "skip"
  label: string
  repo?: string
}

export interface TrashEntry {
  id: string
  name: string
  /** Where it came from (library path or agent dir). */
  origin: string
  path: string
  when: string
  /**
   * Why it is in the trash: "removed" (moved there by the user), "update"
   * (replaced by an update), "adopt" (replaced while adopting), "purge"
   * (a removed source). Optional; the UI falls back to where it came from.
   */
  reason?: "removed" | "update" | "adopt" | "purge"
  example?: boolean
  restorable?: boolean
  problem?: string
}

export interface LibraryStatus {
  /** Absolute library root, or null when there is none yet. */
  root: string | null
  remote?: string
  branch: string
  /** Uncommitted or unpushed changes in the library (private/ excluded). */
  changes: number
  lastSync?: string
  /** Unmanaged skills found on this machine (for onboarding). */
  unmanaged: number
  /** The library follows the skilldots skill (agents know how to use skd). */
  selfSkill?: boolean
  /** initLibrary with the skill: why the skill could not be installed (the library was created). */
  skillError?: string
}

/** The skd command: a link on the PATH to the app's own executable. */
export interface CliStatus {
  /** Where the link goes: /usr/local/bin/skd. */
  path: string
  /** installed: links to this app; stale: to another copy or a moved app; other: another program, never replaced. */
  state: "installed" | "stale" | "other" | "missing"
  target: string
  /** False when this app cannot be linked to: a development build ("dev"), or run from the disk image or moved aside by macOS ("move"). */
  canInstall: boolean
  reason: "" | "dev" | "move"
}

export type Theme = "system" | "light" | "dark"
export type Lang = "zh" | "en"

/** User config (config.toml, shared with the CLI). */
export interface Config {
  editor: "auto" | "code" | "cursor" | "zed" | "custom"
  editorCommand: string
  lang: Lang
  theme: Theme
  checkOnLaunch: boolean
}

export interface FileNode {
  /** Path relative to the skill directory, "/"-separated. */
  path: string
  name: string
  dir: boolean
  children?: FileNode[]
}

export interface SyncOutcome {
  committed: boolean
  pushed: boolean
  remote: string
}

export interface Backend {
  // Library ---------------------------------------------------------------
  /** session.Open + git status. */
  status(): Promise<LibraryStatus>
  /** skd init [dir]; withSkill also installs the skilldots skill for the agents on this machine. */
  initLibrary(dir: string, withSkill?: boolean): Promise<LibraryStatus>
  /** The skilldots skill for every agent on this machine, unless the library has it. */
  installSelfSkill(): Promise<LibraryStatus>
  /** The skd command on the PATH. */
  cliStatus(): Promise<CliStatus>
  /** Links skd (asks for an administrator password when needed). */
  installCli(): Promise<CliStatus>
  removeCli(): Promise<CliStatus>
  /** skd clone <url> [dir] */
  cloneLibrary(url: string, dir: string): Promise<LibraryStatus>
  /** skd use <dir> */
  useLibrary(dir: string): Promise<LibraryStatus>
  /** Native folder picker (window API). */
  pickFolder(): Promise<string | null>

  // Reading ---------------------------------------------------------------
  /** Session.Resolve: every entry. */
  skills(): Promise<Skill[]>
  /** Manifest packs with their lock. */
  packs(): Promise<Pack[]>
  /** skd targets */
  agents(): Promise<Agent[]>
  /** Skills Claude Code turned off in ~/.claude/settings.json skillOverrides (read-only). */
  claudeOverrides(): Promise<string[]>

  // Agents ----------------------------------------------------------------
  /** Suspend distribution while keeping skill selections. */
  setAgentEnabled(name: string, enabled: boolean): Promise<void>
  setAgentHidden(name: string, hidden: boolean): Promise<void>
  /** skills.toml [targets].default */
  setDefaultAgents(names: string[]): Promise<void>

  // Add -------------------------------------------------------------------
  /** Session.ParseAdd */
  parseSource(input: string): Promise<ParsedSource>
  /** Session.Fetch (skd show) */
  show(src: ParsedSource): Promise<RepoPreview>
  /** Session.Add */
  add(preview: RepoPreview, sel: Selection): Promise<AddOutcome>

  // Per skill -------------------------------------------------------------
  /** Session.Enable */
  enable(ref: Ref, agents: string[]): Promise<void>
  /** Session.Disable: unlink only, files stay in the library. */
  disable(ref: Ref, agents: string[]): Promise<void>
  /** Session.SetTargets */
  setTargets(ref: Ref, agents: string[]): Promise<void>
  /** Session.Remove: to the trash. */
  remove(ref: Ref): Promise<void>
  /** Session.Purge: the whole pack to the trash. */
  purge(repo: string): Promise<void>
  /** Session.Fork */
  fork(ref: Ref, opts: { private: boolean; name: string }): Promise<Skill>
  /** Session.New */
  newSkill(opts: { name: string; description: string; private: boolean; agents: string[] }): Promise<Skill>
  /** Finder: reveal the skill directory. */
  reveal(ref: Ref): Promise<void>
  /** Finder: reveal the library root. */
  revealLibrary(): Promise<void>
  /** Open the skill directory in the configured editor. */
  openInEditor(ref: Ref): Promise<void>

  // Files (in-app editor) -------------------------------------------------
  files(ref: Ref): Promise<FileNode[]>
  readFile(ref: Ref, path: string): Promise<string>
  writeFile(ref: Ref, path: string, content: string): Promise<void>

  // Apply -----------------------------------------------------------------
  /** Session.Plan (skd apply -n) */
  plan(): Promise<PlanItem[]>
  /** Session.Apply */
  apply(opts: { force: boolean }): Promise<{ linked: number; unlinked: number; trashed: number }>

  // Updates ---------------------------------------------------------------
  /** Session.RecordedOutdated: last check, no network. */
  recordedOutdated(): Promise<Outdated | null>
  /** Session.Outdated: check only, installs nothing. */
  outdated(repos?: string[]): Promise<Outdated>
  /** Session.Update; addNew adds the listed new upstream skills too. */
  update(repos: string[], opts?: { addNew?: Record<string, string[]> }): Promise<HistoryEntry[]>
  /** Update history kept in the library. */
  history(): Promise<HistoryEntry[]>

  // Scan / adopt ----------------------------------------------------------
  /** Session.Scan + Session.Proposals */
  scan(): Promise<ScanItem[]>
  /** Session.Adopt */
  adopt(ids: string[]): Promise<{ adopted: number }>

  // Trash -----------------------------------------------------------------
  trash(): Promise<TrashEntry[]>
  restore(id: string): Promise<void>

  // Sync ------------------------------------------------------------------
  /** Session.Backup (skd sync); private/ is never pushed. */
  sync(opts: { remote?: string; message?: string }): Promise<SyncOutcome>

  // Config ----------------------------------------------------------------
  config(): Promise<Config>
  setConfig(patch: Partial<Config>): Promise<Config>
}
