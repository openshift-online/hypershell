import "@patternfly/react-core/dist/styles/base.css";
import "./ui/spec-viewer.css";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { IntlProvider } from "react-intl";

import { httpRepository } from "./adapters/http-repository";
import { httpSpecSource } from "./adapters/http-spec-source";
import { createLocalPinStore } from "./adapters/local-pin-store";
import { markedMarkdown } from "./adapters/marked-markdown";
import { shikiHighlighter } from "./adapters/shiki-highlighter";
import type { SpecViewerPorts } from "./application/ports";
import { App } from "./ui/App";
import { PortsProvider } from "./ui/ports-context";

// Pins live in localStorage, which is per origin. Serve one canonical host
// so pins saved at 127.0.0.1 also show up when the page is opened as localhost.
if (window.location.hostname === "localhost") {
  const canonical = new URL(window.location.href);
  canonical.hostname = "127.0.0.1";
  window.location.replace(canonical);
}

// Composition root: the only place that binds ports to adapters.
const ports: SpecViewerPorts = {
  specs: httpSpecSource,
  markdown: markedMarkdown,
  pins: createLocalPinStore(window.localStorage),
  repository: httpRepository,
  highlighter: shikiHighlighter,
};

const darkScheme = window.matchMedia("(prefers-color-scheme: dark)");
const applyScheme = () => {
  document.documentElement.classList.toggle(
    "pf-v6-theme-dark",
    darkScheme.matches,
  );
};
applyScheme();
darkScheme.addEventListener("change", applyScheme);

const root = document.getElementById("root");
if (!root) {
  throw new Error("Missing #root element");
}

createRoot(root).render(
  <StrictMode>
    <IntlProvider defaultLocale="en" locale="en">
      <QueryClientProvider client={new QueryClient()}>
        <PortsProvider ports={ports}>
          <App />
        </PortsProvider>
      </QueryClientProvider>
    </IntlProvider>
  </StrictMode>,
);
