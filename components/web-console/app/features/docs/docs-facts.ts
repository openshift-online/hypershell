import { useEffect, useRef } from "react";

import { docsFacts } from "../../composition/docs-composition";
import type { DocsRouteTemplate } from "../../application/docs/docs-probes";
import type { DocsSessionState } from "./use-docs-session";

/**
 * Publishes `docs.page.viewed` once per page visit, after the session state is
 * known, with the route template only (WEB-UDOC-12).
 */
export function useDocsPageViewed(
  route: DocsRouteTemplate,
  session: DocsSessionState,
): void {
  const published = useRef<DocsRouteTemplate | undefined>(undefined);
  useEffect(() => {
    if (session === "loading" || published.current === route) {
      return;
    }
    published.current = route;
    docsFacts.pageViewed(route, session);
  }, [route, session]);
}

/** Publishes `docs.sign-in-prompt.shown` once while a prompt is visible. */
export function useDocsSignInPromptShown(
  route: DocsRouteTemplate,
  visible: boolean,
): void {
  const published = useRef(false);
  useEffect(() => {
    if (!visible || published.current) {
      return;
    }
    published.current = true;
    docsFacts.signInPromptShown(route);
  }, [route, visible]);
}
