import type { Ref } from "react";

/**
 * Softer resize affordance for PatternFly widgetized-dashboard tiles:
 * two short gray diagonal strokes instead of a solid high-contrast wedge.
 */
export function dashboardResizeHandle(
  resizeHandleAxis: string,
  ref: Ref<HTMLElement>,
) {
  return (
    <div
      ref={ref as Ref<HTMLDivElement>}
      className={`react-resizable-handle react-resizable-handle-${resizeHandleAxis}`}
    >
      <svg
        width="16"
        height="16"
        viewBox="0 0 16 16"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <path
          d="M16 0L0 16"
          stroke="currentColor"
          strokeWidth="1.75"
          strokeLinecap="square"
        />
        <path
          d="M16 4L4 16"
          stroke="currentColor"
          strokeWidth="1.75"
          strokeLinecap="square"
        />
      </svg>
    </div>
  );
}

export const dashboardResizeWidgetConfig = {
  handleComponent: dashboardResizeHandle,
};
