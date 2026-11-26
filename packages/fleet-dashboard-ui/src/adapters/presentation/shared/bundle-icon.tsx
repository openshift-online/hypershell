import { initialsDataUri } from "./initials";
import styles from "./bundle-icon.module.css";

// A bundle identicon in HTML, next to the bundle's alias (FLT-DLV-24).
// Decorative: the alias carries the meaning.
export function BundleIcon({
  seed,
  size = 20,
}: {
  readonly seed: string;
  readonly size?: number;
}): React.ReactElement {
  return (
    <img
      alt=""
      aria-hidden="true"
      className={styles.icon}
      data-identicon-seed={seed}
      height={size}
      src={initialsDataUri(seed)}
      width={size}
    />
  );
}
