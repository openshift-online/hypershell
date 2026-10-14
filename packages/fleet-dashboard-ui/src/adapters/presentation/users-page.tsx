// Fleet users view: summary cards (distinct users, owners, gateways, sandboxes)
// above a sortable per-user table. All user identity arrives from the
// /api/users plane; nothing here knows any username. Counts are rendered
// locale-aware via intl.formatNumber.

import { useState } from "react";
import {
  Card,
  CardBody,
  CardTitle,
  Gallery,
  GalleryItem,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { FormattedMessage, useIntl } from "react-intl";
import type { MessageDescriptor } from "react-intl";
import {
  sortUserUsage,
  userUsageTotals,
  type UserSortKey,
  type UserUsage,
} from "../../domain/users";
import { messages } from "../../messages";
import { PlaneSection } from "./plane-section";
import { useUsers } from "../query/hooks";

/** Column order of the usage table; index maps to the sort key. */
const SORT_KEYS: readonly UserSortKey[] = ["username", "gateways", "sandboxes"];

type SortDirection = "asc" | "desc";

export function UsersPage(): React.ReactElement {
  const users = useUsers();
  return (
    <PlaneSection titleMessage={messages.sectionUsers} query={users}>
      {(data) => <UsersUsageView users={data.users} />}
    </PlaneSection>
  );
}

function UsersUsageView({
  users,
}: {
  users: readonly UserUsage[];
}): React.ReactElement {
  const intl = useIntl();
  // Heaviest users first: the operational question is who holds how much.
  const [sort, setSort] = useState<{
    key: UserSortKey;
    dir: SortDirection;
  }>({ key: "gateways", dir: "desc" });

  const totals = userUsageTotals(users);
  const rows = sortUserUsage(users, sort.key, sort.dir);
  const activeIndex = SORT_KEYS.indexOf(sort.key);
  const number = (value: number): string => intl.formatNumber(value);
  const none = intl.formatMessage(messages.valueNone);

  const cards: {
    readonly key: string;
    readonly label: MessageDescriptor;
    readonly value: number;
  }[] = [
    { key: "users", label: messages.usersCardUsers, value: totals.users },
    { key: "owners", label: messages.usersCardOwners, value: totals.owners },
    {
      key: "gateways",
      label: messages.usersCardGateways,
      value: totals.gateways,
    },
    {
      key: "sandboxes",
      label: messages.usersCardSandboxes,
      value: totals.sandboxes,
    },
  ];

  return (
    <>
      <Gallery hasGutter minWidths={{ default: "220px" }}>
        {cards.map((card) => (
          <GalleryItem key={card.key}>
            <Card isCompact>
              <CardTitle>
                <FormattedMessage {...card.label} />
              </CardTitle>
              <CardBody>
                <span style={{ fontSize: "1.75rem", fontWeight: 600 }}>
                  {number(card.value)}
                </span>
              </CardBody>
            </Card>
          </GalleryItem>
        ))}
      </Gallery>
      <Table
        variant="compact"
        aria-label={intl.formatMessage(messages.sectionUsers)}
      >
        <Thead>
          <Tr>
            <Th
              sort={{
                columnIndex: 0,
                sortBy: { index: activeIndex, direction: sort.dir },
                onSort: (_event, _index, direction) => {
                  setSort({ key: "username", dir: direction });
                },
              }}
            >
              <FormattedMessage {...messages.columnUsername} />
            </Th>
            <Th
              sort={{
                columnIndex: 1,
                sortBy: { index: activeIndex, direction: sort.dir },
                onSort: (_event, _index, direction) => {
                  setSort({ key: "gateways", dir: direction });
                },
              }}
            >
              <FormattedMessage {...messages.columnUserGateways} />
            </Th>
            <Th
              sort={{
                columnIndex: 2,
                sortBy: { index: activeIndex, direction: sort.dir },
                onSort: (_event, _index, direction) => {
                  setSort({ key: "sandboxes", dir: direction });
                },
              }}
            >
              <FormattedMessage {...messages.columnUserSandboxes} />
            </Th>
            {/* Not sortable: a descriptive set, not a count; the default
                gateways-desc ordering already fronts the interesting rows. */}
            <Th>
              <FormattedMessage {...messages.columnSpokeClusters} />
            </Th>
          </Tr>
        </Thead>
        <Tbody>
          {rows.map((user) => (
            <Tr key={user.username}>
              <Td dataLabel={intl.formatMessage(messages.columnUsername)}>
                {user.username}
              </Td>
              <Td dataLabel={intl.formatMessage(messages.columnUserGateways)}>
                {number(user.gateways)}
              </Td>
              <Td dataLabel={intl.formatMessage(messages.columnUserSandboxes)}>
                {number(user.sandboxes)}
              </Td>
              <Td dataLabel={intl.formatMessage(messages.columnSpokeClusters)}>
                {user.clusters.length > 0 ? user.clusters.join(", ") : none}
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
      {rows.length === 0 ? (
        <p>
          <FormattedMessage {...messages.emptyPlane} />
        </p>
      ) : null}
    </>
  );
}
