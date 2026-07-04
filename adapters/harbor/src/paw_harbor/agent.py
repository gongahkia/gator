from __future__ import annotations

import os
import tempfile
from pathlib import Path
from typing import override

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext


class PawAgent(BaseInstalledAgent):
    @staticmethod
    @override
    def name() -> str:
        return "paw"

    @override
    def version(self) -> str | None:
        return "0.1.0"

    @override
    async def install(self, environment: BaseEnvironment) -> None:
        await self.exec_as_root(
            environment,
            command=(
                "apt-get update && "
                "apt-get install -y ripgrep git ca-certificates universal-ctags"
            ),
            env={"DEBIAN_FRONTEND": "noninteractive"},
        )
        await environment.upload_file(self._binary_path(), "/usr/local/bin/paw")
        await self.exec_as_root(environment, command="chmod +x /usr/local/bin/paw")

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        instruction_path = Path(self._write_instruction_file(instruction))
        try:
            await environment.upload_file(instruction_path, "/tmp/paw_instruction.txt")
        finally:
            instruction_path.unlink(missing_ok=True)

        await self.exec_as_agent(
            environment,
            command=(
                "mkdir -p /workspace/.paw && "
                "cd /workspace && "
                "PAW_NONINTERACTIVE=1 paw run "
                "--instruction-file /tmp/paw_instruction.txt "
                f"{self._run_flags()}"
                "--max-turns 40 "
                "--trace-file /workspace/.paw/trace.ndjson"
            ),
            env=self._paw_env(),
        )
        metadata = dict(context.metadata or {})
        metadata["paw_trace"] = "/workspace/.paw/trace.ndjson"
        context.metadata = metadata

    def _binary_path(self) -> Path:
        path = Path(os.environ.get("PAW_HARBOR_BINARY", "bin/paw-linux-amd64"))
        if not path.is_file():
            raise FileNotFoundError(f"paw binary not found: {path}")
        return path

    def _write_instruction_file(self, instruction: str) -> str:
        with tempfile.NamedTemporaryFile(
            "w", encoding="utf-8", prefix="paw-instruction-", delete=False
        ) as handle:
            handle.write(instruction)
            return handle.name

    def _paw_env(self) -> dict[str, str]:
        env = {
            key: value
            for key, value in os.environ.items()
            if key.startswith("PAW_") and key != "PAW_HARBOR_BINARY"
        }
        env.update(
            {
                key: value
                for key, value in self.extra_env.items()
                if key.startswith("PAW_") and key != "PAW_HARBOR_BINARY"
            }
        )
        env["PAW_NONINTERACTIVE"] = "1"
        return env

    def _run_flags(self) -> str:
        match self._paw_env().get("PAW_BENCH_CONFIG", "full"):
            case "full":
                return ""
            case "no-compress":
                return "--disable-compress "
            case "raw":
                return "--raw-context "
            case value:
                raise ValueError(f"unsupported PAW_BENCH_CONFIG: {value}")
