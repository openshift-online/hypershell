/**
 * Reader context a refusal link may carry (WEB-UDOC-09). A value is shown only
 * when it is 1 to 128 ASCII letters, digits, `.`, `_`, or `-`; any other value
 * is ignored, so the page never shows markup or arbitrary text from a URL.
 */
export interface RefusalContext {
  readonly gateway?: string;
  readonly policy?: string;
}

const safeRefusalValue = /^[A-Za-z0-9._-]{1,128}$/u;

export function safeRefusalContextValue(
  value: string | null | undefined,
): string | undefined {
  return typeof value === "string" && safeRefusalValue.test(value)
    ? value
    : undefined;
}

export function readRefusalContext(
  params: Pick<URLSearchParams, "get">,
): RefusalContext {
  const policy = safeRefusalContextValue(params.get("policy"));
  const gateway = safeRefusalContextValue(params.get("gateway"));
  return {
    ...(policy === undefined ? {} : { policy }),
    ...(gateway === undefined ? {} : { gateway }),
  };
}
