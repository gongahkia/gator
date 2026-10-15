# Product constraints
Upload accepts CSV up to 20 MB. An upload can queue for up to five minutes. Failed jobs are never automatically retried. The server returns a job ID on acceptance, not on local file selection. Product telemetry can count completed jobs and duplicate retries. No new backend status endpoint is in scope for this design iteration.
