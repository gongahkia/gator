import { createContext, isValidElement, useContext, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cx } from "../../../../components/primitives";

export const settingsSelectClass =
  "h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

interface SettingsSearchContextValue {
  groupTitleMatches: boolean;
  query: string;
}

interface SearchableElementProps {
  "aria-label"?: unknown;
  children?: ReactNode;
  description?: unknown;
  label?: unknown;
  title?: unknown;
}

const SettingsSearchContext = createContext<SettingsSearchContextValue>({
  groupTitleMatches: false,
  query: ""
});

function normalizedSettingsSearchText(value: string): string {
  return value.trim().toLowerCase();
}

function compactSettingsSearchText(value: string): string {
  return normalizedSettingsSearchText(value).replace(/[^a-z0-9]+/g, "");
}

function searchableValue(value: unknown): string {
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}

function collectSearchableText(node: ReactNode): string {
  if (node == null || typeof node === "boolean") {
    return "";
  }

  if (typeof node === "string" || typeof node === "number") {
    return String(node);
  }

  if (Array.isArray(node)) {
    return node.map(collectSearchableText).join(" ");
  }

  if (isValidElement<SearchableElementProps>(node)) {
    return [
      searchableValue(node.props.label),
      searchableValue(node.props.description),
      searchableValue(node.props.title),
      searchableValue(node.props["aria-label"]),
      collectSearchableText(node.props.children)
    ]
      .filter(Boolean)
      .join(" ");
  }

  return "";
}

export function settingsSearchMatches(text: string, query: string): boolean {
  const normalizedQuery = normalizedSettingsSearchText(query);

  if (!normalizedQuery) {
    return true;
  }

  const normalizedText = normalizedSettingsSearchText(text);

  if (normalizedText.includes(normalizedQuery)) {
    return true;
  }

  const compactQuery = compactSettingsSearchText(query);

  return compactQuery.length > 0 && compactSettingsSearchText(text).includes(compactQuery);
}

export function SettingsSearchProvider({
  children,
  query
}: {
  children: ReactNode;
  query: string;
}): JSX.Element {
  return (
    <SettingsSearchContext.Provider value={{ groupTitleMatches: false, query }}>
      {children}
    </SettingsSearchContext.Provider>
  );
}

export function SettingsTabButton({
  active,
  alertCount = 0,
  icon: _Icon,
  label,
  onClick
}: {
  active: boolean;
  alertCount?: number;
  icon: LucideIcon;
  label: string;
  onClick: () => void;
}): JSX.Element {
  return (
    <button
      aria-pressed={active}
      className={cx(
        "inline-flex h-8 shrink-0 items-center justify-center gap-2 rounded-hcbMd border px-2.5 text-[var(--text-base)] font-medium transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
        active
          ? "border-border bg-surface-0 text-text-primary"
          : "border-transparent text-text-muted hover:bg-surface-0 hover:text-text-primary"
      )}
      onClick={onClick}
      type="button"
    >
      <span className="truncate">{label}</span>
      {alertCount > 0 ? (
        <span
          aria-hidden="true"
          className="inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-danger px-1 text-[10px] font-bold leading-none text-bg-tertiary"
          title={`${alertCount} ${alertCount === 1 ? "issue" : "issues"}`}
        >
          {alertCount}
        </span>
      ) : null}
    </button>
  );
}

export function SettingsGroup({
  children,
  searchText = "",
  title
}: {
  children: ReactNode;
  searchText?: string;
  title: string;
}): JSX.Element | null {
  const search = useContext(SettingsSearchContext);
  const hasQuery = search.query.trim().length > 0;
  const groupTitleMatches = settingsSearchMatches(title, search.query);
  const groupContentMatches =
    groupTitleMatches || settingsSearchMatches(`${searchText} ${collectSearchableText(children)}`, search.query);

  if (hasQuery && !groupContentMatches) {
    return null;
  }

  return (
    <SettingsSearchContext.Provider value={{ groupTitleMatches, query: search.query }}>
      <section className="grid gap-1.5">
        <h2 className="px-1 text-[var(--text-md)] font-semibold text-text-primary">{title}</h2>
        <div className="overflow-hidden rounded-hcbMd border border-border bg-bg-secondary">
          {children}
        </div>
      </section>
    </SettingsSearchContext.Provider>
  );
}

export function SettingsControlRow({
  children,
  description,
  icon: _Icon,
  label
}: {
  children?: ReactNode;
  description?: string;
  icon?: LucideIcon;
  label: string;
}): JSX.Element | null {
  const search = useContext(SettingsSearchContext);
  const hasQuery = search.query.trim().length > 0;
  const rowMatches = settingsSearchMatches(`${label} ${description ?? ""} ${collectSearchableText(children)}`, search.query);

  if (hasQuery && !search.groupTitleMatches && !rowMatches) {
    return null;
  }

  return (
    <div className="grid min-h-11 gap-2 border-b border-border px-3 py-2 last:border-b-0 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
      <div className="flex min-w-0 items-start gap-2.5">
        <div className="min-w-0">
          <div className="truncate text-[var(--text-base)] font-medium text-text-primary">{label}</div>
          {description ? (
            <p className="mt-0.5 text-[var(--text-sm)] text-text-muted">{description}</p>
          ) : null}
        </div>
      </div>
      {children ? (
        <div className="flex min-w-0 items-center justify-start sm:justify-end">{children}</div>
      ) : null}
    </div>
  );
}

export function SettingsSwitch({
  checked,
  description,
  icon,
  label,
  onChange,
  trailing
}: {
  checked: boolean;
  description?: string;
  icon?: LucideIcon;
  label: string;
  onChange: (checked: boolean) => void;
  trailing?: ReactNode;
}): JSX.Element | null {
  return (
    <SettingsControlRow description={description} icon={icon} label={label}>
      <div className="flex items-center gap-3">
        {trailing}
        <input
          aria-label={label}
          checked={checked}
          className="h-5 w-9 accent-[var(--color-accent)]"
          onChange={(event) => onChange(event.target.checked)}
          type="checkbox"
        />
      </div>
    </SettingsControlRow>
  );
}

export function SegmentedControl({
  onChange,
  options,
  value
}: {
  onChange: (value: string) => void;
  options: Array<{ icon?: LucideIcon; label: string; value: string }>;
  value: string;
}): JSX.Element {
  return (
    <div className="inline-flex max-w-full overflow-hidden rounded-hcbMd border border-border bg-surface-0 p-1">
      {options.map((option) => {
        const active = option.value === value;

        return (
          <button
            aria-pressed={active}
            className={cx(
              "inline-flex h-7 min-w-20 items-center justify-center gap-2 rounded-hcbSm px-3 text-[var(--text-sm)] font-semibold transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
              active ? "bg-accent text-bg-tertiary" : "text-text-secondary hover:bg-surface-1 hover:text-text-primary"
            )}
            key={option.value}
            onClick={() => onChange(option.value)}
            type="button"
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
