#!/usr/bin/env python3
"""Mac/Linux checkout launcher PTY check with an isolated retained conversation."""
import fcntl, json, os, pathlib, pty, select, shlex, shutil, struct, subprocess, tempfile, termios, time
repo = pathlib.Path(__file__).resolve().parents[2]
bun = shutil.which('bun') or str(repo / '.gator/native-tools/node_modules/.bin/bun')
launcher = repo / 'ui/launcher/gator.mjs'
with tempfile.TemporaryDirectory(prefix='gator-resume-') as temporary:
    root = pathlib.Path(temporary)
    workspace = root / 'workspace'; workspace.mkdir()
    config = root / 'config/gator'; config.mkdir(parents=True)
    (config / 'config.json').write_text(json.dumps({'version':4,'defaults':{'provider':'fixture','model':'fixture'},'custom_providers':[{'id':'fixture','base_url':'http://127.0.0.1:9','models':['fixture'],'default_model':'fixture'}]}))
    identity = 'work-resume-fixture'
    folder = root / 'state/gator/conversations' / identity; folder.mkdir(parents=True)
    (folder / 'conversation.json').write_text(json.dumps({'version':2,'id':identity,'title':'Resume fixture','source_path':str(workspace),'snapshot_id':'snap-resume','created_at':'2026-10-29T00:00:00Z','updated_at':'2026-10-29T00:00:00Z'}))
    env = {**os.environ, 'GATOR_STATE_DIR':str(root/'state'), 'GATOR_CONFIG_DIR':str(root/'config'), 'GATOR_TERMINAL_USER_DATA':str(root/'app'), 'GATOR_TERMINAL_WORKSPACE':str(workspace), 'TERM':'xterm-256color', 'GATOR_REDUCED_MOTION':'1'}
    def run(command, typed):
        master, slave = pty.openpty(); fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',32,110,0,0))
        process = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave, cwd=root, env=env, start_new_session=True); os.close(slave)
        output = bytearray()
        def read_until(predicate, label):
            deadline = time.monotonic()+25
            while not predicate():
                if time.monotonic()>deadline: raise AssertionError(label + ': ' + repr(bytes(output[-2500:])))
                if select.select([master],[],[],.05)[0]:
                    try: output.extend(os.read(master,65536))
                    except OSError: pass
        try:
            read_until(lambda: b'Resume fixture' in output and b'Enter sends' in output, 'requested conversation ready')
            time.sleep(.15)
            if typed.endswith(b"\r"):
                os.write(master, typed[:-1]); time.sleep(.3); os.write(master, b"\r")
            else: os.write(master, typed)
            read_until(lambda: process.poll() is not None and b'Resume this chat:' in output, 'clean shutdown and resume hint')
            assert process.returncode == 0, bytes(output[-2000:])
            # The hint follows terminal restoration, so it remains in the shell.
            hint = bytes(output).split(b'Resume this chat:')[-1].decode().strip()
            assert hint.endswith('resume '+identity), hint
            return shlex.split(hint)
        finally:
            if process.poll() is None: process.terminate(); process.wait(timeout=10)
            os.close(master)
    command = run([bun,str(launcher),'resume',identity], b'\x15/exit\r')
    command = run(command, b'\x15/quit\r')
    run(command, b'\x11')
    assert json.loads((folder/'conversation.json').read_text())['id'] == identity
    print('PASS: launcher resume opens the exact chat; /exit, /quit and Ctrl+Q exit and print a runnable resume command after terminal restoration.')
