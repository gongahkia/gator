import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState
} from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { AlertTriangle, Link2, Pencil, RotateCcw, Search } from "lucide-react";
import type { NoteLinkSuggestResponse } from "@shared/ipc/contracts";
import type { AutoTagRule } from "@shared/ipc/contracts";
import { useDirtyState, useInspector } from "../../../components/Inspector";
import { EmojiInput, EmojiTextarea } from "../../../components/EmojiTextField";
import { Badge, Button, Input, cx } from "../../../components/primitives";
import { AutoTagAudit } from "../AutoTagAudit";
import { AttachmentPanel } from "../AttachmentPanel";
import { EntityLinksPanel } from "../EntityLinksPanel";
import type { useCoreViewModelSource } from "../coreViewModelSource";
import type { NoteViewModel } from "../coreViewModels";
import { MarkdownPreview } from "../MarkdownPreview";
import { plannerLinkTargets } from "../plannerLinkTargets";
import { TagBadges, TagInput } from "../TagInput";
import {
  extractNoteLinks,
  extractNoteProperties,
  extractPlannerLinks,
  normalizedNoteTitle,
  parsePlannerLink,
  plannerLinkDisplayLabel
} from "../notesParsing";

export interface NoteDraftValue {
  title: string;
  body: string;
  tags: string[];
}

export interface NoteInspectorBodyHandle {
  flush: () => Promise<void>;
  getDraft: () => NoteDraftValue;
}

export interface NoteTemplateOption {
  body: string;
  id: string;
  name: string;
  title: string;
}

type LinkSuggestion = NoteLinkSuggestResponse["items"][number];

interface NoteInspectorBodyProps {
  createMode?: boolean;
  error?: string;
  note: NoteViewModel;
  notes: NoteViewModel[];
  onDraftChange: (noteId: string, draft: NoteDraftValue) => void;
  onOpenNote: (noteId: string) => Promise<void> | void;
  onPersist: (noteId: string, draft: NoteDraftValue) => Promise<boolean>;
  rules: readonly AutoTagRule[];
  source: ReturnType<typeof useCoreViewModelSource>;
  templates?: NoteTemplateOption[];
}

function noteDraftValuesEqual(left: NoteDraftValue, right: NoteDraftValue): boolean {
  return left.title === right.title &&
    left.body === right.body &&
    JSON.stringify(left.tags) === JSON.stringify(right.tags);
}

export function NoteInspectorSummary({
  note,
  notes,
  onOpenNote,
  source
}: {
  note: NoteViewModel;
  notes: NoteViewModel[];
  onOpenNote: (noteId: string) => Promise<void> | void;
  source: ReturnType<typeof useCoreViewModelSource>;
}): JSX.Element {
  const noteByNormalizedTitle = useMemo(
    () => new Map(notes.map((candidate) => [normalizedNoteTitle(candidate.title), candidate])),
    [notes]
  );
  const links = useMemo(() => extractPlannerLinks(note.body), [note.body]);
  const properties = useMemo(() => extractNoteProperties(note.body), [note.body]);
  const backlinks = useMemo(
    () =>
      notes.filter(
        (candidate) =>
          candidate.id !== note.id &&
          extractNoteLinks(candidate.body).some(
            (title) => normalizedNoteTitle(title) === normalizedNoteTitle(note.title)
          )
      ),
    [note.body, note.id, note.title, notes]
  );

  return (
    <div className="grid gap-5 py-1">
      <div className="grid grid-cols-[24px_minmax(0,1fr)] gap-4">
        <span aria-hidden="true" className="mt-2 size-3.5 rounded-hcbSm bg-info" />
        <div className="min-w-0">
          <h3 className="min-w-0 break-words text-[var(--text-2xl)] font-semibold leading-tight text-text-primary">
            {note.title || "Untitled note"}
          </h3>
          <p className="mt-2 text-[var(--text-sm)] text-text-muted">{note.updatedLabel}</p>
          {note.preview ? (
            <p className="mt-2 text-[var(--text-base)] leading-relaxed text-text-secondary">{note.preview}</p>
          ) : null}
          <div className="mt-2">
            <TagBadges tags={note.tags} />
          </div>
        </div>
      </div>

      {note.body.trim() ? (
        <MarkdownPreview ariaLabel="Note preview" body={note.body} plannerLinkTargets={plannerLinkTargets(source)} />
      ) : null}

      <AttachmentPanel entityId={note.id} entityKind="note" />

      {properties.length > 0 ? (
        <div className="grid gap-2">
          <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">Properties</div>
          <div className="flex flex-wrap gap-2">
            {properties.map((property) => (
              <Badge key={`${property.key}-${property.value}`} tone="info">
                {property.key}: {property.value}
              </Badge>
            ))}
          </div>
        </div>
      ) : null}

      {links.length > 0 ? (
        <div className="grid gap-2">
          <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">Links</div>
          <div className="flex flex-wrap gap-2">
            {links.map((link) => {
              const linkedNote = link.kind === "note"
                ? noteByNormalizedTitle.get(normalizedNoteTitle(link.label))
                : undefined;
              const action = linkedNote ? () => void onOpenNote(linkedNote.id) : () => undefined;

              return (
                <button
                  aria-label={linkedNote ? `Open linked note ${linkedNote.title}` : `${link.kind} link ${link.label}`}
                  className={linkButtonClass(linkedNote ? "accent" : "neutral")}
                  key={`${link.type}-${link.raw}`}
                  onClick={action}
                  type="button"
                >
                  <Search aria-hidden="true" size={14} />
                  {linkedNote ? linkedNote.title : `${link.kind}: ${plannerLinkDisplayLabel(link)}`}
                </button>
              );
            })}
          </div>
        </div>
      ) : null}

      {backlinks.length > 0 ? (
        <div className="grid gap-2">
          <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">Backlinks</div>
          <div className="flex flex-wrap gap-2">
            {backlinks.map((backlink) => (
              <button
                aria-label={`Open backlink ${backlink.title}`}
                className={linkButtonClass("accent")}
                key={backlink.id}
                onClick={() => void onOpenNote(backlink.id)}
                type="button"
              >
                <RotateCcw aria-hidden="true" size={14} />
                {backlink.title}
              </button>
            ))}
          </div>
        </div>
      ) : null}

      <EntityLinksPanel entityId={note.id} entityKind="note" />
    </div>
  );
}

function linkButtonClass(tone: "accent" | "warning" | "neutral"): string {
  return cx(
    "inline-flex min-h-8 items-center gap-1 rounded-hcbSm border px-2 text-[var(--text-sm)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
    tone === "accent" && "border-accent bg-bg-tertiary text-accent",
    tone === "warning" && "border-warning bg-bg-tertiary text-warning",
    tone === "neutral" && "border-border bg-bg-tertiary text-text-secondary"
  );
}

export const NoteInspectorBody = forwardRef<NoteInspectorBodyHandle, NoteInspectorBodyProps>(
  function NoteInspectorBody(
    { createMode = false, error, note, notes, onDraftChange, onOpenNote, onPersist, rules, source, templates = [] },
    ref
  ): JSX.Element {
    const dirty = useDirtyState<NoteDraftValue>({ title: note.title, body: note.body, tags: note.tags ?? [] });
    const { update } = useInspector();
    const [viewMode, setViewMode] = useState<"edit" | "preview">("edit");
    const [selectedTemplateId, setSelectedTemplateId] = useState("blank");
    const [linkQuery, setLinkQuery] = useState("");
    const [suggestions, setSuggestions] = useState<LinkSuggestion[]>([]);
    const [activeSuggestionIndex, setActiveSuggestionIndex] = useState(0);
    const [repairLinkText, setRepairLinkText] = useState<string | null>(null);
    const [brokenLinks, setBrokenLinks] = useState<Array<{ linkText: string }>>([]);
    const textareaRef = useRef<HTMLTextAreaElement | null>(null);
    const linkInputRef = useRef<HTMLInputElement | null>(null);
    const draftRef = useRef<NoteDraftValue>(dirty.value);
    const dirtyRef = useRef(dirty.isDirty);
    const lastNotifiedDraftRef = useRef<NoteDraftValue>(dirty.value);
    const onDraftChangeRef = useRef(onDraftChange);
    const flushRef = useRef<() => Promise<void>>(async () => undefined);
    const debounceRef = useRef<number | null>(null);
    const noteByNormalizedTitle = useMemo(
      () => new Map(notes.map((candidate) => [normalizedNoteTitle(candidate.title), candidate])),
      [notes]
    );
    const links = useMemo(() => extractNoteLinks(dirty.value.body), [dirty.value.body]);
    const properties = useMemo(() => extractNoteProperties(dirty.value.body), [dirty.value.body]);
    const backlinks = useMemo(
      () =>
        notes.filter(
          (candidate) =>
            candidate.id !== note.id &&
            extractNoteLinks(candidate.body).some(
              (title) => normalizedNoteTitle(title) === normalizedNoteTitle(dirty.value.title)
            )
        ),
      [dirty.value.title, note.id, notes]
    );
    const brokenLinkTexts = useMemo(
      () => new Set(brokenLinks.map((item) => item.linkText)),
      [brokenLinks]
    );

    useEffect(() => {
      if (note.id.startsWith("note-draft-")) {
        setBrokenLinks([]);
        return;
      }

      let cancelled = false;

      void window.hcb?.notes.listBrokenLinks({ noteId: note.id }).then((result) => {
        if (cancelled || !result?.ok) {
          return;
        }

        setBrokenLinks(result.data.items);
      });

      return () => {
        cancelled = true;
      };
    }, [dirty.value.body, note.id]);

    useEffect(() => {
      const query = linkQuery.trim();

      if (!query) {
        setSuggestions([]);
        setActiveSuggestionIndex(0);
        return;
      }

      let cancelled = false;
      const timer = window.setTimeout(() => {
        void window.hcb?.notes.linkSuggest({ query, limit: 8 }).then((result) => {
          if (cancelled || !result?.ok) {
            return;
          }

          setSuggestions(result.data.items);
          setActiveSuggestionIndex(0);
        });
      }, 180);

      return () => {
        cancelled = true;
        window.clearTimeout(timer);
      };
    }, [linkQuery]);

    useEffect(() => {
      onDraftChangeRef.current = onDraftChange;
    }, [onDraftChange]);

    useEffect(() => {
      if (createMode) {
        setSelectedTemplateId("blank");
      }
    }, [createMode, note.id]);

    useEffect(() => {
      const next = { title: note.title, body: note.body, tags: note.tags ?? [] };

      if (!dirtyRef.current && !noteDraftValuesEqual(draftRef.current, next)) {
        lastNotifiedDraftRef.current = next;
        dirty.reset(next);
      }
    }, [dirty.reset, note.body, note.id, note.tags, note.title]);

    useEffect(() => {
      draftRef.current = dirty.value;
      dirtyRef.current = dirty.isDirty;

      if (!noteDraftValuesEqual(lastNotifiedDraftRef.current, dirty.value)) {
        lastNotifiedDraftRef.current = dirty.value;
        onDraftChangeRef.current(note.id, dirty.value);
      }

      update({
        dirty: dirty.isDirty,
        subtitle: note.updatedLabel,
        title: dirty.value.title || "Untitled note"
      });
    }, [dirty.isDirty, dirty.value, note.id, note.updatedLabel, update]);

    const flush = useCallback(async (): Promise<void> => {
      if (debounceRef.current !== null) {
        window.clearTimeout(debounceRef.current);
        debounceRef.current = null;
      }

      if (!dirtyRef.current || note.id.startsWith("note-draft-")) {
        return;
      }

      const draft = draftRef.current;
      const saved = await onPersist(note.id, draft);

      if (saved) {
        dirty.reset(draft);
      }
    }, [dirty.reset, note.id, onPersist]);

    useEffect(() => {
      flushRef.current = flush;
    }, [flush]);

    useEffect(() => {
      if (!dirty.isDirty || note.id.startsWith("note-draft-")) {
        return;
      }

      if (debounceRef.current !== null) {
        window.clearTimeout(debounceRef.current);
      }

      debounceRef.current = window.setTimeout(() => {
        debounceRef.current = null;
        void flushRef.current();
      }, 250);

      return () => {
        if (debounceRef.current !== null) {
          window.clearTimeout(debounceRef.current);
          debounceRef.current = null;
        }
      };
    }, [dirty.isDirty, dirty.value, note.id]);

    useEffect(
      () => () => {
        void flushRef.current();
      },
      []
    );

    useImperativeHandle(
      ref,
      () => ({
        flush,
        getDraft: () => draftRef.current
      }),
      [flush]
    );

    function patchDraft(partial: Partial<NoteDraftValue>): void {
      dirty.patch(partial);
    }

    function applyTemplate(templateId: string): void {
      const template = templates.find((candidate) => candidate.id === templateId);

      if (!template) {
        return;
      }

      setSelectedTemplateId(templateId);
      patchDraft({ title: template.title, body: template.body });
    }

    function markerForSuggestion(suggestion: LinkSuggestion): string {
      return `[[${suggestion.kind}:${suggestion.label}]]`;
    }

    function insertMarkerAtCursor(marker: string): void {
      const textarea = textareaRef.current;
      const body = dirty.value.body;
      const start = textarea?.selectionStart ?? body.length;
      const end = textarea?.selectionEnd ?? start;
      const nextBody = `${body.slice(0, start)}${marker}${body.slice(end)}`;
      patchDraft({ body: nextBody });
      setLinkQuery("");
      setSuggestions([]);

      queueMicrotask(() => {
        const nextCursor = start + marker.length;
        textareaRef.current?.focus();
        textareaRef.current?.setSelectionRange(nextCursor, nextCursor);
      });
    }

    function repairLink(marker: string): void {
      if (!repairLinkText) {
        insertMarkerAtCursor(marker);
        return;
      }

      const escaped = repairLinkText.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const pattern = new RegExp(`\\[\\[${escaped}\\]\\]`);
      patchDraft({ body: dirty.value.body.replace(pattern, marker) });
      setRepairLinkText(null);
      setLinkQuery("");
      setSuggestions([]);
      textareaRef.current?.focus();
    }

    function chooseSuggestion(suggestion: LinkSuggestion): void {
      const marker = markerForSuggestion(suggestion);

      if (repairLinkText) {
        repairLink(marker);
        return;
      }

      insertMarkerAtCursor(marker);
    }

    function openRepair(linkText: string): void {
      const parsed = parsePlannerLink(linkText);
      setRepairLinkText(linkText);
      setLinkQuery(parsed.label);
      setActiveSuggestionIndex(0);
      queueMicrotask(() => linkInputRef.current?.focus());
    }

    function handleSuggestionKeyDown(event: ReactKeyboardEvent<HTMLInputElement>): void {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setActiveSuggestionIndex((current) =>
          suggestions.length === 0 ? 0 : (current + 1) % suggestions.length
        );
        return;
      }

      if (event.key === "ArrowUp") {
        event.preventDefault();
        setActiveSuggestionIndex((current) =>
          suggestions.length === 0 ? 0 : (current - 1 + suggestions.length) % suggestions.length
        );
        return;
      }

      if (event.key === "Enter" && suggestions[activeSuggestionIndex]) {
        event.preventDefault();
        chooseSuggestion(suggestions[activeSuggestionIndex]);
        return;
      }

      if (event.key === "Escape") {
        event.preventDefault();
        setSuggestions([]);
        setRepairLinkText(null);
      }
    }

    function handleLinkChipKeyDown(
      event: ReactKeyboardEvent<HTMLButtonElement>,
      action: () => void
    ): void {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        action();
      }
    }

    return (
      <div className="grid gap-4">
        <div className="grid gap-3">
          <EmojiInput
            aria-label="Note title"
            onValueChange={(title) => patchDraft({ title })}
            value={dirty.value.title}
          />

          {createMode && templates.length > 0 ? (
            <label className="grid gap-1 text-[var(--text-sm)] font-medium text-text-secondary">
              Template
              <select
                aria-label="Note template"
                className="h-9 rounded-hcbMd border border-border bg-surface-0 px-3 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                onChange={(event) => applyTemplate(event.currentTarget.value)}
                value={selectedTemplateId}
              >
                {templates.map((template) => (
                  <option key={template.id} value={template.id}>
                    {template.name}
                  </option>
                ))}
              </select>
            </label>
          ) : null}

          <TagInput onChange={(tags) => patchDraft({ tags })} value={dirty.value.tags} />
          <AutoTagAudit
            input={{
              kind: "note",
              title: dirty.value.title,
              body: dirty.value.body,
              existingTags: dirty.value.tags
            }}
            rules={rules}
          />

          {error ? (
            <div
              className="flex items-start gap-2 rounded-hcbMd border border-danger/40 bg-danger/10 px-3 py-2 text-[var(--text-sm)] text-danger"
              role="alert"
            >
              <AlertTriangle aria-hidden="true" className="mt-0.5 shrink-0" size={15} />
              <span>{error}</span>
            </div>
          ) : null}

          <div className="flex items-center justify-between gap-2">
            {!createMode ? (
              <div className="flex items-center gap-2" role="tablist" aria-label="Note view mode">
                <Button
                  aria-selected={viewMode === "edit"}
                  onClick={() => setViewMode("edit")}
                  role="tab"
                  size="sm"
                  variant={viewMode === "edit" ? "secondary" : "ghost"}
                >
                  <Pencil aria-hidden="true" size={14} />
                  Edit
                </Button>
                <Button
                  aria-selected={viewMode === "preview"}
                  onClick={() => setViewMode("preview")}
                  role="tab"
                  size="sm"
                  variant={viewMode === "preview" ? "secondary" : "ghost"}
                >
                  <Search aria-hidden="true" size={14} />
                  Preview
                </Button>
              </div>
            ) : (
              <span />
            )}
            <Badge tone={dirty.isDirty ? "warning" : "success"}>
              {dirty.isDirty ? "Saving" : "Saved"}
            </Badge>
          </div>

          {createMode || viewMode === "edit" ? (
            <EmojiTextarea
              aria-label="Note body"
              className="min-h-[260px] w-full resize-none rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary placeholder:text-text-muted transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              onValueChange={(body) => patchDraft({ body })}
              ref={textareaRef}
              value={dirty.value.body}
            />
          ) : (
            <MarkdownPreview
              ariaLabel="Note preview"
              body={dirty.value.body}
              plannerLinkTargets={plannerLinkTargets(source)}
            />
          )}
        </div>

        {!createMode && viewMode === "edit" ? (
          <AttachmentPanel editable entityId={note.id} entityKind="note" />
        ) : null}

        <div className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
          <div className="flex items-center gap-2 text-[var(--text-sm)] font-medium text-text-primary">
            <Link2 aria-hidden="true" size={15} />
            {repairLinkText ? `Fix ${repairLinkText}` : "Insert link"}
          </div>
          <div className="relative grid gap-2">
            <Input
              aria-autocomplete="list"
              aria-controls="note-link-suggestions"
              aria-expanded={suggestions.length > 0}
              aria-label="Planner link target"
              onChange={(event) => setLinkQuery(event.target.value)}
              onKeyDown={handleSuggestionKeyDown}
              placeholder="Search notes, tasks, events"
              ref={linkInputRef}
              role="combobox"
              value={linkQuery}
            />
            {suggestions.length > 0 ? (
              <div
                className="absolute left-0 right-0 top-9 z-10 grid max-h-48 overflow-auto rounded-hcbMd border border-border bg-surface-0 p-1 shadow-hcbMd"
                id="note-link-suggestions"
                role="listbox"
              >
                {suggestions.map((suggestion, index) => (
                  <button
                    aria-selected={index === activeSuggestionIndex}
                    className={cx(
                      "flex min-h-8 items-center justify-between gap-2 rounded-hcbSm px-2 text-left text-[var(--text-sm)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                      index === activeSuggestionIndex ? "bg-bg-tertiary text-text-primary" : "text-text-secondary hover:bg-bg-tertiary"
                    )}
                    key={`${suggestion.kind}-${suggestion.id}`}
                    onClick={() => chooseSuggestion(suggestion)}
                    onMouseEnter={() => setActiveSuggestionIndex(index)}
                    role="option"
                    type="button"
                  >
                    <span className="truncate">{suggestion.label}</span>
                    <Badge tone={suggestion.kind === "note" ? "info" : suggestion.kind === "task" ? "success" : "accent"}>
                      {suggestion.kind}
                    </Badge>
                  </button>
                ))}
              </div>
            ) : null}
            {repairLinkText ? (
              <Button
                onClick={() => {
                  setRepairLinkText(null);
                  setLinkQuery("");
                  setSuggestions([]);
                }}
                size="sm"
                variant="ghost"
              >
                Cancel repair
              </Button>
            ) : null}
          </div>
        </div>

        <div className="grid gap-2">
          <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">Properties</div>
          {properties.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {properties.map((property) => (
                <Badge key={`${property.key}-${property.value}`} tone="info">
                  {property.key}: {property.value}
                </Badge>
              ))}
            </div>
          ) : (
            <span className="text-[var(--text-sm)] text-text-muted">No properties</span>
          )}
        </div>

        <div className="grid gap-3 rounded-hcbMd border border-border bg-bg-tertiary p-3">
          <div className="grid gap-2">
            <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">Links</div>
            {links.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {links.map((title) => {
                  const link = parsePlannerLink(title);
                  const linkedNote = link.kind === "note"
                    ? noteByNormalizedTitle.get(normalizedNoteTitle(link.label))
                    : undefined;
                  const isBroken = brokenLinkTexts.has(title) || (link.kind === "note" && !linkedNote);
                  const label = linkedNote?.title ?? link.label;
                  const action = linkedNote ? () => void onOpenNote(linkedNote.id) : () => undefined;

                  return (
                    <button
                      aria-label={linkedNote ? `Open linked note ${linkedNote.title}` : `${link.kind} link ${link.label}`}
                      className={linkButtonClass(linkedNote ? "accent" : isBroken ? "warning" : "neutral")}
                      key={title}
                      onClick={action}
                      onKeyDown={(event) => handleLinkChipKeyDown(event, action)}
                      type="button"
                    >
                      <Search aria-hidden="true" size={14} />
                      {linkedNote ? label : `${isBroken ? "Broken" : link.kind}: ${label}`}
                    </button>
                  );
                })}
              </div>
            ) : (
              <span className="text-[var(--text-sm)] text-text-muted">None</span>
            )}
          </div>

          {brokenLinks.length > 0 ? (
            <div className="grid gap-2">
              <div className="text-[var(--text-xs)] font-semibold uppercase text-warning">Broken links</div>
              <div className="flex flex-wrap gap-2">
                {brokenLinks.map((item) => (
                  <span
                    className="inline-flex min-h-8 items-center gap-2 rounded-hcbSm border border-warning bg-bg-tertiary px-2 text-[var(--text-sm)] text-warning"
                    key={item.linkText}
                  >
                    {item.linkText}
                    <button
                      aria-label={`Fix link ${item.linkText}`}
                      className="rounded-hcbSm px-1 text-warning underline-offset-2 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                      onClick={() => openRepair(item.linkText)}
                      type="button"
                    >
                      Fix link
                    </button>
                  </span>
                ))}
              </div>
            </div>
          ) : null}

          <div className="grid gap-2">
            <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">Backlinks</div>
            {backlinks.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {backlinks.map((backlink) => {
                  const action = () => void onOpenNote(backlink.id);

                  return (
                    <button
                      aria-label={`Open backlink ${backlink.title}`}
                      className={linkButtonClass("accent")}
                      key={backlink.id}
                      onClick={action}
                      onKeyDown={(event) => handleLinkChipKeyDown(event, action)}
                      type="button"
                    >
                      <RotateCcw aria-hidden="true" size={14} />
                      {backlink.title}
                    </button>
                  );
                })}
              </div>
            ) : (
              <span className="text-[var(--text-sm)] text-text-muted">None</span>
            )}
          </div>
        </div>
      </div>
    );
  }
);
