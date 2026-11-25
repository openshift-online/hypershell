/**
 * Builds the BFF sign-in URL that returns the reader to the given path and
 * query after sign-in (WEB-UDOC-03, WEB-TERM-01). The BFF sanitizes
 * `return_to` to a same-origin path before it stores it.
 */
export function signInHref(pathAndQuery: string): string {
  return `/auth/login?return_to=${encodeURIComponent(pathAndQuery)}`;
}

/** Path and query of a router location, used as the sign-in `return_to`. */
export function currentPathAndQuery(location: {
  pathname: string;
  search: string;
}): string {
  return `${location.pathname}${location.search}`;
}
