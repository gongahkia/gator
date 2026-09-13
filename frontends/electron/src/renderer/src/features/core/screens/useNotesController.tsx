import { useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { ArrowRightLeft, Copy, Pencil, Save, Trash2, X } from "lucide-react";
import type { TaskListSummary, TaskSummary } from "@shared/ipc/contracts";
import { useInspector } from "../../../components/Inspector";
import { Button } from "../../../components/primitives";
import {
  conversionCleanup,
  dispatchConvertCommand,
  type ConvertSourceCleanup
} from "../conversionEvents";
import { copiedTitle } from "../copyLabels";
import type { CoreViewModelSource } from "../coreViewModelSource";
import type { NoteViewModel } from "../coreViewModels";
import { dateInputValue, scheduleRendererFrame } from "../coreScreenShared";
import {
  readLocalStorageNumberRecord,
  readLocalStorageStringArray,
  writeLocalStorageJSON
} from "../localStorageHelpers";
import {
  NoteInspectorBody,
  NoteInspectorSummary,
  type NoteDraftValue,
  type NoteInspectorBodyHandle,
  type NoteTemplateOption
} from "../inspectors/NoteInspectorBody";
import { buildNotePreview } from "../notesParsing";
import { rendererNow, reportRendererTimingSince } from "../../../hooks/useRenderTiming";
import type { NoteBoardSelection, NoteViewColumn } from "./notesTypes";

const starredNotesStorageKey = "hcb.starredNoteIds";
const starredNotesAtStorageKey = "hcb.starredNoteAt";
const defaultNoteListTitle = "Notes";

function noteListSelection(listId: string): NoteBoardSelection {
  return `list:${listId}`;
}

function initialNoteViews(noteLists: CoreViewModelSource["noteLists"]): NoteBoardSelection[] {
  return noteLists.map((list) => noteListSelection(list.id));
}

function displayNote(note: NoteViewModel): NoteViewModel {
  return note;
}

function noteFromTask(
  task: TaskSummary,
  noteLists: CoreViewModelSource["noteLists"],
  updatedLabel = "Just now"
): NoteViewModel {
  const body = task.notes ?? "";
  const listTitle = noteLists.find((list) => list.id === task.listId)?.title ?? defaultNoteListTitle;

  return {
    id: task.id,
    listId: task.listId,
    listTitle,
    title: task.title,
    body,
    preview: buildNotePreview(body),
    tags: task.tags ?? [],
    updatedLabel
  };
}

function noteListFromTaskList(list: TaskListSummary): CoreViewModelSource["noteLists"][number] {
  return {
    id: list.id,
    title: list.title,
    updatedAt: list.updatedAt,
    noteCount: 0
  };
}

export function useNotesController(source: CoreViewModelSource): {
  createNote: (listId?: string) => Promise<void>;
  createNoteList: () => Promise<void>;
  deleteNoteList: (listId: string, title: string) => Promise<void>;
  deleteNote: (noteId: string) => Promise<void>;
  duplicateNote: (noteId: string) => Promise<void>;
  noteViewColumns: NoteViewColumn[];
  noteLists: CoreViewModelSource["noteLists"];
  notes: NoteViewModel[];
  selectedNoteId: string | null;
  selectedNoteViews: NoteBoardSelection[];
  selectNote: (noteId: string, mode?: "view" | "edit") => Promise<void>;
  starredNoteIds: ReadonlySet<string>;
  moveNoteToList: (noteId: string, listId: string) => Promise<void>;
  renameNoteList: (listId: string, currentTitle: string) => Promise<void>;
  toggleNoteStar: (noteId: string) => void;
  toggleNoteView: (view: NoteBoardSelection) => void;
} {
  const {
    close: closeInspector,
    current: currentInspector,
    open: openInspector,
    update: updateInspector
  } = useInspector();
  const [notes, setNotes] = useState<NoteViewModel[]>(() => source.initialNotes.map(displayNote));
  const [localNoteLists, setLocalNoteLists] = useState(source.noteLists);
  const [selectedNoteId, setSelectedNoteId] = useState<string | null>(
    source.initialNotes[0]?.id ?? null
  );
  const [selectedNoteViews, setSelectedNoteViews] = useState<NoteBoardSelection[]>(() =>
    initialNoteViews(source.noteLists)
  );
  const [noteInspectorMode, setNoteInspectorModeState] = useState<"view" | "edit">("edit");
  const [noteActionError, setNoteActionError] = useState<string | undefined>();
  const [starredNoteIds, setStarredNoteIds] = useState<Set<string>>(
    () => new Set(readLocalStorageStringArray(starredNotesStorageKey))
  );
  const [starredNoteAt, setStarredNoteAt] = useState<Record<string, number>>(
    () => readLocalStorageNumberRecord(starredNotesAtStorageKey)
  );
  const [draftCounter, setDraftCounter] = useState(1);
  const createNoteIds = useRef(new Set<string>());
  const conversionCleanupByNoteId = useRef(new Map<string, ConvertSourceCleanup>());
  const lastNoteEditReportAt = useRef(0);
  const noteInspectorBodyRef = useRef<NoteInspectorBodyHandle | null>(null);
  const noteInspectorModeRef = useRef<"view" | "edit">("edit");
  const selectedNote = notes.find((note) => note.id === selectedNoteId) ?? null;
  const noteLists = localNoteLists;
  const noteListSignature = noteLists.map((list) => list.id).join("\n");
  const noteTemplateOptions = useMemo<NoteTemplateOption[]>(() => {
    const today = dateInputValue(new Date().toISOString());

    return [
      { id: "blank", name: "Blank", title: "Untitled note", body: "" },
      {
        id: "daily",
        name: "Daily note",
        title: `Daily ${today}`,
        body: `status: open\ntags: daily\ndate: ${today}\n\n# Daily ${today}\n- [ ] Review calendar\n- [ ] Triage inbox\n`
      },
      {
        id: "meeting",
        name: "Meeting note",
        title: `Meeting ${today}`,
        body: `status: draft\ntags: meeting\ndate: ${today}\n\n# Meeting ${today}\nAttendees:\n\nNotes:\n\nDecisions:\n- \n`
      },
      ...(source.settings.noteTemplates ?? []).map((template) => ({
        id: template.id,
        name: template.name,
        title: template.title,
        body: template.body
      }))
    ];
  }, [source.settings.noteTemplates]);
  const noteViewColumns = useMemo(
    () =>
      selectedNoteViews.map((view) => {
        const listId = view.slice("list:".length);
        const list = noteLists.find((candidate) => candidate.id === listId);
        return {
          id: view,
          listId,
          title: list?.title ?? defaultNoteListTitle,
          description: "Notes in this list",
          emptyDescription: "Drag notes here or create a note in this list.",
          emptyTitle: "No notes in this list",
          notes: notes.filter((note) => note.listId === listId)
        };
      }),
    [noteLists, notes, selectedNoteViews]
  );

  function setNoteInspectorMode(mode: "view" | "edit"): void {
    noteInspectorModeRef.current = mode;
    setNoteInspectorModeState(mode);
  }

  useEffect(() => {
    writeLocalStorageJSON(starredNotesStorageKey, [...starredNoteIds]);
  }, [starredNoteIds]);

  useEffect(() => {
    writeLocalStorageJSON(starredNotesAtStorageKey, starredNoteAt);
  }, [starredNoteAt]);

  useEffect(() => {
    setNotes(source.initialNotes.map(displayNote));
    setSelectedNoteId((current) =>
      current && source.initialNotes.some((note) => note.id === current)
        ? current
        : source.initialNotes[0]?.id ?? null
    );
  }, [source.initialNotes]);

  useEffect(() => {
    setLocalNoteLists(source.noteLists);
  }, [source.noteLists]);

  useEffect(() => {
    const availableViews = noteLists.map((list) => noteListSelection(list.id));
    const available = new Set<NoteBoardSelection>(availableViews);

    setSelectedNoteViews((current) => {
      const next = current.filter((view) => available.has(view));
      return next.length > 0 ? next : availableViews;
    });
  }, [noteListSignature]);

  useEffect(() => {
    function handleNoteCommand(event: Event): void {
      const detail = (event as CustomEvent<{
        action: string;
        body?: string;
        cleanup?: ConvertSourceCleanup;
        draft?: {
          body: string;
          id?: string;
          listId?: string;
          listTitle?: string;
          replaceSource?: boolean;
          title: string;
        };
        listId?: string;
        noteId?: string;
        title?: string;
      }>).detail;

      if (detail?.action === "new-note") {
        if (detail.title !== undefined || detail.body !== undefined || detail.listId !== undefined) {
          void createNoteWithTemplate(
            detail.title?.trim() || "Untitled note",
            detail.body ?? "",
            detail.listId
          );
          return;
        }

        void createNote();
      }

      if (detail?.action === "open-note" && detail.noteId) {
        void selectNote(detail.noteId, "view");
      }

      if (detail?.action === "convert-to-note" && detail.draft) {
        void openConvertedNoteDraft(detail.draft, detail.cleanup);
      }
    }

    window.addEventListener("hcb:note-command", handleNoteCommand);
    return () => window.removeEventListener("hcb:note-command", handleNoteCommand);
  });

  useEffect(() => {
    if (currentInspector?.kind !== "note" || !selectedNote || currentInspector.id !== selectedNote.id) {
      return;
    }

    updateInspector({
      actions: noteInspectorActions(selectedNote),
      body: noteInspectorBody(selectedNote),
      dirty: noteInspectorMode === "view" ? false : currentInspector.dirty,
      hideHeader: noteInspectorMode === "view",
      subtitle: selectedNote.updatedLabel,
      title: selectedNote.title || "Untitled note"
    });
  }, [
    currentInspector?.id,
    currentInspector?.dirty,
    currentInspector?.kind,
    noteTemplateOptions,
    notes,
    noteActionError,
    noteInspectorMode,
    selectedNote,
    updateInspector
  ]);

  function noteInspectorBody(
    note: NoteViewModel,
    mode = noteInspectorModeRef.current,
    error = noteActionError
  ): ReactNode {
    if (mode === "view") {
      return (
        <NoteInspectorSummary
          key={`view-${note.id}`}
          note={note}
          notes={notes}
          onOpenNote={(noteId) => selectNote(noteId, "view")}
          source={source}
        />
      );
    }

    return (
      <NoteInspectorBody
        key={note.id}
        note={note}
        notes={notes}
        onDraftChange={updateNoteDraft}
        onOpenNote={(noteId) => selectNote(noteId, "view")}
        onPersist={persistNoteDraft}
        rules={source.settings.autoTagRules}
        source={source}
        templates={noteTemplateOptions}
        createMode={createNoteIds.current.has(note.id)}
        error={error}
        ref={noteInspectorBodyRef}
      />
    );
  }

  function noteInspectorActions(note: NoteViewModel, mode = noteInspectorModeRef.current): ReactNode {
    const localDraft = note.id.startsWith("note-draft-");

    if (mode === "view") {
      return (
        <div className="flex w-full items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Button onClick={() => void deleteNote(note.id)} size="sm" variant="danger">
              <Trash2 aria-hidden="true" size={14} />
              Delete selected note
            </Button>
            <Button onClick={() => setNoteInspectorMode("edit")} size="sm" variant="secondary">
              <Pencil aria-hidden="true" size={14} />
              Edit
            </Button>
            <Button onClick={() => void duplicateNote(note.id)} size="sm" variant="secondary">
              <Copy aria-hidden="true" size={14} />
              Duplicate
            </Button>
            {!localDraft ? (
              <>
                <Button onClick={() => convertNote(note, "task")} size="sm" variant="secondary">
                  <ArrowRightLeft aria-hidden="true" size={14} />
                  Convert to task
                </Button>
                <Button onClick={() => convertNote(note, "event")} size="sm" variant="secondary">
                  <ArrowRightLeft aria-hidden="true" size={14} />
                  Convert to event
                </Button>
              </>
            ) : null}
          </div>
          <Button onClick={() => void closeInspector()} size="sm" variant="ghost">
            <X aria-hidden="true" size={14} />
            Close
          </Button>
        </div>
      );
    }

    if (localDraft) {
      return (
        <>
          <Button onClick={() => void deleteNote(note.id)} size="sm" variant="danger">
            <Trash2 aria-hidden="true" size={14} />
            Discard
          </Button>
          <Button onClick={() => void saveLocalNoteDraft(note.id)} size="sm" variant="primary">
            <Save aria-hidden="true" size={14} />
            Save
          </Button>
        </>
      );
    }

    return (
      <>
        <Button onClick={() => void deleteNote(note.id)} size="sm" variant="danger">
          <Trash2 aria-hidden="true" size={14} />
          Delete selected note
        </Button>
        <Button onClick={() => void duplicateNote(note.id)} size="sm" variant="secondary">
          <Copy aria-hidden="true" size={14} />
          Duplicate
        </Button>
        {!localDraft ? (
          <>
            <Button onClick={() => convertNote(note, "task")} size="sm" variant="secondary">
              <ArrowRightLeft aria-hidden="true" size={14} />
              Convert to task
            </Button>
            <Button onClick={() => convertNote(note, "event")} size="sm" variant="secondary">
              <ArrowRightLeft aria-hidden="true" size={14} />
              Convert to event
            </Button>
          </>
        ) : null}
        <Button onClick={() => void closeInspector()} size="sm" variant="ghost">
          <X aria-hidden="true" size={14} />
          Close
        </Button>
      </>
    );
  }

  function openNoteInspector(
    note: NoteViewModel,
    mode: "view" | "edit" = note.id.startsWith("note-draft-") ? "edit" : "view"
  ): void {
    setNoteActionError(undefined);
    setNoteInspectorMode(mode);
    openInspector({
      actions: noteInspectorActions(note, mode),
      body: noteInspectorBody(note, mode),
      dirty: false,
      hideHeader: mode === "view",
      id: note.id,
      kind: "note",
      onConfirmClose: async () => {
        if (noteInspectorModeRef.current === "edit") {
          await noteInspectorBodyRef.current?.flush();
        }
        return true;
      },
      subtitle: note.updatedLabel,
      title: note.title || "Untitled note"
    });
  }

  async function selectNote(noteId: string, mode: "view" | "edit" = "view"): Promise<void> {
    const note = notes.find((candidate) => candidate.id === noteId);

    if (!note) {
      return;
    }

    if (noteInspectorModeRef.current === "edit") {
      await noteInspectorBodyRef.current?.flush();
    }
    setSelectedNoteId(note.id);
    openNoteInspector(note, mode);
  }

  async function createNote(listId?: string): Promise<void> {
    if (noteInspectorModeRef.current === "edit") {
      await noteInspectorBodyRef.current?.flush();
    }

    const list = noteLists.find((candidate) => candidate.id === listId) ?? noteLists[0];
    if (!list) {
      return;
    }

    const fallbackId = `note-draft-${draftCounter}`;
    const fallbackNote: NoteViewModel = {
      id: fallbackId,
      listId: list.id,
      listTitle: list.title,
      title: "Untitled note",
      body: "",
      preview: "Empty note",
      tags: [],
      updatedLabel: "Just now"
    };

    setDraftCounter((current) => current + 1);
    setNotes((current) => [fallbackNote, ...current]);
    setSelectedNoteId(fallbackId);
    createNoteIds.current.add(fallbackId);
    openNoteInspector(fallbackNote, "edit");

    const result = await window.hcb?.tasks.create({
      title: "Untitled note",
      notes: "",
      listId: list.id,
      dueDate: null,
      tags: []
    });

    if (result?.ok) {
      const persisted = noteFromTask(result.data, noteLists);

      setNotes((current) =>
        current.map((note) => (note.id === fallbackId ? persisted : note))
      );
      setSelectedNoteId(result.data.id);
      createNoteIds.current.delete(fallbackId);
      createNoteIds.current.add(result.data.id);
      openNoteInspector(persisted, "edit");
      source.refreshUndoStatus();
    }
  }

  function openLocalNoteDraft(seed: {
    body: string;
    listId: string;
    listTitle: string;
    tags?: string[];
    title: string;
  }): string {
    const fallbackId = `note-draft-${draftCounter}`;
    const fallbackNote: NoteViewModel = {
      id: fallbackId,
      listId: seed.listId,
      listTitle: seed.listTitle,
      title: seed.title,
      body: seed.body,
      preview: buildNotePreview(seed.body),
      tags: seed.tags ?? [],
      updatedLabel: "Just now"
    };

    setDraftCounter((current) => current + 1);
    setNotes((current) => [fallbackNote, ...current]);
    setSelectedNoteId(fallbackId);
    createNoteIds.current.add(fallbackId);
    openNoteInspector(fallbackNote, "edit");

    return fallbackId;
  }

  async function openConvertedNoteDraft(
    seed: {
      body: string;
      id?: string;
      listId?: string;
      listTitle?: string;
      replaceSource?: boolean;
      tags?: string[];
      title: string;
    },
    cleanup?: ConvertSourceCleanup
  ): Promise<void> {
    if (noteInspectorModeRef.current === "edit") {
      await noteInspectorBodyRef.current?.flush();
    }

    const list = noteLists.find((candidate) => candidate.id === seed.listId) ?? noteLists[0];

    if (!list) {
      return;
    }

    if (seed.replaceSource && seed.id) {
      const note: NoteViewModel = {
        id: seed.id,
        listId: list.id,
        listTitle: seed.listTitle ?? list.title,
        title: seed.title,
        body: seed.body,
        preview: buildNotePreview(seed.body),
        tags: seed.tags ?? [],
        updatedLabel: "Edited"
      };

      setNotes((current) => current.some((candidate) => candidate.id === note.id)
        ? current.map((candidate) => candidate.id === note.id ? note : candidate)
        : [note, ...current]
      );
      setSelectedNoteId(note.id);
      createNoteIds.current.delete(note.id);
      if (cleanup) {
        conversionCleanupByNoteId.current.set(note.id, cleanup);
      }
      openNoteInspector(note, "edit");
      void persistNoteDraft(note.id, { title: note.title, body: note.body, tags: note.tags ?? [] });
      return;
    }

    const draftId = openLocalNoteDraft({
      title: seed.title,
      body: seed.body,
      tags: seed.tags ?? [],
      listId: list.id,
      listTitle: list.title
    });

    if (cleanup) {
      conversionCleanupByNoteId.current.set(draftId, cleanup);
    }
  }

  function convertNote(note: NoteViewModel, target: "event" | "task"): void {
    if (note.id.startsWith("note-draft-")) {
      return;
    }

    const liveDraft =
      currentInspector?.kind === "note" && currentInspector.id === note.id
        ? noteInspectorBodyRef.current?.getDraft()
        : null;
    const title = liveDraft?.title ?? note.title;
    const body = liveDraft?.body ?? note.body;
    const tags = liveDraft?.tags ?? note.tags ?? [];

    if (target === "event") {
      dispatchConvertCommand({
        cleanup: conversionCleanup("note", note.id, target),
        target,
        eventDraft: {
          title,
          notes: body,
          tags
        }
      });
      return;
    }

    const replace = window.confirm(
      "Remove the original note after saving the converted task? Cancel keeps the original note."
    );
    const dueDate = dateInputValue(new Date().toISOString());

    dispatchConvertCommand({
      target,
      taskDraft: replace
        ? {
            mode: "edit",
            id: note.id,
            title,
            notes: body,
            dueDate,
            listId: note.listId,
            parentId: "",
            priority: "none",
            plannedStart: null,
            plannedEnd: null,
            durationMinutes: null,
            lockedSchedule: false,
            tags
          }
        : {
            title,
            notes: body,
            dueDate,
            listId: note.listId,
            priority: "none",
            plannedStart: null,
            plannedEnd: null,
            durationMinutes: null,
            lockedSchedule: false,
            tags
          }
    });
  }

  async function cleanupConvertedSource(noteId: string): Promise<string | null> {
    const cleanup = conversionCleanupByNoteId.current.get(noteId);

    if (!cleanup) {
      return null;
    }

    conversionCleanupByNoteId.current.delete(noteId);

    if (cleanup.kind === "event") {
      const result = await window.hcb?.calendar.delete({ id: cleanup.id });
      return result?.ok ? null : result?.error.message ?? "Original event was not removed.";
    }

    const result = await window.hcb?.tasks.delete({ id: cleanup.id });
    return result?.ok ? null : result?.error.message ?? "Original task was not removed.";
  }

  async function createNoteWithTemplate(title: string, body: string, listId?: string): Promise<void> {
    if (noteInspectorModeRef.current === "edit") {
      await noteInspectorBodyRef.current?.flush();
    }

    const list = noteLists.find((candidate) => candidate.id === listId) ?? noteLists[0];
    if (!list) {
      return;
    }

    const fallbackId = `note-draft-${draftCounter}`;
    const fallbackNote: NoteViewModel = {
      id: fallbackId,
      listId: list.id,
      listTitle: list.title,
      title,
      body,
      preview: buildNotePreview(body),
      tags: [],
      updatedLabel: "Just now"
    };

    setDraftCounter((current) => current + 1);
    setNotes((current) => [fallbackNote, ...current]);
    setSelectedNoteId(fallbackId);
    createNoteIds.current.add(fallbackId);
    openNoteInspector(fallbackNote, "edit");

    const result = await window.hcb?.tasks.create({
      title,
      notes: body,
      listId: list.id,
      dueDate: null,
      tags: []
    });

    if (result?.ok) {
      const persisted = noteFromTask(result.data, noteLists);

      setNotes((current) =>
        current.map((note) => (note.id === fallbackId ? persisted : note))
      );
      setSelectedNoteId(result.data.id);
      createNoteIds.current.delete(fallbackId);
      createNoteIds.current.add(result.data.id);
      openNoteInspector(persisted, "edit");
      source.refreshUndoStatus();
    }
  }

  async function duplicateNote(noteId: string): Promise<void> {
    if (noteInspectorModeRef.current === "edit") {
      await noteInspectorBodyRef.current?.flush();
    }

    const note = notes.find((candidate) => candidate.id === noteId);
    if (!note) {
      return;
    }

    const liveDraft =
      currentInspector?.kind === "note" && currentInspector.id === note.id
        ? noteInspectorBodyRef.current?.getDraft()
        : null;

    openLocalNoteDraft({
      title: copiedTitle(liveDraft?.title ?? note.title, "Untitled note"),
      body: liveDraft?.body ?? note.body,
      tags: liveDraft?.tags ?? note.tags ?? [],
      listId: note.listId,
      listTitle: note.listTitle
    });
  }

  async function saveLocalNoteDraft(noteId: string): Promise<void> {
    const note = notes.find((candidate) => candidate.id === noteId);

    if (!note) {
      return;
    }

    const draft = noteInspectorBodyRef.current?.getDraft() ?? {
      title: note.title,
      body: note.body,
      tags: note.tags ?? []
    };
    setNoteActionError(undefined);
    const result = await window.hcb?.tasks.create({
      title: draft.title || "Untitled note",
      notes: draft.body,
      listId: note.listId,
      dueDate: null,
      tags: draft.tags
    });

    if (!result?.ok) {
      const message = result?.error.message ?? "Note duplicate was not saved.";
      const failedDraft = {
        ...note,
        title: draft.title || "Untitled note",
        body: draft.body,
        preview: buildNotePreview(draft.body),
        tags: draft.tags,
        updatedLabel: "Edited"
      };

      setNoteActionError(message);
      updateInspector({
        actions: noteInspectorActions(failedDraft, "edit"),
        body: noteInspectorBody(failedDraft, "edit", message),
        dirty: true,
        hideHeader: false,
        subtitle: failedDraft.updatedLabel,
        title: failedDraft.title || "Untitled note"
      });
      return;
    }

    const cleanupError = await cleanupConvertedSource(noteId);
    if (cleanupError) {
      window.alert(`Converted item was saved, but ${cleanupError}`);
    }
    const persisted = noteFromTask(result.data, noteLists);

    setNotes((current) => current.map((candidate) => candidate.id === noteId ? persisted : candidate));
    setSelectedNoteId(persisted.id);
    createNoteIds.current.delete(noteId);
    createNoteIds.current.add(persisted.id);
    openNoteInspector(persisted, "edit");
    source.refreshUndoStatus();
    source.refresh();
  }

  async function createNoteList(): Promise<void> {
    const title = `Note list ${localNoteLists.length + 1}`;
    const result = await window.hcb?.tasks.createTaskList({ title });

    if (result?.ok) {
      setLocalNoteLists((current) => [...current, noteListFromTaskList(result.data)]);
      setSelectedNoteViews((current) => [...current, noteListSelection(result.data.id)]);
      source.refreshUndoStatus();
    }
  }

  async function deleteNoteList(listId: string, title: string): Promise<void> {
    if (!window.confirm(`Delete ${title}? Notes in this list will be deleted in Google Tasks.`)) {
      return;
    }

    const result = await window.hcb?.tasks.deleteTaskList({ id: listId });

    if (!result?.ok) {
      return;
    }

    setLocalNoteLists((current) =>
      current.filter((list) => list.id !== listId)
    );
    setNotes((current) => current.filter((note) => note.listId !== listId));
    setSelectedNoteViews((current) => {
      const next = current.filter((view) => view !== noteListSelection(listId));
      return next.length > 0 ? next : noteLists.filter((list) => list.id !== listId).map((list) => noteListSelection(list.id));
    });
    source.refreshUndoStatus();
  }

  async function renameNoteList(listId: string, currentTitle: string): Promise<void> {
    const title = window.prompt("Rename list", currentTitle)?.trim();

    if (!title || title === currentTitle) {
      return;
    }

    const result = await window.hcb?.tasks.renameTaskList({ id: listId, title });

    if (result?.ok) {
      const displayTitle = result.data.title;
      setLocalNoteLists((current) =>
        current.map((list) => (list.id === listId ? noteListFromTaskList(result.data) : list))
      );
      setNotes((current) =>
        current.map((note) => (note.listId === listId ? { ...note, listTitle: displayTitle } : note))
      );
      source.refreshUndoStatus();
    }
  }

  function updateNoteDraft(noteId: string, draft: NoteDraftValue): void {
    const startedAt = rendererNow();
    if (currentInspector?.kind === "note" && currentInspector.id === noteId) {
      setNoteActionError(undefined);
    }
    setNotes((current) =>
      current.map((note) => {
        if (note.id !== noteId) {
          return note;
        }

        return {
          ...note,
          title: draft.title,
          body: draft.body,
          preview: buildNotePreview(draft.body),
          tags: draft.tags,
          updatedLabel: "Edited"
        };
      })
    );

    if (startedAt !== null && startedAt - lastNoteEditReportAt.current > 250) {
      lastNoteEditReportAt.current = startedAt;
      scheduleRendererFrame(() => {
        reportRendererTimingSince("notes.edit.local", startedAt, {
          field: "body",
          noteCount: notes.length
        });
      });
    }
  }

  async function persistNoteDraft(noteId: string, draft: NoteDraftValue): Promise<boolean> {
    if (noteId.startsWith("note-draft-")) {
      const note = notes.find((candidate) => candidate.id === noteId);
      if (!note) {
        return false;
      }

      const result = await window.hcb?.tasks.create({
        title: draft.title || "Untitled note",
        notes: draft.body,
        listId: note.listId,
        dueDate: null,
        tags: draft.tags
      });

      if (!result?.ok) {
        return false;
      }

      const cleanupError = await cleanupConvertedSource(noteId);
      if (cleanupError) {
        window.alert(`Converted item was saved, but ${cleanupError}`);
      }
      const persisted = noteFromTask(result.data, noteLists);
      setNotes((current) => current.map((candidate) => candidate.id === noteId ? persisted : candidate));
      setSelectedNoteId(persisted.id);
      createNoteIds.current.delete(noteId);
      createNoteIds.current.add(persisted.id);
      source.refreshUndoStatus();
      source.refresh();
      return true;
    }

    const result = await window.hcb?.tasks.update({
      id: noteId,
      title: draft.title,
      notes: draft.body,
      dueDate: null,
      tags: draft.tags
    });

    if (result?.ok) {
      const cleanupError = await cleanupConvertedSource(noteId);
      if (cleanupError) {
        window.alert(`Converted item was saved, but ${cleanupError}`);
      }
      source.refreshUndoStatus();
      source.refresh();
    } else {
      setNoteActionError(result?.error.message ?? "Note was not saved.");
    }

    return result?.ok ?? false;
  }

  async function deleteNote(noteId: string): Promise<void> {
    const note = notes.find((candidate) => candidate.id === noteId);

    if (!note) {
      return;
    }

    if (!note.id.startsWith("note-draft-")) {
      const result = await window.hcb?.tasks.delete({ id: note.id });

      if (result?.ok) {
        source.refreshUndoStatus();
      }
    }

    setStarredNoteIds((current) => {
      if (!current.has(note.id)) {
        return current;
      }

      const next = new Set(current);
      next.delete(note.id);
      return next;
    });
    setStarredNoteAt((current) => {
      if (!(note.id in current)) {
        return current;
      }

      const next = { ...current };
      delete next[note.id];
      return next;
    });

    const nextNotes = notes.filter((candidate) => candidate.id !== note.id);
    const nextNote = nextNotes[0] ?? null;

    setNotes(nextNotes);
    setSelectedNoteId(nextNote?.id ?? null);
    createNoteIds.current.delete(note.id);
    conversionCleanupByNoteId.current.delete(note.id);

    if (nextNote) {
      openNoteInspector(nextNote, "view");
      return;
    }

    await closeInspector();
  }

  function toggleNoteStar(noteId: string): void {
    setStarredNoteIds((current) => {
      const next = new Set(current);

      if (next.has(noteId)) {
        next.delete(noteId);
        setStarredNoteAt((timestamps) => {
          const result = { ...timestamps };
          delete result[noteId];
          return result;
        });
      } else {
        next.add(noteId);
        setStarredNoteAt((timestamps) => ({
          ...timestamps,
          [noteId]: Date.now()
        }));
      }

      return next;
    });
  }

  async function moveNoteToList(noteId: string, listId: string): Promise<void> {
    const note = notes.find((candidate) => candidate.id === noteId);
    const list = noteLists.find((candidate) => candidate.id === listId);

    if (!note || !list) {
      return;
    }

    setNotes((current) =>
      current.map((candidate) =>
        candidate.id === noteId
          ? { ...candidate, listId, listTitle: list.title }
          : candidate
      )
    );

    if (!note.id.startsWith("note-draft-")) {
      const result = await window.hcb?.tasks.move({ id: note.id, listId });

      if (!result?.ok) {
        setNotes((current) =>
          current.map((candidate) =>
            candidate.id === noteId
              ? { ...candidate, listId: note.listId, listTitle: note.listTitle }
              : candidate
          )
        );
      }
    }
  }

  function toggleNoteView(view: NoteBoardSelection): void {
    setSelectedNoteViews((current) => {
      if (current.includes(view)) {
        return current.filter((selectedView) => selectedView !== view);
      }

      return [...current, view];
    });
  }

  return {
    createNote,
    createNoteList,
    deleteNoteList,
    deleteNote,
    duplicateNote,
    noteViewColumns,
    noteLists,
    notes,
    selectedNoteId,
    selectedNoteViews,
    selectNote,
    starredNoteIds,
    moveNoteToList,
    renameNoteList,
    toggleNoteStar,
    toggleNoteView
  };
}
