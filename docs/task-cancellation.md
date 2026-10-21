# Task interruption and execution cancellation

The Coordinator owns the decision to stop work. The Runner owns execution termination. A committed stop decision and a confirmed process exit are separate facts.

## State contract

Task and Run use `interrupted` for work stopped by an administrator. Runner executions keep their existing distinction: `cancelled` for requested cancellation and `interrupted` for interruption during Runner restart.

The Task state machine permits these manual transitions to `interrupted`:

- `pending`, `claimed`, `running`, `finalizing`
- `retry_pending`, `waiting_upstream`, `waiting_merge`
- `human_required`, `blocked`

`interrupted` is terminal. Repeating the same transition is idempotent. It has no transition to dispatch, retry, recovery or success. A successful Task retains `succeeded`.

The Task drawer shows manual stop transitions as dashed arrows. Arrows describe allowed transitions, not the observed history of a Task. Run and Execution drawers do not have separate state diagrams.

## Cancellation boundary

Task, current Run and current Execution stop requests must resolve to the same Task decision. A request against a historical Run must not stop a newer Run. No placeholder Run or Execution is created when no such object exists.

Stopping a Task must preserve completed results, attempt counts, dedupe identity, logs and artifacts. Active Runs become `interrupted`; completed Runs retain their outcomes. A successful Task with a remaining execution requires execution cleanup without rewriting the successful delivery.

Stop and dispatch must arbitrate in the same tenant database transaction boundary. Each external action must also check that decision before admission. If stop commits first, the action must not be sent. An action admitted first can still reach its destination; cancellation cannot retract an external request that was already admitted.

Stopping work must not itself create a code-host comment or an IM notification. Durable cancellation on the original Runner must block a delayed Create request, including after restart. The Coordinator must track delivery of cancellation and actual exit independently.

## Implementation availability

The state contract and diagram are the first implementation increment. They do not expose a stop endpoint or stop control. Stop controls require atomic stop persistence, action admission, durable Runner cancellation and exit reconciliation before they can be enabled.
