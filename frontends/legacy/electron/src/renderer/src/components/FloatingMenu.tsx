import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { CSSProperties, ReactNode, RefObject } from "react";
import { createPortal } from "react-dom";
import { cx } from "./primitives";

interface FloatingMenuPosition {
  left: number;
  maxHeight: number;
  top: number;
}

export function FloatingMenu({
  anchorPoint,
  anchorRef,
  children,
  className,
  onClose,
  width = 256
}: {
  anchorPoint?: { x: number; y: number };
  anchorRef?: RefObject<HTMLElement>;
  children: ReactNode;
  className?: string;
  onClose?: () => void;
  width?: number;
}): JSX.Element | null {
  const [position, setPosition] = useState<FloatingMenuPosition | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!onClose) {
      return undefined;
    }

    const close = onClose;

    function closeFromOutside(event: MouseEvent | PointerEvent): void {
      const target = event.target;

      if (target instanceof Node && menuRef.current?.contains(target)) {
        return;
      }

      close();
    }

    function closeOnEscape(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        close();
      }
    }

    document.addEventListener("pointerdown", closeFromOutside, true);
    document.addEventListener("contextmenu", closeFromOutside, true);
    document.addEventListener("keydown", closeOnEscape, true);
    return () => {
      document.removeEventListener("pointerdown", closeFromOutside, true);
      document.removeEventListener("contextmenu", closeFromOutside, true);
      document.removeEventListener("keydown", closeOnEscape, true);
    };
  }, [onClose]);

  useLayoutEffect(() => {
    function updatePosition(): void {
      const anchor = anchorRef?.current;

      if (!anchor && !anchorPoint) {
        return;
      }

      const rect = anchor?.getBoundingClientRect() ?? {
        bottom: anchorPoint?.y ?? 0,
        left: anchorPoint?.x ?? 0,
        right: anchorPoint?.x ?? 0,
        top: anchorPoint?.y ?? 0
      };
      const offset = 6;
      const margin = 8;
      const viewportWidth = window.innerWidth;
      const viewportHeight = window.innerHeight;
      const availableBelow = viewportHeight - rect.bottom - offset - margin;
      const availableAbove = rect.top - offset - margin;
      const below = availableBelow >= 220 || availableBelow >= availableAbove;
      const maxHeight = Math.max(160, below ? availableBelow : availableAbove);
      const top = below ? rect.bottom + offset : Math.max(margin, rect.top - offset - maxHeight);
      const left = Math.min(
        Math.max(margin, rect.right - width),
        Math.max(margin, viewportWidth - width - margin)
      );

      setPosition({ left, maxHeight, top });
    }

    updatePosition();
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    return () => {
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
    };
  }, [anchorPoint, anchorRef, width]);

  if (!position) {
    return null;
  }

  const style: CSSProperties = {
    left: position.left,
    maxHeight: position.maxHeight,
    top: position.top,
    width
  };

  return createPortal(
    <div
      className={cx(
        "fixed z-[1000] overflow-y-auto rounded-hcbLg border border-border bg-bg-primary py-2 shadow-xl",
        className
      )}
      ref={menuRef}
      style={style}
    >
      {children}
    </div>,
    document.body
  );
}
