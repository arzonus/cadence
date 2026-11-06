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
- Only runs that close while the mode is `on` move. Existing closed runs stay where they are until retention deletes
  them.

## Modes

`system.warmStorageMode` is read per domain ID. The default is `off`; an unknown value counts as `off`.

| Mode | Close task | Reads, deletes, put-back |
|---|---|---|
| `off` | Nothing | Never touch `closed_executions`: no reads, no deletes, no connection |
| `shadow` | Writes the slim row, keeps the `executions` row | Read fallback, delete of both rows; a hot closed run is compared with its slim row |
| `read_only` | Nothing | Read fallback, delete of both rows, put-back of a moved run on write |
| `on` | Moves the run | Same as `read_only` |

Shadow follows the same skip rules as `on` (runs that aren't Completed, and runs with activities or children in
flight, are skipped and counted in `warm_storage_moves_skipped`) and fails open the same way.

When `GetWorkflowExecution` reads a hot Completed run in shadow mode, it also reads the slim row. No slim row is
fine: the run may have closed before shadow was turned on. Otherwise it compares the state the slim row restores to
with the hot state projected the same way, field by field. A difference increments
`warm_storage_shadow_mismatches` and logs the domain, workflow and run IDs and the names of the fields, never the
values. Read or decode errors are only logged. The caller always gets the hot row. Shadow adds one `closed_executions`
read per hot closed-run read, so keep it for the soak, not for good.

## Enabling it

1. Run `update-schema` on every database so the `closed_executions` table exists (Cassandra 0.56, MySQL 0.9,
   Postgres 0.9, SQLite 0.2). Services refuse to start on an older schema. This applies even when every domain stays
   `off`.
2. Roll out one domain at a time, in this order: `off` -> `shadow` -> `on`. Set `system.warmStorageMode` with a
   `domainID` constraint. The order also covers dynamic-config lag between hosts: a host that is one step behind
   still reads and deletes what the others wrote.
3. In shadow, watch `warm_storage_shadow_mismatches` stay at 0 before moving to `on`.
4. Watch the metrics below, then widen the rollout.

Supported stores: Cassandra, MySQL, Postgres, SQLite. MongoDB and DynamoDB are not supported: do not set any mode
but `off` there (the move fails open and every close logs an error).

## Metrics

| Metric | Meaning | Alert on |
|---|---|---|
| `warm_storage_moves` | Runs moved | Flat at 0 while the mode is `on` |
| `warm_storage_move_failures` | A move failed; the run stayed in `executions` (nothing is lost) | Sustained rate above 0: table missing, DB errors |
| `warm_storage_move_conflicts` | A writer changed the run between the move's read and its delete; the run stayed hot | A steady rate: something keeps writing to freshly closed runs |
| `warm_storage_moves_skipped` | A closed run still listed an activity or child in flight (it timed out or was terminated), so it stayed in `executions` and Describe keeps listing them | Informational; a high share of closes means fewer savings |
| `warm_storage_rehydrates` | A write (replication backfill, conflict resolution, refresh, child-start dedupe) targeted a Warm run, and its `executions` row was put back | Informational; a steady high rate means closed runs keep being written and are not saving space |
| `warm_storage_fallback_hits` | Reads served from `closed_executions` | Sudden drops or spikes against the move rate |
| `warm_storage_shadow_mismatches` | In shadow, a hot closed run and its slim row disagree | Any value above 0: do not go to `on` |

## Rolling back

Go `on` -> `read_only` -> wait one retention period -> `off`. In `read_only` nothing new moves, and moved runs stay
readable and deletable, so retention removes both rows as it goes. Once retention has passed, no slim rows are left
and `off` is safe. Nothing needs migrating back.

Do not go straight from `on` to `off`. Moved runs become invisible (reads miss, and `IsWorkflowExecutionExists`
reports them gone), and retention deletes only the `executions` row, which is already gone, so their slim rows stay
in `closed_executions` for good. The history scavenger also treats an invisible run as deleted (`DescribeMutableState`
returns not-found) and deletes its history, so the run can't be brought back by switching the mode on again.

Shadow leaves slim rows too, so back out of it the same way: `shadow` -> `read_only` -> wait -> `off`. The runs
stay readable from `executions` either way; only the slim rows would be left behind.

## Cost for a cluster that never enables it

Nothing. In `off` the decorator makes no call to `closed_executions`, and the closed store is opened on first use, so
there is no connection at startup either. The table does not have to exist, but the schema version still has to be
current.

## Known limits

- Runs closed before the mode was `on` are not migrated.
- Runs closed during shadow keep both rows. After the domain switches to `on` they stay in `executions` until
  retention deletes both rows; nothing moves them.
- Runs that close with an activity or child still in flight (timed out, terminated, or a parent whose children are
  still running) are not moved, so Describe keeps listing them exactly as before.
- A write to a Warm run puts its `executions` row back (the slim row stays until retention). If the write leads to a
  new close (for example a replicated branch that closes at a higher version), the close task moves the run again.
  Otherwise (a lower-version backfill after a failover) it stays in `executions` until retention; reads prefer the hot
  row and retention deletes both. Its auto-reset points are lost with the move, so Describe rebuilds them from history
  (a closed run with no points and a slim row is treated as Warm). That costs one extra `closed_executions` read when
  such a run is loaded, which only happens for closed runs without points.
- Split close tasks do not move runs. `RecordWorkflowClosed` has no producer any more, and
  `RecordChildExecutionCompleted` is only created for an already-closed child whose own close task is what moves it.
- The schema must be at the new version (`update-schema`) before the binary starts, even with every mode `off`.
- Signal-with-start dedup on a closed Warm run loses `SignalRequestedIDs`; a duplicate signal request ID on a Warm run
  returns AlreadyCompleted.
- The first Describe of a Warm run after a cache miss replays its history (one read per history page). Measured on a
  single Cassandra node: Describe p99 +4.9ms, GetHistory p99 +0.4ms.
- On Cassandra the move's delete is a lightweight transaction (`IF next_event_id = ?`), so each closed run costs one
  more LWT. Measured: close-task processing +16–22ms on average. The condition is what stops a write that lands during
  the move (including one from a new shard owner) from being deleted, so a plain delete is not a safe substitute.
- Concrete-execution scans (`ListConcreteExecutions`, the executions scanner and fixer) do not visit Warm runs. Current
  execution checks do, through `IsWorkflowExecutionExists`.
- With a mode other than `off` and the table missing, a read of a run that does not exist returns the table error instead of
  `EntityNotExists`, so the scavenger can never mistake it for a deleted run.
