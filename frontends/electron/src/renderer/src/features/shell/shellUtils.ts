import type { CoreViewModelSource } from "../core/coreViewModelSource";
import type { SectionId } from "../../data/mockPlanner";

export function scheduleFrame(callback: () => void): void {
  if (typeof window.requestAnimationFrame === "function") {
    window.requestAnimationFrame(callback);
    return;
  }

  window.setTimeout(callback, 0);
}

export function shellCanBeReported(source: CoreViewModelSource): boolean {
  return (
    source.appearanceReady ||
    source.dataState === "offline" ||
    source.dataState === "error"
  );
}

export function isEditableShortcutTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  const tagName = target.tagName.toLowerCase();
  return (
    target.isContentEditable ||
    tagName === "input" ||
    tagName === "select" ||
    tagName === "textarea"
  );
}

export function sectionMetric(source: CoreViewModelSource, sectionId: SectionId): string {
  if (sectionId === "tasks") {
    if (source.resourceCounts.tasks === null) {
      return deferredCountMetric(source);
    }

    return String(
      source.largeTaskWindow.filter((task) =>
        task.parentId === null &&
        task.dueDate !== null &&
        (task.status === "open" || task.status === "completed")
      ).length
    );
  }

  if (sectionId === "calendar") {
    return deferredCountMetric(source, source.resourceCounts.calendarEvents);
  }

  if (sectionId === "notes") {
    if (source.resourceCounts.notes === null) {
      return deferredCountMetric(source);
    }

    const loadedNoteCount = source.noteLists.reduce((count, list) => count + list.noteCount, 0);
    return String(source.noteLists.length > 0 ? loadedNoteCount : source.resourceCounts.notes);
  }

  if (sectionId === "settings") {
    return source.syncStatus.state;
  }

  return "0";
}

function deferredCountMetric(source: CoreViewModelSource, count: number | null = null): string {
  if (count !== null) {
    return String(count);
  }

  return source.hydrationState === "failed" ? "!" : "...";
}
