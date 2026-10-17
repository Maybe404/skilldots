import * as React from "react"
import { toast } from "sonner"
import { ArrowRight, Loader2 } from "lucide-react"

import { useApp, useEnabledAgents } from "@/lib/app-state"
import { backend, type ParsedSource, type RepoPreview, type UpstreamSkill } from "@/lib/backend"
import type { MessageKey } from "@/lib/i18n"
import { Mono } from "@/components/app/bits"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

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

export function AddDialog({ initial, onClose }: { initial?: string; onClose: () => void }) {
  const { t, run, reload, agents: allAgents } = useApp()
  const enabled = useEnabledAgents()
  const [input, setInput] = React.useState(initial ?? "")
  const [parsed, setParsed] = React.useState<ParsedSource | null>(null)
  const [parseError, setParseError] = React.useState(false)
  const [preview, setPreview] = React.useState<RepoPreview | null>(null)
  const [loading, setLoading] = React.useState(false)
  const [fetchError, setFetchError] = React.useState<string | null>(null)
  const [picked, setPicked] = React.useState<Set<string>>(new Set())
  const [followAll, setFollowAll] = React.useState(false)
  const [targets, setTargets] = React.useState<Set<string>>(() => new Set(allAgents.filter((a) => a.isDefault).map((a) => a.name)))
  const [alias, setAlias] = React.useState("")
  const [adding, setAdding] = React.useState(false)

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

  const read = React.useCallback(
    async (src: ParsedSource) => {
      setLoading(true)
      setFetchError(null)
      try {
        const p = await backend.show(src)
        setPreview(p)
        setPicked(new Set(src.skills.filter((n) => p.skills.some((s) => s.name === n) && !p.have.includes(n))))
        setFollowAll(false)
      } catch (e) {
        setFetchError(e instanceof Error ? e.message : String(e))
      } finally {
        setLoading(false)
      }
    },
    [],
  )

  // Opened from a source ("choose skills…"): read it straight away.
  const auto = React.useRef(!!initial)
  React.useEffect(() => {
    if (auto.current && parsed) {
      auto.current = false
      void read(parsed)
    }
  }, [parsed, read])

  const groups = React.useMemo(() => {
    const m = new Map<string, UpstreamSkill[]>()
    for (const s of preview?.skills ?? []) {
      const k = s.category
      m.set(k, [...(m.get(k) ?? []), s])
    }
    return [...m.entries()].sort(([a], [b]) => (a === "" ? 1 : b === "" ? -1 : a.localeCompare(b)))
  }, [preview])

  const have = new Set(preview?.have ?? [])
  const count = followAll ? (preview?.skills.filter((s) => !have.has(s.name)).length ?? 0) : picked.size
  const toggle = (name: string, on: boolean) =>
    setPicked((p) => {
      const n = new Set(p)
      if (on) n.add(name)
      else n.delete(name)
      return n
    })

  const submit = async () => {
    if (!preview) return
    setAdding(true)
    const r = await run(() =>
      backend.add(preview, { skills: [...picked], categories: [], all: followAll, alias: alias.trim() || undefined, agents: [...targets] }),
    )
    setAdding(false)
    if (!r.ok) return
    await reload()
    toast(t("addedToast", { n: r.value.linked.length }))
    for (const [from, to] of Object.entries(r.value.renamed)) toast(t("renamedToast", { from, to }))
    onClose()
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="flex max-h-[min(44rem,calc(100svh-4rem))] flex-col gap-0 p-0 sm:max-w-2xl">
        <DialogHeader className="px-5 pt-5 pb-4">
          <DialogTitle>{t("addTitle")}</DialogTitle>
          <DialogDescription>{t("addInputHint")}</DialogDescription>
        </DialogHeader>

        <form
          className="px-5 pb-4"
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
                className="font-mono text-[0.82rem]"
                spellCheck={false}
                autoComplete="off"
                aria-invalid={parseError || undefined}
                aria-describedby="add-recognized"
              />
              <Button type="submit" variant="outline" disabled={!parsed || loading}>
                {loading ? <Loader2 className="animate-spin motion-reduce:animate-none" /> : <ArrowRight />}
                {t("readRepo")}
              </Button>
            </div>
            <div id="add-recognized" aria-live="polite" className="min-h-5 text-[0.8rem]">
              {parsed && (
                <p className="text-muted-foreground">
                  {t("recognized")} {t(FORM_LABEL[parsed.form])} · <Mono className="text-foreground">{parsed.repo}</Mono>
                  {parsed.ref && (
                    <>
                      {" "}
                      @<Mono>{parsed.ref}</Mono>
                    </>
                  )}
                  {parsed.skills.length > 0 && <> · {t("preselected", { names: parsed.skills.join(", ") })}</>}
                </p>
              )}
              {parseError && <p className="text-foreground">{t("unrecognized")}</p>}
            </div>
          </Field>
          {!input && (
            <div className="mt-2">
              <p className="mb-1.5 text-[0.78rem] text-muted-foreground">{t("tryThese")}</p>
              <div className="flex flex-wrap gap-1.5">
                {EXAMPLES.map((ex) => (
                  <Button key={ex} type="button" variant="secondary" size="xs" className="max-w-full font-mono font-normal" onClick={() => setInput(ex)}>
                    <span className="truncate">{ex}</span>
                  </Button>
                ))}
              </div>
            </div>
          )}
        </form>

        <div className="min-h-0 flex-1 overflow-y-auto border-t">
          {loading && (
            <p className="flex items-center gap-2 px-5 py-6 text-sm text-muted-foreground" aria-live="polite">
              <Loader2 className="size-4 animate-spin motion-reduce:animate-none" /> {t("reading")}
            </p>
          )}
          {fetchError && (
            <div className="px-5 py-5 text-sm" role="alert">
              <p className="font-medium">{t("readFailed")}</p>
              <p className="mt-1 text-muted-foreground">{fetchError}</p>
            </div>
          )}
          {preview && !loading && (
            <div className="px-5 py-4">
              <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[0.8rem] text-muted-foreground">
                <span className="font-medium text-foreground">{t("pickSkills")}</span>
                <Mono>{preview.commit}</Mono>
                <span>{preview.license ? t("licenseLabel", { license: preview.license }) : t("noLicense")}</span>
                {preview.subscribed && <span>{t("alreadySubscribed")}</span>}
              </div>

              <label className="mb-3 flex items-start gap-2.5 text-[0.85rem]">
                <Checkbox checked={followAll} onCheckedChange={(v) => setFollowAll(!!v)} className="mt-0.5" />
                <span>{t("followAllLabel")}</span>
              </label>

              <div className="space-y-4">
                {groups.map(([cat, list]) => {
                  const selectable = list.filter((s) => !have.has(s.name))
                  const n = selectable.filter((s) => picked.has(s.name)).length
                  return (
                    <fieldset key={cat || "-"} disabled={followAll} className="disabled:opacity-60">
                      <legend className="mb-1 flex w-full items-center gap-2.5 text-[0.78rem] font-medium text-muted-foreground">
                        <Checkbox
                          aria-label={t("selectCategory", { category: cat || t("uncategorized") })}
                          checked={selectable.length > 0 && n === selectable.length}
                          indeterminate={n > 0 && n < selectable.length}
                          disabled={followAll || selectable.length === 0}
                          onCheckedChange={(v) => selectable.forEach((s) => toggle(s.name, !!v))}
                        />
                        {cat || t("uncategorized")}
                      </legend>
                      <ul className="ml-1 border-l pl-4">
                        {list.map((s) => {
                          const installed = have.has(s.name)
                          return (
                            <li key={s.name}>
                              <label className="flex items-start gap-2.5 rounded-md py-1.5 text-[0.85rem]">
                                <Checkbox
                                  className="mt-0.5"
                                  checked={installed || followAll || picked.has(s.name)}
                                  disabled={installed || followAll}
                                  onCheckedChange={(v) => toggle(s.name, !!v)}
                                />
                                <span className="min-w-0">
                                  <span className="flex items-center gap-2 font-medium">
                                    {s.name}
                                    {installed && (
                                      <Badge variant="secondary" className="font-normal">
                                        {t("installed")}
                                      </Badge>
                                    )}
                                  </span>
                                  <span className="block text-[0.8rem] text-muted-foreground">{s.description}</span>
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
              {preview.rejected.length > 0 && <p className="mt-3 text-[0.78rem] text-muted-foreground">{t("rejected", { n: preview.rejected.length })}</p>}

              <div className="mt-5 grid gap-4 border-t pt-4 sm:grid-cols-2">
                <fieldset>
                  <legend className="mb-2 text-[0.8rem] font-medium">{t("installTo")}</legend>
                  <div className="space-y-1.5">
                    {enabled.map((a) => (
                      <label key={a.name} className="flex items-center gap-2.5 text-[0.85rem]">
                        <Checkbox
                          checked={targets.has(a.name)}
                          onCheckedChange={(v) =>
                            setTargets((s) => {
                              const n = new Set(s)
                              if (v) n.add(a.name)
                              else n.delete(a.name)
                              return n
                            })
                          }
                        />
                        {a.label}
                      </label>
                    ))}
                  </div>
                </fieldset>
                {!preview.subscribed && (
                  <Field>
                    <FieldLabel htmlFor="add-alias">{t("aliasLabel")}</FieldLabel>
                    <Input id="add-alias" value={alias} onChange={(e) => setAlias(e.target.value)} className="font-mono text-[0.82rem]" spellCheck={false} />
                    <FieldDescription>{t("aliasHint")}</FieldDescription>
                  </Field>
                )}
              </div>
            </div>
          )}
          {!preview && !loading && !fetchError && <p className="px-5 py-5 text-[0.8rem] text-muted-foreground">{t("previewOnly")}</p>}
        </div>

        <DialogFooter className="m-0 rounded-b-xl px-5 py-3">
          <span className="mr-auto self-center text-[0.8rem] text-muted-foreground">{preview && t("selectedCount", { n: count })}</span>
          <Button variant="outline" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button onClick={submit} disabled={!preview || count === 0 || targets.size === 0 || adding}>
            {adding && <Loader2 className="animate-spin motion-reduce:animate-none" />}
            {t("addN", { n: count })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
