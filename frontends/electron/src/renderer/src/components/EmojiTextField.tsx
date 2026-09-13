import data from "@emoji-mart/data";
import {
  forwardRef,
  useMemo,
  useRef,
  useState
} from "react";
import type {
  ChangeEvent,
  InputHTMLAttributes,
  KeyboardEvent,
  MutableRefObject,
  Ref,
  TextareaHTMLAttributes
} from "react";
import { cx } from "./primitives";

type EmojiElement = HTMLInputElement | HTMLTextAreaElement;

interface EmojiEntry {
  id: string;
  name: string;
  native: string;
  terms: string[];
}

const emojiEntries = Object.entries(((data as { emojis?: Record<string, unknown> }).emojis ?? {}))
  .map(([id, value]) => {
    const emoji = value as {
      id?: string;
      keywords?: string[];
      name?: string;
      skins?: Array<{ native?: string }>;
    };
    const native = emoji.skins?.[0]?.native;

    if (!native || !emoji.name) {
      return null;
    }

    return {
      id: emoji.id ?? id,
      name: emoji.name,
      native,
      terms: [emoji.id ?? id, emoji.name, ...(emoji.keywords ?? [])].map((term) => term.toLowerCase())
    };
  })
  .filter((entry): entry is EmojiEntry => entry !== null);

function setRefs<T>(node: T, refs: Array<Ref<T> | undefined>): void {
  for (const ref of refs) {
    if (!ref) {
      continue;
    }

    if (typeof ref === "function") {
      ref(node);
    } else {
      (ref as MutableRefObject<T>).current = node;
    }
  }
}

function emojiQuery(value: string, cursor: number): { end: number; query: string; start: number } | null {
  const before = value.slice(0, cursor);
  const match = /(^|\s):([a-zA-Z0-9_+-]{1,32})$/.exec(before);

  if (!match || match.index < 0) {
    return null;
  }

  return {
    end: cursor,
    query: match[2].toLowerCase(),
    start: match.index + match[1].length
  };
}

function useEmojiField(
  value: string,
  onValueChange: (value: string) => void,
  externalKeyDown?: (event: KeyboardEvent<EmojiElement>) => void
): {
  activeIndex: number;
  handleChange: (event: ChangeEvent<EmojiElement>) => void;
  handleKeyDown: (event: KeyboardEvent<EmojiElement>) => void;
  innerRef: React.MutableRefObject<EmojiElement | null>;
  pickEmoji: (entry: EmojiEntry) => void;
  suggestions: EmojiEntry[];
} {
  const innerRef = useRef<EmojiElement | null>(null);
  const [queryRange, setQueryRange] = useState<{ end: number; query: string; start: number } | null>(null);
  const [activeIndex, setActiveIndex] = useState(0);
  const suggestions = useMemo(() => {
    if (!queryRange) {
      return [];
    }

    return emojiEntries
      .filter((entry) => entry.terms.some((term) => term.includes(queryRange.query)))
      .slice(0, 8);
  }, [queryRange]);

  function refreshQuery(nextValue: string, cursor: number): void {
    setQueryRange(emojiQuery(nextValue, cursor));
    setActiveIndex(0);
  }

  function handleChange(event: ChangeEvent<EmojiElement>): void {
    onValueChange(event.target.value);
    refreshQuery(event.target.value, event.target.selectionStart ?? event.target.value.length);
  }

  function pickEmoji(entry: EmojiEntry): void {
    const element = innerRef.current;
    const range = queryRange;

    if (!element || !range) {
      return;
    }

    const nextValue = `${value.slice(0, range.start)}${entry.native}${value.slice(range.end)}`;
    const nextCursor = range.start + entry.native.length;
    onValueChange(nextValue);
    setQueryRange(null);
    requestAnimationFrame(() => {
      element.focus();
      element.setSelectionRange(nextCursor, nextCursor);
    });
  }

  function handleKeyDown(event: KeyboardEvent<EmojiElement>): void {
    if (suggestions.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        event.stopPropagation();
        setActiveIndex((current) => (current + 1) % suggestions.length);
        return;
      }

      if (event.key === "ArrowUp") {
        event.preventDefault();
        event.stopPropagation();
        setActiveIndex((current) => (current - 1 + suggestions.length) % suggestions.length);
        return;
      }

      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault();
        event.stopPropagation();
        pickEmoji(suggestions[activeIndex]);
        return;
      }

      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        setQueryRange(null);
        return;
      }
    }

    externalKeyDown?.(event);
  }

  return { activeIndex, handleChange, handleKeyDown, innerRef, pickEmoji, suggestions };
}

function EmojiSuggestions({
  activeIndex,
  onPick,
  suggestions
}: {
  activeIndex: number;
  onPick: (entry: EmojiEntry) => void;
  suggestions: EmojiEntry[];
}): JSX.Element | null {
  if (suggestions.length === 0) {
    return null;
  }

  return (
    <div className="absolute left-0 top-[calc(100%+4px)] z-[1001] grid max-h-56 min-w-64 overflow-auto rounded-hcbMd border border-border bg-surface-0 p-1 shadow-xl">
      {suggestions.map((entry, index) => (
        <button
          aria-selected={index === activeIndex}
          className={cx(
            "grid min-h-8 grid-cols-[28px_minmax(0,1fr)] items-center gap-2 rounded-hcbSm px-2 text-left text-[var(--text-sm)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
            index === activeIndex ? "bg-bg-tertiary text-text-primary" : "text-text-secondary hover:bg-bg-tertiary"
          )}
          key={entry.id}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => onPick(entry)}
          role="option"
          type="button"
        >
          <span className="text-[var(--text-md)]">{entry.native}</span>
          <span className="truncate">:{entry.id}:</span>
        </button>
      ))}
    </div>
  );
}

export const EmojiInput = forwardRef<HTMLInputElement, Omit<InputHTMLAttributes<HTMLInputElement>, "onChange"> & {
  onValueChange: (value: string) => void;
}>(
  function EmojiInput({ className, onKeyDown, onValueChange, value = "", ...props }, ref) {
    const field = useEmojiField(String(value), onValueChange, onKeyDown as ((event: KeyboardEvent<EmojiElement>) => void) | undefined);

    return (
      <div className="relative min-w-0">
        <input
          className={cx(
            "h-8 w-full rounded-hcbMd border border-border bg-surface-0 px-3 text-[var(--text-base)] text-text-primary placeholder:text-text-muted transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
            className
          )}
          onChange={field.handleChange}
          onKeyDown={field.handleKeyDown}
          ref={(node) => {
            field.innerRef.current = node;
            setRefs(node, [ref]);
          }}
          value={value}
          {...props}
        />
        <EmojiSuggestions activeIndex={field.activeIndex} onPick={field.pickEmoji} suggestions={field.suggestions} />
      </div>
    );
  }
);

export const EmojiTextarea = forwardRef<HTMLTextAreaElement, Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, "onChange"> & {
  onValueChange: (value: string) => void;
}>(
  function EmojiTextarea({ className, onKeyDown, onValueChange, value = "", ...props }, ref) {
    const field = useEmojiField(String(value), onValueChange, onKeyDown as ((event: KeyboardEvent<EmojiElement>) => void) | undefined);

    return (
      <div className="relative min-w-0">
        <textarea
          className={cx(
            "w-full resize-none rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary placeholder:text-text-muted transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
            className
          )}
          onChange={field.handleChange}
          onKeyDown={field.handleKeyDown}
          ref={(node) => {
            field.innerRef.current = node;
            setRefs(node, [ref]);
          }}
          value={value}
          {...props}
        />
        <EmojiSuggestions activeIndex={field.activeIndex} onPick={field.pickEmoji} suggestions={field.suggestions} />
      </div>
    );
  }
);
