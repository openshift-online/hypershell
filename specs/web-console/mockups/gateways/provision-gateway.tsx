import "@patternfly/react-core/dist/styles/base.css";

import {
  ActionGroup,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  ExpandableSection,
  Form,
  FormGroup,
  FormSection,
  FormSelect,
  FormSelectOption,
  Grid,
  GridItem,
  Label,
  LabelGroup,
  Radio,
  TextInput,
  Tooltip,
} from "@patternfly/react-core";
import { SlidersHIcon, StarIcon } from "@patternfly/react-icons";
import { useState } from "react";

import awsLogo from "../../../../packages/gateway-management-ui/src/assets/aws-logo.svg";
import ibmCloudLogo from "../../../../packages/gateway-management-ui/src/assets/ibm-cloud.svg";
import { MockupShell } from "../shell/mockup-template";
import { ProvisionGatewayPageSurface } from "./provision-gateway-page-surface";
import "./provision-gateway.css";

type Provider = "aws" | "ibm";
type YesNo = "yes" | "no";

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
  options: { description?: string; label: string; value: string }[];
  value: string;
}) {
  return (
    <FormGroup fieldId={id} isInline isRequired label={label} role="radiogroup">
      {options.map((option) => {
        const disabled = disabledOption?.value === option.value;
        const radio = (
          <Radio
            id={`${id}-${option.value}`}
            isChecked={value === option.value}
            isDisabled={disabled}
            label={
              option.description
                ? `${option.label} (${option.description})`
                : option.label
            }
            name={id}
            onChange={() => onChange(option.value)}
            value={option.value}
          />
        );
        return disabled ? (
          <Tooltip
            content={disabledOption.reason}
            key={option.value}
            position="top"
          >
            <span className="provision-choice-tooltip">{radio}</span>
          </Tooltip>
        ) : (
          <span className="provision-choice-tooltip" key={option.value}>
            {radio}
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
  disabledValue?: YesNo;
  id: string;
  label: string;
  onChange: (value: YesNo) => void;
  value: YesNo;
}) {
  return (
    <FormGroup fieldId={id} isInline isRequired label={label} role="radiogroup">
      <Radio
        id={`${id}-yes`}
        isChecked={value === "yes"}
        isDisabled={disabledValue === "yes"}
        label="Yes, use Kata"
        name={id}
        onChange={() => onChange("yes")}
        value="yes"
      />
      <Radio
        id={`${id}-no`}
        isChecked={value === "no"}
        isDisabled={disabledValue === "no"}
        label="No, use the default runtime"
        name={id}
        onChange={() => onChange("no")}
        value="no"
      />
    </FormGroup>
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
  provider: Provider;
  recommended?: boolean;
  specialized?: boolean;
  title: string;
  version?: string;
}) {
  const featureColors = {
    AWS: "orange",
    "IBM Cloud": "purple",
    Kata: "green",
    "VPN required": "blue",
    Public: "teal",
  } as const;

  return (
    <Card className="gateway-set-card--recommended">
      <CardHeader>
        <CardTitle>
          <span className="gateway-set-card-title">{title}</span>
          {recommended ? (
            <Label color="green" icon={<StarIcon />} isCompact>
              Recommended
            </Label>
          ) : null}
          {specialized ? (
            <Label color="grey" icon={<SlidersHIcon />} isCompact>
              Specialized
            </Label>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardBody>
        <LabelGroup aria-label="Gateway set features" isCompact>
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
          alt={provider === "aws" ? "Amazon Web Services" : "IBM Cloud"}
          className={
            provider === "aws"
              ? "card-provider-icon card-provider-icon-aws"
              : "card-provider-icon card-provider-icon-ibm"
          }
          height="40"
          src={provider === "aws" ? awsLogo : ibmCloudLogo}
        />
        {version ? (
          <p className="gateway-set-card-version">
            OpenShell version: {version}
          </p>
        ) : null}
        <p className="gateway-set-card-description">{description}</p>
      </CardBody>
    </Card>
  );
}

export function ProvisionGatewayForm({
  localDevelopmentOnly = false,
}: {
  localDevelopmentOnly?: boolean;
}) {
  const [kata, setKata] = useState<YesNo>("no");
  const [name, setName] = useState("");
  const [needsInternalServices, setNeedsInternalServices] = useState(false);
  const [provider, setProvider] = useState<Provider>("ibm");
  const [version, setVersion] = useState("any");

  const isAws = provider === "aws";
  const isVpn = needsInternalServices;
  const recommendation = {
    description: isVpn
      ? "For gateways that need to reach GitLab and other internal Red Hat services."
      : kata === "yes"
        ? "For workloads that require special security considerations."
        : isAws
          ? "For publicly accessible workloads that rely heavily on AWS services or data."
          : "General-purpose workloads with no special network or data needs.",
    features: [
      isVpn ? "VPN required" : "Public",
      ...(kata === "yes" ? ["Kata"] : []),
      isAws ? "AWS" : "IBM Cloud",
    ],
    provider,
    recommended: !isVpn && !isAws && kata === "no",
    specialized: kata === "yes",
    title: `${isVpn ? "VPN" : "Public"}${kata === "yes" ? " Kata" : ""} ${isAws ? "AWS" : "IBM Cloud"}`,
    version: version === "any" ? undefined : version,
  };

  return (
    <Grid hasGutter>
      <GridItem span={7}>
        <Form aria-label="Provision gateway">
          <FormGroup
            fieldId="chunky-gateway-name"
            isRequired
            label="Gateway name"
          >
            <TextInput
              id="chunky-gateway-name"
              isRequired
              onChange={(_event, value) => setName(value)}
              value={name}
            />
          </FormGroup>
          {localDevelopmentOnly ? (
            <FormGroup
              fieldId="chunky-local-development"
              isRequired
              label="Local development"
              role="radiogroup"
            >
              <Radio
                id="chunky-local-kind"
                isChecked
                label="Use local-kind"
                name="chunky-local-development"
                onChange={() => undefined}
                value="local-kind"
              />
            </FormGroup>
          ) : (
            <>
              <ChoiceQuestion
                id="chunky-internal-services"
                label="Does the gateway need to reach GitLab or other internal Red Hat services?"
                onChange={(value) => {
                  setNeedsInternalServices(value === "yes");
                  if (value === "yes") {
                    setKata("no");
                    setProvider("aws");
                  } else {
                    setProvider("ibm");
                  }
                }}
                options={[
                  {
                    label: "No",
                    value: "no",
                  },
                  {
                    description: "Requires Red Hat VPN",
                    label: "Yes",
                    value: "yes",
                  },
                ]}
                value={needsInternalServices ? "yes" : "no"}
              />
              <ChoiceQuestion
                disabledOption={{
                  reason: isVpn
                    ? "This option is not available behind VPN"
                    : "This option is not available with Public network access",
                  value: isVpn ? "ibm" : "aws",
                }}
                id="chunky-provider"
                label="Cloud provider"
                onChange={(value) => setProvider(value as Provider)}
                options={[
                  {
                    description: "default",
                    label: "IBM",
                    value: "ibm",
                  },
                  {
                    description:
                      "For workloads that rely heavily on AWS services or data",
                    label: "AWS",
                    value: "aws",
                  },
                ]}
                value={provider}
              />
            </>
          )}
          {!localDevelopmentOnly ? (
            <ExpandableSection toggleText="Advanced options">
              <FormSection>
                <YesNoQuestion
                  disabledValue={isVpn || isAws ? "yes" : undefined}
                  id="chunky-kata"
                  label="Is Kata required?"
                  onChange={setKata}
                  value={kata}
                />
                <FormGroup
                  fieldId="chunky-openshell-version"
                  isRequired
                  label="OpenShell version"
                >
                  <FormSelect
                    id="chunky-openshell-version"
                    onChange={(_event, value) => setVersion(value)}
                    value={version}
                  >
                    <FormSelectOption label="Any version" value="any" />
                    <FormSelectOption
                      label="OpenShell version 1.2.3"
                      value="1.2.3"
                    />
                    <FormSelectOption
                      label="OpenShell version 1.2.2"
                      value="1.2.2"
                    />
                    <FormSelectOption
                      label="OpenShell version 1.1.0"
                      value="1.1.0"
                    />
                  </FormSelect>
                </FormGroup>
              </FormSection>
            </ExpandableSection>
          ) : null}
          <ActionGroup>
            <Button type="submit" variant="primary">
              Provision gateway
            </Button>
            <Button type="button" variant="link">
              Cancel
            </Button>
          </ActionGroup>
        </Form>
      </GridItem>
      {!localDevelopmentOnly ? (
        <GridItem span={5}>
          <RecommendationCard {...recommendation} />
        </GridItem>
      ) : null}
    </Grid>
  );
}

export function ProvisionGatewayParityMockup({
  localDevelopmentOnly = false,
}: {
  localDevelopmentOnly?: boolean;
}) {
  return (
    <MockupShell>
      <ProvisionGatewayPageSurface
        localDevelopmentOnly={localDevelopmentOnly}
      />
    </MockupShell>
  );
}
