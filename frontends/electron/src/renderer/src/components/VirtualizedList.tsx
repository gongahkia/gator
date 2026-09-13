import { useMemo, useState } from "react";
import { useRef } from "react";
import type { Key, ReactNode, UIEvent } from "react";
import {
  rendererNow,
  reportRendererTimingSince
} from "../hooks/useRenderTiming";

interface VirtualizedListProps<T> {
  ariaLabel: string;
  emptyState?: ReactNode;
  estimateRowHeight?: number;
  getEstimatedRowHeight?: (item: T, index: number) => number;
  getKey: (item: T, index: number) => Key;
  items: T[];
  overscan?: number;
  performanceLabel?: string;
  renderRow: (item: T, index: number) => ReactNode;
  viewportHeight?: number;
}

export function VirtualizedList<T>({
  ariaLabel,
  emptyState,
  estimateRowHeight = 48,
  getEstimatedRowHeight,
  getKey,
  items,
  overscan = 4,
  performanceLabel,
  renderRow,
  viewportHeight = 320
}: VirtualizedListProps<T>): JSX.Element {
  const [scrollTop, setScrollTop] = useState(0);
  const lastScrollReportAt = useRef(0);
  const rowHeights = useMemo(
    () => items.map((item, index) => Math.max(1, getEstimatedRowHeight?.(item, index) ?? estimateRowHeight)),
    [estimateRowHeight, getEstimatedRowHeight, items]
  );
  const rowOffsets = useMemo(() => {
    const offsets = [0];

    for (const height of rowHeights) {
      offsets.push(offsets[offsets.length - 1] + height);
    }

    return offsets;
  }, [rowHeights]);

  const windowState = useMemo(() => {
    const findStartIndex = (top: number): number => {
      let low = 0;
      let high = items.length;

      while (low < high) {
        const mid = Math.floor((low + high) / 2);

        if (rowOffsets[mid + 1] <= top) {
          low = mid + 1;
        } else {
          high = mid;
        }
      }

      return low;
    };
    const visibleStartIndex = findStartIndex(scrollTop);
    const visibleEndIndex = findStartIndex(scrollTop + viewportHeight);
    const startIndex = Math.max(0, visibleStartIndex - overscan);
    const endIndex = Math.min(items.length, visibleEndIndex + overscan + 1);

    return {
      endIndex,
      offsetY: rowOffsets[startIndex],
      startIndex,
      totalHeight: rowOffsets[rowOffsets.length - 1] ?? 0
    };
  }, [items.length, overscan, rowOffsets, scrollTop, viewportHeight]);

  function handleScroll(event: UIEvent<HTMLDivElement>): void {
    const startedAt = rendererNow();

    setScrollTop(event.currentTarget.scrollTop);

    if (performanceLabel && startedAt !== null && startedAt - lastScrollReportAt.current > 250) {
      lastScrollReportAt.current = startedAt;
      window.requestAnimationFrame(() => {
        reportRendererTimingSince(`${performanceLabel}.scroll`, startedAt, {
          itemCount: items.length,
          renderedCount: Math.max(0, windowState.endIndex - windowState.startIndex)
        });
      });
    }
  }

  if (items.length === 0) {
    return (
      <div className="grid place-items-center overflow-hidden" style={{ height: viewportHeight }}>
        {emptyState}
      </div>
    );
  }

  return (
    <div
      aria-label={ariaLabel}
      className="overflow-auto"
      onScroll={handleScroll}
      role="list"
      style={{ height: viewportHeight }}
    >
      <div className="relative" style={{ height: windowState.totalHeight }}>
        <div
          className="absolute inset-x-0 top-0"
          style={{ transform: `translateY(${windowState.offsetY}px)` }}
        >
          {items.slice(windowState.startIndex, windowState.endIndex).map((item, visibleIndex) => {
            const index = windowState.startIndex + visibleIndex;

            return (
              <div key={getKey(item, index)} style={{ minHeight: rowHeights[index] ?? estimateRowHeight }}>
                {renderRow(item, index)}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
