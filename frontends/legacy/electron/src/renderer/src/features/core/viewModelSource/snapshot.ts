import type { CoreDataSnapshot } from "./types";

export function hasSnapshotData(snapshot: CoreDataSnapshot): boolean {
  return (
    snapshot.taskLists.length > 0 ||
    snapshot.tasks.length > 0 ||
    snapshot.calendars.length > 0 ||
    snapshot.events.length > 0 ||
    snapshot.scheduledTaskBlocks.length > 0 ||
    snapshot.notes.length > 0
  );
}
