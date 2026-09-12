import android.content.Context;
import android.content.ContextWrapper;
import android.os.SystemClock;
import android.os.Looper;
import android.system.Os;
import android.system.OsConstants;
import android.system.StructStat;
import java.io.File;
import java.io.InputStream;
import java.lang.reflect.Array;
import java.lang.reflect.InvocationTargetException;
import java.util.Objects;

/** Owner-only, bounded AVF capability probe; not a VM service or Docker deployment. */
public final class AvfProbe {
    static final String BASE = "/data/local/tmp/pixel-avf-probe";
    static final String PAYLOAD = BASE + "/payload/";
    static final String NAME = "pixel-docker-probe";
    static final String API = "android.system.virtualmachine.";
    static final long MEMORY = 512L * 1024 * 1024;
    static final long STOP_AFTER_MS = 115_000;
    static final long DEADLINE_MS = 120_000;
    private static volatile Object vm;
    private static volatile SshRelay relay;

    private static Object call(Object target, String name, Class<?>[] types, Object... args)
            throws Exception {
        try {
            return target.getClass().getMethod(name, types).invoke(target, args);
        } catch (InvocationTargetException e) {
            Throwable cause = e.getCause();
            if (cause instanceof Exception) throw (Exception) cause;
            throw e;
        }
    }

    private static Object get(Object target, String name) throws Exception {
        return call(target, name, new Class<?>[0]);
    }

    private static void set(Object target, String name, Class<?> type, Object value)
            throws Exception {
        call(target, name, new Class<?>[]{type}, value);
    }

    private static void require(boolean condition, String message) {
        if (!condition) throw new IllegalStateException(message);
    }

    @SuppressWarnings("deprecation")
    private static Context context() throws Exception {
        if (Looper.myLooper() == null) Looper.prepareMainLooper();
        Class<?> activityThread = Class.forName("android.app.ActivityThread");
        Object thread = activityThread.getMethod("systemMain").invoke(null);
        Context system = (Context) activityThread.getMethod("getSystemContext").invoke(thread);
        Context shell = system.createPackageContext("com.android.shell", 0);
        return new ContextWrapper(shell) {
            @Override public File getDataDir() { return new File(BASE + "/state"); }
            @Override public String getPackageName() { return "com.android.shell"; }
        };
    }

    private static Object config(Context context, boolean network) throws Exception {
        Class<?> customClass = Class.forName(API + "VirtualMachineCustomImageConfig");
        Class<?> diskClass = Class.forName(API + "VirtualMachineCustomImageConfig$Disk");
        Class<?> partitionClass = Class.forName(API + "VirtualMachineCustomImageConfig$Partition");
        Object custom = Class.forName(API + "VirtualMachineCustomImageConfig$Builder")
                .getConstructor().newInstance();
        set(custom, "setName", String.class, NAME);
        set(custom, "setKernelPath", String.class, PAYLOAD + "vmlinuz");
        set(custom, "setInitrdPath", String.class, PAYLOAD + "initrd.img");
        set(custom, "useNetwork", boolean.class, network);
        set(custom, "useAutoMemoryBalloon", boolean.class, false);
        for (String method : new String[]{"useKeyboard", "useMouse", "useTouch",
                "useTrackpad", "useSwitches"}) set(custom, method, boolean.class, false);
        Object rootDisk = diskClass.getMethod("RWDisk", String.class).invoke(null, (Object) null);
        Object root = partitionClass.getConstructor(String.class, String.class,
                boolean.class, String.class).newInstance("ROOT", PAYLOAD + "root_part", true,
                "50A10C2A-2FCE-4E69-8CFE-5FE637B873D6");
        set(rootDisk, "addPartition", partitionClass, root);
        set(custom, "addDisk", diskClass, rootDisk);
        Object seed = diskClass.getMethod("RODisk", String.class)
                .invoke(null, PAYLOAD + "cidata.iso");
        set(custom, "addDisk", diskClass, seed);
        for (String param : new String[]{"root=/dev/vda1", "ds=nocloud", "arm64.nompam",
                "8250.nr_uarts=4", "console=ttyS0"}) set(custom, "addParam", String.class, param);
        Object image = get(custom, "build");
        require(Array.getLength(get(image, "getSharedPaths")) == 0, "Unexpected host shares");
        require(Array.getLength(get(image, "getDisks")) == 2, "Unexpected disks");
        require(Objects.equals(get(image, "useNetwork"), network), "Network mismatch");
        require(Boolean.FALSE.equals(get(image, "useAutoMemoryBalloon")), "Balloon enabled");
        for (String name : new String[]{"getDisplayConfig", "getGpuConfig", "getAudioConfig",
                "getUsbConfig", "getBootloaderPath"}) require(get(image, name) == null, name);
        for (String name : new String[]{"useKeyboard", "useMouse", "useTouch", "useTrackpad",
                "useSwitches"}) require(Boolean.FALSE.equals(get(image, name)), name);
        Class<?> configClass = Class.forName(API + "VirtualMachineConfig");
        Object builder = Class.forName(API + "VirtualMachineConfig$Builder")
                .getConstructor(Context.class).newInstance(context);
        int oneCpu = configClass.getField("CPU_TOPOLOGY_ONE_CPU").getInt(null);
        int noDebug = configClass.getField("DEBUG_LEVEL_NONE").getInt(null);
        set(builder, "setCustomImageConfig", customClass, image);
        set(builder, "setMemoryBytes", long.class, MEMORY);
        set(builder, "setCpuTopology", int.class, oneCpu);
        set(builder, "setProtectedVm", boolean.class, false);
        set(builder, "setDebugLevel", int.class, noDebug);
        // Capture is best-effort under the existing debug policy; never enable console input.
        set(builder, "setVmOutputCaptured", boolean.class, true);
        set(builder, "setVmConsoleInputSupported", boolean.class, false);
        set(builder, "setConnectVmConsole", boolean.class, false);
        set(builder, "setShouldBoostUclamp", boolean.class, false);
        set(builder, "setShouldUseHugepages", boolean.class, false);
        Object result = get(builder, "build");
        require(Objects.equals(get(result, "getMemoryBytes"), MEMORY), "Memory mismatch");
        require(Objects.equals(get(result, "getCpuTopology"), oneCpu), "CPU mismatch");
        require(Objects.equals(get(result, "getDebugLevel"), noDebug), "Debug enabled");
        require(Boolean.FALSE.equals(get(result, "isProtectedVm")), "Unexpected protected VM");
        require(Boolean.FALSE.equals(get(result, "isVmConsoleInputSupported")), "Console input enabled");
        require(Boolean.FALSE.equals(get(result, "isConnectVmConsole")), "Console connected");
        return result;
    }

    private static void validatePath(String path, boolean directory) throws Exception {
        File file = new File(path);
        require(file.getCanonicalPath().equals(path), "Symlink/noncanonical path: " + path);
        StructStat stat = Os.lstat(path);
        require(stat.st_uid == 2000, "Not shell-owned: " + path);
        require((stat.st_mode & 0077) == 0, "Not owner-private: " + path);
        require(directory ? OsConstants.S_ISDIR(stat.st_mode) : OsConstants.S_ISREG(stat.st_mode),
                "Wrong file type: " + path);
        if (!directory) require(stat.st_size > 0, "Empty payload: " + path);
    }

    private static boolean stop() {
        SshRelay currentRelay = relay;
        if (currentRelay != null) currentRelay.close();
        Object current = vm;
        if (current == null) return true;
        try {
            get(current, "stop");
            Object status = get(current, "getStatus");
            System.out.println("STOP_RETURNED status=" + status);
            return Objects.equals(status, current.getClass().getField("STATUS_STOPPED").getInt(null));
        } catch (Exception e) {
            System.err.println("STOP_RESULT " + e);
            return false;
        }
    }

    private static void boot(Context context, Object config) throws Exception {
        validatePath(BASE, true);
        validatePath(BASE + "/state", true);
        validatePath(BASE + "/payload", true);
        for (String file : new String[]{"vmlinuz", "initrd.img", "root_part", "cidata.iso"}) {
            validatePath(PAYLOAD + file, false);
        }
        require(!new File(BASE + "/state/vm/" + NAME).exists(),
                "Probe state already exists; refuse reuse or deletion");
        final long started = SystemClock.elapsedRealtime();
        Thread watchdog = new Thread(() -> {
            SystemClock.sleep(STOP_AFTER_MS);
            Thread stopper = new Thread(AvfProbe::stop, "avf-deadline-stop");
            stopper.setDaemon(true);
            stopper.start();
            SystemClock.sleep(Math.max(0, started + DEADLINE_MS - SystemClock.elapsedRealtime()));
            System.err.println("HARD_DEADLINE: terminating probe; verify vm list is empty");
            Runtime.getRuntime().halt(124);
        }, "avf-deadline");
        watchdog.setDaemon(true);
        watchdog.start();
        Runtime.getRuntime().addShutdownHook(new Thread(AvfProbe::stop, "avf-shutdown-stop"));
        try {
            Object manager = Class.forName(API + "VirtualMachineManager")
                    .getConstructor(Context.class).newInstance(context);
            vm = call(manager, "create", new Class<?>[]{String.class,
                    Class.forName(API + "VirtualMachineConfig")}, NAME, config);
            System.out.println("CREATE_RETURNED");
            get(vm, "run");
            System.out.println("RUN_RETURNED: this is not guest boot or Docker proof");
            relay = new SshRelay(vm);
            Thread console = new Thread(() -> {
                try (InputStream stream = (InputStream) get(vm, "getConsoleOutput")) {
                    byte[] buffer = new byte[4096];
                    int remaining = 256 * 1024;
                    while (remaining > 0) {
                        int n = stream.read(buffer, 0, Math.min(buffer.length, remaining));
                        if (n < 0) break;
                        System.out.write(buffer, 0, n);
                        remaining -= n;
                    }
                } catch (Exception e) { System.err.println("CONSOLE_UNAVAILABLE " + e); }
            }, "avf-console");
            console.setDaemon(true);
            console.start();
            while (SystemClock.elapsedRealtime() - started < STOP_AFTER_MS) {
                System.out.println("VM_STATUS " + get(vm, "getStatus"));
                SystemClock.sleep(5000);
            }
        } finally {
            require(stop(), "STOP_UNVERIFIED: inspect vm list before continuing");
        }
    }

    public static void main(String[] args) throws Exception {
        Thread.setDefaultUncaughtExceptionHandler((thread, error) -> {
            System.err.println("PROBE_FAILED thread=" + thread.getName());
            error.printStackTrace(System.err);
            System.err.flush();
            System.exit(1);
        });
        require(args.length == 1 || args.length == 2,
                "Usage: AvfProbe preflight|boot [--network]");
        require(args[0].equals("preflight") || args[0].equals("boot"), "Invalid mode");
        boolean network = args.length == 2;
        require(!network || args[1].equals("--network"), "Invalid option");
        require(android.os.Process.myUid() == 2000, "Must run as authorized Android shell UID");
        Context context = context();
        Object config = config(context, network);
        System.out.println("CONFIG_OK memoryMiB=512 cpu=1 network=" + network
                + " shares=0 balloon=false debug=NONE consoleInput=false");
        if (args[0].equals("preflight")) {
            System.out.println("PREFLIGHT_ONLY: no manager, VM creation, payload open or boot");
            System.exit(0);
        }
        boot(context, config);
        System.exit(0);
    }
}
