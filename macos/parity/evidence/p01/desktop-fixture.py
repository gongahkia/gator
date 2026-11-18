# Explicit desktop fixture only. Seeds retained public dialogue without a model.
import hashlib
import json
import os
import pathlib
import subprocess
import tempfile

root = pathlib.Path(tempfile.mkdtemp(prefix='gator-p01-', dir='/tmp'))
ids = json.loads(subprocess.check_output(['go', 'run', 'macos/parity/evidence/p01/seed-transcripts.go', str(root)], text=True))
(root / 'config/gator').mkdir(parents=True)
(root / 'config/gator/config.json').write_text(json.dumps({
    'version': 4, 'defaults': {'provider': 'fixture', 'model': 'fixture'},
    'custom_providers': [{'id': 'fixture', 'base_url': 'http://127.0.0.1:9/chat', 'models': ['fixture'], 'default_model': 'fixture'}]
}))
env = {'PATH': os.environ['PATH'], 'HOME': str(root), 'GATOR_NATIVE_USER_DATA': str(root / 'native'),
       'GATOR_NATIVE_WORKSPACE': str(root / 'workspace'), 'GATOR_CONFIG_DIR': str(root / 'config'),
       'GATOR_STATE_DIR': str(root / 'state'), 'GATOR_NATIVE_TEST_APPEARANCE': 'light',
       'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'}
archive = root / 'native/native/drafts'
archive.mkdir(parents=True, mode=0o700)
path = archive / (hashlib.sha256(str(root / 'state').encode()).hexdigest() + '.json')
path.write_text(json.dumps({'version': 1, 'selectedConversationID': ids[0], 'drafts': {}}))
path.chmod(0o600)
metadata = {'root': str(root), 'conversations': ids, 'env': env, 'app': '/tmp/Gator-Native-Parity-P01-Desktop.app', 'bundle': 'dev.gator.native.p01-disposable'}
pathlib.Path('/tmp/gator-p01-session.json').write_text(json.dumps(metadata, indent=2))
print(json.dumps({'root': str(root), 'conversations': ids}))
