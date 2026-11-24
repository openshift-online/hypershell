import "@patternfly/react-core/dist/styles/base.css";

import {
  ActionGroup,
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  CodeBlock,
  CodeBlockCode,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Dropdown,
  DropdownItem,
  DropdownList,
  Flex,
  FlexItem,
  Form,
  FormGroup,
  FormHelperText,
  FormSection,
  Grid,
  GridItem,
  HelperText,
  HelperTextItem,
  Label,
  LabelGroup,
  List,
  ListItem,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  PageSection,
  Radio,
  Stack,
  StackItem,
  Tab,
  Tabs,
  TabTitleText,
  TextArea,
  TextInput,
  TextInputGroup,
  TextInputGroupMain,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useState } from "react";

import { HealthStatus } from "../common/health-status";
import { stories, storyLinkProps } from "../common/story-links";

/*
 * Cedar policy editor for platform policy admins (issue #1145, section 7).
 * Production should use a real code editor with line numbers and inline
 * diagnostics; @patternfly/react-code-editor is not installed in the mockup
 * workspace, so the mockup uses a monospace TextArea.
 */

export type PlatformPolicyEditorState = "valid" | "invalid";
export type PlatformPolicyEditorPanel = "reference" | "snippets" | "test";
export type PlatformPolicyEditorModal = "insert-snippet";

const validCedar = `// CPU and memory requests must stay within the shared-cluster plan.
forbid (
  principal,
  action in [OpenShell::Action::"CreateSandbox", OpenShell::Action::"ResumeSandbox"],
  resource
)
when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };

forbid (
  principal,
  action in [OpenShell::Action::"CreateSandbox", OpenShell::Action::"ResumeSandbox"],
  resource
)
when { context.memoryRequestBytes > 17179869184 };

// Only members of gpu-users can request more than 1 GPU.
forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.gpuCount > 1 }
unless { principal.roles.contains("gpu-users") };`;

const invalidCedar = `// CPU and memory requests must stay within the shared-cluster plan.
forbid (
  principal,
  action == OpenShell::Action::"CreateSandbox",
  resource
)
when { context.cpuRequest > 4 };

permit (principal, action, resource)
when { principal.roles.contains("platform-admins") };`;

const contextAttributes = [
  ["cpuRequestMillis", "Long", "millicores", "500m = 500"],
  ["cpuLimitMillis", "Long", "millicores", "2 = 2000"],
  ["memoryRequestBytes", "Long", "bytes", "1Gi = 1073741824"],
  ["memoryLimitBytes", "Long", "bytes", "4Gi = 4294967296"],
  ["gpuCount", "Long", "GPUs", "1"],
  ["image", "String", "image reference", '"quay.io/org/img:1"'],
];

const snippets = [
  ["CPU request range", "Minimum and maximum CPU request."],
  ["CPU limit maximum", "Largest CPU limit."],
  ["Memory request range", "Minimum and maximum memory request."],
  ["Memory limit maximum", "Largest memory limit."],
  ["GPU maximum", "Largest GPU count."],
  ["Allowed image registries", "Reject images from other registries."],
  ["Roles that may use GPUs", "Allow GPUs only for some roles."],
];

function ReferencePanel() {
  return (
    <Stack hasGutter>
      <StackItem>
        <Title headingLevel="h3" size="md">
          Statements
        </Title>
        <Content component="p">
          Use <code>forbid</code> only. A request is allowed unless a statement
          forbids it, so a policy can only add restrictions.
        </Content>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          Actions
        </Title>
        <LabelGroup numLabels={5}>
          <Label color="green" isCompact>
            CreateSandbox
          </Label>
          <Label color="green" isCompact>
            StartSandbox
          </Label>
          <Label color="green" isCompact>
            ResumeSandbox
          </Label>
          <Label color="grey" isCompact variant="outline">
            UpdateSandboxPolicy (later)
          </Label>
        </LabelGroup>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          context
        </Title>
        <Table aria-label="Context attributes" variant="compact">
          <Thead>
            <Tr>
              <Th modifier="wrap">Attribute</Th>
              <Th modifier="wrap">Type and example</Th>
            </Tr>
          </Thead>
          <Tbody>
            {contextAttributes.map(([name, type, unit, example]) => (
              <Tr key={name}>
                <Td dataLabel="Attribute">
                  <code>{name}</code>
                </Td>
                <Td dataLabel="Type and example">
                  {type}, {unit}
                  <br />
                  <code>{example}</code>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
        <HelperText>
          <HelperTextItem>
            Missing values are filled with the fleet defaults before evaluation,
            so every attribute is always present.
          </HelperTextItem>
        </HelperText>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          principal and resource
        </Title>
        <DescriptionList isCompact>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <code>principal.subject</code>
            </DescriptionListTerm>
            <DescriptionListDescription>String</DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <code>principal.roles</code>
            </DescriptionListTerm>
            <DescriptionListDescription>
              Set of String
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <code>resource.name</code>
            </DescriptionListTerm>
            <DescriptionListDescription>
              String (gateway name)
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          Not available
        </Title>
        <List isPlain>
          <ListItem>
            <code>permit</code> statements
          </ListItem>
          <ListItem>Policy templates and slots</ListItem>
          <ListItem>Quotas over other sandboxes</ListItem>
          <ListItem>Time or date conditions</ListItem>
          <ListItem>
            <code>ip()</code> conditions, until egress rules ship
          </ListItem>
          <ListItem>
            Your own <code>@</code> annotations (HyperShell adds{" "}
            <code>@id</code>)
          </ListItem>
        </List>
      </StackItem>
    </Stack>
  );
}

function SnippetsPanel({ onInsert }: { onInsert: () => void }) {
  return (
    <Stack hasGutter>
      <StackItem>
        <Content component="p">
          Snippets ask for Kubernetes quantities and insert Cedar that the
          gateway Policies tab can show in plain words.
        </Content>
      </StackItem>
      {snippets.map(([title, description]) => (
        <StackItem key={title}>
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            justifyContent={{ default: "justifyContentSpaceBetween" }}
          >
            <FlexItem>
              <Content component="p">
                <strong>{title}</strong>
              </Content>
              <Content component="small">{description}</Content>
            </FlexItem>
            <FlexItem>
              <Button onClick={onInsert} size="sm" variant="secondary">
                Add
              </Button>
            </FlexItem>
          </Flex>
        </StackItem>
      ))}
    </Stack>
  );
}

function TestPanel() {
  return (
    <Form>
      <Content component="p">
        Evaluate a sample request against this draft. Empty fields use the fleet
        defaults.
      </Content>
      <Grid hasGutter md={6}>
        <GridItem>
          <FormGroup fieldId="test-cpu-request" label="CPU request">
            <TextInput id="test-cpu-request" value="100m" />
          </FormGroup>
        </GridItem>
        <GridItem>
          <FormGroup fieldId="test-memory-request" label="Memory request">
            <TextInput
              id="test-memory-request"
              placeholder="1Gi (default)"
              value=""
            />
          </FormGroup>
        </GridItem>
        <GridItem>
          <FormGroup fieldId="test-gpus" label="GPUs">
            <TextInput id="test-gpus" value="2" />
          </FormGroup>
        </GridItem>
        <GridItem>
          <FormGroup fieldId="test-roles" label="User roles">
            <TextInput id="test-roles" value="developers" />
          </FormGroup>
        </GridItem>
      </Grid>
      <Alert isInline title="Rejected by 2 statements" variant="danger">
        <List>
          <ListItem>
            Line 2: CPU request 100m is below the minimum 250m.
          </ListItem>
          <ListItem>
            Line 16: GPUs 2 is above 1, and the user is not in gpu-users.
          </ListItem>
        </List>
      </Alert>
    </Form>
  );
}

function PreviewCard({ isInvalid }: { isInvalid: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Title headingLevel="h2" size="lg">
            Preview
          </Title>
        </CardTitle>
      </CardHeader>
      <CardBody>
        {isInvalid ? (
          <Content component="p">
            Fix the problems in the rules to see the preview.
          </Content>
        ) : (
          <Grid hasGutter>
            <GridItem lg={6}>
              <Stack hasGutter>
                <StackItem>
                  <Title headingLevel="h3" size="md">
                    On the gateway Policies tab
                  </Title>
                </StackItem>
                <StackItem>
                  <DescriptionList isCompact isHorizontal>
                    <DescriptionListGroup>
                      <DescriptionListTerm>CPU request</DescriptionListTerm>
                      <DescriptionListDescription>
                        250m to 4
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Memory request</DescriptionListTerm>
                      <DescriptionListDescription>
                        Up to 16Gi
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Additional rule</DescriptionListTerm>
                      <DescriptionListDescription>
                        <Label color="grey" isCompact variant="outline">
                          Shown as Cedar
                        </Label>{" "}
                        Line 16 (GPU count with a role condition)
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  </DescriptionList>
                </StackItem>
              </Stack>
            </GridItem>
            <GridItem lg={6}>
              <Stack hasGutter>
                <StackItem>
                  <Title headingLevel="h3" size="md">
                    When OpenShell rejects the test request
                  </Title>
                </StackItem>
                <StackItem>
                  <CodeBlock>
                    <CodeBlockCode>
                      {[
                        "Rejected by 1 policy:",
                        "- sandbox-resource-baseline: CPU request 100m is below the minimum 250m.",
                        "  Another rule in this policy also rejects the request.",
                        "  Sandboxes on shared clusters must set CPU and memory within",
                        "  platform limits. Ask #hypershell-help for an exception.",
                        "Details: https://docs.example.com/hypershell/sandbox-resources",
                      ].join("\n")}
                    </CodeBlockCode>
                  </CodeBlock>
                </StackItem>
                <StackItem>
                  <HelperText>
                    <HelperTextItem>
                      Statements shown as Cedar can only report the policy name
                      and message, not the failed condition.
                    </HelperTextItem>
                  </HelperText>
                </StackItem>
              </Stack>
            </GridItem>
          </Grid>
        )}
      </CardBody>
    </Card>
  );
}

export function PlatformPolicyFormPageSurface({
  modal: initialModal,
  panel: initialPanel = "reference",
  state = "valid",
}: {
  modal?: PlatformPolicyEditorModal;
  panel?: PlatformPolicyEditorPanel;
  state?: PlatformPolicyEditorState;
}) {
  const [panel, setPanel] = useState(initialPanel);
  const [modal, setModal] = useState(initialModal);
  const [isAddOpen, setIsAddOpen] = useState(false);
  const isInvalid = state === "invalid";

  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Title headingLevel="h1" size="2xl">
          Create platform policy
        </Title>
        <Content component="p">
          Write Cedar rules for sandboxes on many gateways. New policies start
          in audit only.
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <Grid hasGutter>
          <GridItem lg={8}>
            <Stack hasGutter>
              <StackItem>
                <Card>
                  <CardBody>
                    <Form>
                      <FormSection title="Rules" titleElement="h2">
                        <Toolbar>
                          <ToolbarContent>
                            <ToolbarItem>
                              <Dropdown
                                isOpen={isAddOpen}
                                onOpenChange={setIsAddOpen}
                                onSelect={() => {
                                  setIsAddOpen(false);
                                  setModal("insert-snippet");
                                }}
                                toggle={(toggleRef) => (
                                  <MenuToggle
                                    isExpanded={isAddOpen}
                                    onClick={() =>
                                      setIsAddOpen((open) => !open)
                                    }
                                    ref={toggleRef}
                                    variant="secondary"
                                  >
                                    Add a rule
                                  </MenuToggle>
                                )}
                              >
                                <DropdownList>
                                  {snippets.map(([title, description]) => (
                                    <DropdownItem
                                      description={description}
                                      key={title}
                                    >
                                      {title}
                                    </DropdownItem>
                                  ))}
                                </DropdownList>
                              </Dropdown>
                            </ToolbarItem>
                            <ToolbarItem>
                              {isInvalid ? (
                                <HealthStatus
                                  appearance="danger"
                                  label="2 problems"
                                />
                              ) : (
                                <HealthStatus label="Valid, 3 statements" />
                              )}
                            </ToolbarItem>
                          </ToolbarContent>
                        </Toolbar>
                        <FormGroup fieldId="policy-cedar" label="Cedar">
                          <TextArea
                            aria-describedby="policy-cedar-help"
                            className="pf-v6-u-font-family-monospace"
                            id="policy-cedar"
                            resizeOrientation="vertical"
                            rows={isInvalid ? 11 : 22}
                            validated={isInvalid ? "error" : "default"}
                            value={isInvalid ? invalidCedar : validCedar}
                          />
                          <FormHelperText>
                            <HelperText id="policy-cedar-help">
                              {isInvalid ? (
                                <HelperTextItem variant="error">
                                  Fix 2 problems before you save.
                                </HelperTextItem>
                              ) : (
                                <>
                                  <HelperTextItem variant="success">
                                    Valid against the HyperShell Cedar schema
                                    v1. All statements use forbid.
                                  </HelperTextItem>
                                  <HelperTextItem variant="success">
                                    The fleet defaults pass: CPU request 500m,
                                    memory request 1Gi, GPUs 0.
                                  </HelperTextItem>
                                </>
                              )}
                            </HelperText>
                          </FormHelperText>
                        </FormGroup>
                        {isInvalid ? (
                          <Alert isInline title="2 problems" variant="danger">
                            <List>
                              <ListItem>
                                Line 7: <code>context.cpuRequest</code> is not
                                available. Use{" "}
                                <code>context.cpuRequestMillis</code> (Long,
                                millicores). 4 CPU is 4000.
                              </ListItem>
                              <ListItem>
                                Line 9: <code>permit</code> is not allowed.
                                Platform policies can only use{" "}
                                <code>forbid</code>.
                              </ListItem>
                            </List>
                          </Alert>
                        ) : null}
                      </FormSection>
                    </Form>
                  </CardBody>
                </Card>
              </StackItem>
              <StackItem>
                <PreviewCard isInvalid={isInvalid} />
              </StackItem>
              <StackItem>
                <Card>
                  <CardBody>
                    <Form>
                      <FormSection title="Details" titleElement="h2">
                        <FormGroup
                          fieldId="policy-name"
                          isRequired
                          label="Name"
                        >
                          <TextInput
                            id="policy-name"
                            isRequired
                            value="sandbox-resource-baseline"
                          />
                        </FormGroup>
                        <FormGroup
                          fieldId="policy-description"
                          label="Description"
                        >
                          <TextInput
                            id="policy-description"
                            value="Shared-cluster capacity plan for sandboxes."
                          />
                        </FormGroup>
                        <FormGroup
                          fieldId="policy-message"
                          isRequired
                          label="Message for users"
                        >
                          <TextArea
                            id="policy-message"
                            isRequired
                            resizeOrientation="vertical"
                            value="Sandboxes on shared clusters must set CPU and memory within platform limits. Ask #hypershell-help for an exception."
                          />
                          <FormHelperText>
                            <HelperText>
                              <HelperTextItem>
                                Shown on the gateway Policies tab and in every
                                rejection from this policy.
                              </HelperTextItem>
                              <HelperTextItem variant="warning">
                                Everyone who uses an affected gateway can read
                                this message. Do not include secrets,
                                credentials, or internal hostnames.
                              </HelperTextItem>
                            </HelperText>
                          </FormHelperText>
                        </FormGroup>
                        <FormGroup
                          fieldId="policy-docs"
                          label="Documentation URL"
                        >
                          <TextInput
                            id="policy-docs"
                            type="url"
                            value="https://docs.example.com/hypershell/sandbox-resources"
                          />
                        </FormGroup>
                      </FormSection>
                      <FormSection title="Applies to" titleElement="h2">
                        <FormGroup
                          fieldId="applies-all"
                          isRequired
                          label="Gateways"
                          role="radiogroup"
                        >
                          <Radio
                            id="applies-all"
                            label="All gateways"
                            name="applies"
                          />
                          <Radio
                            id="applies-except"
                            isChecked
                            label="All gateways except"
                            name="applies"
                          />
                          <Radio
                            id="applies-selected"
                            label="Selected gateways"
                            name="applies"
                          />
                        </FormGroup>
                        <FormGroup
                          fieldId="applies-except-gateways"
                          label="Excluded gateways"
                        >
                          <MenuToggle isFullWidth variant="typeahead">
                            <TextInputGroup isPlain>
                              <TextInputGroupMain
                                aria-label="Find gateways to exclude"
                                id="applies-except-gateways"
                                placeholder="Find gateways"
                                value=""
                              >
                                <LabelGroup aria-label="Excluded gateways">
                                  <Label isCompact onClose={() => undefined}>
                                    ml-training-gateway
                                  </Label>
                                </LabelGroup>
                              </TextInputGroupMain>
                            </TextInputGroup>
                          </MenuToggle>
                        </FormGroup>
                      </FormSection>
                      <ActionGroup>
                        <Button variant="primary">Create policy</Button>
                        <Button
                          component="a"
                          {...storyLinkProps(stories.platformPolicies)}
                          variant="link"
                        >
                          Cancel
                        </Button>
                      </ActionGroup>
                    </Form>
                  </CardBody>
                </Card>
              </StackItem>
            </Stack>
          </GridItem>
          <GridItem lg={4}>
            <Card>
              <CardBody>
                <Tabs
                  activeKey={panel}
                  aria-label="Cedar reference, snippets, and test"
                  onSelect={(_event, key) =>
                    setPanel(key as PlatformPolicyEditorPanel)
                  }
                >
                  <Tab
                    eventKey="reference"
                    title={<TabTitleText>What you can use</TabTitleText>}
                  >
                    <ReferencePanel />
                  </Tab>
                  <Tab
                    eventKey="snippets"
                    title={<TabTitleText>Snippets</TabTitleText>}
                  >
                    <SnippetsPanel
                      onInsert={() => setModal("insert-snippet")}
                    />
                  </Tab>
                  <Tab
                    eventKey="test"
                    title={<TabTitleText>Test</TabTitleText>}
                  >
                    <TestPanel />
                  </Tab>
                </Tabs>
              </CardBody>
            </Card>
          </GridItem>
        </Grid>
      </PageSection>
      {modal === "insert-snippet" ? (
        <Modal
          isOpen
          onClose={() => setModal(undefined)}
          variant={ModalVariant.small}
        >
          <ModalHeader
            description="Enter Kubernetes quantities. HyperShell converts them for Cedar."
            title="Add rule: CPU request range"
          />
          <ModalBody>
            <Form>
              <Grid hasGutter md={6}>
                <GridItem>
                  <FormGroup fieldId="snippet-min" label="Minimum">
                    <TextInput id="snippet-min" value="250m" />
                  </FormGroup>
                </GridItem>
                <GridItem>
                  <FormGroup fieldId="snippet-max" label="Maximum">
                    <TextInput id="snippet-max" value="4" />
                  </FormGroup>
                </GridItem>
              </Grid>
              <FormGroup
                fieldId="snippet-actions"
                label="Applies to operations"
                role="group"
              >
                <LabelGroup>
                  <Label color="blue" isCompact>
                    CreateSandbox
                  </Label>
                  <Label color="blue" isCompact>
                    ResumeSandbox
                  </Label>
                </LabelGroup>
              </FormGroup>
              <FormGroup fieldId="snippet-preview" label="Cedar to insert">
                <CodeBlock id="snippet-preview">
                  <CodeBlockCode>
                    {[
                      "forbid (",
                      "  principal,",
                      '  action in [OpenShell::Action::"CreateSandbox", OpenShell::Action::"ResumeSandbox"],',
                      "  resource",
                      ")",
                      "when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };",
                    ].join("\n")}
                  </CodeBlockCode>
                </CodeBlock>
              </FormGroup>
            </Form>
          </ModalBody>
          <ModalFooter>
            <Button onClick={() => setModal(undefined)} variant="primary">
              Insert rule
            </Button>
            <Button onClick={() => setModal(undefined)} variant="link">
              Cancel
            </Button>
          </ModalFooter>
        </Modal>
      ) : null}
    </>
  );
}
