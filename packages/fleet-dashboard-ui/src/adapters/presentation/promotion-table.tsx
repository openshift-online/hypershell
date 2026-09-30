// Promotion path as an accessible table. Column order and every value come from
// the payload via the domain helpers - no environment names or hub mapping here.

import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { FormattedMessage, useIntl } from "react-intl";

import type { PromotionData } from "../../domain/promotion";
import {
  environmentGateBadge,
  governingInstance,
  hasPendingPromotion,
  orderedEnvironments,
} from "../../domain/promotion";
import { messages } from "../../messages";
import { StatusLabel } from "./status-label";

export function PromotionTable({
  promotion,
}: {
  promotion: PromotionData;
}): React.ReactElement {
  const intl = useIntl();
  const environments = orderedEnvironments(promotion);
  const none = intl.formatMessage(messages.valueNone);

  return (
    <Table
      variant="compact"
      aria-label={intl.formatMessage(messages.sectionPromotion)}
    >
      <Thead>
        <Tr>
          <Th>
            <FormattedMessage {...messages.columnEnvironment} />
          </Th>
          <Th>
            <FormattedMessage {...messages.columnRelease} />
          </Th>
          <Th>
            <FormattedMessage {...messages.columnGoverning} />
          </Th>
          <Th>
            <FormattedMessage {...messages.columnGates} />
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {environments.map((env) => (
          <Tr key={env.name}>
            <Td dataLabel={intl.formatMessage(messages.columnEnvironment)}>
              {env.name}
            </Td>
            <Td dataLabel={intl.formatMessage(messages.columnRelease)}>
              {env.activeRelease ?? none}
              {hasPendingPromotion(env) ? (
                <>
                  {" "}
                  <FormattedMessage {...messages.pendingPromotion} />
                </>
              ) : null}
            </Td>
            <Td dataLabel={intl.formatMessage(messages.columnGoverning)}>
              {governingInstance(env) ?? none}
            </Td>
            <Td dataLabel={intl.formatMessage(messages.columnGates)}>
              <StatusLabel badge={environmentGateBadge(env)} />
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}
