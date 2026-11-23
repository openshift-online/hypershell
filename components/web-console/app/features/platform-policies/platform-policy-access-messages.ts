import { defineMessages } from "react-intl";

export const platformPolicyAccessMessages = defineMessages({
  checkingAccess: {
    id: "app.platformPolicies.access.checking",
    defaultMessage: "Checking access",
    description:
      "Accessible label while the console checks platform policy admin access.",
  },
  notAuthorizedTitle: {
    id: "app.platformPolicies.access.notAuthorizedTitle",
    defaultMessage: "You do not have access to platform policies",
    description:
      "Title of the not-authorized state on a platform policy admin route.",
  },
  notAuthorizedBody: {
    id: "app.platformPolicies.access.notAuthorizedBody",
    defaultMessage:
      "Platform policies are managed by platform policy administrators. Ask a platform administrator for the platform-policy:admin role.",
    description:
      "Body of the not-authorized state on a platform policy admin route.",
  },
  accessErrorTitle: {
    id: "app.platformPolicies.access.errorTitle",
    defaultMessage: "Could not check your access",
    description: "Title when the access check failed for another reason.",
  },
  retry: {
    id: "app.platformPolicies.access.retry",
    defaultMessage: "Try again",
    description: "Button that retries the access check.",
  },
});
