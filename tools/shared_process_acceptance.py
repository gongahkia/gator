"""Exercise Qt, the CLI, a terminal TUI, reminders, and sync locking on one synthetic core."""

from __future__ import annotations

import argparse
import fcntl
import json
import os
import pty
import select
import signal
import stat
import struct
import subprocess
import sys
import tempfile
import termios
from contextlib import suppress
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from pathlib import Path
from time import monotonic, sleep
from unittest.mock import patch

from hcb.benchmarks import create_large_fixture
from hcb.errors import SyncBusyError
from hcb.models import Calendar, DateTimeKind, Event, EventDateTime, ReminderOverride
from hcb.paths import AppPaths
from hcb.storage import Storage
from hcb.sync import SyncEngine

ACCOUNT_ID = "benchmark"
TUI_SENTINEL = b"Deterministic task 00000"


@dataclass
class TerminalTui:
    process: subprocess.Popen[bytes]
    master: int
    closed: bool = False


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--native", type=Path, help="Built Hot Cross Buns Qt executable for the parent acceptance."
    )
    parser.add_argument(
        "--hold-sync-lock",
        action="store_true",
        help=argparse.SUPPRESS,
    )
    parser.add_argument("--database", type=Path, help=argparse.SUPPRESS)
    parser.add_argument("--ready-file", type=Path, help=argparse.SUPPRESS)
    parser.add_argument("--release-file", type=Path, help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.hold_sync_lock:
        if not all((args.database, args.ready_file, args.release_file)):
            parser.error("--hold-sync-lock requires the hidden database, ready, and release paths")
    elif args.native is None:
        parser.error("--native is required")
    return args


def isolated_environment(root: Path) -> dict[str, str]:
    environment = dict(os.environ)
    for name in tuple(environment):
        if name.startswith("HCB_GOOGLE_"):
            environment.pop(name)
    for name in ("HCB_ACCOUNT", "HCB_ENV_FILE", "HCB_PROFILE", "NO_COLOR"):
        environment.pop(name, None)
    environment.update(
        {
            "XDG_CONFIG_HOME": str(root / "config"),
            "XDG_DATA_HOME": str(root / "data"),
            "XDG_CACHE_HOME": str(root / "cache"),
            "HCB_ACCOUNT": ACCOUNT_ID,
            "HCB_ENV_FILE": str(root / "no-credentials.env"),
            "TERM": "xterm-256color",
            "COLUMNS": "120",
            "LINES": "40",
        }
    )
    return environment


def discovered_paths(environment: dict[str, str]) -> AppPaths:
    with patch.dict(os.environ, environment, clear=False):
        return AppPaths.discover()


def write_fixture(paths: AppPaths) -> None:
    paths.ensure()
    create_large_fixture(paths.database_file, task_count=1_000, event_count=1_500)
    started = datetime.now(UTC) - timedelta(minutes=1)
    with Storage(paths.database_file) as storage, storage.transaction():
        storage.upsert_calendar(
            Calendar(
                "reminder",
                ACCOUNT_ID,
                "Synthetic reminders",
                time_zone="UTC",
                default_reminders=(ReminderOverride("popup", 0),),
            )
        )
        storage.upsert_event(
            Event(
                "reminder-event",
                ACCOUNT_ID,
                "reminder",
                "Synthetic shared-process reminder",
                EventDateTime(DateTimeKind.DATETIME, started, "UTC"),
                EventDateTime(DateTimeKind.DATETIME, started + timedelta(minutes=30), "UTC"),
            )
        )


def cli_command(*arguments: str) -> list[str]:
    return [sys.executable, "-m", "hcb.cli", *arguments]


def stop_process(process: subprocess.Popen[object], *, timeout_seconds: float = 5) -> None:
    if process.poll() is not None:
        return
    with suppress(ProcessLookupError):
        process.terminate()
    try:
        process.wait(timeout=timeout_seconds)
    except subprocess.TimeoutExpired:
        with suppress(ProcessLookupError):
            process.kill()
        process.wait(timeout=timeout_seconds)


def wait_for_descriptor(path: Path, process: subprocess.Popen[object]) -> None:
    deadline = monotonic() + 10
    while monotonic() < deadline:
        if path.exists():
            if stat.S_IMODE(path.stat().st_mode) != 0o600:
                raise RuntimeError("bridge descriptor is not owner-only")
            payload = json.loads(path.read_text())
            if (
                not isinstance(payload, dict)
                or payload.get("api_version") != 1
                or not isinstance(payload.get("token"), str)
                or not isinstance(payload.get("url"), str)
                or not payload["url"].startswith("http://127.0.0.1:")
            ):
                raise RuntimeError("bridge descriptor is malformed")
            return
        if process.poll() is not None:
            raise RuntimeError("bridge process exited before writing its descriptor")
        sleep(0.05)
    raise RuntimeError("bridge process did not write its descriptor")


def start_bridge(descriptor: Path, environment: dict[str, str]) -> subprocess.Popen[str]:
    process = subprocess.Popen(
        cli_command("bridge", "serve", "--ready-file", str(descriptor)),
        cwd=Path.cwd(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=environment,
        start_new_session=True,
    )
    wait_for_descriptor(descriptor, process)
    return process


def start_tui(environment: dict[str, str]) -> TerminalTui:
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    try:
        process = subprocess.Popen(
            [sys.executable, "-c", "from hcb.cli import main; main()"],
            cwd=Path.cwd(),
            stdin=slave,
            stdout=slave,
            stderr=slave,
            env=environment,
            start_new_session=True,
        )
    finally:
        os.close(slave)
    deadline = monotonic() + 30
    output = bytearray()
    while monotonic() < deadline:
        ready, _, _ = select.select([master], [], [], 0.2)
        if ready:
            try:
                chunk = os.read(master, 65_536)
            except OSError:
                break
            if not chunk:
                break
            output.extend(chunk)
            if TUI_SENTINEL in output:
                return TerminalTui(process, master)
            if len(output) > 131_072:
                del output[:-65_536]
        if process.poll() is not None:
            break
    stop_process(process)
    os.close(master)
    raise RuntimeError("terminal TUI did not render the synthetic task frame")


def stop_tui(tui: TerminalTui) -> None:
    if tui.closed:
        return
    if tui.process.poll() is None:
        with suppress(OSError):
            os.write(tui.master, b"q")
        try:
            tui.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            with suppress(ProcessLookupError):
                os.killpg(tui.process.pid, signal.SIGTERM)
            tui.process.wait(timeout=5)
    with suppress(OSError):
        os.close(tui.master)
    tui.closed = True
    if tui.process.returncode != 0:
        raise RuntimeError(f"terminal TUI did not shut down cleanly (exit {tui.process.returncode})")


def wait_for_qt_ready(path: Path, process: subprocess.Popen[str]) -> dict[str, int]:
    deadline = monotonic() + 30
    while monotonic() < deadline:
        if path.exists():
            payload = json.loads(path.read_text())
            if isinstance(payload, dict) and all(
                isinstance(payload.get(key), int) for key in ("tasks", "events")
            ):
                return {"tasks": payload["tasks"], "events": payload["events"]}
            raise RuntimeError("Qt bridge readiness probe returned malformed JSON")
        if process.poll() is not None:
            raise RuntimeError("Qt bridge process exited before readiness")
        sleep(0.05)
    raise RuntimeError("Qt bridge process did not finish its initial load")


def run_sync_lock_holder(database: Path, ready: Path, release: Path) -> None:
    with Storage(database) as storage:
        engine = SyncEngine(storage, None)  # type: ignore[arg-type]
        with engine.sync_ownership():
            ready.write_text("ready")
            deadline = monotonic() + 15
            while not release.exists():
                if monotonic() >= deadline:
                    raise RuntimeError("sync-lock parent did not release the holder")
                sleep(0.05)


def verify_sync_lock_handoff(paths: AppPaths, environment: dict[str, str], root: Path) -> None:
    ready = root / "sync-lock-ready"
    release = root / "sync-lock-release"
    holder = subprocess.Popen(
        [
            sys.executable,
            str(Path(__file__).resolve()),
            "--hold-sync-lock",
            "--database",
            str(paths.database_file),
            "--ready-file",
            str(ready),
            "--release-file",
            str(release),
        ],
        cwd=Path.cwd(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=environment,
        start_new_session=True,
    )
    try:
        deadline = monotonic() + 10
        while not ready.exists():
            if holder.poll() is not None:
                raise RuntimeError("sync-lock holder exited before acquiring the lock")
            if monotonic() >= deadline:
                raise RuntimeError("sync-lock holder did not become ready")
            sleep(0.05)
        with Storage(paths.database_file) as storage:
            engine = SyncEngine(storage, None)  # type: ignore[arg-type]
            try:
                with engine.sync_ownership():
                    raise RuntimeError("second process acquired an active sync lock")
            except SyncBusyError:
                pass
        release.touch()
        if holder.wait(timeout=10) != 0:
            raise RuntimeError("sync-lock holder did not exit cleanly")
        with Storage(paths.database_file) as storage:
            engine = SyncEngine(storage, None)  # type: ignore[arg-type]
            with engine.sync_ownership():
                pass
    finally:
        stop_process(holder)


def run_acceptance(native: Path) -> dict[str, int | bool]:
    if not native.is_file() or not os.access(native, os.X_OK):
        raise RuntimeError(f"native executable is unavailable: {native}")
    with tempfile.TemporaryDirectory(prefix="hcb-shared-process-") as temporary:
        root = Path(temporary)
        environment = isolated_environment(root)
        Path(environment["HCB_ENV_FILE"]).touch(mode=0o600)
        paths = discovered_paths(environment)
        write_fixture(paths)
        descriptor = root / "bridge.json"
        ready = root / "qt-ready.json"
        bridge: subprocess.Popen[str] | None = None
        tui: TerminalTui | None = None
        qt: subprocess.Popen[str] | None = None
        try:
            bridge = start_bridge(descriptor, environment)
            tui = start_tui(environment)
            qt_environment = dict(environment)
            qt_environment.update(
                {"QT_QPA_PLATFORM": "offscreen", "HCB_BRIDGE_READY_FILE": str(ready)}
            )
            qt = subprocess.Popen(
                [
                    str(native),
                    "--bridge-descriptor",
                    str(descriptor),
                    "--bridge-account",
                    ACCOUNT_ID,
                ],
                cwd=Path.cwd(),
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=qt_environment,
                start_new_session=True,
            )
            writer = subprocess.Popen(
                cli_command(
                    "--json",
                    "--account",
                    ACCOUNT_ID,
                    "tasks",
                    "create",
                    "Synthetic concurrent CLI task",
                    "--list",
                    "inbox",
                ),
                cwd=Path.cwd(),
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=environment,
                start_new_session=True,
            )
            reminder = subprocess.Popen(
                cli_command("--json", "--account", ACCOUNT_ID, "daemon", "run", "--once"),
                cwd=Path.cwd(),
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=environment,
                start_new_session=True,
            )
            if writer.wait(timeout=15) != 0:
                raise RuntimeError("synthetic CLI mutation process failed")
            if reminder.wait(timeout=15) != 0:
                raise RuntimeError("synthetic reminder process failed")
            loaded = wait_for_qt_ready(ready, qt)
            if qt.wait(timeout=10) != 0:
                raise RuntimeError("Qt bridge process failed")
            if tui.process.poll() is not None:
                raise RuntimeError("terminal TUI exited during concurrent core activity")
            with Storage(paths.database_file) as storage:
                matching = [
                    task
                    for task in storage.list_tasks(ACCOUNT_ID)
                    if task.title == "Synthetic concurrent CLI task"
                ]
                deliveries = storage.reminder_delivery_rows(ACCOUNT_ID)
            if len(matching) != 1:
                raise RuntimeError("synthetic CLI mutation was not persisted exactly once")
            if not any(row.get("delivered_at") is not None for row in deliveries):
                raise RuntimeError("synthetic reminder delivery was not recorded")
            verify_sync_lock_handoff(paths, environment, root)
            stop_tui(tui)
            tui = None
            stop_process(bridge)
            bridge = None
            if descriptor.exists():
                raise RuntimeError("bridge descriptor remained after clean shutdown")
            restarted = start_bridge(descriptor, environment)
            stop_process(restarted)
            if descriptor.exists():
                raise RuntimeError("restarted bridge descriptor remained after shutdown")
            if loaded["tasks"] < 1_000 or loaded["events"] < 1:
                raise RuntimeError("Qt bridge did not load the synthetic workspace")
            return {
                "qt_tasks": loaded["tasks"],
                "qt_events": loaded["events"],
                "reminder_deliveries": len(deliveries),
                "terminal_tui": True,
                "cli_mutation": True,
                "sync_lock_handoff": True,
                "bridge_restart": True,
            }
        finally:
            if qt is not None:
                stop_process(qt)
            if tui is not None:
                with suppress(RuntimeError):
                    stop_tui(tui)
            if bridge is not None:
                stop_process(bridge)


def main() -> int:
    args = parse_args()
    if args.hold_sync_lock:
        run_sync_lock_holder(args.database, args.ready_file, args.release_file)
        return 0
    result = run_acceptance(args.native.resolve())
    print(json.dumps({"shared_process_acceptance": result}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
