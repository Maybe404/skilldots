import type { ParsedSource } from "./types"

const SHORT = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/

/**
 * Recognises what people paste into the add box: owner/repo, GitHub links
 * (including /tree/<ref>/<path> and /blob/<ref>/…/SKILL.md), a whole
 * `npx skills add … --skill x` line (bunx, pnpm dlx, add-skill variants) and
 * skills.sh pages. It never runs anything. Throws on input it cannot read.
 */
export function parseSource(raw: string): ParsedSource {
  const input = raw.trim().replace(/^\$\s*/, "")
  if (!input) throw new Error("empty")

  const cmd = parseCommand(input)
  if (cmd) {
    const inner = parseSource(cmd.target)
    return { ...inner, skills: unique([...inner.skills, ...cmd.skills]), form: "npx" }
  }

  // skills.sh/<owner>/<repo>[/<skill>]
  const ss = input.match(/^(?:https?:\/\/)?(?:www\.)?skills\.sh\/([^/?#\s]+)\/([^/?#\s]+)(?:\/([^/?#\s]+))?/)
  if (ss) {
    const repo = `${ss[1]}/${ss[2]}`
    return { repo, url: gh(repo), skills: ss[3] ? [ss[3]] : [], form: "skills.sh" }
  }

  const gm = input.match(/^(?:https?:\/\/)?(?:www\.)?github\.com\/([^/\s]+)\/([^/?#\s]+)(\/.*)?$/)
  if (gm) {
    const repo = `${gm[1]}/${gm[2].replace(/\.git$/, "")}`
    const rest = (gm[3] ?? "").replace(/[?#].*$/, "").replace(/\/$/, "")
    const tree = rest.match(/^\/(tree|blob)\/([^/]+)(?:\/(.*))?$/)
    if (tree) {
      const [, kind, ref, path = ""] = tree
      const parts = path.split("/").filter(Boolean)
      if (kind === "blob") {
        if (parts.at(-1)?.toUpperCase() === "SKILL.MD") parts.pop()
        else parts.pop() // a file inside the skill
      }
      const skill = parts.length ? parts.at(-1)! : undefined
      return {
        repo,
        url: gh(repo),
        ref: ref === "main" || ref === "master" ? undefined : ref,
        skills: skill && !STRUCTURAL.has(skill) ? [skill] : [],
        form: kind === "blob" ? "blob" : "tree",
      }
    }
    return { repo, url: gh(repo), skills: [], form: "github" }
  }

  const ssh = input.match(/^git@github\.com:([^/\s]+)\/([^\s]+?)(?:\.git)?$/)
  if (ssh) return { repo: `${ssh[1]}/${ssh[2]}`, url: input, skills: [], form: "git" }

  let s = input.replace(/^github:/, "")
  let ref: string | undefined
  const at = s.lastIndexOf("@")
  if (at > 0 && !s.slice(at).includes("/")) {
    ref = s.slice(at + 1)
    s = s.slice(0, at)
  }
  if (SHORT.test(s)) return { repo: s.replace(/\.git$/, ""), url: gh(s), ref, skills: [], form: "shorthand" }

  const git = s.match(/^(?:https?|ssh|git):\/\/([^/\s]+)\/(.+?)(?:\.git)?$/)
  if (git) return { repo: `${git[1]}/${git[2]}`, url: s, ref, skills: [], form: "git" }

  throw new Error("unrecognised")
}

const STRUCTURAL = new Set(["skills", "skill", "plugins", "plugin", ".agents", ".claude"])

function gh(repo: string) {
  return `https://github.com/${repo.replace(/\.git$/, "")}`
}

function unique(xs: string[]) {
  return [...new Set(xs.filter(Boolean))]
}

/** `npx skills add <target> --skill a,b -s c`, `bunx …`, `pnpm dlx …`, `npx add-skill <target>`. */
function parseCommand(input: string): { target: string; skills: string[] } | null {
  const words = input.split(/\s+/)
  let i = 0
  const runner = words[i]
  if (runner === "npx" || runner === "bunx") i++
  else if ((runner === "pnpm" || runner === "yarn") && words[1] === "dlx") i += 2
  else if (runner === "pnpx") i++
  else return null
  while (words[i]?.startsWith("-")) i++ // npx -y
  const tool = words[i]?.replace(/@[^/]*$/, "")
  if (tool === "skills") {
    i++
    if (words[i] !== "add") return null
    i++
  } else if (tool === "add-skill") {
    i++
  } else return null

  let target = ""
  const skills: string[] = []
  for (; i < words.length; i++) {
    const w = words[i]
    if (w === "--skill" || w === "-s") {
      skills.push(...(words[++i] ?? "").split(","))
    } else if (w.startsWith("--skill=")) {
      skills.push(...w.slice(8).split(","))
    } else if (w.startsWith("-")) {
      // flags such as -g, -y, --agent claude: skip a value that follows a long flag
      if ((w === "--agent" || w === "-a") && words[i + 1]) i++
    } else if (!target) target = w
  }
  if (!target) return null
  return { target, skills: unique(skills.map((x) => x.replace(/^['"]|['"]$/g, ""))) }
}
