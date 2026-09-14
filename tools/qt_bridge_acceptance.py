"""Run the Qt bridge against a synthetic core while a separate core process writes."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tempfile
import threading
from pathlib import Path
from time import monotonic, sleep

from hcb.benchmarks import create_large_fixture
from hcb.desktop_bridge import DesktopBridge
from hcb.paths import AppPaths
from hcb.runtime import Runtime


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--native", type=Path, required=True, help="Built Hot Cross Buns Qt executable."
    )
    return parser.parse_args()


def wait_for(path: Path, process: subprocess.Popen[str], timeout_seconds: float) -> dict[str, int]:
    deadline = monotonic() + timeout_seconds
    while monotonic() < deadline:
        if path.exists():
            data = json.loads(path.read_text())
            if isinstance(data, dict) and all(
                isinstance(data.get(key), int) for key in ("tasks", "events")
            ):
                return {"tasks": data["tasks"], "events": data["events"]}
            raise RuntimeError("Qt bridge readiness probe returned malformed JSON")
        if process.poll() is not None:
            stdout, stderr = process.communicate()
            detail = stderr or stdout
            raise RuntimeError(
                f"Qt bridge process exited before readiness ({process.returncode}): {detail}"
            )
        sleep(0.05)
    process.terminate()
    stdout, stderr = process.communicate(timeout=5)
    raise RuntimeError(f"Qt bridge process timed out: {stderr or stdout}")


def main() -> None:
    args = parse_args()
    native = args.native.resolve()
    if not native.is_file() or not os.access(native, os.X_OK):
        raise SystemExit(f"native executable is unavailable: {native}")

    with tempfile.TemporaryDirectory(prefix="hcb-qt-bridge-") as temporary:
        root = Path(temporary)
        paths = AppPaths(root / "config", root / "data", root / "cache")
        paths.ensure()
        create_large_fixture(paths.database_file, task_count=1_000, event_count=1_500)
        descriptor = root / "bridge.json"
        ready = root / "qt-ready.json"
        bridge = DesktopBridge(lambda: Runtime(paths, environ={}))
        bridge.write_descriptor(descriptor)
        worker = threading.Thread(target=bridge.serve_forever, daemon=True)
        worker.start()
        try:
            environment = dict(os.environ)
            environment.update(
                {
                    "QT_QPA_PLATFORM": "offscreen",
                    "HCB_BRIDGE_READY_FILE": str(ready),
                }
            )
            process = subprocess.Popen(
                [
                    str(native),
                    "--bridge-descriptor",
                    str(descriptor),
                    "--bridge-account",
                    "benchmark",
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=environment,
            )
            writer = subprocess.run(
                [
                    sys.executable,
                    "-c",
                    (
                        "from pathlib import Path; "
                        "from hcb.paths import AppPaths; "
                        "from hcb.runtime import Runtime; "
                        "import sys; "
                        "paths = AppPaths(Path(sys.argv[1]), Path(sys.argv[2]), "
                        "Path(sys.argv[3])); "
                        "Runtime(paths, environ={}).application.create_task("
                        "'benchmark', 'inbox', 'Concurrent bridge acceptance task')"
                    ),
                    str(paths.config_dir),
                    str(paths.data_dir),
                    str(paths.cache_dir),
                ],
                cwd=Path.cwd(),
                capture_output=True,
                text=True,
                env=environment,
                check=False,
            )
            if writer.returncode:
                process.terminate()
                process.communicate(timeout=5)
                raise RuntimeError(
                    f"Concurrent core mutation failed: {writer.stderr or writer.stdout}"
                )
            loaded = wait_for(ready, process, 30)
            stdout, stderr = process.communicate(timeout=5)
            if process.returncode:
                raise RuntimeError(f"Qt bridge process failed: {stderr or stdout}")
            if loaded["tasks"] < 1_000:
                raise RuntimeError("Qt bridge loaded fewer tasks than the synthetic core contains")
            print(json.dumps({"qt_bridge_acceptance": loaded}, sort_keys=True))
        finally:
            bridge.shutdown()
            worker.join(timeout=5)
            bridge.close()


if __name__ == "__main__":
    main()
