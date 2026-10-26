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
import shlex
import signal
import struct
import subprocess
import tempfile
import termios
import time
import pyte

parser = argparse.ArgumentParser()
parser.add_argument("artifact", type=Path)
parser.add_argument("--editor", action="store_true", help="Also verify a Go-owned editor handoff and terminal mode restoration")
parser.add_argument("--untrusted-file", action="store_true", help="Open terminal-control payloads as file content and verify they never reach the PTY as commands")
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
    payload = b"Visible file content\n\x1b]52;c;R0FUT1JfUFJJVkFURV9DTElQQk9BUkQ=\x07\x1b]0;GATOR_UNTRUSTED_TITLE\x07\x1b[2J\nRetained final line\n"
    if args.untrusted_file:
        (root / "untrusted.txt").write_bytes(payload)
    if args.editor:
        (root / "path/sh").symlink_to(shutil.which("sh"))
        editor = root / "editor.py"
        editor.write_text('''import json, os, sys, termios
from pathlib import Path
path = Path(sys.argv[1])
flags = termios.tcgetattr(sys.stdin.fileno())[3]
print("GATOR_EDITOR_READY", flush=True)
answer = input()
Path(os.environ["GATOR_EDITOR_RESULT"]).write_text(json.dumps({"canonical": bool(flags & termios.ICANON), "echo": bool(flags & termios.ECHO), "answer": answer, "before": path.read_text(), "temporary": str(path)}))
path.write_text("Editor returned draft 🐊\\nSecond retained line")
''')
        environment.update(VISUAL="", EDITOR=f"{shlex.quote(shutil.which('python3'))} {shlex.quote(str(editor))}", GATOR_EDITOR_RESULT=str(root / "editor-result.json"))
    master, slave = pty.openpty()
    original_modes = termios.tcgetattr(slave)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 110, 0, 0))
    child = subprocess.Popen([str(artifact / "bin/gator")], cwd=root, env=environment, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    output = bytearray()
    screen = pyte.Screen(110, 32)
    stream = pyte.ByteStream(screen)

    def visible(text):
        return text in "\n".join(screen.display)

    def wait_for(predicate, label, timeout=15):
        deadline = time.monotonic() + timeout
        while not predicate():
            if time.monotonic() > deadline:
                raise AssertionError(f"Timed out waiting for {label}; output tail: {output[-2000:]!r}")
            if select.select([master], [], [], 0.05)[0]:
                try:
                    chunk = os.read(master, 65536)
                    output.extend(chunk)
                    stream.feed(chunk)
                except OSError:
                    pass

    def saved_draft(needle="Installed draft"):
        for path in (root / "app/drafts").glob("*.json"):
            try:
                if any(needle in draft["text"] for draft in json.loads(path.read_text())["drafts"]):
                    return True
            except (ValueError, OSError):
                pass
        return False

    try:
        wait_for(lambda: visible("connected"), "connected Go session")
        os.write(master, b"/mo")
        wait_for(lambda: visible("Tab complete"), "inline slash completion")
        os.write(master, b"\t")
        wait_for(lambda: saved_draft("/mode "), "Tab completes without sending")
        os.write(master, b"\x15")
        wait_for(lambda: not saved_draft("/mode "), "clear completed command")
        os.write(master, "\x1b[200~Installed draft 🐊\nRetain across navigation\x1b[201~".encode())
        wait_for(saved_draft, "private persisted draft")
        os.write(master, b"\x10")  # Ctrl+P, independent of desktop key preferences.
        wait_for(lambda: visible("Gator commands"), "command palette")
        if args.editor:
            os.write(master, b"external-editor")
            wait_for(lambda: visible("external-editor"), "editor command")
            time.sleep(0.1)
            os.write(master, b"\r")
            wait_for(lambda: visible("GATOR_EDITOR_READY"), "interactive editor stdin")
            os.write(master, b"accept\n")
            wait_for(lambda: saved_draft("Editor returned draft"), "returned editor draft")
            result = json.loads((root / "editor-result.json").read_text())
            assert result["canonical"] and result["echo"], result
            assert result["answer"] == "accept" and "Installed draft" in result["before"], result
            assert not Path(result["temporary"]).exists(), result
            wait_for(lambda: not termios.tcgetattr(master)[3] & termios.ICANON, "raw input after editor")
            os.write(master, b"\x1b[200~ After editor input\x1b[201~")
            wait_for(lambda: saved_draft("After editor input"), "resumed composer input")
        else:
            os.write(master, b"\x1b")
            time.sleep(0.1)
        if args.untrusted_file:
            os.write(master, b"\x10")
            wait_for(lambda: visible("Gator commands"), "attachment command palette")
            os.write(master, b"attach-file")
            time.sleep(0.1)
            os.write(master, b"\r")
            wait_for(lambda: visible("Choose file"), "file picker")
            os.write(master, b"\x0c")
            wait_for(lambda: visible("relative path"), "path field")
            os.write(master, b"\x15" + str(root / "untrusted.txt").encode() + b"\r")
            wait_for(lambda: visible("untrusted.txt") and not visible("relative path"), "staged untrusted file")
            time.sleep(0.2)
            os.write(master, b"\x10")
            wait_for(lambda: visible("Gator commands"), "files command palette")
            os.write(master, b"files")
            time.sleep(0.1)
            os.write(master, b"\r")
            wait_for(lambda: visible("Gator › Files"), "Files command section")
            os.write(master, b"files")
            time.sleep(0.1)
            os.write(master, b"\r")
            wait_for(lambda: visible("Files in this conversation"), "retained file list")
            os.write(master, b"\r")
            wait_for(lambda: visible("Retained final line"), "complete untrusted file view")
            assert b"\x1b]52;c;R0FUT1JfUFJJVkFURV9DTElQQk9BUkQ=" not in output, "Untrusted OSC 52 reached terminal output"
            assert b"\x1b]0;GATOR_UNTRUSTED_TITLE" not in output, "Untrusted title command reached terminal output"
            assert (root / "untrusted.txt").read_bytes() == payload, "Viewing changed original file bytes"
            os.write(master, b"\x1b")
            time.sleep(0.1)
        os.write(master, b"\x11")
        wait_for(lambda: child.poll() is not None, "clean terminal exit", timeout=10)
        assert child.returncode == 0, child.returncode
        assert saved_draft("Editor returned draft" if args.editor else "Installed draft")
        assert termios.tcgetattr(master) == original_modes, "Terminal modes were not restored on exit"
        print("PASS: standalone production PTY, Go connection, Unicode/bracketed paste, inline completion, grouped palette, retained draft, clean exit and terminal mode restoration; no Electron/OpenCode server.")
        print("PASS: interactive Go-owned editor, canonical input/echo during handoff, saved text, temporary cleanup and resumed raw composer." if args.editor else "PATH contains Bun only.")
        if args.untrusted_file:
            print("PASS: keyboard file intake/view preserves source bytes and draft; file OSC clipboard/title payloads never become PTY commands.")
    finally:
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGKILL)
            child.wait()
        os.close(master)
