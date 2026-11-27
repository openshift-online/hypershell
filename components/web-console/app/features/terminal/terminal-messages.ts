import { defineMessages } from "react-intl";

/** Copy for the browser hsctl terminal (WEB-TERM, WEB-I18N-01). */
export const terminalMessages = defineMessages({
  openTerminal: {
    id: "app.terminal.trigger.open",
    defaultMessage: "Open hsctl terminal",
    description:
      "Accessible name of the masthead icon button while the terminal drawer is closed.",
  },
  closeTerminal: {
    id: "app.terminal.trigger.close",
    defaultMessage: "Close hsctl terminal",
    description:
      "Accessible name of the masthead icon button while the terminal drawer is open.",
  },
  regionLabel: {
    id: "app.terminal.region.label",
    defaultMessage: "hsctl terminal",
    description:
      "Accessible name of the terminal drawer region. hsctl is the HyperShell CLI and is not translated.",
  },
  title: {
    id: "app.terminal.title",
    defaultMessage: "hsctl terminal",
    description: "Heading of the terminal drawer.",
  },
  closePanel: {
    id: "app.terminal.panel.close",
    defaultMessage: "Close terminal",
    description: "Accessible name of the close button inside the drawer.",
  },
  dockRight: {
    id: "app.terminal.dock.right",
    defaultMessage: "Dock right",
    description: "Button that moves the terminal drawer to the right side.",
  },
  dockBottom: {
    id: "app.terminal.dock.bottom",
    defaultMessage: "Dock bottom",
    description: "Button that moves the terminal drawer to the bottom.",
  },
  resizeHandle: {
    id: "app.terminal.resize.label",
    defaultMessage: "Resize hsctl terminal",
    description:
      "Accessible name of the drag handle that changes the terminal drawer size.",
  },
  resizeValue: {
    id: "app.terminal.resize.value",
    defaultMessage: "{size} pixels",
    description:
      "Spoken value of the terminal resize handle. size is the drawer size in CSS pixels.",
  },
  signIn: {
    id: "app.terminal.signIn",
    defaultMessage: "Sign in to use the hsctl terminal",
    description:
      "Link shown in the terminal drawer to a signed-out reader. It opens the console sign-in page and returns here.",
  },
  checkingSession: {
    id: "app.terminal.session.checking",
    defaultMessage: "Checking your console session",
    description:
      "Status shown in the terminal drawer while the console session is read.",
  },
  loading: {
    id: "app.terminal.runtime.loading",
    defaultMessage: "Loading hsctl",
    description: "Status while the hsctl browser runtime downloads and starts.",
  },
  loadingBytes: {
    id: "app.terminal.runtime.loadingBytes",
    defaultMessage: "Loading hsctl: {loaded} downloaded",
    description:
      "Status while the hsctl runtime downloads and its total size is unknown. loaded is a formatted size such as 2.4 MB.",
  },
  loadFailedTitle: {
    id: "app.terminal.runtime.loadFailed.title",
    defaultMessage: "hsctl could not be loaded",
    description:
      "Title of the error shown when the hsctl runtime fails to load.",
  },
  loadFailedBody: {
    id: "app.terminal.runtime.loadFailed.body",
    defaultMessage:
      "Check your connection and try again. You can still use hsctl in a local terminal.",
    description:
      "Body of the error shown when the hsctl runtime fails to load.",
  },
  unavailableBody: {
    id: "app.terminal.runtime.unavailable.body",
    defaultMessage:
      "This console build does not include the browser hsctl runtime. Use hsctl in a local terminal.",
    description:
      "Body of the error shown when the console was built without the hsctl runtime.",
  },
  retry: {
    id: "app.terminal.runtime.retry",
    defaultMessage: "Retry",
    description: "Button that tries to load the hsctl runtime again.",
  },
  inputLabel: {
    id: "app.terminal.input.label",
    defaultMessage: "hsctl command",
    description: "Accessible name of the terminal command input.",
  },
  inputHelp: {
    id: "app.terminal.input.help",
    defaultMessage:
      "Enter an hsctl command, for example hsctl list gateways. Type clear to clear the log.",
    description: "Accessible description of the terminal command input.",
  },
  logLabel: {
    id: "app.terminal.log.label",
    defaultMessage: "hsctl output",
    description: "Accessible name of the terminal output log.",
  },
  runbookTag: {
    id: "app.terminal.log.runbookTag",
    defaultMessage: "[runbook]",
    description:
      "Tag shown before a command that a docs runbook cell ran. Keep the square brackets.",
  },
  stderrMarker: {
    id: "app.terminal.log.stderr",
    defaultMessage: "Error output:",
    description:
      "Screen-reader text before a line the command wrote to standard error.",
  },
  droppedLines: {
    id: "app.terminal.log.droppedLines",
    defaultMessage:
      "{count, plural, one {# older line was} other {# older lines were}} removed. The log keeps the latest {limit, number} lines.",
    description:
      "Notice at the top of the log after old lines are removed to respect the line limit.",
  },
  emptyLog: {
    id: "app.terminal.log.empty",
    defaultMessage:
      "Run hsctl commands with your console session. Commands that need a local file or login run only in a local terminal.",
    description: "Text shown in the log before any command has run.",
  },
  exitCode: {
    id: "app.terminal.notice.exitCode",
    defaultMessage: "Exit code {exitCode, number}",
    description: "Shown after the output of a command that failed.",
  },
  cancelled: {
    id: "app.terminal.notice.cancelled",
    defaultMessage: "Command cancelled.",
    description: "Shown after a running command is cancelled with Ctrl+C.",
  },
  interrupted: {
    id: "app.terminal.notice.interrupted",
    defaultMessage: "^C",
    description:
      "Shown after an unfinished input line when the user presses Ctrl+C with no running command. Keep as ^C.",
  },
  shellOperators: {
    id: "app.terminal.notice.shellOperators",
    defaultMessage:
      "Shell operators are not supported. Pipes, redirection, ;, &, $( and backticks only work in a local terminal.",
    description:
      "Shown when an input line contains shell operators, which the browser terminal never runs.",
  },
  incompleteLine: {
    id: "app.terminal.notice.incompleteLine",
    defaultMessage:
      "The command has an unclosed quote or a trailing backslash.",
    description: "Shown when an input line cannot be split into words.",
  },
  notHsctl: {
    id: "app.terminal.notice.notHsctl",
    defaultMessage:
      "Only hsctl commands run here. Run {program} and other programs, such as openshell or kubectl, in a local terminal.",
    description:
      "Shown when the first word of a line is not hsctl or an hsctl command. program is the word the user typed.",
  },
  runtimeError: {
    id: "app.terminal.notice.runtimeError",
    defaultMessage: "hsctl stopped unexpectedly. Run the command again.",
    description: "Shown when the hsctl runtime fails while running a command.",
  },
  sessionExpired: {
    id: "app.terminal.notice.sessionExpired",
    defaultMessage: "Your console session expired. <link>Sign in again</link>.",
    description:
      "Shown after a terminal command fails because the console session expired. The link opens sign-in and returns to this page.",
  },
  commandFinished: {
    id: "app.terminal.status.finished",
    defaultMessage: "Command finished with exit code {exitCode, number}.",
    description:
      "Polite screen-reader announcement when a terminal command finishes.",
  },
  commandCancelledStatus: {
    id: "app.terminal.status.cancelled",
    defaultMessage: "Command cancelled.",
    description:
      "Polite screen-reader announcement when a terminal command is cancelled.",
  },
  commandFailedStatus: {
    id: "app.terminal.status.runtimeError",
    defaultMessage: "Command stopped because hsctl failed.",
    description:
      "Polite screen-reader announcement when the hsctl runtime fails during a command.",
  },
  running: {
    id: "app.terminal.status.running",
    defaultMessage: "Running. Press Ctrl+C to cancel.",
    description: "Hint shown under the prompt while a command runs.",
  },
});
