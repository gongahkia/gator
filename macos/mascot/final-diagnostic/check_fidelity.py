#!/usr/bin/env python3
"""Keep end-to-end material parity distinct from controlled geometry fidelity."""
from pathlib import Path
import json
import numpy as np
from image_analysis import read_png,write_png,metrics

root=Path(__file__).resolve().parent.parent/'evidence/p04m-a2-final';images=root/'images-final'
regions={'eyes-and-lids':(20,16,76,46),'snout':(10,44,86,66),'jaw':(20,64,76,89)}
checks=[];end_to_end=[];negative=[]
for pose in ['neutral','articulated','jaw','lids','gaze']:
    source,source_meta=read_png(images/f'usd-{pose}-0.png')
    native,native_meta=read_png(images/f'native-{pose}-0.png')
    canonical,canonical_meta=read_png(images/f'native-usd-materials-{pose}-0.png')
    assert source_meta==native_meta==canonical_meta and 'sRGB'in source_meta
    assert source.shape==native.shape==canonical.shape==(96,96,4)
    exact_alpha=np.array_equal(source[:,:,3],canonical[:,:,3])
    row={'pose':pose,'commonMaterialComparison':metrics(source,canonical),'alphaExact':exact_alpha,
         'boundsAndDimensionsExact':metrics(source,canonical)['visibleBounds']==metrics(source,source)['visibleBounds'],
         'regions':{}}
    for name,(x0,y0,x1,y1) in regions.items():
        difference=np.abs(source[y0:y1,x0:x1].astype(int)-canonical[y0:y1,x0:x1].astype(int))
        row['regions'][name]={'maxDecoded8BitChannelDifference':int(difference.max())}
    # One stored RGBA8 code is a quantization-level bound for the controlled,
    # same-material mesh path, not permission for arbitrary lighting changes.
    row['passed']=exact_alpha and row['boundsAndDimensionsExact'] and row['commonMaterialComparison']['maxChannel8Bit']<=1 and all(r['maxDecoded8BitChannelDifference']<=1 for r in row['regions'].values())
    checks.append(row)
    direct=metrics(source,native);direct['pose']=pose;direct['strictOneCodeParityPassed']=direct['maxChannel8Bit']<=1
    end_to_end.append(direct)
    for name in ['negative-missing-eye','negative-missing-jaw']:
        broken,_=read_png(images/f'{name}-{pose}-0.png');bad=metrics(source,broken)
        rejected=bad['maxChannel8Bit']>1 or bad['maxAlphaDelta']!=0
        negative.append({'name':name,'pose':pose,'rejected':rejected,'metrics':bad})
    if pose in ['neutral','articulated']:
        old,_=read_png(root/'images'/f'native-{pose}-0.png')
        def composite(a):
            checker=np.where((np.indices(a.shape[:2]).sum(axis=0)//8%2)[:,:,None],235,255)
            alpha=a[:,:,3:4].astype(float)/255
            return np.concatenate([(a[:,:,:3]*alpha+checker*(1-alpha)).astype(np.uint8),np.full_like(a[:,:,3:4],255)],axis=2)
        diff=np.zeros_like(source);diff[:,:,:3]=np.minimum(255,np.abs(source[:,:,:3].astype(int)-native[:,:,:3].astype(int))*8);diff[:,:,3]=255
        grid=np.concatenate([composite(source),composite(old),composite(native),diff],axis=1)
        write_png(root/f'{pose}-reference-before-after-difference8x.png',np.repeat(np.repeat(grid,4,axis=0),4,axis=1))
        write_png(root/f'{pose}-final-difference8x.png',np.repeat(np.repeat(diff,4,axis=0),4,axis=1))
result={'commonMaterialGeometryChecks':checks,'endToEndMaterialComparisons':end_to_end,
        'negativeControls':negative,'commonMaterialChecksPassed':all(x['passed']for x in checks),
        'allNegativeControlsRejected':all(x['rejected']for x in negative),
        'endToEndStrictParityPassed':all(x['strictOneCodeParityPassed']for x in end_to_end),
        'testScope':'Canonical-material checks isolate geometry. End-to-end native material parity remains a separate result; its failure is not turned into a pass.'}
(root/'fidelity-final.json').write_text(json.dumps(result,indent=2)+'\n')
assert result['commonMaterialChecksPassed'] and result['allNegativeControlsRejected']
print('Canonical-material geometry:',result['commonMaterialChecksPassed'],'negative controls rejected:',result['allNegativeControlsRejected'],'end-to-end strict material parity:',result['endToEndStrictParityPassed'])
