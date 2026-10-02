# WSM Server — agent starting point

Before working on this application, read `../aimemory/wsm-server.md`, then
`README.md` and the source files relevant to the task. The memory describes the
verified capabilities, protocol, configuration, known gaps and next steps.

This is a separate Go module: run Go commands from `wsm-server/`.
Update the memory when changing behavior, configuration, protocol or deployment.
Distinguish local verification from a deployed service or a real WiseMED test.
Preserve existing deployment configuration and runtime files; use a temporary
configuration and loopback port for smoke tests. Never copy credentials or tokens
into documentation. Reader commands are implemented in `../readersv3/`, not here.
