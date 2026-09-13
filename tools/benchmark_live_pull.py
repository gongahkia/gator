#!/usr/bin/env python3
"""Measure live read-only Google pulls without touching the normal HCB database."""

from __future__ import annotations

import argparse
import json
import socket
import sys
import tempfile
from collections import defaultdict
from collections.abc import Callable
from contextlib import contextmanager
from dataclasses import asdict
from pathlib import Path
from time import monotonic, perf_counter
from typing import Any, cast

from hcb.auth import GoogleAuthenticator, TokenStore
from hcb.benchmarks import TimingSummary
from hcb.credentials import EncryptedFileTokenStore, load_client_config
from hcb.errors import GoogleApiError
from hcb.google_client import GoogleApiClient, GoogleGateway, Page
from hcb.models import Account
from hcb.storage import Storage
from hcb.sync import SyncEngine

ACCOUNT_ID = "live-benchmark"


class BenchmarkLimitError(RuntimeError):
    """Stop before an unexpectedly large account consumes unlimited requests."""


class TimedStorage(Storage):
    def __init__(self, path: Path) -> None:
        self.transaction_seconds = 0.0
        super().__init__(path)

    @contextmanager
    def transaction(self):
        started = perf_counter()
        with super().transaction() as connection:
            yield connection
        self.transaction_seconds += perf_counter() - started


class ReadOnlyTimedGateway:
    """Expose and time only the four list calls used by pull sync."""

    def __init__(self, client: GoogleApiClient, *, max_requests: int, deadline: float) -> None:
        self.client = client
        self.max_requests = max_requests
        self.deadline = deadline
        self.requests = 0
        self.phase = "initial"
        self.samples: dict[str, dict[str, list[tuple[float, int]]]] = defaultdict(
            lambda: defaultdict(list)
        )

    def _call(self, name: str, fetch: Callable[[], Page]) -> Page:
        if self.requests >= self.max_requests or monotonic() >= self.deadline:
            raise BenchmarkLimitError("live pull request or time budget exhausted")
        self.requests += 1
        started = perf_counter()
        try:
            page = fetch()
        except BaseException:
            self.samples[self.phase][name].append((perf_counter() - started, 0))
            raise
        self.samples[self.phase][name].append((perf_counter() - started, len(page.items)))
        if self.requests % 10 == 0:
            print(f"{self.phase}: {self.requests} read-only requests", file=sys.stderr, flush=True)
        return page

    def list_task_lists(self, *, page_token: str | None = None) -> Page:
        return self._call("task_lists", lambda: self.client.list_task_lists(page_token=page_token))

    def list_tasks(
        self,
        task_list_id: str,
        *,
        page_token: str | None = None,
        updated_min: str | None = None,
    ) -> Page:
        return self._call(
            "tasks",
            lambda: self.client.list_tasks(
                task_list_id, page_token=page_token, updated_min=updated_min
            ),
        )

    def list_calendars(
        self, *, page_token: str | None = None, sync_token: str | None = None
    ) -> Page:
        return self._call(
            "calendar_list",
            lambda: self.client.list_calendars(page_token=page_token, sync_token=sync_token),
        )

    def list_events(
        self,
        calendar_id: str,
        *,
        page_token: str | None = None,
        sync_token: str | None = None,
        time_min: str | None = None,
        time_max: str | None = None,
        single_events: bool = False,
    ) -> Page:
        return self._call(
            "events",
            lambda: self.client.list_events(
                calendar_id,
                page_token=page_token,
                sync_token=sync_token,
                time_min=time_min,
                time_max=time_max,
                single_events=single_events,
            ),
        )

    def summary(self, phase: str) -> dict[str, dict[str, Any]]:
        result = {}
        for name, samples in self.samples[phase].items():
            result[name] = {
                "requests": len(samples),
                "items": sum(count for _, count in samples),
                "latency": asdict(TimingSummary.from_samples([elapsed for elapsed, _ in samples])),
            }
        return result


def measure(args: argparse.Namespace) -> dict[str, Any]:
    credential_file = args.credential_file.expanduser().resolve()
    client_config = load_client_config(credential_file)
    token_store = EncryptedFileTokenStore(ACCOUNT_ID, credential_file, TokenStore())
    credentials = GoogleAuthenticator(client_config, token_store).credentials(ACCOUNT_ID)
    socket.setdefaulttimeout(args.request_timeout)
    client = GoogleApiClient(credentials)
    gateway = ReadOnlyTimedGateway(
        client,
        max_requests=args.max_requests,
        deadline=monotonic() + args.max_seconds,
    )
    with (
        tempfile.TemporaryDirectory(prefix="hcb-live-pull-") as directory,
        TimedStorage(Path(directory) / "pull.sqlite3") as storage,
    ):
        storage.upsert_account(Account(ACCOUNT_ID, "benchmark@example.invalid"))
        engine = SyncEngine(storage, cast(GoogleGateway, gateway), max_retries=1)
        stages = {}
        with engine.sync_ownership():
            for phase in ("initial", "incremental"):
                gateway.phase = phase
                transaction_start = storage.transaction_seconds
                started = perf_counter()
                task_result = engine.sync_task_lists(ACCOUNT_ID)
                task_seconds = perf_counter() - started
                started = perf_counter()
                calendar_result = engine.sync_calendars(ACCOUNT_ID)
                calendar_seconds = perf_counter() - started
                stages[phase] = {
                    "task_seconds": task_seconds,
                    "calendar_seconds": calendar_seconds,
                    "transaction_seconds": storage.transaction_seconds - transaction_start,
                    "pulled": task_result.pulled + calendar_result.pulled,
                    "requests": gateway.summary(phase),
                }
        return {
            "status": "complete",
            "limits": {
                "max_requests": args.max_requests,
                "max_seconds": args.max_seconds,
                "request_timeout": args.request_timeout,
            },
            "stages": stages,
            "rows": {
                "task_lists": len(storage.list_task_lists(ACCOUNT_ID)),
                "tasks": storage.connection.execute("SELECT COUNT(*) FROM tasks").fetchone()[0],
                "calendars": len(storage.list_calendars(ACCOUNT_ID)),
                "events": storage.connection.execute("SELECT COUNT(*) FROM events").fetchone()[0],
            },
            "total_requests": gateway.requests,
        }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--credential-file", type=Path, default=Path("~/.config/hcb/personal.env"))
    parser.add_argument("--max-requests", type=int, default=300)
    parser.add_argument("--max-seconds", type=int, default=600)
    parser.add_argument("--request-timeout", type=int, default=30)
    args = parser.parse_args()
    if min(args.max_requests, args.max_seconds, args.request_timeout) < 1:
        parser.error("all limits must be positive")
    try:
        print(json.dumps(measure(args), sort_keys=True))
    except (Exception, KeyboardInterrupt) as exc:
        # Never print remote IDs, payloads, credential values, or Google error messages.
        print(
            json.dumps(
                {
                    "status": "failed",
                    "error_type": type(exc).__name__,
                    "http_status": exc.status if isinstance(exc, GoogleApiError) else None,
                },
                sort_keys=True,
            )
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
