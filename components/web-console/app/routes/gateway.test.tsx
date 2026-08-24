import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as GatewayUi from "@openshift-online/hypershell-gateway-management-ui";
import type {
  ServiceAccountLeaveDecision,
  ServiceAccountLeaveGuard,
} from "@openshift-online/hypershell-gateway-management-ui";
import { useEffect, useRef, useState } from "react";
import { createMemoryRouter, Link, RouterProvider } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import GatewayRoute from "./gateway";

// Plain labels kept out of JSX literals so the formatjs lint stays quiet in
// this framework-level test.
const PROTECT = "protect";
const GO_ELSEWHERE = "go elsewhere";
const LEAVE = "leave";
const STAY = "stay";
const ELSEWHERE_HEADING = "Elsewhere";

// Replace GatewayPage with a controllable stub so the test exercises only the
// route-level navigation blocker wiring. The stub owns a leave guard exactly as
// the real service-account dialog does and surfaces it through
// onLeaveGuardChange, plus a link so an in-app navigation can be attempted.
vi.mock(
  "@openshift-online/hypershell-gateway-management-ui",
  async (importOriginal) => {
    const actual = await importOriginal<typeof GatewayUi>();
    function StubGatewayPage({
      onLeaveGuardChange,
    }: {
      onLeaveGuardChange?: (guard: ServiceAccountLeaveGuard | null) => void;
    }) {
      const [blocking, setBlocking] = useState(false);
      const [pending, setPending] =
        useState<ServiceAccountLeaveDecision | null>(null);
      const blockingRef = useRef(false);
      blockingRef.current = blocking;
      useEffect(() => {
        onLeaveGuardChange?.({
          confirmLeave: (decision) => {
            setPending(decision);
          },
          shouldBlock: () => blockingRef.current,
        });
        return () => {
          onLeaveGuardChange?.(null);
        };
      }, [onLeaveGuardChange]);
      return (
        <div>
          <button
            onClick={() => {
              setBlocking(true);
            }}
            type="button"
          >
            {PROTECT}
          </button>
          <Link to="/elsewhere">{GO_ELSEWHERE}</Link>
          {pending ? (
            <>
              <button
                onClick={() => {
                  const decision = pending;
                  setPending(null);
                  setBlocking(false);
                  decision.onConfirm();
                }}
                type="button"
              >
                {LEAVE}
              </button>
              <button
                onClick={() => {
                  const decision = pending;
                  setPending(null);
                  decision.onCancel();
                }}
                type="button"
              >
                {STAY}
              </button>
            </>
          ) : null}
        </div>
      );
    }
    return { ...actual, GatewayPage: StubGatewayPage };
  },
);

function renderRoute(
  initialEntries = ["/gateways/gateway-a"],
  initialIndex = 0,
) {
  const router = createMemoryRouter(
    [
      { element: <GatewayRoute />, path: "/gateways/:gatewayId" },
      { element: <h1>{ELSEWHERE_HEADING}</h1>, path: "/elsewhere" },
    ],
    { initialEntries, initialIndex },
  );
  render(<RouterProvider router={router} />);
  return router;
}

describe("GatewayRoute navigation blocker", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("navigates freely when nothing is protected", async () => {
    const user = userEvent.setup();
    renderRoute();

    await user.click(screen.getByRole("link", { name: GO_ELSEWHERE }));

    expect(
      screen.getByRole("heading", { name: ELSEWHERE_HEADING }),
    ).toBeTruthy();
  });

  it("blocks an in-app navigation and stays when the user cancels", async () => {
    const user = userEvent.setup();
    renderRoute();

    await user.click(screen.getByRole("button", { name: PROTECT }));
    await user.click(screen.getByRole("link", { name: GO_ELSEWHERE }));

    // Confirmation surfaced instead of navigating away.
    await user.click(screen.getByRole("button", { name: STAY }));

    expect(
      screen.queryByRole("heading", { name: ELSEWHERE_HEADING }),
    ).toBeNull();
    expect(screen.getByRole("button", { name: PROTECT })).toBeTruthy();
  });

  it("blocks an in-app navigation and proceeds when the user confirms", async () => {
    const user = userEvent.setup();
    renderRoute();

    await user.click(screen.getByRole("button", { name: PROTECT }));
    await user.click(screen.getByRole("link", { name: GO_ELSEWHERE }));
    await user.click(screen.getByRole("button", { name: LEAVE }));

    expect(
      screen.getByRole("heading", { name: ELSEWHERE_HEADING }),
    ).toBeTruthy();
  });

  it("blocks a Back/Forward (POP) navigation until confirmed", async () => {
    const user = userEvent.setup();
    // Start on /elsewhere, then forward-navigate onto the gateway route so a
    // Back navigation (history POP) is available to block.
    const router = renderRoute(["/elsewhere", "/gateways/gateway-a"], 1);

    await user.click(screen.getByRole("button", { name: PROTECT }));
    await act(async () => {
      await router.navigate(-1);
    });

    // POP was intercepted; the guard's confirmation is shown.
    await user.click(await screen.findByRole("button", { name: STAY }));
    expect(
      screen.queryByRole("heading", { name: ELSEWHERE_HEADING }),
    ).toBeNull();

    await act(async () => {
      await router.navigate(-1);
    });
    await user.click(await screen.findByRole("button", { name: LEAVE }));
    expect(
      screen.getByRole("heading", { name: ELSEWHERE_HEADING }),
    ).toBeTruthy();
  });
});
