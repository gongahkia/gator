# Desktop-only acceptance fixture. Use only the packaged candidate and temp state.
import hashlib,json,os,pathlib,plistlib,shutil,subprocess,tempfile,time,uuid,sys
accepted='--accepted' in sys.argv
repo=pathlib.Path.cwd();root=pathlib.Path(tempfile.mkdtemp(prefix='gator-native-test-p03-acceptance-',dir='/tmp'))
helper=root/'seed-catalog'
subprocess.run(['go','build','-o',str(helper),'macos/Tests/GatorCoreTests/Fixtures/native-catalog-seed.go'],check=True)
ids=json.loads(subprocess.check_output([str(helper),str(root),'6'],text=True))
(root/'config/gator').mkdir(parents=True)
(root/'config/gator/config.json').write_text(json.dumps({'version':4,'defaults':{'provider':'fixture','model':'fixture'},'custom_providers':[{'id':'fixture','base_url':'http://127.0.0.1:9/chat','models':['fixture'],'default_model':'fixture'}]}))
a=root/'Workspace A';b=root/'A deliberately long parent folder for readable truncated workspace paths with spaces and café 日本'/'Workspace B'
a.mkdir(mode=0o700);b.mkdir(parents=True,mode=0o700)
env={'PATH':os.environ['PATH'],'HOME':str(root),'GATOR_NATIVE_USER_DATA':str(root/'native'),'GATOR_NATIVE_WORKSPACE':str(root/'workspace'),'GATOR_CONFIG_DIR':str(root/'config'),'GATOR_STATE_DIR':str(root/'state'),'GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null','GATOR_NATIVE_TEST_APPEARANCE':'light'}
archive=root/'native/native/drafts';archive.mkdir(parents=True,mode=0o700)
file=archive/(hashlib.sha256(str(root/'state').encode()).hexdigest()+'.json')
file.write_text(json.dumps({'version':1,'selectedConversationID':ids[-1],'drafts':{'conversation:'+ids[-1]:{'text':'Original acceptance draft stays here','revision':str(uuid.uuid4()),'updated':time.time()-978307200}}}));file.chmod(0o600)
app=pathlib.Path('/tmp/Gator-Native-Parity-P03-Accepted-Desktop.app' if accepted else '/tmp/Gator-Native-Parity-P03-Acceptance-Desktop.app');assert not app.exists(),'Preserve previous fixture bundles'
shutil.copytree('/tmp/Gator-Native-Parity-P03-Accepted.app' if accepted else '/tmp/Gator-Native-Parity-P03.app',app)
p=app/'Contents/Info.plist';info=plistlib.loads(p.read_bytes());info['CFBundleIdentifier']='dev.gator.native.p03.acceptance.fixed' if accepted else 'dev.gator.native.p03.acceptance';p.write_bytes(plistlib.dumps(info))
subprocess.run(['/usr/bin/defaults','write',info['CFBundleIdentifier'],'gator.notch.enabled','-bool','false'],check=True)
subprocess.run(['/usr/bin/defaults','write',info['CFBundleIdentifier'],'gator.menuBar.enabled','-bool','true'],check=True)
m={'root':str(root),'ids':ids,'env':env,'app':str(app),'archive':str(file),'bundle':info['CFBundleIdentifier'],'a':str(a),'b':str(b)}
pathlib.Path('/tmp/gator-p03-acceptance.json').write_text(json.dumps(m,indent=2));print(json.dumps(m,indent=2))
