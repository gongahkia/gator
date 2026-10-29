#!/usr/bin/env python3
"""Check unattended companion/supervisor startup across two isolated Linux boots.

Requires Docker with privileged containers and a Linux Gator executable matching
the Docker engine's architecture. No host mounts, ports, accounts or services are
used. Only this disposable container's user has lingering enabled.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import time


def run(*args, timeout=60):
    result = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"Command failed ({result.returncode}): {' '.join(map(str, args))}\n{result.stdout}")
    return result.stdout


def until(read, message, timeout=30):
    deadline = time.monotonic() + timeout
    error = None
    while time.monotonic() < deadline:
        try:
            return read()
        except RuntimeError as caught:
            error = caught
            time.sleep(0.2)
    raise RuntimeError(message) from error


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("runtime", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    runtime, output = args.runtime.resolve(strict=True), args.output.resolve()
    repo = Path(__file__).resolve().parents[2]
    output.mkdir(parents=True, exist_ok=False)
    image_file = output / "image-id.txt"
    container = None
    report = {"scope": "Disposable Linux systemd container restart; not physical-host reboot, remote deployment or phone acceptance.", "runtime_sha256": hashlib.sha256(runtime.read_bytes()).hexdigest(), "boots": []}
    try:
        # Save build output incrementally; avoid collecting apt output in memory.
        with (output / "build.log").open("w") as log:
            subprocess.run(["docker", "build", "--iidfile", str(image_file), str(repo / "scripts/companion-boot")], check=True, stdout=log, stderr=subprocess.STDOUT, timeout=600)
        image = image_file.read_text().strip()
        container = run("docker", "run", "--detach", "--privileged", "--cgroupns", "private", "--network", "none", "--tmpfs", "/run", "--tmpfs", "/run/lock", "--tmpfs", "/tmp:exec,mode=1777", image).strip()
        print(f"Disposable container: {container}", flush=True)
        root = lambda *command: run("docker", "exec", container, *command)
        user = lambda *command, timeout=60: run("docker", "exec", "-u", "node", "-e", "XDG_RUNTIME_DIR=/run/user/1000", container, *command, timeout=timeout)
        report["versions"] = {"image": image, "systemd": root("systemctl", "--version").splitlines()[0], "node": root("node", "--version").strip(), "docker": run("docker", "version", "--format", "{{.Server.Version}}").strip()}
        until(lambda: root("loginctl", "enable-linger", "node"), "Container login manager did not start.")
        until(lambda: user("systemctl", "--user", "show-environment"), "Lingering did not start the user manager.")
        root("mkdir", "-p", "/validation/app-host", "/validation/scripts")
        for source, destination in [(repo / "companion", "/validation/companion"), (repo / "app-host/src", "/validation/app-host/src"), (repo / "scripts/companion-boot", "/validation/scripts/companion-boot"), (runtime, "/validation/gator-runtime")]:
            run("docker", "cp", str(source), container + ":" + destination)
        phases = "/validation/scripts/companion-boot/phases.mjs"
        setup = user("node", phases, "setup", timeout=120)
        (output / "setup.log").write_text(setup)
        print(setup, end="", flush=True)
        for phase in ("first", "second"):
            # /run is a fresh tmpfs after restart; this proves the test did not
            # merely restart the listeners or reuse their existing user manager.
            root("touch", "/run/gator-before-restart")
            before = root("systemctl", "show", "user@1000.service", "--property=InvocationID", "--value").strip()
            run("docker", "restart", "--timeout", "30", container, timeout=60)
            until(lambda: user("systemctl", "--user", "show-environment"), "User manager did not start after boot.")
            root("test", "!", "-e", "/run/gator-before-restart")
            after = root("systemctl", "show", "user@1000.service", "--property=InvocationID", "--value").strip()
            if not before or not after or before == after:
                raise AssertionError("User manager did not restart")
            if root("loginctl", "show-user", "node", "--property=Linger", "--value").strip() != "yes":
                raise AssertionError("Lingering is not enabled")
            if root("loginctl", "show-user", "node", "--property=Sessions", "--value").strip():
                raise AssertionError("Unexpected login session; unattended availability is unproven")
            # No systemctl start or login is issued here: verify automatic startup.
            result = user("node", phases, phase, timeout=120)
            (output / (phase + ".log")).write_text(result)
            print(result, end="", flush=True)
            evidence = json.loads(user("cat", f"/home/node/fixture/{phase}.json"))
            evidence.update(user_manager_invocation=after, login_sessions=0, linger=True, fresh_runtime_directory=True)
            report["boots"].append(evidence)
        report["result"] = "passed"
    except Exception as error:
        report["result"] = "failed"
        report["error"] = str(error)
        raise
    finally:
        if container:
            try:
                (output / "journal.log").write_text(run("docker", "exec", container, "journalctl", "--no-pager", "-n", "300"))
            except Exception as error:
                (output / "diagnostic-error.txt").write_text(str(error))
            try:
                run("docker", "rm", "--force", container)
                report["container_removed"] = True
                print("Removed disposable container.", flush=True)
            finally:
                (output / "boot-report.json").write_text(json.dumps(report, indent=2) + "\n")
        else:
            (output / "boot-report.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
