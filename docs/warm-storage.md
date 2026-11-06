# Warm storage

## What it does

When a workflow run closes, the close transfer task finishes its visibility, parent-reply and child-policy work and
then calls `ExecutionManager.MoveToClosedExecutions`. The store decorator
(`common/persistence/wrappers/warmstorage`) copies a slim row into the `closed_executions` table, keyed by
(domain ID, workflow ID, run ID), and deletes the run's `executions` row. The `executions` partition stays small.
On Cassandra the partition key is (domain ID, workflow ID) with run ID as the clustering key, so every closed run of
one workflow ID shares a partition. Retention bounds its width; a cron or continue-as-new chain with a very short
period and long retention makes the widest partitions.

- The slim row holds the run's `WorkflowExecutionInfo` minus auto-reset points, plus version histories. Memo and
  search attributes are kept. Activity, timer, child, signal and request-cancel maps are dropped (they are empty on a
  closed run).
- Reads that miss `executions` fall back to `closed_executions`. Deletes (retention) remove both rows.
- History events never move. `current_executions` is untouched.
- Describe rebuilds the auto-reset points of a Warm run by replaying its history once per cached state.
- Only runs that close after the flag is on move. Existing closed runs stay where they are until retention deletes
  them.

## Enabling it

1. Run `update-schema` on every database so the `closed_executions` table exists (Cassandra 0.56, MySQL 0.9,
   Postgres 0.9, SQLite 0.2). Services refuse to start on an older schema.
2. Set the dynamic config key `system.enableWarmStorage` to `true` with a `domainID` constraint for one domain.
   The default is `false`. The key is read per domain ID.
3. Watch the metrics below, then widen the rollout.

Supported stores: Cassandra, MySQL, Postgres, SQLite. MongoDB and DynamoDB are not supported: do not enable the flag
there (the move fails open and every close logs an error).

## Metrics

| Metric | Meaning | Alert on |
|---|---|---|
| `warm_storage_moves` | Runs moved | Flat at 0 while the flag is on |
| `warm_storage_move_failures` | A move failed; the run stayed in `executions` (nothing is lost) | Sustained rate above 0: table missing, DB errors |
| `warm_storage_move_conflicts` | A writer changed the run between the move's read and its delete; the run stayed hot | A steady rate: something keeps writing to freshly closed runs |
| `warm_storage_rehydrates` | A write (replication backfill, conflict resolution, refresh, child-start dedupe) targeted a Warm run, and its `executions` row was put back | Informational; a steady high rate means closed runs keep being written and are not saving space |
| `warm_storage_fallback_hits` | Reads served from `closed_executions` | Sudden drops or spikes against the move rate |

## Rolling back

Set the flag to `false`. New closes stay in `executions`. Warm rows stay readable and deletable: once any run has
moved, a sentinel row in `closed_executions` keeps the read fallback and the retention delete on for every domain,
until retention has removed the Warm rows. Nothing needs migrating back.

## Cost for a cluster that never enables it

- No connection and no query at startup: the closed store is opened on first use.
- One sentinel point read per missed `executions` read and per retention delete (at most one a minute while
  `closed_executions` can't be read, for example when the table is missing). A host whose flag lags uses it to learn
  that another host has moved runs, so its retention deletes never leave a slim row behind.
- The table does not have to exist.

## Known limits

- Runs closed before the flag was turned on are not migrated.
- A write to a Warm run puts its `executions` row back (the slim row stays until retention). If the write leads to a
  new close (for example a replicated branch that closes at a higher version), the close task moves the run again.
  Otherwise (a lower-version backfill after a failover) it stays in `executions` until retention; reads prefer the hot
  row and retention deletes both. Its auto-reset points are lost with the move, so Describe rebuilds them from history
  (a closed run with no points and a slim row is treated as Warm). That costs one extra `closed_executions` read when
  such a run is loaded, which only happens for closed runs without points.
- Split close tasks do not move runs. `RecordWorkflowClosed` has no producer any more, and
  `RecordChildExecutionCompleted` is only created for an already-closed child whose own close task is what moves it.
- The schema must be at the new version (`update-schema`) before the binary starts, even with the flag off.
- Signal-with-start dedup on a closed Warm run loses `SignalRequestedIDs`; a duplicate signal request ID on a Warm run
  returns AlreadyCompleted.
- The first Describe of a Warm run after a cache miss replays its history (one read per history page). Measured on a
  single Cassandra node: Describe p99 +4.9ms, GetHistory p99 +0.4ms.
- On Cassandra the move's delete is a lightweight transaction (`IF next_event_id = ?`), so each closed run costs one
  more LWT. Measured: close-task processing +16–22ms on average. The condition is what stops a write that lands during
  the move (including one from a new shard owner) from being deleted, so a plain delete is not a safe substitute.
- Concrete-execution scans (`ListConcreteExecutions`, the executions scanner and fixer) do not visit Warm runs. Current
  execution checks do, through `IsWorkflowExecutionExists`.
- With the flag on and the table missing, a read of a run that does not exist returns the table error instead of
  `EntityNotExists`, so the scavenger can never mistake it for a deleted run.
