import { toast } from "sonner"
import { useApp } from "@/lib/app-state"
import { backend, type Ref, type Skill } from "@/lib/backend"

/** Skill operations shared by the table, the detail panel and the palette. */
export function useSkillActions() {
  const { t, agents, run, reload, navigate } = useApp()
  const label = (name: string) => agents.find((a) => a.name === name)?.label ?? name

  const setAgent = async (skill: Skill, agent: string, on: boolean) => {
    if (on) {
      if (!(await run(() => backend.enable(skill.ref, [agent]))).ok) return
      await reload()
      toast(t("enabledIn", { skill: skill.linkName, agent: label(agent) }))
    } else {
      if (!(await run(() => backend.disable(skill.ref, [agent]))).ok) return
      await reload()
      toast(t("disabledIn", { skill: skill.linkName, agent: label(agent) }), {
        action: {
          label: t("undo"),
          onClick: async () => {
            await run(() => backend.enable(skill.ref, [agent]))
            await reload()
          },
        },
      })
    }
  }

  const reveal = async (ref: Ref) => {
    if ((await run(() => backend.reveal(ref))).ok) toast(t("revealed"))
  }
  const openInEditor = async (ref: Ref) => {
    if ((await run(() => backend.openInEditor(ref))).ok) toast(t("openedInEditor"))
  }
  const trashToast = (name: string) =>
    toast(t("movedToTrash", { name }), { action: { label: t("openTrash"), onClick: () => navigate("trash") } })

  return { setAgent, reveal, openInEditor, trashToast, agentLabel: label }
}
