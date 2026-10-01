import {
  ActionGroup,
  Button,
  Form,
  FormHelperText,
  FormGroup,
  HelperText,
  HelperTextItem,
  MenuToggle,
  Select,
  SelectOption,
  SelectList,
  TextInput,
} from "@patternfly/react-core";
import { useState } from "react";

import { MockupTemplate } from "../shell/mockup-template";

export function ProvisionGatewayMockup({
  showValidationErrors = false,
}: {
  showValidationErrors?: boolean;
}) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <MockupTemplate
      breadcrumbs={["OpenShell Gateways", "Provision gateway"]}
      contentVariant="secondary"
      description="Configure a new OpenShell gateway."
      showRefresh={false}
      title="Provision gateway"
    >
      <Form isWidthLimited>
        <FormGroup isRequired label="Gateway name" fieldId="gateway-name">
          <TextInput
            id="gateway-name"
            name="gateway-name"
            validated={showValidationErrors ? "error" : "default"}
          />
          {showValidationErrors ? (
            <FormHelperText>
              <HelperText>
                <HelperTextItem screenReaderText="error status" variant="error">
                  This field is required.
                </HelperTextItem>
              </HelperText>
            </FormHelperText>
          ) : null}
        </FormGroup>
        <FormGroup
          isRequired
          label="Cluster"
          fieldId="gateway-cluster"
        >
          <Select
            isOpen={isOpen}
            onOpenChange={setIsOpen}
            toggle={(toggleRef) => (
              <MenuToggle
                onClick={() => setIsOpen((open) => !open)}
                ref={toggleRef}
                isExpanded={isOpen}
                isFullWidth
                status={showValidationErrors ? "danger" : undefined}
              >
                Select a cluster
              </MenuToggle>
            )}
          >
            <SelectList>
              <SelectOption value="hyp0-hub">hyp0-hub</SelectOption>
              <SelectOption value="hyp0-spoke1">hyp0-spoke1</SelectOption>
            </SelectList>
          </Select>
          <FormHelperText>
            <HelperText>
              <HelperTextItem
                screenReaderText={showValidationErrors ? "error status" : undefined}
                variant={showValidationErrors ? "error" : "default"}
              >
                {showValidationErrors
                  ? "This field is required."
                  : "Only clusters with a connected control plane can host a gateway."}
              </HelperTextItem>
            </HelperText>
          </FormHelperText>
        </FormGroup>
        <ActionGroup>
          <Button type="submit" variant="primary">
            Provision gateway
          </Button>
          <Button type="button" variant="link">
            Cancel
          </Button>
        </ActionGroup>
      </Form>
    </MockupTemplate>
  );
}
