import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionToggle,
  Alert,
  Button,
  Content,
  Flex,
  FlexItem,
  Form,
  FormGroup,
  Grid,
  GridItem,
  HelperText,
  HelperTextItem,
  Label,
  Skeleton,
  Spinner,
  Stack,
  StackItem,
  TextInput,
} from "@patternfly/react-core";
import { CopyIcon, ExternalLinkAltIcon } from "@patternfly/react-icons";
import { FormattedMessage, useIntl } from "react-intl";
import { useEffect, useRef, useState } from "react";

import { messages } from "../messages";
import { externalLinkUrls } from "../application/external-links";
import { CommandBlock, sanitizeFieldValue } from "./editable-command";
import {
  buildGatewayAddCommand,
  buildOpenShellInstallCommand,
  isGatewayReadyToConnect,
  type GatewayConnection,
  sandboxConnectDocsUrl,
  shellArgument,
} from "./gateway-connections";
import styles from "./gateway-connection-steps.module.css";

type ConnectionStepId =
  | "install-cli"
  | "add-gateway"
  | "create-profile"
  | "import-profile"
  | "create-provider"
  | "create-sandbox"
  | "open-claude";

const enmaasDocsUrl = externalLinkUrls.enmaasDocs;
const enmaasProviderName = "my-enmaas";
const enmaasModel = "rits/zai-org/glm-5-3";
const enmaasSandboxName = "enmaas-claude";
const enmaasHostedImage =
  "quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-agent-runtime-main@sha256:0d7f71e4c078f5e637e6ea03e5f53b317dc65d6c9765c7810f27dd111cca6b64";

function buildEnmaasProfileCommand(): string {
  return [
    "cat > enmaas.yaml <<'YAML'",
    "id: enmaas",
    "display_name: EnMaaS",
    "description: Anthropic-compatible EnMaaS API",
    "category: inference",
    "inference_capable: true",
    "credentials:",
    "  - name: api_key",
    "    description: EnMaaS API key used by Claude Code",
    "    env_vars: [ANTHROPIC_API_KEY]",
    "    required: true",
    "    auth_style: header",
    "    header_name: x-api-key",
    "discovery:",
    "  credentials: [api_key]",
    "endpoints:",
    "  - host: api.enmaas.devshift.net",
    "    port: 443",
    "    protocol: rest",
    "    access: read-write",
    "    enforcement: enforce",
    "binaries:",
    "  - /usr/local/bin/claude",
    "YAML",
  ].join("\n");
}

function buildEnmaasProfileImportCommand(): string {
  return [
    "if ! openshell profile describe enmaas --global >/dev/null 2>&1; then",
    "  openshell profile lint -f enmaas.yaml",
    "  openshell profile import -f enmaas.yaml --global",
    "fi",
  ].join("\n");
}

function buildEnmaasProviderCreateCommand(
  gatewayName: string,
  providerName: string,
): string {
  return [
    `if ! openshell provider get -g ${shellArgument(gatewayName)} ${shellArgument(providerName)} >/dev/null 2>&1; then`,
    '  if [[ -z "${ENMAAS_KEY:-}" ]]; then',
    "    printf 'Enter ENMAAS_KEY: '",
    "    read -rs ENMAAS_KEY",
    "    printf '\\n'",
    "    export ENMAAS_KEY",
    "  fi",
    "",
    '  : "${ENMAAS_KEY:?ENMAAS_KEY must not be empty}"',
    "",
    '  ANTHROPIC_API_KEY="$ENMAAS_KEY" openshell provider create \\',
    `    -g ${shellArgument(gatewayName)} \\`,
    `    --name ${shellArgument(providerName)} \\`,
    "    --type enmaas \\",
    "    --global-profile",
    "    --from-existing",
    "fi",
  ].join("\n");
}

function buildOpenClaudeCommand(name: string): string {
  return `openshell sandbox connect ${shellArgument(name)}`;
}

function buildEnmaasSandboxCommand(
  gatewayName: string,
  name: string,
  model: string,
  providerName: string,
): string {
  const settings = JSON.stringify({
    modelPicker: {
      options: [
        {
          model,
          label: "EnMaaS model",
          description: "Hosted via EnMaaS",
        },
      ],
    },
  });

  return [
    "openshell sandbox create \\",
    `  --name ${shellArgument(name)} \\`,
    `  --from ${shellArgument(enmaasHostedImage)} \\`,
    "  --cpu 500m \\",
    "  --memory 512Mi \\",
    `  --provider ${shellArgument(providerName)} \\`,
    `  -g ${shellArgument(gatewayName)} \\`,
    `  --env=ANTHROPIC_BASE_URL=${externalLinkUrls.enmaasApiEndpoint} \\`,
    "  --env=CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1 \\",
    "  --no-auto-providers \\",
    `  -- bash -lc "claude --bare --settings ${shellArgument(settings)} --model ${shellArgument(model)}; exec bash -l"`,
  ].join("\n");
}

function buildGatewaySelectCommand(gateway: GatewayConnection) {
  const gatewayAdd = buildGatewayAddCommand(gateway);
  if (!gatewayAdd) {
    return undefined;
  }
  const gatewayMatch = `"name": "${gateway.name}"`;

  return [
    `if ! openshell gateway list -o json | grep -qF -- ${shellArgument(gatewayMatch)}; then`,
    `  ${gatewayAdd.replaceAll("\n", "\n  ")}`,
    "fi",
    `openshell gateway select ${shellArgument(gateway.name)}`,
  ].join("\n");
}

export function GatewayConnectionSteps({
  gateway,
  isProvisioning,
}: {
  gateway: GatewayConnection;
  isProvisioning?: boolean;
}) {
  const intl = useIntl();
  const [expandedStep, setExpandedStep] = useState<ConnectionStepId | "">("");
  const [providerName, setProviderName] = useState(enmaasProviderName);
  const [model, setModel] = useState(enmaasModel);
  const [sandboxName, setSandboxName] = useState(enmaasSandboxName);
  const [allCommandsResult, setAllCommandsResult] = useState<
    "copied" | "error" | undefined
  >();
  const copyResetTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    return () => {
      window.clearTimeout(copyResetTimer.current);
    };
  }, []);

  const gatewayReady = !isProvisioning && isGatewayReadyToConnect(gateway);
  const installCommand = buildOpenShellInstallCommand(gateway);
  const gatewayCommand = gatewayReady
    ? buildGatewaySelectCommand(gateway)
    : undefined;
  const profileCommand = buildEnmaasProfileCommand();
  const importCommand = gatewayReady
    ? buildEnmaasProfileImportCommand()
    : undefined;
  const providerCommand = gatewayReady
    ? buildEnmaasProviderCreateCommand(gateway.name, providerName)
    : undefined;
  const sandboxCommand = gatewayReady
    ? buildEnmaasSandboxCommand(gateway.name, sandboxName, model, providerName)
    : undefined;
  const openClaudeCommand = gatewayReady
    ? buildOpenClaudeCommand(sandboxName)
    : undefined;

  const connectionSteps: readonly {
    command?: string;
    description: string;
    id: ConnectionStepId;
    requiresGateway?: boolean;
    timing: "every-time" | "one-time";
    title: string;
  }[] = [
    {
      command: installCommand,
      description: intl.formatMessage(
        messages.connectionInstallStepDescription,
      ),
      id: "install-cli",
      timing: "one-time",
      title: intl.formatMessage(messages.connectionInstallStepTitle),
    },
    {
      command: gatewayCommand,
      description: intl.formatMessage(messages.connectionAddGatewayDescription),
      id: "add-gateway",
      requiresGateway: true,
      timing: "one-time",
      title: intl.formatMessage(messages.connectionAddGatewayTitle),
    },
    {
      command: profileCommand,
      description: intl.formatMessage(
        messages.connectionCreateProfileDescription,
      ),
      id: "create-profile",
      timing: "one-time",
      title: intl.formatMessage(messages.connectionCreateProfileTitle),
    },
    {
      command: importCommand,
      description: intl.formatMessage(
        messages.connectionImportProfileDescription,
      ),
      id: "import-profile",
      requiresGateway: true,
      timing: "one-time",
      title: intl.formatMessage(messages.connectionImportProfileTitle),
    },
    {
      command: providerCommand,
      description: intl.formatMessage(
        messages.connectionConnectEnmaasDescription,
      ),
      id: "create-provider",
      requiresGateway: true,
      timing: "one-time",
      title: intl.formatMessage(messages.connectionConnectEnmaasTitle),
    },
    {
      command: sandboxCommand,
      description: intl.formatMessage(messages.connectionSandboxDescription),
      id: "create-sandbox",
      requiresGateway: true,
      timing: "every-time",
      title: intl.formatMessage(messages.connectionSandboxTitle),
    },
    {
      command: openClaudeCommand,
      description: intl.formatMessage(messages.connectionOpenClaudeDescription),
      id: "open-claude",
      requiresGateway: true,
      timing: "every-time",
      title: intl.formatMessage(messages.connectionOpenClaudeTitle),
    },
  ];

  const allCommands = connectionSteps
    .map(({ command }) => command)
    .filter((command): command is string => Boolean(command))
    .join("\n\n");

  const copyAllCommands = async () => {
    window.clearTimeout(copyResetTimer.current);
    try {
      await navigator.clipboard.writeText(allCommands);
      setAllCommandsResult("copied");
      copyResetTimer.current = window.setTimeout(() => {
        setAllCommandsResult(undefined);
      }, 1500);
    } catch {
      setAllCommandsResult("error");
    }
  };

  return (
    <Stack hasGutter>
      <StackItem>
        <Alert
          component="h3"
          isInline
          title={intl.formatMessage(messages.connectionBeforeYouBeginTitle)}
          variant="info"
        >
          <Content component="p">
            {intl.formatMessage(messages.connectionBeforeYouBegin)}
          </Content>
          <Button
            aria-label={intl.formatMessage(messages.connectionEnmaasLinkNewTab)}
            component="a"
            href={enmaasDocsUrl}
            icon={<ExternalLinkAltIcon aria-hidden />}
            iconPosition="end"
            isInline
            rel="noopener noreferrer"
            size="sm"
            target="_blank"
            variant="link"
          >
            <FormattedMessage {...messages.connectionEnmaasLink} />
          </Button>
        </Alert>
      </StackItem>
      <StackItem>
        <Flex
          flexWrap={{ default: "wrap" }}
          justifyContent={{ default: "justifyContentFlexEnd" }}
        >
          <FlexItem>
            <span aria-live="polite">
              <Button
                icon={<CopyIcon />}
                isDanger={allCommandsResult === "error"}
                onClick={() => {
                  void copyAllCommands();
                }}
                variant="secondary"
              >
                {allCommandsResult === "copied"
                  ? intl.formatMessage(messages.copied)
                  : allCommandsResult === "error"
                    ? intl.formatMessage(messages.clipboardCopyFailed)
                    : intl.formatMessage(messages.copyAllConnectionCommands)}
              </Button>
            </span>
          </FlexItem>
        </Flex>
      </StackItem>
      <StackItem>
        <Accordion className={styles.connectionGuide} isBordered>
          {connectionSteps.map(
            (
              { command, description, id, requiresGateway, timing, title },
              index,
            ) => (
              <AccordionItem isExpanded={expandedStep === id} key={id}>
                <AccordionToggle
                  id={`${id}-toggle`}
                  onClick={() => {
                    setExpandedStep(expandedStep === id ? "" : id);
                  }}
                >
                  <span className={styles.connectionStepTitle}>
                    <span className={styles.connectionStepNumber}>
                      {index + 1}
                    </span>
                    <span>{title}</span>
                    {timing === "one-time" ? (
                      <Label color="grey" isCompact variant="outline">
                        <FormattedMessage {...messages.connectionOneTime} />
                      </Label>
                    ) : null}
                  </span>
                </AccordionToggle>
                <AccordionContent id={`${id}-content`}>
                  <Stack hasGutter>
                    <StackItem>
                      <Content component="p">{description}</Content>
                    </StackItem>
                    {id === "install-cli" && !command ? (
                      <StackItem>
                        <Skeleton width="65%" />
                      </StackItem>
                    ) : null}
                    {requiresGateway && !gatewayReady && expandedStep === id ? (
                      <StackItem>
                        <Content component="p">
                          <Spinner isInline size="md" />
                          &#32;
                          <FormattedMessage
                            {...messages.connectionWaitingForProvisioning}
                          />
                        </Content>
                      </StackItem>
                    ) : null}
                    {id === "create-provider" && gatewayReady ? (
                      <StackItem>
                        <Form className={styles.commandSettings}>
                          <Grid hasGutter>
                            <GridItem lg={4} span={12}>
                              <FormGroup
                                fieldId="connection-provider-name"
                                label={intl.formatMessage(
                                  messages.connectionProviderNameLabel,
                                )}
                              >
                                <TextInput
                                  id="connection-provider-name"
                                  onChange={(_event, value) => {
                                    setProviderName(sanitizeFieldValue(value));
                                  }}
                                  value={providerName}
                                />
                              </FormGroup>
                            </GridItem>
                          </Grid>
                          <HelperText>
                            <HelperTextItem>
                              <FormattedMessage
                                {...messages.connectionProviderNameHelper}
                              />
                            </HelperTextItem>
                          </HelperText>
                        </Form>
                      </StackItem>
                    ) : null}
                    {id === "create-sandbox" && gatewayReady ? (
                      <StackItem>
                        <Form className={styles.commandSettings}>
                          <Grid hasGutter>
                            <GridItem lg={6} span={12}>
                              <FormGroup
                                fieldId="connection-sandbox-name"
                                label={intl.formatMessage(
                                  messages.connectionSandboxNameLabel,
                                )}
                              >
                                <TextInput
                                  id="connection-sandbox-name"
                                  onChange={(_event, value) => {
                                    setSandboxName(sanitizeFieldValue(value));
                                  }}
                                  value={sandboxName}
                                />
                              </FormGroup>
                            </GridItem>
                            <GridItem lg={6} span={12}>
                              <FormGroup
                                fieldId="connection-model-name"
                                label={intl.formatMessage(
                                  messages.connectionModelNameLabel,
                                )}
                              >
                                <TextInput
                                  id="connection-model-name"
                                  onChange={(_event, value) => {
                                    setModel(sanitizeFieldValue(value));
                                  }}
                                  value={model}
                                />
                              </FormGroup>
                            </GridItem>
                          </Grid>
                          <HelperText>
                            <HelperTextItem>
                              <FormattedMessage
                                {...messages.connectionSandboxNameHelper}
                              />
                            </HelperTextItem>
                          </HelperText>
                        </Form>
                      </StackItem>
                    ) : null}
                    {id === "open-claude" ? (
                      <StackItem>
                        <Button
                          aria-label={intl.formatMessage(
                            messages.connectionEditorOptionsLinkNewTab,
                          )}
                          component="a"
                          href={sandboxConnectDocsUrl}
                          icon={<ExternalLinkAltIcon aria-hidden />}
                          iconPosition="end"
                          isInline
                          rel="noopener noreferrer"
                          target="_blank"
                          variant="link"
                        >
                          <FormattedMessage
                            {...messages.connectionEditorOptionsLink}
                          />
                        </Button>
                      </StackItem>
                    ) : null}
                    {command ? (
                      <StackItem>
                        <div className={styles.steps}>
                          <CommandBlock
                            copyAriaLabel={intl.formatMessage(
                              messages.copyConnectionStepCommand,
                              {
                                number: index + 1,
                                title,
                              },
                            )}
                            copyText={command}
                          />
                        </div>
                      </StackItem>
                    ) : null}
                  </Stack>
                </AccordionContent>
              </AccordionItem>
            ),
          )}
        </Accordion>
      </StackItem>
    </Stack>
  );
}
