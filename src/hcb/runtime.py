"""Shared construction of local and remote application dependencies."""

from __future__ import annotations

import os
from collections.abc import Callable
from dataclasses import replace
from datetime import datetime
from functools import cached_property
from pathlib import Path
from typing import Any

from .application import ApplicationService
from .auth import GoogleAuthenticator, OAuthResult, TokenStore
from .config import (
    Config,
    ConfigError,
    KeyBindings,
    Theme,
    ThemeColors,
    ThemeRoles,
    TuiSettings,
    load,
    profile_path,
    save,
    save_profile,
)
from .credentials import (
    CredentialFileError,
    EncryptedFileTokenStore,
    create_credential_template,
    discover_credential_files,
    load_client_config,
)
from .errors import AuthenticationRequired, ConfigurationError, NotFoundError, StorageError
from .google_client import GoogleApiClient, GoogleGateway
from .models import Account, CapturePreferences, Event
from .paths import AppPaths
from .storage import Storage
from .sync import SyncEngine, SyncResult
from .themes import apply_preset

GatewayFactory = Callable[[Any], GoogleGateway]
DEFAULT_CREDENTIAL_FILE = Path("~/.config/hcb/personal.env")


class Runtime:
    """Dependency container which keeps all Google construction behind ``sync``/auth."""

    def __init__(
        self,
        paths: AppPaths | None = None,
        *,
        environ: dict[str, str] | None = None,
        token_store: TokenStore | None = None,
        gateway_factory: GatewayFactory | None = None,
        credential_file: Path | None = None,
    ) -> None:
        self.paths = paths or AppPaths.discover()
        self.environ = os.environ if environ is None else environ
        self._token_store = token_store
        self._gateway_factory = gateway_factory
        self.credential_file_override = credential_file

    @cached_property
    def config(self) -> Config:
        try:
            return load(self.paths.config_file, profile=self.environ.get("HCB_PROFILE"))
        except ConfigError as exc:
            raise ConfigurationError(str(exc)) from exc

    @cached_property
    def storage(self) -> Storage:
        try:
            return Storage(self.paths.database_file)
        except Exception as exc:
            raise StorageError(f"cannot open local database: {exc}") from exc

    @cached_property
    def application(self) -> ApplicationService:
        return ApplicationService(self.storage)

    @cached_property
    def keyring_store(self) -> TokenStore:
        return self._token_store or TokenStore()

    def credential_file(self, account_id: str) -> Path:
        """Resolve the default credential file, with explicit process overrides."""
        configured = self.credential_file_override or self.environ.get("HCB_ENV_FILE")
        if configured:
            return Path(configured).expanduser()
        return DEFAULT_CREDENTIAL_FILE.expanduser()

    def credential_file_suggestions(self, account_id: str) -> tuple[Path, ...]:
        """List safe local credential-file suggestions for first-run setup."""

        return discover_credential_files(
            self.credential_file(account_id),
            DEFAULT_CREDENTIAL_FILE,
            config_dir=self.paths.config_dir,
        )

    def token_store_for(self, account_id: str) -> EncryptedFileTokenStore:
        return EncryptedFileTokenStore(
            account_id, self.credential_file(account_id), self.keyring_store
        )

    def account_id(self, explicit: str | None = None) -> str:
        selected = (
            explicit
            or self.environ.get("HCB_ACCOUNT")
            or self.config.preferences.default_account_id
        )
        if selected:
            if self.storage.get_account(selected) is None:
                raise NotFoundError(f"Account {selected!r} does not exist")
            return selected
        accounts = self.storage.list_accounts()
        if len(accounts) == 1:
            return accounts[0].id
        if not accounts:
            raise AuthenticationRequired("no account is configured", hint="Run `hcb auth connect`.")
        raise ConfigurationError(
            "multiple accounts are configured; select one with --account, HCB_ACCOUNT, "
            "or preferences.default_account_id"
        )

    def _client_config(self, account_id: str) -> dict[str, Any]:
        try:
            return load_client_config(self.credential_file(account_id))
        except (OSError, CredentialFileError) as exc:
            raise ConfigurationError(str(exc)) from exc

    def save_onboarding(
        self,
        *,
        account_id: str,
        email: str,
        time_zone: str,
        reminders_enabled: bool,
        theme_preset: str | None = None,
        credential_file: Path | None = None,
    ) -> bool:
        if not account_id.strip() or any(character.isspace() for character in account_id):
            raise ValueError("account identifier is required and cannot contain whitespace")
        if "@" not in email or email.strip() != email:
            raise ValueError("a valid account email is required")
        preferences = replace(
            self.config.preferences,
            default_account_id=account_id,
            time_zone=time_zone,
            reminders_enabled=reminders_enabled,
        )
        updated = replace(
            self.config,
            preferences=preferences,
            theme=(
                apply_preset(self.config.theme, theme_preset) if theme_preset else self.config.theme
            ),
        )
        # Constructing Preferences validates the IANA zone before any state is written.
        template_created = (
            create_credential_template(credential_file) if credential_file is not None else False
        )
        self._persist_config(updated)
        with self.storage.transaction():
            self.storage.upsert_account(Account(account_id, email))
        return template_created

    def authenticator(self, account_id: str) -> GoogleAuthenticator:
        try:
            return GoogleAuthenticator(
                self._client_config(account_id), self.token_store_for(account_id)
            )
        except ValueError as exc:
            raise ConfigurationError(str(exc)) from exc

    def sync_engine(self, account_id: str) -> SyncEngine:
        try:
            credentials = self.authenticator(account_id).credentials(account_id)
        except (LookupError, CredentialFileError) as exc:
            raise AuthenticationRequired(str(exc), hint="Run `hcb auth connect`.") from exc
        gateway = (
            self._gateway_factory(credentials)
            if self._gateway_factory is not None
            else GoogleApiClient(credentials)
        )
        return SyncEngine(
            self.storage,
            gateway,
            pull_workers=4,
            conflict_policy=self.config.preferences.conflict_policy,
        )

    def connect_account(
        self,
        account_id: str,
        *,
        expected_email: str,
        cancelled: Callable[[], bool] | None = None,
    ) -> OAuthResult:
        return self.authenticator(account_id).connect(
            account_id, expected_email=expected_email, cancelled=cancelled
        )

    def query_freebusy(self, account_id: str, body: dict[str, Any]) -> dict[str, Any]:
        return self.sync_engine(account_id).gateway.freebusy(body)

    def sync_account(
        self,
        account_id: str,
        *,
        progress: Callable[[str], None] | None = None,
        cancelled: Callable[[], bool] | None = None,
        cancel_hint: str = "Press Ctrl+C to cancel.",
    ) -> SyncResult:
        """Run interactive sync on an independent SQLite connection."""
        engine = self.sync_engine(account_id)
        with Storage(self.paths.database_file) as worker_storage:
            engine.storage = worker_storage
            return engine.sync(
                account_id,
                progress=progress,
                cancelled=cancelled,
                cancel_hint=cancel_hint,
            )

    def refresh_occurrences(
        self, account_id: str, calendar_id: str, start: datetime, end: datetime
    ) -> list[Event]:
        """Fetch a named range and persist instances on an independent connection."""
        engine = self.sync_engine(account_id)
        with Storage(self.paths.database_file) as worker_storage:
            engine.storage = worker_storage
            return engine.refresh_occurrences(account_id, calendar_id, start, end)

    def disconnect(self, account_id: str, *, reset_local_data: bool = False) -> bool:
        """Remove local credentials without requiring a client configuration to be readable."""
        removed = self.token_store_for(account_id).delete(account_id)
        if reset_local_data:
            with self.storage.transaction():
                self.storage.delete_account(account_id)
        return removed

    def update_tui_settings(
        self,
        *,
        profile: str,
        density: str,
        borders: str,
        focus: str,
        mouse: bool,
        loader: str,
        theme_preset: str | None,
        week_starts_on: int,
        date_time_format: str,
        editor: str,
        external_editor: str,
        colors: ThemeColors,
        roles: ThemeRoles,
        stylesheet: str | None,
        time_zone: str,
        default_account_id: str | None,
        default_task_list_id: str | None,
        default_calendar_id: str | None,
        reminders_enabled: bool,
        reminder_poll_seconds: int,
        reminder_catch_up_minutes: int,
        reminder_jitter_seconds: int,
        reminder_sync_interval_minutes: int,
        reminder_sync_mode: str,
        conflict_policy: str,
        capture: CapturePreferences,
        keys: KeyBindings,
        tui: TuiSettings,
        active_profile: str | None,
    ) -> Config:
        """Persist interactive presentation settings through the runtime boundary."""
        current = self.config
        selected_theme = (
            apply_preset(current.theme, theme_preset) if theme_preset is not None else current.theme
        )
        if theme_preset is not None and (
            profile != selected_theme.profile or colors != selected_theme.colors
        ):
            raise ValueError("a selected theme palette must keep its bundled profile and colors")
        updated = replace(
            current,
            preferences=replace(
                current.preferences,
                week_starts_on=week_starts_on,
                date_time_format=date_time_format,
                editor=editor,
                time_zone=time_zone,
                default_account_id=default_account_id,
                default_task_list_id=default_task_list_id,
                default_calendar_id=default_calendar_id,
                reminders_enabled=reminders_enabled,
                reminder_poll_seconds=reminder_poll_seconds,
                reminder_catch_up_minutes=reminder_catch_up_minutes,
                reminder_jitter_seconds=reminder_jitter_seconds,
                reminder_sync_interval_minutes=reminder_sync_interval_minutes,
                reminder_sync_mode=reminder_sync_mode,
                conflict_policy=conflict_policy,
                capture=capture,
            ),
            keys=replace(keys, external_editor=external_editor),
            tui=tui,
            active_profile=active_profile,
            theme=replace(
                selected_theme,
                profile=profile,
                preset=theme_preset,
                density=density,
                borders=borders,
                focus=focus,
                mouse=mouse,
                loader=loader,
                colors=colors,
                roles=roles,
                stylesheet=stylesheet,
            ),
        )
        return self._persist_config(updated)

    def update_theme(self, theme: Theme) -> Config:
        """Persist a complete visual theme selected by the CLI or a custom file."""
        updated = replace(self.config, theme=theme)
        return self._persist_config(updated)

    def _persist_config(self, updated: Config) -> Config:
        """Write the base or active overlay without flattening a profile into the base."""
        selected = self.environ.get("HCB_PROFILE") or updated.active_profile
        if selected is None:
            save(updated, self.paths.config_file)
        else:
            profile_path(self.paths.config_file, selected)
            base = load(self.paths.config_file, resolve_profile=False)
            save(replace(base, active_profile=updated.active_profile), self.paths.config_file)
            save_profile(updated, self.paths.config_file, selected)
        self.__dict__.pop("config", None)
        return self.config

    def close(self) -> None:
        if "storage" in self.__dict__:
            self.storage.close()
