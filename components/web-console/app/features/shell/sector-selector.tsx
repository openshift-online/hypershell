import {
  Button,
  MenuFooter,
  MenuToggle,
  Select,
  SelectList,
  SelectOption,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
} from "@patternfly/react-core";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, useNavigate } from "react-router";

import { messages } from "../../i18n/messages";
import { apiClient } from "../../lib/api.client";
import styles from "./application-shell.module.css";

export interface SectorOption {
  id: string;
  name: string;
}

interface SectorSelectorProps {
  hasError?: boolean;
  isLoading?: boolean;
  onSelectSector: (sectorId: string) => void;
  sectors: readonly SectorOption[];
  selectedSectorId: string;
}

const sectorQueryKeys = {
  list: () => ["sectors", "list", { page: 1, size: 100 }] as const,
};

export function getSectorDestination(pathname: string, sectorId: string) {
  const segments = pathname.split("/").filter(Boolean);
  const sectorBase = `/fleets/${encodeURIComponent(sectorId)}`;
  const section = segments[2];

  if (section === "gateways") {
    return `${sectorBase}/gateways`;
  }

  if (section === "clients" || section === "keys" || section === "settings") {
    return `${sectorBase}/${section}`;
  }

  return sectorBase;
}

/**
 * Canonical sector switcher for sector-scoped console routes. The selected
 * sector is controlled by the route; this component owns only menu state.
 */
export function SectorSelector({
  hasError = false,
  isLoading = false,
  onSelectSector,
  sectors,
  selectedSectorId,
}: SectorSelectorProps) {
  const intl = useIntl();
  const [isOpen, setIsOpen] = useState(false);
  const selectedSector = sectors.find(
    (sector) => sector.id === selectedSectorId,
  ) ?? { id: selectedSectorId, name: selectedSectorId };

  const onSelect = (
    _event: React.MouseEvent | undefined,
    value: string | number | undefined,
  ) => {
    if (typeof value !== "string") {
      return;
    }

    setIsOpen(false);
    onSelectSector(value);
  };

  const toggle = (toggleRef: React.Ref<HTMLButtonElement>) => (
    <MenuToggle
      aria-label={intl.formatMessage(messages.sectorSelectorToggle, {
        sectorName: selectedSector.name,
      })}
      isExpanded={isOpen}
      onClick={() => {
        setIsOpen((open) => !open);
      }}
      ref={toggleRef}
    >
      {selectedSector.name}
    </MenuToggle>
  );

  return (
    <Select
      id="sector-selector"
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onOpenChangeKeys={["Escape"]}
      onSelect={onSelect}
      selected={selectedSectorId}
      shouldFocusToggleOnSelect
      toggle={toggle}
    >
      <SelectList>
        {sectors.map((sector) => (
          <SelectOption key={sector.id} value={sector.id}>
            {sector.name}
          </SelectOption>
        ))}
        {isLoading ? (
          <SelectOption isDisabled value="sector-selector-loading">
            <FormattedMessage {...messages.sectorSelectorLoading} />
          </SelectOption>
        ) : null}
        {hasError ? (
          <SelectOption isDisabled value="sector-selector-error">
            <FormattedMessage {...messages.sectorSelectorError} />
          </SelectOption>
        ) : null}
      </SelectList>
      <MenuFooter>
        <Button
          component={Link}
          isInline
          onClick={() => {
            setIsOpen(false);
          }}
          variant="link"
          {...{ to: "/fleets" }}
        >
          <FormattedMessage {...messages.viewSectors} />
        </Button>
      </MenuFooter>
    </Select>
  );
}

interface SectorContextBarProps {
  pathname: string;
  selectedSectorId: string;
}

export function SectorContextBar({
  pathname,
  selectedSectorId,
}: SectorContextBarProps) {
  const intl = useIntl();
  const navigate = useNavigate();
  // Fleet is the current API/SDK transport name; the product term is Sector.
  const sectorsQuery = useQuery({
    queryKey: sectorQueryKeys.list(),
    queryFn: ({ signal }) =>
      apiClient.fleets.list({ page: 1, size: 100 }, { signal }),
  });
  const sectors = (sectorsQuery.data?.items ?? []).map(({ id, name }) => ({
    id,
    name,
  }));

  if (!sectors.some((sector) => sector.id === selectedSectorId)) {
    sectors.unshift({ id: selectedSectorId, name: selectedSectorId });
  }

  return (
    <div className={styles.sectorContextBar}>
      <Toolbar
        aria-label={intl.formatMessage(messages.sectorContextToolbar)}
        hasNoPadding
      >
        <ToolbarContent>
          <ToolbarGroup>
            <ToolbarItem>
              <span className={styles.sectorSelectorLabel}>
                <FormattedMessage {...messages.sectorSelectorLabel} />
              </span>
            </ToolbarItem>
            <ToolbarItem>
              <SectorSelector
                hasError={sectorsQuery.isError}
                isLoading={sectorsQuery.isPending}
                onSelectSector={(sectorId) => {
                  if (sectorId !== selectedSectorId) {
                    void navigate(getSectorDestination(pathname, sectorId));
                  }
                }}
                sectors={sectors}
                selectedSectorId={selectedSectorId}
              />
            </ToolbarItem>
          </ToolbarGroup>
        </ToolbarContent>
      </Toolbar>
    </div>
  );
}
