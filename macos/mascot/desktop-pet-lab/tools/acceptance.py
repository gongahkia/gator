# all processes and screenshots are disposable, allowlisted fixtures
import pathlib,subprocess,json,tempfile,time
root=pathlib.Path(tempfile.mkdtemp(prefix='gator-pet-b1-native-',dir='/tmp'))
evidence=pathlib.Path('macos/mascot/evidence/p04m-b1');control='/tmp/gator-pet-control';capture='/tmp/gator-pet-capture';pointer='/tmp/gator-pet-pointer'
app='/tmp/Gator-Mascot-P04M-B1-Desktop-Pet-Lab.app/Contents/MacOS/PetLab'
owned=[];results=[]
def state(path):
 try:return json.loads(path.read_text())
 except (OSError,ValueError):return {}
def until(check,label):
 limit=time.monotonic()+8
 while time.monotonic()<limit:
  if check():return
  time.sleep(.03)
 raise RuntimeError('Not observed: '+label)
def run(*args):return subprocess.run(list(map(str,args)),check=True,timeout=12)
def launch(args,name):
 p=subprocess.Popen(args,stdout=open(evidence/(name+'.log'),'w'),stderr=subprocess.STDOUT);owned.append(p);return p
def lab(placement,mode,style,demo=False):
 out=root/(placement+'-'+mode);p=launch([app,'--show','--locked','--background','--placement',placement,'--mode',mode,'--style',style,'--output',str(out)]+(['--demo'] if demo else []),placement+'-'+mode)
 until(lambda:state(out/'state.json').get('pid')==p.pid,'new lab process readiness');return p,out
def tuck_quit(p,out):
 run(control,p.pid,'press','menu','Tuck Away Desktop Pet');until(lambda:state(out/'state.json').get('desktopEnabled') is False,'Tuck Away')
 run(capture,evidence/(out.name+'-hidden-final.png'),p.pid,peer.pid)
 run(control,p.pid,'press','menu','Quit Pet Lab');p.wait(timeout=8);results.append({'phase':out.name,'exit':p.returncode,'state':state(out/'state.json')})
try:
 peerRoot=root/'peer';peer=launch(['/tmp/Gator-Pet-B1-Focus-Final.app/Contents/MacOS/FocusFixture',str(peerRoot)],'peer-acceptance')
 until(lambda:state(peerRoot/'peer.json').get('pid')==peer.pid,'peer readiness')
 p,out=lab('B','passive','rendered',True)
 run(control,peer.pid,'focus','peer');before=state(peerRoot/'peer.json')['clicks'];run(pointer,p.pid,peer.pid,.5,.5);until(lambda:state(peerRoot/'peer.json')['clicks']>before,'passive click reaches underlying button');results.append({'passiveClick':True,'peerClicks':state(peerRoot/'peer.json')['clicks']})
 recording=launch([capture,str(evidence/'native-motion-accepted.mp4'),str(p.pid),str(peer.pid),'--seconds','18'],'native-recording-accepted')
 recording.wait(timeout=25)
 run(capture,evidence/'top-center-B-final.png',p.pid,peer.pid)
 run(control,peer.pid,'press','peer','Focus secure');run(control,peer.pid,'state')
 run(control,peer.pid,'press','peer','Toggle Fullscreen');until(lambda:state(peerRoot/'peer.json').get('fullScreen') is True,'native fullscreen enter')
 run(capture,evidence/'fullscreen-final.png',p.pid,peer.pid)
 run(control,peer.pid,'press','peer','Toggle Fullscreen');until(lambda:state(peerRoot/'peer.json').get('fullScreen') is False,'native fullscreen exit')
 recording.wait(timeout=25);tuck_quit(p,out)
 for place,style in [('A','rendered'),('C','classic')]:
  p,out=lab(place,'passive',style);run(control,peer.pid,'focus','peer');run(capture,evidence/(place+'-placement-final.png'),p.pid,peer.pid);tuck_quit(p,out)
 p,out=lab('B','interactive','classic');run(control,peer.pid,'focus','peer');before=state(out/'state.json')['petFrame'];run(pointer,p.pid,peer.pid,.5,.5,'drag');until(lambda:state(out/'state.json')['petFrame']!=before,'native drag movement');after=state(out/'state.json');results.append({'dragBefore':before,'dragAfter':after['petFrame'],'dragEvents':[e for e in after['events'] if e['event']=='drag-without-activation']});run(capture,evidence/'drag-final.png',p.pid,peer.pid)
 run(control,peer.pid,'focus','peer');run(pointer,p.pid,peer.pid,.5,.5);until(lambda:any(e['event']=='intentional-click-open-fixture' for e in state(out/'state.json')['events']),'interactive click callback');run(control,p.pid,'state');results.append({'interactiveClick':True})
 # the moved pet still leaves the peer's larger native target reachable immediately outside its frame
 run(control,peer.pid,'focus','peer');run(pointer,p.pid,peer.pid,-.06,.5);results.append({'outsidePointerPosted':True})
 tuck_quit(p,out)
finally:
 (evidence/'native-acceptance.json').write_text(json.dumps({'root':str(root),'results':results,'processes':[{'pid':p.pid,'exit':p.poll()} for p in owned]},indent=2))
 for p in owned:
  if p.poll() is None:p.terminate()
