#!/usr/bin/env python3
"""Headless verification against the fresh packaged Go runtime."""
import concurrent.futures, datetime, hashlib, json, os, pathlib, subprocess, time
ROOT=pathlib.Path(__file__).resolve().parents[3]
E=ROOT/'macos/motion/evidence/p04u-1'
RUNTIME='/tmp/Gator-Native-P04U1-Motion.app/Contents/Resources/gator-runtime'
NODE='/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin'
env=dict(os.environ,PATH=NODE+':'+os.environ['PATH'],GATOR_TEST_BINARY=RUNTIME)
results=[]
def run(name,args):
 start=time.monotonic();stamp=datetime.datetime.now(datetime.timezone.utc).isoformat()
 with (E/(name+'.log')).open('w') as log:
  p=subprocess.run(args,cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT)
 return dict(name=name,command=args,startedUTC=stamp,seconds=time.monotonic()-start,exit=p.returncode,runtime=RUNTIME,runtimeSHA256=hashlib.sha256(pathlib.Path(RUNTIME).read_bytes()).hexdigest())
def swift():
 out=[run('swift-build',['swift','build','--package-path','macos','--scratch-path','/tmp/gator-native-build'])]
 out.append(run('swift-full',['swift','test','--package-path','macos','--scratch-path','/tmp/gator-native-build','--no-parallel']))
 out.append(run('swift-strict',['swift','test','--package-path','macos','--scratch-path','/tmp/gator-native-build','--no-parallel','-Xswiftc','-strict-concurrency=complete','-Xswiftc','-warnings-as-errors','--filter','streaming|mascot|presence|desktopPet|desktopPresence|realGoPresentationNavigationAndShutdownDoNotDecidePendingApproval|approvalGuardsExactIdentity|terminalSpinner|nativeSnapshotComparison|meaningfulSnapshots|identicalStateCanReconnect|realGoSpinnerOnly']))
 return out
pattern='Test(DesktopApprovalCannotRespondToAnotherLiveRequest|WebResearchReviewIdentityDoesNotDependOnDisplayFields|ReadInteractionRecordsValidatesAndScopesRetainedDecisions|InteractionDigestSurvivesRetainedActionPreview|InteractionDigestPreservesLargeJSONNumber|InteractionRunLockReportsActiveThenStopped|PublicResearchWaitsForBoundForegroundReviewAndRetainsFetchedEvidence|WebScopeDenialInvalidRequestsAndRevocation|RevocationDuringReviewExpiresApproval|PublicGrantDoesNotBypassPrivateDNSOrCancelledReview)$'
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
 futures=[pool.submit(swift),pool.submit(run,'go-safety',['go','test','-count=1','-json','./internal/workapp','./internal/workrun','-run',pattern]),pool.submit(run,'host-node26',['npm','test','--prefix','app-host'])]
 for future in concurrent.futures.as_completed(futures):
  value=future.result();results.extend(value if isinstance(value,list) else [value]);print(json.dumps(value),flush=True)
  (E/'verification.json').write_text(json.dumps(results,indent=2))
if any(r['exit'] for r in results):raise SystemExit(1)
