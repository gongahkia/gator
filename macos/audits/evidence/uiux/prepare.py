import json,os,pathlib,subprocess,tempfile,selectors,time,shutil
repo=pathlib.Path.cwd();root=pathlib.Path(tempfile.mkdtemp(prefix='gator-uiux-audit-'));(root/'workspace').mkdir();(root/'config/gator').mkdir(parents=True)
provider=subprocess.Popen(['node',str(repo/'macos/audits/evidence/uiux/provider.mjs')],env={'PATH':os.environ['PATH'],'GATOR_FIXTURE_CONTROL':str(root)},stdout=subprocess.PIPE,stderr=open(root/'provider.log','w'),text=True)
port=json.loads(provider.stdout.readline())['port'];app='/tmp/Gator-Native-Tranche08-Enhanced-Final.app';runtime=app+'/Contents/Resources/gator-runtime'
config={'version':4,'defaults':{'provider':'openai','model':'gpt-5.6-terra'},'provider_endpoints':{'openai':f'http://127.0.0.1:{port}/responses'},'custom_providers':[{'id':'audit-chat','base_url':f'http://127.0.0.1:{port}/chat','models':['fixture'],'default_model':'fixture'}]}
(root/'config/gator/config.json').write_text(json.dumps(config));(root/'seed-fast').touch()
env={'PATH':os.environ['PATH'],'HOME':str(root),'GATOR_CONFIG_DIR':str(root/'config'),'GATOR_STATE_DIR':str(root/'state'),'GATOR_UI_BACKEND':'1','OPENAI_API_KEY':'synthetic-audit-key-not-a-credential','GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null'}
p=subprocess.Popen([runtime,'ui-backend','',''],cwd=root/'workspace',env=env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=open(root/'seed.log','w'),text=True,bufsize=1)
sel=selectors.DefaultSelector();sel.register(p.stdout,selectors.EVENT_READ)
def send(text):p.stdin.write(json.dumps({'version':1,'type':'submit','text':text})+'\n');p.stdin.flush()
def wait_for(test):
 end=time.monotonic()+12
 while time.monotonic()<end:
  if not sel.select(.1):continue
  line=p.stdout.readline()
  if not line:raise RuntimeError('runtime exited')
  e=json.loads(line)
  if e.get('state') and test(e['state']):return e['state']
 raise RuntimeError('fixture seeding timed out')
wait_for(lambda s:s.get('composer',{}).get('readiness',{}).get('state')=='configured')
conversation=''
for i in range(1,41):
 send(f'History item {i}');s=wait_for(lambda s:s.get('running') is not True and any(m.get('text', '').startswith(f'## Retained note {i}') for m in s.get('messages',[])));conversation=s['conversation_id']
p.stdin.write('{"version":1,"type":"exit"}\n');p.stdin.flush();p.wait(timeout=10);(root/'seed-fast').unlink()
(root/'Trip notes.md').write_text('# Trip notes\n\nCompare morning and afternoon trains. Do not book anything.\n')
shutil.copy2(repo/'macos/Resources/GatorHead.png',root/'Alligator reference.png')
metadata={'root':str(root),'providerPID':provider.pid,'port':port,'app':app,'historyConversation':conversation,'historyWorkTurns':40,'fixtureOnly':True,'network':'loopback only','paidModelCalls':0}
pathlib.Path('/tmp/gator-uiux-audit-session.json').write_text(json.dumps(metadata,indent=2));print(json.dumps(metadata),flush=True);provider.wait()
