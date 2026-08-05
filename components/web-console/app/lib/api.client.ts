import { SDKClient } from "@openshift-online/hypershell-sdk";

export const apiClient = new SDKClient({
  baseUrl: "",
  credentials: "same-origin",
});
