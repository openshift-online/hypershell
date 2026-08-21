import { zodResolver } from "@hookform/resolvers/zod";
import {
  Alert,
  Button,
  Content,
  Form,
  FormGroup,
  FormHelperText,
  FormSelect,
  FormSelectOption,
  HelperText,
  HelperTextItem,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Stack,
  StackItem,
  TextArea,
  TextInput,
} from "@patternfly/react-core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useMemo, useState } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { useIntl } from "react-intl";
import { z } from "zod";

import {
  GatewayOperationError,
  type OpenShellGatewayServiceAccountCapabilities,
  type OpenShellGatewayServiceAccountCreateResult,
  type OpenShellGatewayServiceAccountRole,
} from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { serviceAccountListQueryRoot } from "./service-account-data";
import { ServiceAccountSetupView } from "./service-account-setup";

interface ServiceAccountFormValues {
  description: string;
  name: string;
  role: string;
  validitySeconds: string;
}

interface ServiceAccountFormOutput {
  description: string;
  name: string;
  role: OpenShellGatewayServiceAccountRole;
  validitySeconds: number;
}

function validityOptions(
  capabilities: OpenShellGatewayServiceAccountCapabilities,
): number[] {
  const { defaultSeconds, maximumSeconds, minimumSeconds } =
    capabilities.expirationPolicy;
  return [
    minimumSeconds,
    86_400,
    2_592_000,
    7_776_000,
    maximumSeconds,
    defaultSeconds,
  ]
    .filter((seconds) => seconds >= minimumSeconds && seconds <= maximumSeconds)
    .filter((seconds, index, values) => values.indexOf(seconds) === index)
    .sort((left, right) => left - right);
}

function FieldError({ id, message }: { id: string; message?: string }) {
  return message ? (
    <FormHelperText>
      <HelperText>
        <HelperTextItem id={id} variant="error">
          {message}
        </HelperTextItem>
      </HelperText>
    </FormHelperText>
  ) : null;
}

export function ServiceAccountCreateDialog({
  capabilities,
  gatewayId,
  isOpen,
  onClose,
}: {
  capabilities: OpenShellGatewayServiceAccountCapabilities;
  gatewayId: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const [currentTime, setCurrentTime] = useState(() => Date.now());
  const [handoff, setHandoff] =
    useState<OpenShellGatewayServiceAccountCreateResult>();
  const [isAcknowledged, setIsAcknowledged] = useState(false);
  const [isConfirmingLoss, setIsConfirmingLoss] = useState(false);
  const titleId = useId();
  const options = useMemo(() => validityOptions(capabilities), [capabilities]);
  const required = intl.formatMessage(messages.requiredField);
  const schema = useMemo(
    () =>
      z.object({
        description: z.string(),
        name: z.string().trim().min(1, required),
        role: z
          .string()
          .refine(
            (value): value is OpenShellGatewayServiceAccountRole =>
              capabilities.allowedRoles.includes(
                value as OpenShellGatewayServiceAccountRole,
              ),
            required,
          ),
        validitySeconds: z
          .string()
          .transform(Number)
          .refine(
            (value) =>
              Number.isFinite(value) &&
              value >= capabilities.expirationPolicy.minimumSeconds &&
              value <= capabilities.expirationPolicy.maximumSeconds,
            required,
          ),
      }),
    [capabilities, required],
  );
  const { control, handleSubmit } = useForm<
    ServiceAccountFormValues,
    undefined,
    ServiceAccountFormOutput
  >({
    defaultValues: {
      description: "",
      name: "",
      role: capabilities.allowedRoles.includes("openshell-user")
        ? "openshell-user"
        : (capabilities.allowedRoles[0] ?? "openshell-user"),
      validitySeconds: String(capabilities.expirationPolicy.defaultSeconds),
    },
    resolver: zodResolver(schema),
  });
  const selectedValidity = Number(
    useWatch({ control, name: "validitySeconds" }),
  );
  const expiration = new Date(currentTime + selectedValidity * 1000);
  useEffect(() => {
    if (!isOpen || handoff) {
      return;
    }
    const interval = window.setInterval(() => {
      setCurrentTime(Date.now());
    }, 30_000);
    return () => {
      window.clearInterval(interval);
    };
  }, [handoff, isOpen]);
  const creation = useMutation({
    gcTime: 0,
    mutationFn: async (values: ServiceAccountFormOutput) => {
      const result = await gateways.createOpenShellGatewayServiceAccount(
        gatewayId,
        {
          ...(values.description.trim()
            ? { description: values.description.trim() }
            : {}),
          expiresAt: new Date(
            Date.now() + values.validitySeconds * 1000,
          ).toISOString(),
          name: values.name,
          role: values.role,
        },
      );
      // The mutation resolves with void. The one-time secret therefore never
      // becomes TanStack mutation data; it exists only in this mounted view.
      setHandoff(result);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: serviceAccountListQueryRoot(gatewayId),
      });
    },
    retry: false,
  });

  const clearAndClose = () => {
    setHandoff(undefined);
    setIsAcknowledged(false);
    setIsConfirmingLoss(false);
    creation.reset();
    onClose();
  };
  const requestClose = () => {
    if (handoff && !isAcknowledged) {
      setIsConfirmingLoss(true);
      return;
    }
    clearAndClose();
  };
  const submit = handleSubmit((values) => {
    creation.mutate(values);
  });
  const uncertainFailure =
    creation.isError &&
    (!(creation.error instanceof GatewayOperationError) ||
      ["unavailable", "unknown"].includes(creation.error.kind));

  return (
    <Modal
      aria-labelledby={titleId}
      isOpen={isOpen}
      onClose={creation.isPending ? undefined : requestClose}
      variant={ModalVariant.large}
    >
      <ModalHeader
        labelId={titleId}
        title={intl.formatMessage(
          isConfirmingLoss
            ? messages.leaveWithoutSecretTitle
            : handoff
              ? messages.serviceAccountSetupTitle
              : messages.createServiceAccount,
          handoff
            ? { serviceAccountName: handoff.serviceAccount.name }
            : undefined,
        )}
      />
      <ModalBody>
        {isConfirmingLoss ? (
          <Content component="p">
            {intl.formatMessage(messages.leaveWithoutSecretBody)}
          </Content>
        ) : handoff ? (
          <ServiceAccountSetupView
            account={handoff.serviceAccount}
            clientSecret={handoff.credential.clientSecret}
            connection={handoff.credential}
            isAcknowledged={isAcknowledged}
            onAcknowledgedChange={setIsAcknowledged}
          />
        ) : (
          <Form
            aria-label={intl.formatMessage(messages.createServiceAccount)}
            onSubmit={(event) => void submit(event)}
          >
            <Stack hasGutter>
              {creation.isError ? (
                <StackItem>
                  <Alert
                    isInline
                    title={intl.formatMessage(
                      uncertainFailure
                        ? messages.serviceAccountCreateUncertain
                        : messages.serviceAccountCreateError,
                    )}
                    variant={uncertainFailure ? "warning" : "danger"}
                  >
                    {intl.formatMessage(
                      uncertainFailure
                        ? messages.serviceAccountCreateUncertainBody
                        : messages.serviceAccountCreateErrorBody,
                    )}
                  </Alert>
                </StackItem>
              ) : null}
              <StackItem>
                <Controller
                  control={control}
                  name="name"
                  render={({ field, fieldState }) => (
                    <FormGroup
                      fieldId="service-account-name"
                      isRequired
                      label={intl.formatMessage(messages.serviceAccountName)}
                    >
                      <TextInput
                        aria-describedby={
                          fieldState.error
                            ? "service-account-name-error"
                            : undefined
                        }
                        id="service-account-name"
                        isDisabled={creation.isPending}
                        isRequired
                        onBlur={field.onBlur}
                        onChange={(_event, value) => {
                          field.onChange(value);
                        }}
                        validated={fieldState.error ? "error" : "default"}
                        value={field.value}
                      />
                      <FieldError
                        id="service-account-name-error"
                        message={fieldState.error?.message}
                      />
                    </FormGroup>
                  )}
                />
              </StackItem>
              <StackItem>
                <Controller
                  control={control}
                  name="description"
                  render={({ field }) => (
                    <FormGroup
                      fieldId="service-account-description"
                      label={intl.formatMessage(
                        messages.serviceAccountDescription,
                      )}
                    >
                      <TextArea
                        id="service-account-description"
                        isDisabled={creation.isPending}
                        onBlur={field.onBlur}
                        onChange={(_event, value) => {
                          field.onChange(value);
                        }}
                        value={field.value}
                      />
                    </FormGroup>
                  )}
                />
              </StackItem>
              <StackItem>
                <Controller
                  control={control}
                  name="role"
                  render={({ field, fieldState }) => (
                    <FormGroup
                      fieldId="service-account-role"
                      isRequired
                      label={intl.formatMessage(messages.serviceAccountRole)}
                    >
                      <FormSelect
                        aria-describedby={`service-account-role-description${
                          fieldState.error ? " service-account-role-error" : ""
                        }`}
                        id="service-account-role"
                        isDisabled={creation.isPending}
                        onChange={(_event, value) => {
                          field.onChange(value);
                        }}
                        validated={fieldState.error ? "error" : "default"}
                        value={field.value}
                      >
                        {capabilities.allowedRoles.map((role) => (
                          <FormSelectOption
                            key={role}
                            label={role}
                            value={role}
                          />
                        ))}
                      </FormSelect>
                      <HelperText>
                        <HelperTextItem id="service-account-role-description">
                          {intl.formatMessage(
                            field.value === "openshell-admin"
                              ? messages.openshellAdminRoleDescription
                              : messages.openshellUserRoleDescription,
                          )}
                        </HelperTextItem>
                      </HelperText>
                      <FieldError
                        id="service-account-role-error"
                        message={fieldState.error?.message}
                      />
                    </FormGroup>
                  )}
                />
              </StackItem>
              <StackItem>
                <Controller
                  control={control}
                  name="validitySeconds"
                  render={({ field, fieldState }) => (
                    <FormGroup
                      fieldId="service-account-validity"
                      isRequired
                      label={intl.formatMessage(
                        messages.serviceAccountValidity,
                      )}
                    >
                      <FormSelect
                        aria-describedby={`service-account-expiration-preview service-account-token-lifetime-note${
                          fieldState.error
                            ? " service-account-validity-error"
                            : ""
                        }`}
                        id="service-account-validity"
                        isDisabled={creation.isPending}
                        onChange={(_event, value) => {
                          field.onChange(value);
                        }}
                        validated={fieldState.error ? "error" : "default"}
                        value={field.value}
                      >
                        {options.map((seconds) => (
                          <FormSelectOption
                            key={seconds}
                            label={intl.formatMessage(
                              messages.serviceAccountValidityOption,
                              { hours: seconds / 3_600 },
                            )}
                            value={String(seconds)}
                          />
                        ))}
                      </FormSelect>
                      <HelperText>
                        <HelperTextItem id="service-account-expiration-preview">
                          {intl.formatMessage(
                            messages.serviceAccountExpiresPreview,
                            {
                              expiration: intl.formatDate(expiration, {
                                dateStyle: "medium",
                                timeStyle: "long",
                              }),
                            },
                          )}
                        </HelperTextItem>
                        <HelperTextItem id="service-account-token-lifetime-note">
                          {intl.formatMessage(
                            messages.serviceAccountTokenLifetimeNote,
                          )}
                        </HelperTextItem>
                      </HelperText>
                      <FieldError
                        id="service-account-validity-error"
                        message={fieldState.error?.message}
                      />
                    </FormGroup>
                  )}
                />
              </StackItem>
            </Stack>
          </Form>
        )}
      </ModalBody>
      <ModalFooter>
        {isConfirmingLoss ? (
          <>
            <Button onClick={clearAndClose} variant="danger">
              {intl.formatMessage(messages.leaveSetup)}
            </Button>
            <Button
              onClick={() => {
                setIsConfirmingLoss(false);
              }}
              variant="link"
            >
              {intl.formatMessage(messages.returnToSetup)}
            </Button>
          </>
        ) : handoff ? (
          <Button
            isDisabled={!isAcknowledged}
            onClick={clearAndClose}
            variant="primary"
          >
            {intl.formatMessage(messages.finishSetup)}
          </Button>
        ) : (
          <>
            <Button
              isDisabled={creation.isPending}
              isLoading={creation.isPending}
              onClick={() => {
                void submit();
              }}
              spinnerAriaValueText={intl.formatMessage(
                messages.creatingServiceAccount,
              )}
              variant="primary"
            >
              {intl.formatMessage(messages.createServiceAccount)}
            </Button>
            <Button
              isDisabled={creation.isPending}
              onClick={requestClose}
              variant="link"
            >
              {intl.formatMessage(messages.cancel)}
            </Button>
          </>
        )}
      </ModalFooter>
    </Modal>
  );
}
