import {
  ActionGroup,
  Alert,
  Button,
  Content,
  Form,
  FormGroup,
  FormHelperText,
  HelperText,
  HelperTextItem,
  PageSection,
  TextInput,
  Title,
} from "@patternfly/react-core";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { Controller, useForm, type Control } from "react-hook-form";
import { FormattedMessage, useIntl } from "react-intl";
import { z } from "zod";

import { useGatewayProfileUi } from "../gateway-profile-ui-provider";
import type { GatewayProfileCreateInput } from "../application/gateway-profile-types";
import { gatewayProfileMessages } from "../gateway-profile-messages";
import { messages } from "../messages";
import {
  gatewayProfileListQueryRoot,
  gatewayProfileQueryKey,
} from "./gateway-profile-data";

export interface GatewayProfileCreatePageProps {
  onCreated?: (gatewayProfileId: string) => Promise<void> | void;
}

interface GatewayProfileFormValues {
  containerCpuLimitMax: string;
  containerCpuRequestDefault: string;
  containerMemoryLimitMax: string;
  containerMemoryRequestDefault: string;
  cpuLimitTotal: string;
  cpuRequestTotal: string;
  description: string;
  ephemeralStorageTotal: string;
  memoryLimitTotal: string;
  memoryRequestTotal: string;
  name: string;
  podCount: string;
  pvcCount: string;
}

type GatewayProfileTextFieldName = keyof GatewayProfileFormValues;

interface GatewayProfileTextFieldProps {
  control: Control<
    GatewayProfileFormValues,
    undefined,
    GatewayProfileCreateInput
  >;
  fieldId: string;
  isDisabled: boolean;
  isRequired?: boolean;
  label: string;
  name: GatewayProfileTextFieldName;
}

function GatewayProfileTextField({
  control,
  fieldId,
  isDisabled,
  isRequired = false,
  label,
  name,
}: GatewayProfileTextFieldProps) {
  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FormGroup fieldId={fieldId} isRequired={isRequired} label={label}>
          <TextInput
            aria-describedby={
              fieldState.error ? `${fieldId}-helper` : undefined
            }
            id={fieldId}
            isDisabled={isDisabled}
            isRequired={isRequired}
            name={field.name}
            onBlur={field.onBlur}
            onChange={(_event, value) => {
              field.onChange(value);
            }}
            validated={fieldState.error ? "error" : "default"}
            value={field.value}
          />
          {fieldState.error ? (
            <FormHelperText>
              <HelperText>
                <HelperTextItem id={`${fieldId}-helper`} variant="error">
                  {fieldState.error.message}
                </HelperTextItem>
              </HelperText>
            </FormHelperText>
          ) : null}
        </FormGroup>
      )}
    />
  );
}

export function GatewayProfileCreatePage({
  onCreated,
}: GatewayProfileCreatePageProps = {}) {
  const intl = useIntl();
  const { gatewayProfiles, navigation } = useGatewayProfileUi();
  const queryClient = useQueryClient();
  const requiredMessage = intl.formatMessage(messages.requiredField);
  const schema = useMemo(() => {
    const optionalString = z
      .string()
      .trim()
      .transform((value) => (value.length > 0 ? value : undefined));
    const optionalCount = z
      .string()
      .trim()
      .transform((value, context) => {
        if (value.length === 0) {
          return undefined;
        }
        const parsed = Number(value);
        if (!Number.isInteger(parsed) || parsed < 0) {
          context.addIssue({ code: "custom", message: requiredMessage });
          return z.NEVER;
        }
        return parsed;
      });

    return z.object({
      containerCpuLimitMax: optionalString,
      containerCpuRequestDefault: optionalString,
      containerMemoryLimitMax: optionalString,
      containerMemoryRequestDefault: optionalString,
      cpuLimitTotal: optionalString,
      cpuRequestTotal: optionalString,
      description: optionalString,
      ephemeralStorageTotal: optionalString,
      memoryLimitTotal: optionalString,
      memoryRequestTotal: optionalString,
      name: z.string().trim().min(1, requiredMessage),
      podCount: optionalCount,
      pvcCount: optionalCount,
    });
  }, [requiredMessage]);
  const { control, handleSubmit } = useForm<
    GatewayProfileFormValues,
    undefined,
    GatewayProfileCreateInput
  >({
    defaultValues: {
      containerCpuLimitMax: "",
      containerCpuRequestDefault: "",
      containerMemoryLimitMax: "",
      containerMemoryRequestDefault: "",
      cpuLimitTotal: "",
      cpuRequestTotal: "",
      description: "",
      ephemeralStorageTotal: "",
      memoryLimitTotal: "",
      memoryRequestTotal: "",
      name: "",
      podCount: "",
      pvcCount: "",
    },
    resolver: zodResolver(schema),
  });

  const createGatewayProfile = useMutation({
    mutationFn: (values: GatewayProfileCreateInput) => {
      return gatewayProfiles.createGatewayProfile(values);
    },
    onSuccess: async (gatewayProfile) => {
      queryClient.setQueryData(
        gatewayProfileQueryKey(gatewayProfile.id),
        gatewayProfile,
      );
      await queryClient.invalidateQueries({
        queryKey: gatewayProfileListQueryRoot,
      });
      if (onCreated) {
        await onCreated(gatewayProfile.id);
      } else {
        await navigation.navigate(navigation.detailHref(gatewayProfile.id));
      }
    },
  });

  const submit = handleSubmit((values) => {
    createGatewayProfile.mutate(values);
  });

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage
              {...gatewayProfileMessages.createGatewayProfile}
            />
          </Title>
          <p>
            <FormattedMessage
              {...gatewayProfileMessages.createGatewayProfileDescription}
            />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <Form
          aria-label={intl.formatMessage(
            gatewayProfileMessages.createGatewayProfile,
          )}
          isWidthLimited
          onSubmit={(event) => void submit(event)}
        >
          {createGatewayProfile.isError ? (
            <Alert
              isInline
              title={intl.formatMessage(
                gatewayProfileMessages.gatewayProfilesProvisionError,
              )}
              variant="danger"
            >
              <FormattedMessage
                {...gatewayProfileMessages.gatewayProfilesProvisionErrorBody}
              />
            </Alert>
          ) : null}
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-name"
            isDisabled={createGatewayProfile.isPending}
            isRequired
            label={intl.formatMessage(
              gatewayProfileMessages.gatewayProfileName,
            )}
            name="name"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-description"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(
              gatewayProfileMessages.gatewayProfileDescription,
            )}
            name="description"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-cpu-request-total"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(gatewayProfileMessages.cpuRequestTotal)}
            name="cpuRequestTotal"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-cpu-limit-total"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(gatewayProfileMessages.cpuLimitTotal)}
            name="cpuLimitTotal"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-memory-request-total"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(
              gatewayProfileMessages.memoryRequestTotal,
            )}
            name="memoryRequestTotal"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-memory-limit-total"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(gatewayProfileMessages.memoryLimitTotal)}
            name="memoryLimitTotal"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-ephemeral-storage-total"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(
              gatewayProfileMessages.ephemeralStorageTotal,
            )}
            name="ephemeralStorageTotal"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-container-cpu-request-default"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(
              gatewayProfileMessages.containerCpuRequestDefault,
            )}
            name="containerCpuRequestDefault"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-container-cpu-limit-max"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(
              gatewayProfileMessages.containerCpuLimitMax,
            )}
            name="containerCpuLimitMax"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-container-memory-request-default"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(
              gatewayProfileMessages.containerMemoryRequestDefault,
            )}
            name="containerMemoryRequestDefault"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-container-memory-limit-max"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(
              gatewayProfileMessages.containerMemoryLimitMax,
            )}
            name="containerMemoryLimitMax"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-pod-count"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(gatewayProfileMessages.podCount)}
            name="podCount"
          />
          <GatewayProfileTextField
            control={control}
            fieldId="gateway-profile-pvc-count"
            isDisabled={createGatewayProfile.isPending}
            label={intl.formatMessage(gatewayProfileMessages.pvcCount)}
            name="pvcCount"
          />
          <ActionGroup>
            <Button
              isDisabled={createGatewayProfile.isPending}
              type="submit"
              variant="primary"
              {...(createGatewayProfile.isPending
                ? {
                    isLoading: true,
                    spinnerAriaValueText: intl.formatMessage(
                      gatewayProfileMessages.creatingGatewayProfile,
                    ),
                  }
                : {})}
            >
              <FormattedMessage
                {...gatewayProfileMessages.createGatewayProfile}
              />
            </Button>
            <Button
              isDisabled={createGatewayProfile.isPending}
              onClick={() => {
                void navigation.navigate(navigation.collectionHref);
              }}
              type="button"
              variant="link"
            >
              <FormattedMessage {...messages.cancel} />
            </Button>
          </ActionGroup>
        </Form>
      </PageSection>
    </>
  );
}
