#!/usr/bin/env python3
"""Compare isolated Linux application process trees with the same retained chat.
Requires a built terminal artifact, desktop dependencies/build, Xvfb and Node.
Reports observations, not pass/fail performance budgets. Never uses live profiles.
"""
import argparse
import datetime
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import shutil
import signal
import struct
import subprocess
import tempfile
import termios
import time

parser = argparse.ArgumentParser()
parser.add_argument("artifact", type=Path)
parser.add_argument("--revisions", type=int, default=150)
args = parser.parse_args()
assert os.uname().sysname == "Linux", "This sampler uses Linux /proc PSS."
assert 0 <= args.revisions <= 500
repo = Path(__file__).resolve().parents[2]
artifact = args.artifact.resolve()


def tree(root_pid):
    processes = {}
    for entry in Path('/proc').iterdir():
        if not entry.name.isdigit():
            continue
        try:
            fields = (entry / 'stat').read_text().rsplit(')', 1)[1].split()
            if fields[0] != 'Z':
                processes[int(entry.name)] = int(fields[1])
        except (OSError, ValueError):
            pass
    owned = {root_pid} if root_pid in processes else set()
    while True:
        children = {pid for pid, parent in processes.items() if parent in owned}
        if children <= owned:
            return owned
        owned |= children


def memory(pids):
    rss = pss = 0
    for pid in pids:
        try:
            values = dict(line.split(':', 1) for line in Path(f'/proc/{pid}/smaps_rollup').read_text().splitlines() if ':' in line)
            rss += int(values['Rss'].split()[0]) * 1024
            pss += int(values['Pss'].split()[0]) * 1024
        except (OSError, KeyError, ValueError):
            pass
    return rss, pss


def seed(root):
    (root / 'workspace').mkdir()
    if not args.revisions:
        return
    directory = root / 'state/gator/conversations/work-performance'
    (directory / 'revisions').mkdir(parents=True)
    at = datetime.datetime.now(datetime.timezone.utc).isoformat()
    (directory.parent / 'active.json').write_text(json.dumps({'version': 1, 'conversation_id': 'work-performance'}))
    (directory / 'conversation.json').write_text(json.dumps({'version': 2, 'id': 'work-performance', 'title': 'Retained performance fixture', 'source_path': str(root / 'workspace'), 'snapshot_id': 'snap-performance', 'head_revision_id': f'rev-{args.revisions - 1}', 'created_at': at, 'updated_at': at}))
    for index in range(args.revisions):
        (directory / f'revisions/rev-{index}.json').write_text(json.dumps({'version': 2, 'id': f'rev-{index}', 'conversation_id': 'work-performance', 'parent_revision_id': f'rev-{index - 1}' if index else '', 'snapshot_id': 'snap-performance', 'objective': f'Question {index}: retain this project context.', 'bundle_path': str(root / 'missing-result' / str(index)), 'status': 'completed', 'final_text': f'Answer {index}: Benchmark needle {index}.\nSurrounding retained project detail stays readable.', 'created_at': at}))


def measure(frontend):
    with tempfile.TemporaryDirectory(prefix=f'gator-measure-{frontend}-') as temporary:
        root = Path(temporary)
        seed(root)
        environment = dict(os.environ, GATOR_STATE_DIR=str(root / 'state'), GATOR_CONFIG_DIR=str(root / 'config'), GATOR_DESKTOP_USER_DATA=str(root / 'app'), GATOR_TERMINAL_USER_DATA=str(root / 'app'), GATOR_DESKTOP_WORKSPACE=str(root / 'workspace'), GATOR_TERMINAL_WORKSPACE=str(root / 'workspace'), GATOR_DESKTOP_BINARY=str(artifact / 'runtime/gator-runtime'))
        output = bytearray()
        pids = set()
        peak_rss = peak_pss = peak_processes = 0
        master = None
        start = time.monotonic()
        if frontend == 'terminal':
            master, slave = pty.openpty()
            original = termios.tcgetattr(slave)
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 36, 110, 0, 0))
            child = subprocess.Popen([str(artifact / 'bin/gator')], cwd=root, env=environment, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
            os.close(slave)
            product_pid = child.pid
        else:
            driver = root / 'desktop-driver.mjs'
            driver.write_text('''import { createRequire } from 'node:module';
import { writeFile, access, rename } from 'node:fs/promises';
import { performance } from 'node:perf_hooks';
const [repo, root, count] = process.argv.slice(2), require=createRequire(repo+'/desktop/package.json');
const { _electron: electron }=require('@playwright/test');
const publish=async(name,value)=>{await writeFile(root+'/'+name+'.tmp',value);await rename(root+'/'+name+'.tmp',root+'/'+name)};
const start=performance.now(), app=await electron.launch({args:[repo+'/desktop'],env:process.env});
try {
 await publish('pid',String(app.process().pid));
 const page=await app.firstWindow(); await page.getByRole('textbox',{name:'Message Gator',exact:true}).waitFor();
 if(Number(count)) await page.getByText(`Answer ${Number(count)-1}: Benchmark needle ${Number(count)-1}.`,{exact:false}).waitFor();
 await publish('ready',JSON.stringify({ready_ms:performance.now()-start,rendered_messages:await page.locator('.message').count()}));
 const interaction=performance.now(); await page.keyboard.press('Control+f');
 await page.getByRole('textbox',{name:'Search conversation'}).fill(Number(count)?`Benchmark needle ${Math.floor(Number(count)/2)}.`:'not present');
 await page.waitForFunction(()=>!!document.querySelector('.search-count')?.textContent?.trim());
 await publish('interaction',String(performance.now()-interaction));
 while(!await access(root+'/stop').then(()=>true,()=>false)) await new Promise(resolve=>setTimeout(resolve,25));
} finally {await app.close()}
''')
            log = (root / 'driver.log').open('w')
            child = subprocess.Popen(['xvfb-run', '-a', 'node', str(driver), str(repo), str(root), str(args.revisions)], env=environment, stdout=log, stderr=log, start_new_session=True)
            product_pid = None

        def poll():
            nonlocal product_pid, peak_rss, peak_pss, peak_processes
            if product_pid is None and (root / 'pid').exists():
                product_pid = int((root / 'pid').read_text())
            current = tree(product_pid) if product_pid else set()
            pids.update(current)
            rss, pss = memory(current)
            peak_rss, peak_pss, peak_processes = max(peak_rss, rss), max(peak_pss, pss), max(peak_processes, len(current))
            if master is not None and select.select([master], [], [], 0.02)[0]:
                try:
                    output.extend(os.read(master, 65536))
                except OSError:
                    pass
            else:
                time.sleep(0.02)

        def wait(predicate, label):
            deadline = time.monotonic() + 30
            while not predicate():
                poll()
                if child.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError(f'{frontend} {label}: {output[-2000:]!r} ' + ((root / 'driver.log').read_text()[-3000:] if frontend == 'desktop' else ''))

        try:
            if frontend == 'terminal':
                wait(lambda: b'connected' in output and (not args.revisions or f'Answer {args.revisions - 1}:'.encode() in output), 'ready')
                ready_ms = (time.monotonic() - start) * 1000
                output.clear(); interaction = time.monotonic(); os.write(master, b'\x06')
                wait(lambda: b'Search this transcript' in output, 'search prompt')
                query = f'Benchmark needle {args.revisions // 2}.' if args.revisions else 'not present'
                os.write(master, query.encode() + b'\r'); output.clear()
                wait(lambda: b'Transcript' in output and b'matches' in output, 'search results')
                if args.revisions:
                    wait(lambda: f'Answer {args.revisions // 2}:'.encode() in output, 'retained match')
                interaction_ms = (time.monotonic() - interaction) * 1000
                os.write(master, b'\x1b')
            else:
                wait(lambda: (root / 'ready').exists(), 'ready')
                ready_ms = json.loads((root / 'ready').read_text())['ready_ms']
                wait(lambda: (root / 'interaction').exists(), 'search')
                interaction_ms = float((root / 'interaction').read_text())
            end = time.monotonic() + 1
            while time.monotonic() < end:
                poll()
            ready_rss, ready_pss = memory(tree(product_pid))
            if frontend == 'terminal':
                os.write(master, b'\x11')
            else:
                (root / 'stop').touch()
            deadline = time.monotonic() + 10
            while child.poll() is None and time.monotonic() < deadline:
                poll()
            assert child.poll() == 0, f'{frontend} did not exit cleanly'
            if frontend == 'terminal':
                assert termios.tcgetattr(master) == original
            time.sleep(0.2)
            survivors = sorted(pid for pid in pids if tree(pid))
            assert not survivors, f'Product process survivors: {survivors}'
            return dict(frontend=frontend, retained_revisions=args.revisions, ready_ms=round(ready_ms,1), search_ms=round(interaction_ms,1), ready_tree_rss_bytes=ready_rss, ready_tree_pss_bytes=ready_pss, sampled_peak_tree_rss_bytes=peak_rss, sampled_peak_tree_pss_bytes=peak_pss, max_product_processes=peak_processes, cleanup='pass')
        finally:
            if child.poll() is None:
                os.killpg(child.pid, signal.SIGKILL); child.wait()
            for pid in pids:
                try:
                    # A recorded PID can be recycled after exit. Only clean up
                    # a process still carrying this isolated probe's profile.
                    marker = f'GATOR_STATE_DIR={root / "state"}'.encode()
                    if marker in Path(f'/proc/{pid}/environ').read_bytes().split(b'\0'):
                        os.kill(pid, signal.SIGKILL)
                except OSError:
                    pass
            if master is not None:
                os.close(master)


for frontend in ['terminal', 'desktop']:
    print(json.dumps(measure(frontend)), flush=True)
