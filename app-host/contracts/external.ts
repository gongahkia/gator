/** Display-only external enrollment projection. No native target or authority. */
export type ExternalTargetSnapshot = {
  target: {binding:{id:string;backend:"external_tmux";owner:string;generation:string;incarnation:string};label:string;available:true;capabilities:["inspect"]};
  controls?:("text_input"|"key_input"|"interrupt")[];
  captured_at?:string;text:string;truncated:boolean;stale:boolean;
};
export type ExternalTargetResponse = {targets:ExternalTargetSnapshot[];unavailable?:boolean;reason?:string};

export type ExternalTerminalAction={id:string;target:string;generation:string;input:{kind:"text"|"text_enter";text:string}|{kind:"key";key:"Enter"|"Escape"|"Tab"|"Backspace"|"Up"|"Down"|"Left"|"Right"}|{kind:"interrupt"}};
export type ExternalTerminalReceipt={id:string;target:{id:string;generation:string};operation:"text_input"|"key_input"|"interrupt";kind:ExternalTerminalAction["input"]["kind"];digest:string;phase:"claimed"|"settled";delivery:"not_dispatched"|"dispatch_acknowledged"|"outcome_unknown";note:string;created_at:string;updated_at:string};
