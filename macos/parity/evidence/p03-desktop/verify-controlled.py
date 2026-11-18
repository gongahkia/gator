# Final user-reserved session. Poll actual controls; no model/task submission.
import json,pathlib,subprocess,re,os,time
m=json.load(open('/tmp/gator-p03-acceptance.json'));pid=str(m['pid']);ctl='/tmp/gator-p03-acceptance-control';base=pathlib.Path('macos/parity/evidence/p03-desktop');log=open(base/'controlled-final.txt','w',buffering=1)
def call(*args,optional=False):
 p=subprocess.run([ctl,pid,*args],text=True,capture_output=True,timeout=30);log.write(repr(args)+'\n'+p.stdout+p.stderr)
 if p.returncode and not optional:raise RuntimeError((args,p.returncode,p.stdout,p.stderr))
 return p.stdout

def capture(name,width):
 frames=call('frames');matches=re.findall(r'captureID=(\d+) bounds=\{(.*?)\}',frames,re.S)
 id=next(i for i,s in matches if re.search(r'Width = "?'+str(width)+r'"?;',s))
 subprocess.run(['python3','/Users/gongahkia/.codex/skills/screenshot/scripts/take_screenshot.py','--window-id',id,'--path','/tmp/'+name+'.png'],check=True,stdout=log)
 log.write('CAPTURE '+name+' '+id+'\n')

def assert_state(id,text):
 d=json.load(open(m['archive']));assert d['selectedConversationID']==id;assert d['drafts']['conversation:'+id]['text']==text
 assert subprocess.check_output(['pgrep','-P',pid],text=True).split()==pids
 return d

def open_id(id):
 call('press','menu','All Conversations…');call('wait','catalog','gator.catalog.row.'+id);call('select-row','catalog','gator.catalog.row.'+id);call('press','catalog','Open');call('wait-gone','catalog')

pids=subprocess.check_output(['pgrep','-P',pid],text=True).split();before=assert_state(m['bID'],'B desktop draft remains with B')
call('press','app','Gator, Ready',optional=True);call('wait','compact','More');call('compact-cancel');assert json.load(open(m['archive']))==before
# The compact presentation may remain visible or close with the native panel.
if 'missing' in call('inspect','compact','gator.composer.text',optional=True):call('press','app','Gator, Ready',optional=True)
call('wait','compact','More');call('inspect','compact','gator.composer.text');capture('gator-p03-controlled-compact',440)
assert_state(m['bID'],'B desktop draft remains with B');log.write('PASS compact More/picker/Cancel/draft/owner isolation\n')
call('focus','conversation');call('resize','conversation','540','480');call('inspect','conversation','Choose Workspace…');call('inspect','conversation','gator.composer.text');capture('gator-p03-controlled-minimum',540)
log.write('PASS minimum window, long path and controls observed\n')
open_id(m['aID']);assert_state(m['aID'],'A desktop draft remains with A');call('inspect','conversation','gator.composer.text')
source=json.load(open(pathlib.Path(m['root'])/'state/gator/conversations'/m['aID']/'conversation.json'))['source_path'];call('wait','conversation','Current workspace: '+source)
call('press','conversation','Choose Workspace…');call('wait','open-panel','OKButton');call('inspect','open-panel','where popup');call('focus','open-panel');call('confirm-key','open-panel');call('wait','conversation','Already using this workspace · conversation kept');assert_state(m['aID'],'A desktop draft remains with A')
log.write('PASS native keyboard Return same A, original owner/draft/source retained\n')
open_id(m['bID']);assert_state(m['bID'],'B desktop draft remains with B');log.write('PASS return B exact draft and unchanged PIDs\n')
# Inspect an older retained message, then choose the unchanged source.
open_id(m['ids'][-1]);assert_state(m['ids'][-1],'Original acceptance draft stays here');call('resize','conversation','820','740');call('focus','conversation');call('press','menu','Find in Current Transcript…');call('wait','conversation','Find in loaded transcript');call('set','conversation','Find in loaded transcript','Retained note 0005-15');call('wait','conversation','Retained note 0005-15')
head_before=call('inspect','conversation','Retained note 0005-15');call('inspect','conversation','Find in loaded transcript');capture('gator-p03-controlled-reading-before',820)
call('press','conversation','Choose Workspace…');call('wait','open-panel','OKButton');call('focus','open-panel');call('confirm-key','open-panel');call('wait','conversation','Already using this workspace · conversation kept');call('inspect','conversation','Find in loaded transcript');head_after=call('inspect','conversation','Retained note 0005-15');capture('gator-p03-controlled-reading-after',820)
assert_state(m['ids'][-1],'Original acceptance draft stays here')
# Check the same heading stays in the actual transcript viewport, not just loaded.
for head in [head_before,head_after]:
 y=float(re.search(r'pos=\([^,]+, ([^)]+)\)',head).group(1));assert 170<y<690,head
log.write('PASS unchanged-source Find query, older visible heading, exact draft and owner PIDs\n')
# Escape must close Find without sending, then Cmd+L returns to the composer.
call('focus','conversation');call('focus-element','conversation','Find in loaded transcript');call('key-focus','conversation','53','0','gator.composer.text');call('inspect','conversation','gator.composer.text')
assert_state(m['ids'][-1],'Original acceptance draft stays here')
records={p.parent.name:json.load(open(p)) for p in (pathlib.Path(m['root'])/'state/gator/conversations').glob('*/conversation.json')}
for id in [m['aID'],m['bID']]:
 d=pathlib.Path(m['root'])/'state/gator/conversations'/id
 assert not (d/'requests.json').exists();assert records[id]['snapshot_id']==''
 assert not list((d/'revisions').glob('*'))
summary={'root':m['root'],'app':m['app'],'pid':m['pid'],'original':m['ids'][-1],'aID':m['aID'],'bID':m['bID'],'ownerPIDs':pids,'records':records,'drafts':json.load(open(m['archive']))['drafts'],'checks':'A/B handoff, compact cancel, minimum, same A, returns, reading, Escape and no request admission PASS'}
(base/'controlled-final.json').write_text(json.dumps(summary,indent=2));log.write('PASS controlled acceptance flow\n');print('PASS controlled acceptance flow',flush=True)
