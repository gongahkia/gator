#!/usr/bin/env python3
"""Exercise a CalendarGrid drag through Textual's real terminal input path.

The regular TUI tests post Textual messages directly. This smoke opens a pseudo
terminal, starts the Linux terminal driver, and sends SGR mouse press, motion,
and release sequences through its stdin. It is deliberately Linux/POSIX-only:
HCB's supported local development environment is Fedora.
"""

from __future__ import annotations

import argparse
import fcntl
import os
import pty
import select
import struct
import subprocess
import sys
import termios
import time
from datetime import UTC, date, datetime
from pathlib import Path
from typing import NoReturn

from textual.app import App, ComposeResult

from hcb.models import DateTimeKind, Event, EventDateTime
from hcb.tui import CalendarGrid

_STATUS_ENV = "HCB_CALENDAR_MOUSE_SMOKE_FD"
_WIDTH = 100
_HEIGHT = 40
_TIMEOUT_SECONDS = 10.0


class CalendarMouseSmokeApp(App[None]):
    """Minimal full-screen app which proves one terminal-originated drag."""

    CSS = """
    CalendarGrid {
        width: 100%;
        height: 100%;
    }
    """

    def __init__(self, status_fd: int) -> None:
        super().__init__()
        self._status_fd = status_fd
        self._outcome: str | None = None

    def compose(self) -> ComposeResult:
        yield CalendarGrid()

    def on_mount(self) -> None:
        grid = self.query_one(CalendarGrid)
        drag_item = Event(
            "mouse-drag",
            "local",
            "calendar",
            "Terminal drag",
            EventDateTime(DateTimeKind.DATETIME, datetime(2026, 9, 7, 9, tzinfo=UTC)),
            EventDateTime(DateTimeKind.DATETIME, datetime(2026, 9, 7, 11, tzinfo=UTC)),
        )
        grid.set_calendar(
            "Day",
            date(2026, 9, 7),
            (drag_item,),
            (),
            week_starts_on=0,
            time_zone="UTC",
            calendar_colors={"calendar": "#00aa00"},
            fallback_color="#888888",
            selected=None,
        )
        self.call_after_refresh(self._send_status, "ready")
        self.set_timer(5, lambda: self._finish("timed out waiting for drag"))

    def on_calendar_grid_item_changed(self, message: CalendarGrid.ItemChanged) -> None:
        if (
            message.item.item_id == "mouse-drag"
            and message.operation == "move"
            and message.day == date(2026, 9, 7)
            and message.minute == 13 * 60
        ):
            self._finish("passed")
            return
        self._finish(
            "unexpected drag result "
            f"operation={message.operation!r} day={message.day.isoformat()} "
            f"minute={message.minute!r}"
        )

    def _send_status(self, status: str) -> None:
        os.write(self._status_fd, f"{status}\n".encode())

    def _finish(self, outcome: str) -> None:
        if self._outcome is not None:
            return
        self._outcome = outcome
        self._send_status(outcome)
        self.exit()


def _run_child(status_fd: int) -> int:
    app = CalendarMouseSmokeApp(status_fd)
    app.run()
    if app._outcome is None:
        os.write(status_fd, b"app exited without a result\n")
        return 1
    return 0 if app._outcome == "passed" else 1


def _set_terminal_size(fd: int) -> None:
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", _HEIGHT, _WIDTH, 0, 0))


def _drain(fd: int, output: bytearray) -> bool:
    try:
        data = os.read(fd, 65_536)
    except OSError:
        return False
    output.extend(data)
    return bool(data)


def _wait_for_child_exit(
    process: subprocess.Popen[bytes], terminal_fd: int, output: bytearray
) -> None:
    deadline = time.monotonic() + _TIMEOUT_SECONDS
    terminal_open = True
    while process.poll() is None:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            _raise_smoke_failure("child did not exit after reporting passed", output)
        if terminal_open:
            readable, _, _ = select.select([terminal_fd], [], [], min(0.1, remaining))
            if readable:
                terminal_open = _drain(terminal_fd, output)
        else:
            try:
                process.wait(timeout=remaining)
            except subprocess.TimeoutExpired:
                _raise_smoke_failure("child did not exit after reporting passed", output)
    if process.returncode:
        _raise_smoke_failure(f"child exited {process.returncode}", output)


def _wait_for_status(
    process: subprocess.Popen[bytes],
    status_fd: int,
    terminal_fd: int,
    output: bytearray,
    expected: str,
    *,
    deadline: float,
) -> list[str]:
    """Drain the terminal while waiting for a status line from the child."""
    statuses: list[str] = []
    pending = b""
    while time.monotonic() < deadline:
        if process.poll() is not None and not pending:
            break
        readable, _, _ = select.select(
            [status_fd, terminal_fd], [], [], max(0.0, deadline - time.monotonic())
        )
        for fd in readable:
            if fd == terminal_fd:
                _drain(terminal_fd, output)
                continue
            chunk = os.read(status_fd, 4_096)
            if not chunk:
                continue
            pending += chunk
            while b"\n" in pending:
                line, pending = pending.split(b"\n", 1)
                status = line.decode(errors="replace")
                statuses.append(status)
                if status == expected:
                    return statuses
                if status != "ready":
                    return statuses
    return statuses


def _raise_smoke_failure(message: str, output: bytearray) -> NoReturn:
    transcript = output.decode(errors="replace")[-2_000:]
    raise RuntimeError(f"{message}\nterminal transcript tail:\n{transcript}")


def _run_parent() -> int:
    if os.name != "posix":
        raise RuntimeError("calendar mouse smoke requires a POSIX pseudo-terminal")

    terminal_master, terminal_slave = pty.openpty()
    status_read, status_write = os.pipe()
    _set_terminal_size(terminal_slave)
    os.set_inheritable(status_write, True)
    environment = os.environ | {
        _STATUS_ENV: str(status_write),
        "COLUMNS": str(_WIDTH),
        "LINES": str(_HEIGHT),
        "TERM": "xterm-256color",
    }
    process = subprocess.Popen(
        [sys.executable, str(Path(__file__).resolve()), "--child"],
        stdin=terminal_slave,
        stdout=terminal_slave,
        stderr=terminal_slave,
        close_fds=True,
        pass_fds=(status_write,),
        start_new_session=True,
        env=environment,
    )
    os.close(terminal_slave)
    os.close(status_write)
    output = bytearray()
    try:
        deadline = time.monotonic() + _TIMEOUT_SECONDS
        statuses = _wait_for_status(
            process,
            status_read,
            terminal_master,
            output,
            "ready",
            deadline=deadline,
        )
        if not statuses or statuses[-1] != "ready":
            _raise_smoke_failure(f"app did not become ready: {statuses!r}", output)

        # Coordinates are one-based terminal cells. The test event starts at
        # x=8/y=22 (zero-based); dragging an interior row to y=30 moves it
        # from 09:00 to 13:00. SGR 0/M is a left press, 32/M is a held-button
        # motion, and 0/m is the matching release.
        os.write(terminal_master, b"\x1b[<0;11;24M")
        os.write(terminal_master, b"\x1b[<32;11;31M")
        os.write(terminal_master, b"\x1b[<0;11;31m")
        statuses = _wait_for_status(
            process,
            status_read,
            terminal_master,
            output,
            "passed",
            deadline=deadline,
        )
        if not statuses or statuses[-1] != "passed":
            _raise_smoke_failure(f"drag did not pass: {statuses!r}", output)
        _wait_for_child_exit(process, terminal_master, output)
    finally:
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=2)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        os.close(status_read)
        os.close(terminal_master)
    print("calendar terminal mouse smoke passed")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--child", action="store_true")
    arguments = parser.parse_args()
    if arguments.child:
        fd = os.environ.get(_STATUS_ENV)
        if fd is None:
            parser.error(f"--child requires {_STATUS_ENV}")
        return _run_child(int(fd))
    return _run_parent()


if __name__ == "__main__":
    raise SystemExit(main())
