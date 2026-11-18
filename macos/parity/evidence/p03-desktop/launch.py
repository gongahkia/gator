import json,pathlib,plistlib,subprocess,sys
p=pathlib.Path('/tmp/gator-p03-acceptance.json');m=json.loads(p.read_text());app=pathlib.Path(m['app'])
assert str(app) in ['/tmp/Gator-Native-Parity-P03-Acceptance-Desktop.app','/tmp/Gator-Native-Parity-P03-Accepted-Desktop.app']
assert plistlib.loads((app/'Contents/Info.plist').read_bytes())['CFBundleIdentifier']==m['bundle'] and m['bundle'] in ['dev.gator.native.p03.acceptance','dev.gator.native.p03.acceptance.fixed']
mode=sys.argv[1] if len(sys.argv)>1 else 'light';assert mode in ['light','dark']
log=open(pathlib.Path(m['root'])/('acceptance-'+mode+'.log'),'w')
process=subprocess.Popen([str(app/'Contents/MacOS/GatorMac'),'--open-window'],env=m['env']|{'GATOR_NATIVE_TEST_APPEARANCE':mode},stdout=log,stderr=subprocess.STDOUT)
m['pid']=process.pid;m['mode']=mode;p.write_text(json.dumps(m,indent=2));print(json.dumps({'pid':process.pid,'root':m['root']}),flush=True);print('exit',process.wait(),flush=True)
