#!/usr/bin/env python3
"""Headless regressions using the delivered development bundle's runtime."""
import concurrent.futures,datetime,hashlib,json,os,pathlib,subprocess,time
ROOT=pathlib.Path(__file__).resolve().parents[3];E=ROOT/'macos/reactions/evidence/p04e-2'
APP=pathlib.Path('/tmp/Gator-Native-P04E2-Reactions.app');RUNTIME=APP/'Contents/Resources/gator-runtime'
start=time.monotonic()
while not RUNTIME.exists():
 if time.monotonic()-start>180:raise RuntimeError('Final package unavailable')
 time.sleep(.1)
env=dict(os.environ,PATH='/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin:'+os.environ['PATH'],GATOR_TEST_BINARY=str(RUNTIME),GATOR_REACTION_CAPTURE='1')
results=[]
def run(name,args):
 start=time.monotonic();stamp=datetime.datetime.now(datetime.timezone.utc).isoformat()
 with (E/(name+'.log')).open('w') as out:p=subprocess.run(args,cwd=ROOT,env=env,stdout=out,stderr=subprocess.STDOUT)
 return dict(name=name,command=args,startedUTC=stamp,seconds=time.monotonic()-start,exit=p.returncode,runtime=str(RUNTIME),runtimeSHA256=hashlib.sha256(RUNTIME.read_bytes()).hexdigest())
def swift():
 args=['swift','test','--package-path','macos','--scratch-path','/tmp/gator-native-build','--no-parallel']
 return [run('swift-build',['swift','build','--package-path','macos','--scratch-path','/tmp/gator-native-build']),run('swift-full',args),run('swift-full-strict',args+['-Xswiftc','-strict-concurrency=complete','-Xswiftc','-warnings-as-errors'])]
def go():
 return [run('go-all',['go','test','-count=1','-json','./internal/workrun','./internal/worksession','./internal/workapp','./cmd/gator']),run('go-race',['go','test','-race','-count=1','-json','./internal/workrun','./internal/worksession','./internal/workapp']),run('classifier-benchmark',['go','test','./internal/workrun','-run','^$','-bench','BenchmarkQuickAcknowledgement','-benchmem','-count=3'])]
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
 tasks=[pool.submit(swift),pool.submit(go),pool.submit(run,'host-node26',['npm','test','--prefix','app-host'])]
 for task in concurrent.futures.as_completed(tasks):
  value=task.result();results.extend(value if isinstance(value,list) else [value])
  (E/'verification.json').write_text(json.dumps(results,indent=2));print(json.dumps(value),flush=True)
if any(v['exit'] for v in results):raise SystemExit(1)
