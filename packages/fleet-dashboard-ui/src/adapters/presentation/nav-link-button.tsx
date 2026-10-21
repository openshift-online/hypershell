// A PatternFly link button backed by a real anchor and a typed navigation action,
// so copy-link, open-in-new-tab, and Ctrl/Cmd-click work while a plain click
// navigates in place (FLT-NAV-08). One hook call per instance keeps it usable
// inside table rows and lists (render it as a component, never call useNavLink in a
// map callback).

import { Button } from "@patternfly/react-core";

import type { NavAction } from "../../domain/navigation/actions";
import { useNavLink } from "../routing/router";

export function NavLinkButton({
  action,
  children,
  icon,
  iconPosition,
  onActivate,
  "aria-label": ariaLabel,
}: {
  readonly action: NavAction;
  readonly children: React.ReactNode;
  readonly icon?: React.ReactNode;
  readonly iconPosition?: "start" | "end";
  /** Runs on every click (before any in-place navigation), e.g. to record the
   *  trigger element for focus restoration. */
  readonly onActivate?: (event: React.MouseEvent) => void;
  readonly "aria-label"?: string;
}): React.ReactElement {
  const link = useNavLink(action);
  return (
    <Button
      component="a"
      variant="link"
      isInline
      icon={icon}
      iconPosition={iconPosition}
      aria-label={ariaLabel}
      href={link.href}
      onClick={(event) => {
        onActivate?.(event);
        link.onClick(event);
      }}
    >
      {children}
    </Button>
  );
}
