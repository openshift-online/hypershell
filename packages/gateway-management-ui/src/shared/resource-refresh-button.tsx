import { Button } from "@patternfly/react-core";
import { SyncAltIcon } from "@patternfly/react-icons";

interface ResourceRefreshButtonProps {
  ariaLabel: string;
  isRefreshing?: boolean;
  onRefresh: () => unknown;
}

export function ResourceRefreshButton({
  ariaLabel,
  isRefreshing = false,
  onRefresh,
}: ResourceRefreshButtonProps) {
  return (
    <Button
      aria-label={ariaLabel}
      icon={<SyncAltIcon />}
      isDisabled={isRefreshing}
      onClick={() => {
        onRefresh();
      }}
      spinnerAriaValueText={ariaLabel}
      title={ariaLabel}
      variant="plain"
      {...(isRefreshing ? { isLoading: true } : {})}
    />
  );
}
