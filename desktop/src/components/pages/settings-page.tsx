import { CloudUpload, Download, FolderOpen, FolderPlus, Replace } from "lucide-react"
import { toast } from "sonner"

import { useApp } from "@/lib/app-state"
import { backend, type Config, type Theme } from "@/lib/backend"
import { ago } from "@/lib/i18n"
import { Mono, PageHeader } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

function Row({ label, hint, htmlFor, children }: { label: string; hint?: string; htmlFor?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-6 px-4 py-3.5">
      <div className="min-w-0">
        <label htmlFor={htmlFor} className="text-[0.88rem]">
          {label}
        </label>
        {hint && <p className="mt-0.5 text-[0.78rem] text-muted-foreground">{hint}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-7">
      <h2 className="mb-2 px-1 text-[0.8rem] font-semibold text-muted-foreground">{title}</h2>
      <div className="divide-y rounded-xl border">{children}</div>
    </section>
  )
}

export function SettingsPage() {
  const { t, config, setConfig, status, openDialog, run } = useApp()
  const editors: [Config["editor"], string][] = [
    ["auto", t("editorAuto")],
    ["code", "VS Code"],
    ["cursor", "Cursor"],
    ["zed", "Zed"],
    ["custom", t("editorCustom")],
  ]
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <PageHeader title={t("settingsTitle")} hint={t("settingsShared")} />
      <div className="max-w-3xl px-8 pb-10">
        <Group title={t("appearance")}>
          <Row label={t("theme")}>
            <ToggleGroup
              aria-label={t("theme")}
              value={[config.theme]}
              onValueChange={(v) => v[0] && setConfig({ theme: v[0] as Theme })}
              variant="outline"
              size="sm"
              spacing={0}
            >
              {(["system", "light", "dark"] as const).map((v) => (
                <ToggleGroupItem key={v} value={v} className="data-[pressed]:bg-primary data-[pressed]:text-primary-foreground">
                  {t(v === "system" ? "themeSystem" : v === "light" ? "themeLight" : "themeDark")}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </Row>
          <Row label={t("language")}>
            <Select value={config.lang} onValueChange={(v) => v && setConfig({ lang: v as Config["lang"] })} items={{ zh: "简体中文", en: "English" }}>
              <SelectTrigger aria-label={t("language")} className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="zh">简体中文</SelectItem>
                <SelectItem value="en">English</SelectItem>
              </SelectContent>
            </Select>
          </Row>
        </Group>

        <Group title={t("editorSection")}>
          <Row label={t("editorLabel")}>
            <Select value={config.editor} onValueChange={(v) => v && setConfig({ editor: v as Config["editor"] })} items={Object.fromEntries(editors)}>
              <SelectTrigger aria-label={t("editorLabel")} className="w-64">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {editors.map(([v, label]) => (
                  <SelectItem key={v} value={v}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Row>
          {config.editor === "custom" && (
            <div className="px-4 py-3.5">
              <Field>
                <FieldLabel htmlFor="editor-cmd">{t("editorCommand")}</FieldLabel>
                <Input
                  id="editor-cmd"
                  defaultValue={config.editorCommand}
                  onBlur={(e) => setConfig({ editorCommand: e.target.value })}
                  placeholder="subl {path}"
                  className="font-mono text-[0.82rem]"
                  spellCheck={false}
                />
                <FieldDescription>{t("editorCommandHint", { path: "{path}" })}</FieldDescription>
              </Field>
            </div>
          )}
        </Group>

        <Group title={t("updatesSection")}>
          <Row label={t("checkOnLaunch")} hint={t("checkOnLaunchHint")} htmlFor="check-launch">
            <Switch id="check-launch" checked={config.checkOnLaunch} onCheckedChange={(v) => setConfig({ checkOnLaunch: v })} />
          </Row>
        </Group>

        <Group title={t("librarySection")}>
          <div className="px-4 py-3.5">
            <p className="text-[0.78rem] text-muted-foreground">{t("libraryPath")}</p>
            <Mono className="mt-0.5 block break-all">{status?.root}</Mono>
            <div className="mt-3 flex flex-wrap gap-2">
              <Button variant="outline" size="sm" onClick={async () => (await run(() => backend.revealLibrary())).ok && toast(t("revealed"))}>
                <FolderOpen /> {t("revealInFinder")}
              </Button>
              <Button variant="outline" size="sm" onClick={() => openDialog({ kind: "library", mode: "use" })}>
                <Replace /> {t("switchLibrary")}
              </Button>
              <Button variant="outline" size="sm" onClick={() => openDialog({ kind: "library", mode: "clone" })}>
                <Download /> {t("importLibrary")}
              </Button>
              <Button variant="outline" size="sm" onClick={() => openDialog({ kind: "library", mode: "init" })}>
                <FolderPlus /> {t("newLibrary")}
              </Button>
            </div>
          </div>
        </Group>

        <Group title={t("backupSection")}>
          <div className="flex items-center justify-between gap-6 px-4 py-3.5">
            <div className="min-w-0">
              {status?.remote ? <Mono className="block truncate">{status.remote}</Mono> : <p className="text-[0.88rem]">{t("noRemote")}</p>}
              <p className="mt-0.5 text-[0.78rem] text-muted-foreground">
                {status?.changes ? t("unsynced", { n: status.changes }) : t("synced")}
                {status?.lastSync && ` · ${t("lastSync", { ago: ago(status.lastSync, t) })}`}
              </p>
            </div>
            <Button size="sm" onClick={() => openDialog({ kind: "sync" })}>
              <CloudUpload /> {t("syncNow")}
            </Button>
          </div>
        </Group>
      </div>
    </div>
  )
}
