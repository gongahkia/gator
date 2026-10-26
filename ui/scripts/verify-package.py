#!/usr/bin/env python3
"""Unix installed-artifact PTY check; never reads the user's Gator profile."""
import argparse
import fcntl
import glob
import json
import os
from pathlib import Path
import pty
import select
import shutil
import signal
import struct
import subprocess
import tempfile
import termios
import time

parser = argparse.ArgumentParser()
parser.add_argument("artifact", type=Path)
args = parser.parse_args()
artifact = args.artifact.resolve()
assert not (artifact / "desktop").exists()
assert not (artifact / "ui/node_modules/electron").exists()
assert not (artifact / "ui/node_modules/@opencode/core").exists()
with tempfile.TemporaryDirectory(prefix="gator-installed-pty-") as temporary:
    root = Path(temporary)
    (root / "path").mkdir()
    (root / "path/bun").symlink_to(shutil.which("bun"))
    environment = dict(os.environ, PATH=str(root / "path"), GATOR_STATE_DIR=str(root / "state"), GATOR_CONFIG_DIR=str(root / "config"), GATOR_TERMINAL_USER_DATA=str(root / "app"))
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 110, 0, 0))
    child = subprocess.Popen([str(artifact / "bin/gator")], cwd=root, env=environment, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    output = bytearray()

    def wait_for(predicate, label, timeout=15):
        deadline = time.monotonic() + timeout
        while not predicate():
            if time.monotonic() > deadline:
                raise AssertionError(f"Timed out waiting for {label}; output tail: {output[-2000:]!r}")
            if select.select([master], [], [], 0.05)[0]:
                try:
                    output.extend(os.read(master, 65536))
                except OSError:
                    pass

    def saved_draft():
        for path in (root / "app/drafts").glob("*.json"):
            try:
                if any("Installed draft" in draft["text"] for draft in json.loads(path.read_text())["drafts"]):
                    return True
            except (ValueError, OSError):
                pass
        return False

    try:
        wait_for(lambda: b"connected" in output, "connected Go session")
        os.write(master, "\x1b[200~Installed draft 🐊\nRetain across navigation\x1b[201~".encode())
        wait_for(saved_draft, "private persisted draft")
        os.write(master, b"\x10")  # Ctrl+P, independent of desktop key preferences.
        wait_for(lambda: b"Gator commands" in output, "command palette")
        os.write(master, b"\x1b")
        time.sleep(0.1)
        os.write(master, b"\x11")
        wait_for(lambda: child.poll() is not None, "clean terminal exit", timeout=10)
        assert child.returncode == 0, child.returncode
        assert saved_draft()
        print("PASS: standalone production PTY, Go connection, Unicode/bracketed paste, palette, retained draft, clean exit; PATH contains Bun only; no Electron/OpenCode server.")
    finally:
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGKILL)
            child.wait()
        os.close(master)
