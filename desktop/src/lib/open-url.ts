import { backend } from "@/lib/backend"

/** Opens a web page (a PR, a commit) in the browser, outside the app; only https links. */
export function openExternal(url: string) {
  if (!/^https:\/\//.test(url)) return
  void backend.openUrl(url).catch(() => undefined)
}
