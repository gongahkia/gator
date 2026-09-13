import { useEffect, useRef, useState } from "react";
import type { RefObject } from "react";

export function useAutoCollapsedSidebar(threshold = 760): {
  autoCollapsed: boolean;
  containerRef: RefObject<HTMLDivElement>;
} {
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoCollapsed, setAutoCollapsed] = useState(false);

  useEffect(() => {
    const element = containerRef.current;

    if (!element || typeof ResizeObserver === "undefined") {
      return;
    }

    const update = (): void => {
      const width = element.getBoundingClientRect().width;
      setAutoCollapsed(width > 0 && width < threshold);
    };
    const observer = new ResizeObserver(update);

    update();
    observer.observe(element);

    return () => observer.disconnect();
  }, [threshold]);

  return { autoCollapsed, containerRef };
}
