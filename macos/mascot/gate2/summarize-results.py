#!/usr/bin/env python3
"""Archive test-controlled measurements and summarize exact-build results."""
import hashlib
import json
from pathlib import Path
import re
import shutil
import statistics
import subprocess

evidence = Path(__file__).resolve().parent.parent / "evidence/p04m-a2-gate2"
for source, name in [(Path('/tmp/gator-gate2-profile-v2'), 'profile-v2'),
                     (Path('/tmp/gator-gate2-profile-v3'), 'profile-v3'),
                     (Path('/tmp/gator-gate2-profile-v4'), 'profile-v4'),
                     (Path('/tmp/gator-gate2-v10-final'), 'final-v10')]:
    for experiment in source.iterdir():
        if not experiment.is_dir() or experiment.name == 'disposable-home':
            continue
        destination = evidence / name / experiment.name
        destination.mkdir(parents=True, exist_ok=True)
        for item in experiment.iterdir():
            if item.is_file() and item.suffix in ['.json', '.ndjson', '.txt', '.log', '.png']:
                shutil.copy2(item, destination / item.name)

final = json.loads((evidence / 'final-v10/endurance/results.json').read_text())
records = final['records']
baseline = records[0]
mib = 1024 ** 2
summary = {'elapsedSeconds': final['elapsed'], 'frames': final['frames'],
           'created': final['created'], 'liveAfterTeardown': final['live'],
           'decodeCount': final['decodeCount'], 'phases': {}}
for label in ['static-idle', 'working', 'responding', 'compact', 'settled-idle', 'hidden', 'reduced-motion']:
    rows = [r for r in records if re.fullmatch(r'cycle-\d+-' + label, r.get('phase', ''))]
    seconds = sum(r['seconds'] for r in rows)
    summary['phases'][label] = {
        'phaseCount': len(rows),
        'cpuOneCorePercentWeightedMean': sum(r['cpuPercentOneCore'] * r['seconds'] for r in rows) / seconds,
        'rssIncrementMiBRange': [fn((r['medianRSSBytes'] - baseline['medianRSSBytes']) / mib for r in rows) for fn in [min, max]],
        'footprintIncrementMiBRange': [fn((r['medianFootprintBytes'] - baseline['medianFootprintBytes']) / mib for r in rows) for fn in [min, max]],
        'framesDuringSamples': sum(r['frames'] for r in rows),
        'maximumCheckpointRendererOwners': max(r['live'].get('RealityRenderer', 0) for r in rows),
        'pendingAnimationAtAnyCheckpoint': any(r['pendingAnimationTask'] or r['hasAnimationWork'] for r in rows)}
summary['checkpoints'] = {}
for name in ['A0-host-no-RealityKit-initialization', 'initialized-static', 'final-settled-idle', 'final-all-destroyed']:
    row = next(r for r in records if r.get('phase') == name)
    summary['checkpoints'][name] = {key: row[key] for key in ['cpuPercentOneCore', 'medianRSSBytes', 'medianFootprintBytes', 'medianHeapBytes', 'live', 'frames']}
totals = next(r for r in records if r.get('event') == 'presentationTotals')
resumes = sorted(totals['resumeMilliseconds'])
summary['boundedLastResumeSamples'] = {'count': len(resumes), 'medianMS': statistics.median(resumes),
                                     'p95MS': resumes[int(.95 * (len(resumes) - 1))], 'maxMS': max(resumes)}
summary['peakRSSMiB'] = max(r.get('lastSample', {}).get('peakRSSBytes', 0) for r in records) / mib
summary['controlledWindowClosures'] = sum(e.get('event') == 'windowWillClose' and e.get('controlled', False) for e in final['events'])
(evidence / 'results-summary.json').write_text(json.dumps(summary, indent=2) + '\n')

digest = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
baseline_record = json.loads((evidence / 'baseline.json').read_text())
changed = [name for name, old in baseline_record['files'].items() if not Path(name).is_file() or digest(Path(name)) != old]
bundle_changes = {}
for name, files in baseline_record['bundles'].items():
    root = Path(name)
    now = {str(p.relative_to(root)): digest(p) for p in root.rglob('*') if p.is_file()}
    bundle_changes[name] = [key for key in files.keys() | now.keys() if files.get(key) != now.get(key)]
source_bundle = Path('/tmp/Gator-Gate2-FinalCandidate-v10.app')
delivered = Path('/tmp/Gator-Mascot-P04M-A2-Gate2.app')
bundle_hashes = lambda root: {str(p.relative_to(root)): digest(p) for p in root.rglob('*') if p.is_file()}
revision = json.loads((delivered / 'Contents/Resources/revision.json').read_text())
runtime = ['Engine.swift', 'Metrics.swift', 'NativeMesh.swift', 'Presentation.swift', 'Gate2Lab.swift']
source_changes = [name for name in runtime if digest(Path(__file__).parent / name) != revision['sourceFiles'][name]]
preservation = {'startingHead': baseline_record['head'],
                'closingHead': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
                'protectedFileCount': len(baseline_record['files']), 'changedProtectedFiles': changed,
                'changedProtectedBundleFiles': bundle_changes,
                'deliveredBundleMatchesFinalCandidate': bundle_hashes(source_bundle) == bundle_hashes(delivered),
                'changedRuntimeSourcesSinceFinalBuild': source_changes,
                'finalRevision': revision}
(evidence / 'preservation.json').write_text(json.dumps(preservation, indent=2) + '\n')
print(json.dumps(summary, indent=2))
print('Preservation:', len(changed), 'changed protected files;', 'bundle changes', bundle_changes,
      'final bundle identical', preservation['deliveredBundleMatchesFinalCandidate'], 'runtime changes', source_changes)
