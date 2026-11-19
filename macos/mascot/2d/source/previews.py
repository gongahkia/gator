# convenience previews only; production uses finite native keyframes and PNGs
import json,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parent.parent
manifest=json.loads((root/'assets/manifest.json').read_text());out=root/'previews';out.mkdir(exist_ok=True)
for clip in manifest['clips']:
 if clip['frames']<2:continue
 frames=[str(root/'assets'/clip['style']/clip['state']/f'{i:02}.png') for i in range(clip['frames'])]
 subprocess.run(['magick','-dispose','Background','-delay',str(clip['duration']*100/clip['frames']),*frames,'-loop','0',str(out/(clip['style']+'-'+clip['state']+'.gif'))],check=True)
