import { call } from "mygo-runtime"
import type { Backend } from "./types"

function method<K extends keyof Backend>(name: K) {
  return (...args: Parameters<Backend[K]>): ReturnType<Backend[K]> =>
    call("Service.Invoke", name, args) as ReturnType<Backend[K]>
}

export const nativeBackend: Backend = {
  status: method("status"), initLibrary: method("initLibrary"), installSelfSkill: method("installSelfSkill"),
  cliStatus: method("cliStatus"), installCli: method("installCli"), removeCli: method("removeCli"),
  cloneLibrary: method("cloneLibrary"), useLibrary: method("useLibrary"),
  pickFolder: method("pickFolder"), skills: method("skills"), packs: method("packs"),
  agents: method("agents"), claudeOverrides: method("claudeOverrides"),
  setAgentHidden: method("setAgentHidden"),
  setAgentEnabled: method("setAgentEnabled"), setDefaultAgents: method("setDefaultAgents"),
  parseSource: method("parseSource"), show: method("show"), add: method("add"),
  enable: method("enable"), disable: method("disable"), setTargets: method("setTargets"),
  remove: method("remove"), purge: method("purge"), fork: method("fork"),
  newSkill: method("newSkill"), reveal: method("reveal"), revealLibrary: method("revealLibrary"),
  openInEditor: method("openInEditor"), files: method("files"),
  readFile: method("readFile"), writeFile: method("writeFile"), openUrl: method("openUrl"),
  plan: method("plan"), apply: method("apply"), recordedOutdated: method("recordedOutdated"),
  outdated: method("outdated"), update: method("update"), history: method("history"),
  scan: method("scan"), adopt: method("adopt"), trash: method("trash"), restore: method("restore"),
  trashUsage: method("trashUsage"), emptyTrash: method("emptyTrash"), deleteTrash: method("deleteTrash"),
  updateDiff: method("updateDiff"), historyDiff: method("historyDiff"),
  resolveStart: method("resolveStart"), resolveState: method("resolveState"), resolvePick: method("resolvePick"),
  resolveContinue: method("resolveContinue"), resolveAbort: method("resolveAbort"),
  localChanges: method("localChanges"), contributeOptions: method("contributeOptions"),
  contribute: method("contribute"), refreshContributions: method("refreshContributions"),
  sync: method("sync"), config: method("config"), setConfig: method("setConfig"),
}
