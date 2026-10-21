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
  /** Also loads skills from ~/.agents/skills: "*" does not link into it separately when Agents is a target. */
  readsShared?: boolean
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
  /** Agents it names that are not installed on this machine: nothing is linked there. */
  absent?: string[]
  /** Agents among its targets that read it from ~/.agents/skills, so get no link of their own (engine.Entry.ViaShared). */
  viaShared?: string[]
  /** Installed agents that read it from ~/.agents/skills at all, chosen or not: turning it on for them changes nothing. */
  sharedReach?: string[]
  /**
   * An update is shelved: upstream changed what you edited as well and the merge has conflicts, so the folder was
   * left as it was (lock Shelved). The bridge always sends it; optional for the sample data and older bridges.
   */
  shelved?: boolean
  /** Being resolved (`skd resolve` started): its files hold conflict markers until the merge is finished or shelved again. */
  resolving?: boolean
  /** The files with conflicts, relative to the skill folder. */
  conflictFiles?: string[]
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
  /** Skills whose edits were merged with upstream's changes (app.UpdateRecord.Merged). */
  merged?: string[]
  /**
   * Skills left as they were because the merge had conflicts. An update's own result names the files
   * ({name, files}); a history record may name only the skill. shelvedNames() reads either.
   */
  shelved?: (ShelvedSkill | string)[]
  /** Fictional sample shown by the mock. */
  example?: boolean
}

/** A skill an update shelved, with its conflicted files (engine.ShelvedSkill). */
export interface ShelvedSkill {
  name: string
  files: string[]
}

/** The skill names of a record's shelved list, whichever form it has. */
export function shelvedNames(h: Pick<HistoryEntry, "shelved">): string[] {
  return (h.shelved ?? []).map((s) => (typeof s === "string" ? s : s.name))
}

/** One conflict in a file: the two sides between the markers (and the common ancestor, when known). */
export interface ConflictBlock {
  /** 0-based position among the file's conflicts; what resolvePick names. */
  index: number
  local: string
  upstream: string
  base?: string
}

/** A file of a skill being resolved, parsed from the conflict markers in the work tree. */
export interface ConflictFile {
  /** Relative to the skill folder. */
  path: string
  /** Changed on both sides and not text: nothing to pick block by block. */
  binary: boolean
  /** The conflicts still in the file; empty once every one is resolved. */
  blocks: ConflictBlock[]
  /** The whole file as it is now, markers included. */
  text: string
}

export interface ResolveState {
  files: ConflictFile[]
}

export type ResolveChoice = "local" | "upstream" | "both"

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
  /**
   * What the input named, as upstream skill names (matched by name,
   * category/name, folder or link path). Both backends set this and
   * `missing`; optional only so the sample catalogue can omit them.
   */
  preselected?: string[]
  /** Names or paths the input asked for that the repository does not have. */
  missing?: string[]
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
  absent?: Absent
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
  /** pack | app | own | enable | drop | skip, or a kind the core added since: the UI then shows label. */
  proposal: "pack" | "app" | "own" | "enable" | "drop" | "skip" | (string & {})
  /** An "enable" for a copy the agent does not need: it reads the library's from ~/.agents/skills, so adopting moves this copy to the trash. */
  sharedCopy?: boolean
  /** An "own" copy's name in skills/: its SKILL.md name, or its folder name when that one is taken. */
  as?: string
  /** The library skill an "enable" installs; another name than the copy's when it was found by its content. */
  inLibrary?: string
  /** The repository it came from (as recorded by its installer, or its link). */
  upstream?: string
  /** Its content differs from what upstream has now: proposed as your own skill ("own"). */
  differs?: boolean
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
  reason?: "removed" | "update" | "merged" | "adopt" | "purge" | "remove" | "deselected" | "replaced" | "in-the-way" | "broken" | ""
  /** The skills it holds and the source they came from, as recorded when it was set aside. */
  skills?: string[]
  repo?: string
  example?: boolean
  restorable?: boolean
  problem?: string
  /** Bytes on disk (links counted as themselves). */
  size?: number
  /** The name is not one skd gives what it sets aside: shown as data only. */
  unsafe?: boolean
}

/** One line of a diff hunk: " " context, "-" removed, "+" added. */
export interface DiffLine {
  op: " " | "-" | "+"
  text: string
}

export interface DiffHunk {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  lines: DiffLine[]
}

/** One file of a skill that an update would change (app.FileDiff). */
export interface FileDiff {
  /** Relative to the skill folder. */
  path: string
  status: "added" | "removed" | "modified"
  /** You changed this file in the library's copy. */
  local?: boolean
  binary?: boolean
  tooLarge?: boolean
  added: number
  removed: number
  hunks: DiffHunk[]
}

/** One skill an update would replace or bring in (app.SkillDiff). */
export interface SkillDiff {
  name: string
  status: "changed" | "added" | "moved"
  /** The library's copy has your edits: the whole folder goes to the trash before the upstream version is copied in. */
  local: boolean
  /** Files you changed, when known. */
  localFiles: string[]
  /** Edited, but which files is not known. */
  localUnknown?: boolean
  files: FileDiff[]
  moreFiles?: number
}

/** app.UpdateDiff: what updating one source would change, before it runs. */
export interface UpdateDiff {
  repo: string
  commit: string
  latest: string
  skills: SkillDiff[]
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
  /** The skilldots skill would come from this app's own copy (the packaged app); otherwise from GitHub, which needs the network. */
  bundledSkill?: boolean
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

/** Agent not installed on this machine → the skills an apply did not link there. */
export type Absent = Record<string, string[]>

/** What a change of links reports besides success. */
export interface ApplyNote {
  absent?: Absent
}

/** app.RestoreOutcome */
export interface RestoreOutcome {
  skills?: string[]
  repo?: string
  /** Selected again in skills.toml (what remove or purge took out is back). */
  selected?: boolean
  /** Linked again (it went back into the library, not into an agent's folder). */
  linked?: boolean
  /** Back in vendor/ but nothing selects it. */
  unused?: boolean
  hint?: string
  absent?: Absent
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
  enable(ref: Ref, agents: string[]): Promise<ApplyNote | void>
  /** Session.Disable: unlink only, files stay in the library. */
  disable(ref: Ref, agents: string[]): Promise<ApplyNote | void>
  /** Session.SetTargets */
  setTargets(ref: Ref, agents: string[]): Promise<ApplyNote | void>
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
  apply(opts: { force: boolean }): Promise<{ linked: number; unlinked: number; trashed: number; absent?: Absent }>

  // Updates ---------------------------------------------------------------
  /** Session.RecordedOutdated: last check, no network. */
  recordedOutdated(): Promise<Outdated | null>
  /** Session.Outdated: check only, installs nothing. */
  outdated(repos?: string[]): Promise<Outdated>
  /** Session.Update; addNew adds the listed new upstream skills too. */
  update(repos: string[], opts?: { addNew?: Record<string, string[]> }): Promise<HistoryEntry[]>
  /** Session.UpdateDiff: file by file, what updating repo would change; nothing is written. latest: the commit last seen (no fetch when the cache is at it). */
  updateDiff(repo: string, latest?: string): Promise<UpdateDiff>
  /** Update history kept in the library. */
  history(): Promise<HistoryEntry[]>
  /** app.HistoryDiff: what one recorded update changed, skill by skill (time is the record's `when`). */
  historyDiff(repo: string, time: string): Promise<{ skills: SkillDiff[] }>

  // Resolving a shelved update (skd resolve) -------------------------------
  /** Writes the merge with conflict markers into the skill's folder (the folder as it was goes to the trash first). */
  resolveStart(ref: Ref): Promise<ResolveState>
  /** The conflicts left, read from the files as they are now (manual edits included). */
  resolveState(ref: Ref): Promise<ResolveState>
  /** Replaces one conflict of a file with one side, or both one after the other. */
  resolvePick(ref: Ref, path: string, block: number, choice: ResolveChoice): Promise<void>
  /** skd resolve --continue: refused while a file still has conflict markers; then the skill is up to date and linked again. */
  resolveContinue(ref: Ref): Promise<ApplyNote | void>
  /** skd resolve --abort: the folder comes back as it was, and the update is shelved again. */
  resolveAbort(ref: Ref): Promise<void>

  // Scan / adopt ----------------------------------------------------------
  /** Session.Scan + Session.Proposals */
  scan(): Promise<ScanItem[]>
  /** Session.Adopt */
  /** Session.Adopt; each pick carries the proposal kind the person saw, and a row that changed since is refused. */
  adopt(picks: { id: string; kind: ScanItem["proposal"] }[]): Promise<{ adopted: number }>

  // Trash -----------------------------------------------------------------
  trash(): Promise<TrashEntry[]>
  /** Session.RestoreEntry: what remove or purge took out comes back into skills.toml and is linked again. */
  restore(id: string): Promise<RestoreOutcome>
  /** Entries and bytes the trash holds (what emptying it deletes). */
  trashUsage(): Promise<{ count: number; bytes: number }>
  /** Permanently deletes everything; count is what the person confirmed and must still match. */
  emptyTrash(opts: { ids: string[] }): Promise<{ deleted: number }>
  /** Permanently deletes one entry. */
  deleteTrash(id: string): Promise<void>

  // Sync ------------------------------------------------------------------
  /** Session.Backup (skd sync); private/ is never pushed. */
  sync(opts: { remote?: string; message?: string }): Promise<SyncOutcome>

  // Config ----------------------------------------------------------------
  config(): Promise<Config>
  setConfig(patch: Partial<Config>): Promise<Config>
}
