import { useCallback, useEffect, useState } from "react";

const STORAGE_KEY = "helix.autoScroll";

// How close to the bottom (in px) is "at bottom enough" to hide the
// jump-to-latest pill. Only used when auto-scroll is OFF.
export const AUTO_SCROLL_NEAR_BOTTOM_PX = 80;

const readStored = (): boolean => {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === null) return true;
    return v === "true";
  } catch {
    return true;
  }
};

const writeStored = (value: boolean) => {
  try {
    localStorage.setItem(STORAGE_KEY, String(value));
  } catch {
    // ignore
  }
};

// Single global preference, shared across all EmbeddedSessionView instances
// in the page. Each subscriber re-renders when the value changes (whether
// the change came from this hook or another instance via the storage event).
const subscribers = new Set<(value: boolean) => void>();

const broadcast = (value: boolean) => {
  for (const fn of subscribers) fn(value);
};

if (typeof window !== "undefined") {
  window.addEventListener("storage", (e) => {
    if (e.key === STORAGE_KEY) broadcast(readStored());
  });
}

export const useAutoScrollPreference = (): [boolean, (next: boolean) => void] => {
  const [value, setValue] = useState<boolean>(readStored);

  useEffect(() => {
    subscribers.add(setValue);
    return () => {
      subscribers.delete(setValue);
    };
  }, []);

  const setPreference = useCallback((next: boolean) => {
    writeStored(next);
    broadcast(next);
  }, []);

  return [value, setPreference];
};
