# skilldots desktop

macOS application built with MyGo 0.5.5, React, TypeScript and Vite.
The native window calls `api.Service.Invoke`, which uses the shared Go
application layer. Browser-only previews use an in-memory mock; native errors
never fall back to mock data.

## Development

```sh
bun install
bun run dev             # browser preview with mock data
bun run desktop:dev     # native window with the real Go backend
bun run desktop:build   # native application bundle under build/
bun run typecheck
bun run lint
```

Native development can change library files and agent links. For testing,
use a dedicated empty HOME, set SKILLDOTS_LIBRARY to an isolated library,
and isolate Git configuration and SKILLDOTS_CACHE. Do not test against your
normal skill library. Generated bindings and build products are ignored.

## Behavior

- Hiding an agent column only changes desktop preferences.
- Pausing an agent persists in `targets.disabled`, removes managed links on
  apply, and preserves skill selections so resuming restores distribution.
- Library skills, including private and self-authored skills, move to the
  library trash. A machine-local provenance record permits safe restoration.
- App-owned skills can be unlinked but are not moved or edited in place.
- Permanent deletion is unavailable in the desktop bridge. Empty the trash
  explicitly from an interactive terminal with `skd trash empty`.
- File editing is confined to regular UTF-8 files within the selected skill;
  hidden paths, symlinks and traversal are rejected. Copy-based destinations
  require Apply after editing.
- The desktop bridge is separate from the CLI JSON contract.

## Verification

Go API tests use isolated temporary libraries. The native smoke harness
(`cmd/smoke`, build tag `desktop_smoke`) launches a real WKWebView, clicks
agent toggles, checks the resulting links, reads and writes a skill, checks
an empty apply plan, and records font/blur support plus a screenshot.
Build the harness from the repository root:

```sh
go build -tags desktop_smoke -o /path/to/test-directory/desktop-smoke ./desktop/cmd/smoke
```

Run it with an empty isolated HOME and matching SKILLDOTS_SMOKE_HOME; pass
absolute paths to `desktop/dist` and an output JSON report. Keep all test
artifacts inside a dedicated test directory. Each run needs a fresh HOME.

The native toggle/editor path has been verified on macOS. Folder pickers,
Finder integration, repository cloning/updating and all theme/window-size
combinations still need broader interactive acceptance testing. History
backup details are not yet populated. Some Go
error details remain English. This is an integration build, not a release.
