import "@patternfly/react-core/dist/styles/base.css";

import {
  ActionGroup,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Label,
  LabelGroup,
  PageSection,
  Radio,
  Switch,
  TextInput,
  Title,
} from "@patternfly/react-core";
import { useState } from "react";
import { SlidersHIcon, StarIcon } from "@patternfly/react-icons";
import awsLogo from "../../../../packages/gateway-management-ui/src/assets/aws-logo.svg";
import ibmCloudLogo from "../../../../packages/gateway-management-ui/src/assets/ibm-cloud.svg";
import "./provision-gateway-option-a.css";

import { MockupShell } from "../shell/mockup-template";

export function RecommendationCard({
  description,
  features,
  provider,
  recommended = false,
  specialized = false,
  version = "1.2.3",
  title,
}: {
  description: string;
  features: string[];
  provider: "aws" | "ibm";
  recommended?: boolean;
  specialized?: boolean;
  version?: string;
  title: string;
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
        <CardTitle >
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
            <Label key={feature} color={featureColors[feature as keyof typeof featureColors]} isCompact>
              {feature}
            </Label>
          ))}
        </LabelGroup>
        <img
          alt={provider === "aws" ? "Amazon Web Services" : "IBM Cloud"}
          height="40"
          src={provider === "aws" ? awsLogo : ibmCloudLogo}
          className={provider === "aws" ? "card-provider-icon card-provider-icon-aws" : "card-provider-icon card-provider-icon-ibm"}
        />
        <p className="gateway-set-card-version">OpenShell version: {version}</p>
        <p className="gateway-set-card-description">{description}</p>
      </CardBody>
    </Card>
  );
}

export function YesNoQuestion({
  id,
  label,
  disabledValue,
  value,
  onChange,
}: {
  id: string;
  label: string;
  disabledValue?: "yes" | "no";
  value?: "yes" | "no";
  onChange: (value: "yes" | "no") => void;
}) {
  return (
    <FormGroup fieldId={id} isInline isRequired label={label} role="radiogroup">
      <Radio
          id={`${id}-yes`}
          isDisabled={disabledValue === "yes"}
          isChecked={value === "yes"}
          label="Yes"
          name={id}
          onChange={() => onChange("yes")}
          value="yes"
      />
      <Radio
          id={`${id}-no`}
          isDisabled={disabledValue === "no"}
          isChecked={value === "no"}
          label="No"
          name={id}
          onChange={() => onChange("no")}
          value="no"
      />
    </FormGroup>
  );
}

export function ProvisionGatewayOptionB() {
  const [name, setName] = useState("");
  const [needsGitLab, setNeedsGitLab] = useState<"yes" | "no">();
  const [needsSecurity, setNeedsSecurity] = useState<"yes" | "no">();
  const [requiresDifferentVersion, setRequiresDifferentVersion] = useState(false);
  const [version, setVersion] = useState("1.2.3");

  const recommendation = needsGitLab === "yes"
    ? {
        description: "For gateways that need to reach GitLab and other internal Red Hat services.",
        features: ["VPN required", "AWS"],
        provider: "aws" as const,
        title: "VPN AWS",
      }
    : needsGitLab === "no" && needsSecurity === "yes"
      ? {
          description: "For workloads that require special security considerations.",
          features: ["Kata", "IBM Cloud", "Public"],
          provider: "ibm" as const,
          specialized: true,
          title: "Public Kata IBM Cloud",
        }
      : needsGitLab === "no" && needsSecurity === "no"
      ? {
          description: "General-purpose workloads with no special network or data needs.",
          features: ["Public", "IBM Cloud"],
          provider: "ibm" as const,
          recommended: true,
          title: "Public IBM Cloud",
        }
      : null;

  return (
    <MockupShell>
      <PageSection data-page-region="header" hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">Provision gateway</Title>
          <p>Configure a new OpenShell gateway.</p>
        </Content>
      </PageSection>
      <PageSection data-page-region="body" hasBodyWrapper={false} isFilled variant="default">
        <Form aria-label="Provision gateway" isWidthLimited>
          <FormGroup isRequired label="Gateway name" fieldId="wizard-gateway-name">
            <TextInput
              id="wizard-gateway-name"
              isRequired
              onChange={(_event, value) => setName(value)}
              value={name}
            />
          </FormGroup>
          <YesNoQuestion
            id="wizard-gitlab-access"
            label="Does the gateway need to reach GitLab or other internal Red Hat services?"
            value={needsGitLab}
            onChange={setNeedsGitLab}
          />
          {needsGitLab === "no" ? (
            <YesNoQuestion
              id="wizard-security"
              label="Do workloads require special security considerations?"
              value={needsSecurity}
              onChange={setNeedsSecurity}
            />
          ) : null}
          {recommendation ? <RecommendationCard {...recommendation} /> : null}
          {recommendation ? (
            <FormGroup fieldId="wizard-version-override">
              <Switch
                id="wizard-version-override"
                isChecked={requiresDifferentVersion}
                label="Requires a different OpenShell version"
                onChange={(_event, checked) => setRequiresDifferentVersion(checked)}
              />
              {requiresDifferentVersion ? (
                <FormSelect
                  aria-label="OpenShell version"
                  className="pf-v6-u-mt-md"
                  id="wizard-openshell-version"
                  onChange={(_event, value) => setVersion(value)}
                  value={version}
                >
                  <FormSelectOption label="OpenShell version 1.2.3" value="1.2.3" />
                  <FormSelectOption label="OpenShell version 1.2.2" value="1.2.2" />
                  <FormSelectOption label="OpenShell version 1.1.0" value="1.1.0" />
                </FormSelect>
              ) : null}
            </FormGroup>
          ) : null}
          <ActionGroup>
            <Button type="submit" variant="primary">Provision gateway</Button>
            <Button type="button" variant="link">Cancel</Button>
          </ActionGroup>
        </Form>
      </PageSection>
    </MockupShell>
  );
}
