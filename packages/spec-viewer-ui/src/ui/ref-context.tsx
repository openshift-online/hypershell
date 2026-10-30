import { createContext, useContext } from "react";

import type { RefName } from "../application/ports";

const RefContext = createContext<RefName>("");

/** Provides the branch every query of the viewer reads. */
export const RefProvider = RefContext.Provider;

/** The branch the viewer shows; "" is the server's default. */
export function useSpecRef(): RefName {
  return useContext(RefContext);
}
