import type { Lang } from "@/lib/backend"
import { en, zh, type MessageKey } from "./messages"

export type { MessageKey }
export type Vars = Record<string, string | number>
export type T = (key: MessageKey, vars?: Vars) => string

const tables: Record<Lang, Record<MessageKey, string>> = { zh, en }

export function translator(lang: Lang): T {
  const table = tables[lang]
  return (key, vars) => {
    let s = table[key] ?? zh[key] ?? key
    if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v))
    return s
  }
}

/** "2 分钟前" / "2 min ago" */
export function ago(iso: string | undefined, t: T, now = Date.now()): string {
  if (!iso) return ""
  const s = Math.max(0, (now - new Date(iso).getTime()) / 1000)
  if (s < 60) return t("justNow")
  if (s < 3600) return t("minutesAgo", { n: Math.floor(s / 60) })
  if (s < 86400) return t("hoursAgo", { n: Math.floor(s / 3600) })
  return t("daysAgo", { n: Math.floor(s / 86400) })
}

export function formatDate(iso: string, lang: Lang): string {
  return new Intl.DateTimeFormat(lang === "zh" ? "zh-CN" : "en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(iso))
}
