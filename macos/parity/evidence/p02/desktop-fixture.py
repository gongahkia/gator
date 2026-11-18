# Explicit disposable desktop fixture. Never reads a live Gator profile.
import hashlib
import json
import os
import pathlib
import plistlib
import shutil
import subprocess
import tempfile

root = pathlib.Path(tempfile.mkdtemp(prefix='gator-p02-', dir='/tmp'))
helper = root / 'seed-catalog'
subprocess.run(['go', 'build', '-o', str(helper), 'macos/Tests/GatorCoreTests/Fixtures/native-catalog-seed.go'], check=True)
ids = json.loads(subprocess.check_output([str(helper), str(root), '310'], text=True))
(root / 'config/gator').mkdir(parents=True)
(root / 'config/gator/config.json').write_text(json.dumps({
    'version': 4, 'defaults': {'provider': 'fixture', 'model': 'fixture'},
    'custom_providers': [{'id': 'fixture', 'base_url': 'http://127.0.0.1:9/chat', 'models': ['fixture'], 'default_model': 'fixture'}]
}))
env = {'PATH': os.environ['PATH'], 'HOME': str(root), 'GATOR_NATIVE_USER_DATA': str(root / 'native'),
       'GATOR_NATIVE_WORKSPACE': str(root / 'workspace'), 'GATOR_CONFIG_DIR': str(root / 'config'),
       'GATOR_STATE_DIR': str(root / 'state'), 'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'}
archive = root / 'native/native/drafts'; archive.mkdir(parents=True, mode=0o700)
path = archive / (hashlib.sha256(str(root / 'state').encode()).hexdigest() + '.json')
path.write_text(json.dumps({'version': 1, 'selectedConversationID': ids[-1], 'drafts': {}})); path.chmod(0o600)
app = pathlib.Path('/tmp/Gator-Native-Parity-P02-Desktop.app')
assert not app.exists(), 'Use a fresh destination; preserve the existing desktop evidence bundle.'
shutil.copytree('/tmp/Gator-Native-Parity-P02.app', app)
plist = app / 'Contents/Info.plist'; info = plistlib.loads(plist.read_bytes())
info['CFBundleIdentifier'] = 'dev.gator.native.p02-disposable'; plist.write_bytes(plistlib.dumps(info))
metadata = {'root': str(root), 'ids': ids, 'helper': str(helper), 'env': env, 'app': str(app), 'bundle': info['CFBundleIdentifier']}
pathlib.Path('/tmp/gator-p02-session.json').write_text(json.dumps(metadata, indent=2))
print(json.dumps({'root': str(root), 'records': len(ids), 'older': ids[5]}))
