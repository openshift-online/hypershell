import {
  Alert,
  AlertActionLink,
  Button,
  Form,
  FormGroup,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Radio,
  Select,
  SelectList,
  SelectOption,
  Stack,
  StackItem,
  TextInputGroup,
  TextInputGroupMain,
  TextInputGroupUtilities,
  type MenuToggleElement,
} from "@patternfly/react-core";
import RhMicronsCloseIcon from "@patternfly/react-icons/dist/esm/icons/rh-microns-close-icon";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useIntl } from "react-intl";

import type {
  GatewayAccessCapabilities,
  GatewayAccessRole,
  GatewayDirectoryUser,
} from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { useDebouncedValue } from "../shared/use-debounced-value";
import {
  accessDirectoryQueryKey,
  accessListQueryRoot,
  accessSearchDebounceMilliseconds,
} from "./access-data";
import {
  actionErrorMessage,
  assignableRoles,
  roleDescription,
  roleLabel,
} from "./access-role";

const loadingValue = Symbol();
const noResultsValue = Symbol();

function DirectoryTypeahead({
  gatewayId,
  onSelect,
  selected,
}: {
  gatewayId: string;
  onSelect: (user: GatewayDirectoryUser) => void;
  selected: GatewayDirectoryUser | null;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const [isOpen, setIsOpen] = useState(false);
  const [inputValue, setInputValue] = useState(selected?.username ?? "");
  const [searchValue, setSearchValue] = useState("");
  const normalizedSearch = searchValue.trim();
  const debouncedSearch = useDebouncedValue(
    normalizedSearch,
    accessSearchDebounceMilliseconds,
  );
  const isSearchPending = normalizedSearch !== debouncedSearch;
  const directory = useQuery({
    queryFn: ({ signal }) =>
      gateways.searchGatewayDirectory(gatewayId, debouncedSearch, signal),
    queryKey: accessDirectoryQueryKey(gatewayId, debouncedSearch),
    staleTime: 10_000,
  });

  const candidates = !isSearchPending ? (directory.data ?? []) : [];
  const options = candidates.map((candidate) => ({
    key: candidate.username,
    label: candidate.name?.trim() ? candidate.name : candidate.username,
    user: candidate,
  }));

  const choose = (username: string) => {
    const match = options.find((option) => option.key === username);
    if (!match) {
      return;
    }
    onSelect(match.user);
    setInputValue(match.label);
    setSearchValue("");
    setIsOpen(false);
  };

  const toggle = (toggleRef: React.Ref<MenuToggleElement>) => (
    <MenuToggle
      aria-label={intl.formatMessage(messages.accessDirectorySearchLabel)}
      isExpanded={isOpen}
      isFullWidth
      onClick={() => {
        setIsOpen((open) => !open);
      }}
      ref={toggleRef}
      variant="typeahead"
    >
      <TextInputGroup isPlain>
        <TextInputGroupMain
          aria-label={intl.formatMessage(messages.accessDirectorySearchLabel)}
          inputProps={{
            "aria-autocomplete": "list",
            "aria-busy": isSearchPending || directory.isFetching,
            autoComplete: "off",
          }}
          isExpanded={isOpen}
          onChange={(_event, value) => {
            setInputValue(value);
            setSearchValue(value);
            setIsOpen(true);
          }}
          onClick={() => {
            setIsOpen(true);
          }}
          placeholder={intl.formatMessage(messages.accessFindPeople)}
          role="combobox"
          value={inputValue}
        />
        {inputValue ? (
          <TextInputGroupUtilities>
            <Button
              aria-label={intl.formatMessage(messages.accessDirectoryClear)}
              icon={<RhMicronsCloseIcon />}
              onClick={() => {
                setInputValue("");
                setSearchValue("");
              }}
              variant="plain"
            />
          </TextInputGroupUtilities>
        ) : null}
      </TextInputGroup>
    </MenuToggle>
  );

  const showLoading = isSearchPending || directory.isPending;
  const showNoResults =
    !showLoading && options.length === 0 && !directory.isError;

  return (
    <Stack hasGutter>
      <StackItem>
        <Select
          id="gateway-access-directory-select"
          isOpen={isOpen}
          onOpenChange={setIsOpen}
          onSelect={(_event, value) => {
            if (typeof value === "string") {
              choose(value);
            }
          }}
          selected={selected?.username}
          toggle={toggle}
          variant="typeahead"
        >
          <SelectList>
            {showLoading ? (
              <SelectOption isAriaDisabled key="loading" value={loadingValue}>
                {intl.formatMessage(messages.accessDirectoryLoading)}
              </SelectOption>
            ) : showNoResults ? (
              <SelectOption
                isAriaDisabled
                key="no-results"
                value={noResultsValue}
              >
                {intl.formatMessage(messages.accessDirectoryNoResults)}
              </SelectOption>
            ) : (
              options.map((option) => (
                <SelectOption
                  description={option.user.username}
                  key={option.key}
                  value={option.key}
                >
                  {option.label}
                </SelectOption>
              ))
            )}
          </SelectList>
        </Select>
      </StackItem>
      {directory.isError && !isSearchPending ? (
        <StackItem>
          <Alert
            actionLinks={
              <AlertActionLink onClick={() => void directory.refetch()}>
                {intl.formatMessage(messages.retry)}
              </AlertActionLink>
            }
            isInline
            title={intl.formatMessage(messages.accessDirectoryError)}
            variant="warning"
          />
        </StackItem>
      ) : null}
    </Stack>
  );
}

export function AddUsersDialog({
  capabilities,
  gatewayId,
  isOpen,
  onClose,
}: {
  capabilities: GatewayAccessCapabilities;
  gatewayId: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const titleId = useId();
  const roles = assignableRoles(capabilities);
  const [selected, setSelected] = useState<GatewayDirectoryUser | null>(null);
  const [role, setRole] = useState<GatewayAccessRole>(roles[0] ?? "user");

  const mutation = useMutation({
    mutationFn: async () => {
      if (!selected) {
        return;
      }
      await gateways.grantGatewayAccess(gatewayId, {
        role,
        ...(selected.subject ? { subject: selected.subject } : {}),
        username: selected.username,
      });
    },
    onSuccess: async () => {
      onClose();
      await queryClient.invalidateQueries({
        queryKey: accessListQueryRoot(gatewayId),
      });
    },
    retry: false,
  });

  const close = () => {
    mutation.reset();
    onClose();
  };

  return (
    <Modal
      aria-labelledby={titleId}
      isOpen={isOpen}
      onClose={mutation.isPending ? undefined : close}
      variant={ModalVariant.small}
    >
      <ModalHeader
        labelId={titleId}
        title={intl.formatMessage(messages.accessAddUsers)}
      />
      <ModalBody>
        <Form>
          <FormGroup
            fieldId="gateway-access-directory"
            label={intl.formatMessage(messages.accessDirectorySearchLabel)}
          >
            <DirectoryTypeahead
              gatewayId={gatewayId}
              onSelect={setSelected}
              selected={selected}
            />
          </FormGroup>
          {selected ? (
            <FormGroup
              label={intl.formatMessage(messages.accessSelectRole)}
              role="radiogroup"
            >
              <Stack hasGutter>
                {roles.map((candidateRole) => (
                  <StackItem key={candidateRole}>
                    <Radio
                      description={roleDescription(
                        intl.formatMessage,
                        candidateRole,
                      )}
                      id={`gateway-access-role-${candidateRole}`}
                      isChecked={role === candidateRole}
                      label={roleLabel(intl.formatMessage, candidateRole)}
                      name="gateway-access-role"
                      onChange={() => {
                        setRole(candidateRole);
                      }}
                    />
                  </StackItem>
                ))}
              </Stack>
            </FormGroup>
          ) : null}
          {mutation.isError ? (
            <Alert
              isInline
              title={actionErrorMessage(intl.formatMessage, mutation.error)}
              variant="danger"
            />
          ) : null}
        </Form>
      </ModalBody>
      <ModalFooter>
        <Button
          isDisabled={!selected || mutation.isPending}
          isLoading={mutation.isPending}
          onClick={() => {
            mutation.mutate();
          }}
          spinnerAriaValueText={intl.formatMessage(messages.accessGranting)}
          variant="primary"
        >
          {intl.formatMessage(messages.accessGrant)}
        </Button>
        <Button isDisabled={mutation.isPending} onClick={close} variant="link">
          {intl.formatMessage(messages.cancel)}
        </Button>
      </ModalFooter>
    </Modal>
  );
}
