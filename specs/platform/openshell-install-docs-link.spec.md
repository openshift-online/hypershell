# OpenShell Installation Documentation Link Specification

## Purpose

The gateway connection tab provides copyable CLI commands but does not link to
the upstream installation documentation. Operators who have not yet installed
the OpenShell CLI need a clear path to the installation guide before following
the connection steps.

**Upstream:** [NVIDIA OpenShell Installation](https://docs.nvidia.com/openshell/about/installation)

## Requirements

### Requirement: WEB-DOCS-01 -- Installation Link Visibility

The connection tab of the gateway details page SHALL display a link to the
NVIDIA OpenShell installation documentation
(`https://docs.nvidia.com/openshell/about/installation`) above the connection
steps so that operators can install the CLI before following the setup commands.

#### Scenario: WEB-DOCS-01.01 -- Link Present on Connection Tab

- GIVEN a gateway in any phase (Running, Provisioning, etc.)
- WHEN the operator views the connection tab
- THEN a link labeled "Install the OpenShell CLI" is visible
- AND the link points to `https://docs.nvidia.com/openshell/about/installation`
- AND the link opens in a new browser tab (`target="_blank"`)

### Requirement: WEB-DOCS-02 -- Accessibility

The installation link SHALL include an accessible name and an external-link
indicator so screen readers announce both the destination and the fact that it
opens in a new tab.

#### Scenario: WEB-DOCS-02.01 -- Screen Reader Announces External Link

- GIVEN the installation link is rendered
- WHEN a screen reader focuses the link
- THEN the announced name includes "Install the OpenShell CLI"
- AND the link includes `rel="noopener noreferrer"` for security
