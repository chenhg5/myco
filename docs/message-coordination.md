# Task trees and reassignment

Use `myco message tree --id <task>` to inspect delegations without reading their
contents. Text output is a compact tree of task IDs, agents, statuses, ages and
waiting dependencies. JSON output nests nodes under `root.children`, with an
`as_of` timestamp and a `truncated` indicator. `--limit` defaults to 200 and
accepts 1–500 nodes; traversal also stops at depth 32. Repeated nodes are marked
`reference` and are not expanded again. The view is a live traversal rather than
an atomic snapshot.

New task sends (including task broadcasts) automatically carry the incoming `MYCO_MESSAGE_ID` as `parent_id`;
`myco message send --parent <id>` can specify it explicitly. The Hub verifies that
the parent was addressed to the sending agent. This includes tasks delegated
while handling a reply to a task. Existing reply and hold links are also used.
For older tasks without a recorded parent, delegations by a task's recipient in
the same thread are marked `thread_inferred`; they are hints, not proven ancestry.
`waiting_on` distinguishes task, after, answer, manual, unknown and restricted
waits. Restricted waits do not expose their IDs.

Participants, project coordinators and administrators can inspect task trees.
Descendant metadata is filtered using project and agent visibility; it does not
grant access to bodies, results, attachments or private agents. A tree can omit
inaccessible descendants. Metadata from task sends in another project is not
made visible just by sharing a parent.

Use `myco message reassign --id <task> --to <agent> [--note "context"]` to move an
unclaimed queued task. Only its sender, a coordinator of its project, or an
administrator can do this. `--dry-run` performs validation without changing the
task, granting file access, waking a recipient or writing an audit record (CLI
exit 10). A task that was ever delivered is refused even if a lease recovery or
hold later returned it to queued. Push delivery also records possible disclosure
before invoking the external waker, even if the wake subsequently fails.
Inbox wait also marks the tasks it returns, since its response includes bodies. Stop
the task and send a new one instead. Tasks that predate this delivery guard are
conservatively refused after upgrade: their queued state cannot prove that a
previous push or inbox wait never disclosed them. Newly created tasks can be
reassigned normally.

Reassignment atomically preserves the task ID, original sender, body,
attachments, project and thread while changing the recipient. Existing holds
and sender references remain valid. The new recipient must be visible to the caller and, for a project task,
belong to that unarchived project. Agent-origin tasks retain the original
sender's recipient visibility and allow-from restrictions. Console-origin tasks
use the normal console-send exception; receiving exec nodes still enforce their
console-owner rules. Reassignment does not change the original sender. Coordinators cannot use a
known task ID to forward an otherwise hidden private task. Claims and reassignment race atomically: the old recipient cannot
claim a successfully moved task. Existing file grants are not revoked (they may
also serve other messages); the new recipient receives attachment access in the
same transaction. Previously shared files are not recalled.

The optional note (at most 2,000 characters) is separate from the original body;
the runner presents it as context, not authorization. The audit event
`message.reassign` records the actor, task and old/new recipients, but never the
body or note text. The task's original send-time grant is cleared on reassignment.
End-to-end encrypted tasks return `unsupported`, with a suggestion that the
original sender use stop + send to encrypt for the new recipient. The Hub never
decrypts or re-encrypts them.

Old runners ignore the new optional response fields. Against an older Hub, the
client retries a send without `parent_id` only for that exact unknown-field
validation error. Tree and reassign return a clear `unsupported` error when the
routes are unavailable; they do not change identities or silently emulate the
operation.
