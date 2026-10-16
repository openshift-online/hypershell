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
  Gallery,
  Label,
  LabelGroup,
  PageSection,
  TextInput,
  Title,
} from "@patternfly/react-core";
import { useState } from "react";
import { LockIcon, SlidersHIcon, StarIcon } from "@patternfly/react-icons";
import { MockupShell } from "../shell/mockup-template";
import "./provision-gateway-option-a.css";
import awsLogo from "../../../../packages/gateway-management-ui/src/assets/aws-logo.svg";
import ibmCloudLogo from "../../../../packages/gateway-management-ui/src/assets/ibm-cloud.svg";

const gatewaySets = [
  { id: "public-ibm", title: "Public IBM Cloud", features: ["Public", "IBM Cloud"], description: "General-purpose workloads with no special network or data needs.", recommended: true },
  { id: "private-aws", title: "VPN AWS", features: ["VPN required", "AWS"], description: "For gateways that need to reach GitLab and other internal Red Hat services that rely heavily on AWS services or data." },
  { id: "public-aws", title: "Public AWS", features: ["Public", "AWS"], description: "For publicly accessible workloads that rely heavily on AWS services or data." },
  { id: "private-ibm", title: "VPN  IBM Cloud", features: ["VPN required", "IBM Cloud"], description: "For gateways that need to reach GitLab and other internal Red Hat services. For general-purpose workloads." },
  { id: "kata-ibm-public", title: "Public Kata IBM Cloud", features: ["Kata", "IBM Cloud", "Public"], description: "For workloads that require special security considerations.", specialized: true },
] as const;

const featureColors = {
  AWS: "orange",
  IBM: "purple",
  "IBM Cloud": "purple",
  Kata: "green",
  "VPN required": "blue",
  Public: "teal",
} as const;

type GatewaySetCardProps = (typeof gatewaySets)[number] & {
  isSelected: boolean;
  onSelect: () => void;
  recommended?: boolean;
  specialized?: boolean;
  className?: string;
};

function GatewaySetCard({
  description,
  features,
  id,
  isSelected,
  onSelect,
  className,
  recommended,
  specialized,
  title,
}: GatewaySetCardProps) {
  const labelId = `gateway-set-${id}`;

  return (
    <Card className={className} isSelectable isSelected={isSelected} onClick={onSelect}>
      <CardHeader selectableActions={{
          isChecked: isSelected,
          name: "gateway-set",
          onChange: onSelect,
          selectableActionAriaLabelledby: labelId,
          selectableActionProps: { value: id },
          variant: "single",
        }}
      >
        <CardTitle>
          <span id={labelId} className="gateway-set-card-title">{title}</span>
          {recommended ? <Label color="green" icon={<StarIcon />} isCompact>Recommended</Label> : null}
          {specialized ? <Label color="grey" icon={<SlidersHIcon />} isCompact>Specialized</Label> : null}
        </CardTitle>
      </CardHeader>
      <CardBody>
        <LabelGroup aria-label="Features" isCompact>
          {features.map((feature) => (
            <Label
              key={feature}
              color={featureColors[feature]}
              icon={feature === "VPN required" ? <LockIcon /> : undefined}
              isCompact
            >
              {feature}
            </Label>
          ))}
        </LabelGroup>
        {features.some((feature) => feature === "AWS") ? (
          <img alt="Amazon Web Services" height="40" src={awsLogo}  className="card-provider-icon card-provider-icon-aws"/>
        ) : (
          <img alt="IBM Cloud" height="40" src={ibmCloudLogo}  className="card-provider-icon card-provider-icon-ibm"/>
        )}
        <p className="gateway-set-card-version">OpenShell version: 1.2.3</p>
        <p className="gateway-set-card-description">{description}</p>
        {recommended ? <p><strong>Recommended for most gateways.</strong></p> : null}
      </CardBody>
    </Card>
  );
}

export function ProvisionGatewayOptionA() {
  const [selectedSet, setSelectedSet] = useState("public-ibm");

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
          <FormGroup isRequired label="Gateway name" fieldId="option-a-gateway-name">
            <TextInput id="option-a-gateway-name" isRequired />
          </FormGroup>
          <FormGroup isRequired label="Select a set" fieldId="option-a-gateway-set">
            <div className="gateway-set-recommended-row">
              <GatewaySetCard
                {...gatewaySets[0]}
                className="gateway-set-card--recommended"
                isSelected={selectedSet === gatewaySets[0].id}
                onSelect={() => setSelectedSet(gatewaySets[0].id)}
              />
            </div>
            <Gallery hasGutter minWidths={{ default: "250px", md: "300px" }} role="radiogroup">
              {gatewaySets.slice(1).map((gatewaySet) => (
                <GatewaySetCard
                  key={gatewaySet.id}
                  {...gatewaySet}
                  isSelected={selectedSet === gatewaySet.id}
                  onSelect={() => setSelectedSet(gatewaySet.id)}
                />
              ))}
            </Gallery>
          </FormGroup>
          <ActionGroup>
            <Button type="submit" variant="primary">Provision gateway</Button>
            <Button type="button" variant="link">Cancel</Button>
          </ActionGroup>
        </Form>
      </PageSection>
    </MockupShell>
  );
}
