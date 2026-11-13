import Foundation

public struct BridgeLaunchConfiguration: Sendable {
    public var runtime: URL
    public var workspace: URL
    public var environment: [String: String]
    public var conversationID: String
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
}

actor GoBridgeConnection: BridgeTransport {
    nonisolated let events: AsyncStream<BridgeEvent>
    private let continuation: AsyncStream<BridgeEvent>.Continuation
    private var process: Process?
    private var input: FileHandle?
    private var outputTask: Task<Void, Never>?
    private var errorTask: Task<Void, Never>?
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
        p.environment = environment; p.standardInput = stdin; p.standardOutput = stdout; p.standardError = stderr
        process = p; input = stdin.fileHandleForWriting
        do { try p.run() }
        catch { process = nil; input = nil; throw error }
        let reader = stdout.fileHandleForReading, errors = stderr.fileHandleForReading
        outputTask = Task.detached { [weak self] in
            do {
                while true {
                    let bytes = try await Self.readChunk(reader)
                    if bytes.isEmpty { break }
                    guard let self else { break }; await self.receive(bytes)
                }
                await self?.endOfOutput()
            } catch { await self?.fail("Reading Gator failed: " + error.localizedDescription) }
            try? reader.close()
        }
        errorTask = Task.detached { [weak self] in
            while let bytes = try? await Self.readChunk(errors), !bytes.isEmpty { await self?.receiveDiagnostics(bytes) }
            try? errors.close()
        }
    }

    // Foundation read(upToCount:) can wait to fill a pipe read on macOS. A
    // single POSIX read returns currently available bytes; blocking IO stays off
    // Swift's cooperative executor, so an idle stderr pipe cannot starve actors.
    nonisolated private static func readChunk(_ handle: FileHandle) async throws -> Data {
        try await withCheckedThrowingContinuation { done in
            DispatchQueue.global(qos: .utility).async {
                var buffer = [UInt8](repeating: 0, count: 64 * 1024)
                var count: Int
                repeat { count = Darwin.read(handle.fileDescriptor, &buffer, buffer.count) } while count < 0 && errno == EINTR
                if count < 0 { done.resume(throwing: BridgeFailure("The Gator pipe could not be read.")) }
                else { done.resume(returning: Data(buffer.prefix(count))) }
            }
        }
    }

    private func receive(_ bytes: Data) {
        guard !closed else { return }
        do {
            for event in try framer.append(bytes) {
                if case .acknowledged(let nonce) = event { waiters.removeValue(forKey: nonce)?.resume() }
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
        try? input?.close(); input = nil
        if let process, process.isRunning { process.terminate() }
    }

    func send(_ command: BridgeCommand, nonce: String) async throws {
        guard !closed, let input, process?.isRunning == true else { throw BridgeFailure("Gator is disconnected.") }
        let bytes = try command.data(nonce: nonce)
        // focus_session deliberately publishes state without action_applied.
        // Its caller must await the exact foreground projection, not a nonce.
        if case .host(.focus) = command {
            writeQueue.async { [weak self] in
                do { try input.write(contentsOf: bytes) }
                catch { Task { await self?.fail("Gator did not confirm focus. Original work was not moved.") } }
            }
            return
        }
        guard waiters[nonce] == nil else { throw BridgeFailure("Duplicate transport identity.") }
        try await withCheckedThrowingContinuation { waiter in
            waiters[nonce] = waiter
            writeQueue.async { [weak self] in
                do { try input.write(contentsOf: bytes) }
                catch { Task { await self?.fail("Writing to Gator failed. Admission is unconfirmed.") } }
            }
            Task { [weak self, acknowledgementTimeout] in
                try? await Task.sleep(for: acknowledgementTimeout)
                await self?.expire(nonce)
            }
        }
    }
    private func expire(_ nonce: String) {
        waiters.removeValue(forKey: nonce)?.resume(throwing: BridgeFailure("Gator did not acknowledge this action. Inspect its retained state before retrying."))
    }

    func processIdentifier() -> Int32? { process?.processIdentifier }

    func shutdown() async {
        deliberate = true
        if let input, !closed, let data = try? BridgeCommand.exit.data(nonce: "") {
            // A full stdin pipe must not prevent the escalation timer from running.
            writeQueue.async { try? input.write(contentsOf: data); try? input.close() }
        }
        // Presentation dismissal never calls this method. Only guarded app shutdown/reconnect does.
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
        // waitUntilExit reaps the child after the bounded escalation above.
        process?.waitUntilExit()
        await outputTask?.value; await errorTask?.value
        fail("Gator closed.")
    }
}
