# Sequential desktop checks against the current isolated fixture, never production.
import json,os,pathlib,re,subprocess,sys,time
m=json.load(open('/tmp/gator-p03-acceptance.json'));pid=str(m['pid']);ctl='/tmp/gator-p03-acceptance-control';base=pathlib.Path('macos/parity/evidence/p03-desktop')
assert m['bundle']=='dev.gator.native.p03.acceptance.fixed'
log=open(base/'selection-accepted.txt','w',buffering=1)
def call(*args):
 p=subprocess.run([ctl,pid,*args],text=True,capture_output=True,timeout=30)
 log.write('CONTROL '+repr(args)+'\n'+p.stdout+p.stderr)
 if p.returncode:raise RuntimeError((args,p.returncode,p.stdout,p.stderr))
 return p.stdout

def checkpoint(phase):
 d=json.load(open(m['archive']));id=d['selectedConversationID'];directory=pathlib.Path(m['root'])/'state/gator/conversations'
 records={p.parent.name:json.load(open(p)) for p in directory.glob('*/conversation.json')}
 children=subprocess.check_output(['pgrep','-P',pid],text=True).split()
 selected=directory/id
 record={'phase':phase,'selected':id,'source':records[id]['source_path'],'drafts':d['drafts'],'children':children,'records':records,'selectedFiles':[str(p.relative_to(selected)) for p in selected.rglob('*') if p.is_file()]}
 (base/(phase+'.json')).write_text(json.dumps(record,indent=2));log.write(json.dumps({'phase':phase,'selected':id,'source':record['source'],'children':children})+'\n');return record

call('wait','conversation','gator.composer.text');call('move','conversation','325','125');call('focus','conversation')
original=checkpoint('accepted-before')
call('press','menu','Conversation');call('press','menu','Choose Workspace…');call('wait','open-panel','OKButton')
call('select-file','open-panel','AXTextField::Workspace A');call('inspect','open-panel','where popup')
call('press','open-panel','OKButton');call('wait-gone','open-panel');call('wait','conversation','New conversation · workspace stays read-only during Work')
a=checkpoint('accepted-after-a');assert a['selected']!=original['selected'];assert os.path.samefile(a['source'],m['a']);assert len(a['children'])==2
call('inspect','conversation','gator.composer.text');call('set','conversation','gator.composer.text','A desktop draft remains with A')
call('press','conversation','Choose Workspace…');call('wait','open-panel','OKButton')
call('select-file','open-panel','AXTextField::'+pathlib.Path(m['b']).parent.name);call('wait','open-panel','AXTextField::Workspace B');call('select-file','open-panel','AXTextField::Workspace B');call('inspect','open-panel','where popup')
call('press','open-panel','OKButton');call('wait-gone','open-panel');call('wait','conversation','New conversation · workspace stays read-only during Work')
b=checkpoint('accepted-after-b');assert b['selected'] not in [original['selected'],a['selected']];assert os.path.samefile(b['source'],m['b']);assert len(b['children'])==3
assert set(a['children']).issubset(b['children']);assert b['records'][a['selected']]==a['records'][a['selected']]
assert b['drafts']['conversation:'+a['selected']]['text']=='A desktop draft remains with A'
assert b['drafts']['conversation:'+original['selected']]['text']=='Original acceptance draft stays here'
assert not any(x in b['selectedFiles'] for x in ['requests.json','dialogue.json','options.json']);assert b['records'][b['selected']]['snapshot_id']==''
call('set','conversation','gator.composer.text','B desktop draft remains with B')
m['aID']=a['selected'];m['bID']=b['selected'];m['originalID']=original['selected'];pathlib.Path('/tmp/gator-p03-acceptance.json').write_text(json.dumps(m,indent=2))
log.write('PASS actual native A/B selection, canonical persisted sources, distinct selected identities, existing owner PIDs retained, exact independent drafts, no request/snapshot admission\n')
print('PASS selection A -> B',flush=True)
