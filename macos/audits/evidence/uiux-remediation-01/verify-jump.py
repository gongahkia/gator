# Explicit desktop regression only; never part of routine headless verification.
import json, pathlib, re, subprocess, sys, time
pid = str(int(sys.argv[1])); tool = sys.argv[2]
metadata = json.loads(pathlib.Path('/tmp/gator-uiux-remediation-session.json').read_text())
assert int(pid) == metadata['desktopPID'], 'Only the current disposable session is allowed'
def call(*args):return subprocess.check_output([tool,pid,*args],text=True)
def offset():
    rows=[]
    for line in call('scrollbars').splitlines():
        value=re.search(r'value=Optional\(([0-9.eE+-]+)\)',line)
        height=re.search(r'size=\([0-9.]+, ([0-9.]+)\)',line)
        if value and height:rows.append((float(height[1]),float(value[1])))
    assert rows, 'No native scrollbar was observed'
    return max(rows)[1]
subprocess.run(['osascript','-e',f'tell application "System Events" to set frontmost of (first application process whose unix id is {pid}) to true'],check=True)
# the fixture window is positioned at (325, 125); this point is in its transcript.
call('scroll','520','270','1800')
end=time.monotonic()+8
while 'Jump to Latest' not in call('tree'):
    if time.monotonic()>end:raise RuntimeError('Older reading position was not established')
    time.sleep(.05)
before=offset();assert before < .95, before
call('press','Jump to Latest')
end=time.monotonic()+8
while True:
    after=offset()
    if after>=.98:break
    if time.monotonic()>end:raise RuntimeError(f'Jump did not reach latest: {before} -> {after}')
    time.sleep(.05)
assert 'Jump to Latest' not in call('tree')
pathlib.Path('macos/audits/evidence/uiux-remediation-01/jump-to-latest.txt').write_text(f'Explicit native Jump to Latest\nnormalized scroll position before={before}\nnormalized scroll position after={after}\n'+call('heads'))
print(f'PASS: actual native scroll position {before} -> {after}')
