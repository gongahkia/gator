// Display contract shared by the terminal and desktop. Go owns workflow state.
export type ResultTone = "success" | "warning" | "error" | "muted" | "";
export type ResultField = { label: string; value: string; tone?: ResultTone };
export type ResultItem = { label: string; detail?: string; state?: string; tone?: ResultTone };
export type ResultSection = { title: string; summary?: string; fields?: ResultField[]; items?: ResultItem[]; lines?: string[]; markdown?: string };
export type BackendResult = {
  kind: "notice" | "details" | "list" | "report" | "preview" | "approval";
  title: string;
  tone?: ResultTone;
  summary?: string;
  fields?: ResultField[];
  items?: ResultItem[];
  sections?: ResultSection[];
  lines?: string[];
  markdown?: string;
  preview?: unknown;
  footer?: string;
};
export type BackendBundle = {
  Path: string;
  Status: string;
  Artifacts: { Path: string; MediaType: string; Valid: boolean }[] | null;
  Candidates: { ID: string; Status: string; ChangedPaths: string[] }[] | null;
};
export type Overview = {
  at: string;
  since: string;
  sections: { id: string; title: string; summary: string; total: number }[];
};
export type BackendCommand = { name: string; description: string; advanced: boolean; needs_input: boolean };
export type BackendMessage = { role: string; text: string; bundle?: BackendBundle; result?: BackendResult };
export type BackendPanel = {
  id?: string;
  kind: string;
  title: string;
  subtitle?: string;
  query?: string;
  rows?: {
    label: string; detail?: string; state?: string; tone?: ResultTone;
    input?: { value?: string; placeholder?: string; secret?: boolean; multiline?: boolean; limit: number };
  }[];
  blocks?: { title?: string; lines: string[] }[];
  selected?: number;
  notice?: string;
  footer?: string;
  hints?: { shortcut: string; action: string }[];
};
export type ExternalCommand = { kind: string; program: string; args: string[]; dir?: string; env?: string[] };
export type BackendState = {
  mode: "chat" | "panel";
  panel?: BackendPanel;
  messages: BackendMessage[];
  commands: BackendCommand[];
  actions: { Label: string; Command: string }[];
  title: string;
  source: string;
  goal_id?: string;
  checks?: { title: string; health: string; state: string; tone?: ResultTone }[];
  provider?: string;
  model?: string;
  theme: string;
  status?: string;
  status_line: string[];
  status_line_enabled: boolean;
  input?: string;
  running: boolean;
  cancelling: boolean;
  elapsed?: string;
  loading_frame: number;
  queued: number;
  home: boolean;
  first_run: boolean;
  supervisor_needs_start: boolean;
  overview?: Overview;
  overview_issue?: string;
  external?: ExternalCommand;
  pending_approval?: { id: number };
  notice?: string;
  destination?: string;
};
export type PanelAction = {
  id: string; action: "select" | "activate" | "key" | "query";
  row?: number; fields?: Record<number, string>; text?: string;
};
export type Frame = {
  version: 1;
  type: "submit" | "navigate" | "key" | "resize" | "exit" | "external_done" | "panel" | "approval";
  text?: string; key?: string; error?: string; width?: number; height?: number;
  panel?: PanelAction; interaction_id?: number; approved?: boolean;
};
