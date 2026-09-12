import java.net.ConnectException;
import java.net.Socket;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/** Actual host TCP guardrails with a deliberately stalled fake AVF Binder. */
public final class RelayHostTest {
    public static final class StalledVm {
        final CountDownLatch entered = new CountDownLatch(2);
        final CountDownLatch release = new CountDownLatch(1);
        public Object connectVsock(long port) throws Exception {
            if (port != 22022L) throw new AssertionError("Unexpected guest port");
            entered.countDown();
            release.await();
            throw new IllegalStateException("Expected fake Binder disconnect");
        }
    }
    @SuppressWarnings("try") // Deliberately verify explicit close before scope exit.
    public static void main(String[] args) throws Exception {
        StalledVm vm = new StalledVm();
        try (SshRelay relay = new SshRelay(vm);
             Socket first = new Socket("127.0.0.1", 27682);
             Socket second = new Socket("127.0.0.1", 27682)) {
            first.setSoTimeout(12_000);
            second.setSoTimeout(12_000);
            if (!vm.entered.await(2, TimeUnit.SECONDS)) throw new AssertionError("No Binder calls");
            try (Socket third = new Socket("127.0.0.1", 27682)) {
                third.setSoTimeout(2000);
                if (third.getInputStream().read() != -1) throw new AssertionError("Third slot accepted");
            }
            if (first.getInputStream().read() != -1) throw new AssertionError("Connect timeout missing");
            if (second.getInputStream().read() != -1) throw new AssertionError("Connect timeout missing");
            relay.close();
            try (Socket unexpected = new Socket("127.0.0.1", 27682)) {
                throw new AssertionError("Listener survived close: " + unexpected);
            } catch (ConnectException expected) { }
        } finally { vm.release.countDown(); }
        System.out.println("PASS: two-slot cap, real 10-second connect deadline, close removes listener");
    }
}
