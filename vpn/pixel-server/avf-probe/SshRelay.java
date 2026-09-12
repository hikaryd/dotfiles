import android.os.ParcelFileDescriptor;
import java.io.Closeable;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.util.HashSet;
import java.util.Set;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;

/** Fixed loopback relay to authenticated guest SSH; no port/address/command input. */
final class SshRelay implements Closeable {
    static final int LOCAL_PORT = 27682;
    static final long GUEST_PORT = 22022L;
    static final int TIMEOUT_MS = 10_000;
    private final Object vm;
    private final ServerSocket server = new ServerSocket();
    private final Semaphore slots = new Semaphore(2);
    private final Set<Connection> connections = new HashSet<>();
    private final ScheduledExecutorService timers = Executors.newSingleThreadScheduledExecutor(r -> {
        Thread thread = new Thread(r, "avf-ssh-timeout");
        thread.setDaemon(true);
        return thread;
    });
    private boolean closed;

    SshRelay(Object vm) throws IOException {
        this.vm = vm;
        try {
            server.bind(new InetSocketAddress(InetAddress.getByName("127.0.0.1"), LOCAL_PORT), 2);
        } catch (IOException e) {
            server.close();
            timers.shutdownNow();
            throw e;
        }
        Thread accept = new Thread(this::accept, "avf-ssh-accept");
        accept.setDaemon(true);
        accept.start();
        System.out.println("SSH_RELAY 127.0.0.1:" + LOCAL_PORT + " -> guest-vsock:" + GUEST_PORT);
    }

    private void accept() {
        try {
            while (!server.isClosed()) {
                Socket socket = server.accept();
                if (!slots.tryAcquire()) {
                    socket.close();
                    continue;
                }
                Connection connection = new Connection(socket);
                synchronized (this) {
                    if (closed) {
                        socket.close();
                        slots.release();
                        return;
                    }
                    connections.add(connection);
                    Thread worker = new Thread(connection::run, "avf-ssh-connection");
                    worker.setDaemon(true);
                    worker.start();
                }
            }
        } catch (IOException e) {
            if (!server.isClosed()) System.err.println("SSH_RELAY_ACCEPT " + e);
        }
    }

    private static void copy(InputStream source, OutputStream destination) throws IOException {
        byte[] buffer = new byte[16 * 1024];
        int length;
        while ((length = source.read(buffer)) != -1) {
            destination.write(buffer, 0, length);
            destination.flush();
        }
    }

    private static void quietlyClose(Closeable resource) {
        if (resource == null) return;
        try { resource.close(); } catch (IOException ignored) { }
    }

    @Override public synchronized void close() {
        if (closed) return;
        closed = true;
        quietlyClose(server);
        for (Connection connection : connections) connection.close();
        timers.shutdownNow();
    }

    private final class Connection implements Closeable {
        private final Socket socket;
        private ParcelFileDescriptor guest;
        private boolean connected;
        private boolean ended;

        Connection(Socket socket) { this.socket = socket; }

        private synchronized void expireConnect() {
            if (!connected) close();
        }

        @Override public synchronized void close() {
            ended = true;
            quietlyClose(socket);
            quietlyClose(guest);
        }

        void run() {
            try {
                socket.setSoTimeout(TIMEOUT_MS);
                // Binder connect has no timeout parameter: close the client at 10 seconds.
                // Keep its concurrency slot until Binder returns; late FDs are closed below.
                timers.schedule(this::expireConnect, TIMEOUT_MS, TimeUnit.MILLISECONDS);
                ParcelFileDescriptor fd = (ParcelFileDescriptor) vm.getClass()
                        .getMethod("connectVsock", long.class).invoke(vm, GUEST_PORT);
                synchronized (this) {
                    if (ended) {
                        fd.close();
                        return;
                    }
                    guest = fd;
                    connected = true;
                }
                // Each stream owns a duplicated descriptor. Closing guest + streams at the
                // end closes the whole connection without double ownership of one raw FD.
                try (InputStream guestInput = new ParcelFileDescriptor.AutoCloseInputStream(
                             ParcelFileDescriptor.dup(fd.getFileDescriptor()));
                     OutputStream guestOutput = new ParcelFileDescriptor.AutoCloseOutputStream(
                             ParcelFileDescriptor.dup(fd.getFileDescriptor()))) {
                    Thread upload = new Thread(() -> {
                        try { copy(socket.getInputStream(), guestOutput); }
                        catch (IOException e) { /* Disconnect/idle timeout: close both ends. */ }
                        finally { close(); quietlyClose(guestInput); quietlyClose(guestOutput); }
                    }, "avf-ssh-upload");
                    upload.setDaemon(true);
                    upload.start();
                    copy(guestInput, socket.getOutputStream());
                }
            } catch (Exception e) {
                System.err.println("SSH_RELAY_CONNECTION " + e);
            } finally {
                close();
                synchronized (SshRelay.this) { connections.remove(this); }
                slots.release();
            }
        }
    }
}
