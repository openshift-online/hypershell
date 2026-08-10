import { useEffect, useState } from "react";

export function useDebouncedValue<Value>(
  value: Value,
  delayMilliseconds: number,
): Value {
  const [debouncedValue, setDebouncedValue] = useState(value);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setDebouncedValue(value);
    }, delayMilliseconds);

    return () => {
      window.clearTimeout(timeout);
    };
  }, [delayMilliseconds, value]);

  return debouncedValue;
}
