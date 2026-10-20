import * as React from "react"
import { toast } from "sonner"
import { ArrowRight, Search } from "lucide-react"

import { absentFor, toastAbsent, useApp } from "@/lib/app-state"
import { backend, type ParsedSource, type RepoPreview, type UpstreamSkill } from "@/lib/backend"
import type { MessageKey } from "@/lib/i18n"
import { Mono, RefreshIcon, Sha, Tag } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { AgentTargets } from "./skill-dialogs"

const EXAMPLES = [
  "mattpocock/skills",
  "npx skills add trailofbits/skills --skill semgrep-rule-creator",
  "https://github.com/anthropics/skills/tree/main/skills/pdf",
  "https://skills.sh/vercel-labs/agent-skills/web-design-guidelines",
]

const FORM_LABEL: Record<ParsedSource["form"], MessageKey> = {
  shorthand: "formShorthand",
  github: "formGithub",
  tree: "formTree",
  blob: "formBlob",
  npx: "formNpx",
  "skills.sh": "formSkillsSh",
  git: "formGit",
}

/** With more skills than this, the list gets a search box. */
const SEARCH_FROM = 8

/** A message with {placeholders} filled by nodes (commit ids in their own face). */
function fill(text: string, parts: Record<string, React.ReactNode>): React.ReactNode {
  return text.split(/\{(\w+)\}/).map((s, i) => (i % 2 ? <React.Fragment key={i}>{parts[s]}</React.Fragment> : s))
}

/**
 * Add skills from a repository, or (adjust, opened from a source) change
 * which of its skills the library uses: ticking adds, unticking an installed
 * one removes it (unlinked everywhere, its files moved to the trash).
 */
export function AddDialog({ initial, adjust, onClose }: { initial?: string; adjust?: boolean; onClose: () => void }) {
  const { t, runTask, reload, agents: allAgents, openDialog, packs, skills } = useApp()
  const [input, setInput] = React.useState(initial ?? "")
  const [parsed, setParsed] = React.useState<ParsedSource | null>(null)
  const [parseError, setParseError] = React.useState(false)
  const [preview, setPreview] = React.useState<RepoPreview | null>(null)
  const [loading, setLoading] = React.useState(false)
  const [fetchError, setFetchError] = React.useState<string | null>(null)
  const [picked, setPicked] = React.useState<Set<string>>(new Set())
  /** Installed skills unticked (adjust): removed on save. */
  const [dropped, setDropped] = React.useState<Set<string>>(new Set())
  const [followAll, setFollowAll] = React.useState(false)
  const [targets, setTargets] = React.useState<Set<string>>(() => new Set(allAgents.filter((a) => a.isDefault && a.enabled).map((a) => a.name)))
  const [alias, setAlias] = React.useState("")
  const [query, setQuery] = React.useState("")
  const [adding, setAdding] = React.useState(false)
  const pack = adjust ? packs.find((p) => p.repo === initial) : undefined

  // Recognise as the user types; nothing is fetched yet.
  React.useEffect(() => {
    const v = input.trim()
    setPreview(null)
    setFetchError(null)
    if (!v) {
      setParsed(null)
      setParseError(false)
      return
    }
    let live = true
    backend.parseSource(v).then(
      (p) => live && (setParsed(p), setParseError(false)),
      () => live && (setParsed(null), setParseError(true)),
    )
    return () => {
      live = false
    }
  }, [input])

  const read = React.useCallback(async (src: ParsedSource) => {
    setLoading(true)
    setFetchError(null)
    try {
      const p = await backend.show(src)
      setPreview(p)
      // The backend matched what the input named (by name, category/name or folder); older ones did not say.
      const named = p.preselected ?? src.skills.filter((n) => p.skills.some((s) => s.name === n))
      setPicked(new Set(named.filter((n) => !p.have.includes(n))))
      setDropped(new Set())
      setFollowAll(false)
      setQuery("")
    } catch (e) {
      setFetchError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [])

  // Opened from a source: read it straight away.
  const auto = React.useRef(!!initial)
  React.useEffect(() => {
    if (auto.current && parsed) {
      auto.current = false
      void read(parsed)
    }
  }, [parsed, read])

  // Names the pasted line asked for that the repository does not have (any more): said, not silently dropped.
  const missing = preview ? (preview.missing ?? (parsed ? parsed.skills.filter((n) => !preview.skills.some((s) => s.name === n)) : [])) : []

  const q = query.trim().toLowerCase()
  const shownSkills = React.useMemo(
    () => (preview?.skills ?? []).filter((s) => !q || [s.name, s.description, s.category].some((x) => x.toLowerCase().includes(q))),
    [preview, q],
  )
  const groups = React.useMemo(() => {
    const m = new Map<string, UpstreamSkill[]>()
    for (const s of shownSkills) m.set(s.category, [...(m.get(s.category) ?? []), s])
    return [...m.entries()].sort(([a], [b]) => (a === "" ? 1 : b === "" ? -1 : a.localeCompare(b)))
  }, [shownSkills])

  const have = new Set(preview?.have ?? [])
  const adds = followAll ? (preview?.skills.filter((s) => !have.has(s.name)).length ?? 0) : picked.size
  const removes = followAll ? 0 : dropped.size
  // Ticked = installed and kept, or newly picked.
  const isTicked = (name: string) => followAll || (have.has(name) ? !dropped.has(name) : picked.has(name))
  const toggle = (name: string, on: boolean) => {
    const set = have.has(name) ? setDropped : setPicked
    const add = have.has(name) ? !on : on
    set((p) => {
      const n = new Set(p)
      if (add) n.add(name)
      else n.delete(name)
      return n
    })
  }

  const submit = async () => {
    if (!preview) return
    const repo = preview.source.repo
    const gone = followAll ? [] : [...dropped]
    setAdding(true)
    const r = await runTask(t("operating"), async () => {
      const added = adds > 0 ? await backend.add(preview, { skills: [...picked], categories: [], all: followAll, alias: alias.trim() || undefined, agents: [...targets] }) : null
      // Removing goes through the library's own remove: links first, then the files to the trash.
      for (const name of gone) {
        const s = skills.find((x) => x.kind === "vendor" && x.origin === repo && x.name === name)
        if (s) await backend.remove(s.ref)
      }
      return added
    })
    setAdding(false)
    if (!r.ok) return
    await reload()
    if (adjust) toast(t("adjustedToast", { repo, add: r.value?.linked.length ?? 0, remove: gone.length }))
    else toast(t("addedToast", { n: r.value?.linked.length ?? 0 }))
    for (const [from, to] of Object.entries(r.value?.renamed ?? {})) toast(t("renamedToast", { from, to }))
    toastAbsent(t, allAgents, absentFor(r.value?.absent, r.value?.linked ?? []))
    onClose()
  }

  const action = !adjust
    ? t("addN", { n: adds })
    : adds && removes
      ? t("adjustAddRemove", { add: adds, remove: removes })
      : adds
        ? t("adjustAdd", { n: adds })
        : removes
          ? t("adjustRemove", { n: removes })
          : t("adjustNothing")
  const versions =
    pack && preview
      ? pack.commit && pack.commit !== preview.commit
        ? fill(t("versionBoth"), { yours: <Sha id={pack.commit} />, latest: <Sha id={preview.commit} /> })
        : fill(t("versionSame"), { yours: <Sha id={preview.commit} /> })
      : null

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="flex max-h-[min(46rem,calc(100svh-4rem))] w-[680px] flex-col gap-0 p-0">
        <DialogHeader className="px-6 pt-[22px] pb-4">
          <DialogTitle>{adjust ? t("adjustTitle", { repo: initial ?? "" }) : t("addTitle")}</DialogTitle>
          <DialogDescription>{adjust ? t("adjustHint") : t("addInputHint")}</DialogDescription>
        </DialogHeader>

        {!adjust && (
          <form
            className="px-6 pb-4"
            onSubmit={(e) => {
              e.preventDefault()
              if (parsed) void read(parsed)
            }}
          >
            <Field>
              <FieldLabel htmlFor="add-input">{t("addInputLabel")}</FieldLabel>
              <div className="flex gap-2">
                <Input
                  id="add-input"
                  autoFocus
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  placeholder={t("addInputPlaceholder")}
                  className="fs-body"
                  spellCheck={false}
                  autoComplete="off"
                  aria-invalid={parseError || undefined}
                  aria-describedby="add-recognized"
                />
                <Button type="submit" variant="outline" size="lg" disabled={!parsed || loading}>
                  {loading ? <RefreshIcon spinning /> : <ArrowRight />}
                  {t("readRepo")}
                </Button>
              </div>
              <div id="add-recognized" aria-live="polite" className="min-h-5 fs-meta">
                {parsed && (
                  <p className="text-mut">
                    {t("recognized")} {t(FORM_LABEL[parsed.form])} · <span className="text-ink">{parsed.repo}</span>
                    {parsed.ref && (
                      <>
                        {" "}
                        @<Mono>{parsed.ref}</Mono>
                      </>
                    )}
                    {parsed.skills.length > 0 && <> · {t("preselected", { names: parsed.skills.join(", ") })}</>}
                  </p>
                )}
                {parseError && <p className="text-ink">{t("unrecognized")}</p>}
              </div>
            </Field>
            {!input && (
              <div className="mt-2">
                <p className="mb-1.5 fs-meta text-mut">{t("tryThese")}</p>
                <div className="flex flex-wrap gap-1.5">
                  {EXAMPLES.map((ex) => (
                    <Button key={ex} type="button" variant="secondary" size="xs" className="max-w-full font-normal" onClick={() => setInput(ex)}>
                      <span className="truncate">{ex}</span>
                    </Button>
                  ))}
                </div>
              </div>
            )}
          </form>
        )}

        <div className="min-h-0 flex-1 overflow-y-auto [border-top:var(--tblRow)]">
          {loading && (
            <p className="flex items-center gap-2 px-6 py-6 fs-body text-mut" aria-live="polite">
              <RefreshIcon spinning /> {t("reading")}
            </p>
          )}
          {fetchError && (
            <div className="px-6 py-5 fs-body" role="alert">
              <p className="font-heading fs-h3 text-ink">{t("readFailed")}</p>
              <p className="mt-1 text-mut">{fetchError}</p>
            </div>
          )}
          {preview && !loading && (
            <div className="px-6 py-4">
              <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 fs-meta text-mut">
                <span className="font-medium text-ink">{t("pickSkills")}</span>
                {versions ? <span>{versions}</span> : <Sha id={preview.commit} />}
                <span>{preview.license ? t("licenseLabel", { license: preview.license }) : t("noLicense")}</span>
                {preview.subscribed && !adjust && <span>{t("alreadySubscribed")}</span>}
              </div>
              {missing.length > 0 && (
                <p role="status" className="mb-3 fs-body text-ink">
                  {t("preselectMissing", { repo: preview.source.repo, names: missing.join(", ") })}
                </p>
              )}

              {preview.skills.length > SEARCH_FROM && (
                <InputGroup variant="search" className="mb-3 h-8 fs-body">
                  <InputGroupAddon className="pl-3">
                    <Search aria-hidden className="size-3.5" strokeWidth={1.8} />
                  </InputGroupAddon>
                  <InputGroupInput
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Escape" && query) {
                        e.preventDefault()
                        e.stopPropagation()
                        setQuery("")
                      }
                    }}
                    placeholder={t("searchRepoSkills")}
                    aria-label={t("searchRepoSkills")}
                    className="h-full px-0"
                    spellCheck={false}
                    autoComplete="off"
                  />
                </InputGroup>
              )}

              {!(pack?.followAll) && (
                <label className="mb-3 flex items-start gap-2.5 fs-body text-ink">
                  <Checkbox checked={followAll} onCheckedChange={(v) => setFollowAll(!!v)} className="mt-0.5" />
                  <span>{t("followAllLabel")}</span>
                </label>
              )}

              {groups.length === 0 && <p className="py-3 fs-body text-mut">{t("noRepoMatch", { query })}</p>}
              <div className="space-y-4">
                {groups.map(([cat, list]) => {
                  // In adjust mode installed skills can be unticked too; otherwise they stay as they are.
                  const selectable = list.filter((s) => adjust || !have.has(s.name))
                  const n = selectable.filter((s) => isTicked(s.name)).length
                  return (
                    <fieldset key={cat || "-"} disabled={followAll} className="disabled:opacity-60">
                      <legend className="mb-1 flex w-full items-center gap-2.5 fs-meta font-medium text-mut">
                        <Checkbox
                          aria-label={t("selectCategory", { category: cat || t("uncategorized") })}
                          checked={selectable.length > 0 && n === selectable.length}
                          indeterminate={n > 0 && n < selectable.length}
                          disabled={followAll || selectable.length === 0}
                          onCheckedChange={(v) => selectable.forEach((s) => toggle(s.name, !!v))}
                        />
                        {cat || t("uncategorized")}
                      </legend>
                      <ul className="ml-1 [border-left:1.5px_solid_var(--ln)] pl-4">
                        {list.map((s) => {
                          const installed = have.has(s.name)
                          return (
                            <li key={s.name}>
                              <label className="flex items-start gap-2.5 rounded-ctl py-1.5 fs-body">
                                <Checkbox className="mt-0.5" checked={isTicked(s.name)} disabled={(installed && !adjust) || followAll} onCheckedChange={(v) => toggle(s.name, !!v)} />
                                <span className="min-w-0">
                                  <span className="flex items-center gap-2 font-medium text-ink">
                                    {s.name}
                                    {installed && <Tag>{t("installed")}</Tag>}
                                  </span>
                                  <span className="block fs-meta text-mut">{s.description}</span>
                                </span>
                              </label>
                            </li>
                          )
                        })}
                      </ul>
                    </fieldset>
                  )
                })}
              </div>
              {preview.rejected.length > 0 && <p className="mt-3 fs-meta text-mut">{t("rejected", { n: preview.rejected.length })}</p>}

              {(!adjust || adds > 0) && (
                <div className="mt-5 grid gap-4 [border-top:var(--tblRow)] pt-4">
                  <AgentTargets value={targets} onChange={setTargets} label={adjust ? t("installToNew") : undefined} />
                  {!preview.subscribed && (
                    <Field>
                      <FieldLabel htmlFor="add-alias">{t("aliasLabel")}</FieldLabel>
                      <Input id="add-alias" value={alias} onChange={(e) => setAlias(e.target.value)} className="fs-body" spellCheck={false} />
                      <FieldDescription>{t("aliasHint")}</FieldDescription>
                    </Field>
                  )}
                </div>
              )}
            </div>
          )}
          {!preview && !loading && !fetchError && !adjust && <p className="px-6 py-5 fs-meta text-mut">{t("previewOnly")}</p>}
        </div>

        <DialogFooter className="m-0 items-center px-6 py-3.5 [border-top:var(--tblRow)]">
          {preview ? (
            <span className="mr-auto self-center fs-meta text-mut">{removes > 0 ? t("adjustRemoveNote", { n: removes }) : t("selectedCount", { n: adds })}</span>
          ) : (
            !adjust && (
              <Button variant="link" size="inline" className="mr-auto fs-meta" onClick={() => openDialog({ kind: "new" })}>
                {t("orNewSkill")}
              </Button>
            )
          )}
          <Button variant="outline" size="lg" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button size="lg" onClick={submit} disabled={!preview || adds + removes === 0 || (adds > 0 && targets.size === 0) || adding}>
            {adding && <RefreshIcon spinning />}
            {action}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
