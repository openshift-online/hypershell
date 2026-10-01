// Composition root. The only module that constructs concrete adapters and wires
// providers together. The BFF base path is relative ("/api") - the dashboard is
// served same-origin by the Go binary, so no host is ever hard-coded.

import "@patternfly/react-core/dist/styles/base.css";

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";

import { createHttpFleetApi } from "./adapters/api/http-fleet-api";
import { App } from "./adapters/presentation/app";
import { FleetApiProvider } from "./adapters/query/api-context";
import { resolveLocale } from "./composition/intl";
import { createQueryClient } from "./composition/query-client";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("Root element #root not found");
}

const api = createHttpFleetApi({ basePath: "/api" });
const queryClient = createQueryClient();
const locale = resolveLocale();

createRoot(rootElement).render(
  <StrictMode>
    <IntlProvider locale={locale} defaultLocale="en">
      <QueryClientProvider client={queryClient}>
        <FleetApiProvider value={api}>
          <App />
        </FleetApiProvider>
      </QueryClientProvider>
    </IntlProvider>
  </StrictMode>,
);
