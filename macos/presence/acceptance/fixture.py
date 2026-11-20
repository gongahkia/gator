"""Disposable real-Go fixture. Seeded draft avoids all keyboard injection."""
import hashlib,json,os,pathlib,plistlib,shutil,subprocess,tempfile,time,uuid
EVIDENCE=pathlib.Path('macos/presence/evidence/p04e-1r').resolve()
def setup(candidate,number,mode='emoji'):
    root=pathlib.Path(tempfile.mkdtemp(prefix='gator-native-test-presence-r1-',dir='/tmp'));root.chmod(0o700)
    seed=pathlib.Path('/tmp/gator-presence-r1-seed')
    if not seed.exists():subprocess.run(['go','build','-o',str(seed),'macos/Tests/GatorCoreTests/Fixtures/native-catalog-seed.go'],check=True)
    ids=json.loads(subprocess.check_output([str(seed),str(root),'2']))
    provider=subprocess.Popen([shutil.which('node'),'macos/mascot/2d/acceptance/provider.mjs'],env={'PATH':os.environ['PATH'],'GATOR_FIXTURE_CONTROL':str(root)},stdout=subprocess.PIPE,stderr=open(root/'provider.log','w'))
    port=json.loads(provider.stdout.readline())['port']
    config=root/'config/gator';config.mkdir(parents=True,mode=0o700)
    p=config/'config.json';p.write_text(json.dumps({'version':4,'defaults':{'provider':'openai','model':'gpt-5.6'},'provider_endpoints':{'openai':f'http://127.0.0.1:{port}/responses'}}));p.chmod(0o600)
    bundle='dev.gator.presence.r1';app=root/'PresenceClosure.app';shutil.copytree(candidate,app)
    p=app/'Contents/Info.plist';info=plistlib.loads(p.read_bytes());info.update(CFBundleIdentifier=bundle,CFBundleName='PresenceClosure',CFBundleDisplayName='PresenceClosure');p.write_bytes(plistlib.dumps(info))
    subprocess.run(['defaults','delete',bundle],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    for k,v in [('gator.notch.enabled','false'),('gator.menuBar.enabled','true'),('gator.mascot.shown','true'),('gator.mascot.animated','true'),('gator.desktopPet.shown','false')]:subprocess.run(['defaults','write',bundle,k,'-bool',v],check=True)
    subprocess.run(['defaults','write',bundle,'gator.presence.mode','-string',mode],check=True)
    archive=root/'native/native/drafts';archive.mkdir(parents=True,mode=0o700)
    p=archive/(hashlib.sha256(str(root/'state').encode()).hexdigest()+'.json');p.write_text(json.dumps({'version':1,'selectedConversationID':ids[-1],'drafts':{'conversation:'+ids[-1]:{'text':'Review mascot','revision':str(uuid.uuid4()),'updated':time.time()-978307200}}}));p.chmod(0o600)
    wrapper=root/'runtime';source=pathlib.Path('macos/Tests/GatorCoreTests/Fixtures/dev-acceptance-runtime.py').read_text().replace('import hashlib','import hashlib\nimport time').replace("output.write(json.dumps(value)","value['epoch'] = time.time()\n        value['wrapperPID'] = owner\n        output.write(json.dumps(value)")
    wrapper.write_text('#!/usr/bin/env python3\n'+source);wrapper.chmod(0o700)
    env={'PATH':os.path.dirname(shutil.which('node'))+':'+os.environ['PATH'],'HOME':str(root),'GATOR_NATIVE_USER_DATA':str(root/'native'),'GATOR_NATIVE_WORKSPACE':str(root/'workspace'),'GATOR_CONFIG_DIR':str(root/'config'),'GATOR_STATE_DIR':str(root/'state'),'OPENAI_API_KEY':'fixture-only-inert-presence-closure','GATOR_NATIVE_RUNTIME':str(wrapper),'GATOR_FIXTURE_CONTROL':str(root),'GATOR_FIXTURE_GO_RUNTIME':str(app/'Contents/Resources/gator-runtime'),'GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null','GATOR_NATIVE_TEST_APPEARANCE':'light'}
    proc=subprocess.Popen([str(app/'Contents/MacOS/GatorMac'),'--open-window'],env=env,stdout=open(root/'app.log','w'),stderr=subprocess.STDOUT)
    metadata={'number':number,'candidate':str(candidate),'root':str(root),'app':str(app),'bundle':bundle,'pid':proc.pid,'providerPID':provider.pid,'ids':ids,'mode':mode,'binarySHA256':hashlib.sha256((app/'Contents/MacOS/GatorMac').read_bytes()).hexdigest(),'GoSHA256':hashlib.sha256((app/'Contents/Resources/gator-runtime').read_bytes()).hexdigest()}
    (EVIDENCE/f'cycle-{number:02d}-fixture.json').write_text(json.dumps(metadata,indent=2))
    (root/'private-launch.json').write_text(json.dumps(env));(root/'private-launch.json').chmod(0o600)
    return metadata,proc,provider
if __name__=='__main__':
    import sys
    m,_,_=setup(pathlib.Path(sys.argv[1]),int(sys.argv[2]),sys.argv[3] if len(sys.argv)>3 else 'emoji')
    print(json.dumps(m))
