from __future__ import annotations

import json
import os
from pathlib import Path
from types import SimpleNamespace

import pytest

from hcb.auth import GoogleAuthenticator, TokenStore
from hcb.credentials import EncryptedFileTokenStore, create_credential_template, load_client_config
from hcb.models import Account
from hcb.paths import AppPaths
from hcb.runtime import DEFAULT_CREDENTIAL_FILE, Runtime
from hcb.storage import Storage


class FakeKeyring:
    def __init__(self) -> None:
        self.values: dict[tuple[str, str], str] = {}

    def get_password(self, service_name: str, username: str) -> str | None:
        return self.values.get((service_name, username))

    def set_password(self, service_name: str, username: str, password: str) -> None:
        self.values[(service_name, username)] = password

    def delete_password(self, service_name: str, username: str) -> None:
        del self.values[(service_name, username)]


CLIENT_CONFIG = {
    "installed": {
        "client_id": "client-id.apps.googleusercontent.com",
        "client_secret": "not-a-real-secret",
        "token_uri": "https://oauth2.googleapis.com/token",
        "auth_uri": "https://accounts.google.com/o/oauth2/auth",
        "redirect_uris": ["http://localhost"],
    }
}


def test_token_store_round_trip_and_idempotent_delete() -> None:
    tokens = TokenStore(FakeKeyring())
    assert tokens.get("account") is None
    tokens.set("account", "refresh")
    assert tokens.get("account") == "refresh"
    assert tokens.delete("account")
    assert not tokens.delete("account")
    with pytest.raises(ValueError):
        tokens.set("account", "")


def test_credentials_are_built_without_network() -> None:
    tokens = TokenStore(FakeKeyring())
    tokens.set("account", "refresh")
    credentials = GoogleAuthenticator(CLIENT_CONFIG, tokens).credentials("account")
    assert credentials.refresh_token == "refresh"
    assert credentials.client_id == "client-id.apps.googleusercontent.com"


def test_connect_checks_google_identity_before_storing_token(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from google_auth_oauthlib.flow import InstalledAppFlow

    class FakeFlow:
        def run_local_server(self, **_kwargs: object) -> SimpleNamespace:
            return SimpleNamespace(refresh_token="refresh", token="access", scopes=("openid",))

    monkeypatch.setattr(
        InstalledAppFlow, "from_client_config", lambda *_args, **_kwargs: FakeFlow()
    )
    tokens = TokenStore(FakeKeyring())
    auth = GoogleAuthenticator(CLIENT_CONFIG, tokens)
    monkeypatch.setattr(auth, "_verified_email", lambda _credentials: "other@example.test")

    with pytest.raises(ValueError, match="does not match"):
        auth.connect("account", expected_email="person@example.test")
    assert tokens.get("account") is None

    monkeypatch.setattr(auth, "_verified_email", lambda _credentials: "PERSON@example.test")
    assert auth.connect("account", expected_email="person@example.test").refresh_token == "refresh"
    assert tokens.get("account") == "refresh"


def test_connect_rejects_unverified_userinfo(monkeypatch: pytest.MonkeyPatch) -> None:
    from google.auth.transport import requests as google_requests

    class FakeResponse:
        def raise_for_status(self) -> None:
            pass

        def json(self) -> dict[str, object]:
            return {"email": "person@example.test", "email_verified": False}

    class FakeSession:
        def __init__(self, _credentials: object) -> None:
            pass

        def __enter__(self) -> FakeSession:
            return self

        def __exit__(self, *_args: object) -> None:
            pass

        def get(self, url: str, *, timeout: int) -> FakeResponse:
            assert url == "https://openidconnect.googleapis.com/v1/userinfo"
            assert timeout == 20
            return FakeResponse()

    monkeypatch.setattr(google_requests, "AuthorizedSession", FakeSession)
    with pytest.raises(RuntimeError, match="verified account email"):
        GoogleAuthenticator._verified_email(object())


def test_disconnect_retains_cache_and_explicit_reset_removes_it(tmp_path: Path) -> None:
    tokens = TokenStore(FakeKeyring())
    tokens.set("account", "refresh")
    auth = GoogleAuthenticator(CLIENT_CONFIG, tokens)
    with Storage(tmp_path / "db.sqlite") as store:
        store.upsert_account(Account("account", "person@example.test"))
        assert auth.disconnect("account", storage=store)
        assert store.get_account("account") is not None
        auth.disconnect("account", storage=store, reset_local_data=True)
        assert store.get_account("account") is None
    assert tokens.get("account") is None


def test_runtime_disconnect_does_not_require_a_credential_file(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    runtime = Runtime(
        paths,
        environ={},
        token_store=TokenStore(FakeKeyring()),
        credential_file=tmp_path / "personal.env",
    )
    runtime.storage.upsert_account(Account("offline", "offline@example.test"))
    assert not runtime.disconnect("offline")
    assert runtime.storage.get_account("offline") is not None
    assert not runtime.disconnect("offline", reset_local_data=True)
    assert runtime.storage.get_account("offline") is None
    runtime.close()


def test_runtime_defaults_to_the_personal_credential_file(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    runtime = Runtime(paths, environ={})
    assert runtime.credential_file("any-account") == DEFAULT_CREDENTIAL_FILE.expanduser()
    assert (
        Runtime(paths, environ={"HCB_ENV_FILE": "~/alternate.env"}).credential_file("any-account")
        == Path("~/alternate.env").expanduser()
    )
    assert (
        Runtime(paths, credential_file=tmp_path / "override.env").credential_file("any-account")
        == tmp_path / "override.env"
    )


def test_runtime_suggests_configured_default_and_detected_credential_files(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    configured = tmp_path / "configured.env"
    detected = paths.config_dir / "accounts" / "work.env"
    detected.parent.mkdir(parents=True)
    detected.write_text("HCB_GOOGLE_CLIENT_ID=client-id\n")

    runtime = Runtime(paths, environ={"HCB_ENV_FILE": str(configured)})

    assert runtime.credential_file_suggestions("work") == (
        configured,
        DEFAULT_CREDENTIAL_FILE.expanduser(),
        detected,
    )


def test_credential_template_is_private_and_never_overwrites_an_existing_file(
    tmp_path: Path,
) -> None:
    credential_file = tmp_path / "config" / "personal.env"

    assert create_credential_template(credential_file)
    assert credential_file.stat().st_mode & 0o777 == 0o600
    assert credential_file.read_text() == (
        "# Google OAuth credentials for Hot Cross Buns.\n"
        "# Create a Desktop OAuth client with the Google Calendar, Tasks, and Drive APIs enabled.\n"
        "HCB_GOOGLE_CLIENT_ID=\n"
        "# Optional for a Desktop OAuth client.\n"
        "HCB_GOOGLE_CLIENT_SECRET=\n"
    )

    credential_file.write_text("HCB_GOOGLE_CLIENT_ID=existing\n")
    assert not create_credential_template(credential_file)
    assert credential_file.read_text() == "HCB_GOOGLE_CLIENT_ID=existing\n"


def test_invalid_client_and_missing_credentials() -> None:
    with pytest.raises(ValueError):
        GoogleAuthenticator({}, TokenStore(FakeKeyring()))
    auth = GoogleAuthenticator(CLIENT_CONFIG, TokenStore(FakeKeyring()))
    with pytest.raises(LookupError):
        auth.credentials("missing")


def test_encrypted_environment_token_store_keeps_only_a_key_in_keyring(tmp_path: Path) -> None:
    credential_file = tmp_path / "account.env"
    credential_file.write_text(
        "HCB_GOOGLE_CLIENT_ID=client-id.apps.googleusercontent.com\n"
        "HCB_GOOGLE_CLIENT_SECRET=not-a-real-secret\n"
    )
    os.chmod(credential_file, 0o600)
    keyring = FakeKeyring()
    tokens = EncryptedFileTokenStore("account", credential_file, TokenStore(keyring))

    assert (
        load_client_config(credential_file)["installed"]["client_id"]
        == CLIENT_CONFIG["installed"]["client_id"]
    )
    tokens.set("account", "refresh-token-sentinel")
    assert tokens.get("account") == "refresh-token-sentinel"
    assert "refresh-token-sentinel" not in credential_file.read_text()
    assert len(keyring.values) == 1
    assert tokens.delete("account")
    assert tokens.get("account") is None


def test_diagnostics_config_and_sqlite_dump_never_contain_credentials(
    tmp_path: Path,
) -> None:
    refresh_token = "refresh-token-sentinel-never-export"
    access_token = "access-token-sentinel-never-export"
    backend = FakeKeyring()
    tokens = TokenStore(backend)
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    credential_file = paths.config_dir / "accounts" / "account.env"
    credential_file.parent.mkdir(parents=True)
    credential_file.write_text(
        "HCB_GOOGLE_CLIENT_ID=client-id.apps.googleusercontent.com\n"
        "HCB_GOOGLE_CLIENT_SECRET=not-a-real-secret\n"
    )
    os.chmod(credential_file, 0o600)
    runtime = Runtime(paths, environ={}, token_store=tokens, credential_file=credential_file)
    runtime.save_onboarding(
        account_id="account",
        email="redacted@example.test",
        time_zone="UTC",
        reminders_enabled=False,
    )
    runtime.token_store_for("account").set("account", refresh_token)

    diagnostics = json.dumps(runtime.application.diagnostics(), sort_keys=True)
    config_text = paths.config_file.read_text()
    sqlite_dump = "\n".join(runtime.storage.connection.iterdump())
    combined = diagnostics + config_text + sqlite_dump
    assert refresh_token not in combined
    assert access_token not in combined
    assert "client_secret" not in config_text
    assert "redacted@example.test" not in diagnostics
    runtime.close()
