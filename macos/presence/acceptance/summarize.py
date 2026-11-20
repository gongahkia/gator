"""Read-only evidence aggregation. Does not drive the desktop."""
import datetime,hashlib,json,pathlib,platform,statistics,subprocess
root=pathlib.Path('macos/presence/evidence/p04e-1r')
calibration=json.loads((root/'cpu-units.json').read_text())
scale=calibration['machNumer']/calibration['machDenom']
rows=[]
for number in (3,4,5):
    metadata=json.loads((root/f'cycle-{number:02d}-fixture.json').read_text());fixture=pathlib.Path(metadata['root'])
    samples=[json.loads(s) for s in (root/f'cycle-{number:02d}-resources.jsonl').read_text().splitlines()]
    rates=[100*(b['cpuNanoseconds']-a['cpuNanoseconds'])*scale/1e9/(b['epoch']-a['epoch']) for a,b in zip(samples,samples[1:])]
    events=[];launches=[]
    for p in fixture.glob('bridge-*.jsonl'):
        records=[json.loads(x) for x in p.read_text().splitlines()];events+=records;launches += [x for x in records if x['direction']=='launch']
    decisions=[x for x in events if x.get('direction')=='command' and x.get('type')=='approval']
    interactions=[]
    for p in (fixture/'state/gator/interactions').glob('*/*.json'):
        value=json.loads(p.read_text());interactions.append({'path':str(p),'status':value['status'],'approved':value.get('approved'),'SHA256':hashlib.sha256(p.read_bytes()).hexdigest()})
    live=[]
    for x in launches:
        result=subprocess.run(['ps','-p',str(x['pid']),'-o','pid='],capture_output=True,text=True)
        if result.stdout.strip():live.append(x['pid'])
    rows.append({'cycle':number,'appPID':metadata['pid'],'sampleCount':len(samples),'sampledSeconds':samples[-1]['epoch']-samples[0]['epoch'] if samples else 0,'rssMiB':{'min':min(s['rss'] for s in samples)/2**20,'max':max(s['rss'] for s in samples)/2**20},'physicalFootprintMiB':{'min':min(s['footprint'] for s in samples)/2**20,'max':max(s['footprint'] for s in samples)/2**20,'lifetimePeak':max(s['peakFootprint'] for s in samples)/2**20},'cpuPercentOneCore':{'median':statistics.median(rates),'maxOneSecond':max(rates),'settledLast5Median':statistics.median(rates[-5:])},'approvalCommands':decisions,'durableInteractions':interactions,'GoLaunches':launches,'survivingGoPIDs':live,'GoExitStatuses':[x['status'] for x in events if x['direction']=='exit'],'acceptance':'ABORTED for foreign input' if number==5 else 'partial sequence only; no passive input audit'})
(root/'native-summary.json').write_text(json.dumps({'CPUCalibration':calibration,'samplerFieldWarning':'Original cpuNanoseconds field contains Mach ticks on this host; values converted by measured timebase','targetCleanEndToEndCycles':10,'certifiedCleanEndToEndCycles':0,'cycles':rows,'limitations':['Not matched performance trials: different durations and interaction patterns','Source-PID zero input is unattributed, not proven to be another automation process','No frame-by-frame/resource-owner instrumentation added to production','No new native recording completed before the user requested headless-only continuation']},indent=2))
counts={}
for line in (root/'go-approval.jsonl').read_text().splitlines():
    try:x=json.loads(line)
    except ValueError:continue
    if x.get('Test') and x.get('Action') in ['pass','fail','skip']:counts[x['Action']]=counts.get(x['Action'],0)+1
(root/'go-summary.json').write_text(json.dumps(counts,indent=2))
print(json.dumps({'nativeCycles':[{'cycle':r['cycle'],'peakMiB':r['physicalFootprintMiB']['lifetimePeak'],'medianCPU':r['cpuPercentOneCore']['median'],'decisions':len(r['approvalCommands']),'survivingGoPIDs':r['survivingGoPIDs']} for r in rows],'goTests':counts},indent=2))
