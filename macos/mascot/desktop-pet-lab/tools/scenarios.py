# bounded native-window evidence, never includes unrelated application windows
import subprocess,pathlib,json,time,os
root=pathlib.Path("macos/mascot/evidence/p04m-b1")
app="/tmp/Gator-Mascot-P04M-B1-Desktop-Pet-Lab.app/Contents/MacOS/PetLab"
peer=json.loads(pathlib.Path("/tmp/gator-pet-b1-peer-final/peer.json").read_text())["pid"]
control="/tmp/gator-pet-control";capture="/tmp/gator-pet-capture"
def ready(path):
    limit=time.monotonic()+8
    while time.monotonic()<limit:
        if path.exists():
            try:return json.loads(path.read_text())
            except ValueError:pass
        time.sleep(.03)
    raise RuntimeError("fixture readiness not observed")
for placement,mode,style in [("B","interactive","classic")]:
    key=placement+"-"+mode+"-v2";out=pathlib.Path("/tmp/gator-pet-b1-scenario-"+key);assert not out.exists()
    log=open(root/(key+"-console.log"),"w")
    p=subprocess.Popen([app,"--show","--locked","--background","--placement",placement,"--mode",mode,"--style",style,"--demo","--output",str(out)],stdout=log,stderr=subprocess.STDOUT)
    state=ready(out/"state.json")
    subprocess.run([control,str(peer),"focus","peer"],check=True)
    if placement=="B" and mode=="passive":
        subprocess.run(["/tmp/gator-pet-pointer",str(p.pid),str(peer),"0.5","0.5"],check=True)
        subprocess.run([capture,str(root/"native-motion-final.mp4"),str(p.pid),str(peer),"--seconds","18"],check=True)
    subprocess.run([capture,str(root/(key+".png")),str(p.pid),str(peer)],check=True)
    if mode=="interactive":
        subprocess.run([control,str(peer),"focus","peer"],check=True)
        before=json.loads((out/"state.json").read_text())
        subprocess.run(["/tmp/gator-pet-pointer",str(p.pid),str(peer),"0.5","0.5","drag"],check=True)
        limit=time.monotonic()+3
        while time.monotonic()<limit:
            after=json.loads((out/"state.json").read_text())
            if after.get("petFrame")!=before.get("petFrame"):break
            time.sleep(.03)
        (root/"drag-observation.json").write_text(json.dumps({"before":before.get("petFrame"),"after":after.get("petFrame"),"events":after["events"][-3:]},indent=2))
        subprocess.run([capture,str(root/"after-drag.png"),str(p.pid),str(peer)],check=True)
        subprocess.run([control,str(peer),"focus","peer"],check=True)
        subprocess.run(["/tmp/gator-pet-pointer",str(p.pid),str(peer),"0.5","0.5"],check=True)
        subprocess.run([control,str(p.pid),"wait-window","controls"],check=True)
    subprocess.run([control,str(p.pid),"press","menu","Tuck Away Desktop Pet"],check=True)
    hidden=ready(out/"state.json")
    subprocess.run([capture,str(root/(key+"-tucked.png")),str(p.pid),str(peer)],check=True)
    subprocess.run([control,str(p.pid),"press","menu","Quit Pet Lab"],check=True)
    p.wait(timeout=8)
    (root/(key+"-state.json")).write_text((out/"state.json").read_text())
