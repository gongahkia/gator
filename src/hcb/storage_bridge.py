"""Durable request receipts for local desktop bridge mutations."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime, timedelta

from .storage import _StorageCore


@dataclass(frozen=True, slots=True)
class BridgeMutationReceipt:
    method: str
    path: str
    request_hash: str
    response_status: int
    response_data: str


class BridgeRepository(_StorageCore):
    """Store replay-safe local bridge results alongside the affected write."""

    def bridge_mutation_receipt(
        self, account_id: str, idempotency_key: str
    ) -> BridgeMutationReceipt | None:
        row = self.connection.execute(
            """SELECT method,path,request_hash,response_status,response_data
            FROM bridge_mutation_receipts WHERE account_id=? AND idempotency_key=?""",
            (account_id, idempotency_key),
        ).fetchone()
        return (
            BridgeMutationReceipt(
                row["method"],
                row["path"],
                row["request_hash"],
                int(row["response_status"]),
                row["response_data"],
            )
            if row is not None
            else None
        )

    def save_bridge_mutation_receipt(
        self,
        account_id: str,
        idempotency_key: str,
        *,
        method: str,
        path: str,
        request_hash: str,
        response_status: int,
        response_data: str,
    ) -> None:
        self.connection.execute(
            """INSERT INTO bridge_mutation_receipts(
                account_id,idempotency_key,method,path,request_hash,response_status,response_data,
                created_at
            ) VALUES (?,?,?,?,?,?,?,?)""",
            (
                account_id,
                idempotency_key,
                method,
                path,
                request_hash,
                response_status,
                response_data,
                datetime.now(UTC).isoformat(),
            ),
        )
        expires_before = (datetime.now(UTC) - timedelta(days=7)).isoformat()
        self.connection.execute(
            "DELETE FROM bridge_mutation_receipts WHERE created_at<?", (expires_before,)
        )
