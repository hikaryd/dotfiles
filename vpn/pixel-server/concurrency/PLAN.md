# Controlled Pixel browser concurrency

1. Lock the contract with unit tests before implementing: strict read grammar, four
   read slots, exclusive legacy work, own-target cleanup, and failure paths.
2. Add an optional managed CLI shim and stdlib runtime. No gateway changes,
   dependencies, personal addresses, or VPN installer dependency.
3. Verify tests and Python compilation; deployment and real CDP checks belong to
   the caller. This is cooperative scheduling, not a security sandbox.

Read jobs are atomic tool calls. Named legacy sessions persist until explicit
finish; conversation completion cannot be inferred. Owner VNC still disconnects
CDP and can interrupt any job. Cookies remain shared.

## Four-reader capacity update

1. Add cross-process regression tests before changing admission: four overlapping
   readers, a waiting fifth, writers waiting for every reader (including the last
   two slots), and compatibility with the previous two-slot protocol. Keep the
   existing same-session serialization test.
2. Preserve the two existing outer locks: readers acquire both shared, writers
   acquire both exclusive. Add four exclusive read-capacity locks. Old readers
   drain before new readers enter, preventing a mixed-generation capacity spike.
3. Run the scheduler/shim suite, Ruff, and compilation. Remote activation and real
   four-page browser/resource checks remain separate caller-owned validation.
