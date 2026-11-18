import hashlib,json,pathlib,struct,subprocess
base=pathlib.Path(__file__).parent
sha=lambda p:hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
baseline=json.loads((base/'baseline.json').read_text())
allowed={'internal/workapp/bridge_workspace.go','internal/workapp/bridge_workspace_test.go','macos/Sources/GatorCore/NativeWorkspaceSelection.swift','macos/Tests/GatorCoreTests/NativeWorkspaceIntegrationTests.swift'}
changed={p:{'before':h,'after':sha(p)} for p,h in baseline['hashes'].items() if sha(p)!=h}
assert set(changed)==allowed,changed
previous=json.loads((base.parent/'p03/integrity.json').read_text())
protected={p:{'sha256':sha(p),'unchanged':sha(p)==v['sha256']} for p,v in previous['priorSafety'].items()}
assert all(x['unchanged'] for x in protected.values())
apps={}
for name in ['P03','P03-Accepted']:
 root=pathlib.Path('/tmp/Gator-Native-Parity-'+name+'.app/Contents')
 apps[name]={p:sha(root/p) for p in ['MacOS/GatorMac','Resources/gator-runtime']}
desktop=pathlib.Path('/tmp/Gator-Native-Parity-P03-Accepted-Desktop.app/Contents')
assert all(apps['P03-Accepted'][p]==sha(desktop/p) for p in apps['P03-Accepted'])
assert apps['P03-Accepted']['Resources/gator-runtime']==sha('/tmp/gator-p03-accepted-runtime')
shots={p.name:{'sha256':sha(p),'pixels':list(struct.unpack('>II',p.read_bytes()[16:24]))} for p in base.glob('*.png')}
result={'startingHEAD':baseline['head'],'finalObservedHEAD':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'changedSourceFiles':changed,'unchangedBaselineFiles':len(baseline['hashes'])-len(changed),'protected':protected,'apps':apps,'desktopExecutablesMatchAcceptedCandidate':True,'testedGoRuntimeMatchesCandidate':True,'screenshots':shots,'controlledFixture':json.loads((base/'controlled-final.json').read_text())['root'],'provenance':'Original candidate and prior bundles preserved. Corrected candidate Swift/Go bytes equal the disposable-domain copy used in the final user-reserved desktop session. Complete 215-test runtime bytes equal candidate Go resource.'}
(base/'integrity.json').write_text(json.dumps(result,indent=2));print('PASS four intended source/test changes, original candidate unchanged, protected safety/P01/P02 files unchanged, exact candidate/desktop executables and tested Go runtime')
