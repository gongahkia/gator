"""Safety-critical outbox delivery and remote mutation dispatch."""

from __future__ import annotations

import hashlib
from dataclasses import replace

from .errors import (
    AuthenticationRequired,
    GoogleApiError,
    RequestNotSentError,
    TransientTransportError,
)
from .google_client import Json
from .models import Conflict, EntityType, MutationOperation, OutboxDeliveryState, PendingMutation
from .sync import (
    RATE_LIMIT_REASONS,
    SyncResult,
    _clean_body,
    _metadata,
    _required_remote,
    _RetryCancelled,
    _RetryContext,
    _RetryExhausted,
    _SyncEngineBase,
)


class DeliverySyncMixin(_SyncEngineBase):
    @staticmethod
    def _non_idempotent_create(mutation: PendingMutation) -> bool:
        if mutation.operation is MutationOperation.CREATE and mutation.entity_type in {
            EntityType.TASK,
            EntityType.TASK_LIST,
            EntityType.CALENDAR,
        }:
            return True
        # A cross-list Google Tasks move is sent to the source list. After a
        # response is lost, repeating it may target a task that has already
        # left that source, so require explicit delivery resolution instead.
        return (
            mutation.entity_type is EntityType.TASK
            and mutation.operation is MutationOperation.MOVE
            and bool(mutation.payload.get("source_list_id"))
            and mutation.payload.get("source_list_id") != mutation.payload.get("list_id")
        )

    @staticmethod
    def _event_request_id(mutation: PendingMutation) -> str:
        identity = (
            f"{mutation.account_id}\0{mutation.entity_type.value}\0"
            f"{mutation.entity_id}\0{mutation.operation.value}"
        )
        # Google Calendar event IDs accept base32hex characters. A SHA-256 hex
        # prefix is therefore valid, deterministic, and safely below its limit.
        return "hcb" + hashlib.sha256(identity.encode()).hexdigest()[:40]

    @staticmethod
    def _uncertain_payload(mutation: PendingMutation) -> Json:
        return {
            "kind": "uncertain-delivery",
            "mutation": {
                "entity_type": mutation.entity_type.value,
                "entity_id": mutation.entity_id,
                "operation": mutation.operation.value,
                "payload": mutation.payload,
                "request_id": mutation.request_id,
            },
        }

    def _quarantine_uncertain_create(self, mutation: PendingMutation, reason: str) -> None:
        assert mutation.id is not None
        with self.storage.transaction():
            self.storage.add_conflict(
                Conflict(
                    None,
                    mutation.account_id,
                    mutation.entity_type,
                    mutation.entity_id,
                    self._uncertain_payload(mutation),
                    {
                        "kind": "delivery-status-unknown",
                        "reason": reason,
                        "required_action": "verify Google, then choose delivered or retry",
                    },
                )
            )
            self.storage.complete_mutation(mutation.account_id, mutation.id)

    def recover_interrupted_deliveries(self, account_id: str) -> int:
        """Recover persisted ``sending`` rows without blindly replaying creates."""
        with self.sync_ownership():
            return self._recover_interrupted_deliveries_owned(account_id)

    def _recover_interrupted_deliveries_owned(self, account_id: str) -> int:
        conflicts = 0
        inflight = self.storage.pending_mutations(
            account_id, delivery_state=OutboxDeliveryState.SENDING
        )
        for mutation in inflight:
            assert mutation.id is not None
            if self._non_idempotent_create(mutation):
                self._quarantine_uncertain_create(mutation, "process stopped while sending")
                conflicts += 1
            else:
                # Event creates carry a deterministic Google event ID. Updates,
                # deletes, moves and responses are repeatable against a remote ID.
                self.storage.reset_mutation_pending(
                    account_id, mutation.id, "recovering interrupted delivery"
                )
        return conflicts

    def flush_outbox(self, account_id: str, *, context: _RetryContext | None = None) -> SyncResult:
        with self.sync_ownership():
            return self._flush_outbox_owned(account_id, context=context)

    def _flush_outbox_owned(
        self, account_id: str, *, context: _RetryContext | None = None
    ) -> SyncResult:
        context = context or self._retry_context()
        pushed = 0
        conflicts = self.recover_interrupted_deliveries(account_id)
        for mutation in self.storage.pending_mutations(
            account_id, delivery_state=OutboxDeliveryState.PENDING
        ):
            assert mutation.id is not None
            request_id = mutation.request_id
            if (
                mutation.entity_type is EntityType.EVENT
                and mutation.operation is MutationOperation.CREATE
            ):
                request_id = request_id or self._event_request_id(mutation)
            self.storage.mark_mutation_sending(
                account_id,
                mutation.id,
                request_id=request_id,
                started_at=self.now(),
            )
            sending = self.storage.get_mutation(account_id, mutation.id)
            if sending is None:
                raise RuntimeError(f"outbox mutation {mutation.id} disappeared")
            self.crash_hook("before-request", sending)
            try:

                def push_sending(sending: PendingMutation = sending) -> Json | None:
                    return self._push(sending)

                def retry_is_safe(error: Exception, sending: PendingMutation = sending) -> bool:
                    return isinstance(
                        error, RequestNotSentError
                    ) or not self._non_idempotent_create(sending)

                response = self._retry_call(
                    "Sending local change",
                    push_sending,
                    context,
                    safe_to_retry=retry_is_safe,
                )
            except _RetryCancelled:
                self.storage.reset_mutation_pending(account_id, mutation.id, "sync cancelled")
                return SyncResult(
                    pushed=pushed,
                    conflicts=conflicts,
                    cancelled=True,
                    retry_message=(
                        "Sync cancelled. Local changes remain queued; run sync to resume."
                    ),
                )
            except _RetryExhausted as error:
                self.storage.fail_mutation(account_id, mutation.id, str(error.error))
                return SyncResult(
                    pushed=pushed,
                    conflicts=conflicts,
                    retry_pending=True,
                    retry_after=error.retry_after,
                    retry_exhausted=True,
                    retry_message=(
                        "Sync paused after bounded retries. Local changes remain queued; "
                        "run sync to resume."
                    ),
                )
            except TransientTransportError as exc:
                if self._non_idempotent_create(sending):
                    self._quarantine_uncertain_create(
                        sending, "transport ended before Google confirmed delivery"
                    )
                    conflicts += 1
                    continue
                self.storage.fail_mutation(account_id, mutation.id, str(exc))
                raise
            except GoogleApiError as exc:
                if (
                    sending.entity_type is EntityType.EVENT
                    and sending.operation is MutationOperation.CREATE
                    and sending.request_id is not None
                    and exc.status == 409
                ):
                    response = {"id": sending.request_id}
                elif self._non_idempotent_create(sending) and self._transient(exc):
                    self._quarantine_uncertain_create(
                        sending, f"Google returned ambiguous status {exc.status}"
                    )
                    conflicts += 1
                    continue
                elif exc.status in {401, 403} and exc.reason not in RATE_LIMIT_REASONS:
                    self.storage.fail_mutation(account_id, mutation.id, str(exc))
                    raise AuthenticationRequired(
                        "Google authorization is required", hint="Reconnect this account"
                    ) from exc
                elif exc.is_conflict:
                    with self.storage.transaction():
                        self.storage.add_conflict(
                            Conflict(
                                None,
                                account_id,
                                mutation.entity_type,
                                mutation.entity_id,
                                mutation.payload,
                                {"status": exc.status, "reason": exc.reason},
                            )
                        )
                        self.storage.complete_mutation(account_id, mutation.id)
                    conflicts += 1
                    continue
                else:
                    self.storage.fail_mutation(account_id, mutation.id, str(exc))
                    raise
            except KeyboardInterrupt:
                if self._non_idempotent_create(sending):
                    self._quarantine_uncertain_create(
                        sending, "sync interrupted while Google delivery was unconfirmed"
                    )
                else:
                    self.storage.reset_mutation_pending(account_id, mutation.id, "sync cancelled")
                raise
            self.crash_hook("after-remote-success", sending)
            with self.storage.transaction():
                self._accept_push(sending, response)
                self.storage.complete_mutation(account_id, mutation.id)
            pushed += 1
        return SyncResult(pushed=pushed, conflicts=conflicts)

    def _remote_list(self, account_id: str, value: str) -> str:
        item = self.storage.get_task_list(account_id, value)
        return item.remote_id if item and item.remote_id else value

    def _remote_calendar(self, account_id: str, value: str) -> str:
        item = self.storage.get_calendar(account_id, value)
        return item.remote_id if item and item.remote_id else value

    def _push(self, mutation: PendingMutation) -> Json | None:
        payload, account_id = mutation.payload, mutation.account_id
        body = _clean_body(payload)
        etag = payload.get("etag")
        if mutation.entity_type is EntityType.TASK_LIST:
            task_list = self.storage.get_task_list(account_id, mutation.entity_id)
            remote = task_list.remote_id if task_list else payload.get("remote_id")
            if mutation.operation is MutationOperation.CREATE:
                return self.gateway.create_task_list(body)
            if mutation.operation is MutationOperation.UPDATE:
                return self.gateway.update_task_list(
                    _required_remote(remote, "task list"), body, etag=etag
                )
            self.gateway.delete_task_list(_required_remote(remote, "task list"), etag=etag)
            return None
        if mutation.entity_type is EntityType.TASK:
            task = self.storage.get_task(account_id, mutation.entity_id)
            remote = task.remote_id if task else payload.get("remote_id")
            local_list_id = payload.get("list_id") or (task.list_id if task else None)
            list_id = self._remote_list(account_id, _required_remote(local_list_id, "task list"))
            if mutation.operation is MutationOperation.CREATE:
                return self.gateway.create_task(list_id, body)
            if mutation.operation is MutationOperation.UPDATE:
                return self.gateway.update_task(
                    list_id, _required_remote(remote, "task"), body, etag=etag
                )
            if mutation.operation is MutationOperation.MOVE:
                source_local = payload.get("source_list_id") or local_list_id
                source = self._remote_list(account_id, _required_remote(source_local, "task list"))
                destination = list_id if source_local != local_list_id else None
                parent_local = payload.get("parent")
                previous_local = payload.get("previous")
                parent_task = (
                    self.storage.get_task(account_id, parent_local)
                    if isinstance(parent_local, str)
                    else None
                )
                previous_task = (
                    self.storage.get_task(account_id, previous_local)
                    if isinstance(previous_local, str)
                    else None
                )
                parent = (
                    _required_remote(parent_task.remote_id if parent_task else None, "parent task")
                    if parent_local
                    else None
                )
                previous = (
                    _required_remote(
                        previous_task.remote_id if previous_task else None, "previous task"
                    )
                    if previous_local
                    else None
                )
                return self.gateway.move_task(
                    source,
                    _required_remote(remote, "task"),
                    destination_task_list_id=destination,
                    parent=parent,
                    previous=previous,
                )
            self.gateway.delete_task(list_id, _required_remote(remote, "task"), etag=etag)
            return None
        if mutation.entity_type is EntityType.CALENDAR:
            calendar = self.storage.get_calendar(account_id, mutation.entity_id)
            remote = calendar.remote_id if calendar else payload.get("remote_id")
            if mutation.operation is MutationOperation.CREATE:
                return self.gateway.create_calendar(body)
            if mutation.operation is MutationOperation.SUBSCRIBE:
                return self.gateway.subscribe_calendar(_required_remote(remote, "calendar"))
            if mutation.operation is MutationOperation.REMOVE:
                self.gateway.remove_calendar(_required_remote(remote, "calendar"))
                return None
            if mutation.operation is MutationOperation.UPDATE:
                if payload.get("resource") == "calendar-list":
                    return self.gateway.update_calendar_list(
                        _required_remote(remote, "calendar"), body, etag=etag
                    )
                return self.gateway.update_calendar(
                    _required_remote(remote, "calendar"), body, etag=etag
                )
            self.gateway.delete_calendar(_required_remote(remote, "calendar"), etag=etag)
            return None
        event = self.storage.get_event(account_id, mutation.entity_id)
        remote = event.remote_id if event else payload.get("remote_id")
        local_calendar_id = payload.get("calendar_id") or (event.calendar_id if event else None)
        calendar_id = self._remote_calendar(
            account_id, _required_remote(local_calendar_id, "calendar")
        )
        if mutation.operation is MutationOperation.CREATE:
            if mutation.request_id is None:
                raise ValueError("event create has no deterministic request id")
            body["id"] = mutation.request_id
            return self.gateway.create_event(
                calendar_id,
                body,
                send_updates=payload.get("send_updates", "none"),
                supports_attachments=bool(payload.get("supports_attachments", False)),
                conference_data_version=int(payload.get("conference_data_version", 0)),
            )
        if mutation.operation is MutationOperation.UPDATE:
            return self.gateway.update_event(
                calendar_id,
                _required_remote(payload.get("target_remote_id") or remote, "event"),
                body,
                etag=etag,
                send_updates=payload.get("send_updates", "none"),
                supports_attachments=bool(payload.get("supports_attachments", False)),
                conference_data_version=int(payload.get("conference_data_version", 0)),
            )
        if mutation.operation is MutationOperation.MOVE:
            destination = self._remote_calendar(account_id, payload["destination"])
            return self.gateway.move_event(
                calendar_id, _required_remote(remote, "event"), destination
            )
        if mutation.operation is MutationOperation.RESPOND:
            return self.gateway.respond_event(
                calendar_id,
                _required_remote(remote, "event"),
                payload["response_status"],
                etag=etag,
                comment=payload.get("comment"),
                send_updates=payload.get("send_updates", "all"),
            )
        self.gateway.delete_event(
            calendar_id,
            _required_remote(payload.get("target_remote_id") or remote, "event"),
            etag=etag,
            send_updates=payload.get("send_updates", "none"),
        )
        return None

    def _accept_push(self, mutation: PendingMutation, response: Json | None) -> None:
        if response is None:
            return
        account_id = mutation.account_id
        if mutation.entity_type is EntityType.TASK_LIST:
            current_list = self.storage.get_task_list(account_id, mutation.entity_id)
            if current_list:
                self.storage.upsert_task_list(
                    replace(
                        current_list,
                        remote_id=response.get("id", current_list.remote_id),
                        metadata=_metadata(response),
                    )
                )
        elif mutation.entity_type is EntityType.TASK:
            current_task = self.storage.get_task(account_id, mutation.entity_id)
            if current_task:
                self.storage.upsert_task(
                    replace(
                        current_task,
                        remote_id=response.get("id", current_task.remote_id),
                        metadata=_metadata(response),
                    )
                )
        elif mutation.entity_type is EntityType.CALENDAR:
            current_calendar = self.storage.get_calendar(account_id, mutation.entity_id)
            if current_calendar:
                self.storage.upsert_calendar(
                    replace(
                        current_calendar,
                        remote_id=response.get("id", current_calendar.remote_id),
                        metadata=_metadata(response),
                    )
                )
        else:
            current_event = self.storage.get_event(account_id, mutation.entity_id)
            if current_event:
                remote_id = (
                    mutation.request_id
                    if mutation.operation is MutationOperation.CREATE
                    else response.get("id", current_event.remote_id)
                )
                self.storage.upsert_event(
                    replace(
                        current_event,
                        remote_id=remote_id,
                        metadata=_metadata(response),
                    )
                )
