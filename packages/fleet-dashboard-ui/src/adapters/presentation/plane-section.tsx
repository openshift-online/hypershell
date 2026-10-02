// Wraps one plane's query in a Card, handling loading / error / stale uniformly
// so a single failing source degrades on its own without blanking the page.

import {
  Alert,
  Card,
  CardBody,
  CardTitle,
  Spinner,
} from "@patternfly/react-core";
import type { UseQueryResult } from "@tanstack/react-query";
import type { MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";

import type { Plane } from "../../domain/plane";
import { messages } from "../../messages";

interface PlaneSectionProps<T> {
  readonly titleMessage: MessageDescriptor;
  readonly query: UseQueryResult<Plane<T>>;
  readonly children: (data: T) => React.ReactNode;
}

export function PlaneSection<T>({
  titleMessage,
  query,
  children,
}: PlaneSectionProps<T>): React.ReactElement {
  const intl = useIntl();
  return (
    <Card aria-label={intl.formatMessage(titleMessage)}>
      <CardTitle>
        <FormattedMessage {...titleMessage} />
      </CardTitle>
      <CardBody>
        <PlaneBody query={query}>{children}</PlaneBody>
      </CardBody>
    </Card>
  );
}

function PlaneBody<T>({
  query,
  children,
}: Pick<PlaneSectionProps<T>, "query" | "children">): React.ReactElement {
  if (query.isPending) {
    return (
      <>
        <Spinner size="md" aria-label="" />{" "}
        <FormattedMessage {...messages.loadingPlane} />
      </>
    );
  }
  if (query.isError) {
    return (
      <Alert
        variant="danger"
        isInline
        title={<FormattedMessage {...messages.errorPlane} />}
      />
    );
  }
  const plane = query.data;
  return (
    <>
      {plane.stale ? (
        <Alert
          variant="warning"
          isInline
          isPlain
          title={<FormattedMessage {...messages.freshnessStale} />}
        />
      ) : null}
      {children(plane.data)}
    </>
  );
}
