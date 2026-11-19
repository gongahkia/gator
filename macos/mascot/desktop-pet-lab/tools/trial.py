# owns both fixture processes so stale PIDs cannot invalidate native assertions
import subprocess,pathlib,tempfile,time,json,shutil
r=pathlib.Path(tempfile.mkdtemp(prefix='gator-pet-b1-trial-',dir='/tmp'));e=pathlib.Path('macos/mascot/evidence/p04m-b1')
peer=subprocess.Popen(['/tmp/Gator-Pet-B1-Focus-Final.app/Contents/MacOS/FocusFixture',str(r/'peer')],stdout=open(e/'trial-peer.log','w'),stderr=subprocess.STDOUT)
try:
 deadline=time.monotonic()+8
 while not (r/'peer/peer.json').exists() and time.monotonic()<deadline:time.sleep(.03)
 assert json.loads((r/'peer/peer.json').read_text())['pid']==peer.pid
 args=['/tmp/Gator-Pet-B1-Window-Lifetime-Final.app/Contents/MacOS/PetLab','--trial','--background','--peer-pid',str(peer.pid),'--output',str(r/'lab')]
 # final renderer configuration, with the corrected native-event ownership probe
 p=subprocess.Popen(args,stdout=open(e/'verified-trial-console.log','w'),stderr=subprocess.STDOUT)
 p.wait(timeout=210);shutil.copytree(r/'lab',e/'verified-trial',dirs_exist_ok=True)
 (e/'verified-trial-exit.json').write_text(json.dumps({'root':str(r),'exit':p.returncode,'peerPID':peer.pid,'labPID':p.pid},indent=2))
finally:
 peer.terminate();peer.wait(timeout=8)
