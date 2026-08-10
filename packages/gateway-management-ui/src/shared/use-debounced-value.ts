import { useEffect, useState } from "react";

export function useDebouncedValue<Value>(
  value: Value,
  delayMilliseconds: number,
): Value {
  const [debouncedValue, setDebouncedValue] = useState(value);

  useEffect(() => {
    const timeout = setTimeout(() => {
      setDebouncedValue(value);
    }, delayMilliseconds);

    return () => {
      clearTimeout(timeout);
    };
  }, [delayMilliseconds, value]);

  return debouncedValue;
}
