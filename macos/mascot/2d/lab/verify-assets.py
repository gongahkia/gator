# image-only QA of the generated fixture assets; requires installed numpy/magick
import pathlib,subprocess,numpy as np,json,hashlib,sys
root=pathlib.Path(__file__).resolve().parent.parent/'assets';rows=[]
for file in sorted(root.glob('*/*/*.png')):
 pixels=np.frombuffer(subprocess.check_output(['magick',str(file),'-colorspace','sRGB','-depth','8','RGBA:-']),dtype=np.uint8).reshape(224,224,4)
 alpha=pixels[:,:,3];edge=np.concatenate([alpha[0],alpha[-1],alpha[:,0],alpha[:,-1]])
 assert edge.max()==0 and alpha.max()==255 and (alpha>0).sum()>1000,file
 rows.append({'file':str(file.relative_to(root)),'sha256':hashlib.sha256(file.read_bytes()).hexdigest(),'transparentPixels':int((alpha==0).sum()),'partialAlphaPixels':int(((alpha>0)&(alpha<255)).sum())})
assert len(rows)==118
pathlib.Path(sys.argv[1]).write_text(json.dumps({'frames':rows,'all224square':True,'allEdgesTransparent':True,'clippingAtImageBoundary':False},indent=2))
print('118 PNG frames: valid dimensions, alpha, nonempty character, transparent borders')
