#!/usr/bin/env python3
"""Deterministic RGBA8 PNG diagnostics; no generated or repainted model pixels."""
from pathlib import Path
import hashlib
import json
import struct
import zlib
import sys
import numpy as np

root=Path(__file__).resolve().parent.parent/'evidence/p04m-a2-final'
def read_png(path):
    data=path.read_bytes();assert data[:8]==b'\x89PNG\r\n\x1a\n'
    offset=8;compressed=b'';metadata={}
    while offset<len(data):
        count=struct.unpack('>I',data[offset:offset+4])[0];kind=data[offset+4:offset+8];part=data[offset+8:offset+8+count];offset+=12+count
        if kind==b'IHDR':width,height,depth,color,compression,filtering,interlace=struct.unpack('>IIBBBBB',part);assert (depth,color,compression,filtering,interlace)==(8,6,0,0,0)
        elif kind==b'IDAT':compressed+=part
        elif kind!=b'IEND':metadata[kind.decode('ascii')]=hashlib.sha256(part).hexdigest()
    raw=zlib.decompress(compressed);stride=width*4;previous=bytearray(stride);rows=[];index=0
    for _ in range(height):
        method=raw[index];index+=1;row=bytearray(raw[index:index+stride]);index+=stride
        for i in range(stride):
            left=row[i-4] if i>=4 else 0;up=previous[i];corner=previous[i-4] if i>=4 else 0
            if method==1:prediction=left
            elif method==2:prediction=up
            elif method==3:prediction=(left+up)//2
            elif method==4:
                p=left+up-corner;a=abs(p-left);b=abs(p-up);c=abs(p-corner)
                prediction=left if a<=b and a<=c else up if b<=c else corner
            else:assert method==0;prediction=0
            row[i]=(row[i]+prediction)&255
        rows.append(row);previous=row
    return np.frombuffer(b''.join(rows),dtype=np.uint8).reshape(height,width,4).copy(),metadata

def write_png(path,pixels):
    def chunk(kind,data):return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data)&0xffffffff)
    h,w,_=pixels.shape
    raw=b''.join(b'\0'+pixels[y].tobytes() for y in range(h))
    path.write_bytes(b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',w,h,8,6,0,0,0))+chunk(b'sRGB',b'\0')+chunk(b'IDAT',zlib.compress(raw))+chunk(b'IEND',b''))

def metrics(a,b):
    assert a.shape==b.shape
    # All source captures are produced by the same bitmap writer. Record equal
    # profile metadata separately; compare decoded samples, not PNG byte streams.
    aa=a.astype(float);bb=b.astype(float);mask=(a[:,:,3]>0)|(b[:,:,3]>0)
    delta=np.abs(aa-bb);delta[~mask,:3]=0
    yy,xx,channel=np.unravel_index(delta.argmax(),delta.shape)
    linear=lambda v:np.where(v<=.04045,v/12.92,((v+.055)/1.055)**2.4)
    la=linear(aa[:,:,:3]/255)*(aa[:,:,3:4]/255);lb=linear(bb[:,:,:3]/255)*(bb[:,:,3:4]/255)
    result={'maxChannel8Bit':float(delta.max()),'meanChannel8Bit':float(delta.mean()),
            'opaqueRGBMean8Bit':float(delta[:,:,:3][mask].mean()),'changedPixels':int((delta.max(axis=2)>0).sum()),
            'maxAt':[int(xx),int(yy),int(channel)],'samplesAtMax':[a[yy,xx].tolist(),b[yy,xx].tolist()],
            'silhouetteDifferentPixels':int(((a[:,:,3]>0)!=(b[:,:,3]>0)).sum()),'maxAlphaDelta':float(delta[:,:,3].max()),
            'linearPremultipliedRGB_RMSE':float(np.sqrt(np.mean((la-lb)**2))),
            'visiblePixels':int((b[:,:,3]>0).sum()),'transparentPixels':int((b[:,:,3]==0).sum()),
            'visibleBounds':[int(v) for v in [np.nonzero(mask)[1].min(),np.nonzero(mask)[0].min(),np.nonzero(mask)[1].max(),np.nonzero(mask)[0].max()]]}
    return result

if __name__=='__main__':
    images=Path(sys.argv[1]) if len(sys.argv)>1 else root/'images';records={};metadata={};comparisons={}
    for path in images.glob('*.png'):
        records[path.stem],metadata[path.stem]=read_png(path)
    names=sorted({r['template'] for r in json.loads((images/'manifest.json').read_text())['images']})
    for pose in ['neutral','articulated','jaw','lids','gaze']:
        for name in names:
            if f'{name}-{pose}-0' not in records:continue
            for repeat in range(5):
                key=f'{name}-{pose}-{repeat}'
                comparisons['usd-reference_vs_'+key]=metrics(records[f'usd-{pose}-0'],records[key])
                comparisons['same-path-first_vs_'+key]=metrics(records[f'{name}-{pose}-0'],records[key])
    historical={}
    old=root.parent/'p04m-a2-gate2/final-v10'
    for pose in ['neutral','articulated']:
        a,ma=read_png(old/f'direct96/{pose}-direct96.png');b,mb=read_png(old/f'native96/{pose}-native96.png')
        historical[pose]=metrics(a,b);historical[pose]['profileMetadataEqual']=ma==mb
        checker=np.where((np.indices(a.shape[:2]).sum(axis=0)//8%2)[:,:,None],235,255).astype(np.uint8)
        def composite(image):
            alpha=image[:,:,3:4].astype(float)/255;rgb=image[:,:,:3]*alpha+checker*(1-alpha)
            return np.concatenate([rgb.astype(np.uint8),np.full_like(image[:,:,3:4],255)],axis=2)
        diff=np.abs(a.astype(int)-b.astype(int));diff[:,:,3]=255
        heat=np.zeros_like(a);heat[:,:,:3]=np.minimum(255,diff[:,:,:3]*8).astype(np.uint8);heat[:,:,3]=255
        sheet=np.concatenate([composite(a),composite(b),heat],axis=1)
        write_png(root/f'historical-{pose}-side-by-side-diff8x.png',np.repeat(np.repeat(sheet,4,axis=0),4,axis=1))
        write_png(root/f'historical-{pose}-difference8x.png',np.repeat(np.repeat(heat,4,axis=0),4,axis=1))
    destination=Path(sys.argv[2]) if len(sys.argv)>2 else root/'image-analysis.json'
    destination.write_text(json.dumps({'historical':historical,'comparisons':comparisons,'metadata':metadata},indent=2)+'\n')
    print('Historical decoded sample comparisons',historical)
    for pose in ['neutral','articulated']:
        for name in names:
            if f'{name}-{pose}-0' not in records:continue
            r=comparisons[f'usd-reference_vs_{name}-{pose}-0'];print(pose,name,r['maxChannel8Bit'],r['changedPixels'],r['maxAt'])
        for name in ['usd','native']:
            print('repeat-variation',pose,name,max(comparisons[f'same-path-first_vs_{name}-{pose}-{i}']['maxChannel8Bit']for i in range(5)))
