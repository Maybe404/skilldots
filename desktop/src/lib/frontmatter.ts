// Minimal SKILL.md frontmatter handling: name and description are edited in
// a form, every other line of the frontmatter is kept verbatim.

export interface SkillDoc {
  hasFrontmatter: boolean
  name: string
  description: string
  /** Frontmatter lines other than name/description, in order. */
  rest: string[]
  body: string
}

const FM = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/

export function parseSkillMd(text: string): SkillDoc {
  const m = text.match(FM)
  if (!m) return { hasFrontmatter: false, name: "", description: "", rest: [], body: text }
  const lines = m[1].split(/\r?\n/)
  let name = ""
  let description = ""
  const rest: string[] = []
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const kv = line.match(/^(name|description):\s*(.*)$/)
    if (!kv) {
      rest.push(line)
      continue
    }
    let value = kv[2]
    if (value === ">" || value === "|" || value === ">-" || value === "|-") {
      const block: string[] = []
      while (i + 1 < lines.length && /^\s+/.test(lines[i + 1])) block.push(lines[++i].trim())
      value = block.join(value.startsWith(">") ? " " : "\n")
    } else value = unquote(value)
    if (kv[1] === "name") name = value
    else description = value
  }
  return { hasFrontmatter: true, name, description, rest, body: text.slice(m[0].length).replace(/^\r?\n/, "") }
}

export function serializeSkillMd(doc: SkillDoc): string {
  if (!doc.hasFrontmatter) return doc.body
  const head = [`name: ${yamlScalar(doc.name)}`, `description: ${yamlScalar(doc.description)}`, ...doc.rest]
  return `---\n${head.join("\n")}\n---\n\n${doc.body}`
}

function unquote(v: string): string {
  const s = v.trim()
  if (s.startsWith('"') && s.endsWith('"')) {
    try {
      return JSON.parse(s)
    } catch {
      return s.slice(1, -1)
    }
  }
  if (s.startsWith("'") && s.endsWith("'")) return s.slice(1, -1).replace(/''/g, "'")
  return s
}

/** Plain when safe, otherwise a double-quoted (JSON-compatible) YAML string. */
function yamlScalar(v: string): string {
  if (v === "") return '""'
  if (/^[\s'"&*!|>%@`#[\]{},?:-]|: |\s#|\n|\s$/.test(v)) return JSON.stringify(v)
  return v
}
