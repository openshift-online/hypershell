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
  type LinksFunction,
} from "react-router";

import favicon16 from "../../../images/brand/favicon-16.png";
import favicon32 from "../../../images/brand/favicon-32.png";
import favicon48 from "../../../images/brand/favicon-48.png";
import appleTouchIcon from "../../../images/brand/favicon-180.png";
import { englishMessages } from "./i18n/catalog";
import { messages } from "./i18n/messages";

export const links: LinksFunction = () => [
  { rel: "icon", type: "image/png", sizes: "16x16", href: favicon16 },
  { rel: "icon", type: "image/png", sizes: "32x32", href: favicon32 },
  { rel: "icon", type: "image/png", sizes: "48x48", href: favicon48 },
  { rel: "apple-touch-icon", sizes: "180x180", href: appleTouchIcon },
];

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
