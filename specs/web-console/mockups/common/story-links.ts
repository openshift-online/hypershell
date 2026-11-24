/*
 * Mockup-only navigation. Production routes do not exist inside Storybook,
 * so links between mockup pages open the matching story in the top window.
 */
export function storyHref(storyId: string): string {
  return `/?path=/story/${storyId}`;
}

export const storyLinkProps = (storyId: string) => ({
  href: storyHref(storyId),
  target: "_top",
});

export const stories = {
  platformPolicies: "mockups-administration-platform-policies--audit-only",
  platformPolicyDetail:
    "mockups-administration-platform-policy-detail--default",
  platformPolicyGateways:
    "mockups-administration-platform-policy-detail--gateways",
  createPlatformPolicy:
    "mockups-administration-create-platform-policy--default",
  gatewayPolicies: "mockups-gateways-gateway-detail--policies-audit-only",
};
