import type { TerminalSessionCheck } from "../../application/terminal/terminal-operations";
import type { SessionGateway } from "../session/session-adapter";

/**
 * Re-reads the BFF session resource after a failed terminal command
 * (WEB-TERM-10). The session has expired when the console uses sign-in and
 * the resource no longer reports an authenticated session. The shared session
 * query cache is not changed, so the page stays where it is.
 */
export function createTerminalSessionCheck(
  sessions: SessionGateway,
): TerminalSessionCheck {
  return {
    async isSessionExpired() {
      const session = await sessions.getSession();
      return session.authEnabled && !session.authenticated;
    },
  };
}
