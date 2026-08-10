import {
  Alert,
  AlertActionLink,
  Button,
  FormGroup,
  FormHelperText,
  HelperText,
  HelperTextItem,
  MenuToggle,
  Select,
  SelectList,
  SelectOption,
  TextInputGroup,
  TextInputGroupMain,
  TextInputGroupUtilities,
  type MenuToggleElement,
} from "@patternfly/react-core";
import RhMicronsCloseIcon from "@patternfly/react-icons/dist/esm/icons/rh-microns-close-icon";
import { useQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";

import type { GatewayPlacement } from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { gatewayPlacementQueryKey } from "./gateway-data";

const loadingValue = Symbol("loading placements");
const noResultsValue = Symbol("no placement results");

interface PlacementOption {
  description?: string;
  isDisabled?: boolean;
  label: string;
  value: string | symbol;
}

export interface GatewayPlacementSelectProps {
  error?: string;
  isDisabled: boolean;
  onChange: (clusterId: string | null) => void;
  value: string | null;
}

function placementDescription(
  placement: GatewayPlacement,
  formatMessage: ReturnType<typeof useIntl>["formatMessage"],
) {
  const provider = placement.provider.trim();
  const region = placement.region?.trim() ?? "";
  if (provider && region) {
    return formatMessage(messages.clusterProviderAndRegion, {
      provider,
      region,
    });
  }
  if (provider) {
    return formatMessage(messages.clusterProvider, { provider });
  }
  if (region) {
    return formatMessage(messages.clusterRegion, { region });
  }
  return undefined;
}

export function GatewayPlacementSelect({
  error,
  isDisabled,
  onChange,
  value,
}: GatewayPlacementSelectProps) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const hubLabel = intl.formatMessage(messages.hubClusterDefault);
  const [isOpen, setIsOpen] = useState(false);
  const [inputValue, setInputValue] = useState(hubLabel);
  const [searchValue, setSearchValue] = useState("");
  const [focusedItemIndex, setFocusedItemIndex] = useState<number | null>(null);
  const [activeItemId, setActiveItemId] = useState<string>();
  const inputRef = useRef<HTMLInputElement>(null);
  const normalizedSearch = searchValue.trim();
  const placementQuery = useQuery({
    queryFn: ({ signal }) =>
      gateways.findGatewayPlacements(normalizedSearch, signal),
    queryKey: gatewayPlacementQueryKey(normalizedSearch),
  });
  const hubMatches = hubLabel
    .toLocaleLowerCase(intl.locale)
    .includes(normalizedSearch.toLocaleLowerCase(intl.locale));
  const options: PlacementOption[] = [
    ...(hubMatches
      ? [{ label: hubLabel, value: "" } satisfies PlacementOption]
      : []),
    ...(placementQuery.data?.items.map((placement) => ({
      description: placementDescription(placement, intl.formatMessage),
      label: placement.name,
      value: placement.id,
    })) ?? []),
  ];

  if (placementQuery.isPending) {
    options.push({
      isDisabled: true,
      label: intl.formatMessage(messages.loadingClusters),
      value: loadingValue,
    });
  } else if (options.length === 0) {
    options.push({
      isDisabled: true,
      label: intl.formatMessage(messages.noMatchingClusters),
      value: noResultsValue,
    });
  }

  const resetFocus = () => {
    setFocusedItemIndex(null);
    setActiveItemId(undefined);
  };
  const closeMenu = () => {
    setIsOpen(false);
    resetFocus();
  };
  const chooseOption = (option: PlacementOption) => {
    if (typeof option.value !== "string" || option.isDisabled) {
      return;
    }
    onChange(option.value);
    setInputValue(option.label);
    setSearchValue("");
    closeMenu();
  };
  const focusOption = (index: number) => {
    setFocusedItemIndex(index);
    setActiveItemId(`gateway-cluster-option-${String(index)}`);
  };
  const moveFocus = (direction: -1 | 1) => {
    if (!isOpen) {
      setIsOpen(true);
    }
    if (options.every(({ isDisabled: optionDisabled }) => optionDisabled)) {
      return;
    }

    let nextIndex = focusedItemIndex ?? (direction === 1 ? -1 : 0);
    do {
      nextIndex = (nextIndex + direction + options.length) % options.length;
    } while (options[nextIndex]?.isDisabled);
    focusOption(nextIndex);
  };
  const toggle = (toggleRef: React.Ref<MenuToggleElement>) => (
    <MenuToggle
      aria-label={intl.formatMessage(messages.selectCluster)}
      isDisabled={isDisabled}
      isExpanded={isOpen}
      isFullWidth
      onClick={() => {
        setIsOpen((open) => !open);
        inputRef.current?.focus();
      }}
      ref={toggleRef}
      status={error ? "danger" : undefined}
      variant="typeahead"
    >
      <TextInputGroup
        isDisabled={isDisabled}
        isPlain
        validated={error ? "error" : "default"}
      >
        <TextInputGroupMain
          {...(activeItemId ? { "aria-activedescendant": activeItemId } : {})}
          aria-label={intl.formatMessage(messages.cluster)}
          aria-controls="gateway-cluster-listbox"
          innerRef={inputRef}
          inputId="gateway-cluster"
          inputProps={{
            "aria-autocomplete": "list",
            "aria-busy": placementQuery.isFetching,
            ...(error ? { "aria-describedby": "gateway-cluster-helper" } : {}),
            autoComplete: "off",
            required: true,
          }}
          isExpanded={isOpen}
          onChange={(_event, nextValue) => {
            setInputValue(nextValue);
            setSearchValue(nextValue);
            onChange(null);
            resetFocus();
            setIsOpen(true);
          }}
          onClick={() => {
            setIsOpen(true);
          }}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              event.preventDefault();
              moveFocus(event.key === "ArrowDown" ? 1 : -1);
            } else if (event.key === "Enter") {
              if (isOpen && focusedItemIndex !== null) {
                event.preventDefault();
                const focusedOption = options[focusedItemIndex];
                if (focusedOption) {
                  chooseOption(focusedOption);
                }
              } else if (!isOpen) {
                event.preventDefault();
                setIsOpen(true);
              }
            }
          }}
          placeholder={intl.formatMessage(messages.selectCluster)}
          role="combobox"
          value={inputValue}
        />
        <TextInputGroupUtilities
          {...(!inputValue ? { style: { display: "none" } } : {})}
        >
          <Button
            aria-label={intl.formatMessage(messages.clearClusterSearch)}
            icon={<RhMicronsCloseIcon />}
            onClick={() => {
              setInputValue("");
              setSearchValue("");
              onChange(null);
              resetFocus();
              inputRef.current?.focus();
            }}
            variant="plain"
          />
        </TextInputGroupUtilities>
      </TextInputGroup>
    </MenuToggle>
  );

  return (
    <FormGroup
      fieldId="gateway-cluster"
      isRequired
      label={intl.formatMessage(messages.cluster)}
    >
      <Select
        id="gateway-cluster-select"
        isOpen={isOpen}
        onOpenChange={(open) => {
          if (!open) {
            closeMenu();
          }
        }}
        onSelect={(_event, selectedValue) => {
          const option = options.find(
            ({ value: optionValue }) => optionValue === selectedValue,
          );
          if (option) {
            chooseOption(option);
          }
        }}
        selected={value ?? undefined}
        toggle={toggle}
        variant="typeahead"
      >
        <SelectList id="gateway-cluster-listbox">
          {options.map((option, index) => (
            <SelectOption
              description={option.description}
              id={`gateway-cluster-option-${String(index)}`}
              isAriaDisabled={option.isDisabled}
              isFocused={focusedItemIndex === index}
              isSelected={
                typeof option.value === "string" && value === option.value
              }
              key={
                typeof option.value === "string"
                  ? option.value
                  : option.value.description
              }
              value={option.value}
            >
              {option.label}
            </SelectOption>
          ))}
        </SelectList>
      </Select>
      {error ? (
        <FormHelperText>
          <HelperText>
            <HelperTextItem
              id="gateway-cluster-helper"
              screenReaderText={intl.formatMessage(messages.error)}
              variant="error"
            >
              {error}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      ) : null}
      {placementQuery.isFetching ? (
        <FormHelperText>
          <HelperText>
            <HelperTextItem role="status" variant="indeterminate">
              {intl.formatMessage(messages.loadingClusters)}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      ) : null}
      {placementQuery.data?.hasMore && !placementQuery.isFetching ? (
        <FormHelperText>
          <HelperText>
            <HelperTextItem>
              {intl.formatMessage(messages.moreClustersAvailable)}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      ) : null}
      {placementQuery.isError ? (
        <Alert
          actionLinks={
            <AlertActionLink onClick={() => void placementQuery.refetch()}>
              {intl.formatMessage(messages.retry)}
            </AlertActionLink>
          }
          isInline
          isLiveRegion
          title={intl.formatMessage(messages.clusterLoadError)}
          variant="warning"
        >
          {intl.formatMessage(messages.clusterLoadErrorBody)}
        </Alert>
      ) : null}
    </FormGroup>
  );
}
