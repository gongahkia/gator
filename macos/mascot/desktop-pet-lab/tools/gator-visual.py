# exact accepted app copied to an inert profile for visual comparison only
import json,pathlib,subprocess,tempfile,plistlib,shutil,os,time
r=pathlib.Path(tempfile.mkdtemp(prefix='gator-native-test-pet-b1-',dir='/tmp'));r.chmod(0o700)
app=r/'Gator.app';shutil.copytree('/tmp/Gator-Native-P04M-2D.app',app)
p=app/'Contents/Info.plist';info=plistlib.loads(p.read_bytes());info['CFBundleIdentifier']='dev.gator.pet.b1.gator-fixture';p.write_bytes(plistlib.dumps(info))
for k in ['gator.notch.enabled','gator.menuBar.enabled','gator.mascot.shown']:subprocess.run(['defaults','write',info['CFBundleIdentifier'],k,'-bool','true'],check=True)
folder=r/'config/gator';folder.mkdir(parents=True,mode=0o700);config=folder/'config.json';config.write_text(json.dumps({'version':4,'defaults':{'provider':'openai','model':'gpt-5.6'},'provider_endpoints':{'openai':'http://127.0.0.1:9/responses'}}));config.chmod(0o600)
env={'PATH':os.environ['PATH'],'HOME':str(r),'GATOR_NATIVE_USER_DATA':str(r/'native'),'GATOR_NATIVE_WORKSPACE':str(r/'workspace'),'GATOR_STATE_DIR':str(r/'state'),'GATOR_CONFIG_DIR':str(r/'config'),'OPENAI_API_KEY':'inert-fixture-no-provider-call','GATOR_NATIVE_TEST_APPEARANCE':'light','GIT_CONFIG_GLOBAL':'/dev/null','GIT_CONFIG_NOSYSTEM':'1'}
p=subprocess.Popen([str(app/'Contents/MacOS/GatorMac'),'--open-window'],env=env,stdout=open(r/'gator.log','w'),stderr=subprocess.STDOUT)
pathlib.Path('/tmp/gator-pet-b1-gator.json').write_text(json.dumps({'pid':p.pid,'root':str(r),'app':str(app)},indent=2));print(p.pid,flush=True);print(p.wait(),flush=True)
