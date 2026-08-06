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
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { Controller, useForm, type Control } from "react-hook-form";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, useNavigate } from "react-router";
import { z } from "zod";

import { messages } from "../../i18n/messages";
import { apiClient } from "../../adapters/api/api.client";
import { gatewayQueryKey } from "./gateway-data";
interface GatewayFormValues {
  name: string;
  namespace: string;
}

const fieldNames = [
  "name",
  "namespace",
] as const satisfies readonly (keyof GatewayFormValues)[];

interface GatewayTextFieldProps {
  control: Control<GatewayFormValues>;
  fieldId: string;
  isDisabled: boolean;
  label: string;
  name: keyof GatewayFormValues;
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

export function GatewayCreatePage() {
  const intl = useIntl();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const requiredMessage = intl.formatMessage(messages.requiredField);
  const schema = useMemo(() => {
    const requiredString = z.string().trim().min(1, requiredMessage);

    return z.object({
      name: requiredString,
      namespace: requiredString,
    });
  }, [requiredMessage]);
  const { control, handleSubmit, setError } = useForm<GatewayFormValues>({
    defaultValues: {
      name: "",
      namespace: "openshell",
    },
  });

  const createGateway = useMutation({
    mutationFn: (values: GatewayFormValues) => {
      const request = {
        ...values,
        cluster_id: "",
        database_id: "",
        fleet_id: "",
        release_id: "",
      };
      return apiClient.gateways.create(request);
    },
    onSuccess: async (gateway) => {
      queryClient.setQueryData(gatewayQueryKey(gateway.id), gateway);
      await queryClient.invalidateQueries({
        exact: true,
        queryKey: ["gateways"],
      });
      await navigate(`/gateways/${gateway.id}`);
    },
  });

  const submit = handleSubmit((values) => {
    const result = schema.safeParse(values);
    if (!result.success) {
      for (const issue of result.error.issues) {
        const fieldName = issue.path[0];
        if (fieldNames.includes(fieldName as keyof GatewayFormValues)) {
          setError(fieldName as keyof GatewayFormValues, {
            message: issue.message,
            type: "validate",
          });
        }
      }
      return;
    }

    createGateway.mutate(result.data);
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
          <GatewayTextField
            control={control}
            fieldId="gateway-namespace"
            isDisabled={createGateway.isPending}
            label={intl.formatMessage(messages.namespace)}
            name="namespace"
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
              component={Link}
              isDisabled={createGateway.isPending}
              variant="link"
              {...{ to: "/" }}
            >
              <FormattedMessage {...messages.cancel} />
            </Button>
          </ActionGroup>
        </Form>
      </PageSection>
    </>
  );
}
