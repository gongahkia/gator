import json
import socket
from concurrent.futures import ThreadPoolExecutor
from threading import Barrier

import pytest

from hcb.errors import GoogleApiError, RequestNotSentError, TransientTransportError
from hcb.google_client import GoogleApiClient, Page


class Response(dict):
    def __init__(self, status, **headers):
        super().__init__(headers)
        self.status = status


class HttpFailure(Exception):
    def __init__(self, status, reason="bad", **headers):
        self.resp = Response(status, **headers)
        self.content = json.dumps(
            {"error": {"message": "sanitized", "errors": [{"reason": reason}]}}
        ).encode()


class Request:
    def __init__(self, result=None, error=None):
        self.result = result or {}
        self.error = error
        self.headers = {}

    def execute(self):
        if self.error:
            raise self.error
        return self.result


class CalendarListResource:
    def __init__(self, calls):
        self.calls = calls
        self.last_request = None

    def patch(self, **kwargs):
        self.calls.append(("calendarList.patch", kwargs))
        self.last_request = Request({"id": kwargs["calendarId"]})
        return self.last_request


class ColorsResource:
    def __init__(self, calls):
        self.calls = calls

    def get(self, **kwargs):
        self.calls.append(("colors.get", kwargs))
        return Request({"calendar": {"1": {"background": "#123456"}}})


class EventsResource:
    def __init__(self, calls):
        self.calls = calls

    def list(self, **kwargs):
        self.calls.append(("events.list", kwargs))
        return Request({"items": [], "nextSyncToken": "next"})


class CalendarService:
    def __init__(self):
        self.calls = []
        self._calendar_list = CalendarListResource(self.calls)
        self._colors = ColorsResource(self.calls)
        self._events = EventsResource(self.calls)

    def calendarList(self):
        return self._calendar_list

    def colors(self):
        return self._colors

    def events(self):
        return self._events


class TasksResource:
    def __init__(self):
        self.calls = []

    def list(self, **kwargs):
        self.calls.append(kwargs)
        return Request({"items": []})


class TasksService:
    def __init__(self):
        self._tasks = TasksResource()

    def tasks(self):
        return self._tasks


def client_with_calendar_service():
    calendar = CalendarService()
    return GoogleApiClient(
        tasks_service=object(), calendar_service=calendar, drive_service=object()
    ), calendar


def test_tasks_list_requests_largest_supported_page() -> None:
    tasks = TasksService()
    client = GoogleApiClient(tasks_service=tasks, calendar_service=object(), drive_service=object())

    assert client.list_tasks(
        "list-id", page_token="page-2", updated_min="2026-09-13T00:00:00Z"
    ) == Page(())
    assert tasks._tasks.calls == [
        {
            "tasklist": "list-id",
            "pageToken": "page-2",
            "updatedMin": "2026-09-13T00:00:00Z",
            "maxResults": 100,
            "showCompleted": True,
            "showDeleted": True,
            "showHidden": True,
        }
    ]


def test_parallel_reads_use_distinct_closing_http_transports(monkeypatch):
    import google_auth_httplib2
    import googleapiclient.http

    barrier = Barrier(2)
    authorized = []

    class WorkerRequest(Request):
        def execute(self, http=None):
            assert http is not None
            authorized.append(http)
            barrier.wait(timeout=2)
            return {"items": []}

    class WorkerTasksResource:
        def list(self, **kwargs):
            return WorkerRequest()

    class WorkerTasksService:
        def tasks(self):
            return WorkerTasksResource()

    class WorkerHttp:
        def __init__(self, credentials, *, http):
            self.credentials = credentials
            self.http = http
            self.closed = False

        def close(self):
            self.closed = True

    monkeypatch.setattr(googleapiclient.http, "build_http", object)
    monkeypatch.setattr(google_auth_httplib2, "AuthorizedHttp", WorkerHttp)
    credentials = object()
    client = GoogleApiClient(
        credentials=credentials,
        tasks_service=WorkerTasksService(),
        calendar_service=object(),
        drive_service=object(),
    )

    with ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(client.list_tasks, ("a", "b")))

    assert results == [Page(()), Page(())]
    assert len(authorized) == 2
    assert authorized[0] is not authorized[1]
    assert all(item.credentials is credentials and item.closed for item in authorized)


def test_credential_free_client_keeps_fake_transport_in_background_thread():
    client, calendar = client_with_calendar_service()

    with ThreadPoolExecutor(max_workers=1) as pool:
        page = pool.submit(client.list_events, "primary").result()

    assert page.next_sync_token == "next"
    assert calendar.calls[0][0] == "events.list"


@pytest.mark.parametrize("status", [401, 403, 404, 409, 410, 429, 500, 503])
def test_http_statuses_are_mapped_without_network(status):
    request = Request(error=HttpFailure(status, Retry_After="4"))
    with pytest.raises(GoogleApiError) as raised:
        GoogleApiClient._execute(request)
    assert raised.value.status == status
    assert raised.value.reason == "bad"
    assert str(raised.value) == "sanitized"
    assert raised.value.retryable is (status in {408, 429} or status >= 500)
    assert raised.value.is_conflict is (status in {409, 410, 412})


def test_execute_applies_if_match_and_returns_json():
    request = Request({"id": "remote"})
    assert GoogleApiClient._execute(request, etag='"v1"') == {"id": "remote"}
    assert request.headers["If-Match"] == '"v1"'


@pytest.mark.parametrize("error", [ConnectionRefusedError(), socket.gaierror()])
def test_pre_connection_failures_are_explicitly_safe_to_retry(error):
    with pytest.raises(RequestNotSentError):
        GoogleApiClient._execute(Request(error=error))


@pytest.mark.parametrize("error", [BrokenPipeError(), ConnectionResetError(), TimeoutError()])
def test_interrupted_requests_are_transient_but_not_confirmed_unsent(error):
    with pytest.raises(TransientTransportError):
        GoogleApiClient._execute(Request(error=error))


def test_retry_after_http_date_is_converted_to_a_delay(monkeypatch: pytest.MonkeyPatch) -> None:
    class FixedDateTime:
        @classmethod
        def now(cls, tz):
            return __import__("datetime").datetime(2026, 8, 24, 12, 0, tzinfo=tz)

    monkeypatch.setattr("hcb.google_client.datetime", FixedDateTime)
    request = Request(error=HttpFailure(429, **{"Retry-After": "Mon, 24 Aug 2026 12:00:07 GMT"}))
    with pytest.raises(GoogleApiError) as raised:
        GoogleApiClient._execute(request)
    assert raised.value.retry_after == 7


def test_calendar_list_patch_uses_calendar_list_endpoint_and_if_match():
    client, calendar = client_with_calendar_service()

    assert client.update_calendar_list(
        "primary", {"selected": False, "defaultReminders": []}, etag='"list-v1"'
    ) == {"id": "primary"}
    name, arguments = calendar.calls[0]
    assert name == "calendarList.patch"
    assert arguments == {
        "calendarId": "primary",
        "body": {"selected": False, "defaultReminders": []},
    }
    assert calendar._calendar_list.last_request.headers == {"If-Match": '"list-v1"'}


def test_calendar_colors_uses_colors_get_endpoint():
    client, calendar = client_with_calendar_service()

    assert client.calendar_colors() == {"calendar": {"1": {"background": "#123456"}}}
    assert calendar.calls == [("colors.get", {})]


def test_expanded_events_request_uses_google_range_contract():
    client, calendar = client_with_calendar_service()

    page = client.list_events(
        "primary",
        page_token="page-2",
        time_min="2026-08-21T00:00:00Z",
        time_max="2026-08-28T00:00:00Z",
        single_events=True,
    )
    assert page.next_sync_token == "next"
    assert calendar.calls == [
        (
            "events.list",
            {
                "calendarId": "primary",
                "pageToken": "page-2",
                "syncToken": None,
                "timeMin": "2026-08-21T00:00:00Z",
                "timeMax": "2026-08-28T00:00:00Z",
                "maxResults": 1000,
                "singleEvents": True,
                "showDeleted": True,
            },
        )
    ]


def test_events_sync_token_rejects_time_bounds():
    client, calendar = client_with_calendar_service()

    with pytest.raises(ValueError, match="syncToken"):
        client.list_events("primary", sync_token="sync-1", time_min="2026-08-21T00:00:00Z")
    assert calendar.calls == []
