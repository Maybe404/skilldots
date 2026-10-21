export { cn } from "cn"

/**
 * A Git commit as shown in the interface: the first 7 hex digits, like `git
 * log --oneline`. The backend returns full 40-digit SHAs; show the full id
 * in a `title`. Anything that is not a long hex id (empty, "HEAD") is kept.
 */
export function shortSha(id: string): string {
  return /^[0-9a-f]{8,}$/i.test(id) ? id.slice(0, 7) : id
}

/** Opens a web page (a PR, a commit) in the browser, outside the app; only https links. */
export function openExternal(url: string) {
  if (!/^https:\/\//.test(url)) return
  window.open(url, "_blank", "noopener,noreferrer")
}
