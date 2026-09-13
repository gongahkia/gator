"""Authentication boundaries and secure refresh-token persistence."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Protocol, cast

DEFAULT_SCOPES = (
    "openid",
    "https://www.googleapis.com/auth/userinfo.email",
    "https://www.googleapis.com/auth/tasks",
    "https://www.googleapis.com/auth/calendar",
    "https://www.googleapis.com/auth/drive.metadata.readonly",
)
KEYRING_SERVICE = "hot-cross-buns"


class KeyringLike(Protocol):
    def get_password(self, service_name: str, username: str) -> str | None: ...

    def set_password(self, service_name: str, username: str, password: str) -> None: ...

    def delete_password(self, service_name: str, username: str) -> None: ...


class RefreshTokenStore(Protocol):
    """Persistence boundary used by OAuth without prescribing its backing store."""

    def get(self, account_id: str) -> str | None: ...

    def set(self, account_id: str, refresh_token: str) -> None: ...

    def delete(self, account_id: str) -> bool: ...


def _system_keyring() -> KeyringLike:
    import keyring

    return cast(KeyringLike, keyring)


class TokenStore:
    """Stores only long-lived refresh tokens; access tokens remain in memory."""

    def __init__(
        self, backend: KeyringLike | None = None, *, service: str = KEYRING_SERVICE
    ) -> None:
        self.backend = backend or _system_keyring()
        self.service = service

    @staticmethod
    def _username(account_id: str) -> str:
        if not account_id or any(character.isspace() for character in account_id):
            raise ValueError("account_id must be non-empty and contain no whitespace")
        return f"google:{account_id}"

    def get(self, account_id: str) -> str | None:
        return self.backend.get_password(self.service, self._username(account_id))

    def set(self, account_id: str, refresh_token: str) -> None:
        if not refresh_token:
            raise ValueError("refresh token must not be empty")
        self.backend.set_password(self.service, self._username(account_id), refresh_token)

    def delete(self, account_id: str) -> bool:
        username = self._username(account_id)
        if self.backend.get_password(self.service, username) is None:
            return False
        try:
            self.backend.delete_password(self.service, username)
        except Exception as exc:
            # keyring's concrete exception types differ by backend.
            if exc.__class__.__name__ != "PasswordDeleteError":
                raise
            return False
        return True


@dataclass(frozen=True, slots=True)
class OAuthResult:
    refresh_token: str
    access_token: str | None
    granted_scopes: tuple[str, ...]


class GoogleAuthenticator:
    """Google installed-application OAuth using PKCE and a loopback callback."""

    def __init__(
        self,
        client_config: dict[str, Any],
        token_store: RefreshTokenStore | None = None,
        scopes: tuple[str, ...] = DEFAULT_SCOPES,
    ) -> None:
        if "installed" not in client_config:
            raise ValueError(
                "Google OAuth client configuration must contain an 'installed' section"
            )
        self.client_config = client_config
        self.token_store = token_store or TokenStore()
        self.scopes = scopes

    def connect(
        self,
        account_id: str,
        *,
        expected_email: str,
        open_browser: bool = True,
        timeout_seconds: int = 180,
    ) -> OAuthResult:
        from google_auth_oauthlib.flow import InstalledAppFlow  # type: ignore[import-untyped]

        flow = InstalledAppFlow.from_client_config(
            self.client_config,
            scopes=self.scopes,
            autogenerate_code_verifier=True,
        )
        credentials = flow.run_local_server(
            host="127.0.0.1",
            port=0,
            open_browser=open_browser,
            authorization_prompt_message="Open this URL to authorize HCB:\n{url}",
            success_message="HCB authorization complete. You may close this window.",
            timeout_seconds=timeout_seconds,
            access_type="offline",
            prompt="consent",
        )
        refresh_token = credentials.refresh_token
        if not refresh_token:
            raise RuntimeError(
                "Google did not return a refresh token; revoke prior consent and try again"
            )
        actual_email = self._verified_email(credentials)
        if actual_email.casefold() != expected_email.strip().casefold():
            raise ValueError(
                "The Google account selected in the browser does not match the requested email"
            )
        self.token_store.set(account_id, refresh_token)
        return OAuthResult(
            refresh_token=refresh_token,
            access_token=credentials.token,
            granted_scopes=tuple(credentials.scopes or self.scopes),
        )

    @staticmethod
    def _verified_email(credentials: Any) -> str:
        from google.auth.transport.requests import AuthorizedSession

        with AuthorizedSession(credentials) as session:  # type: ignore[no-untyped-call]
            response = session.get("https://openidconnect.googleapis.com/v1/userinfo", timeout=20)
            response.raise_for_status()
            identity = response.json()
        if not isinstance(identity, dict) or identity.get("email_verified") is not True:
            raise RuntimeError("Google did not return a verified account email")
        email = identity.get("email")
        if not isinstance(email, str) or not email:
            raise RuntimeError("Google did not return a verified account email")
        return email

    def credentials(self, account_id: str) -> Any:
        """Build refreshable Google credentials without performing network I/O."""
        from google.oauth2.credentials import Credentials

        refresh_token = self.token_store.get(account_id)
        if refresh_token is None:
            raise LookupError(f"no credentials stored for account {account_id!r}")
        installed = self.client_config["installed"]
        return Credentials(  # type: ignore[no-untyped-call]
            token=None,
            refresh_token=refresh_token,
            token_uri=installed.get("token_uri", "https://oauth2.googleapis.com/token"),
            client_id=installed["client_id"],
            client_secret=installed.get("client_secret"),
            scopes=self.scopes,
        )

    def disconnect(
        self, account_id: str, *, storage: Any | None = None, reset_local_data: bool = False
    ) -> bool:
        """Remove local credentials while retaining cached data by default.

        Revocation is intentionally not implicit: disconnect remains reliable offline and
        never claims remote revocation succeeded when the network is unavailable.
        """
        removed = self.token_store.delete(account_id)
        if storage is not None and reset_local_data:
            with storage.transaction():
                storage.delete_account(account_id)
        return removed
