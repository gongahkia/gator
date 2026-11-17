# Disposable loopback-only desktop fixture. Never reads a user's Gator profile.
import atexit, hashlib, json, os, pathlib, queue, signal, subprocess, tempfile, threading, time
repo = pathlib.Path.cwd()
root = pathlib.Path(tempfile.mkdtemp(prefix='gator-uiux-remediation-', dir='/tmp'))
(root/'workspace').mkdir(); (root/'config/gator').mkdir(parents=True)
node = '/Users/gongahkia/.local/share/fnm/node-versions/v26.7.0/installation/bin/node'
provider = subprocess.Popen([node, str(repo/'macos/audits/evidence/uiux/provider.mjs')], env={'PATH': os.environ['PATH'], 'GATOR_FIXTURE_CONTROL': str(root)}, stdout=subprocess.PIPE, stderr=open(root/'provider.log','w'), text=True)
atexit.register(lambda: provider.terminate() if provider.poll() is None else None)
signal.signal(signal.SIGTERM, lambda *_: exit(0))
port = json.loads(provider.stdout.readline())['port']
config = {'version':4,'defaults':{'provider':'openai','model':'gpt-5.6-terra'},'provider_endpoints':{'openai':f'http://127.0.0.1:{port}/responses'}}
(root/'config/gator/config.json').write_text(json.dumps(config)); (root/'seed-fast').touch()
env = {'PATH':os.environ['PATH'], 'HOME':str(root), 'GATOR_UI_BACKEND':'1', 'GATOR_CONFIG_DIR':str(root/'config'), 'GATOR_STATE_DIR':str(root/'state'), 'OPENAI_API_KEY':'synthetic-fixture-not-a-credential', 'GIT_CONFIG_NOSYSTEM':'1', 'GIT_CONFIG_GLOBAL':'/dev/null', 'GATOR_NATIVE_USER_DATA':str(root/'native-data'), 'GATOR_NATIVE_WORKSPACE':str(root/'workspace')}
runtime = '/tmp/Gator-Native-UIUX-Baseline.app/Contents/Resources/gator-runtime'
p = subprocess.Popen([runtime,'ui-backend','',''],cwd=root/'workspace',env=env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=open(root/'seed.log','w'),text=True,bufsize=1)
frames = queue.Queue()
def reader():
 for line in p.stdout:
  frames.put(json.loads(line))
threading.Thread(target=reader, daemon=True).start()
def send(frame):
 p.stdin.write(json.dumps({'version':1,**frame})+'\n');p.stdin.flush()
def wait(test):
 end=time.monotonic()+15
 while time.monotonic()<end:
  try: frame=frames.get(timeout=.1)
  except queue.Empty: continue
  if frame.get('state') and test(frame['state']):return frame['state']
 raise RuntimeError('Fixture seed timed out')
wait(lambda s:s.get('composer',{}).get('readiness',{}).get('state')=='configured')
ids=[]
for label in ['A','B']:
 if ids:
  send({'type':'navigate','text':'/new'})
  wait(lambda s: not s.get('messages'))
 for index in range(1,25):
  send({'type':'submit','text':f'History item {label}-{index:02}'})
  state=wait(lambda s:s.get('running') is not True and any(m.get('text','').startswith(f'## Retained note {label}-{index:02}') for m in s.get('messages',[])))
 ids.append(state['conversation_id'])
send({'type':'exit'});p.wait(timeout=10);assert p.returncode==0
(root/'seed-fast').unlink()
archive=root/'native-data/native/drafts';archive.mkdir(parents=True,mode=0o700)
path=archive/(hashlib.sha256(str(root/'state').encode()).hexdigest()+'.json')
path.write_text(json.dumps({'version':1,'selectedConversationID':ids[0],'drafts':{}}));path.chmod(0o600)
(root/'Trip notes.md').write_text('# Trip notes\nNo booking or sending.\n')
metadata={'root':str(root),'conversations':ids,'providerPID':provider.pid,'port':port,'env':env,'fixtureOnly':True,'network':'loopback only','paidProviderRequests':0}
(root/'session.json').write_text(json.dumps(metadata,indent=2));pathlib.Path('/tmp/gator-uiux-remediation-session.json').write_text(json.dumps(metadata,indent=2))
print(json.dumps({'root':str(root),'conversations':ids,'providerPID':provider.pid,'port':port}),flush=True)
try: provider.wait()
finally:
 if provider.poll() is None:provider.terminate();provider.wait(timeout=5)
