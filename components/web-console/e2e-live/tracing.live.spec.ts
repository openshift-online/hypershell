import { expect, request as playwrightRequest, test } from "@playwright/test";

// End-to-end trace verification against a live cluster (WEB-TRACE-10). Drives a
// real gateway workflow in the browser, then queries Jaeger and asserts one
// trace joins the browser and the BFF by the same trace id -- proving the
// browser OTel SDK, the same-origin telemetry ingest, the BFF server span, and
// W3C context propagation all work together in a deployed environment.

const jaegerUrl =
  process.env.E2E_JAEGER_URL ?? "https://jaeger.hypershell.localhost";
const oidcUser = process.env.E2E_OIDC_USERNAME ?? "admin";
const oidcPassword = process.env.E2E_OIDC_PASSWORD ?? "admin";

const browserService = "hypershell-web-console";
const bffService = "hypershell-web-console-bff";
// Span names must come from the bounded workflow/dependency templates, never a
// raw identifier (WEB-TRACE-07).
const boundedSpanName = /^gateway\.(workflow|dependency)\.[a-z-]+$/u;

interface JaegerSpan {
  readonly traceID: string;
  readonly spanID: string;
  readonly operationName: string;
  readonly processID: string;
}

interface JaegerProcess {
  readonly serviceName: string;
}

interface JaegerTrace {
  readonly traceID: string;
  readonly spans: readonly JaegerSpan[];
  readonly processes: Readonly<Record<string, JaegerProcess>>;
}

interface JaegerTracesResponse {
  readonly data: readonly JaegerTrace[];
}

function serviceOf(trace: JaegerTrace, span: JaegerSpan): string | undefined {
  return trace.processes[span.processID]?.serviceName;
}

// A cross-service trace carries at least one bounded browser span and at least
// one BFF span under the same trace id -- the join that proves propagation.
function isCrossServiceTrace(trace: JaegerTrace): boolean {
  let hasBrowserWorkflow = false;
  let hasBffSpan = false;
  for (const span of trace.spans) {
    const service = serviceOf(trace, span);
    if (
      service === browserService &&
      boundedSpanName.test(span.operationName)
    ) {
      hasBrowserWorkflow = true;
    }
    if (service === bffService) {
      hasBffSpan = true;
    }
  }
  return hasBrowserWorkflow && hasBffSpan;
}

test("browser and BFF spans join one trace in Jaeger", async ({ page }) => {
  // 1. Log in through Keycloak and land on the gateway list. Loading the list
  //    is a gateway "list" workflow, which drives an /api proxy call through the
  //    BFF and produces both a browser workflow span and a BFF server span.
  await page.goto("/", { waitUntil: "domcontentloaded" });
  await page.fill("#username", oidcUser);
  await page.fill("#password", oidcPassword);
  await Promise.all([
    page.waitForURL(/console\.hypershell\.localhost/u),
    page.click("#kc-login, input[type=submit], button[type=submit]"),
  ]);
  await expect(page.locator("h1").first()).toBeVisible();

  // 2. Exercise an explicit refresh to emit another list workflow, then let the
  //    batch span processor timer flush; closing the page fires the pagehide
  //    flush as a belt-and-braces last-chance export (WEB-TRACE-09).
  const refresh = page.getByRole("button", { name: /refresh gateways/iu });
  if (await refresh.count()) {
    await refresh.first().click();
  }
  await page.waitForTimeout(6_000);
  await page.close();

  // 3. Poll Jaeger for a trace that spans both services. Export is best-effort
  //    and asynchronous, so retry within a bounded window before failing.
  const api = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
  let crossServiceTrace: JaegerTrace | undefined;
  await expect
    .poll(
      async () => {
        const response = await api.get(
          `${jaegerUrl}/api/traces?service=${bffService}&lookback=1h&limit=20`,
        );
        if (!response.ok()) {
          return 0;
        }
        const body = (await response.json()) as JaegerTracesResponse;
        crossServiceTrace = body.data.find(isCrossServiceTrace);
        return crossServiceTrace === undefined ? 0 : 1;
      },
      {
        message: `no cross-service trace (${browserService} + ${bffService}) reached Jaeger`,
        timeout: 60_000,
        intervals: [2_000],
      },
    )
    .toBe(1);
  await api.dispose();

  // 4. Confirm the join concretely: a bounded browser span and a BFF span share
  //    one trace id.
  const trace = crossServiceTrace;
  expect(trace).toBeDefined();
  if (trace === undefined) {
    return;
  }
  const browserSpans = trace.spans.filter(
    (span) =>
      serviceOf(trace, span) === browserService &&
      boundedSpanName.test(span.operationName),
  );
  const bffSpans = trace.spans.filter(
    (span) => serviceOf(trace, span) === bffService,
  );
  expect(browserSpans.length).toBeGreaterThan(0);
  expect(bffSpans.length).toBeGreaterThan(0);
  for (const span of [...browserSpans, ...bffSpans]) {
    expect(span.traceID).toBe(trace.traceID);
  }
});
