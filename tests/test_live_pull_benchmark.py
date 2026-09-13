import json
import runpy
import subprocess
import sys
from pathlib import Path
from time import monotonic

import pytest

from hcb.google_client import Page

TOOL = Path(__file__).resolve().parents[1] / "tools/benchmark_live_pull.py"


def test_live_pull_gateway_exposes_only_bounded_list_calls() -> None:
    namespace = runpy.run_path(str(TOOL), run_name="hcb_live_pull_test")
    gateway_class = namespace["ReadOnlyTimedGateway"]
    limit_error = namespace["BenchmarkLimitError"]

    class FakeClient:
        def list_task_lists(self, *, page_token=None):
            assert page_token is None
            return Page(({"id": "opaque"},))

    gateway = gateway_class(FakeClient(), max_requests=1, deadline=monotonic() + 10)
    assert not hasattr(gateway, "create_task")
    assert not hasattr(gateway, "delete_event")
    assert len(gateway.list_task_lists().items) == 1
    assert gateway.summary("initial")["task_lists"]["requests"] == 1
    with pytest.raises(limit_error):
        gateway.list_task_lists()


def test_live_pull_benchmark_redacts_credential_errors(tmp_path: Path) -> None:
    credential_file = tmp_path / "absent.env"
    completed = subprocess.run(
        [sys.executable, str(TOOL), "--credential-file", str(credential_file)],
        capture_output=True,
        text=True,
    )
    assert completed.returncode == 1
    assert json.loads(completed.stdout) == {
        "status": "failed",
        "error_type": "CredentialFileError",
        "http_status": None,
    }
    assert str(credential_file) not in completed.stdout + completed.stderr
