#!/usr/bin/python3
# Controlled processes and private filesystem gates only; no Gator tools or network.
import json, os, pathlib, subprocess, sys, threading, time
root = pathlib.Path(os.environ['GATOR_SAFETY_ROOT'])
mode = os.environ['GATOR_SAFETY_MODE']
if mode in ('stdout', 'stderr', 'both'):
    child_code = '''import pathlib,sys,time
root=pathlib.Path(sys.argv[1]); (root/'descendant.ready').touch()
deadline=time.monotonic()+30
while not (root/'release').exists() and time.monotonic()<deadline: time.sleep(.01)
(root/'descendant.done').touch()
'''
    child = subprocess.Popen([sys.executable, '-c', child_code, str(root)], stdin=subprocess.DEVNULL,
                             stdout=None if mode in ('stdout','both') else subprocess.DEVNULL,
                             stderr=None if mode in ('stderr','both') else subprocess.DEVNULL)
    (root/'descendant.pid').write_text(str(child.pid))
    while not (root/'descendant.ready').exists(): time.sleep(.005)
state={'version':1,'type':'state','state':{'conversation_id':'fixture','messages':[],'mode':'chat','running':False,'queued':0}}
print(json.dumps(state),flush=True)
if mode=='stream':
    def stream():
        for i in range(10000):
            value=dict(state); value['state']=dict(state['state'],title=str(i))
            print(json.dumps(value),flush=True); time.sleep(.002)
    threading.Thread(target=stream,daemon=True).start()
for line in sys.stdin:
    frame=json.loads(line)
    if frame['type']=='exit':
        (root/'exit.received').touch()
        if mode=='partial': os.write(1,b'{"version":1,"type":"state","state":')
        if mode=='stderr': os.write(2,b'diagnostic-'+b'x'*6000)
        os._exit(0)
    if frame.get('nonce'): print(json.dumps({'version':1,'type':'action_applied','nonce':frame['nonce']}),flush=True)
