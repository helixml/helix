import { useEffect, useState } from "react";

/**
 * Observe an element's rendered width. Returns a callback ref to attach to the
 * element and its last measured width in px (0 before the first measurement).
 *
 * Uses a state-mirrored callback ref rather than a plain useRef so the observer
 * effect re-runs once the node actually mounts.
 */
export function useElementWidth<T extends HTMLElement>(): [
  (node: T | null) => void,
  number,
] {
  const [node, setNode] = useState<T | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    if (!node) return;

    const measure = (next: number) => {
      setWidth((prev) => (Math.abs(prev - next) < 1 ? prev : next));
    };

    measure(node.getBoundingClientRect().width);

    if (typeof ResizeObserver === "undefined") return;

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;
      measure(
        entry.borderBoxSize?.[0]?.inlineSize ?? entry.contentRect.width,
      );
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [node]);

  return [setNode, width];
}

export default useElementWidth;
