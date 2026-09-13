import {
  defaultHistoryCategoryVisibility,
  googleCalendarEventColors
} from "@shared/ipc/contracts";
import {
  previewAutoTagRules,
  validateAutoTagRule
} from "@shared/ipc/autoTags";
import type { AutoTagTargetKind } from "@shared/ipc/autoTags";
import type {
  AutoTagRule,
  CalendarListSummary,
  CustomizationStatusResponse,
  IcsSubscriptionsResponse,
  LocalPointerListResponse,
  PortableImportPreview,
  SettingsRecoveryActionRequest,
  SettingsSnapshot,
  SettingsUpdateRequest,
  TagCreateRequest,
  TagDeleteRequest,
  TagAnalyticsResponse,
  TagMergeRequest,
  TagMutationResponse,
  TagSummary,
  TagUpdateRequest,
  TaskListSummary
} from "@shared/ipc/contracts";
import {
  ArrowDown,
  ArrowUp,
  Archive,
  CalendarDays,
  ChevronRight,
  Code2,
  Database,
  FileDown,
  FilePlus2,
  FileUp,
  Filter,
  History,
  Layers3,
  Link2,
  ListChecks,
  Paperclip,
  Pin,
  Puzzle,
  RotateCcw,
  Save,
  Tag,
  Trash2,
  Upload,
  X
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Badge, Button, Input } from "../../../../components/primitives";
import { EmptyState } from "../../../../components/states";
import { parseTagText } from "../../TagInput";
import {
  SettingsControlRow,
  SettingsGroup,
  SettingsSwitch,
  settingsSelectClass
} from "./SettingsPrimitives";

interface AdvancedSettingsTabProps {
  autoTagBulkCounts: Record<AutoTagTargetKind, number>;
  beginRecoveryAction: (action: SettingsRecoveryActionRequest["action"]) => void;
  calendarSources: CalendarListSummary[];
  settings: SettingsSnapshot;
  tags: TagSummary[];
  taskLists: TaskListSummary[];
  createTag: (request: TagCreateRequest) => Promise<TagMutationResponse | null>;
  defaultTagColor: string;
  updateTag: (request: TagUpdateRequest) => Promise<TagMutationResponse | null>;
  deleteTag: (request: TagDeleteRequest) => Promise<TagMutationResponse | null>;
  mergeTags: (request: TagMergeRequest) => Promise<TagMutationResponse | null>;
  onReapplyAutoTags: (kind: AutoTagTargetKind) => void;
  updateSelectedCalendar: (calendarId: string, selected: boolean) => void;
  updateSelectedTaskList: (taskListId: string, selected: boolean) => void;
  updateSettings: (request: SettingsUpdateRequest) => void;
}

const historyCategoryLabels: Record<keyof SettingsSnapshot["historyCategoryVisibility"], string> = {
  created: "Created",
  edited: "Edited",
  deleted: "Deleted",
  completedReopened: "Completed / reopened",
  duplicated: "Duplicated",
  movedBetweenLists: "Moved between lists",
  clipboard: "Clipboard (copy / paste / cut)",
  restored: "Restored",
  bulkActions: "Bulk actions",
  syncDiffs: "Sync diffs",
  other: "Other"
};

const autoTagTargetKinds: AutoTagTargetKind[] = ["task", "event", "note"];
type AutoTagPreviewLocalKind = "normal" | "birthday";

function autoTagRuleHasError(rule: AutoTagRule): boolean {
  return validateAutoTagRule(rule).some((issue) => issue.severity === "error");
}

async function fileToBase64(file: File): Promise<string> {
  const buffer = await file.arrayBuffer();
  let binary = "";
  const bytes = new Uint8Array(buffer);

  for (let index = 0; index < bytes.length; index += 1) {
    binary += String.fromCharCode(bytes[index] ?? 0);
  }

  return btoa(binary);
}

function autoDisableInvalidAutoTagRules(rules: AutoTagRule[], now: string): AutoTagRule[] {
  let changed = false;
  const nextRules = rules.map((rule) => {
    if (!rule.enabled || !autoTagRuleHasError(rule)) {
      return rule;
    }

    changed = true;
    return { ...rule, enabled: false, updatedAt: now };
  });

  return changed ? nextRules : rules;
}

export function AdvancedSettingsTab({
  autoTagBulkCounts,
  beginRecoveryAction,
  calendarSources,
  createTag,
  defaultTagColor,
  deleteTag,
  mergeTags,
  onReapplyAutoTags,
  settings,
  tags,
  taskLists,
  updateTag,
  updateSelectedCalendar,
  updateSelectedTaskList,
  updateSettings
}: AdvancedSettingsTabProps): JSX.Element {
  const selectedTaskLists = new Set(settings.selectedTaskListIds);
  const selectedCalendars = new Set(settings.selectedCalendarIds);
  const noteTemplates = settings.noteTemplates ?? [];
  const [editingTaskTemplateId, setEditingTaskTemplateId] = useState<string | null>(null);
  const [editingEventTemplateId, setEditingEventTemplateId] = useState<string | null>(null);
  const [newTagName, setNewTagName] = useState("");
  const [newTagColor, setNewTagColor] = useState(defaultTagColor);
  const [tagDrafts, setTagDrafts] = useState<Record<string, { color: string; name: string }>>({});
  const [tagMergeSourceId, setTagMergeSourceId] = useState("");
  const [tagMergeTargetId, setTagMergeTargetId] = useState("");
  const [tagAnalytics, setTagAnalytics] = useState<TagAnalyticsResponse | null>(null);
  const [autoTagPreviewKind, setAutoTagPreviewKind] = useState<AutoTagTargetKind>("task");
  const [autoTagPreviewTitle, setAutoTagPreviewTitle] = useState("CODING: Ship planner polish");
  const [autoTagPreviewBody, setAutoTagPreviewBody] = useState("");
  const [autoTagPreviewExistingTags, setAutoTagPreviewExistingTags] = useState("");
  const [autoTagPreviewExplicitTags, setAutoTagPreviewExplicitTags] = useState("");
  const [autoTagPreviewExistingColorId, setAutoTagPreviewExistingColorId] = useState("");
  const [autoTagPreviewRequestedColorId, setAutoTagPreviewRequestedColorId] = useState("");
  const [autoTagPreviewLocalKind, setAutoTagPreviewLocalKind] = useState<AutoTagPreviewLocalKind>("normal");
  const [portableArchivePath, setPortableArchivePath] = useState("");
  const [portableImportPreview, setPortableImportPreview] = useState<PortableImportPreview | null>(null);
  const [portableStatus, setPortableStatus] = useState<string | null>(null);
  const [semanticStatus, setSemanticStatus] = useState<string | null>(null);
  const [localPointers, setLocalPointers] = useState<LocalPointerListResponse | null>(null);
  const [pointerReplacementPath, setPointerReplacementPath] = useState("");
  const [customizationStatus, setCustomizationStatus] = useState<CustomizationStatusResponse | null>(null);
  const [customizationMessage, setCustomizationMessage] = useState<string | null>(null);
  const [icsSubscriptions, setIcsSubscriptions] = useState<IcsSubscriptionsResponse | null>(null);
  const [icsUrl, setIcsUrl] = useState("");
  const [icsTitle, setIcsTitle] = useState("");
  const [icsMessage, setIcsMessage] = useState<string | null>(null);
  const [reportMessage, setReportMessage] = useState<string | null>(null);
  const autoTagAutoDisableRequestRef = useRef<string | null>(null);
  const previousDefaultTagColorRef = useRef(defaultTagColor);
  const autoTagPreviewExistingTagValues = useMemo(
    () => parseTagText(autoTagPreviewExistingTags),
    [autoTagPreviewExistingTags]
  );
  const autoTagPreviewExplicitTagValues = useMemo(
    () => parseTagText(autoTagPreviewExplicitTags),
    [autoTagPreviewExplicitTags]
  );
  const autoTagPreview = useMemo(
    () =>
      previewAutoTagRules(settings.autoTagRules, {
        kind: autoTagPreviewKind,
        title: autoTagPreviewTitle,
        body: autoTagPreviewBody,
        explicitTags: autoTagPreviewExplicitTagValues,
        existingTags: autoTagPreviewExistingTagValues,
        existingEventColorId: autoTagPreviewExistingColorId || null,
        requestedEventColorId: autoTagPreviewRequestedColorId || null,
        hcbKind: autoTagPreviewLocalKind === "birthday" ? "birthday" : null
      }),
    [
      autoTagPreviewBody,
      autoTagPreviewExistingColorId,
      autoTagPreviewExistingTagValues,
      autoTagPreviewExplicitTagValues,
      autoTagPreviewKind,
      autoTagPreviewLocalKind,
      autoTagPreviewRequestedColorId,
      autoTagPreviewTitle,
      settings.autoTagRules
    ]
  );
  const autoTagRuleIssues = useMemo(
    () => settings.autoTagRules.flatMap((rule) => validateAutoTagRule(rule)),
    [settings.autoTagRules]
  );
  const autoTagErrors = autoTagRuleIssues.filter((issue) => issue.severity === "error");
  const autoTagWarnings = autoTagRuleIssues.filter((issue) => issue.severity === "warning");

  useEffect(() => {
    if (newTagColor === previousDefaultTagColorRef.current) {
      setNewTagColor(defaultTagColor);
    }

    previousDefaultTagColorRef.current = defaultTagColor;
  }, [defaultTagColor, newTagColor]);

  useEffect(() => {
    const invalidEnabledRuleKey = settings.autoTagRules
      .filter((rule) => rule.enabled && autoTagRuleHasError(rule))
      .map((rule) => rule.id)
      .join(",");

    if (!invalidEnabledRuleKey) {
      autoTagAutoDisableRequestRef.current = null;
      return;
    }

    if (autoTagAutoDisableRequestRef.current === invalidEnabledRuleKey) {
      return;
    }

    autoTagAutoDisableRequestRef.current = invalidEnabledRuleKey;
    const nextRules = autoDisableInvalidAutoTagRules(settings.autoTagRules, new Date().toISOString());

    if (nextRules !== settings.autoTagRules) {
      updateSettings({ autoTagRules: nextRules });
    }
  }, [settings.autoTagRules, updateSettings]);

  useEffect(() => {
    void refreshTagAnalytics();
    void refreshLocalPointers();
    void refreshCustomizationStatus();
    void refreshIcsSubscriptions();
  }, []);

  async function refreshTagAnalytics(): Promise<void> {
    const result = await window.hcb?.tags.analytics();
    if (result?.ok) {
      setTagAnalytics(result.data);
    }
  }

  async function refreshLocalPointers(): Promise<void> {
    const result = await window.hcb?.settings.listLocalPointers({ includeHealthy: false, limit: 100 });
    if (result?.ok) {
      setLocalPointers(result.data);
    }
  }

  async function refreshCustomizationStatus(): Promise<void> {
    const result = await window.hcb?.settings.customizationStatus();
    if (result?.ok) {
      setCustomizationStatus(result.data);
    }
  }

  async function reloadCustomization(): Promise<void> {
    const result = await window.hcb?.settings.reloadCustomization();
    if (!result?.ok) {
      setCustomizationMessage(result?.error.message ?? "Customization reload failed.");
      return;
    }
    setCustomizationStatus(result.data);
    setCustomizationMessage("Customization reloaded.");
  }

  async function setSnippetEnabled(id: string, enabled: boolean): Promise<void> {
    const result = await window.hcb?.settings.setSnippetEnabled({ id, enabled });
    if (!result?.ok) {
      setCustomizationMessage(result?.error.message ?? "Snippet update failed.");
      return;
    }
    setCustomizationStatus(result.data);
  }

  async function setExtensionEnabled(id: string, enabled: boolean): Promise<void> {
    const result = await window.hcb?.settings.setExtensionEnabled({ id, enabled });
    if (!result?.ok) {
      setCustomizationMessage(result?.error.message ?? "Extension update failed.");
      return;
    }
    setCustomizationStatus(result.data);
  }

  async function refreshIcsSubscriptions(): Promise<void> {
    const result = await window.hcb?.settings.listIcsSubscriptions();
    if (result?.ok) {
      setIcsSubscriptions(result.data);
    }
  }

  async function importIcsFile(file: File): Promise<void> {
    setIcsMessage("Importing ICS file.");
    const result = await window.hcb?.settings.importIcs({
      fileName: file.name,
      dataBase64: await fileToBase64(file)
    });
    if (!result?.ok) {
      setIcsMessage(result?.error.message ?? "ICS import failed.");
      return;
    }
    setIcsMessage(`Imported ${result.data.importedEventCount} events into ${result.data.calendarTitle}.`);
  }

  async function subscribeIcs(): Promise<void> {
    if (!icsUrl.trim()) {
      setIcsMessage("Enter an ICS URL.");
      return;
    }
    setIcsMessage("Subscribing to ICS feed.");
    const result = await window.hcb?.settings.subscribeIcs({
      url: icsUrl.trim(),
      ...(icsTitle.trim() ? { title: icsTitle.trim() } : {}),
      refreshMinutes: 360
    });
    if (!result?.ok) {
      setIcsMessage(result?.error.message ?? "ICS subscription failed.");
      return;
    }
    setIcsSubscriptions(result.data);
    setIcsUrl("");
    setIcsTitle("");
    setIcsMessage("ICS subscription saved.");
  }

  async function refreshIcsSubscription(id: string): Promise<void> {
    setIcsMessage("Refreshing ICS subscription.");
    const result = await window.hcb?.settings.refreshIcsSubscription({ id });
    if (!result?.ok) {
      setIcsMessage(result?.error.message ?? "ICS refresh failed.");
      return;
    }
    setIcsSubscriptions(result.data);
    setIcsMessage("ICS subscription refreshed.");
  }

  async function deleteIcsSubscription(id: string): Promise<void> {
    const result = await window.hcb?.settings.deleteIcsSubscription({ id });
    if (!result?.ok) {
      setIcsMessage(result?.error.message ?? "ICS delete failed.");
      return;
    }
    setIcsSubscriptions(result.data);
    setIcsMessage("ICS subscription removed.");
  }

  async function exportLocalReport(range: "today" | "week", format: "markdown" | "csv" | "ics"): Promise<void> {
    setReportMessage("Exporting local report.");
    const result = await window.hcb?.settings.exportLocalReport({ range, format });
    if (!result?.ok) {
      setReportMessage(result?.error.message ?? "Local report export failed.");
      return;
    }
    setReportMessage(`Exported ${result.data.itemCount} items to ${result.data.path}.`);
  }

  function updatePerTabFilter(
    tab: keyof SettingsSnapshot["perTabListFilters"],
    patch: Partial<SettingsSnapshot["perTabListFilters"][typeof tab]>
  ): void {
    updateSettings({
      perTabListFilters: {
        ...settings.perTabListFilters,
        [tab]: {
          ...settings.perTabListFilters[tab],
          ...patch
        }
      }
    });
  }

  function togglePerTabList(tab: keyof SettingsSnapshot["perTabListFilters"], taskListId: string, selected: boolean): void {
    const current = new Set(settings.perTabListFilters[tab].selectedTaskListIds);

    if (selected) {
      current.add(taskListId);
    } else {
      current.delete(taskListId);
    }

    updatePerTabFilter(tab, { selectedTaskListIds: [...current] });
  }

  function addSavedFilter(): void {
    const now = new Date().toISOString();
    updateSettings({
      savedSearchViews: [
        ...settings.savedSearchViews,
        {
          id: crypto.randomUUID(),
          name: `Filter ${settings.savedSearchViews.length + 1}`,
          query: "source:tasks status:active",
          createdAt: now,
          updatedAt: now
        }
      ]
    });
  }

  function updateSavedFilter(
    filterId: string,
    patch: Partial<SettingsSnapshot["savedSearchViews"][number]>
  ): void {
    const now = new Date().toISOString();
    updateSettings({
      savedSearchViews: settings.savedSearchViews.map((filter) =>
        filter.id === filterId ? { ...filter, ...patch, updatedAt: now } : filter
      )
    });
  }

  function removeSavedFilter(filterId: string): void {
    updateSettings({
      savedSearchViews: settings.savedSearchViews.filter((candidate) => candidate.id !== filterId),
      pinnedSavedSearchViewIds: settings.pinnedSavedSearchViewIds.filter((candidate) => candidate !== filterId)
    });
  }

  function togglePinnedSavedFilter(filterId: string): void {
    const pinned = new Set(settings.pinnedSavedSearchViewIds);

    if (pinned.has(filterId)) {
      pinned.delete(filterId);
    } else {
      pinned.add(filterId);
    }

    updateSettings({ pinnedSavedSearchViewIds: [...pinned].slice(0, 20) });
  }

  async function installSemanticModel(modelId: string): Promise<void> {
    setSemanticStatus("Installing model...");
    const result = await window.hcb?.search.installModel({ modelId });

    if (!result?.ok) {
      setSemanticStatus(result?.error.message ?? "Model install failed.");
      return;
    }

    updateSettings({
      embeddingModelId: result.data.selectedModelId,
      semanticSearchEnabled: result.data.enabled,
      semanticSearchModels: settings.semanticSearchModels.map((model) =>
        model.id === result.data.model.id ? result.data.model : model
      )
    });
    setSemanticStatus(`${result.data.model.label} installed.`);
  }

  async function uninstallSemanticModel(modelId: string): Promise<void> {
    setSemanticStatus("Uninstalling model...");
    const result = await window.hcb?.search.uninstallModel({ modelId });

    if (!result?.ok) {
      setSemanticStatus(result?.error.message ?? "Model uninstall failed.");
      return;
    }

    updateSettings({
      embeddingModelId: result.data.selectedModelId,
      semanticSearchEnabled: result.data.enabled,
      semanticSearchModels: settings.semanticSearchModels.map((model) =>
        model.id === result.data.model.id ? result.data.model : model
      )
    });
    setSemanticStatus(`${result.data.model.label} uninstalled.`);
  }

  async function rebuildSemanticIndex(): Promise<void> {
    setSemanticStatus("Rebuilding index...");
    const result = await window.hcb?.search.rebuildIndex({ modelId: settings.embeddingModelId });

    if (!result?.ok) {
      setSemanticStatus(result?.error.message ?? "Index rebuild failed.");
      return;
    }

    setSemanticStatus(
      result.data.unavailableReason ??
        `Indexed ${result.data.indexedCount} entities; ${result.data.staleCount} stale.`
    );
  }

  async function addTag(): Promise<void> {
    const name = newTagName.trim();

    if (!name) {
      return;
    }

    const result = await createTag({ name, color: newTagColor || null });

    if (result) {
      setNewTagName("");
      void refreshTagAnalytics();
    }
  }

  function tagDraft(tag: TagSummary): { color: string; name: string } {
    return tagDrafts[tag.id] ?? { color: tag.color ?? defaultTagColor, name: tag.name };
  }

  function setTagDraft(tag: TagSummary, patch: Partial<{ color: string; name: string }>): void {
    const current = tagDraft(tag);
    setTagDrafts({
      ...tagDrafts,
      [tag.id]: { ...current, ...patch }
    });
  }

  async function saveTag(tag: TagSummary): Promise<void> {
    const draft = tagDraft(tag);
    const name = draft.name.trim();
    const color = draft.color || null;

    if (!name) {
      return;
    }

    if (name === tag.name && color === tag.color) {
      return;
    }

    const result = await updateTag({
      id: tag.id,
      ...(name === tag.name ? {} : { name }),
      ...(color === tag.color ? {} : { color })
    });

    if (result) {
      const { [tag.id]: _removed, ...rest } = tagDrafts;
      setTagDrafts(rest);
      void refreshTagAnalytics();
    }
  }

  async function removeTag(tag: TagSummary): Promise<void> {
    if (!window.confirm(`Delete tag "${tag.name}"? Existing entity tag values are removed locally.`)) {
      return;
    }

    await deleteTag({ id: tag.id });
    void refreshTagAnalytics();
  }

  async function mergeSelectedTags(): Promise<void> {
    if (!tagMergeSourceId || !tagMergeTargetId || tagMergeSourceId === tagMergeTargetId) {
      return;
    }

    const result = await mergeTags({ sourceId: tagMergeSourceId, targetId: tagMergeTargetId });

    if (result) {
      setTagMergeSourceId("");
      setTagMergeTargetId("");
      void refreshTagAnalytics();
    }
  }

  function addTaskTemplate(): void {
    const now = new Date().toISOString();
    const templateId = crypto.randomUUID();
    updateSettings({
      taskTemplates: [
        ...settings.taskTemplates,
        {
          id: templateId,
          name: `Task Template ${settings.taskTemplates.length + 1}`,
          title: "{{prompt:Title}}",
          notes: null,
          dueExpression: "{{today}}",
          listId: null,
          createdAt: now,
          updatedAt: now
        }
      ]
    });
    setEditingTaskTemplateId(templateId);
  }

  function addEventTemplate(): void {
    const now = new Date().toISOString();
    const templateId = crypto.randomUUID();
    updateSettings({
      eventTemplates: [
        ...settings.eventTemplates,
        {
          id: templateId,
          name: `Event Template ${settings.eventTemplates.length + 1}`,
          title: "{{prompt:Topic}}",
          notes: null,
          location: null,
          calendarId: null,
          startExpression: "{{today}} 09:00",
          endExpression: "{{today}} 10:00",
          attendeeEmails: [],
          createdAt: now,
          updatedAt: now
        }
      ]
    });
    setEditingEventTemplateId(templateId);
  }

  function updateTaskTemplate(
    templateId: string,
    patch: Partial<SettingsSnapshot["taskTemplates"][number]>
  ): void {
    const now = new Date().toISOString();
    updateSettings({
      taskTemplates: settings.taskTemplates.map((template) =>
        template.id === templateId ? { ...template, ...patch, updatedAt: now } : template
      )
    });
  }

  function updateEventTemplate(
    templateId: string,
    patch: Partial<SettingsSnapshot["eventTemplates"][number]>
  ): void {
    const now = new Date().toISOString();
    updateSettings({
      eventTemplates: settings.eventTemplates.map((template) =>
        template.id === templateId ? { ...template, ...patch, updatedAt: now } : template
      )
    });
  }

  function removeTaskTemplate(templateId: string): void {
    updateSettings({
      taskTemplates: settings.taskTemplates.filter((template) => template.id !== templateId)
    });
    setEditingTaskTemplateId(null);
  }

  function removeEventTemplate(templateId: string): void {
    updateSettings({
      eventTemplates: settings.eventTemplates.filter((template) => template.id !== templateId)
    });
    setEditingEventTemplateId(null);
  }

  function addNoteTemplate(): void {
    const now = new Date().toISOString();
    updateSettings({
      noteTemplates: [
        ...noteTemplates,
        {
          id: crypto.randomUUID(),
          name: `Note Template ${noteTemplates.length + 1}`,
          title: "Untitled note",
          body: "",
          createdAt: now,
          updatedAt: now
        }
      ]
    });
  }

  function updateNoteTemplate(
    templateId: string,
    patch: Partial<SettingsSnapshot["noteTemplates"][number]>
  ): void {
    const now = new Date().toISOString();
    updateSettings({
      noteTemplates: noteTemplates.map((template) =>
        template.id === templateId ? { ...template, ...patch, updatedAt: now } : template
      )
    });
  }

  function saveAutoTagRules(autoTagRules: AutoTagRule[]): void {
    updateSettings({
      autoTagRules: autoDisableInvalidAutoTagRules(autoTagRules, new Date().toISOString())
    });
  }

  function addAutoTagRule(): void {
    const now = new Date().toISOString();
    saveAutoTagRules([
      ...settings.autoTagRules,
      {
        id: crypto.randomUUID(),
        name: `Auto tag ${settings.autoTagRules.length + 1}`,
        enabled: true,
        targetKinds: ["task", "event", "note"],
        matchField: "title",
        matchType: "prefix",
        pattern: "TODO",
        tags: ["todo"],
        stripMatchedPrefix: false,
        eventColorId: null,
        overrideExistingEventColor: false,
        createdAt: now,
        updatedAt: now
      }
    ]);
  }

  function moveAutoTagRule(fromIndex: number, toIndex: number): void {
    if (toIndex < 0 || toIndex >= settings.autoTagRules.length || fromIndex === toIndex) {
      return;
    }

    const nextRules = [...settings.autoTagRules];
    const [rule] = nextRules.splice(fromIndex, 1);

    if (!rule) {
      return;
    }

    nextRules.splice(toIndex, 0, rule);
    saveAutoTagRules(nextRules);
  }

  function updateAutoTagRule(ruleId: string, patch: Partial<AutoTagRule>): void {
    const now = new Date().toISOString();
    saveAutoTagRules(
      settings.autoTagRules.map((rule) =>
        rule.id === ruleId ? { ...rule, ...patch, updatedAt: now } : rule
      )
    );
  }

  function toggleAutoTagTarget(rule: AutoTagRule, kind: AutoTagTargetKind, checked: boolean): void {
    const targetKinds = checked
      ? [...new Set([...rule.targetKinds, kind])]
      : rule.targetKinds.filter((candidate) => candidate !== kind);

    updateAutoTagRule(rule.id, { targetKinds: targetKinds.length > 0 ? targetKinds : [kind] });
  }

  async function exportPortableArchive(): Promise<void> {
    setPortableStatus("Exporting portable archive.");
    const result = await window.hcb?.settings.exportPortableArchive();

    if (!result?.ok) {
      setPortableStatus(result?.error.message ?? "Portable export failed.");
      return;
    }

    setPortableArchivePath(result.data.path);
    setPortableStatus(`Exported ${result.data.path}`);
  }

  async function previewPortableArchive(): Promise<void> {
    if (!portableArchivePath.trim()) {
      setPortableStatus("Enter a .hcbexport path.");
      return;
    }

    const result = await window.hcb?.settings.previewPortableImport({ path: portableArchivePath.trim() });

    if (!result?.ok) {
      setPortableImportPreview(null);
      setPortableStatus(result?.error.message ?? "Portable import preview failed.");
      return;
    }

    setPortableImportPreview(result.data);
    setPortableStatus("Import preview ready.");
  }

  async function importPortableArchive(): Promise<void> {
    if (!portableImportPreview) {
      await previewPortableArchive();
      return;
    }

    const result = await window.hcb?.settings.importPortableArchive({
      path: portableImportPreview.path,
      confirm: true
    });

    if (!result?.ok) {
      setPortableStatus(result?.error.message ?? "Portable import failed.");
      return;
    }

    setPortableStatus(`Imported archive. Backup: ${result.data.backupPath}`);
    setPortableImportPreview(result.data.preview);
  }

  async function repairFirstLocalPointer(): Promise<void> {
    const pointer = localPointers?.items[0]?.pointer;

    if (!pointer || !pointerReplacementPath.trim()) {
      setPortableStatus("Select a missing pointer and replacement path.");
      return;
    }

    const result = await window.hcb?.settings.repairLocalPointer({
      pointer,
      replacementPath: pointerReplacementPath.trim(),
      confirm: true
    });

    if (!result?.ok) {
      setPortableStatus(result?.error.message ?? "Local pointer repair failed.");
      return;
    }

    setPortableStatus(`Repaired ${result.data.updated} local pointer reference${result.data.updated === 1 ? "" : "s"}.`);
    setPointerReplacementPath("");
    void refreshLocalPointers();
  }

  const editingTaskTemplate =
    settings.taskTemplates.find((template) => template.id === editingTaskTemplateId) ?? null;
  const editingEventTemplate =
    settings.eventTemplates.find((template) => template.id === editingEventTemplateId) ?? null;

  return (
    <div className="grid gap-5">
      {editingTaskTemplate ? (
        <TaskTemplateDialog
          onClose={() => setEditingTaskTemplateId(null)}
          onRemove={() => removeTaskTemplate(editingTaskTemplate.id)}
          onUpdate={(patch) => updateTaskTemplate(editingTaskTemplate.id, patch)}
          taskLists={taskLists}
          template={editingTaskTemplate}
        />
      ) : null}
      {editingEventTemplate ? (
        <EventTemplateDialog
          calendars={calendarSources}
          onClose={() => setEditingEventTemplateId(null)}
          onRemove={() => removeEventTemplate(editingEventTemplate.id)}
          onUpdate={(patch) => updateEventTemplate(editingEventTemplate.id, patch)}
          template={editingEventTemplate}
        />
      ) : null}
      <SettingsGroup title="Calendars">
        <SettingsSwitch
          checked={settings.showCompletedInCalendarViews}
          icon={CalendarDays}
          label="Show completed tasks and events in calendar views"
          onChange={(checked) => updateSettings({ showCompletedInCalendarViews: checked })}
        />
        <SettingsControlRow
          description="Default scope when marking a repeating event complete or open again."
          icon={CalendarDays}
          label="Event completion scope"
        >
          <select
            aria-label="Event completion scope"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({
                eventCompletionDefaultScope: event.target.value as SettingsSnapshot["eventCompletionDefaultScope"]
              })
            }
            value={settings.eventCompletionDefaultScope}
          >
            <option value="occurrence">This occurrence</option>
            <option value="seriesFuture">Future series</option>
            <option value="seriesAll">Whole series</option>
            <option value="ask">Ask each time</option>
          </select>
        </SettingsControlRow>
        {calendarSources.length === 0 ? (
          <EmptyState description="No calendars are available yet." title="No calendars" />
        ) : calendarSources.map((calendar) => (
          <SettingsSwitch
            checked={selectedCalendars.size === 0 ? calendar.selected : selectedCalendars.has(calendar.id)}
            key={calendar.id}
            label={calendar.title}
            onChange={(checked) => updateSelectedCalendar(calendar.id, checked)}
            trailing={<Badge>{calendar.eventCount ?? 0}</Badge>}
          />
        ))}
      </SettingsGroup>

      <SettingsGroup title="Task lists">
        {taskLists.length === 0 ? (
          <EmptyState description="No Google Tasks lists are available yet." title="No task lists" />
        ) : taskLists.map((taskList) => (
          <SettingsSwitch
            checked={selectedTaskLists.size === 0 || selectedTaskLists.has(taskList.id)}
            icon={ListChecks}
            key={taskList.id}
            label={taskList.title}
            onChange={(checked) => updateSelectedTaskList(taskList.id, checked)}
            trailing={<Badge>{taskList.activeTaskCount ?? taskList.taskCount ?? 0}</Badge>}
          />
        ))}
      </SettingsGroup>

      <SettingsGroup title="Per-tab list filters">
        <SettingsControlRow
          description="Each tab can hide lists independently of the global Task Lists selection."
          icon={Filter}
          label="Tasks tab"
        >
          <Button
            onClick={() =>
              updatePerTabFilter("tasks", {
                useCustomFilter: !settings.perTabListFilters.tasks.useCustomFilter
              })
            }
            variant={settings.perTabListFilters.tasks.useCustomFilter ? "primary" : "secondary"}
          >
            Use custom filter
          </Button>
        </SettingsControlRow>
        {settings.perTabListFilters.tasks.useCustomFilter ? taskLists.map((taskList) => (
          <SettingsSwitch
            checked={settings.perTabListFilters.tasks.selectedTaskListIds.includes(taskList.id)}
            key={`tasks-${taskList.id}`}
            label={taskList.title}
            onChange={(checked) => togglePerTabList("tasks", taskList.id, checked)}
          />
        )) : null}
        <SettingsControlRow icon={ChevronRight} label="Notes tab">
          <Button
            onClick={() =>
              updatePerTabFilter("notes", {
                useCustomFilter: !settings.perTabListFilters.notes.useCustomFilter
              })
            }
            variant={settings.perTabListFilters.notes.useCustomFilter ? "primary" : "secondary"}
          >
            Use custom filter
          </Button>
        </SettingsControlRow>
        {settings.perTabListFilters.notes.useCustomFilter ? taskLists.map((taskList) => (
          <SettingsSwitch
            checked={settings.perTabListFilters.notes.selectedTaskListIds.includes(taskList.id)}
            key={`notes-${taskList.id}`}
            label={taskList.title}
            onChange={(checked) => togglePerTabList("notes", taskList.id, checked)}
          />
        )) : null}
      </SettingsGroup>

      <SettingsGroup title="Data control">
        <SettingsSwitch
          checked={settings.syncTasksEnabled}
          icon={Database}
          label="Tasks and notes"
          description="Google Tasks lists, tasks, notes, and queued task writes."
          onChange={(checked) => updateSettings({ syncTasksEnabled: checked })}
        />
        <SettingsSwitch
          checked={settings.syncCalendarEventsEnabled}
          icon={CalendarDays}
          label="Calendar events"
          description="Google Calendar lists, events, and queued event writes."
          onChange={(checked) => updateSettings({ syncCalendarEventsEnabled: checked })}
        />
      </SettingsGroup>

      <SettingsGroup title="Customization">
        <SettingsControlRow
          description={customizationStatus ? `Config: ${customizationStatus.configDirectory}` : "File-backed customization status."}
          icon={Code2}
          label="External files"
        >
          <Button onClick={() => void reloadCustomization()} variant="secondary">
            <RotateCcw aria-hidden="true" size={14} />
            Reload
          </Button>
        </SettingsControlRow>
        {customizationStatus ? (
          <>
            <SettingsControlRow
              description={customizationStatus.externalSettings.error ?? customizationStatus.settingsJsonPath}
              label="settings.json"
            >
              <Badge tone={customizationStatus.externalSettings.valid ? "success" : "danger"}>
                {customizationStatus.externalSettings.exists ? `${customizationStatus.externalSettings.appliedKeys.length} keys` : "missing"}
              </Badge>
            </SettingsControlRow>
            <SettingsControlRow
              description={customizationStatus.externalKeymap.error ?? customizationStatus.keymapJsonPath}
              label="keymap.json"
            >
              <Badge tone={customizationStatus.externalKeymap.valid ? "success" : "danger"}>
                {customizationStatus.externalKeymap.exists ? `${customizationStatus.externalKeymap.appliedKeys.length} keys` : "missing"}
              </Badge>
            </SettingsControlRow>
            <SettingsControlRow
              description={customizationStatus.snippetsDirectory}
              icon={Code2}
              label="CSS snippets"
            >
              <Badge>{customizationStatus.snippets.length}</Badge>
            </SettingsControlRow>
            {customizationStatus.snippets.length === 0 ? (
              <EmptyState description="Drop .css files into the snippets folder and reload." title="No snippets" />
            ) : customizationStatus.snippets.map((snippet) => (
              <SettingsSwitch
                checked={snippet.enabled}
                description={snippet.error ?? `${snippet.sizeBytes} bytes`}
                key={snippet.id}
                label={snippet.fileName}
                onChange={(checked) => void setSnippetEnabled(snippet.id, checked)}
              />
            ))}
            <SettingsControlRow
              description={customizationStatus.extensionsDirectory}
              icon={Puzzle}
              label="Sandboxed extensions"
            >
              <Badge>{customizationStatus.extensions.length}</Badge>
            </SettingsControlRow>
            {customizationStatus.extensions.length === 0 ? (
              <EmptyState description="Add extension folders with manifest.json and main.js." title="No extensions" />
            ) : customizationStatus.extensions.map((extension) => (
              <SettingsSwitch
                checked={extension.enabled}
                description={extension.error ?? `${extension.version}; ${extension.capabilities.join(", ") || "no capabilities"}`}
                icon={Puzzle}
                key={extension.id}
                label={extension.name}
                onChange={(checked) => void setExtensionEnabled(extension.id, checked)}
                trailing={<Badge tone={extension.error ? "danger" : extension.enabled ? "success" : "neutral"}>{extension.enabled ? "enabled" : "off"}</Badge>}
              />
            ))}
            {customizationStatus.safeMode ? (
              <div className="rounded-hcbMd border border-warning bg-surface-0 px-3 py-2 text-[var(--text-sm)] text-warning">
                Safe mode is active. Snippets and extensions are not loaded.
              </div>
            ) : null}
          </>
        ) : null}
        {customizationMessage ? (
          <div className="rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-sm)] text-text-secondary">
            {customizationMessage}
          </div>
        ) : null}
      </SettingsGroup>

      <SettingsGroup title="ICS import and subscriptions">
        <SettingsControlRow
          description="Imports .ics files into a read-only local calendar."
          icon={FileUp}
          label="ICS file import"
        >
          <label className="inline-flex h-8 cursor-pointer items-center gap-2 rounded-hcbMd border border-border bg-surface-0 px-3 text-[var(--text-sm)] font-semibold text-text-primary">
            <FileUp aria-hidden="true" size={14} />
            Import .ics
            <input
              accept=".ics,text/calendar"
              className="sr-only"
              onChange={(event) => {
                const file = event.currentTarget.files?.[0];
                event.currentTarget.value = "";
                if (file) {
                  void importIcsFile(file);
                }
              }}
              type="file"
            />
          </label>
        </SettingsControlRow>
        <SettingsControlRow
          description="Subscriptions use https/webcal, cache ETag/Last-Modified, and remain read-only."
          icon={CalendarDays}
          label="Subscribe"
        >
          <div className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,12rem)_auto]">
            <Input
              aria-label="ICS subscription URL"
              onChange={(event) => setIcsUrl(event.currentTarget.value)}
              placeholder="webcal://example.com/calendar.ics"
              value={icsUrl}
            />
            <Input
              aria-label="ICS subscription title"
              onChange={(event) => setIcsTitle(event.currentTarget.value)}
              placeholder="Calendar name"
              value={icsTitle}
            />
            <Button onClick={() => void subscribeIcs()} variant="secondary">
              <CalendarDays aria-hidden="true" size={14} />
              Subscribe
            </Button>
          </div>
        </SettingsControlRow>
        {(icsSubscriptions?.items ?? []).map((subscription) => (
          <SettingsControlRow
            description={subscription.lastError ?? `Last success: ${subscription.lastSuccessAt ?? "never"}`}
            icon={CalendarDays}
            key={subscription.id}
            label={subscription.title}
          >
            <div className="flex flex-wrap items-center gap-2">
              <Badge>{subscription.eventCount}</Badge>
              <Button onClick={() => void refreshIcsSubscription(subscription.id)} size="sm" variant="secondary">
                <RotateCcw aria-hidden="true" size={14} />
                Refresh
              </Button>
              <Button onClick={() => void deleteIcsSubscription(subscription.id)} size="sm" variant="secondary">
                <Trash2 aria-hidden="true" size={14} />
                Remove
              </Button>
            </div>
          </SettingsControlRow>
        ))}
        {icsMessage ? (
          <div className="rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-sm)] text-text-secondary">
            {icsMessage}
          </div>
        ) : null}
      </SettingsGroup>

      <SettingsGroup title="Local reports">
        <SettingsControlRow
          description="Exports local task and calendar summaries without syncing."
          icon={FileDown}
          label="Today"
        >
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => void exportLocalReport("today", "markdown")} size="sm" variant="secondary">Markdown</Button>
            <Button onClick={() => void exportLocalReport("today", "csv")} size="sm" variant="secondary">CSV</Button>
            <Button onClick={() => void exportLocalReport("today", "ics")} size="sm" variant="secondary">ICS</Button>
          </div>
        </SettingsControlRow>
        <SettingsControlRow icon={FileDown} label="Week">
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => void exportLocalReport("week", "markdown")} size="sm" variant="secondary">Markdown</Button>
            <Button onClick={() => void exportLocalReport("week", "csv")} size="sm" variant="secondary">CSV</Button>
            <Button onClick={() => void exportLocalReport("week", "ics")} size="sm" variant="secondary">ICS</Button>
          </div>
        </SettingsControlRow>
        {reportMessage ? (
          <div className="rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-sm)] text-text-secondary">
            {reportMessage}
          </div>
        ) : null}
      </SettingsGroup>

      <SettingsGroup title="Agent and semantic search">
        <SettingsSwitch
          checked={settings.semanticSearchEnabled}
          icon={Filter}
          label="Semantic search"
          description={settings.semanticSearchEnabled ? `Local index enabled: ${settings.embeddingModelId}.` : "Off; search uses the existing lexical path."}
          onChange={(checked) => updateSettings({ semanticSearchEnabled: checked })}
        />
        <SettingsControlRow label="Search mode">
          <select
            aria-label="Semantic search mode"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({ semanticSearchMode: event.currentTarget.value as SettingsSnapshot["semanticSearchMode"] })
            }
            value={settings.semanticSearchMode}
          >
            <option value="lexical">Lexical</option>
            <option value="semantic">Semantic</option>
            <option value="hybrid">Hybrid</option>
          </select>
        </SettingsControlRow>
        <SettingsControlRow
          description="Models stay local. Semantic search can be disabled entirely."
          label="Local model"
        >
          <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
            <select
              aria-label="Embedding model"
              className={settingsSelectClass}
              onChange={(event) => updateSettings({ embeddingModelId: event.currentTarget.value })}
              value={settings.embeddingModelId}
            >
              {settings.semanticSearchModels.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.label} {model.installed ? "installed" : "not installed"}
                </option>
              ))}
            </select>
            <Button onClick={() => void rebuildSemanticIndex()} size="sm" variant="secondary">
              <RotateCcw aria-hidden="true" size={14} />
              Rebuild
            </Button>
          </div>
        </SettingsControlRow>
        {settings.semanticSearchModels.map((model) => (
          <SettingsControlRow
            description={`${model.provider}; ${model.dimensions}d${model.lastError ? `; ${model.lastError}` : ""}`}
            key={model.id}
            label={model.label}
          >
            <div className="flex flex-wrap gap-2">
              <Badge tone={model.installed ? "success" : "neutral"}>{model.installState}</Badge>
              {model.installed && model.id !== "hcb-local-hash-384" ? (
                <Button onClick={() => void uninstallSemanticModel(model.id)} size="sm" variant="secondary">
                  <Trash2 aria-hidden="true" size={14} />
                  Uninstall
                </Button>
              ) : !model.installed ? (
                <Button onClick={() => void installSemanticModel(model.id)} size="sm" variant="secondary">
                  <FileDown aria-hidden="true" size={14} />
                  Install
                </Button>
              ) : null}
            </div>
          </SettingsControlRow>
        ))}
        {semanticStatus ? (
          <div className="rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-sm)] text-text-secondary">
            {semanticStatus}
          </div>
        ) : null}
        <SettingsSwitch
          checked={settings.agentActionTrayEnabled}
          icon={Layers3}
          label="Pending agent action tray"
          description="Shows MCP/CLI proposed writes before approval."
          onChange={(checked) => updateSettings({ agentActionTrayEnabled: checked })}
        />
        <SettingsSwitch
          checked={settings.webhooksEnabled}
          icon={Database}
          label="Loopback webhooks"
          description="Delivers enabled localhost webhook subscriptions."
          onChange={(checked) => updateSettings({ webhooksEnabled: checked })}
        />
      </SettingsGroup>

      <SettingsGroup title="Portable export">
        <SettingsControlRow
          description="Exports settings and planner data into a folder package."
          icon={Archive}
          label="Portable archive"
        >
          <Button onClick={() => void exportPortableArchive()} variant="secondary">
            <FileDown aria-hidden="true" size={14} />
            Export portable archive
          </Button>
        </SettingsControlRow>
        <SettingsSwitch
          checked={settings.portableExportOnlySelectedTaskLists}
          label="Only selected task lists"
          onChange={(checked) => updateSettings({ portableExportOnlySelectedTaskLists: checked })}
        />
        <SettingsSwitch
          checked={settings.portableExportOnlySelectedCalendars}
          label="Only selected calendars"
          onChange={(checked) => updateSettings({ portableExportOnlySelectedCalendars: checked })}
        />
        <SettingsSwitch
          checked={settings.portableExportOnlyFutureCurrentEvents}
          label="Only future/current events"
          onChange={(checked) => updateSettings({ portableExportOnlyFutureCurrentEvents: checked })}
        />
        <SettingsControlRow
          description={portableStatus ?? undefined}
          label="Import portable archive"
        >
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Input
              aria-label="Portable archive path"
              className="min-w-64"
              onChange={(event) => {
                setPortableArchivePath(event.currentTarget.value);
                setPortableImportPreview(null);
              }}
              placeholder="/path/to/archive.hcbexport"
              value={portableArchivePath}
            />
            <Button onClick={() => void previewPortableArchive()} variant="secondary">
              <Upload aria-hidden="true" size={14} />
              Preview import
            </Button>
            <Button
              disabled={!portableImportPreview}
              onClick={() => void importPortableArchive()}
              variant="danger"
            >
              <Upload aria-hidden="true" size={14} />
              Import
            </Button>
          </div>
        </SettingsControlRow>
        {portableImportPreview ? (
          <SettingsControlRow label="Import preview">
            <div className="flex flex-wrap gap-2 text-[var(--text-sm)] text-text-secondary">
              <Badge>Tasks {portableImportPreview.tasks.added}/{portableImportPreview.tasks.changed}/{portableImportPreview.tasks.removed}</Badge>
              <Badge>Events {portableImportPreview.events.added}/{portableImportPreview.events.changed}/{portableImportPreview.events.removed}</Badge>
              <Badge>Calendars {portableImportPreview.calendars.added}/{portableImportPreview.calendars.changed}/{portableImportPreview.calendars.removed}</Badge>
              <Badge>Task lists {portableImportPreview.taskLists.added}/{portableImportPreview.taskLists.changed}/{portableImportPreview.taskLists.removed}</Badge>
              <Badge>Queued {portableImportPreview.queuedMutationCount}</Badge>
              <Badge>Attachments {portableImportPreview.attachments.bundled}</Badge>
              <Badge>Missing pointers {portableImportPreview.attachments.skipped + portableImportPreview.attachments.missing}</Badge>
            </div>
          </SettingsControlRow>
        ) : null}
        {portableImportPreview?.items ? (
          <SettingsControlRow label="Changed items">
            <div className="grid max-h-36 min-w-0 gap-1 overflow-auto text-[var(--text-sm)] text-text-secondary">
              {[...portableImportPreview.items.tasks, ...portableImportPreview.items.events]
                .slice(0, 12)
                .map((item) => (
                  <div className="truncate" key={`${item.change}:${item.id}`}>
                    {item.change}: {item.title}
                  </div>
                ))}
            </div>
          </SettingsControlRow>
        ) : null}
        <SettingsControlRow
          description={localPointers ? `${localPointers.totalKnown} missing local pointer${localPointers.totalKnown === 1 ? "" : "s"}.` : "Scan cached task and event text for missing local files."}
          icon={Link2}
          label="Local pointer repair"
        >
          <div className="grid min-w-0 gap-2">
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => void refreshLocalPointers()} variant="secondary">
                <RotateCcw aria-hidden="true" size={14} />
                Scan pointers
              </Button>
              <Input
                aria-label="Replacement local path"
                className="min-w-64"
                onChange={(event) => setPointerReplacementPath(event.currentTarget.value)}
                placeholder="/path/to/replacement-file"
                value={pointerReplacementPath}
              />
              <Button disabled={!localPointers?.items[0] || !pointerReplacementPath.trim()} onClick={() => void repairFirstLocalPointer()} variant="secondary">
                Repair first
              </Button>
            </div>
            {localPointers?.items[0] ? (
              <div className="truncate text-[var(--text-sm)] text-text-secondary">
                {localPointers.items[0].title}: {localPointers.items[0].pointer}
              </div>
            ) : null}
          </div>
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="Local backups">
        <SettingsSwitch
          checked={settings.dailyLocalBackupEnabled}
          label="Daily local backup"
          onChange={(checked) => updateSettings({ dailyLocalBackupEnabled: checked })}
        />
        <SettingsControlRow label="Keep backups">
          <Input
            aria-label="Keep backups"
            className="w-24"
            max={365}
            min={1}
            onChange={(event) =>
              updateSettings({ localBackupRetentionCount: Number(event.currentTarget.value) || 14 })
            }
            type="number"
            value={settings.localBackupRetentionCount}
          />
        </SettingsControlRow>
        <SettingsControlRow
          description={settings.lastLocalBackupAt ? `Last backup ${settings.lastLocalBackupAt}` : "No backups yet"}
          label="Manual backup"
        >
          <Button onClick={() => beginRecoveryAction("backupNow")} variant="secondary">
            <Archive aria-hidden="true" size={14} />
            Back up now
          </Button>
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="History">
        <SettingsControlRow icon={History} label="Visible entries">
          <input
            aria-label="Visible history entries"
            className="w-56 accent-[var(--color-accent)]"
            max={500}
            min={10}
            onChange={(event) => updateSettings({ visibleHistoryEntryCount: Number(event.target.value) })}
            step={10}
            type="range"
            value={settings.visibleHistoryEntryCount}
          />
        </SettingsControlRow>
        <SettingsControlRow label="Storage cap">
          <input
            aria-label="History storage cap"
            className="w-56 accent-[var(--color-accent)]"
            max={50_000}
            min={100}
            onChange={(event) => updateSettings({ historyStorageCap: Number(event.target.value) })}
            step={100}
            type="range"
            value={settings.historyStorageCap}
          />
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="History categories">
        {(Object.keys(defaultHistoryCategoryVisibility) as Array<keyof SettingsSnapshot["historyCategoryVisibility"]>).map((category) => (
          <SettingsSwitch
            checked={settings.historyCategoryVisibility[category] ?? defaultHistoryCategoryVisibility[category]}
            key={category}
            label={historyCategoryLabels[category]}
            onChange={(checked) =>
              updateSettings({
                historyCategoryVisibility: {
                  ...settings.historyCategoryVisibility,
                  [category]: checked
                }
              })
            }
          />
        ))}
      </SettingsGroup>

      <SettingsGroup title="Duplicate detection">
        <SettingsControlRow
          description={`${settings.dismissedDuplicateGroupIds.length} duplicate group dismissal${settings.dismissedDuplicateGroupIds.length === 1 ? "" : "s"} saved.`}
          icon={Layers3}
          label="Dismissed duplicate groups"
        >
          <Button onClick={() => beginRecoveryAction("resetDuplicateDismissals")} variant="secondary">
            <RotateCcw aria-hidden="true" size={14} />
            Reset dismissals
          </Button>
        </SettingsControlRow>
      </SettingsGroup>

      <SettingsGroup title="Custom filters">
        <SettingsControlRow
          description="Save reusable local search queries for tasks, calendar, notes, and tags."
          icon={Filter}
          label="Saved filters"
        >
          <Button onClick={addSavedFilter} variant="secondary">
            <FilePlus2 aria-hidden="true" size={14} />
            New Filter
          </Button>
        </SettingsControlRow>
        {settings.savedSearchViews.length === 0 ? (
          <EmptyState description="No custom filters yet." title="No filters" />
        ) : settings.savedSearchViews.map((filter) => (
          <div className="grid gap-2 border-b border-border px-3 py-3 last:border-b-0" key={filter.id}>
            <div className="grid gap-2 lg:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_auto_auto]">
              <Input
                aria-label={`Saved filter name ${filter.name}`}
                onChange={(event) => updateSavedFilter(filter.id, { name: event.currentTarget.value })}
                value={filter.name}
              />
              <Input
                aria-label={`Saved filter query ${filter.name}`}
                onChange={(event) => updateSavedFilter(filter.id, { query: event.currentTarget.value })}
                value={filter.query}
              />
              <Button
                onClick={() => togglePinnedSavedFilter(filter.id)}
                size="sm"
                variant={settings.pinnedSavedSearchViewIds.includes(filter.id) ? "primary" : "secondary"}
              >
                <Pin aria-hidden="true" size={14} />
                {settings.pinnedSavedSearchViewIds.includes(filter.id) ? "Pinned" : "Pin"}
              </Button>
              <Button onClick={() => removeSavedFilter(filter.id)} size="sm" variant="ghost">
                <Trash2 aria-hidden="true" size={14} />
                Remove
              </Button>
            </div>
          </div>
        ))}
      </SettingsGroup>

      <SettingsGroup title="Tags">
        <SettingsControlRow
          description={`${tags.length} tag${tags.length === 1 ? "" : "s"} in the local catalog.`}
          icon={Tag}
          label="Tag catalog"
        >
          <div className="grid gap-2 sm:grid-cols-[minmax(0,12rem)_3rem_auto]">
            <Input
              aria-label="New tag name"
              onChange={(event) => setNewTagName(event.currentTarget.value)}
              placeholder="tag"
              value={newTagName}
            />
            <input
              aria-label="New tag color"
              className="h-8 w-12 rounded-hcbMd border border-border bg-surface-0 p-1"
              onChange={(event) => setNewTagColor(event.currentTarget.value)}
              type="color"
              value={newTagColor}
            />
            <Button disabled={!newTagName.trim()} onClick={() => void addTag()} variant="secondary">
              <FilePlus2 aria-hidden="true" size={14} />
              Add
            </Button>
          </div>
        </SettingsControlRow>
        {tagAnalytics ? (
          <SettingsControlRow
            description={`${tagAnalytics.linkedEntities} entity links across ${tagAnalytics.totalTags} tags.`}
            label="Tag analytics"
          >
            <div className="flex flex-wrap gap-2 text-[var(--text-sm)] text-text-secondary">
              <Badge>Unused {tagAnalytics.unusedTags}</Badge>
              {tagAnalytics.topTags.slice(0, 5).map((tag) => (
                <Badge key={tag.id}>{tag.name} {tag.totalCount}</Badge>
              ))}
            </div>
          </SettingsControlRow>
        ) : null}
        {tags.length > 1 ? (
          <SettingsControlRow description="Moves entity links from the source tag into the target tag." label="Merge tags">
            <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
              <select
                aria-label="Merge source tag"
                className={settingsSelectClass}
                onChange={(event) => setTagMergeSourceId(event.target.value)}
                value={tagMergeSourceId}
              >
                <option value="">Source</option>
                {tags.map((tag) => (
                  <option key={tag.id} value={tag.id}>{tag.name}</option>
                ))}
              </select>
              <select
                aria-label="Merge target tag"
                className={settingsSelectClass}
                onChange={(event) => setTagMergeTargetId(event.target.value)}
                value={tagMergeTargetId}
              >
                <option value="">Target</option>
                {tags.map((tag) => (
                  <option key={tag.id} value={tag.id}>{tag.name}</option>
                ))}
              </select>
              <Button
                disabled={!tagMergeSourceId || !tagMergeTargetId || tagMergeSourceId === tagMergeTargetId}
                onClick={() => void mergeSelectedTags()}
                variant="secondary"
              >
                Merge
              </Button>
            </div>
          </SettingsControlRow>
        ) : null}
        {tags.length === 0 ? (
          <EmptyState description="No tags have been created or backfilled yet." title="No tags" />
        ) : tags.map((tag) => {
          const draft = tagDraft(tag);
          const unchanged = draft.name.trim() === tag.name && (draft.color || null) === tag.color;

          return (
            <div className="grid gap-2 border-b border-border px-3 py-3 last:border-b-0" key={tag.id}>
              <div className="grid gap-2 lg:grid-cols-[minmax(0,12rem)_3rem_auto_auto]">
                <Input
                  aria-label={`Tag name ${tag.name}`}
                  onChange={(event) => setTagDraft(tag, { name: event.currentTarget.value })}
                  value={draft.name}
                />
                <input
                  aria-label={`Tag color ${tag.name}`}
                  className="h-8 w-12 rounded-hcbMd border border-border bg-surface-0 p-1"
                  onChange={(event) => setTagDraft(tag, { color: event.currentTarget.value })}
                  type="color"
                  value={draft.color}
                />
                <Button
                  disabled={unchanged || !draft.name.trim()}
                  onClick={() => void saveTag(tag)}
                  size="sm"
                  variant="secondary"
                >
                  <Save aria-hidden="true" size={14} />
                  Save
                </Button>
                <Button onClick={() => void removeTag(tag)} size="sm" variant="ghost">
                  <Trash2 aria-hidden="true" size={14} />
                  Delete
                </Button>
              </div>
              <div className="flex flex-wrap gap-1 text-[var(--text-xs)] text-text-muted">
                <Badge>tasks {tag.taskCount}</Badge>
                <Badge>events {tag.eventCount}</Badge>
                <Badge>notes {tag.noteCount}</Badge>
                <Badge tone="accent">total {tag.totalCount}</Badge>
              </div>
            </div>
          );
        })}
      </SettingsGroup>

      <SettingsGroup title="Auto tags">
        <SettingsControlRow
          description="Apply local HCB tags and optional event colors when titles or bodies match."
          icon={Tag}
          label="Rules"
        >
          <Button onClick={addAutoTagRule} variant="secondary">
            <FilePlus2 aria-hidden="true" size={14} />
            New Rule
          </Button>
        </SettingsControlRow>
        <SettingsControlRow
          description="Reapply enabled rules to currently loaded planner data."
          icon={RotateCcw}
          label="Bulk reapply"
        >
          <div className="flex flex-wrap gap-2">
            {autoTagTargetKinds.map((kind) => (
              <Button
                disabled={autoTagBulkCounts[kind] === 0 || autoTagErrors.length > 0}
                key={kind}
                onClick={() => onReapplyAutoTags(kind)}
                size="sm"
                variant="secondary"
              >
                {kind} <Badge>{autoTagBulkCounts[kind]}</Badge>
              </Button>
            ))}
          </div>
        </SettingsControlRow>
        <SettingsControlRow
          description="Runs after saved rule changes."
          icon={RotateCcw}
          label="Background reapply"
        >
          <select
            aria-label="Auto-tag background reapply mode"
            className={settingsSelectClass}
            onChange={(event) =>
              updateSettings({
                autoTagBackgroundReapplyMode: event.currentTarget.value as SettingsSnapshot["autoTagBackgroundReapplyMode"]
              })
            }
            value={settings.autoTagBackgroundReapplyMode}
          >
            <option value="manual">Manual only</option>
            <option value="preview">Preview and notify</option>
            <option value="silent">Silent apply</option>
          </select>
        </SettingsControlRow>
        {autoTagErrors.length > 0 || autoTagWarnings.length > 0 ? (
          <div className="grid gap-1 border-b border-border px-3 py-2 text-[var(--text-sm)]">
            {autoTagErrors.length > 0 ? (
              <p className="font-medium text-danger" role="alert">
                {autoTagErrors.length} auto-tag rule {autoTagErrors.length === 1 ? "error" : "errors"} need review.
              </p>
            ) : null}
            {autoTagWarnings.length > 0 ? (
              <p className="text-warning">
                {autoTagWarnings.length} auto-tag rule {autoTagWarnings.length === 1 ? "warning" : "warnings"}.
              </p>
            ) : null}
          </div>
        ) : null}
        <div className="grid gap-3 border-b border-border px-3 py-3">
          <div>
            <div className="text-[var(--text-base)] font-medium text-text-primary">Rule preview</div>
            <p className="mt-0.5 text-[var(--text-sm)] text-text-muted">
              Test title/body text against enabled rules in order before saving new CRUD items.
            </p>
          </div>
          <div className="grid gap-2 lg:grid-cols-[10rem_minmax(0,1fr)]">
            <select
              aria-label="Auto tag preview target"
              className={settingsSelectClass}
              onChange={(event) => setAutoTagPreviewKind(event.target.value as AutoTagTargetKind)}
              value={autoTagPreviewKind}
            >
              {autoTagTargetKinds.map((kind) => (
                <option key={kind} value={kind}>
                  {kind}
                </option>
              ))}
            </select>
            <Input
              aria-label="Auto tag preview title"
              onChange={(event) => setAutoTagPreviewTitle(event.currentTarget.value)}
              value={autoTagPreviewTitle}
            />
          </div>
          <textarea
            aria-label="Auto tag preview body"
            className="min-h-16 rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            onChange={(event) => setAutoTagPreviewBody(event.currentTarget.value)}
            placeholder="Body preview"
            value={autoTagPreviewBody}
          />
          <div className="grid gap-2 lg:grid-cols-3">
            <Input
              aria-label="Auto tag preview existing tags"
              label="Existing tags"
              onChange={(event) => setAutoTagPreviewExistingTags(event.currentTarget.value)}
              placeholder="ops, coding"
              value={autoTagPreviewExistingTags}
            />
            <Input
              aria-label="Auto tag preview explicit tags"
              label="Explicit tags"
              onChange={(event) => setAutoTagPreviewExplicitTags(event.currentTarget.value)}
              placeholder="manual, launch"
              value={autoTagPreviewExplicitTags}
            />
            <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
              <span>Local kind</span>
              <select
                aria-label="Auto tag preview local kind"
                className={settingsSelectClass}
                onChange={(event) => setAutoTagPreviewLocalKind(event.target.value as AutoTagPreviewLocalKind)}
                value={autoTagPreviewLocalKind}
              >
                <option value="normal">Normal</option>
                <option value="birthday">Birthday</option>
              </select>
            </label>
          </div>
          <div className="grid gap-2 lg:grid-cols-2">
            <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
              <span>Requested event color</span>
              <select
                aria-label="Auto tag preview requested event color"
                className={settingsSelectClass}
                onChange={(event) => setAutoTagPreviewRequestedColorId(event.target.value)}
                value={autoTagPreviewRequestedColorId}
              >
                <option value="">No requested color</option>
                {googleCalendarEventColors.map((color) => (
                  <option key={color.id} value={color.id}>
                    {color.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
              <span>Existing event color</span>
              <select
                aria-label="Auto tag preview existing event color"
                className={settingsSelectClass}
                onChange={(event) => setAutoTagPreviewExistingColorId(event.target.value)}
                value={autoTagPreviewExistingColorId}
              >
                <option value="">No existing color</option>
                {googleCalendarEventColors.map((color) => (
                  <option key={color.id} value={color.id}>
                    {color.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className="grid gap-2 rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-sm)] text-text-secondary">
            <div className="flex flex-wrap gap-2">
              <Badge tone={autoTagPreview.matchedRuleCount > 0 ? "success" : "neutral"}>
                {autoTagPreview.matchedRuleCount} matched
              </Badge>
              <Badge tone={autoTagPreview.hasConflicts ? "warning" : "neutral"}>
                {autoTagPreview.hasConflicts ? "multiple rules" : "single pass"}
              </Badge>
              {autoTagPreview.eventColorId ? <Badge tone="info">color {autoTagPreview.eventColorId}</Badge> : null}
            </div>
            <div className="truncate text-text-primary">Title: {autoTagPreview.title || "Untitled"}</div>
            <div>Tags: {autoTagPreview.tags.length > 0 ? autoTagPreview.tags.join(", ") : "none"}</div>
            {autoTagPreview.hasConflicts ? (
              <p className="text-warning" role="status">
                Multiple rules match this preview. Later rules apply after earlier rules.
              </p>
            ) : null}
            <ol className="grid gap-1">
              {autoTagPreviewLocalKind === "birthday" ? (
                <li>Rules skipped for birthday preview.</li>
              ) : autoTagPreview.traces.length === 0 ? (
                <li>No rules configured.</li>
              ) : autoTagPreview.traces.map((trace) => (
                <li className="flex min-w-0 flex-wrap items-center gap-2" key={trace.ruleId}>
                  <Badge>#{trace.order}</Badge>
                  <span className="min-w-0 max-w-48 truncate font-medium text-text-primary">{trace.ruleName}</span>
                  <Badge
                    tone={
                      trace.status === "matched"
                        ? "success"
                        : trace.status === "invalid"
                          ? "danger"
                          : trace.status === "no-output"
                            ? "warning"
                            : "neutral"
                    }
                  >
                    {trace.status.replace("-", " ")}
                  </Badge>
                  {trace.matchedField ? <Badge>{trace.matchedField}</Badge> : null}
                  {trace.tagsAdded.length > 0 ? <span>+{trace.tagsAdded.join(", ")}</span> : null}
                  {trace.strippedField ? <Badge tone="info">strip {trace.strippedField}</Badge> : null}
                  {trace.eventColorStatus === "applied" ? <Badge tone="info">color applied</Badge> : null}
                  {trace.eventColorStatus === "skipped-explicit" ? <Badge tone="warning">color kept</Badge> : null}
                  {trace.eventColorStatus === "skipped-existing" ? <Badge tone="warning">existing color kept</Badge> : null}
                </li>
              ))}
            </ol>
          </div>
        </div>
        {settings.autoTagRules.length === 0 ? (
          <EmptyState description="No auto tag rules yet." title="No auto tags" />
        ) : settings.autoTagRules.map((rule, index) => {
          const ruleIssues = validateAutoTagRule(rule);
          const patternIssue = ruleIssues.find((issue) => issue.field === "pattern");
          const outputIssue = ruleIssues.find((issue) => issue.field === "output");
          const trace = autoTagPreview.traces.find((candidate) => candidate.ruleId === rule.id);

          return (
            <div className="grid gap-2 border-b border-border px-3 py-3 last:border-b-0" key={rule.id}>
              <div className="flex flex-wrap items-center gap-2 text-[var(--text-sm)] text-text-secondary">
                <Badge>Rule #{index + 1}</Badge>
                {trace ? (
                  <Badge
                    tone={
                      trace.status === "matched"
                        ? "success"
                        : trace.status === "invalid"
                          ? "danger"
                          : trace.status === "no-output"
                            ? "warning"
                            : "neutral"
                    }
                  >
                    preview {trace.status.replace("-", " ")}
                  </Badge>
                ) : null}
                {patternIssue ? <Badge tone="danger">invalid regex</Badge> : null}
                {outputIssue ? <Badge tone="warning">no output</Badge> : null}
                <div className="ml-auto flex gap-1">
                  <Button
                    aria-label={`Move auto tag rule ${rule.name} up`}
                    disabled={index === 0}
                    onClick={() => moveAutoTagRule(index, index - 1)}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowUp aria-hidden="true" size={14} />
                    Up
                  </Button>
                  <Button
                    aria-label={`Move auto tag rule ${rule.name} down`}
                    disabled={index === settings.autoTagRules.length - 1}
                    onClick={() => moveAutoTagRule(index, index + 1)}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowDown aria-hidden="true" size={14} />
                    Down
                  </Button>
                </div>
              </div>
              <div className="grid gap-2 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1fr)_auto]">
                <Input
                  aria-label={`Auto tag rule name ${rule.name}`}
                  onChange={(event) => updateAutoTagRule(rule.id, { name: event.currentTarget.value || "Auto tag" })}
                  value={rule.name}
                />
                <div className="grid gap-1">
                  <Input
                    aria-label={`Auto tag pattern ${rule.name}`}
                    onChange={(event) => updateAutoTagRule(rule.id, { pattern: event.currentTarget.value || "TODO" })}
                    value={rule.pattern}
                  />
                  {patternIssue ? (
                    <p className="text-[var(--text-sm)] text-danger" role="alert">
                      {patternIssue.message}
                    </p>
                  ) : null}
                </div>
                <Button
                  onClick={() =>
                    saveAutoTagRules(settings.autoTagRules.filter((candidate) => candidate.id !== rule.id))
                  }
                  variant="ghost"
                >
                  Remove
                </Button>
              </div>
              <div className="grid gap-2 sm:grid-cols-3">
                <select
                  aria-label={`Auto tag match field ${rule.name}`}
                  className={settingsSelectClass}
                  onChange={(event) =>
                    updateAutoTagRule(rule.id, { matchField: event.target.value as AutoTagRule["matchField"] })
                  }
                  value={rule.matchField}
                >
                  <option value="title">Title</option>
                  <option value="body">Body</option>
                  <option value="anyText">Title or body</option>
                </select>
                <select
                  aria-label={`Auto tag match type ${rule.name}`}
                  className={settingsSelectClass}
                  onChange={(event) =>
                    updateAutoTagRule(rule.id, { matchType: event.target.value as AutoTagRule["matchType"] })
                  }
                  value={rule.matchType}
                >
                  <option value="prefix">Prefix</option>
                  <option value="contains">Contains</option>
                  <option value="regex">Regex</option>
                </select>
                <select
                  aria-label={`Auto tag event color ${rule.name}`}
                  className={settingsSelectClass}
                  onChange={(event) =>
                    updateAutoTagRule(rule.id, {
                      eventColorId: event.target.value === "" ? null : event.target.value as AutoTagRule["eventColorId"]
                    })
                  }
                  value={rule.eventColorId ?? ""}
                >
                  <option value="">No event color</option>
                  {googleCalendarEventColors.map((color) => (
                    <option key={color.id} value={color.id}>
                      {color.label}
                    </option>
                  ))}
                </select>
              </div>
              <Input
                aria-label={`Auto tag output tags ${rule.name}`}
                label="Tags"
                onChange={(event) => updateAutoTagRule(rule.id, { tags: parseTagText(event.currentTarget.value) })}
                value={rule.tags.join(", ")}
              />
              {outputIssue ? (
                <p className="text-[var(--text-sm)] text-warning">
                  {outputIssue.message}
                </p>
              ) : null}
              <div className="flex flex-wrap gap-4 text-[var(--text-sm)] text-text-secondary">
                <label className="inline-flex min-h-8 items-center gap-2">
                  <input
                    aria-label={`Auto tag enabled ${rule.name}`}
                    checked={rule.enabled && !autoTagRuleHasError(rule)}
                    className="accent-[var(--color-accent)]"
                    onChange={(event) => updateAutoTagRule(rule.id, { enabled: event.target.checked })}
                    type="checkbox"
                  />
                  Enabled
                </label>
                <label className="inline-flex min-h-8 items-center gap-2">
                  <input
                    checked={rule.stripMatchedPrefix}
                    className="accent-[var(--color-accent)]"
                    onChange={(event) => updateAutoTagRule(rule.id, { stripMatchedPrefix: event.target.checked })}
                    type="checkbox"
                  />
                  Strip prefix
                </label>
                <label className="inline-flex min-h-8 items-center gap-2">
                  <input
                    checked={rule.overrideExistingEventColor}
                    className="accent-[var(--color-accent)]"
                    onChange={(event) => updateAutoTagRule(rule.id, { overrideExistingEventColor: event.target.checked })}
                    type="checkbox"
                  />
                  Override event color
                </label>
              </div>
              <div className="flex flex-wrap gap-4 text-[var(--text-sm)] text-text-secondary">
                {autoTagTargetKinds.map((kind) => (
                  <label className="inline-flex min-h-8 items-center gap-2" key={kind}>
                    <input
                      checked={rule.targetKinds.includes(kind)}
                      className="accent-[var(--color-accent)]"
                      onChange={(event) => toggleAutoTagTarget(rule, kind, event.target.checked)}
                      type="checkbox"
                    />
                    {kind}
                  </label>
                ))}
              </div>
            </div>
          );
        })}
      </SettingsGroup>

      <SettingsGroup title="Task templates">
        <SettingsControlRow
          description="Define reusable title, notes, due date, and list fields."
          icon={FilePlus2}
          label="Task blueprint"
        >
          <Button onClick={addTaskTemplate} variant="secondary">
            New Task Template
          </Button>
        </SettingsControlRow>
        {settings.taskTemplates.length === 0 ? (
          <EmptyState description="No custom task templates yet." title="No task templates" />
        ) : settings.taskTemplates.map((template) => (
          <SettingsControlRow key={template.id} label={template.name} description={template.title}>
            <div className="flex items-center gap-2">
              <Button onClick={() => setEditingTaskTemplateId(template.id)} size="sm" variant="secondary">
                Edit
              </Button>
              <Button onClick={() => removeTaskTemplate(template.id)} size="sm" variant="ghost">
                Remove
              </Button>
            </div>
          </SettingsControlRow>
        ))}
      </SettingsGroup>

      <SettingsGroup title="Event templates">
        <SettingsControlRow
          description="Define reusable event title, time, location, attendees, and recurrence fields."
          icon={FilePlus2}
          label="Event blueprint"
        >
          <Button onClick={addEventTemplate} variant="secondary">
            New Event Template
          </Button>
        </SettingsControlRow>
        {settings.eventTemplates.length === 0 ? (
          <EmptyState description="No custom event templates yet." title="No event templates" />
        ) : settings.eventTemplates.map((template) => (
          <SettingsControlRow key={template.id} label={template.name} description={template.title}>
            <div className="flex items-center gap-2">
              <Button onClick={() => setEditingEventTemplateId(template.id)} size="sm" variant="secondary">
                Edit
              </Button>
              <Button onClick={() => removeEventTemplate(template.id)} size="sm" variant="ghost">
                Remove
              </Button>
            </div>
          </SettingsControlRow>
        ))}
      </SettingsGroup>

      <SettingsGroup title="Note templates">
        <SettingsControlRow
          description="Define reusable note title and body fields."
          icon={FilePlus2}
          label="Note blueprint"
        >
          <Button onClick={addNoteTemplate} variant="secondary">
            New Note Template
          </Button>
        </SettingsControlRow>
        {noteTemplates.length === 0 ? (
          <EmptyState description="No custom note templates yet." title="No note templates" />
        ) : noteTemplates.map((template) => (
          <div className="grid gap-2 border-b border-border px-3 py-3 last:border-b-0" key={template.id}>
            <div className="grid gap-2 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)_auto]">
              <Input
                aria-label={`Note template name ${template.name}`}
                onChange={(event) => updateNoteTemplate(template.id, { name: event.currentTarget.value || "Note Template" })}
                value={template.name}
              />
              <Input
                aria-label={`Note template title ${template.name}`}
                onChange={(event) => updateNoteTemplate(template.id, { title: event.currentTarget.value || "Untitled note" })}
                value={template.title}
              />
              <Button
                onClick={() =>
                  updateSettings({
                    noteTemplates: noteTemplates.filter((candidate) => candidate.id !== template.id)
                  })
                }
                variant="ghost"
              >
                Remove
              </Button>
            </div>
            <textarea
              aria-label={`Note template body ${template.name}`}
              className="min-h-24 rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              onChange={(event) => updateNoteTemplate(template.id, { body: event.currentTarget.value })}
              value={template.body}
            />
          </div>
        ))}
      </SettingsGroup>
    </div>
  );
}

function TemplateDialogFrame({
  children,
  onClose,
  title
}: {
  children: ReactNode;
  onClose: () => void;
  title: string;
}): JSX.Element {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4">
      <section
        aria-labelledby="template-dialog-title"
        aria-modal="true"
        className="grid max-h-[85vh] w-full max-w-3xl overflow-hidden rounded-hcbLg border border-border bg-bg-secondary shadow-hcbLg"
        role="dialog"
      >
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
          <h2 className="truncate text-[var(--text-lg)] font-semibold text-text-primary" id="template-dialog-title">
            {title}
          </h2>
          <Button onClick={onClose} size="sm" variant="ghost">
            <X aria-hidden="true" size={14} />
            Close
          </Button>
        </div>
        <div className="max-h-[calc(85vh-4rem)] overflow-auto p-4">
          {children}
        </div>
      </section>
    </div>
  );
}

function TaskTemplateDialog({
  onClose,
  onRemove,
  onUpdate,
  taskLists,
  template
}: {
  onClose: () => void;
  onRemove: () => void;
  onUpdate: (patch: Partial<SettingsSnapshot["taskTemplates"][number]>) => void;
  taskLists: TaskListSummary[];
  template: SettingsSnapshot["taskTemplates"][number];
}): JSX.Element {
  const [draft, setDraft] = useState(template);

  useEffect(() => {
    setDraft(template);
  }, [template]);

  return (
    <TemplateDialogFrame onClose={onClose} title="Edit task template">
      <div className="grid gap-3">
        <Input
          aria-label="Task template name"
          label="Name"
          onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })}
          value={draft.name}
        />
        <Input
          aria-label="Task template title"
          label="Title"
          onChange={(event) => setDraft({ ...draft, title: event.currentTarget.value })}
          value={draft.title}
        />
        <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
          <span>Notes</span>
          <textarea
            aria-label="Task template notes"
            className="min-h-24 rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            onChange={(event) => setDraft({ ...draft, notes: event.currentTarget.value || null })}
            value={draft.notes ?? ""}
          />
        </label>
        <Input
          aria-label="Task template due expression"
          label="Due expression"
          onChange={(event) => setDraft({ ...draft, dueExpression: event.currentTarget.value || null })}
          value={draft.dueExpression ?? ""}
        />
        <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
          <span>Task list</span>
          <select
            aria-label="Task template list"
            className={settingsSelectClass}
            onChange={(event) => setDraft({ ...draft, listId: event.currentTarget.value || null })}
            value={draft.listId ?? ""}
          >
            <option value="">Default list</option>
            {taskLists.map((taskList) => (
              <option key={taskList.id} value={taskList.id}>
                {taskList.title}
              </option>
            ))}
          </select>
        </label>
        <div className="flex justify-between gap-2 pt-2">
          <Button onClick={onRemove} variant="danger">
            <Trash2 aria-hidden="true" size={14} />
            Delete template
          </Button>
          <Button
            onClick={() => {
              onUpdate({
                dueExpression: draft.dueExpression,
                listId: draft.listId,
                name: draft.name.trim() || "Task Template",
                notes: draft.notes,
                title: draft.title.trim() || "Untitled task"
              });
              onClose();
            }}
            variant="primary"
          >
            Done
          </Button>
        </div>
      </div>
    </TemplateDialogFrame>
  );
}

function EventTemplateDialog({
  calendars,
  onClose,
  onRemove,
  onUpdate,
  template
}: {
  calendars: CalendarListSummary[];
  onClose: () => void;
  onRemove: () => void;
  onUpdate: (patch: Partial<SettingsSnapshot["eventTemplates"][number]>) => void;
  template: SettingsSnapshot["eventTemplates"][number];
}): JSX.Element {
  const [draft, setDraft] = useState(template);
  const [attendeeText, setAttendeeText] = useState(template.attendeeEmails.join(", "));

  useEffect(() => {
    setDraft(template);
    setAttendeeText(template.attendeeEmails.join(", "));
  }, [template]);

  return (
    <TemplateDialogFrame onClose={onClose} title="Edit event template">
      <div className="grid gap-3">
        <Input
          aria-label="Event template name"
          label="Name"
          onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })}
          value={draft.name}
        />
        <Input
          aria-label="Event template title"
          label="Title"
          onChange={(event) => setDraft({ ...draft, title: event.currentTarget.value })}
          value={draft.title}
        />
        <div className="grid gap-3 sm:grid-cols-2">
          <Input
            aria-label="Event template start expression"
            label="Start expression"
            onChange={(event) => setDraft({ ...draft, startExpression: event.currentTarget.value || null })}
            value={draft.startExpression ?? ""}
          />
          <Input
            aria-label="Event template end expression"
            label="End expression"
            onChange={(event) => setDraft({ ...draft, endExpression: event.currentTarget.value || null })}
            value={draft.endExpression ?? ""}
          />
        </div>
        <Input
          aria-label="Event template location"
          label="Location"
          onChange={(event) => setDraft({ ...draft, location: event.currentTarget.value || null })}
          value={draft.location ?? ""}
        />
        <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
          <span>Calendar</span>
          <select
            aria-label="Event template calendar"
            className={settingsSelectClass}
            onChange={(event) => setDraft({ ...draft, calendarId: event.currentTarget.value || null })}
            value={draft.calendarId ?? ""}
          >
            <option value="">Default calendar</option>
            {calendars.map((calendar) => (
              <option key={calendar.id} value={calendar.id}>
                {calendar.title}
              </option>
            ))}
          </select>
        </label>
        <Input
          aria-label="Event template attendees"
          label="Attendees"
          onChange={(event) => setAttendeeText(event.currentTarget.value)}
          value={attendeeText}
        />
        <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
          <span>Notes</span>
          <textarea
            aria-label="Event template notes"
            className="min-h-24 rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            onChange={(event) => setDraft({ ...draft, notes: event.currentTarget.value || null })}
            value={draft.notes ?? ""}
          />
        </label>
        <div className="flex justify-between gap-2 pt-2">
          <Button onClick={onRemove} variant="danger">
            <Trash2 aria-hidden="true" size={14} />
            Delete template
          </Button>
          <Button
            onClick={() => {
              onUpdate({
                attendeeEmails: parseTemplateEmails(attendeeText),
                calendarId: draft.calendarId,
                endExpression: draft.endExpression,
                location: draft.location,
                name: draft.name.trim() || "Event Template",
                notes: draft.notes,
                startExpression: draft.startExpression,
                title: draft.title.trim() || "Untitled event"
              });
              onClose();
            }}
            variant="primary"
          >
            Done
          </Button>
        </div>
      </div>
    </TemplateDialogFrame>
  );
}

function parseTemplateEmails(value: string): string[] {
  return value
    .split(/[,\s]+/)
    .map((email) => email.trim())
    .filter((email) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))
    .slice(0, 50);
}
