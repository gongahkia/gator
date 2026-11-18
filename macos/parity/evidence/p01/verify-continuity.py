# Desktop-only regression. The controller itself rejects other bundle IDs.
import json
import pathlib
import re
import subprocess
import time

metadata = json.loads(pathlib.Path('/tmp/gator-p01-session.json').read_text())
control = ['/tmp/gator-p01-control', str(metadata['pid'])]
events = []

def call(*args):
    value = subprocess.check_output(control + list(args), text=True)
    events.append(' '.join(args) + ' -> ' + value.strip())
    return value

def wait_for(test):
    for _ in range(80):
        tree = subprocess.check_output(control + ['tree'], text=True)
        if test(tree): return tree
        time.sleep(.1)
    raise RuntimeError('Expected UI condition not observed')

def visible(title):
    for _ in range(80):
        text = subprocess.check_output(control + ['heads'], text=True)
        row = next((row for row in text.splitlines() if row.startswith(title + ' at')), None)
        if row:
            y = float(re.search(r'at \([^,]+, ([^)]+)\)', row)[1])
            if 175 <= y <= 650:
                events.append('VISIBLE ' + row)
                return
        time.sleep(.1)
    raise RuntimeError('Expected visible anchor: ' + title + '; last headings=' + text)

def switch(index, label):
    call('press', 'gator.recent-conversations')
    call('press', 'gator.recent.' + metadata['conversations'][index])
    wait_for(lambda tree: 'History question ' + label + '-' in tree and 'Find in loaded transcript' not in tree)

def find(query):
    call('press', 'Find in Current Transcript')
    call('set', 'Find in loaded transcript', query)
    wait_for(lambda tree: '1 of 2' in tree)
    call('press', 'Next match')

try:
    switch(0, 'A')
    call('set', 'gator.composer.text', 'Draft remains unchanged during Find')
    find('A-050'); visible('Retained note A-050')
    call('press', 'Close Find'); visible('Retained note A-050'); call('scrollbars')
    switch(1, 'B')
    call('set', 'gator.composer.text', 'Independent B draft')
    find('B-012'); visible('Retained note B-012')
    children = subprocess.check_output(['pgrep', '-P', str(metadata['pid'])], text=True)
    events.append('Direct Go children=' + children.strip())
    switch(0, 'A'); visible('Retained note A-050'); call('scrollbars')
    assert 'Draft remains unchanged during Find' in call('inspect', 'gator.composer.text')
    switch(1, 'B'); visible('Retained note B-012')
    assert 'Independent B draft' in call('inspect', 'gator.composer.text')
    assert subprocess.check_output(['pgrep', '-P', str(metadata['pid'])], text=True) == children
    events.append('PASS: independent drafts/anchors; Find scope closes on switch; same two Go owners reused')
    switch(0, 'A'); visible('Retained note A-050')
    call('press', 'Find in Current Transcript'); call('set', 'Find in loaded transcript', 'A-001')
    wait_for(lambda tree: 'No matching loaded text.' in tree)
    events.append('PASS: A-001 retained but outside latest-100-turn projection is not searchable')
    call('set', 'Find in loaded transcript', 'A-050'); wait_for(lambda tree: '1 of 2' in tree)
    call('press', 'Results…')
finally:
    pathlib.Path('macos/parity/evidence/p01/conversation-continuity.txt').write_text('\n'.join(events) + '\n')
    print('\n'.join(events), flush=True)
