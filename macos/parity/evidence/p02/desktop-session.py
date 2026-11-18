# Desktop-only launch of an isolated preference-domain copy.
import json
import pathlib
import plistlib
import subprocess
import sys

metadata = pathlib.Path('/tmp/gator-p02-session.json')
m = json.loads(metadata.read_text()); app = pathlib.Path(m['app'])
assert app == pathlib.Path('/tmp/Gator-Native-Parity-P02-Desktop.app')
assert plistlib.loads((app / 'Contents/Info.plist').read_bytes())['CFBundleIdentifier'] == 'dev.gator.native.p02-disposable'
mode = sys.argv[1]; assert mode in ['light', 'dark']
log = open(pathlib.Path(m['root']) / ('desktop-' + mode + '.log'), 'w')
p = subprocess.Popen([str(app / 'Contents/MacOS/GatorMac'), '--open-window'], env=m['env'] | {'GATOR_NATIVE_TEST_APPEARANCE': mode}, stdout=log, stderr=subprocess.STDOUT)
m['pid'] = p.pid; m['appearance'] = mode; metadata.write_text(json.dumps(m, indent=2))
print(json.dumps({'pid': p.pid, 'appearance': mode, 'profile': m['root']}), flush=True)
print('exit', p.wait(), flush=True)
