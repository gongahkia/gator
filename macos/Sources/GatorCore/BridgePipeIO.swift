import Foundation
import Darwin

// only the cancellation bit crosses queues; descriptors stay with their I/O owner.
final class BridgeIOCancellation: @unchecked Sendable {
    private let lock = NSLock()
    private var cancelled = false
    var isCancelled: Bool { lock.withLock { cancelled } }
    func cancel() { lock.withLock { cancelled = true } }
}

enum BridgePipeIO {
    static func prepare(_ handle: FileHandle) throws {
        let fd = handle.fileDescriptor, flags = fcntl(fd, F_GETFL)
        guard flags >= 0, fcntl(fd, F_SETFL, flags | O_NONBLOCK) == 0 else { throw BridgeFailure("Could not configure Gator's private pipe.") }
    }
    static func read(_ handle: FileHandle, cancellation: BridgeIOCancellation) async throws -> Data? {
        try Task.checkCancellation()
        let result: Data? = try await withCheckedThrowingContinuation { done in
            DispatchQueue.global(qos: .utility).async {
                do {
                    if cancellation.isCancelled { throw CancellationError() }
                    var descriptor = pollfd(fd: handle.fileDescriptor, events: Int16(POLLIN), revents: 0)
                    let ready = poll(&descriptor, 1, 100)
                    if cancellation.isCancelled { throw CancellationError() }
                    if ready == 0 || ready < 0 && errno == EINTR { done.resume(returning: nil); return }
                    guard ready > 0 else { throw BridgeFailure("The Gator pipe could not be read.") }
                    var buffer = [UInt8](repeating: 0, count: 64 * 1024)
                    let count = Darwin.read(descriptor.fd, &buffer, buffer.count)
                    if count < 0 && (errno == EINTR || errno == EAGAIN) { done.resume(returning: nil); return }
                    guard count >= 0 else { throw BridgeFailure("The Gator pipe could not be read.") }
                    done.resume(returning: Data(buffer.prefix(count)))
                } catch { done.resume(throwing: error) }
            }
        }
        try Task.checkCancellation()
        return result
    }
    static func write(_ data: Data, to handle: FileHandle, cancellation: BridgeIOCancellation) throws {
        try data.withUnsafeBytes { bytes in
            var offset = 0
            while offset < bytes.count {
                if cancellation.isCancelled { throw CancellationError() }
                var descriptor = pollfd(fd: handle.fileDescriptor, events: Int16(POLLOUT), revents: 0)
                let ready = poll(&descriptor, 1, 100)
                if ready == 0 || ready < 0 && errno == EINTR { continue }
                guard ready > 0 else { throw BridgeFailure("The Gator pipe could not be written.") }
                let count = Darwin.write(descriptor.fd, bytes.baseAddress!.advanced(by: offset), bytes.count - offset)
                if count < 0 && (errno == EINTR || errno == EAGAIN) { continue }
                guard count > 0 else { throw BridgeFailure("The Gator pipe could not be written.") }
                offset += count
            }
        }
    }
}
