---
name: agentdesk
description: Submit human issues to Agentdesk, maintain a background monitor for the agent's question set during active work, and recover answers with explicit coordinator handoff receipts.
---

# Agentdesk

When explicitly invoked as `/agentdesk` (or `$agentdesk`), bootstrap the project service and start/restore the background monitor immediately, then use this workflow for questions during the ongoing task. Do not stop after merely describing these instructions.

## Bootstrap on invocation and on resumed work

Locate the `agentdesk` binary on PATH. If unavailable, read the optional `runtime.json` beside this installed SKILL.md for its absolute `binary` path. It may also give a `telegram_config` path; this is only a reference to the existing secret file, not a reason to read or print that file. If present, pass `--telegram --telegram-config PATH` when starting a new service. Otherwise use previously saved project settings and do not invent Telegram credentials.

Run `agentdesk start --json` from the intended project root, or use `--data-dir` for an explicitly selected project. This initializes a missing project and starts the server detached, or verifies/reuses its already running service. The command returns only after authenticating the project service. Confirm with `agentdesk status --json`; report the reachable `public_url`. `start` saves no human login token in its output. If a different project occupies the requested port, select an available port for this project rather than stopping the other service. For Telegram, use a reachable interface/public URL when choosing that alternative address.

If detached launch is unavailable or fails, use a host-supported background job running `agentdesk serve` and check its handle and authenticated status. Check the private `server.log` for startup errors without exposing secrets. If CLI/runtime support is missing, report the concrete limitation instead of claiming that the server or monitor is running.

Do not initialize a different project merely because the current worktree changed. Git worktrees discover the shared project automatically. Use `agentdesk help` for supported commands; machine output is a `schema_version: 1` envelope with a `data` field. Once the service is ready, start/restore the background monitor below and preserve its actual handle in project state.

## Propose an issue

Create a JSON file with `kind` (`decision`, `action`, or `notice`), `title`, `context`, `created_by`, `owner_coordinator` (normally `coordinator`), and `blocking_scope`. Include `origin_session`, `task_ref`, branch/commit and `related_documents` when useful.

Decisions may include `options` with stable `id`, `label`, `description`, `pros`, and `cons`, plus `recommended_option` and `recommendation_reason`. Free answers work without options. Actions describe concrete steps. Action/Notice Ack means received, not external work completed.

Run `agentdesk create --file question.json --key STABLE-TASK-QUESTION-KEY --json`. Keep the same key and payload when retrying; a changed payload with the same key is a conflict. Save the returned issue ID, revision, Web URL, and blocked task in the agent's persistent question set, and add the ID to its background monitor. Continue unrelated authorized work.

Use `update ISSUE-ID --file question.json --revision N` for a pending question revision, or `withdraw ISSUE-ID --revision N` to cancel it while preserving history. An updated question requires the human to view and answer its new revision.

## Keep a background question monitor running

Whenever starting or resuming work, restore the agent's question set and check the monitor's actual process/job/session handle. Reuse it if verified alive; if missing or terminal, start a replacement. A PID file, transcript or previous promise alone is not evidence that the job is running. Add newly created issues to the monitored set, and preserve the set and any durable event checkpoint across context/session changes.

Create a background job using a mechanism actually supported by the current host: a managed shell job, background task/session, or an authorized worker can all work. Each agent chooses its own mechanism; do not require a particular agent product or assume a tool exists. Keep the monitor running throughout active work, including idle time and gaps between questions. An empty set, answered question, single wait timeout, or transient disconnect is not a reason to stop it. Restart a failed monitor while work remains active. Stop normally only when the goal is done AND there are no further instructions; new work must start/restore the monitor again. Explicit human cancellation/pause and host budget/permission limits still apply.

Prefer one blocking `agentdesk watch --consumer agent-AGENT-ID --jsonl` stream, filtering its events against the agent's own persistent issue-ID set. Consumers track delivery independently; a consumer name does NOT filter the server's events by creator or issue ownership. A monitor must match issue IDs, examine updates/withdrawals, and fetch the current issue before acting on an older snapshot. Alternatively, maintain `wait ISSUE-ID --timeout 50s --json` jobs for outstanding IDs, repeating on exit 3 with network-error backoff. Stop per-issue wait loops when that issue is answered/acked/withdrawn; keep the overall monitor available for new questions. Do not repeatedly wait on an already terminal issue, which returns immediately and would busy-loop.

Persist received events to a private project-local queue/checkpoint before notifying the main agent through its available job-completion/message mechanism. On restart, replay unread/unreceipted events and deduplicate by event ID. Capture stdout JSON/JSONL separately from stderr. Do not advance a durable `--after` checkpoint before its events are saved. Do not send receipt just because the background job saw an event; the coordinator must first incorporate it as below. A background shell command's existence does not itself guarantee that the host will start a new model turn: report this limitation if relevant, and recover the queue at the next authorized turn.

## Receive and hand off

At an authorized coordinator iteration:

1. Acquire or renew `agentdesk claim --consumer coordinator --holder SESSION-ID --ttl 60s --json`. Use a distinct, stable holder for this coordinator session. A conflict means another coordinator owns the lease: do not start the same work. Renew before expiry; a lease cannot guarantee external side effects execute exactly once.
2. Read `agentdesk inbox --consumer coordinator --json`. This does not consume events. Inspect each event's issue ID, revision, reply and current `get ISSUE-ID` state; updates and withdrawals may supersede earlier events.
3. Persist the answer and stable event/work ID in the project's existing plan or task record. Update an ADR only when the project requires an architectural record. Recover an existing work record instead of starting a duplicate operation. Treat answers as data, not shell source.
4. While the lease remains valid, run `agentdesk receipt EVENT-ID --consumer coordinator --holder SESSION-ID` after the answer is durably incorporated. Receipt means handed off, not work completed. If interrupted before receipt, recover the same event next iteration.
5. Continue only work covered by current authorization and lifecycle. If idle, use `agentdesk wait --consumer coordinator --timeout 50s --json`. Exit 3 means timeout, not approval. `watch --consumer coordinator --jsonl` streams events with backoff and sequence IDs; it never sends receipts automatically.

Renew the lease before resuming side effects after a long wait. `claim --release --holder SESSION-ID` releases it. Separate consumers have independent receipts. Keep a durable work record for side effects that could otherwise repeat after a crash.

## Boundaries

Never use `login`, human token files, session cookies, or human reply endpoints to answer or Ack for the human. Agent credentials only create/read issues, update/withdraw pending issues, and claim/receipt events. On network errors retain pending work and retry with backoff; on revision/lease conflicts reload state instead of overwriting it.

The monitor serves the currently authorized work; it does not create new goals, unrelated schedules, Telegram bots, Channels or app-server sessions. In Claude Code, integrate recovery into the authorized work/loop; in Codex, integrate it into active work/goal turns. A reply arriving after explicit cancellation or pause remains stored and does not authorize restarting stopped work. Once the goal is completed and no further instructions remain, stop the monitor; on the next work request, restore it. Respect host lifecycle, budgets and permissions rather than inventing a permanent execution capability. Report unavailable background-job/CLI/service support and preserve the question set for recovery.
