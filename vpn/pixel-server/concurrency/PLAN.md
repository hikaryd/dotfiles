# Controlled Pixel browser concurrency

1. Lock the contract with unit tests before implementing: strict read grammar, two
   read slots, exclusive legacy work, own-target cleanup, and failure paths.
2. Add an optional managed CLI shim and stdlib runtime. No gateway changes,
   dependencies, personal addresses, or VPN installer dependency.
3. Verify tests and Python compilation; deployment and real CDP checks belong to
   the caller. This is cooperative scheduling, not a security sandbox.

Read jobs are atomic tool calls. Named legacy sessions persist until explicit
finish; conversation completion cannot be inferred. Owner VNC still disconnects
CDP and can interrupt any job. Cookies remain shared.
