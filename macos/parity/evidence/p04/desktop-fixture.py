# Acceptance only: inert credentials, private disposable state, identical candidate binaries.
import hashlib,json,os,pathlib,plistlib,shutil,subprocess,tempfile,time,uuid,sys
meta=pathlib.Path('/tmp/gator-p04-desktop.json')
if sys.argv[1]=='setup':
    candidate=pathlib.Path('/tmp/Gator-Native-Parity-P04.app')
    assert candidate.exists()
    root=pathlib.Path(tempfile.mkdtemp(prefix='gator-native-test-p04-desktop-',dir='/tmp'));root.chmod(0o700)
    helper=root/'seed-catalog'
    subprocess.run(['go','build','-o',str(helper),'macos/Tests/GatorCoreTests/Fixtures/native-catalog-seed.go'],check=True)
    ids=json.loads(subprocess.check_output([str(helper),str(root),'3'],text=True))
    directory=root/'config/gator';directory.mkdir(parents=True,mode=0o700)
    config=directory/'config.json';config.write_text(json.dumps({'version':4,'defaults':{'provider':'fixture','model':'fixture'},'custom_providers':[{'id':'fixture','base_url':'http://127.0.0.1:9/chat','models':['fixture'],'default_model':'fixture'}]}));config.chmod(0o600)
    env={'PATH':os.environ['PATH'],'HOME':str(root),'GATOR_NATIVE_USER_DATA':str(root/'native'),'GATOR_NATIVE_WORKSPACE':str(root/'workspace'),'GATOR_CONFIG_DIR':str(root/'config'),'GATOR_STATE_DIR':str(root/'state'),'GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null'}
    archive=root/'native/native/drafts';archive.mkdir(parents=True,mode=0o700)
    file=archive/(hashlib.sha256(str(root/'state').encode()).hexdigest()+'.json')
    file.write_text(json.dumps({'version':1,'selectedConversationID':ids[-1],'drafts':{'conversation:'+ids[-1]:{'text':'P04 unsent chat draft stays here','revision':str(uuid.uuid4()),'updated':time.time()-978307200}}}));file.chmod(0o600)
    app=root/'Gator-P04-Desktop.app';shutil.copytree(candidate,app)
    p=app/'Contents/Info.plist';info=plistlib.loads(p.read_bytes());info['CFBundleIdentifier']='dev.gator.native.p04.disposable';p.write_bytes(plistlib.dumps(info))
    for setting,value in [('gator.notch.enabled','false'),('gator.menuBar.enabled','true')]:subprocess.run(['/usr/bin/defaults','write',info['CFBundleIdentifier'],setting,'-bool',value],check=True)
    hashes={}
    for path in ['Contents/MacOS/GatorMac','Contents/Resources/gator-runtime']:
        digest=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
        assert digest(candidate/path)==digest(app/path);hashes[path]=digest(app/path)
    m={'root':str(root),'ids':ids,'env':env,'app':str(app),'archive':str(file),'bundle':info['CFBundleIdentifier'],'candidate':str(candidate),'hashes':hashes}
    meta.write_text(json.dumps(m,indent=2));print(json.dumps({'root':str(root),'bundle':m['bundle'],'candidate':str(candidate)}))
elif sys.argv[1] in ['light','dark']:
    m=json.loads(meta.read_text());mode=sys.argv[1]
    assert pathlib.Path(m['root']).name.startswith('gator-native-test-p04-desktop-') and m['bundle']=='dev.gator.native.p04.disposable'
    log=open(pathlib.Path(m['root'])/('application-'+mode+'.log'),'w')
    p=subprocess.Popen([str(pathlib.Path(m['app'])/'Contents/MacOS/GatorMac'),'--open-window'],env=m['env']|{'GATOR_NATIVE_TEST_APPEARANCE':mode},stdout=log,stderr=subprocess.STDOUT)
    m['pid']=p.pid;m['mode']=mode;meta.write_text(json.dumps(m,indent=2));print(json.dumps({'pid':p.pid,'mode':mode}),flush=True);print('exit',p.wait(),flush=True)
else:raise SystemExit('setup/light/dark only')
