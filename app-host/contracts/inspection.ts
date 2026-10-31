// Read-only companion projection. This is not a FrontendFrame or a tool grant.
export type OwnedWorkInspection = {
  controls?: WorkControlOffer[];
  target: {
    binding: { id: string; backend: "gator_work"; owner: string; generation: string; incarnation?: string };
    label: string; capabilities: ["inspect"]; available: true;
  };
  run_id: string; conversation_id?: string; status: "running";
  started_at: string; observed_at: string; updated_at: string;
  progress: string[];
  specialists: { id: string; role: string; status: string; native_status?: string; pending_question: boolean }[];
  pending_approvals: number; limitations: string[];
};
export type OwnedWorkResponse = { targets: OwnedWorkInspection[]; unavailable?: boolean; reason?: string; truncated?: boolean };

export type WorkControlOffer = { operation:"cancel"|"steer"|"answer"|"decision";scope:string;expires_at:string;specialist?:string;question?:string;questions?:{id:string;text:string;options?:string[]}[];decision_id?:number;preview_digest?:string;preview?:string[] };
export type WorkRemoteAction = { id:string;run_id:string;target:OwnedWorkInspection["target"]["binding"];operation:WorkControlOffer["operation"];specialist?:string;question?:string;instruction?:string;answers?:Record<string,string[]>;decision_id?:number;preview_digest?:string;approved?:boolean };
export type WorkControlReceipt = {id:string;run_id:string;target:WorkRemoteAction["target"];operation:WorkControlOffer["operation"];scope:string;digest:string;phase:"claimed"|"settled";delivery:"not_dispatched"|"dispatch_acknowledged"|"outcome_unknown";note:string;created_at:string;updated_at:string};
