import {
  ActionGroup,
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  ExpandableSection,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Gallery,
  Grid,
  GridItem,
  FormHelperText,
  HelperText,
  HelperTextItem,
  Label,
  LabelGroup,
  PageSection,
  Radio,
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
import { SlidersHIcon, StarIcon } from "@patternfly/react-icons";
import { Tooltip } from "@patternfly/react-core";
import { z } from "zod";

import { useGatewayUi } from "../gateway-ui-provider";
import type {
  GatewayPlacementAvailability,
  GatewayPlacementAvailabilityItem,
  GatewayProvisionInput,
} from "../application/gateway-types";
import {
  compareGatewayVersions,
  parseGatewayVersion,
} from "../application/gateway-operations";
import { messages } from "../messages";
import { gatewayListQueryRoot, gatewayQueryKey } from "./gateway-data";
import styles from "./gateway-create.module.css";
import awsLogo from "../assets/aws-logo.svg";
import ibmCloudLogo from "../assets/ibm-cloud.svg";

const placementReasonMessages = {
  "no-eligible-cluster": messages.noEligibleCluster,
} as const;

const placementSelectors = {
  awsPublic: ["network/public", "provider/aws"],
  awsVpn: ["network/vpn", "provider/aws"],
  ibmPublic: ["network/public", "provider/ibm"],
  ibmVpn: ["network/vpn", "provider/ibm"],
} as const;
const kataPlacementSelectors = {
  awsPublic: ["network/public", "provider/aws", "runtime/kata"],
  awsVpn: ["network/vpn", "provider/aws", "runtime/kata"],
  ibmPublic: ["network/public", "provider/ibm", "runtime/kata"],
  ibmVpn: ["network/vpn", "provider/ibm", "runtime/kata"],
} as const;
const requestedPlacementSelectors = [
  ...Object.values(placementSelectors),
  ...Object.values(kataPlacementSelectors),
];

const gatewayVersionSelectorPrefix = "gateway-version/";
// The default provisioning-version option omits the gateway-version label.
const anyVersionOption = "";
// The placement-availability request accepts at most 32 selectors. Four base
// network/provider pairs and their Kata variants leave room for the
// version-bearing selectors, so cap the advertised versions we probe. We probe
// the newest versions first (compareProvisioningVersionsDescending), so when a
// fleet advertises more distinct versions than this cap, only the newest ones
// are offered; an older version advertised solely by an out-of-top-N spoke is
// not selectable. One version per spoke makes exceeding this cap unusual, and
// the gateway-provision-placement spec records it as an accepted limitation.
const maxProvisioningVersions = Math.floor(
  (32 - requestedPlacementSelectors.length) /
    requestedPlacementSelectors.length,
);

// Newest first; version tags that cannot be parsed fall back to a stable
// descending string order so the choice is deterministic.
function compareProvisioningVersionsDescending(left: string, right: string) {
  const leftParsed = parseGatewayVersion(left);
  const rightParsed = parseGatewayVersion(right);
  if (leftParsed && rightParsed) {
    return compareGatewayVersions(rightParsed, leftParsed);
  }
  if (leftParsed) {
    return -1;
  }
  if (rightParsed) {
    return 1;
  }
  return right.localeCompare(left);
}

function versionBearingSelector(
  pair: readonly string[],
  version: string,
): string[] {
  return [...pair, `${gatewayVersionSelectorPrefix}${version}`];
}

function placementResult(
  availability: GatewayPlacementAvailability | undefined,
  selector: readonly string[],
): GatewayPlacementAvailabilityItem | undefined {
  return availability?.placements.find(
    (item) =>
      item.selector.length === selector.length &&
      item.selector.every((label) => selector.includes(label)),
  );
}
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
  kata: boolean;
  // Empty string means the default "Any version" (no gateway-version label).
  version: string;
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
  iconDark,
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
  iconDark?: string;
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
                className={[
                  styles.providerLogo,
                  value === "ibm" ? styles.providerLogoIbm : undefined,
                  iconDark ? styles.providerLogoLight : undefined,
                ]
                  .filter((className): className is string =>
                    Boolean(className),
                  )
                  .join(" ")}
                src={icon}
                alt=""
                style={iconPadding ? { padding: iconPadding } : undefined}
              />
              {iconDark ? (
                <img
                  className={styles.providerLogoDark}
                  src={iconDark}
                  alt=""
                  style={iconPadding ? { padding: iconPadding } : undefined}
                />
              ) : null}
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

function ChoiceQuestion({
  disabledOption,
  id,
  label,
  onChange,
  options,
  value,
}: {
  disabledOption?: { reason: string; value: string };
  id: string;
  label: string;
  onChange: (value: string) => void;
  options: {
    description?: string;
    isDisabled?: boolean;
    label: string;
    value: string;
  }[];
  value: string;
}) {
  return (
    <FormGroup fieldId={id} isInline isRequired label={label} role="radiogroup">
      {options.map((option) => {
        const isDisabled =
          option.isDisabled === true || disabledOption?.value === option.value;
        const disabledReason =
          disabledOption?.value === option.value
            ? disabledOption.reason
            : "This option is not available for the selected placement.";
        const radio = (
          <Radio
            id={`${id}-${option.value}`}
            isChecked={value === option.value}
            isDisabled={isDisabled}
            aria-describedby={
              isDisabled ? `${id}-${option.value}-description` : undefined
            }
            label={option.label}
            name={id}
            onChange={() => {
              onChange(option.value);
            }}
            value={option.value}
          />
        );
        return isDisabled ? (
          <Tooltip content={disabledReason} key={option.value} position="top">
            <span className={styles.choiceTooltip}>
              {radio}
              {option.description ? (
                <span className={styles.choiceDescription}>
                  {option.description}
                </span>
              ) : null}
              <span
                className={styles.visuallyHidden}
                id={`${id}-${option.value}-description`}
              >
                {disabledReason}
              </span>
            </span>
          </Tooltip>
        ) : (
          <span className={styles.choiceTooltip} key={option.value}>
            {radio}
            {option.description ? (
              <span className={styles.choiceDescription}>
                {option.description}
              </span>
            ) : null}
          </span>
        );
      })}
    </FormGroup>
  );
}

function YesNoQuestion({
  disabledValue,
  id,
  label,
  onChange,
  value,
}: {
  disabledValue?: "yes" | "no";
  id: string;
  label: string;
  onChange: (value: "yes" | "no") => void;
  value: "yes" | "no";
}) {
  const intl = useIntl();
  return (
    <ChoiceQuestion
      disabledOption={
        disabledValue
          ? {
              reason: intl.formatMessage(messages.kataUnavailableReason),
              value: disabledValue,
            }
          : undefined
      }
      id={id}
      label={label}
      onChange={(next) => {
        onChange(next as "yes" | "no");
      }}
      options={[
        { label: "Yes", value: "yes" },
        { label: "No", value: "no" },
      ]}
      value={value}
    />
  );
}

function RecommendationCard({
  description,
  features,
  provider,
  recommended,
  specialized,
  title,
  version,
}: {
  description: string;
  features: string[];
  provider: "aws" | "ibm";
  recommended: boolean;
  specialized: boolean;
  title: string;
  version: string;
}) {
  const intl = useIntl();
  const featureColors = {
    AWS: "orange",
    "IBM Cloud": "purple",
    Kata: "green",
    "VPN required": "blue",
    Public: "teal",
  } as const;
  return (
    <Card className={styles.recommendationCard}>
      <CardHeader>
        <CardTitle>
          <span className={styles.recommendationTitle}>{title}</span>
          {recommended ? (
            <Label color="green" icon={<StarIcon />} isCompact>
              <FormattedMessage {...messages.recommended} />
            </Label>
          ) : null}
          {specialized ? (
            <Label color="grey" icon={<SlidersHIcon />} isCompact>
              <FormattedMessage {...messages.specialized} />
            </Label>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardBody>
        <LabelGroup
          aria-label={intl.formatMessage(messages.gatewaySetFeatures)}
          isCompact
        >
          {features.map((feature) => (
            <Label
              color={featureColors[feature as keyof typeof featureColors]}
              isCompact
              key={feature}
            >
              {feature}
            </Label>
          ))}
        </LabelGroup>
        <img
          alt={intl.formatMessage(
            provider === "aws"
              ? messages.awsProviderAlt
              : messages.ibmProviderAlt,
          )}
          className={provider === "aws" ? styles.awsLogo : styles.ibmLogo}
          src={provider === "aws" ? awsLogo : ibmCloudLogo}
        />
        <p className={styles.recommendationVersion}>
          <FormattedMessage
            {...messages.openShellVersion}
            values={{ version }}
          />
        </p>
        <p className={styles.recommendationDescription}>{description}</p>
      </CardBody>
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
        kata: z.boolean(),
        version: z.string(),
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
        kata: false,
        version: anyVersionOption,
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

  const getProvisioningVersions =
    gateways.getGatewayProvisioningVersions?.bind(gateways);
  const provisioningVersionsQuery = useQuery({
    queryKey: ["gateway", "provisioning-versions"],
    queryFn:
      getProvisioningVersions === undefined
        ? skipToken
        : ({ signal }) => getProvisioningVersions(signal),
    staleTime: 30_000,
  });
  const candidateVersions = useMemo(
    () =>
      [...(provisioningVersionsQuery.data ?? [])]
        .sort(compareProvisioningVersionsDescending)
        .slice(0, maxProvisioningVersions),
    [provisioningVersionsQuery.data],
  );
  const requestedSelectors = useMemo(
    () => [
      ...requestedPlacementSelectors.map((selector) => [...selector]),
      ...candidateVersions.flatMap((version) =>
        requestedPlacementSelectors.map((selector) =>
          versionBearingSelector(selector, version),
        ),
      ),
    ],
    [candidateVersions],
  );

  const getPlacementAvailability =
    gateways.getGatewayPlacementAvailability?.bind(gateways);
  const availability = useQuery({
    queryKey: ["gateway", "placement-availability", candidateVersions],
    queryFn:
      getPlacementAvailability === undefined
        ? skipToken
        : ({ signal }) => getPlacementAvailability(requestedSelectors, signal),
    staleTime: 30_000,
  });
  const awsPublic = placementResult(
    availability.data,
    placementSelectors.awsPublic,
  );
  const awsVpn = placementResult(availability.data, placementSelectors.awsVpn);
  const ibmPublic = placementResult(
    availability.data,
    placementSelectors.ibmPublic,
  );
  const ibmVpn = placementResult(availability.data, placementSelectors.ibmVpn);
  const awsPublicKata = placementResult(
    availability.data,
    kataPlacementSelectors.awsPublic,
  );
  const awsVpnKata = placementResult(
    availability.data,
    kataPlacementSelectors.awsVpn,
  );
  const ibmPublicKata = placementResult(
    availability.data,
    kataPlacementSelectors.ibmPublic,
  );
  const ibmVpnKata = placementResult(
    availability.data,
    kataPlacementSelectors.ibmVpn,
  );
  const hasManagedPlacement =
    availability.data?.placements.some(({ available }) => available) === true;
  const canUseLocalKind =
    availability.data?.localKind === true && !hasManagedPlacement;
  const defaultToLocalKind = canUseLocalKind && !hasManagedPlacement;
  const publicPlacementAvailable =
    awsPublic?.available === true || ibmPublic?.available === true;
  const vpnPlacementAvailable =
    awsVpn?.available === true || ibmVpn?.available === true;

  const network = useWatch({ control, name: "network" });
  const provider = useWatch({ control, name: "provider" });
  const localKind = useWatch({ control, name: "localKind" });
  const kata = useWatch({ control, name: "kata" });
  const version = useWatch({ control, name: "version" });
  // Versions advertised by an eligible cluster for the selected network and
  // provider, confirmed through the batched availability request (GP-11). An
  // option is never offered unless a connected eligible spoke advertises it.
  const offeredVersions = useMemo(() => {
    if (!availability.data || network === null || provider === null) {
      return [] as string[];
    }
    const pair = kata
      ? [`network/${network}`, `provider/${provider}`, "runtime/kata"]
      : [`network/${network}`, `provider/${provider}`];
    return candidateVersions.filter(
      (candidate) =>
        placementResult(
          availability.data,
          versionBearingSelector(pair, candidate),
        )?.available === true,
    );
  }, [availability.data, candidateVersions, kata, network, provider]);
  const kataAvailability =
    network === "public"
      ? provider === "aws"
        ? awsPublicKata
        : ibmPublicKata
      : provider === "aws"
        ? awsVpnKata
        : ibmVpnKata;
  const kataAvailable = kataAvailability?.available === true;
  const recommendation = {
    description:
      network === "vpn"
        ? intl.formatMessage(messages.vpnNetworkDescription)
        : provider === "aws"
          ? intl.formatMessage(messages.awsProviderDescription)
          : intl.formatMessage(messages.ibmCloudProviderDescription),
    features: [
      network === "vpn" ? "VPN required" : "Public",
      ...(kata ? ["Kata"] : []),
      provider === "aws" ? "AWS" : "IBM Cloud",
    ] as string[],
    provider: provider ?? "ibm",
    recommended: network === "public" && provider === "ibm" && !kata,
    specialized: kata,
    title: `${network === "vpn" ? "VPN" : "Public"}${kata ? " Kata" : ""} ${provider === "aws" ? "AWS" : "IBM Cloud"}`,
    version:
      version === anyVersionOption
        ? (candidateVersions[0] ?? intl.formatMessage(messages.anyVersion))
        : version,
  } as const;

  const submit = handleSubmit((values) => {
    if (values.localKind) {
      createGateway.mutate({
        name: values.name.trim(),
        placement: { mode: "local-kind" },
      });
      return;
    }
    if (values.network === null || values.provider === null) return;
    const selector = [
      `network/${values.network}`,
      `provider/${values.provider}`,
    ];
    if (values.kata && kataAvailable) {
      selector.push("runtime/kata");
    }
    // Only attach a version label the user actively chose and that an eligible
    // spoke still advertises; the default "Any version" omits it entirely.
    if (values.version && offeredVersions.includes(values.version)) {
      selector.push(`${gatewayVersionSelectorPrefix}${values.version}`);
    }
    createGateway.mutate({
      name: values.name.trim(),
      placement: { selector },
    });
  });
  useEffect(() => {
    if (!kataAvailable && kata) {
      setValue("kata", false, { shouldValidate: true });
    }
  }, [kata, kataAvailable, setValue]);
  useEffect(() => {
    if (!availability.data || localKind) return;

    const availableNetworks = [
      awsPublic?.available || ibmPublic?.available ? "public" : null,
      awsVpn?.available || ibmVpn?.available ? "vpn" : null,
    ].filter((value): value is "public" | "vpn" => value !== null);
    const defaultNetwork =
      awsPublic?.available || ibmPublic?.available
        ? "public"
        : (availableNetworks[0] ?? null);

    if (
      defaultNetwork !== null &&
      (network === null || !availableNetworks.includes(network))
    ) {
      setValue("network", defaultNetwork, { shouldValidate: true });
    }
  }, [
    availability.data,
    awsPublic?.available,
    awsVpn?.available,
    ibmPublic?.available,
    ibmVpn?.available,
    localKind,
    network,
    setValue,
  ]);
  useEffect(() => {
    if (!availability.data || localKind) return;
    if (network === "vpn") {
      const availableProvider = ibmVpn?.available
        ? "ibm"
        : awsVpn?.available
          ? "aws"
          : null;
      if (provider === availableProvider) return;
      setValue("provider", availableProvider, {
        shouldValidate: true,
      });
    } else if (
      network === "public" &&
      (provider === null ||
        (provider === "ibm" && !ibmPublic?.available) ||
        (provider === "aws" && !awsPublic?.available))
    ) {
      setValue(
        "provider",
        ibmPublic?.available ? "ibm" : awsPublic?.available ? "aws" : null,
        { shouldValidate: true },
      );
    }
  }, [
    availability.data,
    awsPublic?.available,
    awsVpn?.available,
    ibmPublic?.available,
    ibmVpn?.available,
    localKind,
    network,
    provider,
    setValue,
  ]);
  useEffect(() => {
    // Re-derive on network/provider change: reset to "Any version" whenever the
    // previously selected version is no longer advertised for the new pair.
    if (version !== anyVersionOption && !offeredVersions.includes(version)) {
      setValue("version", anyVersionOption);
    }
  }, [offeredVersions, setValue, version]);
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
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage {...messages.provisionGateway} />
          </Title>
          <p>
            <FormattedMessage {...messages.provisionGatewayDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <Form
          aria-label={intl.formatMessage(messages.provisionGateway)}
          onSubmit={(event) => void submit(event)}
        >
          <Grid hasGutter>
            <GridItem span={7}>
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
              {!canUseLocalKind ? (
                <Controller
                  control={control}
                  name="network"
                  render={({ field }) => (
                    <ChoiceQuestion
                      disabledOption={
                        availability.data && !publicPlacementAvailable
                          ? {
                              reason: intl.formatMessage(
                                messages.unavailableNetwork,
                                {
                                  network: intl.formatMessage(
                                    messages.publicNetwork,
                                  ),
                                  reason: intl.formatMessage(
                                    placementReasonMessages[
                                      placementReasonKey(
                                        awsPublic?.reason,
                                        ibmPublic?.reason,
                                      )
                                    ],
                                  ),
                                },
                              ),
                              value: "no",
                            }
                          : availability.data && !vpnPlacementAvailable
                            ? {
                                reason: intl.formatMessage(
                                  messages.unavailableNetwork,
                                  {
                                    network: intl.formatMessage(
                                      messages.vpnNetwork,
                                    ),
                                    reason: intl.formatMessage(
                                      placementReasonMessages[
                                        placementReasonKey(
                                          awsVpn?.reason,
                                          ibmVpn?.reason,
                                        )
                                      ],
                                    ),
                                  },
                                ),
                                value: "yes",
                              }
                            : undefined
                      }
                      id="internal-services"
                      label={intl.formatMessage(
                        messages.internalServicesQuestion,
                      )}
                      onChange={(value) => {
                        setValue("localKind", false);
                        const selectedNetwork =
                          value === "yes" ? "vpn" : "public";
                        field.onChange(selectedNetwork);
                        setValue(
                          "provider",
                          selectedNetwork === "vpn"
                            ? awsVpn?.available
                              ? "aws"
                              : ibmVpn?.available
                                ? "ibm"
                                : null
                            : ibmPublic?.available
                              ? "ibm"
                              : awsPublic?.available
                                ? "aws"
                                : null,
                        );
                      }}
                      options={[
                        {
                          description: undefined,
                          isDisabled:
                            availability.data === undefined ||
                            !publicPlacementAvailable,
                          label: intl.formatMessage(
                            messages.internalServicesNo,
                          ),
                          value: "no",
                        },
                        {
                          description: intl.formatMessage(
                            messages.vpnRequiresLabel,
                          ),
                          isDisabled:
                            availability.data === undefined ||
                            !vpnPlacementAvailable,
                          label: intl.formatMessage(
                            messages.internalServicesYes,
                          ),
                          value: "yes",
                        },
                      ]}
                      value={
                        field.value === "vpn"
                          ? "yes"
                          : field.value === "public"
                            ? "no"
                            : ""
                      }
                    />
                  )}
                />
              ) : null}
              {network !== null ? (
                <Controller
                  control={control}
                  name="provider"
                  render={({ field }) => (
                    <ChoiceQuestion
                      disabledOption={undefined}
                      id="cloud-provider"
                      label={intl.formatMessage(messages.cloudProvider)}
                      onChange={(value) => {
                        field.onChange(value);
                      }}
                      options={[
                        {
                          description:
                            availability.data &&
                            !(network === "vpn"
                              ? ibmVpn?.available
                              : ibmPublic?.available)
                              ? intl.formatMessage(
                                  messages.unavailableProvider,
                                  {
                                    provider: intl.formatMessage(
                                      messages.ibmCloudProvider,
                                    ),
                                    reason: intl.formatMessage(
                                      placementReasonMessages[
                                        placementReasonKey(
                                          network === "vpn"
                                            ? ibmVpn?.reason
                                            : ibmPublic?.reason,
                                        )
                                      ],
                                    ),
                                  },
                                )
                              : "default",
                          isDisabled:
                            availability.data === undefined ||
                            !(network === "vpn"
                              ? ibmVpn?.available
                              : ibmPublic?.available),
                          label: "IBM",
                          value: "ibm",
                        },
                        {
                          description:
                            availability.data &&
                            !(network === "vpn"
                              ? awsVpn?.available
                              : awsPublic?.available)
                              ? intl.formatMessage(
                                  messages.unavailableProvider,
                                  {
                                    provider: intl.formatMessage(
                                      messages.awsProvider,
                                    ),
                                    reason: intl.formatMessage(
                                      placementReasonMessages[
                                        placementReasonKey(
                                          network === "vpn"
                                            ? awsVpn?.reason
                                            : awsPublic?.reason,
                                        )
                                      ],
                                    ),
                                  },
                                )
                              : intl.formatMessage(
                                  messages.awsProviderDescription,
                                ),
                          isDisabled:
                            availability.data === undefined ||
                            !(network === "vpn"
                              ? awsVpn?.available
                              : awsPublic?.available),
                          label: "AWS",
                          value: "aws",
                        },
                      ]}
                      value={field.value ?? ""}
                    />
                  )}
                />
              ) : null}
              {!localKind && network !== null && provider !== null ? (
                <ExpandableSection
                  toggleText={intl.formatMessage(messages.advancedOptions)}
                >
                  <Controller
                    control={control}
                    name="kata"
                    render={({ field }) => (
                      <YesNoQuestion
                        disabledValue={kataAvailable ? undefined : "yes"}
                        id="kata"
                        label={intl.formatMessage(messages.kataQuestion)}
                        onChange={(value) => {
                          field.onChange(value === "yes");
                        }}
                        value={field.value ? "yes" : "no"}
                      />
                    )}
                  />
                  <Controller
                    control={control}
                    name="version"
                    render={({ field }) => (
                      <FormGroup
                        fieldId="provisioning-version"
                        label={intl.formatMessage(messages.provisioningVersion)}
                      >
                        <FormSelect
                          aria-label={intl.formatMessage(
                            messages.provisioningVersion,
                          )}
                          id="provisioning-version"
                          isDisabled={createGateway.isPending}
                          onChange={(_event, value) => {
                            field.onChange(value);
                          }}
                          value={field.value}
                        >
                          <FormSelectOption
                            key="any-version"
                            label={intl.formatMessage(messages.anyVersion)}
                            value={anyVersionOption}
                          />
                          {offeredVersions.map((offeredVersion) => (
                            <FormSelectOption
                              key={offeredVersion}
                              label={offeredVersion}
                              value={offeredVersion}
                            />
                          ))}
                        </FormSelect>
                      </FormGroup>
                    )}
                  />
                </ExpandableSection>
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
                  isInline
                  title={intl.formatMessage(
                    messages.placementAvailabilityError,
                  )}
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
            </GridItem>
            <GridItem span={5}>
              <RecommendationCard {...recommendation} />
            </GridItem>
          </Grid>
        </Form>
      </PageSection>
    </>
  );
}
