import json
from dataclasses import replace
from pathlib import Path

import pytest
from jsonschema import Draft202012Validator

from hcb.config import (
    Config,
    ConfigError,
    KeyBindings,
    Theme,
    ThemeColors,
    TuiSettings,
    load,
    loads,
    profile_path,
    save,
    save_profile,
    schema,
)
from hcb.loaders import DEFAULT_LOADER, LOADER_NAMES, LOADER_PRESETS
from hcb.models import Preferences
from hcb.paths import AppPaths
from hcb.runtime import Runtime


def test_defaults_are_terminal_minimal_and_valid(tmp_path: Path) -> None:
    config = load(tmp_path / "missing.json")
    assert config.theme.profile == "terminal"
    assert config.theme.borders == "ascii"
    assert config.theme.colors.background == "transparent"
    assert config.theme.colors.control == "ansi_default"
    assert config.keys.search == "slash,ctrl+p"
    assert config.keys.external_editor == "ctrl+g"
    assert config.preferences.editor == "nvim"
    assert config.preferences.week_starts_on == 0
    assert config.preferences.date_time_format == "friendly"
    assert config.preferences.conflict_policy == "ask"
    assert config.theme.loader == DEFAULT_LOADER
    assert config.tui.default_event_duration_minutes == 60


def test_calendar_click_duration_is_bounded() -> None:
    assert TuiSettings(default_event_duration_minutes=5).default_event_duration_minutes == 5
    with pytest.raises(ValueError, match="default_event_duration_minutes"):
        TuiSettings(default_event_duration_minutes=4)


def test_rattles_loader_catalog_is_complete_and_preserves_source_timing() -> None:
    expected = {
        "arrows.arrow",
        "arrows.double_arrow",
        "ascii.dqpb",
        "ascii.rolling_line",
        "ascii.simple_dots",
        "ascii.simple_dots_scrolling",
        "ascii.arc",
        "ascii.balloon",
        "ascii.circle_halves",
        "ascii.circle_quarters",
        "ascii.point",
        "ascii.square_corners",
        "ascii.toggle",
        "ascii.triangle",
        "ascii.grow_horizontal",
        "ascii.grow_vertical",
        "ascii.noise",
        "braille.dots",
        "braille.dots2",
        "braille.dots3",
        "braille.dots4",
        "braille.dots5",
        "braille.dots6",
        "braille.dots7",
        "braille.dots8",
        "braille.dots9",
        "braille.dots10",
        "braille.dots11",
        "braille.dots12",
        "braille.dots13",
        "braille.dots14",
        "braille.sand",
        "braille.bounce",
        "braille.dots_circle",
        "braille.wave",
        "braille.scan",
        "braille.rain",
        "braille.pulse",
        "braille.snake",
        "braille.sparkle",
        "braille.cascade",
        "braille.columns",
        "braille.orbit",
        "braille.breathe",
        "braille.waverows",
        "braille.checkerboard",
        "braille.helix",
        "braille.fillsweep",
        "braille.diagswipe",
        "braille.infinity",
        "emoji.hearts",
        "emoji.clock",
        "emoji.earth",
        "emoji.moon",
        "emoji.speaker",
        "emoji.weather",
    }
    assert set(LOADER_NAMES) == expected
    rolling_line = LOADER_PRESETS["ascii.rolling_line"]
    assert rolling_line.interval_ms == 80
    assert rolling_line.frame_at(0.16) == "\\"


def test_json_round_trip(tmp_path: Path) -> None:
    expected = Config(
        preferences=Preferences(editor="hx", week_starts_on=6, reminders_enabled=False),
        theme=Theme(profile="dark", colors=ThemeColors(accent="#88c0d0")),
        keys=KeyBindings(sync="ctrl+s", external_editor="ctrl+g"),
    )
    target = tmp_path / "nested" / "config.json"
    assert save(expected, target) == target
    assert load(target) == expected
    document = json.loads(target.read_text())
    assert document["schema_version"] == 2
    assert document["theme"]["colors"]["accent"] == "#88c0d0"


def test_legacy_toml_is_not_read_or_migrated(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    paths.config_dir.mkdir()
    (paths.config_dir / "config.toml").write_text("[theme]\nname = 'dark'\n")
    assert paths.config_file.name == "config.json"
    assert load(paths.config_file) == Config()


@pytest.mark.parametrize(
    "raw",
    (
        '{"preferences":{"week_starts_on":9}}',
        '{"theme":{"unknown":"x"}}',
        '{"keys":{"quit":3}}',
        '{"unknown":{"value":true}}',
        '{"preferences":{"week_starts_on":"0"}}',
        '{"preferences":{"editor":""}}',
        '{"preferences":{"date_time_format":"clockwork"}}',
        '{"keys":{"external_editor":""}}',
        '{"theme":{"colors":{"accent":"not-a-color"}}}',
        '{"theme":{"loader":"not-a-loader"}}',
        '{"theme":{"colors":{"accent":"red","accent":"blue"}}}',
        '{"preferences":{"capture":{"default_event_duration_minutes":0}}}',
        '{"preferences":{"capture":{"task_aliases":[""]}}}',
        '{"active_profile":"Work"}',
    ),
)
def test_invalid_strict_json_config_is_rejected(raw: str) -> None:
    with pytest.raises(ConfigError):
        loads(raw)


@pytest.mark.parametrize(
    "bindings, message",
    (
        ('{"keys":{"quit":""}}', "must not be empty"),
        ('{"keys":{"help":"not-a-key"}}', "unknown key"),
        ('{"keys":{"sync":"ctrl+r"}}', "conflicts"),
        ('{"keys":{"modal_edit":"d"}}', "conflicts"),
        ('{"keys":{"sync":"ctrl+ctrl+r"}}', "invalid shortcut"),
    ),
)
def test_keymap_validation_rejects_empty_unknown_and_conflicting_bindings(
    bindings: str, message: str
) -> None:
    with pytest.raises(ConfigError, match=message):
        loads(bindings)


def test_bundled_schema_accepts_saved_config_and_rejects_unknown_tokens(tmp_path: Path) -> None:
    target = save(Config(theme=Theme(preset="Dracula")), tmp_path / "config.json")
    validator = Draft202012Validator(schema())
    assert list(validator.iter_errors(json.loads(target.read_text()))) == []
    invalid = {"theme": {"colors": {"unexpected": "red"}}}
    assert list(validator.iter_errors(invalid))
    assert list(validator.iter_errors({"keys": {"unrecognized_action": "x"}}))


def test_v1_configuration_migrates_and_a_profile_overlays_only_its_values(tmp_path: Path) -> None:
    target = tmp_path / "config.json"
    target.write_text(
        '{"schema_version":1,"preferences":{"theme":"dark","keymap":"vim","editor":"hx"},'
        '"keys":{"quit":"ctrl+q"}}',
        encoding="utf-8",
    )
    migrated = load(target)
    assert migrated.schema_version == 2
    assert migrated.keys.quit == "ctrl+q"
    assert migrated.preferences.editor == "hx"
    assert not hasattr(migrated.preferences, "theme")
    save(migrated, target)
    assert "theme" not in json.loads(target.read_text())["preferences"]
    assert "keymap" not in json.loads(target.read_text())["preferences"]

    early_v2 = loads('{"schema_version":2,"preferences":{"theme":"dark","keymap":"vim"}}')
    assert not hasattr(early_v2.preferences, "theme")

    save(Config(active_profile="work"), target)
    overlay = profile_path(target, "work")
    overlay.parent.mkdir()
    overlay.write_text(
        '{"preferences":{"capture":{"task_aliases":["todo"]}},"tui":{"agenda_days":21}}',
        encoding="utf-8",
    )
    resolved = load(target)
    assert resolved.preferences.capture.task_aliases == ("todo",)
    assert resolved.tui.agenda_days == 21


def test_profile_persistence_writes_only_the_overlay_and_runtime_keeps_the_base(
    tmp_path: Path,
) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    base = Config(
        active_profile="work",
        preferences=Preferences(editor="nvim"),
        theme=Theme(colors=ThemeColors(accent="blue")),
    )
    save(base, paths.config_file)
    profile_path(paths.config_file, "work").parent.mkdir()
    profile_path(paths.config_file, "work").write_text("{}\n", encoding="utf-8")
    resolved = replace(load(paths.config_file), preferences=replace(base.preferences, editor="hx"))
    save_profile(resolved, paths.config_file, "work")
    assert json.loads(profile_path(paths.config_file, "work").read_text()) == {
        "preferences": {"editor": "hx"}
    }
    assert load(paths.config_file).preferences.editor == "hx"
    assert load(paths.config_file, resolve_profile=False).preferences.editor == "nvim"

    runtime = Runtime(paths, environ={})
    runtime.update_theme(Theme(colors=ThemeColors(accent="red")))
    assert load(paths.config_file, resolve_profile=False).theme.colors.accent == "blue"
    assert load(paths.config_file).theme.colors.accent == "red"
