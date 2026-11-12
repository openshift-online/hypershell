/** The branch most comparisons start from; it is listed right after the default. */
export const MAIN_BRANCH = "main";

/**
 * The order of the branch picker: the default first, then `main`, then the
 * others in the server's order.
 */
export function orderRefs(defaultRef: string, refs: string[]): string[] {
  const rest = refs.filter((ref) => ref !== defaultRef && ref !== MAIN_BRANCH);
  return [
    defaultRef,
    ...(refs.includes(MAIN_BRANCH) && defaultRef !== MAIN_BRANCH
      ? [MAIN_BRANCH]
      : []),
    ...rest,
  ];
}
