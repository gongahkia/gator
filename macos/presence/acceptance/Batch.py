"""Ordinary native actions with transaction settling; stop on any safety mismatch."""
import json,pathlib,subprocess,time,os,sys
import datetime
# a rerun must never overwrite the original acceptance evidence.
run_directory=pathlib.Path(os.environ.get("GATOR_PRESENCE_EVIDENCE_DIRECTORY", "macos/presence/evidence/p04e-1r/desktop-run-"+datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S")+"-"+str(os.getpid())))
run_directory.mkdir(parents=True,exist_ok=False)
os.environ["GATOR_PRESENCE_EVIDENCE_DIRECTORY"]=str(run_directory.resolve())
import Driver as d
settings='com_apple_SwiftUI_Settings_window'
phases=[]
def phase(name):
    row={'epoch':time.time(),'cycle':d.metadata['number'],'phase':name,'state':d.state(),'pending':d.pending,'durable':d.durable() if d.pending else None}
    phases.append(row);(d.EVIDENCE/'phases.json').write_text(json.dumps(phases,indent=2))
    print(json.dumps({'cycle':d.metadata['number'],'phase':name}),flush=True)
def open_settings():
    d.act('press','menu','Conversation','conversation');d.act('press','menu','Agent Presence…','conversation')
    d.wait(lambda:any(w['id']==settings for w in d.state()['windows']))
def choose(mode):
    d.act('press',settings,'gator.presence.mode');d.act('press',settings,mode)
def quit():
    before=d.durable();d.act('quit');d.wait(lambda:any(w.get('description')=='alert' for w in d.state()['windows']))
    alert=next(w['id'] for w in d.state()['windows'] if w.get('description')=='alert')
    # explicit guarded Quit may cancel pending Work, never an approval decision.
    d.check_input();d.finishing=True;d.act('press',alert,'Quit and Keep Retained Work')
    start=time.monotonic();d.proc.wait(timeout=12);d.sampler.wait(timeout=3)
    rows=[];children=[]
    for p in pathlib.Path(d.metadata['root']).glob('bridge-*.jsonl'):
        frames=[json.loads(x) for x in p.read_text().splitlines()];rows+=frames
        children += [x['pid'] for x in frames if x.get('direction')=='launch']
    assert not [x for x in rows if x.get('direction')=='command' and x.get('type')=='approval']
    surviving=[]
    for pid in children:
        try:os.kill(pid,0);surviving.append(pid)
        except ProcessLookupError:pass
    assert not surviving
    result={'exit':d.proc.returncode,'quitWaitSeconds':time.monotonic()-start,'durableBeforeQuit':before,'durableAfterQuit':d.durable(),'zeroApprovalCommands':True,'GoPIDs':children,'survivingGoPIDs':surviving,'controllerPID':d.controller.pid,'inputAudit':str(d.audit)}
    assert d.proc.returncode==0 and not result['durableAfterQuit']['approved']
    (d.EVIDENCE/f"cycle-{d.metadata['number']:02d}-quit.json").write_text(json.dumps(result,indent=2));d.pending=None
    print(json.dumps({'cycle':d.metadata['number'],'QUIT':result}),flush=True)
try:
    for number in range(5,15):
        mode=['off','emoji','reactions'][number%3]
        d.new(number,'/tmp/Gator-Native-P04E1R-Verified.app',mode);phase('idle');time.sleep(1)
        d.prepare();phase('pending review')
        record=None
        if number==5:
            record=subprocess.Popen(['/tmp/gator-presence-r1-record',str(d.proc.pid),str(d.EVIDENCE/'settings-review.mp4'),'22'],stdout=open(d.EVIDENCE/'recording.log','w'),stderr=subprocess.STDOUT)
            d.wait(lambda:'recording started' in (d.EVIDENCE/'recording.log').read_text(),6)
        open_settings();phase('settings open')
        for appearance in ['Emoji only','Emoji + GIF reactions','Off','Emoji + GIF reactions']:choose(appearance)
        for target in ['gator.mascot.animated','gator.desktopPet.shown','gator.mascot.shown']:d.act('press',settings,target)
        phase('desktop enabled; chat/animation disabled');d.act('close',settings);time.sleep(1)
        phase('settings closed with pending review');open_settings()
        d.act('press',settings,'gator.desktopPet.tuck')
        for target in ['gator.mascot.shown','gator.mascot.animated']:d.act('press',settings,target)
        d.act('close',settings);phase('tucked away; settings closed')
        d.act('press',target='Back to Conversation');d.act('press',target='An action needs your review')
        phase('review reopened')
        if record:
            record.wait(timeout=25);assert record.returncode==0
        # close/reopen the conversation while Settings provides the verified key window.
        open_settings();d.act('close','conversation',expected=settings)
        d.act('press','menu','Window',settings);d.act('press','menu','Gator',settings)
        d.act('focus',settings);d.act('close',settings);d.act('focus');phase('main window reopened')
        time.sleep(2);phase('settled before quit');quit()
    (d.EVIDENCE/'batch-result.json').write_text(json.dumps({'completed':10,'controllerPID':d.controller.pid,'inputAudit':str(d.audit),'productionChanged':False},indent=2))
except Exception as error:
    (d.EVIDENCE/'batch-abort.json').write_text(json.dumps({'cycle':d.metadata,'error':str(error),'epoch':time.time()},indent=2));print('ABORT',error,flush=True)
    # retain only this disposable fixture for diagnosis; never label a force-stop as Quit.
    # no further acceptance input after an abort. Collect a bounded native sample.
    if d.proc and d.proc.poll() is None:
        subprocess.run(['sample',str(d.proc.pid),'1','-file',str(d.EVIDENCE/'abort-sample.txt')],stdout=open(d.EVIDENCE/'sample-command.log','w'),stderr=subprocess.STDOUT,timeout=10)
        print('Fixture remains open for explicit guarded cleanup; not an acceptance pass.',flush=True)
        time.sleep(1)
    raise
finally:
    if d.provider:d.provider.terminate();d.provider.wait(timeout=3)
    d.controller.stdin.close();d.controller.wait(timeout=3)
