"""Public error taxonomy shared by the CLI, TUI, and scheduler."""

from __future__ import annotations

from enum import IntEnum


class ExitCode(IntEnum):
    OK = 0
    USAGE = 2
    NOT_FOUND = 3
    CONFLICT = 4
    AUTH_REQUIRED = 5
    OFFLINE = 6
    REMOTE_FAILURE = 7
    STORAGE_FAILURE = 8
    CONFIGURATION = 9


class HcbError(Exception):
    """An actionable application failure safe to present to a user."""

    exit_code = ExitCode.REMOTE_FAILURE

    def __init__(self, message: str, *, hint: str | None = None) -> None:
        super().__init__(message)
        self.message = message
        self.hint = hint


class ConfigurationError(HcbError):
    exit_code = ExitCode.CONFIGURATION


class AuthenticationRequired(HcbError):
    exit_code = ExitCode.AUTH_REQUIRED


class OfflineError(HcbError):
    exit_code = ExitCode.OFFLINE


class NotFoundError(HcbError):
    exit_code = ExitCode.NOT_FOUND


class ConflictError(HcbError):
    exit_code = ExitCode.CONFLICT


class SyncBusyError(ConflictError):
    """Another process owns synchronization for this local database."""


class StorageError(HcbError):
    exit_code = ExitCode.STORAGE_FAILURE


class RequestNotSentError(Exception):
    """A transport failure that guarantees no remote request was transmitted."""


class TransientTransportError(Exception):
    """A temporary transport failure where the remote delivery is unknown."""


class GoogleApiError(HcbError):
    """A sanitized Google API failure."""

    def __init__(
        self,
        status: int,
        message: str,
        *,
        reason: str | None = None,
        retry_after: float | None = None,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.reason = reason
        self.retry_after = retry_after

    @property
    def retryable(self) -> bool:
        return self.status in {408, 429} or 500 <= self.status < 600

    @property
    def is_conflict(self) -> bool:
        return self.status in {409, 410, 412}
