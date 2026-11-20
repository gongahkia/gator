"""Sequential fixture-only actions; no global keyboard events or paid endpoint."""
import json,pathlib,subprocess,time,re
m=json.loads(pathlib.Path('/tmp/gator-presence-final-session.json').read_text());pid=str(m['pid']);root=pathlib.Path(m['root']); evidence=pathlib.Path('macos/presence/evidence/p04e-1'); trace=[]
assert root.name.startswith('gator-native-test-presence-')
def run(*args):
 r=subprocess.run(['/tmp/gator-presence-control',pid,*args],capture_output=True,text=True,timeout=12);trace.append({'args':args,'exit':r.returncode,'output':r.stdout});
 if r.returncode:raise RuntimeError('owned fixture action failed: '+str(args))
 return r.stdout
def states():
 out=[]
 for f in root.glob('latest-*.json'):
  try:out.append(json.loads(f.read_text()))
  except (ValueError,OSError):pass
 return out
def wait(check,seconds):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  value=check()
  if value:return value
  time.sleep(.03)
 raise RuntimeError('observable fixture readiness timed out')
def capture(name):
 frames=run('frames'); candidates=re.findall(r'captureID=(\d+) bounds=\{([^}]+)\}',frames,re.S)
 ids=[i for i,b in candidates if (w:=re.search(r'Width = ([0-9.]+);',b)) and float(w[1])>=540 and (h:=re.search(r'Height = ([0-9.]+);',b)) and float(h[1])>=450]; assert len(ids)==1
 subprocess.run(['python3','/Users/gongahkia/.codex/skills/screenshot/scripts/take_screenshot.py','--window-id',ids[0],'--path',str(evidence/name)],check=True,capture_output=True)
record=None
try:
 run('activate');run('press','menu','AXMenuBarItem::Window');run('press','menu','AXMenuItem::Gator');run('wait-window','conversation')
 log=(evidence/'keyboard-recording.log').open('w');record=subprocess.Popen(['/tmp/gator-presence-record',pid,str((evidence/'keyboard-live-reaction.mp4').resolve()),'22'],stdout=log,stderr=subprocess.STDOUT)
 wait(lambda: (evidence/'keyboard-recording.log').read_text().find('recording started')>=0,8)
 run('focus','conversation');run('focus-element','conversation','gator.composer.text');run('select-all','conversation','gator.composer.text');run('type','conversation','gator.composer.text','Controlled local presence observation');run('press','conversation','Send message')
 running=wait(lambda:next((s for s in states() if s.get('running')),None),6);capture('unique-working.png')
 complete=wait(lambda:next((s for s in states() if not s.get('running') and any((x.get('activity') or {}).get('state')=='completed' for x in s.get('messages',[]))),None),16);capture('unique-completed.png')
 trace.append({'GoRunning':running.get('conversation_id'),'GoCompleted':complete.get('conversation_id')})
 record.wait(timeout=28)
 if record.returncode:raise RuntimeError('recording not confirmed')
 run('focus','conversation');run('focus-element','conversation','gator.composer.text');run('type','conversation','gator.composer.text','Review mascot');run('press','conversation','Send message')
 pending=wait(lambda:next((s for s in states() if s.get('pending_approval')),None),12)
 run('press','conversation','An action needs your review');capture('unique-pending-review.png');trace.append({'pendingReview':True,'decisionSubmitted':False})
except Exception as error:trace.append({'unverified':str(error)})
finally:
 (evidence/'keyboard-native-results.json').write_text(json.dumps(trace,indent=2)+'\n')
 print(json.dumps(trace[-2:],indent=2))
