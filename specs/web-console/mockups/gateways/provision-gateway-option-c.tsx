import "@patternfly/react-core/dist/styles/base.css";

import {
  ActionGroup,
  Button,
  Content,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Grid,
  GridItem,
  ExpandableSection,
  PageSection,
  Radio,
  TextInput,
  Title,
  Tooltip,
} from "@patternfly/react-core";
import { useState } from "react";

import { MockupShell } from "../shell/mockup-template";
import { RecommendationCard, YesNoQuestion } from "./provision-gateway-option-b";

function ChoiceQuestion({
  id,
  label,
  options,
  disabledOption,
  value,
  onChange,
}: {
  id: string;
  label: string;
  options: { description: string; label: string; value: string }[];
  disabledOption?: { reason: string; value: string };
  value: string;
  onChange: (value: string) => void;
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
              label={`${option.label} ${option.description ? `(${option.description})` : ""}`}
              name={id}
              onChange={() => onChange(option.value)}
              value={option.value}
            />
          );
          return disabled ? (
            <Tooltip key={option.value} content={disabledOption.reason} position="top">
              <span style={{ display: "inline-block" }}>{radio}</span>
            </Tooltip>
          ) : (
            <span key={option.value}>{radio}</span>
          );
        
      })}
     
    </FormGroup>
  );
}

export function ProvisionGatewayOptionC() {
  const [name, setName] = useState("");
  const [network, setNetwork] = useState("public");
  const [provider, setProvider] = useState("ibm");
  const [kata, setKata] = useState<"yes" | "no">("no");
  const [version, setVersion] = useState("1.2.3");

  const isVpn = network === "vpn";
  const isAws = provider === "aws";
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
    provider: isAws ? "aws" as const : "ibm" as const,
    recommended: !isVpn && !isAws && kata === "no",
    specialized: kata === "yes",
    title: `${isVpn ? "VPN" : "Public"} ${kata === "yes" ? "Kata " : ""}${isAws ? "AWS" : "IBM Cloud"}`,
    version,
  };

  return (
    <MockupShell>
      <PageSection data-page-region="header" hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">Provision gateway</Title>
          <p>Configure a new OpenShell gateway.</p>
        </Content>
      </PageSection>
      <PageSection data-page-region="body" hasBodyWrapper={false} isFilled variant="default">
        <Grid hasGutter>
          <GridItem span={7}>
            <Form aria-label="Provision gateway">
              <FormGroup isRequired label="Gateway name" fieldId="chunky-gateway-name">
                <TextInput
                  id="chunky-gateway-name"
                  isRequired
                  onChange={(_event, value) => setName(value)}
                  value={name}
                />
              </FormGroup>
              <ChoiceQuestion
                id="chunky-network"
                label="Network access"
                options={[
                  { description: "Accessible through a public endpoint.", label: "Public", value: "public" },
                  { description: "For gateways that need to reach GitLab and other internal Red Hat services.", label: "VPN", value: "vpn" },
                ]}
                value={network}
                onChange={(value) => {
                  setNetwork(value);
                  if (value === "vpn") {
                    setProvider("aws");
                    setKata("no");
                  } else {
                    setProvider("ibm");
                  }
                }}
              />
              <ChoiceQuestion
                id="chunky-provider"
                label="Cloud provider"
                options={[
                  { description: "For workloads that rely heavily on AWS services or data.", label: "AWS", value: "aws" },
                  { description: "The default home for gateways.", label: "IBM", value: "ibm" },
                ]}
                disabledOption={{
                  reason: isVpn
                    ? "This option is not available behind VPN"
                    : "This option is not available with Public network access",
                  value: isVpn ? "ibm" : "aws",
                }}
                value={provider}
                onChange={(value) => {
                  if (!(network === "vpn" && value === "ibm")) {
                    setProvider(value);
                  }
                }}
              />
              <ExpandableSection toggleText="Advanced options">
                <YesNoQuestion
                id="chunky-kata"
                label="Is Kata required?"
                value={kata}
                disabledValue={isVpn || isAws ? "yes" : undefined}
                onChange={setKata}
                />
                <FormGroup isRequired label="OpenShell version" fieldId="chunky-openshell-version">
                  <FormSelect
                    id="chunky-openshell-version"
                    onChange={(_event, value) => setVersion(value)}
                    value={version}
                  >
                    <FormSelectOption label="OpenShell version 1.2.3" value="1.2.3" />
                    <FormSelectOption label="OpenShell version 1.2.2" value="1.2.2" />
                    <FormSelectOption label="OpenShell version 1.1.0" value="1.1.0" />
                  </FormSelect>
                </FormGroup>
              </ExpandableSection>
              <ActionGroup>
                <Button type="submit" variant="primary">Provision gateway</Button>
                <Button type="button" variant="link">Cancel</Button>
              </ActionGroup>
            </Form>
          </GridItem>
          <GridItem span={5}>
            <RecommendationCard {...recommendation} />
          </GridItem>
        </Grid>
      </PageSection>
    </MockupShell>
  );
}
