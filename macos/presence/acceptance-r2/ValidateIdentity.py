"""No desktop input: validate exact executable/bundle/PID/hash refusals."""
import hashlib,json,os,pathlib,plistlib,subprocess,tempfile,time
out=pathlib.Path('macos/presence/evidence/p04e-1r2');root=pathlib.Path(tempfile.mkdtemp(prefix='gator-native-test-presence-r2-identity-',dir='/tmp'));root.chmod(0o700)
app=root/'Identity.app';binary=app/'Contents/MacOS/GatorMac';binary.parent.mkdir(parents=True)
subprocess.run(['swiftc','-swift-version','6','-strict-concurrency=complete','-warnings-as-errors','-target','arm64-apple-macos15.0','macos/presence/acceptance-r2/IdentityHost.swift','-o',str(binary)],check=True)
(app/'Contents/Info.plist').write_bytes(plistlib.dumps({'CFBundleExecutable':'GatorMac','CFBundleIdentifier':'dev.gator.presence.r2.identity','CFBundleName':'HeadlessIdentity','CFBundlePackageType':'APPL','LSUIElement':True,'LSMinimumSystemVersion':'15.0'}))
p=subprocess.Popen([str(binary)],stdout=open(root/'host.log','w'),stderr=subprocess.STDOUT)
controller=subprocess.Popen(['/tmp/gator-presence-r2-controller'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True)
base={'pid':p.pid,'app':str(app),'bundle':'dev.gator.presence.r2.identity','binarySHA256':hashlib.sha256(binary.read_bytes()).hexdigest()};records=[]
def request(label,changes):
 m=base|changes;file=root/(label+'.json');file.write_text(json.dumps(m));file.chmod(0o600)
 command={'pid':p.pid,'manifest':str(file),'action':'state'};controller.stdin.write(json.dumps(command)+'\n');controller.stdin.flush();result=json.loads(controller.stdout.readline());records.append({'case':label,'result':result});return result
try:
 deadline=time.monotonic()+3
 while True:
  ready=request('startup-readiness',{})
  if ready['ok']:break
  if time.monotonic()>deadline:raise RuntimeError('Headless host did not register a valid identity')
  time.sleep(.05)
 assert request('valid',{})['ok']
 for label,changes in [('wrong-bundle',{'bundle':'dev.gator.unrelated'}),('wrong-executable',{'app':str(root/'Other.app')}),('wrong-hash',{'binarySHA256':'0'*64}),('wrong-manifest-pid',{'pid':p.pid+1})]:assert not request(label,changes)['ok']
 assert request('valid-after-refusals',{})['ok']
 (out/'harness-identity-validation.json').write_text(json.dumps({'controllerPID':controller.pid,'hostPID':p.pid,'headlessHost':True,'GoOwners':0,'desktopInput':False,'cases':records},indent=2))
 print('headless identity checks passed',len(records))
finally:
 (out/'harness-identity-validation.json').write_text(json.dumps({'controllerPID':controller.pid,'hostPID':p.pid,'headlessHost':True,'GoOwners':0,'desktopInput':False,'cases':records},indent=2))
 controller.stdin.close();controller.wait(timeout=3);p.terminate();p.wait(timeout=3)
