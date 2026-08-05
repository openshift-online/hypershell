import "@patternfly/react-core/dist/styles/base.css";

import { Alert } from "@patternfly/react-core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider, FormattedMessage } from "react-intl";
import {
  isRouteErrorResponse,
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
  useRouteError,
} from "react-router";

import { englishMessages } from "./i18n/catalog";
import { messages } from "./i18n/messages";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: false,
      staleTime: 30_000,
    },
    mutations: {
      retry: false,
    },
  },
});

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" dir="ltr">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <Meta />
        <Links />
      </head>
      <body>
        <IntlProvider locale="en" messages={englishMessages}>
          <QueryClientProvider client={queryClient}>
            {children}
          </QueryClientProvider>
        </IntlProvider>
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function App() {
  return <Outlet />;
}

export function ErrorBoundary() {
  const error = useRouteError();
  const isNotFound = isRouteErrorResponse(error) && error.status === 404;

  return (
    <main id="main-content">
      <Alert
        isInline
        variant="danger"
        title={
          <FormattedMessage
            {...(isNotFound ? messages.notFoundTitle : messages.errorTitle)}
          />
        }
      >
        <FormattedMessage
          {...(isNotFound ? messages.notFoundBody : messages.errorBody)}
        />
      </Alert>
    </main>
  );
}
