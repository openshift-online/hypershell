import type { ReactNode } from "react";

export function DashboardPurpose({
  heading,
  children,
}: Readonly<{
  heading: ReactNode;
  children: ReactNode;
}>) {
  return (
    <div className="hypershell-dashboard-purpose">
      <strong>{heading}</strong>
      <div>{children}</div>
    </div>
  );
}
