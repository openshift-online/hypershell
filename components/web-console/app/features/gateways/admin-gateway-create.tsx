import type { GatewayCreateRequest } from "@openshift-online/hypershell-sdk";
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
import { Link, useNavigate, useSearchParams } from "react-router";
import { z } from "zod";

import { messages } from "../../i18n/messages";
import { apiClient } from "../../lib/api.client";
import {
  availableClusterOptions,
  getSelectedCluster,
  type ClusterOption,
} from "../clusters/cluster-options";

interface GatewayFormValues {
  database_id: string;
  name: string;
  namespace: string;
  release_id: string;
}

const fieldNames = [
  "name",
  "namespace",
  "release_id",
  "database_id",
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

interface AdminGatewayCreatePageProps {
  clusters?: readonly ClusterOption[];
}

export function AdminGatewayCreatePage({
  clusters = availableClusterOptions,
}: AdminGatewayCreatePageProps = {}) {
  const intl = useIntl();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const selectedCluster = getSelectedCluster(
    clusters,
    searchParams.get("cluster"),
  );
  const requiredMessage = intl.formatMessage(messages.requiredField);
  const schema = useMemo(() => {
    const requiredString = z.string().trim().min(1, requiredMessage);

    return z.object({
      database_id: requiredString,
      name: requiredString,
      namespace: requiredString,
      release_id: requiredString,
    });
  }, [requiredMessage]);
  const { control, handleSubmit, setError } = useForm<GatewayFormValues>({
    defaultValues: {
      database_id: "",
      name: "",
      namespace: "openshell",
      release_id: "",
    },
  });

  const createGateway = useMutation({
    mutationFn: (values: GatewayFormValues) => {
      // Placement is UI context only in this increment and is intentionally
      // absent from the request. The API contract remains unchanged.
      const request = values as GatewayCreateRequest;
      return apiClient.gateways.create(request);
    },
    onSuccess: async (gateway) => {
      await queryClient.invalidateQueries({ queryKey: ["gateways"] });
      await navigate(`/admin/gateways/${gateway.id}`);
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
          <FormGroup
            fieldId="gateway-cluster"
            label={intl.formatMessage(messages.cluster)}
          >
            <TextInput
              id="gateway-cluster"
              readOnlyVariant="default"
              value={selectedCluster.name}
            />
          </FormGroup>
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
          <GatewayTextField
            control={control}
            fieldId="gateway-release-id"
            isDisabled={createGateway.isPending}
            label={intl.formatMessage(messages.gatewayReleaseId)}
            name="release_id"
          />
          <GatewayTextField
            control={control}
            fieldId="gateway-database-id"
            isDisabled={createGateway.isPending}
            label={intl.formatMessage(messages.managedDatabaseId)}
            name="database_id"
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
              {...{ to: "/admin/gateways" }}
            >
              <FormattedMessage {...messages.cancel} />
            </Button>
          </ActionGroup>
        </Form>
      </PageSection>
    </>
  );
}
