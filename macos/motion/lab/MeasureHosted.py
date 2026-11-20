#!/usr/bin/env python3
"""Matched fresh-process native component trials; no shared-desktop input."""
import json, os, pathlib, statistics, subprocess, time
ROOT=pathlib.Path(__file__).resolve().parents[3]
E=ROOT/'macos/motion/evidence/p04u-1'
SAMPLER='/tmp/gator-p04u-process-sampler'
subprocess.run(['clang','-O2',str(ROOT/'macos/presence/acceptance-r2/sample.c'),'-o',SAMPLER],check=True)
all_results=[]
for trial in range(1,4):
 for mode in (['before','after'] if trial!=2 else ['after','before']):
  phases=E/f'hosted-{mode}-phases.jsonl'
  if phases.exists(): phases.unlink()
  env=dict(os.environ,GATOR_MOTION_BENCH=mode)
  log=open(E/f'hosted-{mode}-{trial}.log','w')
  started=time.monotonic()
  app=subprocess.Popen(['swift','test','--package-path','macos','--scratch-path','/tmp/gator-native-build','--skip-build','--no-parallel','--filter','streamingHostedPresentationResourceTrial'],cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT)
  while not phases.exists() and app.poll() is None and time.monotonic()-started<20: time.sleep(.05)
  if not phases.exists(): raise RuntimeError('No component readiness phase')
  pid=json.loads(phases.read_text().splitlines()[0])['pid']
  samples=E/f'hosted-{mode}-{trial}-samples.jsonl'
  with samples.open('w') as out:
   sampler=subprocess.Popen([SAMPLER,str(pid),'40'],stdout=out)
   app.wait(timeout=45)
   sampler.wait(timeout=10)
  log.close()
  if app.returncode: raise RuntimeError(f'Fixture failed: {mode} {trial}')
  phase_values=[json.loads(x) for x in phases.read_text().splitlines()]
  phases.rename(E/f'hosted-{mode}-{trial}-phases.jsonl')
  (E/f'hosted-{mode}-layout.json').rename(E/f'hosted-{mode}-{trial}-layout.json')
  values=[json.loads(x) for x in samples.read_text().splitlines()]
  summaries=[]
  for first,last in zip(phase_values,phase_values[1:]):
   v=[x for x in values if first['epoch']+.15<=x['epoch']<last['epoch']]
   cpu=[(b['cpuNanoseconds']-a['cpuNanoseconds'])/1e9/(b['epoch']-a['epoch'])*100 for a,b in zip(v,v[1:])]
   summaries.append(dict(phase=first['phase'],samples=len(v),cpuMedian=statistics.median(cpu) if cpu else None,cpuPeak=max(cpu) if cpu else None,rssMiBMedian=statistics.median(x['rss'] for x in v)/1048576 if v else None,footprintMiBMedian=statistics.median(x['footprint'] for x in v)/1048576 if v else None,peakFootprintMiB=max(x['peakFootprint'] for x in v)/1048576 if v else None))
  result=dict(trial=trial,mode=mode,pid=pid,seconds=time.monotonic()-started,phases=summaries)
  all_results.append(result)
  (E/'hosted-resource-summary.json').write_text(json.dumps(all_results,indent=2))
  print(json.dumps(result),flush=True)
