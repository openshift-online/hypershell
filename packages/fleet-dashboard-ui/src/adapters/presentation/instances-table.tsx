// Managed instances as an accessible table. Instance/role/provider/region are all
// data returned by the server; nothing about the fleet is baked into this file.

import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { FormattedMessage, useIntl } from "react-intl";

import { healthBadge } from "../../domain/status";
import type { InstancesData } from "../../application/ports";
import { messages } from "../../messages";
import { StatusLabel } from "./status-label";

export function InstancesTable({
  instances,
}: {
  instances: InstancesData;
}): React.ReactElement {
  const intl = useIntl();
  const none = intl.formatMessage(messages.valueNone);

  return (
    <Table
      variant="compact"
      aria-label={intl.formatMessage(messages.sectionInstances)}
    >
      <Thead>
        <Tr>
          <Th>
            <FormattedMessage {...messages.columnInstance} />
          </Th>
          <Th>
            <FormattedMessage {...messages.columnRole} />
          </Th>
          <Th>
            <FormattedMessage {...messages.columnProvider} />
          </Th>
          <Th>
            <FormattedMessage {...messages.columnRegion} />
          </Th>
          <Th>
            <FormattedMessage {...messages.columnHealth} />
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {instances.instances.map((instance) => (
          <Tr key={instance.name}>
            <Td dataLabel={intl.formatMessage(messages.columnInstance)}>
              {instance.name}
            </Td>
            <Td dataLabel={intl.formatMessage(messages.columnRole)}>
              {instance.role ?? none}
            </Td>
            <Td dataLabel={intl.formatMessage(messages.columnProvider)}>
              {instance.provider ?? none}
            </Td>
            <Td dataLabel={intl.formatMessage(messages.columnRegion)}>
              {instance.region ?? none}
            </Td>
            <Td dataLabel={intl.formatMessage(messages.columnHealth)}>
              <StatusLabel badge={healthBadge(instance.health)} />
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}
