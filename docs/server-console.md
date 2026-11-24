# Server Console

Create a **Server Console** page from Apps. This first version manages Linux
hosts over your own local OpenSSH connection. It provides a resource overview,
systemd service and Docker container lists, bounded logs, fixed start/stop/restart
operations, and a separately approved agent workflow. It does not install a
remote daemon, add sudo, deploy applications, manage websites/databases or offer
an arbitrary shell.

## Connect

Use a desktop build advertising `ssh`, with OpenSSH installed. Verify a new
server's host key independently and establish trust with OpenSSH in your terminal.
The same destination must work without password prompts using your local SSH keys
or ssh-agent. Add an SSH config alias or `user@host`; leaving Port blank preserves
your SSH configuration. ProxyJump is not supported in this version; use a direct
or VPN-reachable target.

Connection profiles and trust snapshots are private on this computer. They do not
enter page records. Credentials, private keys and passwords are never stored in
the page. A destination cannot be edited in place: recreate its profile after
verifying changed SSH configuration or host trust. Old/mobile clients show an
unsupported state, not simulated access.

## Inspect and manage

Select a host to read its overview and resources. While connected, the visible
page refreshes every 15 seconds without overlapping reads. Missing values remain
unknown. An unavailable component, denied access or truncated response is distinct
from a successful empty resource list. Unusual systemd unit names outside the
provider's supported token format are view-only.

Start/stop/restart requires reviewing the exact host and resource. A nonzero exit
code is not success. If a connection drops during a mutation, inspect current
state before deciding to retry: the operation may already have happened. There
is no automatic mutation retry. Logs are shown as text and limited to 100 lines
per UI request (the provider permits at most 200). Human UI activity is retained
only for the current page session.

## Agent access

Enable agent access for a profile from the Agent access view. This registers only
its name and resolved destination summary with this page's workspace, not its
keys. An agent discovers it with `myco action targets`, then requests a grant
bound to the controller machine, exact profile ID and binding. Review the actual
SSH destination in the approval.

`server.ssh.read` permits reads and logs. `server.ssh.manage` adds fixed service
and container lifecycle operations. Grants expire, can be revoked, and cannot be
retargeted. The action framework owns private results, jobs and audit records.
Large results use private JSON artifacts. Shared page data never executes a
command. See [Bound SSH resources](api.md#bound-ssh-resources).

This requires a Hub and desktop App that implement resource-bound SSH grants;
old Hubs and encrypted-workspace actions are unsupported. Publishing this template
does not add native SSH or Hub action support to an older installation.
