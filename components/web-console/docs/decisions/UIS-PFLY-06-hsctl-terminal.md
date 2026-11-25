# UIS-PFLY-06 - Decision record: the hsctl browser terminal

**Status:** Accepted
**Standard:** `specs/standards/ui/patternfly.spec.md` UIS-PFLY-05/06/09,
`specs/web-console/architecture.spec.md` WEB-UIEX-01 (specialized widgets need
a concrete feature, an accessibility review, a bundle assessment, and a
documented PatternFly integration).
**Feature spec:** `specs/web-console/hsctl-browser-terminal.spec.md` (WEB-TERM),
`specs/platform/hsctl-browser-build.spec.md` (CLI-WASM).
**Code:** `app/features/terminal/`, `app/application/terminal/`,
`app/adapters/terminal/`, `app/composition/terminal-composition.ts`,
`vite-plugins/hsctl-runtime.ts`, `vite-plugins/web-app-manifest.ts`.

## 1. Gap

Signed-in users need to run the real `hsctl` command tree from any console
page without installing it (WEB-TERM). That needs a command prompt with
line-editing keys, a scrolling text log of commands and output, and a docked,
resizable surface that sits beside the page instead of covering it.

PatternFly 6 has no terminal or command-prompt component. The PatternFly
Drawer gives inline bottom and right docking, and PatternFly form, alert,
progress, and button components cover the controls. The prompt, the log, and
the resize handle are a composition, not a new design system.

## 2. Alternatives considered

| Alternative                                              | Decision                                                                                                                                                                                                                                                                                |
| -------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A terminal emulator (xterm.js or similar)                | Rejected. WEB-TERM-12 forbids an emulator dependency and ANSI interpretation. An emulator draws to a canvas or its own DOM grid, needs its own screen-reader mode, and adds about 300 kB of JavaScript for features `hsctl` does not use.                                               |
| PatternFly `CodeBlock` for the log, `TextArea` for input | Rejected. CodeBlock is a static code sample with copy actions, not a growing live log. A multi-line TextArea makes Enter ambiguous.                                                                                                                                                     |
| PatternFly resizable `DrawerPanelContent` splitter       | Rejected. Its `aria-valuenow` is a percentage (0-100), it steps 5 px by default, and it lets the size pass its CSS bounds while reporting the unclamped value. WEB-TERM-03 needs a value in CSS pixels, 24 px steps, and clamping that is reported (190 px becomes 180 px, not 166 px). |
| A modal or overlay panel                                 | Rejected. WEB-TERM-02 requires the page to stay readable beside the terminal, and WEB-TERM-12 requires a non-modal region with no focus trap.                                                                                                                                           |

**Decision:** compose PatternFly 6 parts:

- `Drawer` (`isInline`, `position` `bottom` or `end`), `DrawerContent`, and
  `DrawerPanelContent` for docking. The drawer wraps the whole page.
- `DrawerHead`, `DrawerActions`, `DrawerCloseButton`, `Title`, and a link
  `Button` for the header.
- `InputGroup`, `InputGroupText`, and `TextInput` for the prompt.
- `Alert` with `AlertActionLink` (Retry), `Progress`, and `Spinner` for loading.
- The `TerminalIcon` plain `Button` as the masthead trigger.

Only three pieces are custom: the text log (`div role="log"`), the
window-splitter resize handle (`div role="separator"`), and a small CSS module
that uses PatternFly semantic tokens only. The CSS module also keeps the
bottom dock inline below the PatternFly `md` breakpoint, where PatternFly would
otherwise overlay the panel.

## 3. Consumers and API

- Consumers: the application shell (trigger and drawer on every console page)
  and the docs runbook cells (WEB-UDOC-05), which write to the same history.
- Public API, from `app/features/terminal/terminal-context.tsx`:
  - `TerminalProvider`. Optional props: `operations`, `preferences`,
    `session`, `lineLimit`.
  - `useTerminal()`, which returns `isOpen`, `open({ entryId? })`, `close()`,
    `toggle()`, and `startRunbookEntry(command)`. `startRunbookEntry` returns
    a `RunbookEntryHandle` with `entryId`, `appendOutput(lines, stream?)`, and
    `finish({ exitCode })`.
- `TerminalPanel` is presentational and props-driven, so stories need no
  worker (`Production/Terminal/hsctl terminal`).

## 4. Accessibility review

Reviewed against `accessibility.spec.md` and the WAI-ARIA Authoring Practices.

| Concern                             | Implementation                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Name, role, value (4.1.2)           | The trigger is named "Open hsctl terminal" or "Close hsctl terminal" and has `aria-expanded`, `aria-controls`, and `aria-keyshortcuts`. The open panel is `role="region"` named "hsctl terminal". The input is named "hsctl command" and described by its help text. The resize handle is `role="separator"` with a name, `aria-orientation`, `aria-valuenow`, `aria-valuemin`, and `aria-valuemax` in CSS pixels, plus `aria-valuetext`. |
| Keyboard (2.1.1, 2.1.2)             | Ctrl+` toggles the drawer from anywhere. Esc closes it when focus is inside. The arrow keys resize in 24 px steps, and Home or End go to the minimum or maximum. The line-editing keys work only while the input has focus. There is no focus trap. The scrollable log is focusable (`tabIndex=0`), so keyboard users can scroll it.                                                                                                      |
| Focus order and return (2.4.3)      | Opening moves focus to the input, or to the sign-in link when signed out. While the runtime loads, the input is read-only rather than disabled, so it can still take focus. Closing returns focus to the element focused before opening, or to the trigger.                                                                                                                                                                               |
| Status messages (4.1.3)             | Each command's completion and exit status are announced through a polite `role="status"` region. The log is `role="log"` with `aria-live="off"`, so long output does not flood screen readers.                                                                                                                                                                                                                                            |
| Use of color (1.4.1)                | Standard error lines have an icon and visually hidden "Error output:" text as well as the danger text color. A non-zero exit code appears as text ("Exit code 2").                                                                                                                                                                                                                                                                        |
| Contrast and themes (1.4.3, 1.4.11) | Only PatternFly semantic text and border tokens are used, so light and dark themes follow PatternFly.                                                                                                                                                                                                                                                                                                                                     |
| Reflow (1.4.10)                     | Long lines wrap (`pre-wrap`, `overflow-wrap: anywhere`). The right dock is not offered below `md`.                                                                                                                                                                                                                                                                                                                                        |
| Motion (2.3.3, UIS-A11Y-08)         | New lines animate `opacity` and `translateY` over 160 ms only under `prefers-reduced-motion: no-preference`. With reduced motion, lines appear at once and scrolling is instant.                                                                                                                                                                                                                                                          |
| Targets and dragging (2.5.7, 2.5.8) | Every drag action has a keyboard alternative. The handle's hit area is the PatternFly `md` spacer.                                                                                                                                                                                                                                                                                                                                        |
| Text safety (WEB-SEC-01)            | Output, commands, and notices render as React text. There is no raw-HTML path and no ANSI interpretation. A unit test checks that `<img src=x onerror=...>` output stays visible text.                                                                                                                                                                                                                                                    |

Automated axe checks (Storybook a11y addon, Playwright) are supporting
evidence only (UIS-VER-08). Manual screen-reader and zoom passes remain
release work under the verification standard.

## 5. Bundle assessment

Measured on the production build with the CLI's local `js/wasm` module
(sizes in bytes):

| Asset                                                        | When it loads                         |        Raw |      gzip |    brotli |
| ------------------------------------------------------------ | ------------------------------------- | ---------: | --------: | --------: |
| `terminal-context-<hash>.js` (provider, drawer, log, prompt) | With the application shell            |     17,340 |     6,085 |     5,508 |
| `hsctl-worker-<hash>.js` (classic worker)                    | First drawer open by a signed-in user |      2,136 |       993 |       862 |
| `wasm_exec-<hash>.js` (Go runtime glue)                      | Same                                  |     16,992 |     4,352 |     3,752 |
| `hsctl-<hash>.wasm`                                          | Same                                  | 24,960,434 | 5,859,142 | 4,069,417 |

- No new npm dependency. The UI uses PatternFly packages the console already
  installs.
- The worker, `wasm_exec.js`, and `hsctl.wasm` are never requested on page
  load (WEB-TERM-09). Playwright and unit tests check this. All three are
  content-hashed immutable assets, so later opens use the browser cache.
- The compressed `hsctl.wasm` (about 3.9 MiB brotli) is within the CLI-WASM-05
  budget of 8 MiB. The CLI CI owns that check.
- Loading shows truthful progress: a byte-accurate bar when the response is not
  compressed, and downloaded megabytes otherwise.

## 6. Lifecycle and retirement

The terminal UI is owned by the web console. If PatternFly ships a
command-prompt or log component, or a resizable drawer splitter with a
pixel-valued, clamped `aria-valuenow`, adopt it and remove the custom log or
handle. The application layer (`app/application/terminal/`) has no React
dependency and does not change. This record admits only the terminal; it does
not sanction other custom components.
