# Run only a disposable-profile copy; it must have a separate preference domain.
import json, pathlib, plistlib, subprocess, sys
metadata=pathlib.Path('/tmp/gator-uiux-remediation-session.json');m=json.loads(metadata.read_text())
app=pathlib.Path(sys.argv[1]);mode=sys.argv[2]
assert app.name in ['Gator-Native-UIUX-Preview.app','Gator-Native-UIUX-Desktop.app']
plist=app/'Contents/Info.plist';info=plistlib.loads(plist.read_bytes());info['CFBundleIdentifier']='dev.gator.native.uiux-remediation';plist.write_bytes(plistlib.dumps(info))
env=m['env']|{'GATOR_NATIVE_TEST_APPEARANCE':mode}
p=subprocess.Popen([str(app/'Contents/MacOS/GatorMac'),'--open-window'],env=env,stdout=open(pathlib.Path(m['root'])/('remediation-'+mode+'.log'),'w'),stderr=subprocess.STDOUT)
m['desktopPID']=p.pid;m['appearance']=mode;metadata.write_text(json.dumps(m,indent=2));print(json.dumps({'pid':p.pid,'app':str(app),'appearance':mode,'profile':m['root']}),flush=True)
print('desktop_exit',p.wait(),flush=True)
