import { extractPlannerLinks as extractSharedPlannerLinks } from "@shared/plannerLinks";

export interface NoteProperty {
  key: string;
  value: string;
}

export {
  extractPlannerLinks,
  normalizedPlannerLinkLabel,
  parsePlannerLink,
  plannerLinkDisplayLabel,
  plannerLinkKinds,
  plannerLinkMarker,
  type PlannerLinkKind,
  type PlannerLinkReference,
  type PlannerLinkType
} from "@shared/plannerLinks";

export function buildNotePreview(body: string): string {
  const trimmed = body.trim();
  if (!trimmed) {
    return "Empty note";
  }

  return trimmed.length > 92 ? `${trimmed.slice(0, 89)}...` : trimmed;
}

export function normalizedNoteTitle(title: string): string {
  return title.trim().toLowerCase();
}

export function extractNoteLinks(body: string): string[] {
  return extractSharedPlannerLinks(body).map((link) => link.raw);
}

export function extractNoteProperties(body: string): NoteProperty[] {
  const supportedKeys = new Set(["status", "tags", "project", "date", "source"]);
  const properties: NoteProperty[] = [];

  for (const line of body.split(/\r?\n/).slice(0, 12)) {
    const match = /^([a-zA-Z][\w-]{1,24}):\s*(.+)$/.exec(line.trim());

    if (!match) {
      continue;
    }

    const key = match[1].toLowerCase();
    const value = match[2].trim();

    if (supportedKeys.has(key) && value) {
      properties.push({ key, value });
    }
  }

  return properties;
}
