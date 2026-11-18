# Explicit desktop-only checks. Never press Return unless Find owns focus.
import json
import pathlib
import re
import subprocess
import time

metadata = json.loads(pathlib.Path('/tmp/gator-p01-session.json').read_text())
pid = metadata['pid']
control = ['/tmp/gator-p01-control', str(pid)]
events = []

def call(*args):
    result = subprocess.check_output(control + list(args), text=True)
    events.append(' '.join(args) + ' -> ' + result.strip())
    return result

def keys(script):
    subprocess.run(['osascript', '-e', f'tell application "System Events"\nset frontmost of (first application process whose unix id is {pid}) to true\ntell (first application process whose unix id is {pid})\n{script}\nend tell\nend tell'], check=True)

def wait_for(test):
    for _ in range(40):
        tree = subprocess.check_output(control + ['tree'], text=True)
        if test(tree): return
        time.sleep(.1)
    raise RuntimeError('Expected keyboard outcome not observed')

try:
    if 'Find in loaded transcript' in call('tree'): call('press', 'Close Find')
    call('set', 'gator.composer.text', 'Draft remains unchanged during Find')
    keys('keystroke "f" using command down')
    wait_for(lambda tree: 'Find in loaded transcript' in tree)
    for _ in range(40):
        focus = subprocess.check_output(control + ['inspect', 'Find in loaded transcript'], text=True)
        if 'focused=Optional(1)' in focus: break
        time.sleep(.1)
    assert 'focused=Optional(1)' in focus
    events.append('PASS Command-F focused Find: ' + focus.strip())
    keys('keystroke "a" using command down\nkeystroke "A-050"'); wait_for(lambda tree: '1 of 2' in tree)
    assert 'focused=Optional(1)' in call('inspect', 'Find in loaded transcript')
    keys('key code 36'); wait_for(lambda tree: '2 of 2' in tree)
    call('inspect', 'Retained note A-050'); call('scrollbars')
    keys('key code 36 using shift down'); wait_for(lambda tree: '1 of 2' in tree)
    keys('key code 53'); wait_for(lambda tree: 'Find in loaded transcript' not in tree)
    assert 'Draft remains unchanged during Find' in call('inspect', 'gator.composer.text')
    events.append('PASS typed query, Return next, Shift Return previous, Escape, preserved draft')
    call('scrollbars'); call('press', 'Jump to Latest')
    for _ in range(40):
        values = [float(value) for value in re.findall(r'value=Optional\(([0-9.]+)\)', subprocess.check_output(control + ['scrollbars'], text=True))]
        if values and max(values) >= .98: break
        time.sleep(.1)
    assert values and max(values) >= .98
    events.append('PASS Jump to Latest actual normalized position=' + str(max(values)))
finally:
    # Keep only relevant scoped observations, not the complete application menu.
    events = [event for event in events if not event.startswith('tree ->')]
    pathlib.Path('macos/parity/evidence/p01/keyboard-and-reveal.txt').write_text('\n'.join(events) + '\n')
    print('\n'.join(events), flush=True)
