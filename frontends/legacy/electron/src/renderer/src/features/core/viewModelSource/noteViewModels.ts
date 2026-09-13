import type { NoteDetail, NoteSummary } from "@shared/ipc/contracts";
import type { NoteViewModel } from "../coreViewModels";
import { shortDateTime } from "./dateFormat";

export function noteViewModel(note: NoteDetail | NoteSummary): NoteViewModel {
  return {
    id: note.id,
    listId: note.listId,
    listTitle: note.listTitle,
    title: note.title,
    body: "body" in note ? note.body : "",
    preview: note.preview,
    tags: note.tags ?? [],
    updatedLabel: shortDateTime(note.updatedAt)
  };
}
