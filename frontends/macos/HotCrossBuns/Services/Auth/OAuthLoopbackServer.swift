import Darwin
import Foundation

struct OAuthLoopbackCallback: Sendable {
    var code: String?
    var state: String?
    var error: String?
}

final class OAuthLoopbackServer: @unchecked Sendable {
    private let lock = NSLock()
    private var socketFD: Int32 = -1
    private var continuation: CheckedContinuation<OAuthLoopbackCallback, Error>?
    private var pendingCallback: OAuthLoopbackCallback?
    private var pendingError: Error?

    func start() throws -> URL {
        let fd = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP)
        guard fd >= 0 else {
            throw CustomGoogleOAuthError.loopbackUnavailable
        }

        var reuse: Int32 = 1
        setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuse, socklen_t(MemoryLayout<Int32>.size))

        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0
        address.sin_addr = in_addr(s_addr: inet_addr("127.0.0.1"))

        let bindStatus = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bindStatus == 0, listen(fd, 1) == 0 else {
            close(fd)
            throw CustomGoogleOAuthError.loopbackUnavailable
        }

        var boundAddress = sockaddr_in()
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let nameStatus = withUnsafeMutablePointer(to: &boundAddress) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                getsockname(fd, $0, &length)
            }
        }
        guard nameStatus == 0 else {
            close(fd)
            throw CustomGoogleOAuthError.loopbackUnavailable
        }

        lock.lock()
        socketFD = fd
        lock.unlock()

        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            self?.acceptOnce(fd: fd)
        }

        let port = UInt16(bigEndian: boundAddress.sin_port)
        return URL(string: "http://127.0.0.1:\(port)/")!
    }

    func waitForCallback() async throws -> OAuthLoopbackCallback {
        try await withCheckedThrowingContinuation { continuation in
            lock.lock()
            if let pendingCallback {
                self.pendingCallback = nil
                lock.unlock()
                continuation.resume(returning: pendingCallback)
                return
            }
            if let pendingError {
                self.pendingError = nil
                lock.unlock()
                continuation.resume(throwing: pendingError)
                return
            }
            self.continuation = continuation
            lock.unlock()
        }
    }

    func stop() {
        lock.lock()
        let fd = socketFD
        socketFD = -1
        lock.unlock()
        if fd >= 0 {
            shutdown(fd, SHUT_RDWR)
            close(fd)
        }
    }

    private func acceptOnce(fd: Int32) {
        var clientAddress = sockaddr()
        var length = socklen_t(MemoryLayout<sockaddr>.size)
        let clientFD = accept(fd, &clientAddress, &length)
        guard clientFD >= 0 else {
            resume(throwing: CustomGoogleOAuthError.loopbackUnavailable)
            return
        }
        defer {
            close(clientFD)
            stop()
        }

        var buffer = [UInt8](repeating: 0, count: 16_384)
        let count = recv(clientFD, &buffer, buffer.count - 1, 0)
        guard count > 0,
              let request = String(bytes: buffer.prefix(count), encoding: .utf8),
              let firstLine = request.components(separatedBy: "\r\n").first else {
            writeResponse(to: clientFD, status: "400 Bad Request", body: "Hot Cross Buns could not read the OAuth callback.")
            resume(throwing: CustomGoogleOAuthError.missingAuthorizationCode)
            return
        }

        let parts = firstLine.split(separator: " ")
        guard parts.count >= 2,
              let url = URL(string: "http://127.0.0.1\(parts[1])"),
              let components = URLComponents(url: url, resolvingAgainstBaseURL: false) else {
            writeResponse(to: clientFD, status: "400 Bad Request", body: "Hot Cross Buns could not parse the OAuth callback.")
            resume(throwing: CustomGoogleOAuthError.missingAuthorizationCode)
            return
        }

        let query = Dictionary(uniqueKeysWithValues: (components.queryItems ?? []).map { ($0.name, $0.value) })
        let callback = OAuthLoopbackCallback(
            code: query["code"] ?? nil,
            state: query["state"] ?? nil,
            error: query["error"] ?? nil
        )

        if let error = callback.error {
            writeResponse(
                to: clientFD,
                status: "400 Bad Request",
                page: .failure("Google sign-in was cancelled or denied: \(error)")
            )
        } else {
            writeResponse(to: clientFD, status: "200 OK", page: .success)
        }
        resume(returning: callback)
    }

    private func writeResponse(to fd: Int32, status: String, body: String) {
        let page: OAuthLoopbackPage = status.hasPrefix("2")
            ? .success
            : .failure(body)
        writeResponse(to: fd, status: status, page: page)
    }

    private func writeResponse(to fd: Int32, status: String, page: OAuthLoopbackPage) {
        let html = page.html
        let response = """
        HTTP/1.1 \(status)\r
        Content-Type: text/html; charset=utf-8\r
        Content-Length: \(Data(html.utf8).count)\r
        Connection: close\r
        \r
        \(html)
        """
        _ = response.withCString { pointer in
            send(fd, pointer, strlen(pointer), 0)
        }
    }

    private func resume(returning callback: OAuthLoopbackCallback) {
        lock.lock()
        if let continuation {
            self.continuation = nil
            lock.unlock()
            continuation.resume(returning: callback)
        } else {
            pendingCallback = callback
            lock.unlock()
        }
    }

    private func resume(throwing error: Error) {
        lock.lock()
        if let continuation {
            self.continuation = nil
            lock.unlock()
            continuation.resume(throwing: error)
        } else {
            pendingError = error
            lock.unlock()
        }
    }
}

private enum OAuthLoopbackPage {
    case success
    case failure(String)

    var html: String {
        let eyebrow: String
        let title: String
        let message: String
        let symbol: String
        let accent: String
        let actionMarkup: String
        let actionHint: String

        switch self {
        case .success:
            eyebrow = "Google account connected"
            title = "You're ready to sync"
            message = "Hot Cross Buns has received Google's sign-in response. Return to the app to finish loading your tasks and calendar."
            symbol = "✓"
            accent = "#e86f3d"
            actionMarkup = #"<a class="primary-action" href="hotcrossbuns://open">Return to Hot Cross Buns</a>"#
            actionHint = "If the browser stays open, close this tab manually. The app already received the sign-in result."
        case .failure(let body):
            eyebrow = "Google sign-in did not finish"
            title = "Connection stopped"
            message = body
            symbol = "!"
            accent = "#d45858"
            actionMarkup = #"<span class="manual-action">Close this tab manually.</span>"#
            actionHint = "Return to Hot Cross Buns and try connecting Google again from Settings."
        }

        return """
        <!doctype html>
        <html lang="en">
        <head>
          <meta charset="utf-8">
          <meta name="viewport" content="width=device-width, initial-scale=1">
          <title>Hot Cross Buns</title>
                  <style>
                    :root {
                      color-scheme: light dark;
                      --bg: #f7f3ed;
                      --panel: rgba(255, 255, 255, 0.88);
                      --panel-strong: #fffdf9;
                      --ink: #272422;
                      --muted: #716a63;
                      --line: rgba(39, 36, 34, 0.12);
                      --accent: \(accent);
                      --accent-soft: color-mix(in srgb, var(--accent) 12%, transparent);
                    }
                    @media (prefers-color-scheme: dark) {
                      :root {
                        --bg: #1f1d1a;
                        --panel: rgba(48, 46, 41, 0.9);
                        --panel-strong: #302e29;
                        --ink: #f2eee8;
                        --muted: #bbb2a8;
                        --line: rgba(242, 238, 232, 0.12);
                      }
                    }
                    * { box-sizing: border-box; }
                    body {
                      min-height: 100vh;
                      margin: 0;
                      display: flex;
                      align-items: center;
                      justify-content: center;
                      padding: clamp(24px, 5vw, 72px);
                      background:
                        radial-gradient(circle at 22% 18%, var(--accent-soft), transparent 34%),
                        linear-gradient(135deg, var(--bg), color-mix(in srgb, var(--bg) 88%, var(--accent) 12%));
                      color: var(--ink);
                      font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", sans-serif;
                      line-height: 1.45;
                    }
                    main {
                      width: min(840px, 100%);
                      min-height: 420px;
                      display: grid;
                      grid-template-columns: minmax(0, 1.25fr) minmax(260px, 0.75fr);
                      overflow: hidden;
                      border: 1px solid var(--line);
                      border-radius: 18px;
                      background: var(--panel);
                      box-shadow: 0 28px 90px rgba(0, 0, 0, 0.18);
                      backdrop-filter: blur(24px);
                    }
                    section {
                      padding: clamp(32px, 5vw, 56px);
                    }
                    aside {
                      display: flex;
                      flex-direction: column;
                      justify-content: space-between;
                      gap: 24px;
                      padding: clamp(28px, 4vw, 42px);
                      border-left: 1px solid var(--line);
                      background: color-mix(in srgb, var(--panel-strong) 88%, var(--accent) 12%);
                    }
                    .mark {
                      width: 56px;
                      height: 56px;
                      display: grid;
                      place-items: center;
                      border-radius: 16px;
                      background: var(--accent);
                      color: white;
                      font-size: 30px;
                      font-weight: 800;
                      margin-bottom: 26px;
                    }
                    .eyebrow {
                      margin: 0 0 6px;
                      color: var(--accent);
                      font-size: 12px;
                      font-weight: 700;
                      letter-spacing: 0;
                      text-transform: uppercase;
                    }
                    h1 {
                      margin: 0;
                      max-width: 12ch;
                      font-size: clamp(40px, 7vw, 64px);
                      line-height: 0.98;
                      letter-spacing: 0;
                    }
                    p {
                      max-width: 48ch;
                      margin: 20px 0 0;
                      color: var(--muted);
                      font-size: 17px;
                    }
                    .primary-action {
                      display: inline-block;
                      margin-top: 30px;
                      border: 0;
                      border-radius: 8px;
                      padding: 12px 16px;
                      background: var(--accent);
                      color: white;
                      font: inherit;
                      font-weight: 700;
                      text-decoration: none;
                      cursor: pointer;
                    }
                    .manual-action {
                      display: inline-block;
                      margin-top: 30px;
                      color: var(--accent);
                      font-weight: 800;
                    }
                    .action-hint {
                      max-width: 42ch;
                      margin-top: 12px;
                      font-size: 14px;
                    }
                    .app-name {
                      display: flex;
                      align-items: center;
                      gap: 10px;
                      color: var(--ink);
                      font-size: 15px;
                      font-weight: 800;
                    }
                    .bun {
                      width: 34px;
                      height: 34px;
                      display: grid;
                      place-items: center;
                      border-radius: 10px;
                      background: var(--accent);
                      color: white;
                      font-weight: 900;
                    }
                    .next {
                      display: grid;
                      gap: 12px;
                      margin: 0;
                      padding: 0;
                      list-style: none;
                    }
                    .next li {
                      display: flex;
                      gap: 10px;
                      align-items: flex-start;
                      color: var(--muted);
                      font-size: 14px;
                    }
                    .dot {
                      width: 8px;
                      height: 8px;
                      flex: 0 0 auto;
                      margin-top: 6px;
                      border-radius: 999px;
                      background: var(--accent);
                    }
                    .footer {
                      margin: 0;
                      padding-top: 18px;
                      border-top: 1px solid var(--line);
                      color: var(--muted);
                      font-size: 13px;
                    }
                    @media (max-width: 760px) {
                      main {
                        min-height: 0;
                        grid-template-columns: 1fr;
                      }
                      aside {
                        border-left: 0;
                        border-top: 1px solid var(--line);
                      }
                      h1 {
                        max-width: none;
                      }
                    }
                  </style>
                </head>
                <body>
                  <main>
                    <section>
                      <div class="mark" aria-hidden="true">\(symbol)</div>
                      <p class="eyebrow">\(Self.escape(eyebrow))</p>
                      <h1>\(Self.escape(title))</h1>
                      <p>\(Self.escape(message))</p>
                      \(actionMarkup)
                      <p class="action-hint">\(Self.escape(actionHint))</p>
                    </section>
                    <aside>
                      <div class="app-name"><span class="bun">H</span><span>Hot Cross Buns</span></div>
                      <ul class="next">
                        <li><span class="dot"></span><span>Your OAuth token is stored in the macOS Keychain.</span></li>
                        <li><span class="dot"></span><span>Tasks and Calendar sync continue inside the app.</span></li>
                        <li><span class="dot"></span><span>This local callback listener closes after sign-in completes.</span></li>
                      </ul>
                      <p class="footer">This page was served from localhost by Hot Cross Buns. It is not a hosted service.</p>
                    </aside>
                  </main>
                </body>
                </html>
        """
    }

    private static func escape(_ string: String) -> String {
        string
            .replacingOccurrences(of: "&", with: "&amp;")
            .replacingOccurrences(of: "<", with: "&lt;")
            .replacingOccurrences(of: ">", with: "&gt;")
            .replacingOccurrences(of: "\"", with: "&quot;")
            .replacingOccurrences(of: "'", with: "&#39;")
    }
}
