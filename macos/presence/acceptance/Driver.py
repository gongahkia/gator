"""One long-lived AX input controller, no global input events or approval actions."""
from fixture import setup,EVIDENCE
import json,pathlib,subprocess,time,sys,hashlib,os
audit=EVIDENCE/f'input-events-{os.getpid()}.jsonl'
controller=subprocess.Popen(['/tmp/gator-presence-r1-controller'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,text=True,bufsize=1,env=os.environ|{'GATOR_PRESENCE_INPUT_AUDIT':str(audit)})
log=(EVIDENCE/'controlled-input.jsonl').open('a',buffering=1)
metadata=proc=provider=sampler=None; pending=None; finishing=False

def states():
    result=[]
    for p in pathlib.Path(metadata['root']).glob('latest-*.json'):
        try:result.append(json.loads(p.read_text()))
        except (OSError,ValueError):pass
    return result

def identity():
    live=[s for s in states() if s.get('pending_approval')]
    if len(live)!=1:raise RuntimeError('expected exactly one pending Go review')
    state=live[0];p=state['pending_approval'];index=p['message_index']
    return {'conversation':state['conversation_id'],'interaction':p['id'],'messageIndex':index,'tokenSHA256':hashlib.sha256(p['token'].encode()).hexdigest(),'reviewSHA256':hashlib.sha256(json.dumps(state['messages'][index]['result'],sort_keys=True).encode()).hexdigest()}

def check_input():
    if audit.exists():
        for line in audit.read_text().splitlines():
            event=json.loads(line)
            if 'sourcePID' in event and event['sourcePID']!=controller.pid:
                raise RuntimeError('foreign input observed; abort: '+json.dumps(event))

def durable():
    files=list((pathlib.Path(metadata['root'])/'state/gator/interactions').glob('*/*.json'))
    if len(files)!=1:raise RuntimeError('unexpected durable interaction count')
    data=files[0].read_bytes();value=json.loads(data)
    return {'path':str(files[0]),'SHA256':hashlib.sha256(data).hexdigest(),'status':value['status'],'approved':value.get('approved',False)}

def ask(request):
    request['pid']=metadata['pid'];controller.stdin.write(json.dumps(request)+'\n');controller.stdin.flush()
    value=json.loads(controller.stdout.readline());log.write(json.dumps({'request':request,'result':value,'pending':pending,'observedPending':identity() if pending and not finishing else None,'durable':durable() if pending else None})+'\n')
    if not value.get('ok'):raise RuntimeError(value)
    return value

def state():return ask({'action':'state'})

def act(action,window='conversation',target=None,expected=None):
    check_input()
    if pending and (identity()!=pending or durable()['status']!='pending' or durable()['approved']):raise RuntimeError('canonical review changed; abort all input')
    before=state();req={'action':action,'window':window,'expectedWindow':expected or window,'expectedFocus':before['focusedElement']}
    if target:req['target']=target
    value=ask(req)
    # settle native transactions, then verify the same canonical review again.
    time.sleep(.35)
    if pending and not finishing and (identity()!=pending or durable()['status']!='pending' or durable()['approved']):raise RuntimeError('canonical review changed after input; abort')
    return value

def wait(check,seconds=6):
    end=time.monotonic()+seconds
    while time.monotonic()<end:
        value=check()
        if value:return value
        time.sleep(.05)
    raise RuntimeError('readiness timed out')

def new(number,candidate,mode):
    global metadata,proc,provider,sampler,pending,finishing
    if proc and proc.poll() is None:raise RuntimeError('prior fixture still alive')
    if provider:provider.terminate();provider.wait(timeout=3)
    pending=None;finishing=False
    metadata,proc,provider=setup(pathlib.Path(candidate),number,mode)
    sampler=subprocess.Popen(['/tmp/gator-presence-r1-sample',str(proc.pid),'300'],stdout=open(EVIDENCE/f'cycle-{number:02d}-resources.jsonl','w'))
    wait(lambda:bool(states()),10);ask({'action':'activate'});wait(lambda:any(w['id']=='conversation' for w in state()['windows']))
    print(json.dumps({'ready':metadata,'controllerPID':controller.pid}),flush=True)

def prepare():
    global pending
    act('focus');act('press',target='Send message');wait(lambda:any(s.get('pending_approval') for s in states()),15);pending=identity()
    # durable Go interaction must still be pending, with no decision.
    durable=list((pathlib.Path(metadata['root'])/'state/gator/interactions').glob('*/*.json'))
    assert len(durable)==1
    record=json.loads(durable[0].read_text());assert record['status']=='pending' and not record.get('approved')
    (EVIDENCE/f"cycle-{metadata['number']:02d}-pending.json").write_text(json.dumps({'identity':pending,'durable':record,'fileSHA256':hashlib.sha256(durable[0].read_bytes()).hexdigest()},indent=2))
    act('press',target='Open Original Task')
    wait(lambda:any(x.get('conversation_id')==pending['conversation'] and x.get('foreground') for x in states()),4)
    act('focus');act('press',target='An action needs your review')
    return pending

if __name__=='__main__':
    raise SystemExit('Use Batch.py with an uninterrupted, disposable desktop session.')
