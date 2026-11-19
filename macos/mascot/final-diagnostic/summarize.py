#!/usr/bin/env python3
"""Summarize finite, versioned diagnostic runs without combining unlike metrics."""
from pathlib import Path
import hashlib
import itertools
import json
import shutil
import statistics

evidence=Path(__file__).resolve().parent.parent/'evidence/p04m-a2-final'
mib=1024**2
summary={}
for directory in [Path('/tmp/gator-final-diag-r1'),Path('/tmp/gator-final-diag-r2')]:
    for case in directory.iterdir():
        if not case.is_dir() or not (case/'results.json').exists():continue
        target=evidence/'runs'/case.name;target.mkdir(parents=True,exist_ok=True)
        for item in case.iterdir():
            if item.is_file():shutil.copy2(item,target/item.name)
        rows=[json.loads(x) for x in (case/'samples.ndjson').read_text().splitlines()]
        baseline=[r for r in rows if r['phase']=='00-before-RealityKit']
        base={k:statistics.median(r[k]for r in baseline)for k in ['rssBytes','footprintBytes','graphicsFootprintBytes']}
        phases={}
        for name,group in itertools.groupby(rows,key=lambda x:x['phase']):
            group=list(group);first,last=group[0],group[-1]
            if len(group)>1 and last['elapsed']>first['elapsed']:
                cpu=(last['cpuSeconds']-first['cpuSeconds'])/(last['elapsed']-first['elapsed'])*100
            else:cpu=None
            fields=['rssBytes','footprintBytes','graphicsFootprintBytes','internalBytes','compressedBytes','defaultHeapInUseBytes','metalAllocatedBytes']
            values={k:[r[k]for r in group if isinstance(r.get(k),(int,float))]for k in fields}
            phases[name]={'sampleCount':len(group),'firstTime':first['elapsed'],'lastTime':last['elapsed'],'cpuOneCorePercent':cpu,
                          'frames':last['frames']-first['frames'],'lastLive':last['live'],
                          'medianMiB':{k:statistics.median(v)/mib if v else None for k,v in values.items()},
                          'minimumMiB':{k:min(v)/mib if v else None for k,v in values.items()},
                          'maximumMiB':{k:max(v)/mib if v else None for k,v in values.items()}}
        data={'baselineMiB':{k:v/mib for k,v in base.items()},'phases':phases,
              'peakIncrementalFootprintMiB':(max(r['footprintBytes']for r in rows)-base['footprintBytes'])/mib,
              'peakLedgerFootprintMiB':max(r['physicalFootprintPeakBytes']for r in rows)/mib}
        if case.name!='ownership':
            tail=[r for r in rows if r['phase'].startswith('11-all')]
            warm=statistics.median(r['footprintBytes']for r in tail[-40:])
            data['finalWarmedFootprintMiB']=warm/mib
            data['finalWarmedIncrementMiB']=(warm-base['footprintBytes'])/mib
            data['decayAfterFinalStaticDraw']=[]
            for cycle in range(3):
                sequence=[r for r in rows if r['phase'].startswith('c'+str(cycle)+'-') and int(r['phase'].split('-')[1])>=8]
                if not sequence:continue
                start=sequence[0]['elapsed'];end=sequence[-1]['elapsed'];low=None
                # require half a second continuously within 2 MiB of the matched
                # late plateau, rather than classifying one lucky low sample.
                for i,r in enumerate(sequence):
                    future=[x for x in sequence[i:] if x['elapsed']<=r['elapsed']+.6]
                    if future and future[-1]['elapsed']-r['elapsed']>=.5 and all(x['footprintBytes']<=warm+2*mib for x in future):
                        low=r['elapsed']-start;break
                data['decayAfterFinalStaticDraw'].append({'cycle':cycle,'secondsToWarmedPlus2MiB':low,'observationWindowSeconds':end-start})
        result=json.loads((case/'results.json').read_text())
        data['result']={k:result[k]for k in ['done','elapsed','decodeCount','frames','live','created','assertions']}
        data['policyEvents']=[x for x in result['events']if x.get('event')=='policy']
        summary[case.name]=data
endurance=Path('/tmp/gator-final-corrected-endurance/endurance')
if (endurance/'results.json').exists():
    result=json.loads((endurance/'results.json').read_text())
    if result['done']:
        target=evidence/'corrected-endurance';target.mkdir(exist_ok=True)
        for item in endurance.iterdir():
            if item.is_file():shutil.copy2(item,target/item.name)
        records=[r for r in result['records']if 'medianRSSBytes'in r];base=records[0]
        summary['corrected-endurance']={'elapsed':result['elapsed'],'frames':result['frames'],'created':result['created'],'live':result['live'],'decodeCount':result['decodeCount'],
            'checkpoints':[{k:r[k]for k in ['phase','medianRSSBytes','medianFootprintBytes','cpuPercentOneCore','live','frames']}for r in records],
            'baselineRSSMiB':base['medianRSSBytes']/mib,'baselineFootprintMiB':base['medianFootprintBytes']/mib,
            'maximumPhaseMedianFootprintIncrementMiB':max((r['medianFootprintBytes']-base['medianFootprintBytes'])/mib for r in records),
            'peakRSSMiB':max(r.get('lastSample',{}).get('peakRSSBytes',0)for r in result['records'])/mib}
(evidence/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
for case,data in summary.items():
    if 'finalWarmedIncrementMiB'in data:
        print(case,'peak delta',round(data['peakIncrementalFootprintMiB'],2),'settled delta',round(data['finalWarmedIncrementMiB'],2),'decay',data['decayAfterFinalStaticDraw'])
print('Archived diagnostic results:',len(summary))
