"""Read-only cadence attribution after an explicitly initiated inert fixture task."""
import json,os,pathlib,queue,shutil,subprocess,tempfile,threading,time,collections
out=pathlib.Path('macos/presence/evidence/p04e-1r2')
root=pathlib.Path(tempfile.mkdtemp(prefix='gator-native-test-presence-r2-cadence-',dir='/tmp'));root.chmod(0o700)
(root/'workspace').mkdir();(root/'config/gator').mkdir(parents=True)
provider=subprocess.Popen([shutil.which('node'),'macos/mascot/2d/acceptance/provider.mjs'],env={'PATH':os.environ['PATH'],'GATOR_FIXTURE_CONTROL':str(root)},stdout=subprocess.PIPE,stderr=open(root/'provider.log','w'))
port=json.loads(provider.stdout.readline())['port']
(root/'config/gator/config.json').write_text(json.dumps({'version':4,'defaults':{'provider':'openai','model':'gpt-5.6'},'provider_endpoints':{'openai':f'http://127.0.0.1:{port}/responses'}}))
runtime='/tmp/Gator-Native-P04E1R2-Diagnostic.app/Contents/Resources/gator-runtime'
env={'PATH':os.environ['PATH'],'HOME':str(root),'GATOR_CONFIG_DIR':str(root/'config'),'GATOR_STATE_DIR':str(root/'state'),'OPENAI_API_KEY':'inert-only-cadence-fixture','GATOR_UI_BACKEND':'1','GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null'}
p=subprocess.Popen([runtime,'ui-backend','',''],cwd=root/'workspace',env=env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=open(root/'go.log','w'),text=True)
q=queue.Queue()
def read():
 for line in p.stdout:
  try:q.put((time.monotonic(),json.loads(line)))
  except ValueError:pass
threading.Thread(target=read,daemon=True).start()
def send(value):p.stdin.write(json.dumps({'version':1,**value})+'\n');p.stdin.flush()
frames=[];previous=None;sent=False;pendingAt=None;changes=collections.Counter();end=time.monotonic()+15
try:
 while time.monotonic()<end:
  epoch,e=q.get(timeout=5)
  if e.get('type')!='state':continue
  s=e['state']
  if not sent:send({'type':'submit','text':'Review mascot'});sent=True
  if s.get('pending_approval'):
   if pendingAt is None:pendingAt=epoch
   diff=sorted(k for k in set(s)|set(previous or {}) if s.get(k)!=(previous or {}).get(k));changes.update(diff)
   frames.append({'time':epoch,'changedTopLevelKeys':diff,'loading_frame':s.get('loading_frame'),'elapsed':s.get('elapsed')})
   if epoch-pendingAt>=5:break
  previous=s
 send({'type':'exit'});p.wait(timeout=5)
 result={'root':str(root),'runtime':runtime,'fixtureSetup':'explicit submit of inert Review mascot; no decision command','states':frames,'changedKeyCounts':dict(changes),'exit':p.returncode,'pendingDuration':frames[-1]['time']-frames[0]['time'] if frames else 0}
 (out/'bridge-cadence.json').write_text(json.dumps(result,indent=2));print(json.dumps({k:v for k,v in result.items() if k!='states'},indent=2))
finally:
 if p.poll() is None:p.terminate();p.wait(timeout=5)
 provider.terminate();provider.wait(timeout=3)
