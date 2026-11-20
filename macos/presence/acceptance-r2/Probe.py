"""Short prospective native baseline; only the setup case can press seeded Send."""
import Driver as d
import json,time,subprocess,pathlib,os,signal
settings='com_apple_SwiftUI_Settings_window'
phases=[];extraSamplers=[]
def measure(name,seconds=7,sample=False):
    start=time.time();before=d.state();native=d.native()
    if sample:
        with open(d.EVIDENCE/f'probe-{name}-sample-command.log','w') as f:subprocess.run(['sample',str(d.proc.pid),'1','-file',str(d.EVIDENCE/f'probe-{name}-sample.txt')],stdout=f,stderr=subprocess.STDOUT,timeout=10)
    until=time.monotonic()+seconds
    while time.monotonic()<until:d.check_input();time.sleep(.1)
    row={'phase':name,'start':start,'end':time.time(),'native':native,'state':before}
    phases.append(row);(d.EVIDENCE/'probe-phases.json').write_text(json.dumps(phases,indent=2));print(json.dumps({'phase':name,'native':native}),flush=True)
def open_settings():
    d.act('press','menu','Conversation','conversation');d.act('press','menu','Agent Presence…','conversation')
    d.wait(lambda:any(w['id']==settings for w in d.state()['windows']))
def select(value):d.act('press',settings,'gator.presence.mode');d.act('press',settings,value)
def stop():
    d.check_input();d.act('focus');d.act('quit');d.wait(lambda:any(w.get('description')=='alert' for w in d.state()['windows']))
    a=next(w['id'] for w in d.state()['windows'] if w.get('description')=='alert')
    d.finishing=True;d.act('press',a,'Quit and Keep Retained Work');d.proc.wait(timeout=12);d.sampler.wait(timeout=3)
    print('guarded Quit exit',d.proc.returncode,flush=True)
try:
    d.new(21,'/tmp/Gator-Native-P04E1R2-Diagnostic.app','off')
    d.arm_input()
    # adversarial harness checks must refuse before any action.
    for bad in [dict(action='press',window='conversation',target='Approve Exact Action',role='AXButton'),dict(action='press',window='conversation',target='Send message',role='AXButton'),dict(action='press',window='conversation',target='Agent Presence…',role='AXMenuItem',preflightID='stale')]:
        state=d.state();bad.setdefault('preflightID',state['preflightID']);bad['expectedWindow']='conversation';bad['expectedFocus']=state['focusedElement']
        try:d.ask(bad);raise AssertionError('unsafe action accepted')
        except RuntimeError as e:print('expected refusal',str(e),flush=True)
    measure('idle-off')
    d.prepare()
    for f in pathlib.Path(d.metadata['root']).glob('bridge-*.jsonl'):
        for line in f.read_text().splitlines():
            event=json.loads(line)
            if event.get('direction')=='launch':extraSamplers.append(subprocess.Popen(['/tmp/gator-presence-r2-sample',str(event['pid']),'120'],stdout=open(d.EVIDENCE/f"probe-Go-{event['pid']}.jsonl",'w')))
    d.act('press',target='Back to Conversation');measure('pending-off',sample=True)
    d.act('press',target='An action needs your review');measure('review-off',sample=True)
    open_settings();measure('settings-off')
    select('Emoji only');measure('review-emoji')
    select('Emoji + GIF reactions');measure('review-gif-settled')
    d.act('press',settings,'gator.desktopPet.shown');measure('review-desktop')
    d.act('press',settings,'gator.desktopPet.tuck');d.act('close',settings);measure('closed-settings',sample=True)
    stop()
    (d.EVIDENCE/'probe-result.json').write_text(json.dumps({'exit':d.proc.returncode,'pending':d.pending,'durable':d.durable(),'inputAudit':str(d.audit),'controllerPID':d.controller.pid},indent=2))
except Exception as e:
    (d.EVIDENCE/'probe-abort.json').write_text(json.dumps({'error':str(e),'metadata':d.metadata},indent=2));print('ABORT',e,flush=True)
    # stay available for explicit guarded cleanup; no more acceptance inputs.
    while d.proc and d.proc.poll() is None:time.sleep(.5)
    raise
finally:
    if d.provider:d.provider.terminate();d.provider.wait(timeout=3)
    d.controller.stdin.close();d.controller.wait(timeout=3)
