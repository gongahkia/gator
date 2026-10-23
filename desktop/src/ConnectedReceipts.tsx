import { useCallback, useEffect, useRef, useState } from "react";
import { Check, ChevronDown, CircleAlert, Clock3, Slash } from "lucide-react";
import type { ActionReceipt, BackendBundle, BackendState, ReceiptCheck } from "./protocol";
import type { DesktopFrame } from "./bridge";
import "./connected-receipts.css";

export type InspectReceipt = (bundle: BackendBundle, receipt: ActionReceipt, action: "inspect" | "check_sent") => Promise<ReceiptCheck>;
export function useReceiptInspection(state: BackendState | undefined, send: (frame: DesktopFrame) => Promise<void>, connectionError?: string) {
  const pending = useRef<{id: string; owner: string; timer: ReturnType<typeof setTimeout>; resolve(value: ReceiptCheck): void; reject(error: Error): void} | null>(null);
  const [busy,setBusy] = useState(false);
  const finish = useCallback((id: string, value?: ReceiptCheck, error?: string) => {
    const p=pending.current;if(!p || p.id!==id)return;
    pending.current=null;clearTimeout(p.timer);setBusy(false);
    if(error || !value)p.reject(new Error(error || "The receipt check was not confirmed."));else p.resolve(value);
  },[]);
  useEffect(()=>{
    const p=pending.current;if(!p)return;
    if(state?.conversation_id!==p.owner)finish(p.id,undefined,"Reopen this receipt in its original chat to inspect the saved check.");
    else if(state.receipt_result?.request_id===p.id)finish(p.id,state.receipt_result.check,state.receipt_result.error);
  },[state?.conversation_id,state?.receipt_result,finish]);
  useEffect(()=>{if(connectionError && pending.current)finish(pending.current.id,undefined,"Reconnect and reopen this receipt to inspect the saved check.");},[connectionError,finish]);
  useEffect(()=>()=>{const p=pending.current;pending.current=null;if(p){clearTimeout(p.timer);p.reject(new Error("The conversation closed. Reopen its receipt to inspect the saved check."));}},[]);
  const inspect: InspectReceipt=useCallback((bundle,receipt,action)=>new Promise((resolve,reject)=>{
    const owner=state?.conversation_id;
    if(pending.current){reject(new Error("Wait for the current receipt check."));return;}
    if(!owner || !bundle.RunID || !bundle.ManifestSHA256 || !receipt.CorrelationID){reject(new Error("Reopen the retained result to inspect this receipt."));return;}
    const id=crypto.randomUUID(),timer=setTimeout(()=>finish(id,undefined,"The check was not confirmed. Reopen this receipt to inspect its saved result."),30_000);
    pending.current={id,owner,timer,resolve,reject};setBusy(true);
    void send({type:"receipt",receipt:{conversation_id:owner,run_id:bundle.RunID,action_id:receipt.ID,correlation_id:receipt.CorrelationID,manifest_sha256:bundle.ManifestSHA256,action,request_id:id}}).catch(error=>finish(id,undefined,error instanceof Error?error.message:String(error)));
  }),[state?.conversation_id,send,finish]);
  return {inspect,busy};
}

function receiptLabel(r: ActionReceipt, check?: ReceiptCheck) {
  if(check?.status==="found")return "Found in Sent";
  if(r.Status==="unknown")return r.Operation==="gmail_send"?"Send not confirmed":"Request outcome unconfirmed";
  if(r.Status==="failed")return "Request failed";
  if(r.Status==="denied")return "Not approved";
  if(r.Status==="pending")return "Proposed request";
  if(r.Operation==="gmail_send" && r.ReceiptID)return "Gmail accepted the message";
  return "Request completed";
}
export function ConnectedReceipts({bundle,inspect,busy,disabled}:{bundle:BackendBundle;inspect:InspectReceipt;busy:boolean;disabled:boolean}) {
  if(!bundle.Actions?.length)return null;
  return <div className="connected-receipts" aria-label="Connected request receipts">{bundle.Actions.map((receipt,index)=><Receipt key={`${receipt.ID}:${receipt.CorrelationID || index}`} bundle={bundle} receipt={receipt} inspect={inspect} busy={busy} disabled={disabled} />)}{bundle.ActionsTruncated && <p>More requests are retained in this task’s full result.</p>}</div>;
}
function Receipt({bundle,receipt:r,inspect,busy,disabled}:{bundle:BackendBundle;receipt:ActionReceipt;inspect:InspectReceipt;busy:boolean;disabled:boolean}) {
  const [check,setCheck]=useState<ReceiptCheck>();
  const [loading,setLoading]=useState<"inspect"|"check_sent">();
  const [issue,setIssue]=useState("");
  const mounted=useRef(true);
  useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;};},[]);
  const canCheck=r.Status==="unknown"&&r.Operation==="gmail_send"&&!!r.CorrelationID&&!!bundle.ManifestSHA256;
  const run=async(action:"inspect"|"check_sent")=>{
    setLoading(action);setIssue("");
    try {const value=await inspect(bundle,r,action);if(mounted.current)setCheck(value);}
    catch(error){if(mounted.current)setIssue(error instanceof Error?error.message:"The receipt could not load.");}
    finally{if(mounted.current)setLoading(undefined);}
  };
  const uncertain=(r.Status==="unknown" || r.Status==="failed")&&check?.status!=="found";
  const label=receiptLabel(r,check);
  return <details className="connected-receipt" data-tone={uncertain?"warning":undefined} onToggle={event=>{if(event.currentTarget.open&&canCheck&&!check&&!busy&&!loading&&!issue)void run("inspect");}}>
    <summary>{uncertain?<CircleAlert size={14} aria-hidden="true" />:r.Status==="denied"?<Slash size={14} aria-hidden="true" />:r.Status==="pending"?<Clock3 size={14} aria-hidden="true" />:<Check size={14} aria-hidden="true" />}<span>{label}{r.Recipient&&` · ${r.Recipient}`}</span><ChevronDown className="receipt-chevron" size={13} aria-hidden="true" /></summary>
    <div className="connected-receipt-body">
      {r.Subject&&<p>{r.Subject}</p>}
      {r.Status==="unknown"&&<p>The original request did not have a confirmed outcome. Verify it before trying again.</p>}
      {r.Operation==="gmail_send"&&r.ReceiptID&&<p>Gmail returned a message receipt. This does not confirm recipient delivery.</p>}
      <dl><dt>Connection</dt><dd>{r.ConnectorID}</dd><dt>Request</dt><dd>{r.Operation.replaceAll("_"," ")}</dd><dt>Original outcome</dt><dd>{r.Status}</dd>{r.ReceiptID&&<><dt>Provider receipt</dt><dd>{r.ReceiptID}</dd></>}{r.ProviderResourceID&&<><dt>Provider resource</dt><dd>{r.ProviderResourceID}</dd></>}</dl>
      {!!r.Steps?.length&&<ol aria-label="Recorded batch steps">{r.Steps.map((step,index)=><li key={index}>{step.Operation.replaceAll("_"," ")} · {step.Status}{step.ProviderResourceID&&` · ${step.ProviderResourceID}`}</li>)}</ol>}
      {r.Error&&<details><summary>Technical details</summary><p>{r.Error}</p></details>}
      {canCheck&&<div className="receipt-sent-check">
        {check?.status==="found"&&<p role="status">A matching copy is in Sent. This confirms a Sent mailbox copy, not recipient delivery.{check.provider_id&&` Message: ${check.provider_id}.`}</p>}
        {check?.status==="ambiguous"&&<p role="status">Multiple Sent copies match. The outcome remains unconfirmed; inspect Sent manually before retrying.</p>}
        {check?.status==="unconfirmed"&&<p role="status">No verified Sent copy was found. Search indexing can lag; this does not establish that the send failed.</p>}
        {check?.checked_at&&<p className="receipt-check-time">Checked {new Date(check.checked_at).toLocaleString()}</p>}
        <p>Checks the account currently connected here. This only reads Sent.</p>
        <button className="small-button" disabled={disabled||busy||!!loading} onClick={()=>void run("check_sent")}>{loading==="check_sent"?"Checking Sent…":"Check Sent"}</button>
        {loading==="inspect"&&<p role="status">Loading the saved check…</p>}
      </div>}
      {issue&&<div role="alert"><p>Couldn’t inspect this receipt. Review the connection in Settings or try again.</p><details><summary>Details</summary><p>{issue}</p></details></div>}
    </div>
  </details>;
}
