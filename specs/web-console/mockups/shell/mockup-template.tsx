import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Content,
  Dropdown,
  DropdownItem,
  DropdownList,
  Flex,
  FlexItem,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadLogo,
  MastheadMain,
  MenuToggle,
  Page,
  PageSection,
  SkipToContent,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Title,
} from "@patternfly/react-core";
import { MoonIcon, SyncAltIcon, UserIcon } from "@patternfly/react-icons";
import { useState } from "react";

import logo from "../../../../images/brand/logo.png";
import styles from "./mockup-template.module.css";

export type MockupTemplateProps = {
  breadcrumbs?: string[];
  children: React.ReactNode;
  contentVariant?: "default" | "secondary";
  description?: React.ReactNode;
  onRefresh?: () => void;
  showBreadcrumbs?: boolean;
  showRefresh?: boolean;
  title: string;
};

function UserDropdown() {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <Dropdown
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onSelect={() => setIsOpen(false)}
      popperProps={{ position: "right" }}
      toggle={(toggleRef) => (
        <MenuToggle
          icon={<UserIcon />}
          isExpanded={isOpen}
          onClick={() => setIsOpen((open) => !open)}
          ref={toggleRef}
        >
          Ada Lovelace
        </MenuToggle>
      )}
    >
      <DropdownList>
        <DropdownItem>Account settings</DropdownItem>
        <DropdownItem>Log out</DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}

function MockupMasthead() {
  return (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          <MastheadLogo className={styles.brand} component="a" href="/">
            <img alt="" className={styles.brandLogo} src={logo} />
            HyperShell
          </MastheadLogo>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar isStatic>
          <ToolbarContent>
            <ToolbarItem align={{ default: "alignEnd" }}>
              <Button aria-label="Toggle dark mode" icon={<MoonIcon />} variant="plain" />
            </ToolbarItem>
            <ToolbarItem>
              <UserDropdown />
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );
}

export function MockupTemplate({
  breadcrumbs = [],
  children,
  contentVariant = "default",
  description,
  onRefresh,
  showBreadcrumbs = true,
  showRefresh = false,
  title,
}: MockupTemplateProps) {
  const breadcrumb = breadcrumbs.length ? (
    <Breadcrumb>
      {breadcrumbs.map((item, index) => (
        <BreadcrumbItem
          key={item}
          isActive={index === breadcrumbs.length - 1}
          to={index === 0 ? "/" : undefined}
        >
          {item}
        </BreadcrumbItem>
      ))}
    </Breadcrumb>
  ) : undefined;

  return (
    <Page
      breadcrumb={showBreadcrumbs ? breadcrumb : undefined}
      isContentFilled
      mainContainerId="main-content"
      masthead={<MockupMasthead />}
      skipToContent={
        <SkipToContent href="#main-content">Skip to content</SkipToContent>
      }
    >
      <PageSection hasBodyWrapper={false}>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Content>
              <Title headingLevel="h1" size="2xl">
                {title}
              </Title>
              {description ? <p>{description}</p> : null}
            </Content>
          </FlexItem>
          {showRefresh ? (
            <FlexItem>
              <Button
                aria-label="Refresh"
                icon={<SyncAltIcon />}
                onClick={onRefresh ?? (() => undefined)}
                variant="plain"
              />
            </FlexItem>
          ) : null}
        </Flex>
      </PageSection>
      <PageSection
        hasBodyWrapper={false}
        isFilled
        variant={contentVariant}
      >
        {children}
      </PageSection>
    </Page>
  );
}

export function TemplatePreview() {
  return (
    <MockupTemplate
      showRefresh
      title="Template file"
    >
      <Content>main content goes here</Content>
    </MockupTemplate>
  );
}
