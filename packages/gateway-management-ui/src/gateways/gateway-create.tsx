import {
  ActionGroup,
  Alert,
  AlertActionLink,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  Form,
  FormGroup,
  Gallery,
  FormHelperText,
  HelperText,
  HelperTextItem,
  Label,
  PageSection,
  Stack,
  StackItem,
  TextInput,
  Title,
} from "@patternfly/react-core";
import { zodResolver } from "@hookform/resolvers/zod";
import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useMemo, type ReactNode } from "react";
import { Controller, useForm, useWatch, type Control } from "react-hook-form";
import { FormattedMessage, useIntl } from "react-intl";
import { z } from "zod";

import { useGatewayUi } from "../gateway-ui-provider";
import type { GatewayProvisionInput } from "../application/gateway-types";
import { messages } from "../messages";
import { gatewayListQueryRoot, gatewayQueryKey } from "./gateway-data";
import styles from "./gateway-create.module.css";
import awsLogo from "../assets/aws-logo.svg";
import ibmCloudLogo from "../assets/ibm-cloud.svg";

const placementReasonMessages = {
  "no-eligible-cluster": messages.noEligibleCluster,
} as const;

function placementReasonKey(
  ...reasons: readonly (string | null | undefined)[]
): keyof typeof placementReasonMessages {
  return (reasons.find(
    (reason) => reason !== undefined && reason !== null && reason !== "",
  ) ?? "no-eligible-cluster") as keyof typeof placementReasonMessages;
}

export interface GatewayCreatePageProps {
  onCreated?: (gatewayId: string) => Promise<void> | void;
}

interface GatewayFormValues {
  name: string;
  network: "public" | "vpn" | null;
  provider: "aws" | "ibm" | null;
  localKind: boolean;
}

interface GatewayTextFieldProps {
  control: Control<GatewayFormValues>;
  fieldId: string;
  isDisabled: boolean;
  label: string;
  name: "name";
}

function Choice({
  title,
  description,
  descriptionLabel,
  value,
  selected,
  icon,
  iconPadding,
  isDisabled,
  name,
  onChoose,
}: {
  title: string;
  description?: string;
  descriptionLabel?: string;
  value: string;
  selected?: boolean;
  icon?: string;
  iconPadding?: string;
  isDisabled?: boolean;
  name: string;
  onChoose: () => void;
}) {
  const labelId = `${name}-${value}-label`;
  const descriptionId = description
    ? `${name}-${value}-description`
    : undefined;
  return (
    <Card
      className={styles.choice}
      isSelectable
      isSelected={selected}
      isDisabled={isDisabled}
      onClick={() => {
        if (!isDisabled) onChoose();
      }}
    >
      <CardHeader
        selectableActions={{
          isChecked: selected,
          isHidden: true,
          name,
          onChange: () => {
            onChoose();
          },
          selectableActionAriaLabelledby: labelId,
          selectableActionProps: {
            value,
            ...(descriptionId ? { "aria-describedby": descriptionId } : {}),
          },
          variant: "single",
        }}
      >
        <CardTitle>
          <span className={styles.choiceHeading}>
            <span className={styles.choiceTitle} id={labelId}>
              {title}
            </span>
          </span>
        </CardTitle>
      </CardHeader>
      {description ? (
        <CardBody>
          {descriptionLabel ? (
            <Label isCompact>{descriptionLabel}</Label>
          ) : null}
          {icon ? (
            <span className={styles.providerContent}>
              <img
                className={styles.providerLogo}
                src={icon}
                alt=""
                style={iconPadding ? { padding: iconPadding } : undefined}
              />
              <span>{description}</span>
            </span>
          ) : descriptionLabel ? (
            <span id={descriptionId} className={styles.descriptionAfterLabel}>
              {description}
            </span>
          ) : (
            description
          )}
        </CardBody>
      ) : null}
    </Card>
  );
}

function ChoiceGroup({
  id,
  label,
  error,
  children,
}: {
  id: string;
  label: string;
  error?: string;
  children: ReactNode;
}) {
  const errorId = `${id}-error`;
  return (
    <FormGroup isRequired label={label} fieldId={id}>
      <Stack hasGutter>
        <StackItem>
          <Gallery
            hasGutter
            minWidths={{ default: "250px", md: "300px" }}
            aria-describedby={error ? errorId : undefined}
            aria-label={label}
            role="radiogroup"
          >
            {children}
          </Gallery>
        </StackItem>
      </Stack>
      {error ? (
        <FormHelperText>
          <HelperText>
            <HelperTextItem id={errorId} variant="error">
              {error}
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      ) : null}
    </FormGroup>
  );
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

    return z
      .object({
        name: requiredString,
        network: z.enum(["public", "vpn"]).nullable(),
        provider: z.enum(["aws", "ibm"]).nullable(),
        localKind: z.boolean(),
      })
      .superRefine((values, context) => {
        if (!values.localKind && values.network === null) {
          context.addIssue({
            code: "custom",
            path: ["network"],
            message: requiredMessage,
          });
        }
        if (
          !values.localKind &&
          values.network !== null &&
          values.provider === null
        ) {
          context.addIssue({
            code: "custom",
            path: ["provider"],
            message: requiredMessage,
          });
        }
      });
  }, [requiredMessage]);
  const { clearErrors, control, handleSubmit, setValue } =
    useForm<GatewayFormValues>({
      defaultValues: {
        name: "",
        network: null,
        provider: null,
        localKind: false,
      },
      resolver: zodResolver(schema) as never,
    });

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
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["gateway", "placement-availability"],
      });
    },
  });

  const getPlacementAvailability =
    gateways.getGatewayPlacementAvailability?.bind(gateways);
  const availability = useQuery({
    queryKey: ["gateway", "placement-availability"],
    queryFn:
      getPlacementAvailability === undefined
        ? skipToken
        : ({ signal }) => getPlacementAvailability(signal),
    staleTime: 30_000,
  });
  const hasManagedPlacement = availability.data
    ? availability.data.awsPublic ||
      availability.data.awsVpn ||
      availability.data.ibmPublic ||
      availability.data.ibmVpn
    : false;
  const canUseLocalKind =
    availability.data?.localKind === true && !hasManagedPlacement;
  const defaultToLocalKind = canUseLocalKind && !hasManagedPlacement;
  const publicPlacementAvailable = availability.data
    ? availability.data.awsPublic || availability.data.ibmPublic
    : false;
  const vpnPlacementAvailable = availability.data?.awsVpn === true;

  const submit = handleSubmit((values) => {
    if (values.localKind) {
      createGateway.mutate({
        name: values.name,
        placement: { mode: "local-kind" },
      });
      return;
    }
    if (values.network === null || values.provider === null) return;
    if (values.network === "vpn") {
      if (values.provider !== "aws") return;
      createGateway.mutate({
        name: values.name,
        placement: { network: "vpn", provider: "aws" },
      });
      return;
    }
    createGateway.mutate({
      name: values.name,
      placement: { network: "public", provider: values.provider },
    });
  });
  const network = useWatch({ control, name: "network" });
  const provider = useWatch({ control, name: "provider" });
  const localKind = useWatch({ control, name: "localKind" });
  useEffect(() => {
    if (!availability.data || localKind) return;
    if (network === "vpn") {
      const availableProvider = availability.data.awsVpn ? "aws" : null;
      if (provider === availableProvider) return;
      setValue("provider", availableProvider, {
        shouldValidate: true,
      });
    } else if (
      network === "public" &&
      (provider === null ||
        (provider === "ibm" && !availability.data.ibmPublic) ||
        (provider === "aws" && !availability.data.awsPublic))
    ) {
      setValue(
        "provider",
        availability.data.ibmPublic
          ? "ibm"
          : availability.data.awsPublic
            ? "aws"
            : null,
        { shouldValidate: true },
      );
    }
  }, [availability.data, localKind, network, provider, setValue]);
  useEffect(() => {
    if (defaultToLocalKind && !localKind) {
      setValue("localKind", true, { shouldValidate: true });
      setValue("network", null);
      setValue("provider", null);
      clearErrors(["network", "provider"]);
    } else if (!canUseLocalKind && localKind) {
      setValue("localKind", false, { shouldValidate: true });
    }
  }, [canUseLocalKind, clearErrors, defaultToLocalKind, localKind, setValue]);

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
      <PageSection hasBodyWrapper={false} isFilled variant="default">
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
          {canUseLocalKind ? (
            <Controller
              control={control}
              name="localKind"
              render={({ field }) => (
                <ChoiceGroup
                  id="local-development"
                  label={intl.formatMessage(messages.localDevelopment)}
                >
                  <Choice
                    name="placement"
                    value="local-kind"
                    selected={localKind}
                    title={intl.formatMessage(messages.localKindPlacement)}
                    description={intl.formatMessage(
                      messages.localKindPlacementDescription,
                    )}
                    onChoose={() => {
                      field.onChange(true);
                      setValue("network", null);
                      setValue("provider", null);
                      clearErrors(["network", "provider"]);
                    }}
                  />
                </ChoiceGroup>
              )}
            />
          ) : null}
          <Controller
            control={control}
            name="network"
            render={({ field, fieldState }) => (
              <ChoiceGroup
                id="network-access"
                error={fieldState.error?.message}
                label={intl.formatMessage(messages.networkAccess)}
              >
                <Choice
                  name="placement"
                  value="public"
                  selected={!localKind && field.value === "public"}
                  title={intl.formatMessage(messages.publicNetwork)}
                  description={
                    availability.data && !publicPlacementAvailable
                      ? intl.formatMessage(messages.unavailableNetwork, {
                          network: intl.formatMessage(messages.publicNetwork),
                          reason: intl.formatMessage(
                            placementReasonMessages[
                              placementReasonKey(
                                availability.data.awsReason,
                                availability.data.ibmReason,
                              )
                            ],
                          ),
                        })
                      : intl.formatMessage(messages.publicNetworkDescription)
                  }
                  isDisabled={!publicPlacementAvailable}
                  onChoose={() => {
                    setValue("localKind", false);
                    field.onChange("public");
                    setValue(
                      "provider",
                      availability.data?.ibmPublic
                        ? "ibm"
                        : availability.data?.awsPublic
                          ? "aws"
                          : null,
                    );
                  }}
                />
                <Choice
                  name="placement"
                  value="vpn"
                  selected={!localKind && field.value === "vpn"}
                  title={intl.formatMessage(messages.vpnNetwork)}
                  descriptionLabel={intl.formatMessage(
                    messages.vpnRequiresLabel,
                  )}
                  description={
                    availability.data && !vpnPlacementAvailable
                      ? intl.formatMessage(messages.unavailableNetwork, {
                          network: intl.formatMessage(messages.vpnNetwork),
                          reason: intl.formatMessage(
                            placementReasonMessages[
                              placementReasonKey(availability.data.awsReason)
                            ],
                          ),
                        })
                      : intl.formatMessage(messages.vpnNetworkDescription)
                  }
                  isDisabled={!vpnPlacementAvailable}
                  onChoose={() => {
                    setValue("localKind", false);
                    field.onChange("vpn");
                    setValue(
                      "provider",
                      availability.data?.awsVpn === true ? "aws" : null,
                    );
                  }}
                />
              </ChoiceGroup>
            )}
          />
          {network !== null ? (
            <Controller
              control={control}
              name="provider"
              render={({ field, fieldState }) => (
                <ChoiceGroup
                  id="cloud-provider"
                  error={fieldState.error?.message}
                  label={intl.formatMessage(messages.cloudProvider)}
                >
                  <Choice
                    name="provider"
                    value="aws"
                    selected={field.value === "aws"}
                    title={intl.formatMessage(messages.awsProvider)}
                    description={
                      availability.data &&
                      !(network === "vpn"
                        ? availability.data.awsVpn
                        : availability.data.awsPublic)
                        ? intl.formatMessage(messages.unavailableProvider, {
                            provider: intl.formatMessage(messages.awsProvider),
                            reason: intl.formatMessage(
                              placementReasonMessages[
                                placementReasonKey(availability.data.awsReason)
                              ],
                            ),
                          })
                        : intl.formatMessage(messages.awsProviderDescription)
                    }
                    icon={awsLogo}
                    iconPadding="0.5rem 0"
                    isDisabled={
                      availability.data
                        ? !(network === "vpn"
                            ? availability.data.awsVpn
                            : availability.data.awsPublic)
                        : true
                    }
                    onChoose={() => {
                      field.onChange("aws");
                    }}
                  />
                  <Choice
                    name="provider"
                    value="ibm"
                    selected={field.value === "ibm"}
                    title={intl.formatMessage(messages.ibmCloudProvider)}
                    description={
                      network === "vpn"
                        ? intl.formatMessage(messages.vpnRequiresAws)
                        : availability.data && !availability.data.ibmPublic
                          ? intl.formatMessage(messages.unavailableProvider, {
                              provider: intl.formatMessage(
                                messages.ibmCloudProvider,
                              ),
                              reason: intl.formatMessage(
                                placementReasonMessages[
                                  placementReasonKey(
                                    availability.data.ibmReason,
                                  )
                                ],
                              ),
                            })
                          : intl.formatMessage(
                              messages.ibmCloudProviderDescription,
                            )
                    }
                    icon={ibmCloudLogo}
                    isDisabled={
                      network === "vpn" ||
                      (availability.data ? !availability.data.ibmPublic : true)
                    }
                    onChoose={() => {
                      field.onChange("ibm");
                    }}
                  />
                </ChoiceGroup>
              )}
            />
          ) : null}
          {availability.data && !hasManagedPlacement && !canUseLocalKind ? (
            <Alert
              isInline
              title={intl.formatMessage(messages.noManagedPlacement)}
              variant="warning"
            />
          ) : null}
          {availability.isError ? (
            <Alert
              actionLinks={
                <AlertActionLink
                  onClick={() => {
                    void availability.refetch();
                  }}
                >
                  {intl.formatMessage(messages.retry)}
                </AlertActionLink>
              }
              isInline
              title={intl.formatMessage(messages.placementAvailabilityError)}
              variant="warning"
            />
          ) : null}
          <p className={styles.help}>
            <FormattedMessage {...messages.placementHelp} />
          </p>
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
