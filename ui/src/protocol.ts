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
export type QueuedMessage = { id: string; text: string };
export type QueueAction = {
  request_id: string; id: string; action: "edit" | "remove" | "earlier" | "later" | "run" | "guide"; text?: string;
};
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
  blocks?: { title?: string; lines: string[] | null }[];
  usage?: Usage;
  selected?: number;
  notice?: string;
  footer?: string;
  hints?: { shortcut: string; action: string }[];
};
export type Usage = {
  day: string;
  reset_at: string;
  paused: boolean;
  requests: number;
  tokens: number;
  active_requests: number;
  failed_requests: number;
  estimated_requests: number;
  daily_request_limit: number;
  daily_token_limit: number;
  concurrent_limit: number;
  categories: { name: string; requests: number }[];
  known_cost_usd?: number;
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
  conversation_id?: string;
  recent_conversations?: { id: string; title: string; source: string; updated_at: string }[];
  recent_id?: string;
  recent_issue?: string;
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
  queue: QueuedMessage[];
  queue_result?: { request_id: string; error?: string };
  guidance_id?: string;
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
  type: "submit" | "navigate" | "key" | "resize" | "exit" | "external_done" | "panel" | "approval" | "queue" | "conversation";
  text?: string; key?: string; error?: string; width?: number; height?: number;
  panel?: PanelAction; interaction_id?: number; approved?: boolean;
  queue?: QueueAction;
  conversation?: { id: string; list_id: string };
};
