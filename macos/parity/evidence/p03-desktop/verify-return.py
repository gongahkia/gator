import json,pathlib,subprocess,os
m=json.load(open('/tmp/gator-p03-acceptance.json'));pid=str(m['pid']);ctl='/tmp/gator-p03-acceptance-control';base=pathlib.Path('macos/parity/evidence/p03-desktop');log=open(base/'return-accepted.txt','w',buffering=1)
def call(*args):
 p=subprocess.run([ctl,pid,*args],capture_output=True,text=True,timeout=30);log.write(repr(args)+'\n'+p.stdout+p.stderr)
 if p.returncode:raise RuntimeError((args,p.returncode,p.stdout,p.stderr))
 return p.stdout
pids=subprocess.check_output(['pgrep','-P',pid],text=True).split()
for name,id,folder,text in [('a',m['aID'],m['a'],'A desktop draft remains with A'),('b',m['bID'],m['b'],'B desktop draft remains with B')]:
 if name=='b':call('press','menu','All Conversations…')
 call('wait','catalog','gator.catalog.row.'+id);call('select-row','catalog','gator.catalog.row.'+id);call('press','catalog','Open');call('wait-gone','catalog')
 # The UI label is supplied by the Go snapshot. Compare actual stored source.
 d=json.load(open(m['archive']));assert d['selectedConversationID']==id
 source=json.load(open(pathlib.Path(m['root'])/'state/gator/conversations'/id/'conversation.json'))['source_path'];assert os.path.samefile(source,folder)
 call('wait','conversation','Current workspace: '+source);call('inspect','conversation','gator.composer.text')
 assert d['drafts']['conversation:'+id]['text']==text
 assert subprocess.check_output(['pgrep','-P',pid],text=True).split()==pids
 log.write('PASS exact '+name.upper()+' conversation/source/draft, original owner PIDs retained\n')
call('frames');print('PASS return A and B via exact native catalog rows')
