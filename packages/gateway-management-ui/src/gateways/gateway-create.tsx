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
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo } from "react";
import { Controller, useForm, useWatch, type Control } from "react-hook-form";
import { FormattedMessage, useIntl } from "react-intl";
import { z } from "zod";

import { useGatewayUi } from "../gateway-ui-provider";
import type { GatewayProvisionInput } from "../application/gateway-types";
import { messages } from "../messages";
import {
  gatewayListQueryRoot,
  gatewayPlacementDetailQueryKey,
  gatewayQueryKey,
} from "./gateway-data";
import { GatewayPlacementSelect } from "./gateway-placement-select";
import { GatewayProfileSelect } from "./gateway-profile-select";

export interface GatewayCreatePageProps {
  onCreated?: (gatewayId: string) => Promise<void> | void;
}

interface GatewayFormValues {
  clusterId: string | null;
  name: string;
  profileId: string | null;
}

interface GatewayTextFieldProps {
  control: Control<GatewayFormValues, undefined, GatewayProvisionInput>;
  fieldId: string;
  isDisabled: boolean;
  label: string;
  name: "name";
}

function GatewayTextField({
  control,
  fieldId,
  isDisabled,
  label,
  name,
}: GatewayTextFieldProps) {
  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FormGroup fieldId={fieldId} isRequired label={label}>
          <TextInput
            aria-describedby={
              fieldState.error ? `${fieldId}-helper` : undefined
            }
            id={fieldId}
            isDisabled={isDisabled}
            isRequired
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

export function GatewayCreatePage({ onCreated }: GatewayCreatePageProps = {}) {
  const intl = useIntl();
  const { gateways, navigation } = useGatewayUi();
  const queryClient = useQueryClient();
  const requiredMessage = intl.formatMessage(messages.requiredField);
  const schema = useMemo(() => {
    const requiredString = z.string().trim().min(1, requiredMessage);

    return z.object({
      clusterId: z
        .string({ error: requiredMessage })
        .nullable()
        .transform((value, context) => {
          if (!value) {
            context.addIssue({ code: "custom", message: requiredMessage });
            return z.NEVER;
          }
          return value;
        }),
      name: requiredString,
      profileId: z
        .string()
        .nullable()
        .transform((value) => value ?? undefined),
    });
  }, [requiredMessage]);
  const { control, handleSubmit, setValue } = useForm<
    GatewayFormValues,
    undefined,
    GatewayProvisionInput
  >({
    defaultValues: {
      clusterId: null,
      name: "",
      profileId: null,
    },
    resolver: zodResolver(schema),
  });

  const clusterId = useWatch({ control, name: "clusterId" });
  const clusterPlacementQuery = useQuery({
    enabled: !!clusterId,
    queryFn: ({ signal }) =>
      gateways.getGatewayPlacement(clusterId ?? "", signal),
    queryKey: gatewayPlacementDetailQueryKey(clusterId ?? ""),
  });

  useEffect(() => {
    const profileId = clusterPlacementQuery.data?.profileId;
    if (profileId) {
      setValue("profileId", profileId);
    }
  }, [clusterPlacementQuery.data, setValue]);

  const createGateway = useMutation({
    mutationFn: (values: GatewayProvisionInput) => {
      return gateways.provisionGateway(values);
    },
    onSuccess: async (gateway) => {
      queryClient.setQueryData(gatewayQueryKey(gateway.id), gateway);
      await queryClient.invalidateQueries({
        queryKey: gatewayListQueryRoot,
      });
      if (onCreated) {
        await onCreated(gateway.id);
      } else {
        await navigation.navigate(navigation.detailHref(gateway.id));
      }
    },
  });

  const submit = handleSubmit((values) => {
    createGateway.mutate(values);
  });

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage {...messages.provisionGateway} />
          </Title>
          <p>
            <FormattedMessage {...messages.provisionGatewayDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <Form
          aria-label={intl.formatMessage(messages.provisionGateway)}
          isWidthLimited
          onSubmit={(event) => void submit(event)}
        >
          {createGateway.isError ? (
            <Alert
              isInline
              title={intl.formatMessage(messages.gatewayProvisionError)}
              variant="danger"
            >
              <FormattedMessage {...messages.gatewayProvisionErrorBody} />
            </Alert>
          ) : null}
          <GatewayTextField
            control={control}
            fieldId="gateway-name"
            isDisabled={createGateway.isPending}
            label={intl.formatMessage(messages.gatewayName)}
            name="name"
          />
          <Controller
            control={control}
            name="clusterId"
            render={({ field, fieldState }) => (
              <GatewayPlacementSelect
                error={fieldState.error?.message}
                isDisabled={createGateway.isPending}
                onChange={field.onChange}
                value={field.value}
              />
            )}
          />
          <Controller
            control={control}
            name="profileId"
            render={({ field, fieldState }) => (
              <GatewayProfileSelect
                error={fieldState.error?.message}
                isDisabled={createGateway.isPending}
                onChange={field.onChange}
                value={field.value}
              />
            )}
          />
          <ActionGroup>
            <Button
              isDisabled={createGateway.isPending}
              type="submit"
              variant="primary"
              {...(createGateway.isPending
                ? {
                    isLoading: true,
                    spinnerAriaValueText: intl.formatMessage(
                      messages.provisioningGateway,
                    ),
                  }
                : {})}
            >
              <FormattedMessage {...messages.provisionGateway} />
            </Button>
            <Button
              isDisabled={createGateway.isPending}
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
