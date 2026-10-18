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

The browser preview accepts `?fresh` (first-run onboarding) and `?drift`
(links that disagree with `skills.toml`, to exercise the apply dialog).

## Interface

The layout follows the approved design (`skilldots 主界面`): a top bar with
the page navigation, then Skill, Sources, Agents, History, Adopt, Trash and
Settings pages. Screenshots of every page, the dialogs and the two styles are
in `docs/screenshots/` (1280×820 window, 1.5× scale, sample data).

- **Window.** The native window has no system title bar
  (`TitleBarStyle: TitleBarHidden` in `main.go`): the app's top bar is the
  only row at the top. The traffic lights sit at its left end, centred on the
  line of its 32px controls (`TrafficLightPosition` 20,20 in a 54px bar);
  the bar keeps clear of them with mygo's `--mygo-titlebar-inset-left`, which
  is 0 in full screen and unset in the browser preview. Empty parts of the
  bar drag the window (`--app-region: drag`, `.skd-titlebar` in
  `src/index.css`) and a double click there does what the system setting
  says (zoom by default); buttons, fields and the control groups do not drag.
- **Components.** Controls are shadcn (base-nova, Base UI) components, most
  with Animate UI's motion (`src/components/animate-ui/`, MIT + Commons
  Clause): segmented controls and the page navigation slide one highlight to
  the current item, switches spring, check marks draw, dialogs and the detail
  sheet animate in and out, tooltips spring from their trigger, counts and
  elapsed seconds roll. All of it follows the system's reduced-motion
  setting (`MotionConfig reducedMotion="user"`, plus explicit fallbacks for
  height and stroke animations). The wire view's layout is written for this
  app; its buttons and hints are components.
- **Styles.** "Clean" (default) and "Doodle" are two sets of CSS variables in
  `src/styles/themes.css`, chosen by `data-style` on `<html>` and remembered
  in the window's local storage. Components use the token utilities from
  `src/index.css` (`text-ink`, `bg-nav`, `rounded-ctl`, `fs-body`,
  `border-b-row` …), never raw values. Only light mode exists so far; the
  dark option in Settings explains that it is still being designed.
- **Fonts.** Geist, JetBrains Mono, Gaegu and Long Cang (all SIL OFL 1.1, from
  `@fontsource`) ship inside the bundle as woff2 (`src/styles/fonts.css`), so
  the window needs no network. Chinese falls back to PingFang SC.
- **Pending link changes.** Toggling a skill for an agent (wire view, table
  view, detail sheet, number keys, drag) only records the wanted state. The
  status bar shows the count with Apply and Undo; while applying it shows a
  spinner and the elapsed time. Apply sends one `enable` (agents added) and
  one `disable` (agents removed) per changed skill instead of overwriting the
  whole target list, so links changed elsewhere meanwhile are kept; it stops
  at the first skill that fails and says which one and how many changes went
  through. Closing the window with pending changes asks first. Hiding an
  agent column that has pending changes asks first and discards them; pending
  changes of hidden columns are never applied.
- **Keyboard.** ↑↓ select a skill, 1–9 toggle the agent columns, ⏎ applies
  pending changes (or opens details), Space opens details, V switches view,
  Esc clears the agent focus and then the pending changes (Esc inside the
  search field only leaves the search). ⌘K opens the
  command palette, ⌘F the search, ⌘N a new skill, ⌘, Settings.

## Behavior

- Hiding an agent column only changes desktop preferences: it also clears the
  agent focus, and the "install to" lists of new and added skills still show
  every enabled agent.
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
agent toggles and presses Apply (a toggle only records a pending change),
checks the resulting links, reads and writes a skill, checks an empty apply
plan, checks the title bar (room for the traffic lights, brand clear of them,
controls on their centre line, drag regions), and records font/blur support
plus a screenshot of the page.

With `SKILLDOTS_SMOKE_SHOTS=<dir>` it then also screenshots the whole window,
traffic lights included, in both styles and in English, double-clicks the bar
(recording the window bounds before and after) and enters full screen, and
writes the title bar numbers of each state to `<dir>/titlebar.json`. The
window screenshots need macOS Screen Recording permission and an unlocked
screen; a screenshot that cannot be taken is reported in `titlebar.json`
(the page alone is always saved as `<name>.page.png`). If the harness process
itself may not record the screen, set `SKILLDOTS_SMOKE_CAPTURE=external`: it
then writes `<name>.rect` and waits for `<name>.png`, which a script with the
permission takes with `screencapture -x -o -R "$(cat <name>.rect)" <name>.png`.
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
