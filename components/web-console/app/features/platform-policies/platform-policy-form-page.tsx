import {
  CedarCodeBlock,
  GatewayStatus,
  policyMessages,
  policyResourceLabel,
  type PolicyFleetDefaults,
} from "@openshift-online/hypershell-gateway-management-ui";
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
  Spinner,
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
import { useState, type KeyboardEvent } from "react";
import { useIntl, type IntlShape, type MessageDescriptor } from "react-intl";

import type {
  PlatformPolicyEvaluation,
  PlatformPolicyFailureKind,
} from "../../application/platform-policies/platform-policy-types";
import { CedarCodeEditor } from "./cedar-code-editor";
import {
  cedarAvailableActions,
  cedarContextAttributes,
  cedarEntityAttributes,
  cedarLaterActions,
  cedarOperators,
  cedarTokens,
  cedarUnavailableConstructs,
  type CedarReferenceUnit,
  type CedarUnavailableConstruct,
} from "./cedar-reference";
import {
  appendCedar,
  buildCedarSnippet,
  cedarSnippet,
  cedarSnippetOperations,
  cedarSnippets,
  type CedarSnippetError,
  type CedarSnippetField,
  type CedarSnippetId,
  type CedarSnippetValues,
} from "./cedar-snippets";
import { editorMessages as messages } from "./platform-policy-editor-messages";
import {
  cedarStatementHeadline,
  platformPolicyPreviewRows,
  platformPolicyRejections,
  toPlatformPolicyTestRequest,
  type PlatformPolicyFormValues,
  type PlatformPolicyTargetScope,
  type PlatformPolicyTestError,
  type PlatformPolicyTestField,
  type PlatformPolicyTestValues,
} from "./platform-policy-form-model";

export type PlatformPolicyEditorPanel = "reference" | "snippets" | "test";

export interface PlatformPolicySaveFailure {
  kind: PlatformPolicyFailureKind;
  /** Server validation reason, present only for `invalid`. */
  reason?: string;
}

export interface PlatformPolicyFormPageProps {
  cancelHref: string;
  evaluation?: PlatformPolicyEvaluation;
  evaluationFailed: boolean;
  /** Initial side-panel tab, for deep links and design-review stories. */
  initialPanel?: PlatformPolicyEditorPanel;
  /** Snippet form to open on first render. */
  initialSnippet?: CedarSnippetId;
  /** Quantities prefilled in the initial snippet form. */
  initialSnippetValues?: CedarSnippetValues;
  isEditing: boolean;
  isEvaluating: boolean;
  isSaving: boolean;
  onCancel: () => void;
  onSave: () => void;
  onTestValuesChange: (values: PlatformPolicyTestValues) => void;
  onValuesChange: (values: PlatformPolicyFormValues) => void;
  saveFailure?: PlatformPolicySaveFailure;
  schemaVersion?: string;
  testValues: PlatformPolicyTestValues;
  values: PlatformPolicyFormValues;
}

const snippetTitles: Record<CedarSnippetId, MessageDescriptor> = {
  "cpu-limit-max": messages.snippetCpuLimitMax,
  "cpu-request-range": messages.snippetCpuRequestRange,
  "gpu-max": messages.snippetGpuMax,
  "gpu-roles": messages.snippetGpuRoles,
  "image-registries": messages.snippetImageRegistries,
  "memory-limit-max": messages.snippetMemoryLimitMax,
  "memory-request-range": messages.snippetMemoryRequestRange,
};

const snippetDescriptions: Record<CedarSnippetId, MessageDescriptor> = {
  "cpu-limit-max": messages.snippetCpuLimitMaxDescription,
  "cpu-request-range": messages.snippetCpuRequestRangeDescription,
  "gpu-max": messages.snippetGpuMaxDescription,
  "gpu-roles": messages.snippetGpuRolesDescription,
  "image-registries": messages.snippetImageRegistriesDescription,
  "memory-limit-max": messages.snippetMemoryLimitMaxDescription,
  "memory-request-range": messages.snippetMemoryRequestRangeDescription,
};

const snippetFieldLabels: Record<CedarSnippetField, MessageDescriptor> = {
  maximum: messages.snippetMaximum,
  minimum: messages.snippetMinimum,
  registries: messages.snippetRegistries,
  roles: messages.snippetRoles,
};

const errorMessages: Record<
  CedarSnippetError | PlatformPolicyTestError,
  MessageDescriptor
> = {
  "invalid-count": messages.errorInvalidCount,
  "invalid-cpu": messages.errorInvalidCpu,
  "invalid-memory": messages.errorInvalidMemory,
  "minimum-above-maximum": messages.errorMinimumAboveMaximum,
  "missing-value": messages.errorMissingValue,
};

const unitMessages: Record<CedarReferenceUnit, MessageDescriptor> = {
  bytes: messages.unitBytes,
  gpus: messages.unitGpus,
  imageReference: messages.unitImageReference,
  millicores: messages.unitMillicores,
  sandboxName: messages.unitSandboxName,
};

function unavailableText(
  intl: IntlShape,
  construct: CedarUnavailableConstruct,
): React.ReactNode {
  switch (construct) {
    case "permit":
      return intl.formatMessage(messages.unavailablePermit, {
        permit: <code key="permit">{cedarTokens.permit}</code>,
      });
    case "templates":
      return intl.formatMessage(messages.unavailableTemplates);
    case "quotas":
      return intl.formatMessage(messages.unavailableQuotas);
    case "time":
      return intl.formatMessage(messages.unavailableTime);
    case "ip":
      return intl.formatMessage(messages.unavailableIp, {
        ip: <code key="ip">{cedarTokens.ip}</code>,
      });
    case "annotations":
      return intl.formatMessage(messages.unavailableAnnotations, {
        annotation: <code key="annotation">{cedarTokens.annotation}</code>,
        id: <code key="id">{cedarTokens.id}</code>,
      });
  }
}

function ReferencePanel() {
  const intl = useIntl();
  return (
    <Stack hasGutter>
      <StackItem>
        <Title headingLevel="h3" size="md">
          {intl.formatMessage(messages.referenceStatements)}
        </Title>
        <Content component="p">
          {intl.formatMessage(messages.referenceStatementsBody, {
            forbid: <code key="forbid">{cedarTokens.forbid}</code>,
          })}
        </Content>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          {intl.formatMessage(messages.referenceActions)}
        </Title>
        <LabelGroup numLabels={5}>
          {cedarAvailableActions.map((action) => (
            <Label color="green" isCompact key={action}>
              {action}
            </Label>
          ))}
          {cedarLaterActions.map((action) => (
            <Label color="grey" isCompact key={action} variant="outline">
              {intl.formatMessage(messages.referenceActionLater, { action })}
            </Label>
          ))}
        </LabelGroup>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          {intl.formatMessage(messages.referenceContext)}
        </Title>
        <Table
          aria-label={intl.formatMessage(messages.referenceContextAttributes)}
          variant="compact"
        >
          <Thead>
            <Tr>
              <Th modifier="wrap">
                {intl.formatMessage(messages.referenceAttribute)}
              </Th>
              <Th modifier="wrap">
                {intl.formatMessage(messages.referenceTypeExample)}
              </Th>
            </Tr>
          </Thead>
          <Tbody>
            {cedarContextAttributes.map((attribute) => (
              <Tr key={attribute.name}>
                <Td dataLabel={intl.formatMessage(messages.referenceAttribute)}>
                  <code>{attribute.name}</code>
                </Td>
                <Td
                  dataLabel={intl.formatMessage(messages.referenceTypeExample)}
                >
                  {intl.formatMessage(messages.referenceTypeUnit, {
                    type: attribute.type,
                    unit: intl.formatMessage(unitMessages[attribute.unit]),
                  })}
                  <br />
                  <code>{attribute.example}</code>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
        <HelperText>
          <HelperTextItem>
            {intl.formatMessage(messages.referenceDefaultsHelper)}
          </HelperTextItem>
        </HelperText>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          {intl.formatMessage(messages.referencePrincipalResource)}
        </Title>
        <DescriptionList isCompact>
          {cedarEntityAttributes.map((attribute) => (
            <DescriptionListGroup key={attribute.name}>
              <DescriptionListTerm>
                <code>{attribute.name}</code>
              </DescriptionListTerm>
              <DescriptionListDescription>
                {intl.formatMessage(
                  attribute.note === "gatewayName"
                    ? messages.typeGatewayName
                    : attribute.note === "gatewayId"
                      ? messages.typeGatewayId
                      : attribute.type === "setOfString"
                        ? messages.typeSetOfString
                        : messages.typeString,
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ))}
        </DescriptionList>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          {intl.formatMessage(messages.referenceOperators)}
        </Title>
        <LabelGroup numLabels={cedarOperators.length}>
          {cedarOperators.map((operator) => (
            <Label isCompact key={operator} variant="outline">
              <code>{operator}</code>
            </Label>
          ))}
        </LabelGroup>
      </StackItem>
      <StackItem>
        <Title headingLevel="h3" size="md">
          {intl.formatMessage(messages.referenceNotAvailable)}
        </Title>
        <List isPlain>
          {cedarUnavailableConstructs.map((construct) => (
            <ListItem key={construct}>
              {unavailableText(intl, construct)}
            </ListItem>
          ))}
        </List>
      </StackItem>
    </Stack>
  );
}

function SnippetsPanel({ onAdd }: { onAdd: (id: CedarSnippetId) => void }) {
  const intl = useIntl();
  return (
    <Stack hasGutter>
      <StackItem>
        <Content component="p">
          {intl.formatMessage(messages.snippetsIntro)}
        </Content>
      </StackItem>
      {cedarSnippets.map(({ id }) => (
        <StackItem key={id}>
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            justifyContent={{ default: "justifyContentSpaceBetween" }}
          >
            <FlexItem>
              <Content component="p">
                <strong>{intl.formatMessage(snippetTitles[id])}</strong>
              </Content>
              <Content component="small">
                {intl.formatMessage(snippetDescriptions[id])}
              </Content>
            </FlexItem>
            <FlexItem>
              <Button
                aria-label={intl.formatMessage(messages.snippetAddAriaLabel, {
                  snippet: intl.formatMessage(snippetTitles[id]),
                })}
                onClick={() => {
                  onAdd(id);
                }}
                size="sm"
                variant="secondary"
              >
                {intl.formatMessage(messages.snippetAdd)}
              </Button>
            </FlexItem>
          </Flex>
        </StackItem>
      ))}
    </Stack>
  );
}

function SnippetModal({
  initialValues,
  onClose,
  onInsert,
  snippetId,
}: {
  initialValues?: CedarSnippetValues;
  onClose: () => void;
  onInsert: (cedar: string) => void;
  snippetId: CedarSnippetId;
}) {
  const intl = useIntl();
  const definition = cedarSnippet(snippetId);
  const [values, setValues] = useState<CedarSnippetValues>(initialValues ?? {});
  const [isSubmitted, setIsSubmitted] = useState(false);
  const result = buildCedarSnippet(snippetId, values);
  const touched = Object.values(values).some((value) => value.trim());
  const showErrors = isSubmitted || touched;
  const errors = result.ok || !showErrors ? {} : result.errors;
  const isRange = definition.kind !== "list";
  const title = intl.formatMessage(snippetTitles[snippetId]);

  const submit = (event?: React.SyntheticEvent) => {
    event?.preventDefault();
    setIsSubmitted(true);
    if (result.ok) {
      onInsert(result.cedar);
    }
  };

  return (
    <Modal
      aria-labelledby="snippet-modal-title"
      isOpen
      onClose={onClose}
      variant={ModalVariant.small}
    >
      <ModalHeader
        description={intl.formatMessage(messages.snippetModalDescription)}
        labelId="snippet-modal-title"
        title={intl.formatMessage(messages.snippetModalTitle, {
          snippet: title,
        })}
      />
      <ModalBody>
        <Form id="snippet-form" onSubmit={submit}>
          <Grid hasGutter md={isRange && definition.fields.length > 1 ? 6 : 12}>
            {definition.fields.map((field) => {
              const error = errors[field];
              return (
                <GridItem key={field}>
                  <FormGroup
                    fieldId={`snippet-${field}`}
                    label={intl.formatMessage(snippetFieldLabels[field])}
                  >
                    <TextInput
                      id={`snippet-${field}`}
                      onChange={(_event, value) => {
                        setValues((current) => ({
                          ...current,
                          [field]: value,
                        }));
                      }}
                      validated={error ? "error" : "default"}
                      value={values[field] ?? ""}
                    />
                    {error ? (
                      <FormHelperText>
                        <HelperText>
                          <HelperTextItem variant="error">
                            {intl.formatMessage(errorMessages[error])}
                          </HelperTextItem>
                        </HelperText>
                      </FormHelperText>
                    ) : null}
                  </FormGroup>
                </GridItem>
              );
            })}
          </Grid>
          <HelperText>
            <HelperTextItem>
              {intl.formatMessage(
                snippetId === "image-registries"
                  ? messages.snippetRegistriesHelper
                  : snippetId === "gpu-roles"
                    ? messages.snippetRolesHelper
                    : messages.snippetRangeHelper,
              )}
            </HelperTextItem>
          </HelperText>
          <FormGroup
            fieldId="snippet-actions"
            label={intl.formatMessage(messages.snippetOperations)}
            role="group"
          >
            <LabelGroup>
              {cedarSnippetOperations.map((operation) => (
                <Label color="blue" isCompact key={operation}>
                  {operation}
                </Label>
              ))}
            </LabelGroup>
          </FormGroup>
          <FormGroup
            fieldId="snippet-preview"
            label={intl.formatMessage(messages.snippetPreview)}
          >
            {result.ok ? (
              <CedarCodeBlock id="snippet-preview" source={result.cedar} />
            ) : (
              <Content component="small" id="snippet-preview">
                {intl.formatMessage(messages.snippetPreviewEmpty)}
              </Content>
            )}
          </FormGroup>
        </Form>
      </ModalBody>
      <ModalFooter>
        <Button form="snippet-form" type="submit" variant="primary">
          {intl.formatMessage(messages.snippetInsert)}
        </Button>
        <Button onClick={onClose} variant="link">
          {intl.formatMessage(messages.cancel)}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

const testFields: readonly (readonly [
  PlatformPolicyTestField,
  MessageDescriptor,
  keyof PolicyFleetDefaults | undefined,
])[] = Object.freeze([
  ["cpuRequest", messages.testCpuRequest, "cpuRequest"],
  ["cpuLimit", messages.testCpuLimit, "cpuLimit"],
  ["memoryRequest", messages.testMemoryRequest, "memoryRequest"],
  ["memoryLimit", messages.testMemoryLimit, "memoryLimit"],
  ["gpuCount", messages.testGpus, undefined],
  ["image", messages.testImage, undefined],
]);

function rejectionText(
  intl: IntlShape,
  evaluation: PlatformPolicyEvaluation,
  cedar: string,
): readonly string[] {
  return platformPolicyRejections(evaluation, cedar).map((rejection) => {
    const rule = rejection.rule;
    const ruleText = rule
      ? rule.recognized && rule.summary.length
        ? intl.formatList(rule.summary)
        : `${cedarStatementHeadline(rule)} (${intl.formatMessage(policyMessages.shownAsCedar)})`
      : "";
    return rejection.line === undefined
      ? intl.formatMessage(messages.testRejectionStatement, {
          number: rejection.statement + 1,
          rule: ruleText,
        })
      : intl.formatMessage(messages.testRejectionLine, {
          line: rejection.line,
          rule: ruleText,
        });
  });
}

function TestPanel({
  cedar,
  defaults,
  evaluation,
  isEvaluating,
  onChange,
  values,
}: {
  cedar: string;
  defaults: PolicyFleetDefaults;
  evaluation?: PlatformPolicyEvaluation;
  isEvaluating: boolean;
  onChange: (values: PlatformPolicyTestValues) => void;
  values: PlatformPolicyTestValues;
}) {
  const intl = useIntl();
  const { errors } = toPlatformPolicyTestRequest(values);
  const set = (field: PlatformPolicyTestField, value: string) => {
    onChange({ ...values, [field]: value });
  };
  const inputFor = (
    field: PlatformPolicyTestField,
    label: MessageDescriptor,
    defaultKey?: keyof PolicyFleetDefaults,
  ) => {
    const error = errors[field];
    const fallback = defaultKey ? defaults[defaultKey] : undefined;
    return (
      <GridItem key={field}>
        <FormGroup fieldId={`test-${field}`} label={intl.formatMessage(label)}>
          <TextInput
            id={`test-${field}`}
            onChange={(_event, value) => {
              set(field, value);
            }}
            placeholder={
              fallback
                ? intl.formatMessage(messages.testDefaultPlaceholder, {
                    value: fallback,
                  })
                : field === "gpuCount"
                  ? intl.formatMessage(messages.testDefaultPlaceholder, {
                      value: "0",
                    })
                  : undefined
            }
            validated={error ? "error" : "default"}
            value={values[field]}
          />
          {error ? (
            <FormHelperText>
              <HelperText>
                <HelperTextItem variant="error">
                  {intl.formatMessage(errorMessages[error])}
                </HelperTextItem>
              </HelperText>
            </FormHelperText>
          ) : null}
        </FormGroup>
      </GridItem>
    );
  };

  let result: React.ReactNode = null;
  if (evaluation && !evaluation.valid) {
    result = (
      <Content component="p">
        {intl.formatMessage(messages.testInvalidDraft)}
      </Content>
    );
  } else if (!evaluation && isEvaluating) {
    result = (
      <Spinner
        aria-label={intl.formatMessage(messages.testChecking)}
        size="md"
      />
    );
  } else if (evaluation?.requestAllowed) {
    result = (
      <Alert
        isInline
        title={intl.formatMessage(messages.testAllowed)}
        variant="success"
      >
        {intl.formatMessage(messages.testAllowedBody)}
      </Alert>
    );
  } else if (evaluation) {
    const lines = rejectionText(intl, evaluation, cedar);
    result = (
      <Alert
        isInline
        title={intl.formatMessage(messages.testRejected, {
          count: evaluation.rejectingStatements.length,
        })}
        variant="danger"
      >
        <List>
          {lines.map((line) => (
            <ListItem key={line}>{line}</ListItem>
          ))}
        </List>
      </Alert>
    );
  }

  return (
    <Form
      onSubmit={(event) => {
        event.preventDefault();
      }}
    >
      <Content component="p">{intl.formatMessage(messages.testIntro)}</Content>
      <Grid hasGutter md={6}>
        {testFields.map(([field, label, defaultKey]) =>
          inputFor(field, label, defaultKey),
        )}
        <GridItem md={12}>
          <FormGroup
            fieldId="test-roles"
            label={intl.formatMessage(messages.testRoles)}
          >
            <TextInput
              id="test-roles"
              onChange={(_event, value) => {
                set("roles", value);
              }}
              placeholder={intl.formatMessage(messages.testRolesPlaceholder)}
              value={values.roles}
            />
          </FormGroup>
        </GridItem>
      </Grid>
      {result}
    </Form>
  );
}

function PreviewCard({
  cedar,
  evaluation,
}: {
  cedar: string;
  evaluation?: PlatformPolicyEvaluation;
}) {
  const intl = useIntl();
  const isValid = evaluation?.valid === true;
  const rows = evaluation
    ? platformPolicyPreviewRows(evaluation.rendered, cedar)
    : [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Title headingLevel="h2" size="lg">
            {intl.formatMessage(messages.previewTitle)}
          </Title>
        </CardTitle>
      </CardHeader>
      <CardBody>
        {!isValid ? (
          <Content component="p">
            {intl.formatMessage(messages.previewInvalid)}
          </Content>
        ) : (
          <Grid hasGutter>
            <GridItem lg={6}>
              <Stack hasGutter>
                <StackItem>
                  <Title headingLevel="h3" size="md">
                    {intl.formatMessage(messages.previewOnTab)}
                  </Title>
                </StackItem>
                <StackItem>
                  {rows.length ? (
                    <DescriptionList isCompact isHorizontal>
                      {rows.map((row) =>
                        row.kind === "resource" ? (
                          <DescriptionListGroup key={row.resource}>
                            <DescriptionListTerm>
                              {policyResourceLabel(intl, row.resource)}
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              {row.minimum !== undefined &&
                              row.maximum !== undefined
                                ? intl.formatMessage(messages.previewRange, {
                                    maximum: row.maximum,
                                    minimum: row.minimum,
                                  })
                                : row.maximum !== undefined
                                  ? intl.formatMessage(messages.previewUpTo, {
                                      maximum: row.maximum,
                                    })
                                  : intl.formatMessage(
                                      messages.previewAtLeast,
                                      { minimum: row.minimum ?? "" },
                                    )}
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                        ) : (
                          <DescriptionListGroup
                            key={`statement-${String(row.statement)}`}
                          >
                            <DescriptionListTerm>
                              {intl.formatMessage(
                                messages.previewAdditionalRule,
                              )}
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              <Flex
                                alignItems={{ default: "alignItemsCenter" }}
                                gap={{ default: "gapXs" }}
                              >
                                <FlexItem>
                                  <Label
                                    color="grey"
                                    isCompact
                                    variant="outline"
                                  >
                                    {intl.formatMessage(
                                      policyMessages.shownAsCedar,
                                    )}
                                  </Label>
                                </FlexItem>
                                <FlexItem>
                                  {row.line === undefined
                                    ? intl.formatMessage(
                                        messages.previewStatement,
                                        { number: row.statement + 1 },
                                      )
                                    : intl.formatMessage(messages.previewLine, {
                                        line: row.line,
                                      })}
                                </FlexItem>
                              </Flex>
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                        ),
                      )}
                    </DescriptionList>
                  ) : (
                    <Content component="p">
                      {intl.formatMessage(messages.previewNoRules)}
                    </Content>
                  )}
                </StackItem>
              </Stack>
            </GridItem>
            <GridItem lg={6}>
              <Stack hasGutter>
                <StackItem>
                  <Title headingLevel="h3" size="md">
                    {intl.formatMessage(messages.previewDenialHeading)}
                  </Title>
                </StackItem>
                <StackItem>
                  {evaluation.requestAllowed || !evaluation.denialText ? (
                    <Content component="p">
                      {intl.formatMessage(messages.previewAllowed)}
                    </Content>
                  ) : (
                    <CodeBlock>
                      <CodeBlockCode>{evaluation.denialText}</CodeBlockCode>
                    </CodeBlock>
                  )}
                </StackItem>
                <StackItem>
                  <HelperText>
                    <HelperTextItem>
                      {intl.formatMessage(messages.previewDenialHelper)}
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

function GatewayChips({
  ariaLabel,
  gateways,
  inputId,
  onChange,
}: {
  ariaLabel: string;
  gateways: readonly string[];
  inputId: string;
  onChange: (gateways: readonly string[]) => void;
}) {
  const intl = useIntl();
  const [text, setText] = useState("");
  const add = () => {
    const value = text.trim();
    if (value && !gateways.includes(value)) {
      onChange([...gateways, value]);
    }
    setText("");
  };
  return (
    <TextInputGroup>
      <TextInputGroupMain
        aria-label={ariaLabel}
        inputId={inputId}
        inputProps={{
          onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => {
            if (event.key === "Enter") {
              event.preventDefault();
              add();
            }
          },
        }}
        onBlur={add}
        onChange={(_event, value) => {
          setText(value);
        }}
        placeholder={intl.formatMessage(messages.findGatewaysPlaceholder)}
        value={text}
      >
        {gateways.length ? (
          <LabelGroup aria-label={ariaLabel}>
            {gateways.map((gateway) => (
              <Label
                closeBtnAriaLabel={intl.formatMessage(messages.removeGateway, {
                  gateway,
                })}
                isCompact
                key={gateway}
                onClose={() => {
                  onChange(gateways.filter((item) => item !== gateway));
                }}
              >
                {gateway}
              </Label>
            ))}
          </LabelGroup>
        ) : null}
      </TextInputGroupMain>
    </TextInputGroup>
  );
}

function saveFailureAlert(
  intl: IntlShape,
  failure: PlatformPolicySaveFailure,
  name: string,
) {
  if (failure.kind === "denied") {
    return (
      <Alert
        isInline
        title={intl.formatMessage(messages.saveDeniedTitle)}
        variant="danger"
      >
        {intl.formatMessage(messages.saveDeniedBody)}
      </Alert>
    );
  }
  if (failure.kind === "conflicted") {
    return (
      <Alert
        isInline
        title={intl.formatMessage(messages.saveConflictTitle, { name })}
        variant="danger"
      >
        {intl.formatMessage(messages.saveConflictBody)}
      </Alert>
    );
  }
  if (failure.kind === "invalid") {
    return (
      <Alert
        isInline
        title={intl.formatMessage(messages.saveInvalidTitle)}
        variant="danger"
      >
        {failure.reason}
      </Alert>
    );
  }
  return (
    <Alert
      isInline
      title={intl.formatMessage(messages.saveFailedTitle)}
      variant="danger"
    >
      {intl.formatMessage(messages.saveFailedBody)}
    </Alert>
  );
}

/**
 * Create and edit page for a platform policy (WEB-PADM-05..09). A pure view:
 * the route container supplies the draft, the live `/evaluate` result, and the
 * save outcome; this component never fetches.
 */
export function PlatformPolicyFormPage({
  cancelHref,
  evaluation,
  evaluationFailed,
  initialPanel = "reference",
  initialSnippet,
  initialSnippetValues,
  isEditing,
  isEvaluating,
  isSaving,
  onCancel,
  onSave,
  onTestValuesChange,
  onValuesChange,
  saveFailure,
  schemaVersion,
  testValues,
  values,
}: PlatformPolicyFormPageProps) {
  const intl = useIntl();
  const [panel, setPanel] = useState<PlatformPolicyEditorPanel>(initialPanel);
  const [openSnippet, setOpenSnippet] = useState<CedarSnippetId | undefined>(
    initialSnippet,
  );
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [isSubmitted, setIsSubmitted] = useState(false);

  const update = (patch: Partial<PlatformPolicyFormValues>) => {
    onValuesChange({ ...values, ...patch });
  };
  const defaults = evaluation?.defaults ?? {};
  const diagnostics = evaluation?.diagnostics ?? [];
  const hasCedar = values.cedar.trim().length > 0;
  const isInvalid = evaluation !== undefined && !evaluation.valid;
  const forbidsDefaults = evaluation?.forbidsDefaults === true;
  const problemCount = diagnostics.length;
  const nameMissing = values.name.trim() === "";
  const messageMissing = values.userMessage.trim() === "";
  const gatewaysMissing =
    values.targetScope === "selected" && values.targetGateways.length === 0;
  const detailsMissing = nameMissing || messageMissing || gatewaysMissing;
  const isPending =
    isEvaluating || (hasCedar && !evaluation && !evaluationFailed);

  let saveBlocked: MessageDescriptor | undefined;
  if (forbidsDefaults) {
    saveBlocked = messages.saveBlockedDefaults;
  } else if (isInvalid || !hasCedar || evaluationFailed) {
    saveBlocked = messages.saveBlockedInvalid;
  } else if (isPending) {
    saveBlocked = messages.saveBlockedPending;
  } else if (detailsMissing) {
    saveBlocked = messages.saveBlockedFields;
  }

  const defaultsValues = {
    cpu: defaults.cpuRequest ?? "-",
    memory: defaults.memoryRequest ?? "-",
  };
  const statementCount = evaluation?.rendered.length ?? 0;

  let status: React.ReactNode;
  if (evaluationFailed) {
    status = (
      <GatewayStatus
        label={intl.formatMessage(messages.statusUnavailable)}
        status="warning"
      />
    );
  } else if (!evaluation) {
    status = hasCedar ? (
      <GatewayStatus
        label={intl.formatMessage(messages.statusChecking)}
        status="pending"
      />
    ) : null;
  } else if (evaluation.valid) {
    status = (
      <GatewayStatus
        label={intl.formatMessage(messages.statusValid, {
          count: statementCount,
        })}
        status="healthy"
      />
    );
  } else {
    status = (
      <GatewayStatus
        label={intl.formatMessage(messages.statusProblems, {
          count: Math.max(problemCount, 1),
        })}
        status="failed"
      />
    );
  }

  const diagnosticText = (diagnostic: (typeof diagnostics)[number]) => {
    const text =
      diagnostic.line > 0
        ? intl.formatMessage(messages.diagnosticLine, {
            line: diagnostic.line,
            message: diagnostic.message,
          })
        : intl.formatMessage(messages.diagnosticWithoutLine, {
            message: diagnostic.message,
          });
    return diagnostic.suggestion
      ? `${text} ${intl.formatMessage(messages.diagnosticSuggestion, {
          suggestion: diagnostic.suggestion,
        })}`
      : text;
  };

  let cedarHelper: React.ReactNode;
  if (!hasCedar) {
    cedarHelper = (
      <HelperTextItem>{intl.formatMessage(messages.emptyCedar)}</HelperTextItem>
    );
  } else if (evaluationFailed) {
    cedarHelper = (
      <HelperTextItem variant="warning">
        {intl.formatMessage(messages.validationUnavailable)}
      </HelperTextItem>
    );
  } else if (forbidsDefaults) {
    cedarHelper = (
      <HelperTextItem variant="error">
        {intl.formatMessage(messages.defaultsForbidden, defaultsValues)}
      </HelperTextItem>
    );
  } else if (isInvalid) {
    cedarHelper = (
      <HelperTextItem variant="error">
        {intl.formatMessage(messages.fixProblems, {
          count: Math.max(problemCount, 1),
        })}
      </HelperTextItem>
    );
  } else if (evaluation?.valid) {
    cedarHelper = (
      <>
        <HelperTextItem variant="success">
          {schemaVersion
            ? intl.formatMessage(messages.validHelper, {
                version: schemaVersion,
              })
            : intl.formatMessage(messages.validHelperNoVersion)}
        </HelperTextItem>
        <HelperTextItem variant="success">
          {intl.formatMessage(messages.defaultsPass, defaultsValues)}
        </HelperTextItem>
      </>
    );
  }

  const targetOptions: readonly (readonly [
    PlatformPolicyTargetScope,
    MessageDescriptor,
  ])[] = [
    ["all", messages.targetAll],
    ["all-except", messages.targetAllExcept],
    ["selected", messages.targetSelected],
  ];

  const pageTitle = isEditing
    ? intl.formatMessage(messages.headingEdit, { name: values.name })
    : intl.formatMessage(messages.pageTitleCreate);

  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Title headingLevel="h1" size="2xl">
          {pageTitle}
        </Title>
        <Content component="p">
          {intl.formatMessage(
            isEditing
              ? messages.pageDescriptionEdit
              : messages.pageDescriptionCreate,
          )}
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
                    <Form
                      onSubmit={(event) => {
                        event.preventDefault();
                      }}
                    >
                      <FormSection
                        title={intl.formatMessage(messages.rulesSection)}
                        titleElement="h2"
                      >
                        <Toolbar>
                          <ToolbarContent>
                            <ToolbarItem>
                              <Dropdown
                                isOpen={isAddOpen}
                                onOpenChange={setIsAddOpen}
                                onSelect={(_event, value) => {
                                  setIsAddOpen(false);
                                  setOpenSnippet(value as CedarSnippetId);
                                }}
                                toggle={(toggleRef) => (
                                  <MenuToggle
                                    isExpanded={isAddOpen}
                                    onClick={() => {
                                      setIsAddOpen((open) => !open);
                                    }}
                                    ref={toggleRef}
                                    variant="secondary"
                                  >
                                    {intl.formatMessage(messages.addRule)}
                                  </MenuToggle>
                                )}
                              >
                                <DropdownList>
                                  {cedarSnippets.map(({ id }) => (
                                    <DropdownItem
                                      description={intl.formatMessage(
                                        snippetDescriptions[id],
                                      )}
                                      key={id}
                                      value={id}
                                    >
                                      {intl.formatMessage(snippetTitles[id])}
                                    </DropdownItem>
                                  ))}
                                </DropdownList>
                              </Dropdown>
                            </ToolbarItem>
                            {status ? (
                              <ToolbarItem>{status}</ToolbarItem>
                            ) : null}
                          </ToolbarContent>
                        </Toolbar>
                        <FormGroup
                          fieldId="policy-cedar"
                          label={intl.formatMessage(messages.cedarLabel)}
                        >
                          <CedarCodeEditor
                            ariaLabel={intl.formatMessage(
                              messages.cedarEditorAriaLabel,
                            )}
                            code={values.cedar}
                            describedBy="policy-cedar-help policy-cedar-problems"
                            diagnostics={diagnostics.map((diagnostic) => ({
                              line: diagnostic.line,
                              message: diagnosticText(diagnostic),
                            }))}
                            id="policy-cedar"
                            onChange={(cedar) => {
                              update({ cedar });
                            }}
                          />
                          <FormHelperText>
                            <HelperText id="policy-cedar-help">
                              {cedarHelper}
                            </HelperText>
                          </FormHelperText>
                        </FormGroup>
                        {isInvalid && problemCount > 0 ? (
                          <Alert
                            id="policy-cedar-problems"
                            isInline
                            title={intl.formatMessage(messages.problemsTitle, {
                              count: problemCount,
                            })}
                            variant="danger"
                          >
                            <List>
                              {diagnostics.map((diagnostic) => (
                                <ListItem
                                  key={`${String(diagnostic.line)}-${String(diagnostic.statement)}-${diagnostic.message}`}
                                >
                                  {diagnosticText(diagnostic)}
                                </ListItem>
                              ))}
                            </List>
                          </Alert>
                        ) : null}
                      </FormSection>
                    </Form>
                  </CardBody>
                </Card>
              </StackItem>
              <StackItem>
                <PreviewCard cedar={values.cedar} evaluation={evaluation} />
              </StackItem>
              <StackItem>
                <Card>
                  <CardBody>
                    <Form
                      onSubmit={(event) => {
                        event.preventDefault();
                        setIsSubmitted(true);
                        if (!saveBlocked) {
                          onSave();
                        }
                      }}
                    >
                      <FormSection
                        title={intl.formatMessage(messages.detailsSection)}
                        titleElement="h2"
                      >
                        <FormGroup
                          fieldId="policy-name"
                          isRequired
                          label={intl.formatMessage(messages.nameLabel)}
                        >
                          <TextInput
                            id="policy-name"
                            isRequired
                            onChange={(_event, name) => {
                              update({ name });
                            }}
                            readOnlyVariant={isEditing ? "default" : undefined}
                            validated={
                              isSubmitted && nameMissing ? "error" : "default"
                            }
                            value={values.name}
                          />
                          {isEditing || (isSubmitted && nameMissing) ? (
                            <FormHelperText>
                              <HelperText>
                                {isEditing ? (
                                  <HelperTextItem>
                                    {intl.formatMessage(
                                      messages.nameHelperEdit,
                                    )}
                                  </HelperTextItem>
                                ) : (
                                  <HelperTextItem variant="error">
                                    {intl.formatMessage(messages.nameRequired)}
                                  </HelperTextItem>
                                )}
                              </HelperText>
                            </FormHelperText>
                          ) : null}
                        </FormGroup>
                        <FormGroup
                          fieldId="policy-description"
                          label={intl.formatMessage(messages.descriptionLabel)}
                        >
                          <TextInput
                            id="policy-description"
                            onChange={(_event, description) => {
                              update({ description });
                            }}
                            value={values.description}
                          />
                        </FormGroup>
                        <FormGroup
                          fieldId="policy-message"
                          isRequired
                          label={intl.formatMessage(messages.messageLabel)}
                        >
                          <TextArea
                            id="policy-message"
                            isRequired
                            onChange={(_event, userMessage) => {
                              update({ userMessage });
                            }}
                            resizeOrientation="vertical"
                            validated={
                              isSubmitted && messageMissing
                                ? "error"
                                : "default"
                            }
                            value={values.userMessage}
                          />
                          <FormHelperText>
                            <HelperText>
                              <HelperTextItem>
                                {intl.formatMessage(messages.messageHelper)}
                              </HelperTextItem>
                              <HelperTextItem variant="warning">
                                {intl.formatMessage(
                                  messages.messageSecretsWarning,
                                )}
                              </HelperTextItem>
                              {isSubmitted && messageMissing ? (
                                <HelperTextItem variant="error">
                                  {intl.formatMessage(messages.messageRequired)}
                                </HelperTextItem>
                              ) : null}
                            </HelperText>
                          </FormHelperText>
                        </FormGroup>
                        <FormGroup
                          fieldId="policy-docs"
                          label={intl.formatMessage(
                            messages.documentationLabel,
                          )}
                        >
                          <TextInput
                            id="policy-docs"
                            onChange={(_event, documentationUrl) => {
                              update({ documentationUrl });
                            }}
                            type="url"
                            value={values.documentationUrl}
                          />
                        </FormGroup>
                      </FormSection>
                      <FormSection
                        title={intl.formatMessage(messages.appliesToSection)}
                        titleElement="h2"
                      >
                        <FormGroup
                          fieldId="applies-all"
                          isRequired
                          label={intl.formatMessage(messages.gatewaysLabel)}
                          role="radiogroup"
                        >
                          {targetOptions.map(([scope, label]) => (
                            <Radio
                              id={`applies-${scope}`}
                              isChecked={values.targetScope === scope}
                              key={scope}
                              label={intl.formatMessage(label)}
                              name="applies"
                              onChange={() => {
                                update({ targetScope: scope });
                              }}
                            />
                          ))}
                        </FormGroup>
                        {values.targetScope === "all" ? null : (
                          <FormGroup
                            fieldId="applies-gateways"
                            label={intl.formatMessage(
                              values.targetScope === "all-except"
                                ? messages.excludedGateways
                                : messages.selectedGateways,
                            )}
                          >
                            <GatewayChips
                              ariaLabel={intl.formatMessage(
                                values.targetScope === "all-except"
                                  ? messages.findGatewaysExclude
                                  : messages.findGatewaysSelect,
                              )}
                              gateways={values.targetGateways}
                              inputId="applies-gateways"
                              onChange={(targetGateways) => {
                                update({ targetGateways });
                              }}
                            />
                            <FormHelperText>
                              <HelperText>
                                {isSubmitted && gatewaysMissing ? (
                                  <HelperTextItem variant="error">
                                    {intl.formatMessage(
                                      messages.gatewaysRequired,
                                    )}
                                  </HelperTextItem>
                                ) : (
                                  <HelperTextItem>
                                    {intl.formatMessage(
                                      messages.gatewayInputHelper,
                                    )}
                                  </HelperTextItem>
                                )}
                              </HelperText>
                            </FormHelperText>
                          </FormGroup>
                        )}
                      </FormSection>
                      {saveFailure
                        ? saveFailureAlert(intl, saveFailure, values.name)
                        : null}
                      {saveBlocked && (isSubmitted || hasCedar) ? (
                        <HelperText id="policy-save-blocked">
                          <HelperTextItem
                            variant={isSubmitted ? "error" : "default"}
                          >
                            {intl.formatMessage(saveBlocked)}
                          </HelperTextItem>
                        </HelperText>
                      ) : null}
                      <ActionGroup>
                        <Button
                          aria-describedby={
                            saveBlocked ? "policy-save-blocked" : undefined
                          }
                          isAriaDisabled={
                            saveBlocked !== undefined &&
                            saveBlocked !== messages.saveBlockedFields
                          }
                          isLoading={isSaving}
                          type="submit"
                          variant="primary"
                        >
                          {intl.formatMessage(
                            isEditing
                              ? messages.saveChanges
                              : messages.createPolicy,
                          )}
                        </Button>
                        <Button
                          component="a"
                          href={cancelHref}
                          onClick={(event) => {
                            event.preventDefault();
                            onCancel();
                          }}
                          variant="link"
                        >
                          {intl.formatMessage(messages.cancel)}
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
                  aria-label={intl.formatMessage(messages.sidePanelLabel)}
                  onSelect={(_event, key) => {
                    setPanel(key as PlatformPolicyEditorPanel);
                  }}
                >
                  <Tab
                    eventKey="reference"
                    title={
                      <TabTitleText>
                        {intl.formatMessage(messages.tabReference)}
                      </TabTitleText>
                    }
                  >
                    <ReferencePanel />
                  </Tab>
                  <Tab
                    eventKey="snippets"
                    title={
                      <TabTitleText>
                        {intl.formatMessage(messages.tabSnippets)}
                      </TabTitleText>
                    }
                  >
                    <SnippetsPanel onAdd={setOpenSnippet} />
                  </Tab>
                  <Tab
                    eventKey="test"
                    title={
                      <TabTitleText>
                        {intl.formatMessage(messages.tabTest)}
                      </TabTitleText>
                    }
                  >
                    <TestPanel
                      cedar={values.cedar}
                      defaults={defaults}
                      evaluation={evaluation}
                      isEvaluating={isEvaluating}
                      onChange={onTestValuesChange}
                      values={testValues}
                    />
                  </Tab>
                </Tabs>
              </CardBody>
            </Card>
          </GridItem>
        </Grid>
      </PageSection>
      {openSnippet ? (
        <SnippetModal
          initialValues={
            openSnippet === initialSnippet ? initialSnippetValues : undefined
          }
          key={openSnippet}
          onClose={() => {
            setOpenSnippet(undefined);
          }}
          onInsert={(cedar) => {
            update({ cedar: appendCedar(values.cedar, cedar) });
            setOpenSnippet(undefined);
          }}
          snippetId={openSnippet}
        />
      ) : null}
    </>
  );
}
