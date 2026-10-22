import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { getDocument, GlobalWorkerOptions, type PDFDocumentProxy } from "pdfjs-dist";
import PDFWorker from "pdfjs-dist/build/pdf.worker.min.mjs?worker";

export default function PDFPreview({ data }: { data: string }) {
  const canvas = useRef<HTMLCanvasElement>(null), root=useRef<HTMLDivElement>(null), [document,setDocument]=useState<PDFDocumentProxy>(), [page,setPage]=useState(1), [width,setWidth]=useState(600), [issue,setIssue]=useState("");
  useEffect(()=>{
    const worker=new PDFWorker();GlobalWorkerOptions.workerPort=worker;
    const bytes=Uint8Array.from(atob(data),character=>character.charCodeAt(0));
    const loading=getDocument({data:bytes,useSystemFonts:true,disableFontFace:true,useWasm:false});let active=true;
    void loading.promise.then(value=>{if(active){setDocument(value);setPage(1);}}).catch(error=>{if(active)setIssue(error instanceof Error?error.message:"Could not render this PDF.");});
    return ()=>{active=false;void loading.destroy().finally(()=>worker.terminate());};
  },[data]);
  useEffect(()=>{if(!root.current)return;const observer=new ResizeObserver(entries=>setWidth(Math.min(1000,Math.max(100,entries[0].contentRect.width-32))));observer.observe(root.current);return()=>observer.disconnect();},[]);
  useEffect(()=>{
    if(!document || !canvas.current)return;let canceled=false, task:ReturnType<Awaited<ReturnType<PDFDocumentProxy["getPage"]>>["render"]>|undefined;
    const target=canvas.current;
    void document.getPage(page).then(value=>{if(canceled)return;const original=value.getViewport({scale:1}),viewport=value.getViewport({scale:width/original.width}),ratio=Math.min(devicePixelRatio || 1,2);target.width=Math.ceil(viewport.width*ratio);target.height=Math.ceil(viewport.height*ratio);target.style.width=`${viewport.width}px`;target.style.height=`${viewport.height}px`;task=value.render({canvas:target,viewport,transform:[ratio,0,0,ratio,0,0]});return task.promise;}).catch(error=>{if(!canceled)setIssue(error instanceof Error?error.message:"Could not render this page.");});
    return()=>{canceled=true;task?.cancel();};
  },[document,page,width]);
  return <div ref={root} className="file-pdf">{issue?<p className="file-feedback file-error" role="alert">{issue}</p>:!document?<p className="file-feedback" role="status">Rendering PDF…</p>:<><div className="file-slide-nav"><button className="icon-button" aria-label="Previous PDF page" disabled={page===1} onClick={()=>setPage(value=>value-1)}><ChevronLeft size={16}/></button><span>Page {page} of {document.numPages}</span><button className="icon-button" aria-label="Next PDF page" disabled={page===document.numPages} onClick={()=>setPage(value=>value+1)}><ChevronRight size={16}/></button></div></>}<canvas ref={canvas} aria-label={`PDF page ${page}`} role="img"/></div>;
}
