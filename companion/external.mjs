// Explicit display-only projection: backend configuration and native IDs stay local.
export function validateExternal(value,target='',generation='',snapshot=false){
 const text=(v,n)=>typeof v==='string'&&Buffer.byteLength(v)<=n;
 if(!value||!Array.isArray(value.targets)||value.targets.length>(target?1:16)||Buffer.byteLength(JSON.stringify(value))>64*1024)throw new Error('Invalid external inspection.');
 const targets=value.targets.map(i=>{
  const t=i?.target,b=t?.binding;
  if(i.controls!==undefined&&(!Array.isArray(i.controls)||i.controls.length>3||i.controls.some(o=>!['text_input','key_input','interrupt'].includes(o))))throw new Error('Invalid external capabilities.');
  if(!b||!/^target-[a-f0-9]{32}$/.test(b.id)||b.backend!=='external_tmux'||!/^tmux-instance-[a-f0-9]{32}$/.test(b.owner)||!/^[a-f0-9]{32}$/.test(b.generation)||!/^[a-f0-9]{64}$/.test(b.incarnation)||target&&(b.id!==target||b.generation!==generation)||!text(t.label,256)||t.available!==true||JSON.stringify(t.capabilities)!=='["inspect"]'||!text(i.text,16384)||(!snapshot&&i.text!=='')||typeof i.truncated!=='boolean'||i.stale!==false||snapshot&&(!text(i.captured_at,64)||!Number.isFinite(Date.parse(i.captured_at))))throw new Error('Invalid external target.');
  return {controls:i.controls||[],target:{binding:{id:b.id,backend:b.backend,owner:b.owner,generation:b.generation,incarnation:b.incarnation},label:t.label,available:true,capabilities:['inspect']},text:i.text,truncated:i.truncated,stale:false,...(snapshot?{captured_at:i.captured_at}:{})};
 });
 return {targets,unavailable:value.unavailable===true,reason:value.unavailable?'Target unavailable or access withdrawn.':''};
}
