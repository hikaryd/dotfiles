import Foundation

struct SentenceCheckResponse: Decodable, Sendable {
    let original: String
    let corrected: String
    let changed: Bool
    let latency_ms: Double
}

struct ServerHealthResponse: Decodable, Sendable {
    let status: String
    let model: String?
    let device: String?
    let error: String?
}

enum ServerState: Equatable, Sendable {
    case starting
    case ready(model: String)
    case offline(String)
}

@MainActor
final class SentenceClient {
    let port: Int
    private let session: URLSession
    private var serverProcess: Process?
    /// Keeping the write end of stdin open notifies the Python watchdog via EOF if this process crashes/SIGKILLs.
    private var stdinPipe: Pipe?
    private var inFlightTask: Task<Void, Never>?
    private var startupTask: Task<Void, Never>?
    private var stateHandler: (@MainActor (ServerState) -> Void)?
    private var autoRecoverEnabled: Bool = true

    init(port: Int) {
        self.port = port
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 4.0
        cfg.timeoutIntervalForResource = 5.0
        self.session = URLSession(configuration: cfg)
    }

    private var baseURL: URL {
        URL(string: "http://127.0.0.1:\(port)")!
    }

    func checkHealth() async -> ServerHealthResponse? {
        let url = baseURL.appendingPathComponent("health")
        var req = URLRequest(url: url)
        req.timeoutInterval = 1.5
        do {
            let (data, resp) = try await session.data(for: req)
            guard (resp as? HTTPURLResponse)?.statusCode == 200 else { return nil }
            return try JSONDecoder().decode(ServerHealthResponse.self, from: data)
        } catch {
            return nil
        }
    }

    func ensureServerRunning(onStateChange: @escaping @MainActor (ServerState) -> Void) {
        self.stateHandler = onStateChange
        startupTask?.cancel()
        startupTask = Task { @MainActor in
            var adoptedExternalServer = false

            if let health = await checkHealth() {
                if Task.isCancelled { return }
                if health.status == "ok" {
                    onStateChange(.ready(model: health.model ?? "local"))
                    return
                }
                if health.status == "error" {
                    stopSubprocess()
                    onStateChange(.offline(health.error ?? "Ошибка загрузки модели"))
                    return
                }
                if health.status == "loading" {
                    // Existing server is already loading on this port -> wait for it instead of spawning a duplicate.
                    adoptedExternalServer = true
                }
            }

            if Task.isCancelled { return }
            onStateChange(.starting)

            if !adoptedExternalServer {
                // Clear stale reference if prior subprocess already exited.
                if let proc = serverProcess, !proc.isRunning {
                    serverProcess = nil
                    stdinPipe = nil
                }
                if serverProcess == nil {
                    startSubprocess()
                }
                guard serverProcess != nil else {
                    onStateChange(.offline("Не найден server/server.py или .venv"))
                    return
                }
            }

            // Poll health for up to 90s while weights load into memory on first boot.
            var externalMissStreak = 0
            for _ in 0..<180 {
                if Task.isCancelled { return }
                let health = await checkHealth()
                if Task.isCancelled { return }
                if let health {
                    externalMissStreak = 0
                    if health.status == "ok" {
                        onStateChange(.ready(model: health.model ?? "local"))
                        return
                    }
                    if health.status == "error" {
                        stopSubprocess()
                        onStateChange(.offline(health.error ?? "Ошибка загрузки модели"))
                        return
                    }
                } else if adoptedExternalServer {
                    externalMissStreak += 1
                    if externalMissStreak >= 2 {
                        adoptedExternalServer = false
                        if serverProcess == nil {
                            startSubprocess()
                        }
                        guard serverProcess != nil else {
                            onStateChange(.offline("Не найден server/server.py или .venv"))
                            return
                        }
                    }
                }
                if let proc = serverProcess, !proc.isRunning {
                    let code = proc.terminationStatus
                    serverProcess = nil
                    stdinPipe = nil
                    onStateChange(.offline("Сервер завершился (код \(code))"))
                    return
                }
                try? await Task.sleep(for: .milliseconds(500))
            }
            if Task.isCancelled { return }
            stopSubprocess()
            onStateChange(.offline("Таймаут запуска сервера"))
        }
    }

    func setAutoRecoverEnabled(_ enabled: Bool) {
        autoRecoverEnabled = enabled
        if !enabled {
            startupTask?.cancel()
            startupTask = nil
            cancelPending()
        }
    }

    func checkSentence(_ text: String, onResult: @escaping @MainActor (SentenceCheckResponse?) -> Void) {
        inFlightTask?.cancel()
        let url = baseURL.appendingPathComponent("check")
        let session = self.session

        inFlightTask = Task { @MainActor [weak self] in
            var req = URLRequest(url: url)
            req.httpMethod = "POST"
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try? JSONSerialization.data(withJSONObject: ["text": text])

            var decoded: SentenceCheckResponse?
            var connectionLost = false
            do {
                let (data, resp) = try await session.data(for: req)
                guard !Task.isCancelled else { return }
                if (resp as? HTTPURLResponse)?.statusCode == 200 {
                    decoded = try? JSONDecoder().decode(SentenceCheckResponse.self, from: data)
                }
            } catch let urlErr as URLError where urlErr.code == .cannotConnectToHost || urlErr.code == .networkConnectionLost {
                connectionLost = true
            } catch {
                // CancellationError or timeout: silently discard.
            }

            guard let self, !Task.isCancelled else { return }
            if connectionLost, self.autoRecoverEnabled, let handler = self.stateHandler {
                self.ensureServerRunning(onStateChange: handler)
            }
            onResult(decoded)
        }
    }

    func cancelPending() {
        inFlightTask?.cancel()
        inFlightTask = nil
    }

    func stopSubprocess() {
        startupTask?.cancel()
        startupTask = nil
        if let pipe = stdinPipe {
            try? pipe.fileHandleForWriting.close()
            stdinPipe = nil
        }
        guard let proc = serverProcess else { return }
        serverProcess = nil
        if proc.isRunning {
            let pid = proc.processIdentifier
            proc.terminate()
            let deadline = Date().addingTimeInterval(2.0)
            while proc.isRunning && Date() < deadline {
                usleep(50_000)
            }
            if proc.isRunning {
                kill(pid, SIGKILL)
            }
        }
    }

    private func startSubprocess() {
        guard let root = locateProjectRoot() else { return }
        let pythonURL = root.appendingPathComponent("server/.venv/bin/python")
        let scriptURL = root.appendingPathComponent("server/server.py")
        guard FileManager.default.isExecutableFile(atPath: pythonURL.path),
              FileManager.default.fileExists(atPath: scriptURL.path)
        else { return }

        let logDir = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Logs/SpellcheckMac", isDirectory: true)
        try? FileManager.default.createDirectory(at: logDir, withIntermediateDirectories: true)
        let logURL = logDir.appendingPathComponent("server.log")
        if !FileManager.default.fileExists(atPath: logURL.path) {
            FileManager.default.createFile(atPath: logURL.path, contents: nil)
        }
        let logHandle = try? FileHandle(forWritingTo: logURL)
        _ = try? logHandle?.seekToEnd()

        let pipe = Pipe()
        let proc = Process()
        proc.executableURL = pythonURL
        proc.arguments = [
            scriptURL.path,
            "--port", "\(port)",
            "--parent-pid", "\(ProcessInfo.processInfo.processIdentifier)",
            "--watch-stdin",
        ]
        proc.currentDirectoryURL = root.appendingPathComponent("server")
        var env = ProcessInfo.processInfo.environment
        env["PYTHONUNBUFFERED"] = "1"
        proc.environment = env
        proc.standardInput = pipe
        if let logHandle {
            proc.standardOutput = logHandle
            proc.standardError = logHandle
        }

        do {
            try proc.run()
            self.stdinPipe = pipe
            self.serverProcess = proc
        } catch {
            try? pipe.fileHandleForWriting.close()
            self.stdinPipe = nil
            self.serverProcess = nil
        }
    }

    private func locateProjectRoot() -> URL? {
        if let envRoot = ProcessInfo.processInfo.environment["SPELLCHECK_ROOT"], !envRoot.isEmpty {
            return URL(fileURLWithPath: envRoot)
        }
        // Check bundled project_root.txt written by install.sh when installed in /Applications.
        if let resURL = Bundle.main.url(forResource: "project_root", withExtension: "txt"),
           let raw = try? String(contentsOf: resURL, encoding: .utf8)
        {
            let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.isEmpty {
                let candidate = URL(fileURLWithPath: trimmed)
                if FileManager.default.fileExists(atPath: candidate.appendingPathComponent("server/server.py").path) {
                    return candidate
                }
            }
        }
        var cur = URL(fileURLWithPath: CommandLine.arguments[0]).resolvingSymlinksInPath().deletingLastPathComponent()
        for _ in 0..<8 {
            let candidate = cur.appendingPathComponent("server/server.py")
            if FileManager.default.fileExists(atPath: candidate.path) {
                return cur
            }
            cur.deleteLastPathComponent()
        }
        let fallback = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("dots/tools/spellcheck")
        if FileManager.default.fileExists(atPath: fallback.appendingPathComponent("server/server.py").path) {
            return fallback
        }
        return nil
    }
}
