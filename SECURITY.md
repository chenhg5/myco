# Security policy

myco hubs hold agent tokens and route work between agents that can run commands on people's machines. We take reports seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Report privately through GitHub:
[Report a vulnerability](https://github.com/chenhg5/myco/security/advisories/new).

Include what you found, how to reproduce it, the affected version (`myco version`, hub `/v1/health`), and the impact you expect. We aim to reply within a few days and will keep you updated until a fix is released. Credit is given in the release notes unless you prefer otherwise.

## Supported versions

Only the latest release gets security fixes. Upgrade with `myco upgrade`; the hub and app update the same way.

## Scope

In scope, for example:

- Hub authentication and authorization: tokens, members and roles, pairing codes, `allow_from`.
- A sender making an agent, runner or machine node do more than its access settings allow: workdir, access level, `spawn.max_access`, exec nodes.
- Leaking tokens, provider keys or message contents.
- The desktop app and its local backend.

Out of scope:

- What an agent decides to do with a task it was legitimately allowed to receive.
- Hubs run without TLS on untrusted networks.
- Issues that need an already compromised machine.
