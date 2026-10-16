# skilldots desktop (front end)

The Mac app's user interface: React + TypeScript + Vite, Tailwind v4 and shadcn
(Base UI, `base-nova`). It talks to skilldots only through the `Backend`
interface in `src/lib/backend/types.ts`, whose methods mirror `internal/app`.
For now an in-memory mock (`src/lib/backend/mock.ts`) stands in for the Go
side; nothing touches the real file system.

```sh
bun install
bun run dev          # http://localhost:5173
bun run typecheck
bun run build
```

`?fresh` in the URL starts without a library, to see onboarding.

## Layout

| Path | What |
|---|---|
| `src/lib/backend/` | `Backend` interface, mock library, add-box parser |
| `src/lib/i18n/messages.ts` | every string, Chinese and English |
| `src/lib/app-state.tsx` | library data, navigation, dialogs, theme and language |
| `src/components/pages/` | one file per sidebar page, plus onboarding |
| `src/components/dialogs/` | add, new, fork, update, apply, sync, library, confirmations |
| `src/components/editor/` | in-app editor (CodeMirror 6, Markdown preview) |
| `src/components/ui/` | shadcn components (CLI-installed) |
| `src/components/animate-ui/` | animate-ui Files, used for the skill file tree |
| `docs/screenshots/` | light and dark screenshots |

## Swapping in the Go backend

Generate bindings for the methods in `Backend` and export an object that
implements it from `src/lib/backend/index.ts`. Methods the CLI does not have
yet: `deleteForever` (remove one trash entry), `history` (update history kept
in the library), `claudeOverrides` (read `skillOverrides` from
`~/.claude/settings.json`), `setAgentEnabled` (which agents are shown),
`files` / `readFile` / `writeFile`, `reveal` / `revealLibrary` /
`openInEditor` / `pickFolder`, and `parseSource` accepting pasted
`npx skills add` lines, `/tree/`, `/blob/` and skills.sh links.
