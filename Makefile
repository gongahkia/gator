SHELL := /bin/sh

.PHONY: sync format format-check lint typecheck test check build benchmark benchmark-desktop-bridge qt-bridge-acceptance qt-bridge-interaction-acceptance qt-bridge-failure-acceptance shared-process-acceptance calendar-mouse-smoke local-ci install-hooks clean

sync:
	uv sync --extra dev

format:
	uv run ruff format src tests tools/benchmark_python.py tools/benchmark_outbox.py tools/benchmark_pull.py tools/benchmark_live_pull.py tools/benchmark_tui_startup.py tools/benchmark_desktop_bridge.py tools/qt_bridge_acceptance.py tools/qt_bridge_interaction_acceptance.py tools/qt_bridge_failure_acceptance.py tools/qt_oauth_acceptance.py tools/shared_process_acceptance.py tools/calendar_mouse_smoke.py
	uv run ruff check --fix src tests tools/benchmark_python.py tools/benchmark_outbox.py tools/benchmark_pull.py tools/benchmark_live_pull.py tools/benchmark_tui_startup.py tools/benchmark_desktop_bridge.py tools/qt_bridge_acceptance.py tools/qt_bridge_interaction_acceptance.py tools/qt_bridge_failure_acceptance.py tools/qt_oauth_acceptance.py tools/shared_process_acceptance.py tools/calendar_mouse_smoke.py

format-check:
	uv run ruff format --check src tests tools/benchmark_python.py tools/benchmark_outbox.py tools/benchmark_pull.py tools/benchmark_live_pull.py tools/benchmark_tui_startup.py tools/benchmark_desktop_bridge.py tools/qt_bridge_acceptance.py tools/qt_bridge_interaction_acceptance.py tools/qt_bridge_failure_acceptance.py tools/qt_oauth_acceptance.py tools/shared_process_acceptance.py tools/calendar_mouse_smoke.py

lint:
	uv run ruff check src tests tools/benchmark_python.py tools/benchmark_outbox.py tools/benchmark_pull.py tools/benchmark_live_pull.py tools/benchmark_tui_startup.py tools/benchmark_desktop_bridge.py tools/qt_bridge_acceptance.py tools/qt_bridge_interaction_acceptance.py tools/qt_bridge_failure_acceptance.py tools/qt_oauth_acceptance.py tools/shared_process_acceptance.py tools/calendar_mouse_smoke.py

typecheck:
	uv run mypy src

test:
	PYTHONDONTWRITEBYTECODE=1 uv run pytest

check: format-check lint typecheck test

build:
	uv build

benchmark:
	PYTHONDONTWRITEBYTECODE=1 uv run python tools/benchmark_python.py

benchmark-desktop-bridge:
	PYTHONDONTWRITEBYTECODE=1 uv run python tools/benchmark_desktop_bridge.py

qt-bridge-acceptance:
	PYTHONDONTWRITEBYTECODE=1 uv run python tools/qt_bridge_acceptance.py --native "/tmp/hcb-qt-tests/native/Hot Cross Buns"

qt-bridge-interaction-acceptance:
	PYTHONDONTWRITEBYTECODE=1 uv run python tools/qt_bridge_interaction_acceptance.py --native "/tmp/hcb-qt-tests/native/Hot Cross Buns"

qt-bridge-failure-acceptance:
	PYTHONDONTWRITEBYTECODE=1 uv run python tools/qt_bridge_failure_acceptance.py --native "/tmp/hcb-qt-tests/native/Hot Cross Buns"

shared-process-acceptance:
	PYTHONDONTWRITEBYTECODE=1 uv run python tools/shared_process_acceptance.py --native "/tmp/hcb-qt-tests/native/Hot Cross Buns"

calendar-mouse-smoke:
	PYTHONDONTWRITEBYTECODE=1 uv run python tools/calendar_mouse_smoke.py

local-ci:
	./scripts/check-local.sh

install-hooks:
	git config --local core.hooksPath .githooks

clean:
	rm -rf .mypy_cache .pytest_cache .ruff_cache dist
