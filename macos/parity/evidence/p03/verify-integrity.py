import hashlib,json,pathlib,struct,subprocess
base=pathlib.Path(__file__).parent
sha=lambda p:hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
baseline=json.loads((base/'baseline.json').read_text())
checks={p:{'sha256':sha(p),'unchanged':sha(p)==h} for p,h in baseline['hashes'].items()}
assert all(x['unchanged'] for x in checks.values()),'Protected source or prior bundle changed'
prior=json.loads((base.parent/'p02/integrity.json').read_text())
safety={p:{'sha256':sha(p),'unchangedFromP02':sha(p)==v['sha256']} for p,v in prior['protected'].items()}
assert all(x['unchangedFromP02'] for x in safety.values())
app=pathlib.Path('/tmp/Gator-Native-Parity-P03.app/Contents');desktop=pathlib.Path('/tmp/Gator-Native-Parity-P03-Desktop.app/Contents')
executables=['MacOS/GatorMac','Resources/gator-runtime']
apps={p:{'candidate':sha(app/p),'desktop':sha(desktop/p)} for p in executables}
assert all(v['candidate']==v['desktop'] for v in apps.values())
shots={p.name:{'sha256':sha(p),'size':list(struct.unpack('>II',p.read_bytes()[16:24]))} for p in base.glob('*.png')}
coordinator=pathlib.Path('macos/Sources/GatorCore/ManagedSessionCoordinator.swift').read_text()
closing=coordinator[coordinator.index('public func shutdown(confirmed:'):]
assert closing.index('for owner in records.values { owner.beginClosing() }')<closing.index('await catalog.finishActions()')
files=['cmd/gator/ui_bridge.go','cmd/gator/ui_bridge_test.go','internal/workapp/app.go','internal/workapp/bridge.go','internal/workapp/update.go','internal/workapp/bridge_workspace.go','internal/workapp/bridge_workspace_test.go','macos/Sources/GatorCore/ManagedSessionCoordinator.swift','macos/Sources/GatorCore/ManagedSessionProtocol.swift','macos/Sources/GatorCore/NativeWorkspaceSelection.swift','macos/Sources/GatorMac/ConversationView.swift','macos/Sources/GatorMac/GatorApp.swift','macos/Sources/GatorMac/NativeWorkspacePicker.swift','macos/Tests/GatorCoreTests/ManagedSessionTests.swift','macos/Tests/GatorCoreTests/ManagedGoIntegrationTests.swift','macos/Tests/GatorCoreTests/NativeWorkspaceTests.swift','macos/Tests/GatorCoreTests/NativeWorkspaceIntegrationTests.swift','macos/Tests/GatorMacTests/NativeWorkspacePickerTests.swift']
result={'startingHEAD':baseline['head'],'observedHEAD':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'protected':checks,'priorSafety':safety,'apps':apps,'screenshots':shots,'sourceHashes':{p:sha(p) for p in files},'closingBarrier':'unchanged synchronous all-owner boundary before await'}
(base/'integrity.json').write_text(json.dumps(result,indent=2));print('PASS protected sources, safety tests, P01/P02 bundles, final desktop executables and closing barrier')
