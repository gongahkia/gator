import Foundation

public struct BridgeLaunchConfiguration: Sendable {
    public var runtime: URL
    public var workspace: URL
    public var environment: [String: String]
    public var conversationID: String
    var nativeUploadDirectory: URL {
        let state = environment["GATOR_STATE_DIR"] ?? environment["XDG_STATE_HOME"] ?? ((environment["HOME"] ?? NSHomeDirectory()) + "/.local/state")
        return URL(fileURLWithPath: state, relativeTo: workspace).standardizedFileURL.appendingPathComponent("gator/native-uploads", isDirectory: true)
    }
    var managed = false
    var bootMode = ""
    var startSource = ""
    var viewOrigin = ""
    var goalID = ""
    public init(runtime: URL, workspace: URL, environment: [String: String] = ProcessInfo.processInfo.environment, conversationID: String = "") {
        self.runtime = runtime; self.workspace = workspace; self.environment = environment; self.conversationID = conversationID
    }
}

protocol BridgeTransport: Sendable {
    var events: AsyncStream<BridgeEvent> { get }
    func start(_ configuration: BridgeLaunchConfiguration) async throws
    func send(_ command: BridgeCommand, nonce: String) async throws
    func shutdown() async
    func shutdownConfirmed() async -> Bool
}
extension BridgeTransport {
    func shutdownConfirmed() async -> Bool { await shutdown(); return true }
}

actor GoBridgeConnection: BridgeTransport {
    nonisolated let events: AsyncStream<BridgeEvent>
    private let continuation: AsyncStream<BridgeEvent>.Continuation
    private var process: Process?
    private var input: FileHandle?
    private var outputTask: Task<Void, Never>?
    private var errorTask: Task<Void, Never>?
    private var cleanupTask: Task<Void, Never>?
    private var shutdownTask: Task<Bool, Never>?
    private var readersFinished = 0
    private let readCancellation = BridgeIOCancellation()
    private let writeCancellation = BridgeIOCancellation()
    private var acknowledgementTasks: [String: Task<Void, Never>] = [:]
    private var framer = BridgeFramer()
    private var diagnostics = Data()
    private var waiters: [String: CheckedContinuation<Void, any Error>] = [:]
    private var closed = false
    private var deliberate = false
    private let writeQueue = DispatchQueue(label: "gator.native.bridge.write")
    private let acknowledgementTimeout: Duration

    init(acknowledgementTimeout: Duration = .seconds(15)) {
        // Snapshots replace state. Bound presentation backpressure while keeping
        // transport acknowledgement waiters independent of the view consumer.
        let channel = AsyncStream<BridgeEvent>.makeStream(bufferingPolicy: .bufferingNewest(32))
        events = channel.stream; continuation = channel.continuation
        self.acknowledgementTimeout = acknowledgementTimeout
    }

    func start(_ configuration: BridgeLaunchConfiguration) async throws {
        signal(SIGPIPE, SIG_IGN)
        guard process == nil, !closed else { throw BridgeFailure("This connection has already been started.") }
        guard FileManager.default.isExecutableFile(atPath: configuration.runtime.path) else { throw BridgeFailure("The bundled Go runtime is missing or not executable.") }
        try FileManager.default.createDirectory(at: configuration.workspace, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let p = Process(), stdin = Pipe(), stdout = Pipe(), stderr = Pipe()
        p.executableURL = configuration.runtime
        p.arguments = ["ui-backend", configuration.conversationID, configuration.goalID]
        p.currentDirectoryURL = configuration.workspace
        var environment = configuration.environment
        for key in ["GATOR_UI_SESSION_HOST", "GATOR_UI_BOOT_MODE", "GATOR_UI_VIEW_ORIGIN", "GATOR_UI_START_SOURCE"] { environment.removeValue(forKey: key) }
        if configuration.managed {
            environment["GATOR_UI_SESSION_HOST"] = "1"
            environment["GATOR_UI_BOOT_MODE"] = configuration.bootMode
            environment["GATOR_UI_START_SOURCE"] = configuration.startSource
            environment["GATOR_UI_VIEW_ORIGIN"] = configuration.viewOrigin
        }
        environment["GATOR_UI_BACKEND"] = "1"
        environment["GATOR_DESKTOP_UPLOAD_DIR"] = configuration.nativeUploadDirectory.path
        p.environment = environment; p.standardInput = stdin; p.standardOutput = stdout; p.standardError = stderr
        p.terminationHandler = { [weak self] _ in
            Task { await self?.finishIO() }
        }
        process = p; input = stdin.fileHandleForWriting
        do { try p.run() }
        catch { process = nil; input = nil; throw error }
        let reader = stdout.fileHandleForReading, errors = stderr.fileHandleForReading
        do {
            try BridgePipeIO.prepare(reader); try BridgePipeIO.prepare(errors)
            try BridgePipeIO.prepare(stdin.fileHandleForWriting)
        } catch {
            try? reader.close(); try? errors.close()
            fail(error.localizedDescription)
            throw error
        }
        let readCancellation = readCancellation
        outputTask = Task.detached { [weak self] in
            do {
                while !Task.isCancelled {
                    guard let bytes = try await BridgePipeIO.read(reader, cancellation: readCancellation) else { continue }
                    if bytes.isEmpty { break }
                    guard let self else { break }; await self.receive(bytes)
                }
                await self?.outputEnded()
            } catch is CancellationError {
                // direct-owner exit bounds draining even if a descendant holds the pipe.
            } catch { await self?.fail("Reading Gator failed: " + error.localizedDescription) }
            try? reader.close()
            await self?.readerFinished()
        }
        errorTask = Task.detached { [weak self] in
            do {
                while !Task.isCancelled {
                    guard let bytes = try await BridgePipeIO.read(errors, cancellation: readCancellation) else { continue }
                    if bytes.isEmpty { break }
                    await self?.receiveDiagnostics(bytes)
                }
            } catch { /* diagnostics are best effort; owner exit still controls cleanup. */ }
            try? errors.close()
            await self?.readerFinished()
        }
    }
    private func readerFinished() { readersFinished += 1 }
    private func outputEnded() { _ = beginIOCleanup() }
    func cleanupIsComplete() -> Bool {
        outputTask == nil && errorTask == nil && input == nil && waiters.isEmpty && acknowledgementTasks.isEmpty
    }
    func diagnosticByteCount() -> Int { diagnostics.count }

    private func receive(_ bytes: Data) {
        guard !closed else { return }
        do {
            for event in try framer.append(bytes) {
                if case .acknowledged(let nonce) = event {
                    acknowledgementTasks.removeValue(forKey: nonce)?.cancel()
                    waiters.removeValue(forKey: nonce)?.resume()
                }
                continuation.yield(event)
            }
        } catch { fail(error.localizedDescription) }
    }
    private func receiveDiagnostics(_ bytes: Data) {
        diagnostics.append(bytes); if diagnostics.count > 4096 { diagnostics = Data(diagnostics.suffix(4096)) }
    }
    private func endOfOutput() {
        guard !closed else { return }
        do { try framer.finish() }
        catch { fail(error.localizedDescription); return }
        let detail = String(decoding: diagnostics, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        fail(deliberate ? "Gator closed." : (detail.isEmpty ? "Gator disconnected. Reconnect to inspect retained work; nothing will be resent." : detail))
    }
    private func fail(_ reason: String) {
        guard !closed else { return }; closed = true
        for waiter in waiters.values { waiter.resume(throwing: BridgeFailure(reason)) }; waiters.removeAll()
        continuation.yield(.disconnected(reason)); continuation.finish()
        for task in acknowledgementTasks.values { task.cancel() }; acknowledgementTasks.removeAll()
        writeCancellation.cancel()
        if let input { writeQueue.async { try? input.close() } }
        if let process, process.isRunning { process.terminate() }
    }

    func send(_ command: BridgeCommand, nonce: String) async throws {
        guard shutdownTask == nil, !closed, let input, process?.isRunning == true else { throw BridgeFailure("Gator is disconnected.") }
        let bytes = try command.data(nonce: nonce)
        // focus_session deliberately publishes state without action_applied.
        // Its caller must await the exact foreground projection, not a nonce.
        if case .host(.focus) = command {
            writeQueue.async { [weak self, writeCancellation] in
                do { try BridgePipeIO.write(bytes, to: input, cancellation: writeCancellation) }
                catch { Task { await self?.fail("Gator did not confirm focus. Original work was not moved.") } }
            }
            return
        }
        guard waiters[nonce] == nil else { throw BridgeFailure("Duplicate transport identity.") }
        try await withCheckedThrowingContinuation { waiter in
            waiters[nonce] = waiter
            writeQueue.async { [weak self, writeCancellation] in
                do { try BridgePipeIO.write(bytes, to: input, cancellation: writeCancellation) }
                catch { Task { await self?.fail("Writing to Gator failed. Admission is unconfirmed.") } }
            }
            acknowledgementTasks[nonce] = Task { [weak self, acknowledgementTimeout] in
                do { try await Task.sleep(for: acknowledgementTimeout) } catch { return }
                await self?.expire(nonce)
            }
        }
    }
    private func expire(_ nonce: String) {
        acknowledgementTasks.removeValue(forKey: nonce)?.cancel()
        waiters.removeValue(forKey: nonce)?.resume(throwing: BridgeFailure("Gator did not acknowledge this action. Inspect its retained state before retrying."))
    }

    func processIdentifier() -> Int32? { process?.processIdentifier }

    func shutdown() async { _ = await shutdownConfirmed() }
    func shutdownConfirmed() async -> Bool {
        if let shutdownTask { return await shutdownTask.value }
        let task = Task { await performShutdown() }
        shutdownTask = task
        let confirmed = await task.value
        if !confirmed { shutdownTask = nil }
        return confirmed
    }
    private func performShutdown() async -> Bool {
        deliberate = true
        if let input, !closed, let data = try? BridgeCommand.exit.data(nonce: "") {
            writeQueue.async { [writeCancellation] in
                try? BridgePipeIO.write(data, to: input, cancellation: writeCancellation)
                try? input.close()
            }
        }
        // keep the existing graceful / SIGTERM / SIGKILL escalation windows.
        for _ in 0..<25 {
            if process?.isRunning != true { break }
            try? await Task.sleep(for: .milliseconds(100))
        }
        if process?.isRunning == true { process?.terminate() }
        for _ in 0..<25 {
            if process?.isRunning != true { break }
            try? await Task.sleep(for: .milliseconds(100))
        }
        if let process, process.isRunning { kill(process.processIdentifier, SIGKILL) }
        // Foundation reaps Process asynchronously; never call waitUntilExit here.
        for _ in 0..<100 {
            if process?.isRunning != true { break }
            try? await Task.sleep(for: .milliseconds(20))
        }
        let confirmed = process?.isRunning != true
        if !confirmed { fail("Gator shutdown could not be confirmed. Inspect the original owner before restarting.") }
        await finishIO()
        if confirmed {
            process?.terminationHandler = nil; process = nil
        }
        return confirmed
    }
    private func finishIO() async { await beginIOCleanup().value }
    private func beginIOCleanup() -> Task<Void, Never> {
        if let cleanupTask { return cleanupTask }
        let task = Task {
            // drain available final frames/diagnostics, independently of inherited EOF.
            let deadline = ContinuousClock.now + .milliseconds(250)
            while readersFinished < (outputTask == nil ? 0 : 1) + (errorTask == nil ? 0 : 1), ContinuousClock.now < deadline {
                try? await Task.sleep(for: .milliseconds(10))
            }
            readCancellation.cancel(); outputTask?.cancel(); errorTask?.cancel()
            await outputTask?.value; await errorTask?.value
            outputTask = nil; errorTask = nil
            writeCancellation.cancel()
            let input = input
            await withCheckedContinuation { done in
                writeQueue.async { try? input?.close(); done.resume() }
            }
            self.input = nil
            endOfOutput()
        }
        cleanupTask = task
        return task
    }
}
