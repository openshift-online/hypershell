// Pipeline now panel (fleet-dashboard-delivery-pipeline.spec FLT-DLV-22,
// FLT-DLV-23): both heads, what is in flight, what happens next, the bundle
// train, and every environment, from the payload alone.

import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  Grid,
  GridItem,
  Spinner,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useIntl, type IntlShape } from "react-intl";

import {
  groupLabel,
  loadingSources,
  nowUnknown,
  type DeliveryData,
  type DeliveryEnvNow,
  type DeliveryNextEvent,
  type DeliveryStageState,
  type DeliveryTrainBundle,
} from "../../../domain/delivery";
import { identiName } from "../../../domain/map/identiname";
import {
  BundleName,
  RichText,
  STATE_LABEL,
  StageStateIcon,
  formatClock,
  formatDuration,
  gateName,
} from "./delivery-format";
import { SOURCE_LABEL } from "./delivery-format";
import { deliveryMessages as m } from "./delivery-messages";
import styles from "./delivery-view.module.css";

/** The separator between the parts of a line, used across the tab. */
const SEPARATOR = " - ";

/** Text with the times and bundles it names (FLT-DLV-24). */
interface Rich {
  readonly text: string;
  readonly times: readonly (string | number | null)[];
  readonly bundles: readonly (string | null)[];
}

function rich(
  text: string,
  times: readonly (string | number | null)[] = [],
  bundles: readonly (string | null)[] = [],
): Rich {
  return { text, times, bundles };
}

interface NowItem extends Rich {
  readonly key: string;
  readonly term: string;
  readonly state: DeliveryStageState;
}

function StateText({
  state,
  text,
}: {
  readonly state: DeliveryStageState;
  readonly text: Rich;
}) {
  const intl = useIntl();
  return (
    <>
      <StageStateIcon state={state} />
      <span className={styles.iconGap}>
        {intl.formatMessage(STATE_LABEL[state])}
      </span>
      {text.text ? (
        <span className={styles.subtle}>
          {SEPARATOR}
          <RichText
            bundles={text.bundles}
            text={text.text}
            times={text.times}
          />
        </span>
      ) : null}
    </>
  );
}

function elapsed(intl: IntlShape, iso: string | null, nowMs: number): string {
  const ms = iso === null ? Number.NaN : Date.parse(iso);
  return Number.isNaN(ms) ? "" : formatDuration(intl, Math.max(0, nowMs - ms));
}

function change(shortSha: string, pr: number | null): string {
  return pr !== null ? `${shortSha} (#${String(pr)})` : shortSha;
}

function alias(digest: string | null, fallback: string | null): string {
  return digest ? identiName(digest) : (fallback ?? "");
}

/** A train bundle's name: its alias, or its components while inferred. */
function bundleName(intl: IntlShape, b: DeliveryTrainBundle): string {
  return b.inferred || !b.digest
    ? intl.formatMessage(m.bundleOf, {
        components: intl.formatList(b.changed.map((c) => c.component)),
      })
    : identiName(b.digest);
}

function gateText(
  intl: IntlShape,
  env: DeliveryEnvNow,
  nowMs: number,
  withBundle: boolean,
): string {
  const values = {
    gate: gateName(intl, env.gate),
    elapsed: elapsed(intl, env.since, nowMs),
    attempt:
      env.attempt !== null && env.attemptOf !== null
        ? intl.formatMessage(m.nowAttempt, {
            n: env.attempt,
            of: env.attemptOf,
          })
        : "",
    bundle: alias(env.digest, env.version),
  };
  return intl.formatMessage(withBundle ? m.nowGateOn : m.nowGate, values);
}

function envText(intl: IntlShape, env: DeliveryEnvNow, nowMs: number): Rich {
  switch (env.state) {
    case "running":
      return rich(gateText(intl, env, nowMs, false));
    case "failed":
      return rich(
        intl.formatMessage(m.nowGateFailed, {
          gate: gateName(intl, env.gate),
        }),
      );
    case "waiting":
      return env.waitingFor.length > 0
        ? rich(
            intl.formatMessage(m.nowWaitingFor, {
              envs: intl.formatList(env.waitingFor),
            }),
          )
        : rich(
            intl.formatMessage(m.nowProposed, {
              bundle: alias(env.proposedDigest, env.proposedVersion),
            }),
            [],
            [env.proposedDigest],
          );
    case "passed":
      return env.passedAt !== null
        ? rich(
            intl.formatMessage(m.nowGatesPassed, {
              time: formatClock(intl, env.passedAt, nowMs),
            }),
            [env.passedAt],
          )
        : rich(intl.formatMessage(m.nowHealthy));
    default:
      return rich("");
  }
}

function inFlight(
  intl: IntlShape,
  data: DeliveryData,
  nowMs: number,
): NowItem[] {
  const now = data.now;
  const items: NowItem[] = now.checks.map((c) => ({
    key: `${c.kind}:${c.component}:${c.shortSha}`,
    term: intl.formatMessage(c.kind === "build" ? m.stageBuild : m.stagePolicy),
    state: "running",
    ...rich(
      intl.formatMessage(m.nowCheckRunning, {
        component: c.component,
        elapsed: elapsed(intl, c.startedAt, nowMs),
        change: change(c.shortSha, c.pr),
      }),
    ),
  }));
  for (const b of data.train) {
    if (b.inferred) {
      items.push({
        key: `infer:${bundleName(intl, b)}`,
        term: intl.formatMessage(m.stageRelease),
        state: "running",
        ...rich(
          intl.formatMessage(m.nowReleasingInferred, {
            bundle: bundleName(intl, b),
            built: formatClock(intl, b.builtAt, nowMs),
            policy:
              b.policyAt !== null
                ? intl.formatMessage(m.nowPolicyPassedAt, {
                    time: formatClock(intl, b.policyAt, nowMs),
                  })
                : "",
          }),
          [b.builtAt, b.policyAt],
        ),
      });
    } else if (!b.locked) {
      items.push({
        key: `pending:${b.digest ?? ""}`,
        term: intl.formatMessage(m.stageGitops),
        state: "waiting",
        ...rich(
          intl.formatMessage(m.nowPendingBundle, {
            bundle: bundleName(intl, b),
            time: formatClock(intl, b.publishedAt, nowMs),
          }),
          [b.publishedAt],
          [b.digest],
        ),
      });
    }
  }
  if (now.bundlePr !== null) {
    items.push({
      key: "bundle-pr",
      term: intl.formatMessage(m.stageGitops),
      state: "running",
      ...rich(
        intl.formatMessage(m.nowBundlePr, {
          number: now.bundlePr.number,
          checks: now.bundlePr.checks ? ` - ${now.bundlePr.checks}` : "",
        }),
      ),
    });
  }
  for (const env of now.environments) {
    if (env.state === "running" || env.state === "failed") {
      items.push({
        key: `env:${env.env}`,
        term: env.env,
        state: env.state,
        ...(env.state === "running"
          ? rich(gateText(intl, env, nowMs, true), [], [env.digest])
          : envText(intl, env, nowMs)),
      });
    }
  }
  // Waiting environments, one line per proposed bundle.
  const waiting = new Map<string, DeliveryEnvNow[]>();
  for (const env of now.environments) {
    if (env.state === "waiting" && env.waitingFor.length > 0) {
      const k = env.proposedDigest ?? env.proposedVersion ?? "";
      waiting.set(k, [...(waiting.get(k) ?? []), env]);
    }
  }
  for (const [k, envs] of waiting) {
    const groups = Array.from(new Set(envs.map((e) => groupLabel(e.group))));
    const first = envs[0];
    items.push({
      key: `waiting:${k}`,
      term: groups.join(", "),
      state: "waiting",
      ...rich(
        intl.formatMessage(m.nowWaitingPromotions, {
          bundle: alias(
            first?.proposedDigest ?? null,
            first?.proposedVersion ?? null,
          ),
        }),
        [],
        [first?.proposedDigest ?? null],
      ),
    });
  }
  return items;
}

function groupOf(data: DeliveryData, envs: readonly string[]): string {
  const groups = data.now.environments
    .filter((e) => envs.includes(e.env))
    .map((e) => groupLabel(e.group));
  return Array.from(new Set(groups)).join(", ");
}

const RENOVATE_LABEL = {
  waiting: m.renovateWaiting,
  running: m.renovateRunning,
  passed: m.renovatePassed,
  failed: m.renovateFailed,
} as const;

function nextLine(
  intl: IntlShape,
  data: DeliveryData,
  e: DeliveryNextEvent,
  nowMs: number,
): Rich {
  let text: string;
  switch (e.kind) {
    case "expecting-bundle":
      // FLT-DLV-23.04: changes riding one expected bundle share the line.
      text = intl.formatMessage(m.nextExpecting, {
        changes: intl.formatList(
          e.changes.map((c) =>
            c.pr !== null ? `#${String(c.pr)}` : c.shortSha,
          ),
        ),
      });
      break;
    case "renovate": {
      // FLT-DLV-23.05: a published bundle waiting on Renovate.
      const state = intl.formatMessage(RENOVATE_LABEL[e.state ?? "waiting"], {
        number: e.pr ?? 0,
      });
      text = intl.formatMessage(m.nextRenovate, {
        bundle: alias(e.digest, e.version),
        state,
      });
      return e.at !== null
        ? rich(
            intl.formatMessage(m.nextLockedAbout, {
              text,
              time: formatClock(intl, e.at, nowMs),
            }),
            [e.at],
            [e.digest],
          )
        : rich(text, [], [e.digest]);
    }
    case "restart":
      text = intl.formatMessage(m.nextRestart, {
        group: groupOf(data, e.envs),
        envs: intl.formatList(e.envs),
      });
      break;
    case "promote":
      text = intl.formatMessage(m.nextPromote, {
        group: groupOf(data, e.envs),
        after: intl.formatList(e.after),
      });
      break;
  }
  return e.at !== null
    ? rich(
        intl.formatMessage(m.nextAbout, {
          text,
          time: formatClock(intl, e.at, nowMs),
        }),
        [e.at],
      )
    : rich(text);
}

function trainWhere(intl: IntlShape, b: DeliveryTrainBundle): string {
  if (b.restarts.length > 0) {
    return intl.formatMessage(m.trainRestarts, {
      envs: intl.formatList(b.restarts),
    });
  }
  if (b.active.length > 0) {
    const active = intl.formatMessage(m.trainActive, {
      envs: intl.formatList(b.active),
    });
    return b.proposed.length > 0
      ? `${active}; ${intl.formatMessage(m.trainProposed, {
          envs: intl.formatList(b.proposed),
        })}`
      : active;
  }
  if (b.replaced.length > 0) {
    return intl.formatMessage(m.trainReplaced, {
      envs: intl.formatList(b.replaced),
    });
  }
  return intl.formatMessage(m.trainNowhere);
}

function trainState(
  data: DeliveryData,
  b: DeliveryTrainBundle,
): DeliveryStageState {
  if (b.inferred || (b.active.length === 0 && b.replaced.length === 0)) {
    return "waiting";
  }
  if (b.active.length === 0) {
    return "not-needed";
  }
  const running = data.now.environments.some(
    (e) => b.active.includes(e.env) && e.state === "running",
  );
  return running ? "running" : "passed";
}

export function PipelineNow({
  data,
  nowMs,
}: {
  readonly data: DeliveryData;
  readonly nowMs: number;
}) {
  const intl = useIntl();
  const now = data.now;
  const unknown = nowUnknown(data);
  const loading = loadingSources(data);
  const items = inFlight(intl, data, nowMs);
  const app = now.appHead;
  const gitops = now.gitopsHead;
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Title headingLevel="h2" size="lg">
            {intl.formatMessage(m.pipelineNowTitle)}
          </Title>
        </CardTitle>
      </CardHeader>
      <CardBody>
        <Stack hasGutter>
          {loading.length > 0 ? (
            <StackItem>
              <Flex
                alignItems={{ default: "alignItemsCenter" }}
                spaceItems={{ default: "spaceItemsSm" }}
                flexWrap={{ default: "nowrap" }}
              >
                <Spinner size="md" aria-hidden="true" />
                <span>
                  {intl.formatMessage(m.nowLoading, {
                    sources: intl.formatList(
                      loading.map((k) => intl.formatMessage(SOURCE_LABEL[k])),
                    ),
                    count: loading.length,
                  })}
                </span>
              </Flex>
            </StackItem>
          ) : null}
          {unknown !== null ? (
            <StackItem>
              <StateText
                state="unknown"
                text={rich(
                  intl.formatMessage(m.nowUnknown, {
                    answers: intl.formatList(
                      unknown.answers.map((a) =>
                        intl.formatMessage(
                          a === "heads"
                            ? m.nowAnswerHeads
                            : a === "in-flight"
                              ? m.nowAnswerInFlight
                              : m.nowAnswerNext,
                        ),
                      ),
                    ),
                    sources: intl.formatList(
                      unknown.sources.map((k) =>
                        intl.formatMessage(SOURCE_LABEL[k]),
                      ),
                    ),
                  }),
                )}
              />
            </StackItem>
          ) : null}
          <StackItem>
            <Grid hasGutter md={4}>
              <GridItem>
                <Title headingLevel="h3" size="md">
                  {intl.formatMessage(m.nowHeads)}
                </Title>
                <DescriptionList isCompact>
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      {intl.formatMessage(m.nowAppMain)}
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {app !== null ? (
                        <RichText
                          nowMs={nowMs}
                          text={intl.formatMessage(m.nowAppHead, {
                            change: change(app.shortSha, app.pr),
                            title: app.title,
                            time: formatClock(intl, app.mergedAt, nowMs),
                          })}
                          times={[app.mergedAt]}
                        />
                      ) : (
                        intl.formatMessage(STATE_LABEL.unknown)
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      {intl.formatMessage(m.nowGitopsMain)}
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {gitops !== null ? (
                        <RichText
                          bundles={[gitops.digest]}
                          nowMs={nowMs}
                          text={intl.formatMessage(
                            gitops.lockedAt !== null
                              ? m.nowGitopsLocked
                              : m.nowGitopsHead,
                            {
                              bundle: alias(gitops.digest, gitops.version),
                              published: formatClock(
                                intl,
                                gitops.publishedAt,
                                nowMs,
                              ),
                              locked: formatClock(intl, gitops.lockedAt, nowMs),
                            },
                          )}
                          times={[gitops.publishedAt, gitops.lockedAt]}
                        />
                      ) : (
                        intl.formatMessage(STATE_LABEL.unknown)
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </GridItem>
              <GridItem>
                <Title headingLevel="h3" size="md">
                  {intl.formatMessage(m.nowInFlight)}
                </Title>
                {items.length === 0 ? (
                  <Content component="p">
                    {intl.formatMessage(m.nowNothingInFlight)}
                  </Content>
                ) : (
                  <DescriptionList isCompact>
                    {items.map((item) => (
                      <DescriptionListGroup key={item.key}>
                        <DescriptionListTerm>{item.term}</DescriptionListTerm>
                        <DescriptionListDescription>
                          <StateText state={item.state} text={item} />
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    ))}
                  </DescriptionList>
                )}
              </GridItem>
              <GridItem>
                <Title headingLevel="h3" size="md">
                  {intl.formatMessage(m.nowNext)}
                </Title>
                {now.next.length === 0 ? (
                  <Content component="p">
                    {intl.formatMessage(m.nowNothingExpected)}
                  </Content>
                ) : (
                  <Content component="ol">
                    {now.next.map((e, i) => {
                      const line = nextLine(intl, data, e, nowMs);
                      return (
                        <li key={`${e.kind}:${String(i)}`}>
                          <RichText
                            bundles={line.bundles}
                            nowMs={nowMs}
                            text={line.text}
                            times={line.times}
                          />
                        </li>
                      );
                    })}
                  </Content>
                )}
              </GridItem>
            </Grid>
          </StackItem>
          {data.train.length > 0 ? (
            <StackItem>
              {/* FLT-DLV-22: one bundle per finished build. */}
              <Table
                aria-label={intl.formatMessage(m.trainTitle)}
                variant="compact"
              >
                <Thead>
                  <Tr>
                    <Th>{intl.formatMessage(m.trainTitle)}</Th>
                    <Th>{intl.formatMessage(m.trainPublished)}</Th>
                    <Th>{intl.formatMessage(m.trainChanged)}</Th>
                    <Th>{intl.formatMessage(m.trainWhere)}</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {data.train.map((b) => {
                    const name = bundleName(intl, b);
                    return (
                      <Tr key={b.digest ?? name}>
                        <Td dataLabel={intl.formatMessage(m.trainTitle)}>
                          <StageStateIcon state={trainState(data, b)} />
                          <span className={styles.iconGap}>
                            {b.inferred || !b.digest ? (
                              name
                            ) : (
                              <BundleName digest={b.digest} />
                            )}
                          </span>
                        </Td>
                        <Td dataLabel={intl.formatMessage(m.trainPublished)}>
                          {b.inferred ? (
                            intl.formatMessage(m.trainReleasing)
                          ) : (
                            <RichText
                              nowMs={nowMs}
                              text={formatClock(intl, b.publishedAt, nowMs)}
                              times={[b.publishedAt]}
                            />
                          )}
                        </Td>
                        <Td dataLabel={intl.formatMessage(m.trainChanged)}>
                          {b.same
                            ? intl.formatMessage(m.trainSame)
                            : b.changed.map((c) => c.component).join(", ")}
                        </Td>
                        <Td dataLabel={intl.formatMessage(m.trainWhere)}>
                          {trainWhere(intl, b)}
                        </Td>
                      </Tr>
                    );
                  })}
                </Tbody>
              </Table>
            </StackItem>
          ) : null}
          <StackItem>
            <Table
              aria-label={intl.formatMessage(m.environmentsNow)}
              variant="compact"
            >
              <Thead>
                <Tr>
                  <Th>{intl.formatMessage(m.envColumn)}</Th>
                  <Th>{intl.formatMessage(m.groupColumn)}</Th>
                  <Th>{intl.formatMessage(m.runsColumn)}</Th>
                  <Th>{intl.formatMessage(m.stateColumn)}</Th>
                </Tr>
              </Thead>
              <Tbody>
                {now.environments.map((env) => (
                  <Tr key={env.env}>
                    <Td dataLabel={intl.formatMessage(m.envColumn)}>
                      {env.env}
                    </Td>
                    <Td dataLabel={intl.formatMessage(m.groupColumn)}>
                      {groupLabel(env.group)}
                    </Td>
                    <Td dataLabel={intl.formatMessage(m.runsColumn)}>
                      <BundleName digest={env.digest} fallback={env.version} />
                    </Td>
                    <Td dataLabel={intl.formatMessage(m.stateColumn)}>
                      <StateText
                        state={env.state}
                        text={envText(intl, env, nowMs)}
                      />
                    </Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          </StackItem>
        </Stack>
      </CardBody>
    </Card>
  );
}
