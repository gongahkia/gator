"""Run the manual Qt-to-Python browser OAuth acceptance on an isolated account."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
from contextlib import suppress
from pathlib import Path
from time import monotonic, sleep
from typing import Any
from unittest.mock import patch
from urllib.request import Request, urlopen

from hcb.credentials import CLIENT_ID_KEY, REFRESH_TOKEN_KEY, read_environment
from hcb.models import Account
from hcb.paths import AppPaths
from hcb.storage import Storage

DEFAULT_ACCOUNT_ID = "qt-oauth-acceptance"
DEFAULT_TIMEOUT_SECONDS = 20 * 60


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--native", type=Path, required=True, help="Built Hot Cross Buns Qt executable."
    )
    parser.add_argument(
        "--env-file",
        type=Path,
        required=True,
        help="Fresh owner-only client-config file for the disposable account.",
    )
    parser.add_argument("--email", required=True, help="Expected disposable Google account email.")
    parser.add_argument(
        "--account", default=DEFAULT_ACCOUNT_ID, help="Temporary local account identifier."
    )
    parser.add_argument(
        "--timeout-seconds",
        type=int,
        default=DEFAULT_TIMEOUT_SECONDS,
        help="Maximum time to wait for browser approval (default: 1200).",
    )
    args = parser.parse_args()
    if not args.email.strip() or "@" not in args.email or args.email != args.email.strip():
        parser.error("--email must be a trimmed email address")
    if not args.account.strip() or any(character.isspace() for character in args.account):
        parser.error("--account must be non-empty and contain no whitespace")
    if args.timeout_seconds < 30:
        parser.error("--timeout-seconds must be at least 30")
    return args


def validate_client_config(path: Path) -> Path:
    """Accept only a fresh client config, without printing any credential values."""

    resolved = path.expanduser().resolve(strict=False)
    personal = Path("~/.config/hcb/personal.env").expanduser().resolve(strict=False)
    if resolved == personal:
        raise RuntimeError("the personal HCB credential file cannot be used for this acceptance")
    try:
        mode = stat.S_IMODE(resolved.stat().st_mode)
    except FileNotFoundError as exc:
        raise RuntimeError("the disposable OAuth client config does not exist") from exc
    if mode & (stat.S_IRWXG | stat.S_IRWXO):
        raise RuntimeError("the disposable OAuth client config is not owner-only")
    values = read_environment(resolved)
    if not values.get(CLIENT_ID_KEY, "").strip():
        raise RuntimeError("the disposable OAuth client config has no client ID")
    if REFRESH_TOKEN_KEY in values:
        raise RuntimeError("the disposable OAuth client config already has a refresh token")
    return resolved


def isolated_environment(root: Path, credential_file: Path, account_id: str) -> dict[str, str]:
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
            "HCB_ACCOUNT": account_id,
            "HCB_ENV_FILE": str(credential_file),
        }
    )
    return environment


def discovered_paths(environment: dict[str, str]) -> AppPaths:
    with patch.dict(os.environ, environment, clear=False):
        return AppPaths.discover()


def seed_account(paths: AppPaths, account_id: str, email: str) -> None:
    paths.ensure()
    with Storage(paths.database_file) as storage, storage.transaction():
        storage.upsert_account(Account(account_id, email))


def cli_command(*arguments: str) -> list[str]:
    return [sys.executable, "-m", "hcb.cli", *arguments]


def stop_process(process: subprocess.Popen[Any]) -> None:
    if process.poll() is not None:
        return
    with suppress(ProcessLookupError):
        os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        with suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=5)


def wait_for_descriptor(path: Path, bridge: subprocess.Popen[Any]) -> None:
    deadline = monotonic() + 10
    while monotonic() < deadline:
        if path.exists():
            if stat.S_IMODE(path.stat().st_mode) != 0o600:
                raise RuntimeError("the bridge descriptor is not owner-only")
            return
        if bridge.poll() is not None:
            raise RuntimeError("the Python bridge exited before it became ready")
        sleep(0.05)
    raise RuntimeError("the Python bridge did not become ready")


def bridge_authenticated(descriptor: Path, account_id: str) -> bool:
    payload = json.loads(descriptor.read_text(encoding="utf-8"))
    url = payload.get("url")
    token = payload.get("token")
    if not isinstance(url, str) or not isinstance(token, str):
        raise RuntimeError("the bridge descriptor is malformed")
    request = Request(
        f"{url}/v1/accounts/{account_id}/auth",
        headers={"Authorization": f"Bearer {token}"},
    )
    with urlopen(request, timeout=5) as response:  # noqa: S310 - private loopback descriptor
        result = json.loads(response.read())
    try:
        connected = result["data"]["authentication"]["connected"]
    except (KeyError, TypeError) as exc:
        raise RuntimeError("the bridge returned an invalid authentication state") from exc
    if not isinstance(connected, bool):
        raise RuntimeError("the bridge returned an invalid authentication state")
    return connected


def run_acceptance(
    native: Path, credential_file: Path, email: str, account_id: str, timeout: int
) -> None:
    if not native.is_file() or not os.access(native, os.X_OK):
        raise RuntimeError("the Qt executable is unavailable")
    root: Path
    with tempfile.TemporaryDirectory(prefix="hcb-qt-oauth-") as temporary:
        root = Path(temporary)
        environment = isolated_environment(root, credential_file, account_id)
        paths = discovered_paths(environment)
        seed_account(paths, account_id, email)
        descriptor = root / "bridge.json"
        bridge = subprocess.Popen(
            cli_command("bridge", "serve", "--ready-file", str(descriptor)),
            cwd=Path.cwd(),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=environment,
            start_new_session=True,
        )
        native_process: subprocess.Popen[Any] | None = None
        try:
            wait_for_descriptor(descriptor, bridge)
            native_process = subprocess.Popen(
                [
                    str(native),
                    "--bridge-descriptor",
                    str(descriptor),
                    "--bridge-account",
                    account_id,
                ],
                cwd=Path.cwd(),
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=environment,
                start_new_session=True,
            )
            print(
                "In the Qt window, choose Open Google setup, then Connect Google. "
                "Approve only the expected disposable account in the browser."
            )
            deadline = monotonic() + timeout
            authenticated = False
            while monotonic() < deadline:
                if native_process.poll() is not None:
                    raise RuntimeError("the Qt app closed before browser authorization completed")
                if bridge.poll() is not None:
                    raise RuntimeError(
                        "the Python bridge stopped before browser authorization completed"
                    )
                if bridge_authenticated(descriptor, account_id):
                    authenticated = True
                    break
                sleep(0.5)
            if not authenticated:
                raise RuntimeError("browser authorization did not complete before the timeout")
        finally:
            if native_process is not None:
                stop_process(native_process)
            stop_process(bridge)
            if descriptor.exists():
                raise RuntimeError("the bridge descriptor remained after shutdown")
    if root.exists():
        shutil.rmtree(root)
    if root.exists():
        raise RuntimeError("the isolated OAuth profile remained after cleanup")
    print(json.dumps({"qt_oauth_acceptance": {"authenticated": True}}))


def main() -> int:
    args = parse_args()
    credential_file = validate_client_config(args.env_file)
    run_acceptance(
        args.native.resolve(), credential_file, args.email, args.account, args.timeout_seconds
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
