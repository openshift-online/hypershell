import { useEffect, useRef } from "react";

function focusPageHeading(): boolean {
  const pageHeading = document.querySelector<HTMLElement>("#main-content h1");
  if (!pageHeading) {
    return false;
  }

  pageHeading.tabIndex = -1;
  pageHeading.focus();
  return true;
}

export function useRouteHeadingFocus(pathname: string) {
  const previousPath = useRef(pathname);

  useEffect(() => {
    if (previousPath.current === pathname) {
      return;
    }

    previousPath.current = pathname;
    if (focusPageHeading()) {
      return;
    }

    const mainContent = document.querySelector("#main-content");
    if (!mainContent) {
      return;
    }

    const observer = new MutationObserver(() => {
      if (focusPageHeading()) {
        observer.disconnect();
      }
    });
    observer.observe(mainContent, { childList: true, subtree: true });

    return () => {
      observer.disconnect();
    };
  }, [pathname]);
}
