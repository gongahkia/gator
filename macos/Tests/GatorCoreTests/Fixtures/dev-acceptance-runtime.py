#!/usr/bin/env python3
"""Transparent fixture-only observation of the actual bundled Go bridge."""
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import threading

root = Path(os.environ['GATOR_FIXTURE_CONTROL'])
runtime = os.environ['GATOR_FIXTURE_GO_RUNTIME']
child = subprocess.Popen([runtime, *sys.argv[1:]], stdin=subprocess.PIPE, stdout=subprocess.PIPE)
owner = os.getpid()
trace = root / f'bridge-{owner}.jsonl'
lock = threading.Lock()


def digest(value):
    return hashlib.sha256(value.encode()).hexdigest() if value else None


def record(value):
    with lock, trace.open('a') as output:
        output.write(json.dumps(value) + '\n')


def commands():
    for line in sys.stdin.buffer:
        try:
            frame = json.loads(line)
            record({'direction': 'command', 'type': frame.get('type'), 'frameKeys': list(frame),
                    'interaction': frame.get('interaction_id'), 'tokenSHA256': digest(frame.get('approval_token')),
                    'approved': frame.get('approved')})
        except (ValueError, TypeError):
            record({'direction': 'command', 'parse': 'unrecognized'})
        try:
            child.stdin.write(line)
            child.stdin.flush()
        except BrokenPipeError:
            return
    child.stdin.close()


def snapshots():
    for line in child.stdout:
        try:
            event = json.loads(line)
            if event.get('type') == 'state':
                state = event['state']
                pending = state.get('pending_approval') or {}
                summary = {'direction': 'state', 'conversation': state.get('conversation_id'),
                           'running': state.get('running'), 'foreground': state.get('foreground'),
                           'interaction': pending.get('id'), 'tokenSHA256': digest(pending.get('token')),
                           'messageIndex': pending.get('message_index'), 'dialogueActive': state.get('dialogue_active')}
                if pending and pending.get('message_index', -1) >= 0:
                    result = state['messages'][pending['message_index']].get('result', {})
                    summary['reviewSHA256'] = digest(json.dumps(result, sort_keys=True))
                record(summary)
                temporary = root / f'latest-{owner}.tmp'
                temporary.write_text(json.dumps(state))
                temporary.chmod(0o600)
                temporary.replace(root / f'latest-{owner}.json')
        except (ValueError, TypeError, KeyError, IndexError):
            record({'direction': 'state', 'parse': 'unrecognized'})
        sys.stdout.buffer.write(line)
        sys.stdout.buffer.flush()


record({'direction': 'launch', 'runtime': runtime, 'pid': child.pid, 'requestedConversation': sys.argv[2] if len(sys.argv) > 2 else ''})
signal.signal(signal.SIGTERM, lambda *_: child.terminate())
threading.Thread(target=commands, daemon=True).start()
output = threading.Thread(target=snapshots)
output.start()
status = child.wait()
output.join(timeout=2)
record({'direction': 'exit', 'status': status})
sys.exit(status)
