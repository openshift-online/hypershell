// Typed navigation actions for the Glass route layer (GLASS-NAV-04, GLASS-NAV-06).
//
// Components dispatch these framework-free actions through the routing adapter
// instead of mutating `window.location` or `history` directly. `reduceRoute` is a
// pure transition from one normalized RouteState to the next; `isReplaceAction`
// encodes the push-versus-replace policy (text input and coalesced changes replace;
// meaningful navigation pushes) so Back/Forward retrace discrete investigation steps.

import {
  normalizeRoute,
  DEFAULT_ROUTE,
  type ExecState,
  type ExecTabKey,
  type NodeTabKey,
  type RouteState,
  type SortState,
  type TabKey,
} from "./route";

const DEFAULT_EXEC: ExecState = DEFAULT_ROUTE.e2e.exec;

/** Every meaningful navigation the UI can perform, as data. */
export type NavAction =
  // Shell.
  | { readonly type: "tab"; readonly tab: TabKey }
  | { readonly type: "toggleBundle"; readonly seed: string }
  // Cross-tab links (one Back-addressable step each, GLASS-NAV-08).
  | { readonly type: "openHistory"; readonly run: string }
  | { readonly type: "openBundleOnMap"; readonly seed: string }
  // Fleet Map.
  | { readonly type: "mapSelectNode"; readonly id: string }
  | { readonly type: "mapSelectGate"; readonly id: string }
  | { readonly type: "mapClose" }
  | { readonly type: "mapNodeTab"; readonly tab: NodeTabKey }
  | { readonly type: "gatePin"; readonly attempt: string | null }
  | { readonly type: "gateExecTab"; readonly tab: ExecTabKey }
  | { readonly type: "gateStep"; readonly step: string | null }
  | { readonly type: "gateStepSort"; readonly sort: SortState | null }
  | { readonly type: "gateLogExpanded"; readonly value: boolean }
  | { readonly type: "gateLogQuery"; readonly value: string }
  // Agent SDLC.
  | { readonly type: "sdlcScopes"; readonly scopes: readonly string[] }
  | { readonly type: "sdlcQuery"; readonly value: string }
  | { readonly type: "sdlcCard"; readonly card: number | null }
  | {
      readonly type: "sdlcShipped";
      readonly shipped: {
        readonly issue: number;
        readonly version: string | null;
      } | null;
    }
  | { readonly type: "sdlcToggleEscalations" }
  | { readonly type: "sdlcOpenEscalationCard"; readonly card: number }
  | { readonly type: "sdlcShippedRow"; readonly seed: string | null }
  | { readonly type: "sdlcClosePanel" }
  // E2E Runs.
  | { readonly type: "e2eQuery"; readonly value: string }
  | { readonly type: "e2eRunSort"; readonly sort: SortState | null }
  | { readonly type: "e2eSelectRun"; readonly run: string | null }
  | { readonly type: "e2eDrawer"; readonly open: boolean }
  | { readonly type: "e2eExecTab"; readonly tab: ExecTabKey }
  | { readonly type: "e2eStep"; readonly step: string | null }
  | { readonly type: "e2eStepSort"; readonly sort: SortState | null }
  | { readonly type: "e2eLogExpanded"; readonly value: boolean }
  | { readonly type: "e2eLogQuery"; readonly value: string };

/**
 * Text-input and coalesced changes replace the current history entry; everything
 * else is a discrete, Back-addressable navigation step (GLASS-NAV-06).
 */
export function isReplaceAction(action: NavAction): boolean {
  switch (action.type) {
    case "sdlcQuery":
    case "e2eQuery":
    case "gateLogQuery":
    case "e2eLogQuery":
      return true;
    default:
      return false;
  }
}

/** Pure transition. Always returns a normalized RouteState. */
export function reduceRoute(state: RouteState, action: NavAction): RouteState {
  switch (action.type) {
    case "tab":
      return normalizeRoute({ ...state, tab: action.tab });

    case "toggleBundle": {
      const selecting = state.bundle !== action.seed;
      return normalizeRoute({
        ...state,
        bundle: selecting ? action.seed : null,
        // Selecting a shared bundle drops any map node/gate so the bundle takes the
        // map drawer (one thing at a time, GLASS-NAV-09).
        map: selecting ? { ...state.map, node: null, gate: null } : state.map,
      });
    }

    case "openHistory":
      // Map gate -> E2E history attempt: switch to the E2E tab with that run
      // selected and its drawer open, as one history step (GLASS-NAV-08).
      // Carry the gate drawer's execution view (sub-tab, selected step, and
      // viewer state) so stepping from a gate step's logs into run history keeps
      // the same scope and Logs sub-tab (GLASS-NAV-11); the run and step ids are
      // identical across both views. Only an exec context already addressing this
      // run on the E2E tab takes precedence over the carried gate exec.
      return normalizeRoute({
        ...state,
        tab: "e2e",
        e2e: {
          ...state.e2e,
          run: action.run,
          drawer: true,
          exec: action.run === state.e2e.run ? state.e2e.exec : state.map.exec,
        },
      });

    case "openBundleOnMap":
      // SDLC shipped bundle -> Fleet Map bundle, as one history step.
      return normalizeRoute({ ...state, tab: "map", bundle: action.seed });

    case "mapSelectNode": {
      const next = state.map.node === action.id ? null : action.id;
      return normalizeRoute({
        ...state,
        map: { ...state.map, node: next, gate: null, nodeTab: "details" },
      });
    }

    case "mapSelectGate": {
      const next = state.map.gate === action.id ? null : action.id;
      return normalizeRoute({
        ...state,
        map: {
          ...state.map,
          node: null,
          gate: next,
          attempt: null,
          exec: DEFAULT_EXEC,
        },
      });
    }

    case "mapClose":
      // Fully closes the drawer: clears the local node/gate selection and the
      // shared freight bundle that otherwise fills it (mirrors closeSelection).
      return normalizeRoute({
        ...state,
        bundle: null,
        map: { ...state.map, node: null, gate: null },
      });

    case "mapNodeTab":
      return normalizeRoute({
        ...state,
        map: { ...state.map, nodeTab: action.tab },
      });

    case "gatePin":
      // Switch the pinned attempt (or the live sentinel), preserving the exec view:
      // the initial auto-pin on open and a pasted deep link must keep their addressed
      // sub-tab/step (GLASS-NAV-05, GLASS-NAV-10). The selected step simply resolves
      // to whole-run logs if it is absent in the new attempt.
      return normalizeRoute({
        ...state,
        map: { ...state.map, attempt: action.attempt },
      });

    case "gateExecTab":
      return normalizeRoute({
        ...state,
        map: { ...state.map, exec: { ...state.map.exec, tab: action.tab } },
      });

    case "gateStep":
      return normalizeRoute({
        ...state,
        map: { ...state.map, exec: selectStep(state.map.exec, action.step) },
      });

    case "gateStepSort":
      return normalizeRoute({
        ...state,
        map: {
          ...state.map,
          exec: { ...state.map.exec, stepSort: action.sort },
        },
      });

    case "gateLogExpanded":
      return normalizeRoute({
        ...state,
        map: {
          ...state.map,
          exec: { ...state.map.exec, logExpanded: action.value },
        },
      });

    case "gateLogQuery":
      return normalizeRoute({
        ...state,
        map: {
          ...state.map,
          exec: { ...state.map.exec, logQuery: action.value },
        },
      });

    case "sdlcScopes":
      return normalizeRoute({
        ...state,
        sdlc: { ...state.sdlc, scopes: action.scopes },
      });

    case "sdlcQuery":
      return normalizeRoute({
        ...state,
        sdlc: { ...state.sdlc, query: action.value },
      });

    case "sdlcCard":
      // A board-card selection is not an escalation-list detail, so it closes the
      // escalations drawer (a direct board selection, not the notifications path).
      return normalizeRoute({
        ...state,
        sdlc: {
          ...state.sdlc,
          card: action.card,
          shipped: null,
          shippedVersion: null,
          escalations: action.card === null ? state.sdlc.escalations : false,
        },
      });

    case "sdlcShipped":
      return normalizeRoute({
        ...state,
        sdlc: {
          ...state.sdlc,
          card: null,
          shipped: action.shipped?.issue ?? null,
          shippedVersion: action.shipped?.version ?? null,
          escalations: false,
        },
      });

    case "sdlcToggleEscalations":
      return normalizeRoute({
        ...state,
        sdlc: {
          ...state.sdlc,
          escalations: !state.sdlc.escalations,
          // Opening or closing the list clears any open detail card.
          card: null,
          shipped: null,
          shippedVersion: null,
        },
      });

    case "sdlcOpenEscalationCard":
      // Opens an escalation's issue detail while keeping the list open underneath,
      // so browser Back and the drawer's Back-to-list both return to the list
      // (GLASS-NAV-13).
      return normalizeRoute({
        ...state,
        sdlc: {
          ...state.sdlc,
          escalations: true,
          card: action.card,
          shipped: null,
        },
      });

    case "sdlcShippedRow":
      // Toggle the expanded matrix row (addressed by bundle seed).
      return normalizeRoute({
        ...state,
        sdlc: {
          ...state.sdlc,
          shippedRow:
            state.sdlc.shippedRow === action.seed ? null : action.seed,
        },
      });

    case "sdlcClosePanel":
      return normalizeRoute({
        ...state,
        sdlc: {
          ...state.sdlc,
          card: null,
          shipped: null,
          shippedVersion: null,
          escalations: false,
        },
      });

    case "e2eQuery":
      return normalizeRoute({
        ...state,
        e2e: { ...state.e2e, query: action.value },
      });

    case "e2eRunSort":
      return normalizeRoute({
        ...state,
        e2e: { ...state.e2e, runSort: action.sort },
      });

    case "e2eSelectRun": {
      // Changing the selected run remounts the detail, resetting its exec view and
      // closing the previous run's subscription (GLASS-NAV-18).
      const changed = action.run !== state.e2e.run;
      return normalizeRoute({
        ...state,
        e2e: {
          ...state.e2e,
          run: action.run,
          drawer: action.run !== null,
          exec: changed ? DEFAULT_EXEC : state.e2e.exec,
        },
      });
    }

    case "e2eDrawer":
      return normalizeRoute({
        ...state,
        e2e: { ...state.e2e, drawer: action.open },
      });

    case "e2eExecTab":
      return normalizeRoute({
        ...state,
        e2e: { ...state.e2e, exec: { ...state.e2e.exec, tab: action.tab } },
      });

    case "e2eStep":
      return normalizeRoute({
        ...state,
        e2e: { ...state.e2e, exec: selectStep(state.e2e.exec, action.step) },
      });

    case "e2eStepSort":
      return normalizeRoute({
        ...state,
        e2e: {
          ...state.e2e,
          exec: { ...state.e2e.exec, stepSort: action.sort },
        },
      });

    case "e2eLogExpanded":
      return normalizeRoute({
        ...state,
        e2e: {
          ...state.e2e,
          exec: { ...state.e2e.exec, logExpanded: action.value },
        },
      });

    case "e2eLogQuery":
      return normalizeRoute({
        ...state,
        e2e: {
          ...state.e2e,
          exec: { ...state.e2e.exec, logQuery: action.value },
        },
      });
  }
}

/** Selecting a concrete step jumps to the Logs sub-tab; whole-run keeps the tab. */
function selectStep(exec: ExecState, step: string | null): ExecState {
  return step !== null
    ? { ...exec, step, tab: "logs" }
    : { ...exec, step: null };
}
