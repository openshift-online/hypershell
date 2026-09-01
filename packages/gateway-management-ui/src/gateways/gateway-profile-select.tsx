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
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";

import { useGatewayUi } from "../gateway-ui-provider";
import { gatewayProfileMessages } from "../gateway-profile-messages";
import { useDebouncedValue } from "../shared/use-debounced-value";
import {
  gatewayPlacementStaleMilliseconds,
  gatewayProfileOptionsQueryKey,
  gatewaySearchDebounceMilliseconds,
} from "./gateway-data";

const loadingValue = Symbol();
const noResultsValue = Symbol();

interface ProfileOption {
  description?: string;
  isDisabled?: boolean;
  key: string;
  label: string;
  value: string | symbol;
}

export interface GatewayProfileSelectProps {
  autoSelect?: boolean;
  error?: string;
  isDisabled: boolean;
  onChange: (profileId: string | null) => void;
  value: string | null;
}

export function GatewayProfileSelect({
  autoSelect = true,
  error,
  isDisabled,
  onChange,
  value,
}: GatewayProfileSelectProps) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const [isOpen, setIsOpen] = useState(false);
  const [searchValue, setSearchValue] = useState("");
  const [focusedItemIndex, setFocusedItemIndex] = useState<number | null>(null);
  const [activeItemId, setActiveItemId] = useState<string>();
  const inputRef = useRef<HTMLInputElement>(null);
  const lastAcceptedSelectionRef = useRef({ label: "", value: "" });
  const hasAutoSelected = useRef(false);
  const onChangeRef = useRef(onChange);
  useLayoutEffect(() => {
    onChangeRef.current = onChange;
  });
  const normalizedSearch = searchValue.trim();
  const debouncedSearch = useDebouncedValue(
    normalizedSearch,
    gatewaySearchDebounceMilliseconds,
  );
  const isSearchPending = normalizedSearch !== debouncedSearch;
  const profileQuery = useQuery({
    queryFn: ({ signal }) =>
      gateways.findGatewayProfiles(debouncedSearch, signal),
    queryKey: gatewayProfileOptionsQueryKey(debouncedSearch),
    staleTime: gatewayPlacementStaleMilliseconds,
  });
  const options: ProfileOption[] = [
    ...(!isSearchPending
      ? (profileQuery.data?.items.map((profile) => ({
          description: profile.description,
          key: `profile:${profile.id}`,
          label: profile.name,
          value: profile.id,
        })) ?? [])
      : []),
  ];
  const selectedProfileName =
    value !== null
      ? profileQuery.data?.items.find((p) => p.id === value)?.name
      : null;
  const displayInputValue =
    searchValue !== ""
      ? searchValue
      : (selectedProfileName ?? lastAcceptedSelectionRef.current.label);

  useEffect(() => {
    if (
      !autoSelect ||
      hasAutoSelected.current ||
      value !== null ||
      debouncedSearch !== ""
    ) {
      return;
    }
    const firstItem = profileQuery.data?.items[0];
    if (firstItem) {
      hasAutoSelected.current = true;
      onChangeRef.current(firstItem.id);
      lastAcceptedSelectionRef.current = {
        label: firstItem.name,
        value: firstItem.id,
      };
    }
  }, [autoSelect, profileQuery.data, value, debouncedSearch]);

  if (isSearchPending || profileQuery.isPending) {
    options.push({
      isDisabled: true,
      key: "loading",
      label: intl.formatMessage(gatewayProfileMessages.loadingGatewayProfiles),
      value: loadingValue,
    });
  } else if (options.length === 0) {
    options.push({
      isDisabled: true,
      key: "no-results",
      label: intl.formatMessage(
        gatewayProfileMessages.noMatchingGatewayProfiles,
      ),
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
  const chooseOption = (option: ProfileOption) => {
    if (typeof option.value !== "string" || option.isDisabled) {
      return;
    }
    lastAcceptedSelectionRef.current = {
      label: option.label,
      value: option.value,
    };
    onChange(option.value === "" ? null : option.value);
    setSearchValue("");
    closeMenu();
  };
  const focusOption = (index: number) => {
    setFocusedItemIndex(index);
    setActiveItemId(`gateway-profile-option-${String(index)}`);
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
      aria-label={intl.formatMessage(
        gatewayProfileMessages.selectGatewayProfile,
      )}
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
          aria-label={intl.formatMessage(
            gatewayProfileMessages.gatewayProfileName,
          )}
          aria-controls="gateway-profile-listbox"
          innerRef={inputRef}
          inputId="gateway-profile"
          inputProps={{
            "aria-autocomplete": "list",
            "aria-busy": isSearchPending || profileQuery.isFetching,
            ...(error ? { "aria-describedby": "gateway-profile-helper" } : {}),
            autoComplete: "off",
          }}
          isExpanded={isOpen}
          onChange={(_event, nextValue) => {
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
            } else if (event.key === "Escape" && isOpen) {
              event.preventDefault();
              const lastAcceptedSelection = lastAcceptedSelectionRef.current;
              setSearchValue("");
              onChange(
                lastAcceptedSelection.value === ""
                  ? null
                  : lastAcceptedSelection.value,
              );
              closeMenu();
            }
          }}
          placeholder={intl.formatMessage(
            gatewayProfileMessages.selectGatewayProfile,
          )}
          role="combobox"
          value={displayInputValue}
        />
        <TextInputGroupUtilities
          {...(!displayInputValue ? { style: { display: "none" } } : {})}
        >
          <Button
            aria-label={intl.formatMessage(
              gatewayProfileMessages.clearGatewayProfileSearch,
            )}
            icon={<RhMicronsCloseIcon />}
            onClick={() => {
              setSearchValue("");
              lastAcceptedSelectionRef.current = { label: "", value: "" };
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
      fieldId="gateway-profile"
      label={intl.formatMessage(gatewayProfileMessages.gatewayProfileName)}
    >
      <Select
        id="gateway-profile-select"
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
        selected={value ?? ""}
        toggle={toggle}
        variant="typeahead"
      >
        <SelectList id="gateway-profile-listbox">
          {options.map((option, index) => (
            <SelectOption
              description={option.description}
              id={`gateway-profile-option-${String(index)}`}
              isAriaDisabled={option.isDisabled}
              isFocused={focusedItemIndex === index}
              isSelected={
                typeof option.value === "string" &&
                (value ?? "") === option.value
              }
              key={option.key}
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
              id="gateway-profile-helper"
              screenReaderText={intl.formatMessage(
                gatewayProfileMessages.gatewayProfile,
              )}
              variant="error"
            >
              {error}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      ) : null}
      {profileQuery.data?.hasMore &&
      !isSearchPending &&
      !profileQuery.isFetching ? (
        <FormHelperText>
          <HelperText>
            <HelperTextItem>
              {intl.formatMessage(
                gatewayProfileMessages.moreGatewayProfilesAvailable,
              )}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      ) : null}
      {profileQuery.isError && !isSearchPending ? (
        <Alert
          actionLinks={
            <AlertActionLink onClick={() => void profileQuery.refetch()}>
              {intl.formatMessage(
                gatewayProfileMessages.refreshGatewayProfiles,
              )}
            </AlertActionLink>
          }
          isInline
          isLiveRegion
          title={intl.formatMessage(
            gatewayProfileMessages.gatewayProfileLoadError,
          )}
          variant="warning"
        >
          {intl.formatMessage(
            gatewayProfileMessages.gatewayProfileLoadErrorBody,
          )}
        </Alert>
      ) : null}
    </FormGroup>
  );
}
