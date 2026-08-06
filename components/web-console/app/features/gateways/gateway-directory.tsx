import {
  Button,
  Card,
  CardBody,
  CardFooter,
  CardTitle,
  ClipboardCopy,
  Content,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  Gallery,
  Label,
  PageSection,
  Title,
} from "@patternfly/react-core";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, useParams } from "react-router";

import { messages } from "../../i18n/messages";
import {
  buildGatewayAddCommand,
  getPreviewGateway,
  previewGateways,
  type GatewayConnection,
} from "./gateway-connections";
import styles from "./gateway-directory.module.css";

function ConnectionCommand({ gateway }: { gateway: GatewayConnection }) {
  const intl = useIntl();

  return (
    <div className={styles.connection}>
      <Title className={styles.connectionTitle} headingLevel="h3" size="md">
        <FormattedMessage {...messages.connectWithCli} />
      </Title>
      <ClipboardCopy
        clickTip={intl.formatMessage(messages.copied)}
        copyAriaLabel={intl.formatMessage(messages.copyConnectionCommand, {
          gatewayName: gateway.name,
        })}
        hoverTip={intl.formatMessage(messages.copy)}
        isCode
        isReadOnly
      >
        {buildGatewayAddCommand(gateway)}
      </ClipboardCopy>
    </div>
  );
}

function GatewayCard({ gateway }: { gateway: GatewayConnection }) {
  const intl = useIntl();

  return (
    <Card isFullHeight>
      <CardTitle>
        <div className={styles.heading}>
          <Title headingLevel="h2" size="lg">
            <Link to={`/gateways/${encodeURIComponent(gateway.id)}`}>
              {gateway.name}
            </Link>
          </Title>
          <Label color="green" isCompact>
            {gateway.status}
          </Label>
        </div>
      </CardTitle>
      <CardBody>
        <Content>
          <h3>
            <FormattedMessage {...messages.gatewayEndpoint} />
          </h3>
          <p className={styles.endpoint}>{gateway.endpoint}</p>
        </Content>
        <ConnectionCommand gateway={gateway} />
      </CardBody>
      <CardFooter>
        <div className={styles.actions}>
          <Button
            aria-label={intl.formatMessage(messages.openGatewayConsoleFor, {
              gatewayName: gateway.name,
            })}
            component="a"
            href={gateway.consoleUrl}
            rel="noreferrer"
            target="_blank"
            variant="primary"
          >
            <FormattedMessage {...messages.openGatewayConsole} />
          </Button>
          <Button
            component={Link}
            isInline
            variant="link"
            {...{ to: `/gateways/${encodeURIComponent(gateway.id)}` }}
          >
            <FormattedMessage {...messages.viewGatewayDetails} />
          </Button>
        </div>
      </CardFooter>
    </Card>
  );
}

export function GatewayDirectory({
  gateways,
}: {
  gateways: readonly GatewayConnection[];
}) {
  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <div className={styles.content}>
          <Content>
            <Title headingLevel="h1" size="2xl">
              <FormattedMessage {...messages.openShellGateways} />
            </Title>
            <p>
              <FormattedMessage {...messages.gatewayDirectoryDescription} />
            </p>
          </Content>
        </div>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <div className={styles.content}>
          {gateways.length === 0 ? (
            <EmptyState
              headingLevel="h2"
              titleText={<FormattedMessage {...messages.noGatewaysAvailable} />}
              variant={EmptyStateVariant.lg}
            >
              <EmptyStateBody>
                <FormattedMessage {...messages.noGatewaysAvailableBody} />
              </EmptyStateBody>
            </EmptyState>
          ) : (
            <Gallery hasGutter minWidths={{ default: "22rem" }}>
              {gateways.map((gateway) => (
                <GatewayCard gateway={gateway} key={gateway.id} />
              ))}
            </Gallery>
          )}
        </div>
      </PageSection>
    </>
  );
}

export function GatewayDirectoryPage() {
  return <GatewayDirectory gateways={previewGateways} />;
}

export function GatewayDetails({ gateway }: { gateway: GatewayConnection }) {
  const intl = useIntl();

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <div className={styles.content}>
          <div className={styles.heading}>
            <Content>
              <Title headingLevel="h1" size="2xl">
                {gateway.name}
              </Title>
              <p>
                <FormattedMessage {...messages.gatewayDetailsDescription} />
              </p>
            </Content>
            <Label color="green">{gateway.status}</Label>
          </div>
        </div>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <div className={styles.content}>
          <Card className={styles.detailCard}>
            <CardTitle>
              <Title headingLevel="h2" size="lg">
                <FormattedMessage {...messages.getStarted} />
              </Title>
            </CardTitle>
            <CardBody>
              <Content>
                <h3>
                  <FormattedMessage {...messages.gatewayEndpoint} />
                </h3>
                <p className={styles.endpoint}>{gateway.endpoint}</p>
              </Content>
              <ConnectionCommand gateway={gateway} />
            </CardBody>
            <CardFooter>
              <Button
                aria-label={intl.formatMessage(messages.openGatewayConsoleFor, {
                  gatewayName: gateway.name,
                })}
                component="a"
                href={gateway.consoleUrl}
                rel="noreferrer"
                target="_blank"
                variant="primary"
              >
                <FormattedMessage {...messages.openGatewayConsole} />
              </Button>
            </CardFooter>
          </Card>
        </div>
      </PageSection>
    </>
  );
}

export function GatewayDetailsPage() {
  const { gatewayId = "gateway" } = useParams();

  return <GatewayDetails gateway={getPreviewGateway(gatewayId)} />;
}
