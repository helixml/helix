import { useCallback, useState } from "react";

const PREFERENCE_PREFIX = "helix.workspace-inspector.";

function readPreference(key: string): string | null {
  try {
    return window.localStorage.getItem(PREFERENCE_PREFIX + key);
  } catch {
    // Storage is unavailable in private windows and some embedded contexts.
    // A missing preference is not an error — fall back to the default.
    return null;
  }
}

function writePreference(key: string, value: string) {
  try {
    window.localStorage.setItem(PREFERENCE_PREFIX + key, value);
  } catch {
    // Ignored for the same reason as above.
  }
}

/**
 * Presentation state that should survive remounting the inspector and
 * reloading the task. The inspector unmounts whenever the task switches away
 * from Changes/Files, so plain component state loses the reviewer's layout
 * choice on every tab switch.
 */
export function usePersistedChoice<T extends string>(
  key: string,
  options: readonly T[],
  fallback: T,
): [T, (value: T) => void] {
  const [value, setValue] = useState<T>(() => {
    const stored = readPreference(key);
    return options.includes(stored as T) ? (stored as T) : fallback;
  });
  const update = useCallback(
    (next: T) => {
      setValue(next);
      writePreference(key, next);
    },
    [key],
  );
  return [value, update];
}

const FLAG_OPTIONS = ["on", "off"] as const;

export function usePersistedFlag(
  key: string,
  fallback = false,
): [boolean, (value: boolean) => void] {
  const [stored, setStored] = usePersistedChoice(
    key,
    FLAG_OPTIONS,
    fallback ? "on" : "off",
  );
  const update = useCallback(
    (next: boolean) => setStored(next ? "on" : "off"),
    [setStored],
  );
  return [stored === "on", update];
}
