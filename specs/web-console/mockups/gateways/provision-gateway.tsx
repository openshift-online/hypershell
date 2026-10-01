import {
  ActionGroup,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Form,
  FormGroup,
  Gallery,
  FormHelperText,
  HelperText,
  HelperTextItem,
  Label,
  PageSection,
  TextInput,
} from "@patternfly/react-core";
import { useState } from "react";

import awsLogo from "../../../../packages/gateway-management-ui/src/assets/aws-logo.svg";
import ibmCloudLogo from "../../../../packages/gateway-management-ui/src/assets/ibm-cloud.svg";
import { MockupTemplate } from "../shell/mockup-template";

function Choice({
  title,
  description,
  descriptionLabel,
  value,
  selected,
  icon,
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
  isDisabled?: boolean;
  name: string;
  onChoose: () => void;
}) {
  const labelId = `${name}-${value}-label`;
  return (
    <Card
      isDisabled={isDisabled}
      isSelectable
      isSelected={selected}
      onClick={() => {
        if (!isDisabled) onChoose();
      }}
    >
      <CardHeader
        selectableActions={{
          isChecked: selected,
          isHidden: true,
          name,
          onChange: onChoose,
          selectableActionAriaLabelledby: labelId,
          selectableActionProps: { value },
          variant: "single",
        }}
      >
        <CardTitle>
          <span id={labelId}>{title}</span>
        </CardTitle>
      </CardHeader>
      {description ? (
        <CardBody>
          {descriptionLabel ? (
            <Label isCompact>{descriptionLabel}</Label>
          ) : null}
          {icon ? (
            <span style={{ display: "block", textAlign: "center" }}>
              <img
                alt=""
                height="40"
                src={icon}
                style={{ display: "block", margin: "0 auto 0.75rem" }}
                width="96"
              />
              {description}
            </span>
          ) : (
            <span style={{ display: "block", marginTop: "0.25rem" }}>
              {description}
            </span>
          )}
        </CardBody>
      ) : null}
    </Card>
  );
}

export function ProvisionGatewayMockup({
  showValidationErrors = false,
  showLocalDevelopment = false,
}: {
  showValidationErrors?: boolean;
  showLocalDevelopment?: boolean;
}) {
  const [name, setName] = useState("");
  const [network, setNetwork] = useState<"public" | "vpn">("public");
  const [provider, setProvider] = useState<"aws" | "ibm">("ibm");
  const [localKind, setLocalKind] = useState(showLocalDevelopment);

  return (
    <MockupTemplate
      breadcrumbs={["OpenShell Gateways", "Provision gateway"]}
      contentVariant="secondary"
      description="Configure a new OpenShell gateway."
      showRefresh={false}
      title="Provision gateway"
    >
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <Form aria-label="Provision gateway" isWidthLimited>
          <FormGroup isRequired label="Gateway name" fieldId="gateway-name">
            <TextInput
              id="gateway-name"
              isRequired
              onChange={(_event, value) => setName(value)}
              value={name}
              validated={showValidationErrors && !name ? "error" : "default"}
            />
            {showValidationErrors ? (
              <FormHelperText>
                <HelperText>
                  <HelperTextItem
                    screenReaderText="error status"
                    variant="error"
                  >
                    This field is required.
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            ) : null}
          </FormGroup>
          <FormGroup isRequired label="Network access" fieldId="network-access">
            <Gallery
              hasGutter
              minWidths={{ default: "250px", md: "300px" }}
              role="radiogroup"
            >
              {showLocalDevelopment ? (
                <Choice
                  name="placement"
                  value="local-kind"
                  selected={localKind}
                  title="Local development"
                  description="Run the gateway on the local Kind cluster."
                  onChoose={() => {
                    setLocalKind(true);
                  }}
                />
              ) : null}
              <Choice
                name="placement"
                value="public"
                selected={!localKind && network === "public"}
                title="Public"
                description="Accessible through a public endpoint."
                onChoose={() => {
                  setLocalKind(false);
                  setNetwork("public");
                }}
              />
              <Choice
                name="placement"
                value="vpn"
                selected={!localKind && network === "vpn"}
                title="VPN"
                description="For gateways that need to reach GitLab and other Red Hat internal services."
                descriptionLabel="Red Hat VPN required"
                onChoose={() => {
                  setLocalKind(false);
                  setNetwork("vpn");
                  setProvider("aws");
                }}
              />
            </Gallery>
            <FormHelperText>
              <HelperText>
                <HelperTextItem
                  screenReaderText={
                    showValidationErrors ? "error status" : undefined
                  }
                  variant={showValidationErrors ? "error" : "default"}
                >
                  {showValidationErrors && !localKind && !network
                    ? "This field is required."
                    : ""}
                </HelperTextItem>
              </HelperText>
            </FormHelperText>
          </FormGroup>
          {network ? (
            <FormGroup
              isRequired
              label="Cloud provider"
              fieldId="cloud-provider"
            >
              <Gallery
                hasGutter
                minWidths={{ default: "250px", md: "300px" }}
                role="radiogroup"
              >
                <Choice
                  name="provider"
                  value="aws"
                  selected={provider === "aws"}
                  title="Amazon Web Services"
                  description="For workloads that rely heavily on AWS services or data."
                  icon={awsLogo}
                  onChoose={() => setProvider("aws")}
                />
                <Choice
                  name="provider"
                  value="ibm"
                  selected={provider === "ibm"}
                  title="IBM Cloud"
                  description="The default home for gateways. General-purpose workloads with no special network or data needs."
                  icon={ibmCloudLogo}
                  isDisabled={network === "vpn"}
                  onChoose={() => setProvider("ibm")}
                />
              </Gallery>
            </FormGroup>
          ) : null}
          <p>
            A matching managed cluster is selected at random for the chosen
            network and provider.
          </p>
          <ActionGroup>
            <Button type="submit" variant="primary">
              Provision gateway
            </Button>
            <Button type="button" variant="link">
              Cancel
            </Button>
          </ActionGroup>
        </Form>
      </PageSection>
    </MockupTemplate>
  );
}
