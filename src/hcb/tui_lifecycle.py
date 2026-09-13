"""TUI lifecycle, configuration, editor, and viewport interaction handlers."""

from __future__ import annotations

import shlex
import tempfile
import webbrowser
from contextlib import suppress
from dataclasses import asdict
from pathlib import Path
from typing import TYPE_CHECKING, Any, Literal, cast

from textual import events, work
from textual.app import SuspendNotSupported
from textual.containers import Vertical
from textual.widgets import (
    Input as TextualInput,
)
from textual.widgets import (
    Static,
    TextArea,
)

from .config import Config, ConfigError, load, profile_path, save
from .credentials import create_credential_template
from .environment import LocalEnvironment, detect_local_environment
from .errors import AuthenticationRequired, ConfigurationError, HcbError
from .tui import (
    MIN_CENTER_WIDTH,
    MIN_SIDEBAR_WIDTH,
    SPLITTER_WIDTH,
    emoji_suggestions,
    is_web_url,
)
from .tui_components import (
    ConfirmScreen,
    EmojiCompletion,
    EmojiTarget,
    EntityRow,
    GoogleSetupScreen,
    ImportScreen,
    Input,
    OnboardingScreen,
    TerminalTextArea,
    WorkspaceTable,
)
from .tui_theme import build_textual_theme

if TYPE_CHECKING:
    pass


class LifecycleMixin:
    _resize_target: Literal["sidebar"] | None
    _resize_handle: Static | None
    _emoji_token_start: int | None
    _emoji_popup: EmojiCompletion | None
    _emoji_popup_screen: object | None
    local_environment: LocalEnvironment | None

    def on_mount(self: Any) -> None:
        self._apply_visual_config(self.runtime.config)
        self._observed_config_marker = self._config_marker()
        # Config files are normally edited deliberately; polling twice per second
        # creates needless filesystem work on every interactive frame sequence.
        self.set_interval(2.0, self._reload_visual_config)
        self._apply_configured_keys(self.runtime.config)
        if self._stylesheet_error is not None:
            self.notify(self._stylesheet_error, severity="error", timeout=15)
        try:
            self.account_id = self.runtime.account_id(self.explicit_account)
        except AuthenticationRequired:
            self.account_id = None
        self.refresh_workspace()
        self._apply_column_widths()
        if self.account_id is None and not self.runtime.application.list_accounts():
            self.call_after_refresh(
                lambda: self.push_screen(self._onboarding_screen(), self._onboarding_result)
            )

    def _set_visual_state(self: Any, config: Config) -> None:
        forced_terminal = (
            self.runtime.environ.get("NO_COLOR") is not None
            or self.runtime.environ.get("TERM") == "dumb"
        )
        self.theme_mode = "mono" if forced_terminal else config.theme.profile
        self.density = config.theme.density
        self.border_style = "ascii" if forced_terminal else config.theme.borders
        self.focus_style = "ascii" if forced_terminal else config.theme.focus
        self.mouse_enabled = config.theme.mouse and not forced_terminal

    def _apply_visual_config(self: Any, config: Config) -> None:
        self._set_visual_state(config)
        colors = config.theme.colors
        self._theme_revision += 1
        name = f"hcb-config-{self._theme_revision}"
        self.register_theme(build_textual_theme(name, colors, dark=self.theme_mode != "light"))
        self.theme = name
        self.remove_class(
            "theme-terminal",
            "theme-dark",
            "theme-light",
            "theme-mono",
            "density-compact",
            "density-comfortable",
            "borders-ascii",
            "borders-unicode",
            "focus-ascii",
            "focus-underline",
            "focus-reverse",
            "ascii",
            "no-mouse",
        )
        self.add_class(
            f"theme-{self.theme_mode}",
            f"density-{self.density}",
            f"borders-{self.border_style}",
            f"focus-{self.focus_style}",
        )
        self.set_class(self.border_style == "ascii", "ascii")
        self.set_class(not self.mouse_enabled, "no-mouse")
        self.set_class(not config.tui.sidebar_visible, "sidebar-hidden")
        self.dark = self.theme_mode != "light"
        for text_area in self.query(TerminalTextArea):
            text_area.apply_terminal_theme()

    def _apply_configured_keys(self: Any, config: Config) -> None:
        self.set_keymap(asdict(config.keys))

    def _config_marker(self: Any) -> tuple[tuple[int, int] | None, ...]:
        paths = [self.runtime.paths.config_file]
        selected = self.runtime.environ.get("HCB_PROFILE") or self.runtime.config.active_profile
        if selected is not None:
            with suppress(ConfigError):
                paths.append(profile_path(self.runtime.paths.config_file, selected))
        marker: list[tuple[int, int] | None] = []
        for path in paths:
            try:
                stat = path.stat()
            except FileNotFoundError:
                marker.append(None)
            else:
                marker.append((stat.st_mtime_ns, stat.st_size))
        return tuple(marker)

    def _reload_visual_config(self: Any) -> None:
        marker = self._config_marker()
        if marker == self._observed_config_marker:
            return
        self._observed_config_marker = marker
        try:
            config = load(
                self.runtime.paths.config_file,
                profile=self.runtime.environ.get("HCB_PROFILE"),
            )
        except ConfigError as exc:
            self.notify(f"config.json not applied: {exc}", severity="error")
            return
        self.runtime.__dict__["config"] = config
        self._apply_visual_config(config)
        self._apply_configured_keys(config)
        self._render_chrome(refresh_resources=False)
        self._render_surface()
        message = "config.json settings reloaded"
        if config.theme.stylesheet != self._loaded_stylesheet:
            message += "; restart HCB to apply the stylesheet change"
        self.notify(message)

    def _onboarding_result(self: Any, result: dict[str, str] | None) -> None:
        if result is None:
            self.notify("Offline mode; setup can be reopened later")
            return
        reminders = result["reminders"].casefold()
        if reminders not in {"true", "false"}:
            self.notify("Reminders must be true or false", severity="error")
            self.push_screen(self._onboarding_screen(), self._onboarding_result)
            return
        credential_file = (
            Path(result["env_file"]).expanduser()
            if result["env_file"]
            else self.runtime.credential_file(result["account_id"])
        )
        try:
            template_created = self.runtime.save_onboarding(
                account_id=result["account_id"],
                email=result["email"],
                time_zone=result["time_zone"],
                reminders_enabled=reminders == "true",
                theme_preset=(
                    None if result["theme_preset"] == "terminal" else result["theme_preset"]
                ),
                credential_file=credential_file,
            )
        except (OSError, ValueError) as exc:
            self.notify(str(exc), severity="error")
            self.push_screen(self._onboarding_screen(), self._onboarding_result)
            return
        self.account_id = result["account_id"]
        if result["env_file"]:
            self.runtime.credential_file_override = credential_file
        self._apply_visual_config(self.runtime.config)
        self.refresh_workspace()
        if result["connect"] == "true":
            if template_created:
                self.push_screen(
                    GoogleSetupScreen(
                        operation="Google connection",
                        account_id=self.account_id,
                        email=result["email"],
                        credential_file=credential_file,
                        reason="credential template created; add your Google OAuth client ID first",
                    )
                )
                return
            self.push_screen(
                ConfirmScreen(
                    "Open the browser and connect this Google account now?",
                    confirm_label="Connect",
                ),
                self._onboarding_connect_confirmed,
            )
        else:
            message = "Offline account created; Google remains disconnected"
            if template_created:
                message = f"Created credential template at {credential_file}; {message}"
            self.notify(message)

    def _onboarding_screen(self: Any) -> OnboardingScreen:
        account_id = "onboarding"
        return OnboardingScreen(
            self._local_environment(),
            credential_file=self.runtime.credential_file(account_id),
            credential_file_suggestions=self.runtime.credential_file_suggestions(account_id),
            editor_command=self._editor_command(),
            external_editor_shortcut=self.runtime.config.keys.external_editor,
        )

    def _local_environment(self: Any) -> LocalEnvironment:
        if self.local_environment is None:
            self.local_environment = detect_local_environment(self.runtime.environ)
        return cast(LocalEnvironment, self.local_environment)

    def _onboarding_connect_confirmed(self: Any, confirmed: bool | None) -> None:
        if not confirmed or self.account_id is None:
            self.notify("Google connection skipped; local cache remains available")
            return
        account = self.runtime.storage.get_account(self.account_id)
        if account is None:
            self.notify("Google account is not configured", severity="error")
            return
        self.start_loading("Connecting to Google")
        self.connect_google(account.email)

    @work(thread=True, exclusive=True, group="auth")
    def connect_google(self: Any, expected_email: str) -> None:
        if self.account_id is None:
            return
        failure: Exception | None = None
        try:
            self.call_from_thread(self.update_loading, "Waiting for browser approval")
            self.runtime.connect_account(self.account_id, expected_email=expected_email)
        except Exception as exc:
            failure = exc
        else:
            self.call_from_thread(self.notify, "Google connected; sync remains explicit")
        finally:
            self.call_from_thread(self.stop_loading)
        if failure is not None:
            self.call_from_thread(
                self._show_remote_failure,
                "Google connection",
                failure,
                self.account_id,
            )

    def _show_remote_failure(
        self: Any, operation: str, error: Exception, account_id: str | None
    ) -> None:
        if account_id is not None and isinstance(
            error, (AuthenticationRequired, ConfigurationError)
        ):
            account = self.runtime.application.account(account_id)
            email = account.email if account is not None else "YOUR_GOOGLE_EMAIL"
            self.push_screen(
                GoogleSetupScreen(
                    operation=operation,
                    account_id=account_id,
                    email=email,
                    credential_file=self.runtime.credential_file(account_id),
                    reason=error.message,
                )
            )
            return
        if isinstance(error, HcbError):
            message = f"{operation} failed: {error.message}"
            if error.hint:
                message += f" Next: {error.hint}"
        else:
            message = f"{operation} failed: {error}"
        self.notify(message, severity="error", timeout=15)

    def on_input_changed(self: Any, event: TextualInput.Changed) -> None:
        if isinstance(event.input, Input):
            self.update_emoji_completion(event.input)

    def on_text_area_changed(self: Any, event: TextArea.Changed) -> None:
        if isinstance(event.text_area, TerminalTextArea):
            self.update_emoji_completion(event.text_area)

    def update_emoji_completion(self: Any, target: EmojiTarget) -> None:
        """Refresh suggestions for the focused editor's active colon token."""
        if self.focused is not target:
            self.dismiss_emoji_completion(target)
            return
        text, cursor = self._emoji_text_and_cursor(target)
        suggestion = emoji_suggestions(text, cursor)
        if suggestion is None:
            self.dismiss_emoji_completion(target)
            return
        token_start, matches = suggestion
        self._emoji_target = target
        self._emoji_token_start = token_start
        self._emoji_matches = matches
        self._emoji_selection = 0
        self._render_emoji_completion()

    def handle_emoji_key(self: Any, target: EmojiTarget, event: events.Key) -> bool:
        """Consume navigation keys only while this editor owns an emoji menu."""
        if target is not self._emoji_target or not self._emoji_matches:
            return False
        if event.key == "up":
            self._emoji_selection = (self._emoji_selection - 1) % len(self._emoji_matches)
        elif event.key == "down":
            self._emoji_selection = (self._emoji_selection + 1) % len(self._emoji_matches)
        elif event.key in {"tab", "enter"}:
            self._accept_emoji_completion(target)
        elif event.key == "escape":
            self.dismiss_emoji_completion(target)
        else:
            return False
        event.stop()
        event.prevent_default()
        if event.key in {"up", "down"}:
            self._render_emoji_completion()
        return True

    def handle_external_editor_key(self: Any, target: EmojiTarget, event: events.Key) -> bool:
        if event.key != self.runtime.config.keys.external_editor:
            return False
        event.stop()
        event.prevent_default()
        self.action_external_editor()
        return True

    def dismiss_emoji_completion(self: Any, target: EmojiTarget | None = None) -> None:
        if target is not None and target is not self._emoji_target:
            return
        self._emoji_target = None
        self._emoji_token_start = None
        self._emoji_matches = ()
        self._emoji_selection = 0
        if self._emoji_popup is not None:
            self._emoji_popup.display = False

    def _render_emoji_completion(self: Any) -> None:
        if not self._emoji_matches:
            return
        screen = self.screen
        if self._emoji_popup_screen is not screen:
            if self._emoji_popup is not None:
                self._emoji_popup.display = False
            self._emoji_popup = EmojiCompletion()
            self._emoji_popup_screen = screen
            screen.mount(self._emoji_popup)
        assert self._emoji_popup is not None
        self._emoji_popup.set_candidates(self._emoji_matches, self._emoji_selection)

    @staticmethod
    def _emoji_text_and_cursor(target: EmojiTarget) -> tuple[str, int]:
        if isinstance(target, Input):
            return target.value, target.cursor_position
        row, column = target.cursor_location
        offset = sum(
            len(target.document[index]) + len(target.document.newline) for index in range(row)
        )
        return target.text, offset + column

    @staticmethod
    def _text_area_location(target: TerminalTextArea, offset: int) -> tuple[int, int]:
        remaining = max(0, min(offset, len(target.text)))
        for row in range(target.document.line_count):
            length = len(target.document[row])
            if remaining <= length:
                return row, remaining
            remaining -= length
            if row < target.document.line_count - 1:
                remaining -= len(target.document.newline)
        return target.document.end

    def _accept_emoji_completion(self: Any, target: EmojiTarget) -> None:
        if self._emoji_token_start is None or not self._emoji_matches:
            return
        token_start = self._emoji_token_start
        emoji = self._emoji_matches[self._emoji_selection][1]
        if isinstance(target, Input):
            target.replace(emoji, token_start, target.cursor_position)
            target.cursor_position = token_start + len(emoji)
        else:
            result = target.replace(
                emoji,
                self._text_area_location(target, token_start),
                target.cursor_location,
                maintain_selection_offset=False,
            )
            target.cursor_location = result.end_location
        self.dismiss_emoji_completion(target)

    def _editor_command(self: Any) -> str:
        return str(self.runtime.environ.get("HCB_EDITOR") or self.runtime.config.preferences.editor)

    def action_external_editor(self: Any) -> None:
        target = self.focused
        if not isinstance(target, (Input, TerminalTextArea)):
            return
        self.dismiss_emoji_completion(target)
        if isinstance(target, Input) and target.id == "onboard-env-file":
            self._open_onboarding_credential_file(target)
            return
        if isinstance(target, Input) and target.id == "import-path":
            self._open_import_file(target)
            return
        temporary_path: Path | None = None
        try:
            command = shlex.split(self._editor_command())
            if not command:
                raise ValueError("external editor command is empty")
            source = target.value if isinstance(target, Input) else target.text
            with tempfile.NamedTemporaryFile(
                mode="w",
                encoding="utf-8",
                prefix="hcb-",
                suffix=".txt",
                delete=False,
            ) as temporary:
                temporary.write(source)
                temporary_path = Path(temporary.name)
            with self._editor_suspend():
                return_code = self._editor_runner([*command, str(temporary_path)])
            if return_code != 0:
                self.notify(f"External editor exited with status {return_code}", severity="error")
                return
            edited = temporary_path.read_text(encoding="utf-8")
        except SuspendNotSupported:
            self.notify("External editor is unavailable in this environment", severity="error")
            return
        except (OSError, UnicodeError, ValueError) as exc:
            self.notify(f"External editor failed: {exc}", severity="error")
            return
        finally:
            if temporary_path is not None:
                temporary_path.unlink(missing_ok=True)
        if isinstance(target, Input):
            target.value = " ".join(edited.rstrip("\r\n").splitlines())
            target.cursor_position = len(target.value)
        else:
            target.load_text(edited)
            target.cursor_location = target.document.end

    def _open_onboarding_credential_file(self: Any, target: Input) -> None:
        """Open the first-run credential file, creating its private template when needed."""

        try:
            if not target.value.strip():
                raise ValueError("credential file path is empty")
            credential_file = Path(target.value.strip()).expanduser()
            template_created = create_credential_template(credential_file)
            command = shlex.split(self._editor_command())
            if not command:
                raise ValueError("external editor command is empty")
            with self._editor_suspend():
                return_code = self._editor_runner([*command, str(credential_file)])
            if return_code != 0:
                self.notify(f"External editor exited with status {return_code}", severity="error")
                return
        except SuspendNotSupported:
            self.notify("External editor is unavailable in this environment", severity="error")
            return
        except (OSError, UnicodeError, ValueError) as exc:
            self.notify(f"External editor failed: {exc}", severity="error")
            return
        if template_created:
            self.notify(f"Created credential template at {credential_file}")

    def _open_import_file(self: Any, target: Input) -> None:
        """Open an existing import source, then require a fresh parse before applying it."""

        try:
            if not target.value.strip():
                raise ValueError("import file path is empty")
            import_file = Path(target.value.strip()).expanduser()
            if not import_file.is_file():
                raise ValueError(f"import file must be an existing file: {import_file}")
            command = shlex.split(self._editor_command())
            if not command:
                raise ValueError("external editor command is empty")
            with self._editor_suspend():
                return_code = self._editor_runner([*command, str(import_file)])
            if return_code != 0:
                self.notify(f"External editor exited with status {return_code}", severity="error")
                return
        except SuspendNotSupported:
            self.notify("External editor is unavailable in this environment", severity="error")
            return
        except (OSError, UnicodeError, ValueError) as exc:
            self.notify(f"External editor failed: {exc}", severity="error")
            return
        if isinstance(self.screen, ImportScreen):
            self.screen.preview = None
            self.screen.query_one("#import-summary", Static).update(
                "Import file closed; choose Preview again before Apply."
            )

    def action_edit_config_file(self: Any) -> None:
        """Open the complete configuration, keeping the running UI on its last valid settings."""

        target = self.runtime.paths.config_file
        try:
            if not target.exists():
                save(self.runtime.config, target)
            command = shlex.split(self._editor_command())
            if not command:
                raise ValueError("external editor command is empty")
            with self._editor_suspend():
                return_code = self._editor_runner([*command, str(target)])
            if return_code != 0:
                self.notify(f"External editor exited with status {return_code}", severity="error")
                return
            config = load(target)
        except SuspendNotSupported:
            self.notify("External editor is unavailable in this environment", severity="error")
            return
        except (OSError, UnicodeError, ValueError) as exc:
            self.notify(f"config.json not applied: {exc}", severity="error")
            return
        self.runtime.__dict__["config"] = config
        self._apply_visual_config(config)
        self._observed_config_marker = self._config_marker()
        self._render_chrome(refresh_resources=False)
        self._render_surface()
        self.notify("config.json validated; visual settings applied")

    def on_unmount(self: Any) -> None:
        self.runtime.close()

    def on_mouse_down(self: Any, event: events.MouseDown) -> None:
        if not self.mouse_enabled:
            event.stop()
            event.prevent_default()
            return
        if event.button != 1:
            return
        widget = event.widget
        if not isinstance(widget, Static):
            return
        targets: dict[str, Literal["sidebar"]] = {"sidebar-resize": "sidebar"}
        target = targets.get(widget.id or "")
        if target is None or not self._can_resize_columns():
            return
        self._resize_target = target
        self._resize_handle = widget
        self._resize_anchor_x = event.screen_x
        widget.capture_mouse()
        event.stop()
        event.prevent_default()

    def on_click(self: Any, event: events.Click) -> None:
        """Open safe links and allow Ctrl/Cmd-click multi-selection in content rows."""
        url = event.style.link
        if not self.mouse_enabled or event.button != 1:
            return
        if url is not None and is_web_url(url):
            try:
                opened = self._url_opener(url)
            except (OSError, webbrowser.Error) as exc:
                self.notify(f"Could not open link: {exc}", severity="error")
                return
            if not opened:
                self.notify("Could not open link in your default browser", severity="error")
                return
            event.stop()
            event.prevent_default()
            return
        if not (event.ctrl or event.meta):
            return
        row = event.widget if isinstance(event.widget, EntityRow) else None
        if row is None and event.widget is not None and isinstance(event.widget.parent, EntityRow):
            row = event.widget.parent
        if row is None or row.kind not in {"task", "event"}:
            return
        self.selected = (row.kind, row.item_id)
        self.action_mark()
        event.stop()
        event.prevent_default()

    def on_workspace_table_link_clicked(self: Any, event: WorkspaceTable.LinkClicked) -> None:
        """Open a workspace link without letting a row click open its item view."""
        if not self.mouse_enabled:
            return
        try:
            opened = self._url_opener(event.url)
        except (OSError, webbrowser.Error) as exc:
            self.notify(f"Could not open link: {exc}", severity="error")
            return
        if not opened:
            self.notify("Could not open link in your default browser", severity="error")

    def on_mouse_move(self: Any, event: events.MouseMove) -> None:
        if self._resize_target is None:
            return
        delta = event.screen_x - self._resize_anchor_x
        if delta:
            self._resize_columns(sidebar_delta=delta)
            self._resize_anchor_x = event.screen_x
        event.stop()
        event.prevent_default()

    def on_mouse_up(self: Any, event: events.MouseUp) -> None:
        if self._resize_target is None:
            return
        if self._resize_handle is not None:
            self._resize_handle.capture_mouse(False)
        self._resize_target = None
        self._resize_handle = None
        event.stop()
        event.prevent_default()

    def on_resize(self: Any, event: events.Resize) -> None:
        self.set_class(event.size.width < 90, "narrow")
        self.set_class(event.size.width < 62, "very-narrow")
        self._apply_column_widths()

    def _can_resize_columns(self: Any) -> bool:
        return bool(self.size.width >= 90)

    def _apply_column_widths(self: Any) -> None:
        """Apply the in-session sidebar width while both workspace columns are visible."""
        sidebar = self.query_one("#sidebar", Vertical)
        if not self.runtime.config.tui.sidebar_visible:
            return
        if not self._can_resize_columns():
            sidebar.styles.clear_rule("width")
            return
        available = self.size.width - SPLITTER_WIDTH
        self.sidebar_width = max(
            MIN_SIDEBAR_WIDTH,
            min(self.sidebar_width, available - MIN_CENTER_WIDTH),
        )
        sidebar.styles.width = self.sidebar_width

    def _resize_columns(self: Any, *, sidebar_delta: int = 0) -> None:
        if not self._can_resize_columns():
            return
        self.sidebar_width += sidebar_delta
        self._apply_column_widths()

    def action_resize_sidebar(self: Any, delta: int | str) -> None:
        self._resize_columns(sidebar_delta=int(delta))
