#!/usr/bin/env python3
"""Run fresh-process diagnostic cases sequentially, sampling only their own PID."""
import argparse
import json
from pathlib import Path
import subprocess
import time

parser=argparse.ArgumentParser()
parser.add_argument('--root',required=True)
parser.add_argument('--app',default='/tmp/Gator-Mascot-Final-Attribution-v2.app')
parser.add_argument('cases',nargs='+')
args=parser.parse_args()
root=Path(args.root);root.mkdir(parents=True,exist_ok=True)
for case in args.cases:
    target=root/case
    target.mkdir(exist_ok=False);(target/'home').mkdir()
    mode='ownership' if case.startswith('ownership') else ('policies' if case.startswith('policies') else 'memory')
    policy=case.split('-')[0] if mode=='memory' else 'offscreen'
    env={'HOME':str(target/'home'),'TMPDIR':'/tmp','PATH':'/usr/bin:/bin:/usr/sbin:/sbin','LANG':'en_US.UTF-8'}
    (target/'revision.json').write_bytes((Path(args.app)/'Contents/Resources/revision.json').read_bytes())
    subprocess.run(['open','-n','--stderr',str(target/'console.log'),args.app,'--args','--mode',mode,'--policy',policy,'--output',str(target)],check=True,env=env)
    previous=None;captured=set();deadline=time.monotonic()+210
    while time.monotonic()<deadline:
        report=target/'results.json'
        if report.exists():
            state=json.loads(report.read_text())
            if state['phase']!=previous:
                previous=state['phase'];print(case,previous,state['pid'],flush=True)
            if state['done']:
                errors=[x for x in state['events'] if 'error' in x]
                if errors:raise SystemExit(str(errors))
                print(case,'DONE',state['elapsed'],state['live'],flush=True);break
            # external tooling is separate from the main 20 Hz in-process sampler
            phase=state['phase']
            if case=='retained-1' and phase not in captured and ('07-GPU-completed' in phase or phase.startswith('11-all')):
                captured.add(phase)
                for command in [['footprint','-p',str(state['pid'])],['vmmap','-summary',str(state['pid'])]]:
                    try:
                        result=subprocess.run(command,capture_output=True,text=True,timeout=8)
                        (target/(phase+'-'+command[0]+'.txt')).write_text('exit='+str(result.returncode)+'\n'+result.stdout+'\n'+result.stderr)
                    except subprocess.TimeoutExpired:
                        (target/(phase+'-'+command[0]+'.txt')).write_text('Profiling tool timed out; no settings changed.\n')
        time.sleep(.15)
    else:raise SystemExit('Finite diagnostic deadline exceeded: '+case)
