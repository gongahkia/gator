#!/usr/bin/env python3
"""Fresh-process hosted reaction comparisons. Sends no desktop input."""
import datetime,hashlib,json,os,pathlib,statistics,subprocess,time
ROOT=pathlib.Path(__file__).resolve().parents[3]; E=ROOT/'macos/reactions/evidence/p04e-2'
SAMPLER='/tmp/gator-p04e2-resource-sampler'
subprocess.run(['clang','-O2',str(ROOT/'macos/presence/acceptance-r2/sample.c'),'-o',SAMPLER],check=True)
sources=list((ROOT/'macos/Sources').rglob('*.swift'))+list((ROOT/'macos/Tests').rglob('*.swift'))
(E/'measured-source.json').write_text(json.dumps({str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sources},indent=2))
results=[]
for trial in range(1,4):
 for mode in (['before','after'] if trial!=2 else ['after','before']):
  phase=E/f'resource-{mode}-phases.jsonl'
  if phase.exists():phase.unlink()
  env=dict(os.environ,GATOR_REACTION_BENCH=mode)
  started=time.monotonic()
  with (E/f'resource-{mode}-{trial}.log').open('w') as log:
   app=subprocess.Popen(['swift','test','--package-path','macos','--scratch-path','/tmp/gator-native-build','--skip-build','--no-parallel','--filter','messageReactionResourceTrial'],cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT)
   while not phase.exists() and app.poll() is None and time.monotonic()-started<20:time.sleep(.05)
   if not phase.exists():raise RuntimeError('No verified component readiness')
   pid=json.loads(phase.read_text().splitlines()[0])['pid']
   sample=E/f'resource-{mode}-{trial}-samples.jsonl'
   with sample.open('w') as out:
    sampler=subprocess.Popen([SAMPLER,str(pid),'60'],stdout=out)
    app.wait(timeout=60);sampler.wait(timeout=10)
  if app.returncode:raise RuntimeError('Component test failed')
  phases=[json.loads(v) for v in phase.read_text().splitlines()]
  phase.rename(E/f'resource-{mode}-{trial}-phases.jsonl')
  (E/f'resource-{mode}-layout.json').rename(E/f'resource-{mode}-{trial}-layout.json')
  values=[json.loads(v) for v in sample.read_text().splitlines()]
  rows=[]
  for a,b in zip(phases,phases[1:]):
   v=[x for x in values if a['epoch']+.15<=x['epoch']<b['epoch']]
   cpu=[(y['cpuNanoseconds']-x['cpuNanoseconds'])/1e9/(y['epoch']-x['epoch'])*100 for x,y in zip(v,v[1:])]
   rows.append(dict(phase=a['phase'],samples=len(v),cpuMedian=statistics.median(cpu) if cpu else None,cpuPeak=max(cpu) if cpu else None,rssMiBMedian=statistics.median(x['rss'] for x in v)/1048576 if v else None,footprintMiBMedian=statistics.median(x['footprint'] for x in v)/1048576 if v else None,lifetimePeakMiB=max(x['peakFootprint'] for x in v)/1048576 if v else None))
  result=dict(trial=trial,mode=mode,pid=pid,seconds=time.monotonic()-started,phases=rows);results.append(result)
  (E/'resources.json').write_text(json.dumps(results,indent=2));print(json.dumps(result),flush=True)
