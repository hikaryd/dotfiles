# AVF capability probe (not Docker deployment)

Owner-operated diagnostic client for the **installed Pixel build
CP11.251209.007.A1**. Public AVF method signatures and raw networking propagation
were inspected in the device's `framework-virtualization.dex`. Reflection avoids
compiling against unavailable SDK system-API stubs; it does not grant permissions
or change Android's hidden API/debug/SELinux policy. Shell UID 2000 must already
hold the normal custom-VM permissions. API access may still fail at runtime.

## Bounds

- Fixed name `pixel-docker-probe`, 512 MiB, one vCPU, debug NONE.
- No network by default; explicit `--network` is a separate experimental mode,
  **not** a restricted-network policy or authorization for deployment.
- No host shares, input devices, GPU, audio, USB or auto memory balloon.
- Fixed Google image layout under `/data/local/tmp/pixel-avf-probe/payload/`:
  `vmlinuz`, `initrd.img`, `root_part` (writable ROOT partition), `cidata.iso`
  (read-only). The leader must verify the archive, inspect seed/root image for
  credentials, and stage private copies. No user paths or arbitrary commands.
- All base/payload/state directories and payload files must be shell-owned,
  owner-private, nonsymlink paths. State lives under `state/vm/`.
- Existing probe state is refused, never reused or deleted automatically.
- Boot attempts stop at 115 seconds, with process termination at 120 seconds
  if the VM API stalls. `finally` calls `vm.stop`; a shutdown hook also attempts
  stop. **The leader must check `vm list` after every attempt**, including errors
  and timeout. Process termination alone is not proof that VM cleanup succeeded.
- Output capture is best-effort, capped at 256 KiB; it may be empty because the
  device's existing debug policy remains unchanged. Console input stays disabled.

## Build and host checks

```sh
export JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home
sh vpn/pixel-server/avf-probe/build.sh
python3 -m unittest discover -s vpn/pixel-server/avf-probe -v
shellcheck vpn/pixel-server/avf-probe/build.sh
```

Output: `/tmp/pixel-avf-probe-build/dex/classes.dex`. These commands do not deploy
or contact the phone. Tests check source guardrails and execute invalid CLI cases
against the compiled Java class; **they do not prove Android APIs or VM boot**.

## Leader-owned device validation

After separately staging this reviewed dex at the fixed private base path:

```sh
CLASSPATH=/data/local/tmp/pixel-avf-probe/classes.dex:/apex/com.android.virt/javalib/framework-virtualization.jar \
  app_process /system/bin AvfProbe preflight
```

Run via the already authorized Android shell. `preflight` only builds/asserts
configuration: no manager construction, `create`, `run`, or payload file open.
It can run before the image download finishes. `CONFIG_OK` and `PREFLIGHT_ONLY`
are API/config evidence, **not guest boot evidence**.

Only after API preflight, staged-image inspection, fresh memory/thermal gates,
and an independent stop/monitoring plan:

```sh
CLASSPATH=/data/local/tmp/pixel-avf-probe/classes.dex:/apex/com.android.virt/javalib/framework-virtualization.jar \
  app_process /system/bin AvfProbe boot
```

`CREATE_RETURNED`, `RUN_RETURNED`, and `VM_STATUS` show host API progress only.
A real guest kernel/userspace marker is required for boot proof. Docker daemon,
container execution, network isolation, long-running ownership and recovery all
remain separate unimplemented acceptance gates. No boot is started by this repo.

## Authenticated SSH access during the bounded boot

`boot` also opens **127.0.0.1:27682 on Android**, with a maximum of two
connections, and uses the same VM owner's `connectVsock(22022)` API. It does not
use adbd's prohibited arbitrary-CID forwarding, a raw VSOCK bypass, a guest shell
console, or a LAN listener. The staged seed must map that guest VSOCK port to an
SSH daemon requiring the leader's one-off owner public key. Do not use the stock
unauthenticated ttyd service for management and never distribute the owner key
to Hermes.

The TCP client's connect wait and input idle timeout are 10 seconds. If Binder
connect itself stalls, the client is closed at the deadline; its concurrency
slot remains occupied until Binder returns, and a late descriptor is closed.
All relays/listeners close before VM stop; the process-wide 120-second deadline
remains. A long command with no client traffic may need normal SSH keepalives.

The leader can forward a temporary **Mac loopback** port to Android's TCP port
27682 using authenticated ADB, then use SSH with strict host-key pinning and the
one-off key. No guest network is needed for this path.

Additional host test (real loopback TCP with a stalled fake Binder, not AVF):

```sh
"$JAVA_HOME/bin/javac" -Xlint:all -Werror \
  -cp "/tmp/pixel-avf-probe-build/classes:$HOME/Library/Android/sdk/platforms/android-35/android.jar" \
  -d /tmp/pixel-avf-probe-build/tests vpn/pixel-server/avf-probe/tests/RelayHostTest.java
"$JAVA_HOME/bin/java" \
  -cp "/tmp/pixel-avf-probe-build/tests:/tmp/pixel-avf-probe-build/classes:$HOME/Library/Android/sdk/platforms/android-35/android.jar" \
  RelayHostTest
```
