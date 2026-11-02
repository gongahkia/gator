import { useContext } from "react";
import { Check, CircleAlert, FileCode2, FileText, Globe } from "lucide-react";
import type { ActivityDetail } from "./protocol";
import { SourceReferenceContext } from "./sourceReferences";

export function ActivityDetails({ details, compact=false }: { details: ActivityDetail[]; compact?: boolean }) {
 const source=useContext(SourceReferenceContext),shown=compact?details.slice(0,6):details;
 return <div className="activity-details" aria-label="Observed activity details">{shown.map((detail,index)=>{
  if(detail.kind==="searched"||detail.kind==="fetched") {
   const label=`${detail.kind==="searched"?"Search result":"Fetched"}: ${detail.domain}`;
   return source.run && source.open && detail.kind==="fetched" && detail.evidence_id ? <button key={index} className="activity-source-chip" title={label} onClick={()=>source.open?.({run:source.run!,path:`evidence:${detail.evidence_id}`})}><Globe size={12}/>{detail.domain}</button> : <span key={index} className="activity-source-chip" title={label}><Globe size={12}/>{detail.kind==="searched"?"Result · ":""}{detail.domain}</span>;
  }
  if(detail.kind==="read_file"&&detail.path?.startsWith("source/")&&source.run&&source.open)return <button key={index} className="activity-file-chip" title="Inspect the retained source" onClick={()=>source.open?.({run:source.run!,path:detail.path!})}><FileText size={12}/>{detail.path}</button>;
  if(detail.kind==="changed_file"||detail.kind==="read_code"||detail.kind==="read_file")return detail.kind==="changed_file" && detail.path && source.codeAvailable?.(detail.path,detail.candidate_id) ? <button key={index} className="activity-file-chip" aria-label={`Inspect proposed changes to ${detail.path}`} onClick={()=>source.inspectCode?.(detail.path!,detail.candidate_id)}><FileCode2 size={12}/>{detail.path}</button> : <span key={index} className="activity-file-chip" title={detail.kind==="changed_file"?"Observed staged edit; a retained proposal is not available yet":"Observed file read"}><FileText size={12}/>{detail.path}</span>;
  if(detail.kind==="command")return <span key={index} className="activity-observation" data-tone={detail.exit_code===0&&!detail.timed_out?undefined:"warning"}>{detail.exit_code===0&&!detail.timed_out?<Check size={12}/>:<CircleAlert size={12}/>}Program {detail.timed_out?"timed out":`exit ${detail.exit_code}`}</span>;
  if(detail.kind==="candidate")return <span key={index} className="activity-observation" data-tone={detail.status==="verified"?undefined:"warning"}>{detail.status==="verified"?<Check size={12}/>:<CircleAlert size={12}/>}Candidate checks {detail.status==="verified"?"passed · review still required":detail.status}</span>;
  return null;
 })}{compact && details.length>6 && <small>+{details.length-6} details in tools</small>}</div>;
}
