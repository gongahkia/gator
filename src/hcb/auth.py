"""Authentication boundaries and secure refresh-token persistence."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from time import monotonic
from typing import Any, Protocol, cast
from urllib.parse import urlsplit
from wsgiref.simple_server import WSGIRequestHandler, WSGIServer, make_server
from wsgiref.util import request_uri

DEFAULT_SCOPES = (
    "openid",
    "https://www.googleapis.com/auth/userinfo.email",
    "https://www.googleapis.com/auth/tasks",
    "https://www.googleapis.com/auth/calendar",
    "https://www.googleapis.com/auth/drive.metadata.readonly",
)
KEYRING_SERVICE = "hot-cross-buns"


class OAuthCancelledError(RuntimeError):
    """Raised when the user cancels before the OAuth callback is exchanged."""


class _LoopbackOAuthHandler(WSGIRequestHandler):
    """Do not log the authorization-code callback URL to stderr."""

    def log_message(self, _format: str, *_args: object) -> None:
        return


class _LoopbackOAuthServer(WSGIServer):
    allow_reuse_address = False


class _OAuthCallbackApplication:
    """Capture one loopback callback without exposing its query string in logs."""

    def __init__(self, success_message: str) -> None:
        self.success_message = success_message
        self.last_request_uri: str | None = None

    def __call__(self, environ: dict[str, Any], start_response: Callable[..., Any]) -> list[bytes]:
        if environ.get("REQUEST_METHOD") != "GET":
            start_response("405 Method Not Allowed", [("Content-Type", "text/plain")])
            return [b"Method not allowed"]
        self.last_request_uri = request_uri(environ)
        payload = self.success_message.encode("utf-8")
        start_response(
            "200 OK",
            [
                ("Content-Type", "text/plain; charset=utf-8"),
                ("Content-Length", str(len(payload))),
                ("Cache-Control", "no-store"),
                ("X-Content-Type-Options", "nosniff"),
            ],
        )
        return [payload]


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
        cancelled: Callable[[], bool] | None = None,
    ) -> OAuthResult:
        if cancelled is not None and cancelled():
            raise OAuthCancelledError("OAuth authorization was cancelled")
        from google_auth_oauthlib.flow import InstalledAppFlow  # type: ignore[import-untyped]

        flow = InstalledAppFlow.from_client_config(
            self.client_config,
            scopes=self.scopes,
            autogenerate_code_verifier=True,
        )
        if cancelled is None:
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
        else:
            credentials = self._run_cancellable_local_server(
                flow,
                open_browser=open_browser,
                timeout_seconds=timeout_seconds,
                cancelled=cancelled,
            )
        if cancelled is not None and cancelled():
            raise OAuthCancelledError("OAuth authorization was cancelled")
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
    def _run_cancellable_local_server(
        flow: Any,
        *,
        open_browser: bool,
        timeout_seconds: int,
        cancelled: Callable[[], bool],
    ) -> Any:
        """Wait for one loopback callback in short intervals so desktop cancel is prompt.

        ``InstalledAppFlow.run_local_server`` owns a blocking server request and has no
        cancellation hook. This keeps the same PKCE flow and loopback redirect shape while
        closing the listener before a cancelled operation returns.
        """
        if timeout_seconds <= 0:
            raise ValueError("OAuth timeout must be positive")
        callback = _OAuthCallbackApplication(
            "HCB received the authorization response. You may return to HCB."
        )
        server = make_server(
            "127.0.0.1",
            0,
            callback,
            server_class=_LoopbackOAuthServer,
            handler_class=_LoopbackOAuthHandler,
        )
        try:
            flow.redirect_uri = f"http://127.0.0.1:{server.server_port}/"
            authorization_url, _ = flow.authorization_url(access_type="offline", prompt="consent")
            if open_browser:
                import webbrowser

                webbrowser.open(authorization_url, new=1, autoraise=True)
            print(f"Open this URL to authorize HCB:\n{authorization_url}")
            deadline = monotonic() + timeout_seconds
            while callback.last_request_uri is None:
                if cancelled():
                    raise OAuthCancelledError("OAuth authorization was cancelled")
                remaining = deadline - monotonic()
                if remaining <= 0:
                    raise TimeoutError("Timed out waiting for response from authorization server")
                server.timeout = min(0.25, remaining)
                server.handle_request()
            if cancelled():
                raise OAuthCancelledError("OAuth authorization was cancelled")
            parsed = urlsplit(callback.last_request_uri)
            if parsed.scheme != "http" or parsed.hostname != "127.0.0.1":
                raise RuntimeError("OAuth callback did not use the loopback listener")
            flow.fetch_token(
                authorization_response=callback.last_request_uri.replace("http", "https", 1)
            )
        finally:
            server.server_close()
        return flow.credentials

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
