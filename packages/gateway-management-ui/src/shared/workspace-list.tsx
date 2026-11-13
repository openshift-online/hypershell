import styles from "./workspace-list.module.css";

interface WorkspaceListProps {
  emptyLabel: string;
  workspaces: readonly string[];
}

export function WorkspaceList({ emptyLabel, workspaces }: WorkspaceListProps) {
  if (workspaces.length === 0) {
    return <>{emptyLabel}</>;
  }

  return (
    <ul className={styles.workspaceList}>
      {[...workspaces]
        .sort((workspace, nextWorkspace) =>
          workspace.localeCompare(nextWorkspace),
        )
        .map((workspace) => (
          <li key={workspace}>{workspace}</li>
        ))}
    </ul>
  );
}
