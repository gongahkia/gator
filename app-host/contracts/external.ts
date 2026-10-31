/** Display-only external enrollment projection. No native target or authority. */
export type ExternalTargetSnapshot = {
  target: {binding:{id:string;backend:"external_tmux";owner:string;generation:string;incarnation:string};label:string;available:true;capabilities:["inspect"]};
  captured_at?:string;text:string;truncated:boolean;stale:boolean;
};
export type ExternalTargetResponse = {targets:ExternalTargetSnapshot[];unavailable?:boolean;reason?:string};
