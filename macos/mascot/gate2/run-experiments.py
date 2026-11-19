#!/usr/bin/env python3
"""Sequential disposable renderer experiments; inspect only their own processes."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import time

parser = argparse.ArgumentParser()
parser.add_argument("--app", required=True)
parser.add_argument("--root", required=True)
parser.add_argument("experiments", nargs="+")
args = parser.parse_args()
root = Path(args.root)
root.mkdir(parents=True, exist_ok=True)
home = root / "disposable-home"
home.mkdir(exist_ok=True)
launch_env = {"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": str(home), "TMPDIR": "/tmp", "LANG": "en_US.UTF-8"}
for experiment in args.experiments:
    output = root / experiment
    if output.exists():
        raise SystemExit(f"Fresh output required: {output}")
    output.mkdir()
    revision = Path(args.app) / "Contents/Resources/revision.json"
    if revision.exists():
        (output / "run-revision.json").write_bytes(revision.read_bytes())
    subprocess.run(["open", "-n", "--stderr", str(output / "console.log"), args.app,
                    "--args", "--experiment", experiment, "--output", str(output)], check=True, env=launch_env)
    deadline = time.monotonic() + (850 if experiment == "endurance" else 350)
    previous, changed = None, time.monotonic()
    collected = set()
    while time.monotonic() < deadline:
        result = output / "results.json"
        if not result.exists():
            time.sleep(0.25)
            continue
        report = json.loads(result.read_text())
        phase = report["phaseReady"]
        if phase != previous:
            previous, changed = phase, time.monotonic()
            print(experiment, phase, "pid", report["pid"], flush=True)
        if report["done"]:
            errors = [r for r in report["records"] if "error" in r]
            if errors:
                raise SystemExit(str(errors))
            print(experiment, "DONE", round(report["elapsed"], 2), flush=True)
            break
        try:
            os.kill(report["pid"], 0)
        except ProcessLookupError:
            raise SystemExit(f"Target exited before a completed report: {experiment}, phase {phase}")
        if experiment == "attribution" and phase not in collected and time.monotonic() - changed >= 7:
            collected.add(phase)
            for tool in [["vmmap", "-summary", str(report["pid"])],
                         ["footprint", "--noCategories", "-p", str(report["pid"])] ]:
                try:
                    proc = subprocess.run(tool, capture_output=True, text=True, timeout=7)
                    (output / f"{phase}-{tool[0]}.txt").write_text(f"exit={proc.returncode}\n{proc.stdout}\n{proc.stderr}")
                except subprocess.TimeoutExpired:
                    (output / f"{phase}-{tool[0]}.txt").write_text("TIMEOUT after seven seconds; no permission/settings change attempted\n")
        time.sleep(0.5)
    else:
        raise SystemExit(f"Experiment deadline exceeded: {experiment}")
