import { Fragment, useCallback, useState, useRef } from "react";
import type { KeyboardEvent } from "react";
import type { SettingsSnapshot } from "@shared/ipc/contracts";
import { ChevronDown, ChevronRight, EyeOff } from "lucide-react";
import appIconUrl from "../../../../../assets/brand/buns-app-icon-sidebar.png";
import { Badge, cx } from "../../components/primitives";
import type { SectionId } from "../../data/mockPlanner";
import { useI18n } from "../../i18n";
import { ariaKeyShortcuts } from "../core/hotkeys";
import type { CoreViewModelSource } from "../core/coreViewModelSource";
import { CalendarSourceSwatch } from "../core/screens/calendar/CalendarEventChips";
import { scheduleFrame, sectionMetric } from "./shellUtils";
import type { VisiblePrimarySection } from "./types";

function SidebarCalendarDropdown({
  defaultTimeZone,
  onShowAll,
  onToggle,
  open,
  setOpen,
  source,
  visibleCalendarIds
}: {
  defaultTimeZone: string;
  onShowAll: () => void;
  onToggle: (calendarId: string, visible: boolean) => void;
  open: boolean;
  setOpen: (open: boolean) => void;
  source: CoreViewModelSource;
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element | null {
  const { t } = useI18n();

  if (source.calendarSources.length === 0) {
    return null;
  }

  const hiddenCount = source.calendarSources.length - visibleCalendarIds.size;
  const ToggleIcon = open ? ChevronDown : ChevronRight;

  return (
    <div className="hidden px-3 pb-2 lg:block">
      <button
        aria-controls="sidebar-calendar-sources"
        aria-expanded={open}
        className="flex h-7 w-full items-center gap-2 rounded-hcbSm px-2 text-left text-[var(--text-xs)] font-semibold text-text-muted transition-colors duration-fast ease-hcb hover:bg-surface-0 hover:text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        onClick={() => setOpen(!open)}
        type="button"
      >
        <ToggleIcon aria-hidden="true" size={13} />
        <span className="min-w-0 flex-1 truncate">{t("nav.calendars")}</span>
        <Badge tone={hiddenCount > 0 ? "warning" : "neutral"}>{visibleCalendarIds.size}</Badge>
      </button>
      {open ? (
        <div
          aria-label="Sidebar calendar visibility"
          className="mt-1 grid gap-1"
          id="sidebar-calendar-sources"
          role="group"
        >
          {source.calendarSources.map((calendar) => {
            const visible = visibleCalendarIds.has(calendar.id);

            return (
              <label
                className={cx(
                  "grid min-h-8 grid-cols-[16px_12px_minmax(0,1fr)] items-center gap-2 rounded-hcbSm px-2 text-[var(--text-xs)] transition-colors duration-fast ease-hcb",
                  visible
                    ? "text-text-secondary hover:bg-surface-0 hover:text-text-primary"
                    : "text-text-muted"
                )}
                key={calendar.id}
                title={`${calendar.title} - ${calendar.timeZone ?? defaultTimeZone}`}
              >
                <input
                  aria-label={`${visible ? "Hide" : "Show"} ${calendar.title}`}
                  checked={visible}
                  className="accent-[var(--color-accent)]"
                  onChange={(event) => onToggle(calendar.id, event.target.checked)}
                  type="checkbox"
                />
                <CalendarSourceSwatch
                  calendarId={calendar.id}
                  className={visible ? undefined : "opacity-45"}
                  color={calendar.backgroundColor}
                />
                <span className="min-w-0 truncate">{calendar.title}</span>
              </label>
            );
          })}
          {hiddenCount > 0 ? (
            <button
              className="flex h-7 items-center gap-2 rounded-hcbSm px-2 text-left text-[var(--text-xs)] font-semibold text-text-muted transition-colors duration-fast ease-hcb hover:bg-surface-0 hover:text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              onClick={onShowAll}
              type="button"
            >
              <EyeOff aria-hidden="true" size={13} />
              <span>{t("nav.showAllCalendars")}</span>
            </button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

export function AppSidebar({
  activeSectionId,
  onShowAllCalendars,
  onToggleVisibleCalendar,
  onNavigateToSection,
  sidebarOnRight,
  source,
  visibleCalendarIds,
  visiblePrimarySections
}: {
  activeSectionId: SectionId;
  onShowAllCalendars: () => void;
  onToggleVisibleCalendar: (calendarId: string, visible: boolean) => void;
  onNavigateToSection: (sectionId: SectionId) => void;
  sidebarOnRight: boolean;
  source: CoreViewModelSource;
  visibleCalendarIds: ReadonlySet<string>;
  visiblePrimarySections: VisiblePrimarySection[];
}): JSX.Element {
  const { t } = useI18n();
  const sectionButtonRefs = useRef(new Map<SectionId, HTMLButtonElement>());
  const [calendarDropdownOpen, setCalendarDropdownOpen] = useState(true);

  const setSectionButtonRef = useCallback(
    (sectionId: SectionId) =>
      (node: HTMLButtonElement | null): void => {
        if (node) {
          sectionButtonRefs.current.set(sectionId, node);
        } else {
          sectionButtonRefs.current.delete(sectionId);
        }
      },
    []
  );

  const focusSection = useCallback(
    (sectionId: SectionId): void => {
      onNavigateToSection(sectionId);
      sectionButtonRefs.current.get(sectionId)?.focus();
      scheduleFrame(() => sectionButtonRefs.current.get(sectionId)?.focus());
    },
    [onNavigateToSection]
  );

  function handleNavigationKeyDown(event: KeyboardEvent<HTMLButtonElement>, sectionId: SectionId): void {
    const currentIndex = visiblePrimarySections.findIndex(({ section }) => section.id === sectionId);
    let nextIndex = currentIndex;

    if (event.key === "ArrowDown" || event.key === "ArrowRight") {
      nextIndex = Math.min(currentIndex + 1, visiblePrimarySections.length - 1);
    } else if (event.key === "ArrowUp" || event.key === "ArrowLeft") {
      nextIndex = Math.max(currentIndex - 1, 0);
    } else if (event.key === "Home") {
      nextIndex = 0;
    } else if (event.key === "End") {
      nextIndex = visiblePrimarySections.length - 1;
    } else {
      return;
    }

    event.preventDefault();
    focusSection(visiblePrimarySections[nextIndex]?.section.id ?? sectionId);
  }

  return (
    <aside
      className={cx(
        "flex min-h-0 min-w-0 flex-row items-center overflow-x-auto border-b border-border bg-bg-secondary md:flex-col md:items-stretch md:overflow-hidden md:border-b-0",
        sidebarOnRight ? "md:order-2 md:border-l" : "md:order-1 md:border-r"
      )}
      id="app-sidebar"
      style={{ fontFamily: "var(--font-family-sidebar)", fontSize: "var(--text-sidebar)" }}
    >
      <div className="flex h-14 w-14 shrink-0 items-center justify-center border-r border-border px-3 md:w-auto md:border-b md:border-r-0 lg:justify-start lg:gap-3 lg:px-4">
        <img
          alt=""
          aria-hidden="true"
          className="size-8 rounded-hcbMd object-cover"
          draggable={false}
          src={appIconUrl}
        />
        <div className="hidden min-w-0 lg:block">
          <div className="truncate text-[var(--text-md)] font-semibold">Hot Cross Buns</div>
        </div>
      </div>

      <nav aria-label="Primary" className="flex min-h-0 min-w-0 flex-1 gap-1 overflow-x-auto px-2 py-2 md:flex-col md:overflow-x-hidden md:overflow-y-auto md:py-3">
        {visiblePrimarySections.map(({ section }) => {
          const Icon = section.icon;
          const selected = section.id === activeSectionId;
          const label = sectionLabel(section.id, t);
          const shortcut = ariaKeyShortcuts(
            source.settings.keybindings[`navigation.${section.id}` as keyof SettingsSnapshot["keybindings"]]
          );

          return (
            <Fragment key={section.id}>
              <button
                aria-current={selected ? "page" : undefined}
                aria-keyshortcuts={shortcut}
                aria-label={label}
                className={cx(
                  "flex h-9 w-auto min-w-9 items-center justify-center gap-3 rounded-hcbMd px-2 text-left text-[var(--text-base)] transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent md:w-full lg:justify-start lg:px-3",
                  selected
                    ? "bg-surface-0 text-text-primary"
                    : "text-text-secondary hover:bg-surface-0 hover:text-text-primary"
                )}
                onClick={() => onNavigateToSection(section.id)}
                onKeyDown={(event) => handleNavigationKeyDown(event, section.id)}
                ref={setSectionButtonRef(section.id)}
                title={label}
                type="button"
              >
                <Icon aria-hidden="true" className="shrink-0" size={16} strokeWidth={2} />
                <span className="hidden min-w-0 flex-1 truncate lg:inline">{label}</span>
                <span className="hidden shrink-0 text-[var(--text-xs)] text-text-muted lg:inline">
                  {sectionMetric(source, section.id)}
                </span>
              </button>
              {section.id === "calendar" ? (
                <SidebarCalendarDropdown
                  defaultTimeZone={source.settings.defaultTimeZone}
                  onShowAll={onShowAllCalendars}
                  onToggle={onToggleVisibleCalendar}
                  open={calendarDropdownOpen}
                  setOpen={setCalendarDropdownOpen}
                  source={source}
                  visibleCalendarIds={visibleCalendarIds}
                />
              ) : null}
            </Fragment>
          );
        })}
      </nav>

    </aside>
  );
}

function sectionLabel(sectionId: SectionId, t: ReturnType<typeof useI18n>["t"]): string {
  if (sectionId === "calendar") {
    return t("nav.calendar");
  }

  if (sectionId === "tasks") {
    return t("nav.tasks");
  }

  if (sectionId === "notes") {
    return t("nav.notes");
  }

  return sectionId;
}
