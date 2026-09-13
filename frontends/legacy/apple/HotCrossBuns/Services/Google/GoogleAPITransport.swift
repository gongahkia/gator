import Foundation

protocol AccessTokenProviding: Sendable {
    @MainActor
    func accessToken() async throws -> String
}

struct StaticAccessTokenProvider: AccessTokenProviding {
    var token: String

    @MainActor
    func accessToken() async throws -> String {
        token
    }
}

struct GoogleAPITransport: Sendable {
    var baseURL: URL
    var tokenProvider: AccessTokenProviding
    var urlSession: URLSession

    init(
        baseURL: URL,
        tokenProvider: AccessTokenProviding,
        urlSession: URLSession = .shared
    ) {
        self.baseURL = baseURL
        self.tokenProvider = tokenProvider
        self.urlSession = urlSession
    }

    func get<Response: Decodable & Sendable>(
        path: String,
        queryItems: [URLQueryItem] = [],
        decoder: JSONDecoder = .googleAPI
    ) async throws -> Response {
        try await request(path: path, queryItems: queryItems, decoder: decoder)
    }

    // §14 — GET variant that also returns the server's Date header. Used by
    // the Tasks client to derive an incremental-sync watermark from Google's
    // clock rather than the local one, eliminating the 300s drift slack.
    // Returns nil for `serverDate` when the header is missing or malformed
    // so callers can fall back safely.
    func getWithServerDate<Response: Decodable & Sendable>(
        path: String,
        queryItems: [URLQueryItem] = [],
        decoder: JSONDecoder = .googleAPI
    ) async throws -> (Response, Date?) {
        let (data, response) = try await perform(
            method: "GET",
            path: path,
            queryItems: queryItems,
            bodyData: nil,
            ifMatch: nil
        )
        let decoded = try decoder.decode(Response.self, from: data)
        let serverDate: Date? = {
            guard let http = response as? HTTPURLResponse,
                  let header = http.value(forHTTPHeaderField: "Date") else { return nil }
            return HTTPDateParser.parse(header)
        }()
        return (decoded, serverDate)
    }

    func request<Response: Decodable & Sendable, Body: Encodable & Sendable>(
        method: String = "GET",
        path: String,
        queryItems: [URLQueryItem] = [],
        body: Body? = nil,
        ifMatch: String? = nil,
        encoder: JSONEncoder = .googleAPI,
        decoder: JSONDecoder = .googleAPI
    ) async throws -> Response {
        let bodyData = try body.map { try encoder.encode($0) }
        let (data, _) = try await perform(
            method: method,
            path: path,
            queryItems: queryItems,
            bodyData: bodyData,
            ifMatch: ifMatch
        )
        return try decoder.decode(Response.self, from: data)
    }

    func send<Body: Encodable & Sendable>(
        method: String,
        path: String,
        queryItems: [URLQueryItem] = [],
        body: Body? = nil,
        ifMatch: String? = nil,
        encoder: JSONEncoder = .googleAPI
    ) async throws {
        let bodyData = try body.map { try encoder.encode($0) }
        _ = try await perform(
            method: method,
            path: path,
            queryItems: queryItems,
            bodyData: bodyData,
            ifMatch: ifMatch
        )
    }

    func send(
        method: String,
        path: String,
        queryItems: [URLQueryItem] = [],
        ifMatch: String? = nil
    ) async throws {
        let emptyBody: EmptyRequestBody? = nil
        try await send(method: method, path: path, queryItems: queryItems, body: emptyBody, ifMatch: ifMatch)
    }

    func request<Response: Decodable & Sendable>(
        method: String = "GET",
        path: String,
        queryItems: [URLQueryItem] = [],
        ifMatch: String? = nil,
        decoder: JSONDecoder = .googleAPI
    ) async throws -> Response {
        let emptyBody: EmptyRequestBody? = nil
        return try await request(
            method: method,
            path: path,
            queryItems: queryItems,
            body: emptyBody,
            ifMatch: ifMatch,
            decoder: decoder
        )
    }

    private func perform(
        method: String,
        path: String,
        queryItems: [URLQueryItem],
        bodyData: Data?,
        ifMatch: String?
    ) async throws -> (Data, URLResponse) {
        let requestID = GoogleDiagnostics.requestID()
        let started = DispatchTime.now().uptimeNanoseconds
        let baseMetadata = GoogleDiagnostics.baseMetadata(
            requestID: requestID,
            method: method,
            path: path,
            queryItems: queryItems,
            requestBody: bodyData
        )
        AppLogger.info(
            "google request start",
            category: .google,
            metadata: baseMetadata,
            localOnlyMetadata: GoogleDiagnostics.rawMetadata(requestBody: bodyData, responseBody: nil)
        )

        var request: URLRequest
        do {
            request = try await makeRequest(method: method, path: path, queryItems: queryItems, ifMatch: ifMatch)
        } catch {
            let failure = GoogleDiagnostics.failureMetadata(
                error: error,
                status: nil,
                responseBody: nil,
                durationMilliseconds: GoogleDiagnostics.elapsedMilliseconds(since: started)
            )
            AppLogger.warn(
                "google request failed",
                category: .google,
                metadata: baseMetadata.merging(failure) { _, new in new }
            )
            throw error
        }

        if let bodyData {
            request.httpBody = bodyData
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await urlSession.data(for: request)
        } catch {
            let failure = GoogleDiagnostics.failureMetadata(
                error: error,
                status: nil,
                responseBody: nil,
                durationMilliseconds: GoogleDiagnostics.elapsedMilliseconds(since: started)
            )
            AppLogger.warn(
                "google request failed",
                category: .google,
                metadata: baseMetadata.merging(failure) { _, new in new }
            )
            throw error
        }

        let status = (response as? HTTPURLResponse)?.statusCode
        do {
            try validate(response: response, data: data)
        } catch {
            let failure = GoogleDiagnostics.failureMetadata(
                error: error,
                status: status,
                responseBody: data,
                durationMilliseconds: GoogleDiagnostics.elapsedMilliseconds(since: started)
            )
            AppLogger.warn(
                "google request failed",
                category: .google,
                metadata: baseMetadata.merging(failure) { _, new in new },
                localOnlyMetadata: GoogleDiagnostics.rawMetadata(requestBody: nil, responseBody: data)
            )
            throw error
        }

        let success = GoogleDiagnostics.successMetadata(
            status: status,
            responseBody: data,
            durationMilliseconds: GoogleDiagnostics.elapsedMilliseconds(since: started)
        )
        AppLogger.info(
            "google request succeeded",
            category: .google,
            metadata: baseMetadata.merging(success) { _, new in new },
            localOnlyMetadata: GoogleDiagnostics.rawMetadata(requestBody: nil, responseBody: data)
        )
        return (data, response)
    }

    private func makeRequest(
        method: String,
        path: String,
        queryItems: [URLQueryItem],
        ifMatch: String? = nil
    ) async throws -> URLRequest {
        var components = URLComponents(url: baseURL.appending(path: path), resolvingAgainstBaseURL: false)
        components?.queryItems = queryItems.isEmpty ? nil : queryItems

        guard let url = components?.url else {
            throw GoogleAPIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("Bearer \(try await tokenProvider.accessToken())", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let ifMatch, ifMatch.isEmpty == false {
            request.setValue(ifMatch, forHTTPHeaderField: "If-Match")
        }
        return request
    }

    private func validate(response: URLResponse, data: Data) throws {
        guard let httpResponse = response as? HTTPURLResponse else {
            throw GoogleAPIError.invalidResponse
        }

        if httpResponse.statusCode == 412 {
            throw GoogleAPIError.preconditionFailed
        }

        if httpResponse.statusCode == 400 {
            throw GoogleAPIError.invalidPayload(String(data: data, encoding: .utf8))
        }

        guard (200..<300).contains(httpResponse.statusCode) else {
            throw GoogleAPIError.httpStatus(httpResponse.statusCode, String(data: data, encoding: .utf8))
        }
    }
}

private struct EmptyRequestBody: Encodable, Sendable {}

enum GoogleAPIError: LocalizedError, Equatable {
    case invalidURL
    case invalidResponse
    case preconditionFailed
    case invalidPayload(String?)
    case httpStatus(Int, String?)

    var errorDescription: String? {
        switch self {
        case .invalidURL:
            "Something went wrong building the Google request. Try again, or share a diagnostic bundle if it keeps happening."
        case .invalidResponse:
            "Google returned a response we couldn't read. Try Refresh."
        case .preconditionFailed:
            "Someone else (or another device) changed this item while you were editing. We'll refresh so you can try again with the latest version."
        case .invalidPayload(let body):
            invalidPayloadMessage(body: body)
        case .httpStatus(let status, let body):
            statusMessage(status: status, body: body)
        }
    }

    var isInvalidPayload: Bool {
        if case .invalidPayload = self {
            return true
        }
        return false
    }

    var responseBody: String? {
        switch self {
        case .httpStatus(_, let body), .invalidPayload(let body):
            body
        default:
            nil
        }
    }

    private func statusMessage(status: Int, body: String?) -> String {
        let baseMessage = switch status {
        case 401:
            "Your Google session expired. Reconnect in Settings to keep syncing."
        case 403 where Self.isQuotaBody(body):
            "Google API quota is exhausted for this account or OAuth project. Automatic retry won't help until Google resets quota; check the Google Cloud quota page for the Calendar and Tasks APIs."
        case 403:
            "Google denied access. Reconnect in Settings so you can re-grant the Tasks + Calendar permissions."
        case 404:
            "That item isn't on Google anymore — it may have been deleted on another device. Refresh to get the latest state."
        case 410:
            "Google's sync window expired. Force Full Resync in Diagnostics to rebuild from scratch."
        case 429 where Self.isQuotaBody(body):
            "Google API quota is exhausted for this account or OAuth project. Automatic retry won't help until Google resets quota; check the Google Cloud quota page for the Calendar and Tasks APIs."
        case 429:
            "Google is rate-limiting Hot Cross Buns. It'll retry automatically in a minute or two."
        case 500...599:
            "Google Calendar is briefly having trouble. We'll retry automatically — check the sync indicator."
        default:
            "Google rejected the request (status \(status)). Try again or share a diagnostic bundle."
        }

        guard let body, body.isEmpty == false else {
            return baseMessage
        }

        return baseMessage + " " + body.prefix(180)
    }

    private func invalidPayloadMessage(body: String?) -> String {
        let baseMessage = "Google rejected the queued write because the payload was invalid. Review it in Sync Issues, copy the payload if needed, then retry after fixing the source data."
        guard let body, body.isEmpty == false else {
            return baseMessage
        }
        return baseMessage + " " + body.prefix(180)
    }

    static func isQuotaBody(_ body: String?) -> Bool {
        guard let lower = body?.lowercased() else { return false }
        return lower.contains("quotaexceeded")
            || lower.contains("dailylimitexceeded")
            || lower.contains("usage limits")
            || lower.contains("quota exceeded")
            || lower.contains("daily limit")
    }
}

enum SyncFailureKind: Equatable, Sendable {
    case offline
    case rateLimited
    case quotaExceeded
    case serviceUnavailable
    case authRequired
    case invalidPayload
    case notFound
    case other

    static func classify(_ error: Error) -> SyncFailureKind {
        if let urlError = error as? URLError {
            switch urlError.code {
            case .notConnectedToInternet, .networkConnectionLost, .cannotFindHost, .cannotConnectToHost, .dnsLookupFailed, .timedOut:
                return .offline
            default:
                break
            }
        }

        if let tokenError = error as? GoogleTokenRefreshError, tokenError.requiresReconnect {
            return .authRequired
        }

        guard let apiError = error as? GoogleAPIError else {
            return .other
        }

        switch apiError {
        case .invalidPayload:
            return .invalidPayload
        case .httpStatus(let status, _):
            if GoogleAPIError.isQuotaBody(apiError.responseBody) {
                return .quotaExceeded
            }
            switch status {
            case 401, 403:
                return .authRequired
            case 404:
                return .notFound
            case 429:
                return .rateLimited
            case 500...599:
                return .serviceUnavailable
            default:
                return .other
            }
        default:
            return .other
        }
    }
}

extension JSONDecoder {
    static var googleAPI: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let value = try container.decode(String.self)
            if let date = GoogleDateParser.parse(value) {
                return date
            }
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "Invalid Google date string: \\(value)"
            )
        }
        return decoder
    }
}

extension JSONEncoder {
    static var googleAPI: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }
}

// Parses the HTTP `Date` response header. RFC 7231 requires IMF-fixdate
// (RFC 1123), but some intermediaries still emit RFC 850 or asctime. We
// accept all three to stay robust; the result is a UTC Date.
enum HTTPDateParser {
    private static let rfc1123: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(secondsFromGMT: 0)
        f.dateFormat = "EEE, dd MMM yyyy HH:mm:ss zzz"
        return f
    }()

    private static let rfc850: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(secondsFromGMT: 0)
        f.dateFormat = "EEEE, dd-MMM-yy HH:mm:ss zzz"
        return f
    }()

    private static let asctime: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(secondsFromGMT: 0)
        f.dateFormat = "EEE MMM d HH:mm:ss yyyy"
        return f
    }()

    static func parse(_ s: String) -> Date? {
        rfc1123.date(from: s) ?? rfc850.date(from: s) ?? asctime.date(from: s)
    }
}

private enum GoogleDateParser {
    static let internetWithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    static let internet: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    static let dateOnly: DateFormatter = {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(secondsFromGMT: 0)
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter
    }()

    static func parse(_ value: String) -> Date? {
        if let date = fastGoogleWireDate(value) {
            return date
        }

        if isGoogleDateOnly(value) {
            return dateOnly.date(from: value)
                ?? internet.date(from: value)
                ?? internetWithFractionalSeconds.date(from: value)
        }

        if value.utf8.contains(46) { // "."
            return internetWithFractionalSeconds.date(from: value)
                ?? internet.date(from: value)
                ?? dateOnly.date(from: value)
        }

        return internet.date(from: value)
            ?? internetWithFractionalSeconds.date(from: value)
            ?? dateOnly.date(from: value)
    }

    private static func isGoogleDateOnly(_ value: String) -> Bool {
        let utf8 = value.utf8
        guard utf8.count == 10 else { return false }

        for (offset, byte) in utf8.enumerated() {
            switch offset {
            case 4, 7:
                guard byte == 45 else { return false } // "-"
            default:
                guard byte >= 48 && byte <= 57 else { return false } // "0"..."9"
            }
        }
        return true
    }

    private static func fastGoogleWireDate(_ value: String) -> Date? {
        if let parsed = value.utf8.withContiguousStorageIfAvailable({ bytes in
            parseFastGoogleWireDate(bytes)
        }) {
            return parsed
        }

        let bytes = Array(value.utf8)
        return bytes.withUnsafeBufferPointer { bytes in
            parseFastGoogleWireDate(bytes)
        }
    }

    // Covers the hot Google Calendar/Tasks wire forms:
    // yyyy-MM-dd, yyyy-MM-ddTHH:mm:ssZ, yyyy-MM-ddTHH:mm:ss.SSSZ,
    // and the same timed forms with +/-HH:mm offsets. Anything outside those
    // exact shapes returns nil so Foundation's parsers preserve old behavior.
    private static func parseFastGoogleWireDate(_ bytes: UnsafeBufferPointer<UInt8>) -> Date? {
        guard bytes.count == 10 || bytes.count == 20 || bytes.count == 24 || bytes.count == 25 || bytes.count == 29 else {
            return nil
        }
        guard let year = digits(bytes, 0, 4),
              bytes[4] == 45, // "-"
              let month = digits(bytes, 5, 2),
              bytes[7] == 45,
              let day = digits(bytes, 8, 2),
              dateIsValid(year: year, month: month, day: day)
        else {
            return nil
        }

        guard bytes.count > 10 else {
            return date(year: year, month: month, day: day)
        }

        guard bytes[10] == 84, // "T"
              let hour = digits(bytes, 11, 2),
              bytes[13] == 58, // ":"
              let minute = digits(bytes, 14, 2),
              bytes[16] == 58,
              let second = digits(bytes, 17, 2),
              hour < 24,
              minute < 60,
              second < 60
        else {
            return nil
        }

        var cursor = 19
        var fractionalSeconds = 0.0
        if cursor < bytes.count, bytes[cursor] == 46 { // "."
            guard bytes.count == 24 || bytes.count == 29,
                  let milliseconds = digits(bytes, cursor + 1, 3)
            else {
                return nil
            }
            fractionalSeconds = Double(milliseconds) / 1_000
            cursor += 4
        }

        let offsetSeconds: Int
        guard cursor < bytes.count else { return nil }
        switch bytes[cursor] {
        case 90: // "Z"
            guard cursor == bytes.count - 1 else { return nil }
            offsetSeconds = 0
        case 43, 45: // "+", "-"
            guard cursor + 5 == bytes.count - 1,
                  let offsetHour = digits(bytes, cursor + 1, 2),
                  bytes[cursor + 3] == 58,
                  let offsetMinute = digits(bytes, cursor + 4, 2),
                  offsetHour < 24,
                  offsetMinute < 60
            else {
                return nil
            }
            let sign = bytes[cursor] == 43 ? 1 : -1
            offsetSeconds = sign * ((offsetHour * 60 + offsetMinute) * 60)
        default:
            return nil
        }

        let days = daysSinceUnixEpoch(year: year, month: month, day: day)
        let localSeconds = days * 86_400 + hour * 3_600 + minute * 60 + second
        return Date(timeIntervalSince1970: Double(localSeconds - offsetSeconds) + fractionalSeconds)
    }

    private static func digits(_ bytes: UnsafeBufferPointer<UInt8>, _ start: Int, _ count: Int) -> Int? {
        guard start >= 0, count > 0, start + count <= bytes.count else { return nil }
        var value = 0
        for index in start..<(start + count) {
            let byte = bytes[index]
            guard byte >= 48 && byte <= 57 else { return nil } // "0"..."9"
            value = value * 10 + Int(byte - 48)
        }
        return value
    }

    private static func date(year: Int, month: Int, day: Int) -> Date {
        Date(timeIntervalSince1970: Double(daysSinceUnixEpoch(year: year, month: month, day: day) * 86_400))
    }

    private static func dateIsValid(year: Int, month: Int, day: Int) -> Bool {
        guard year > 0, (1...12).contains(month), day >= 1 else { return false }
        return day <= daysInMonth(year: year, month: month)
    }

    private static func daysInMonth(year: Int, month: Int) -> Int {
        switch month {
        case 1, 3, 5, 7, 8, 10, 12:
            return 31
        case 4, 6, 9, 11:
            return 30
        case 2:
            return isLeapYear(year) ? 29 : 28
        default:
            return 0
        }
    }

    private static func isLeapYear(_ year: Int) -> Bool {
        (year.isMultiple(of: 4) && year.isMultiple(of: 100) == false) || year.isMultiple(of: 400)
    }

    private static func daysSinceUnixEpoch(year: Int, month: Int, day: Int) -> Int {
        var adjustedYear = year
        adjustedYear -= month <= 2 ? 1 : 0
        let era = (adjustedYear >= 0 ? adjustedYear : adjustedYear - 399) / 400
        let yearOfEra = adjustedYear - era * 400
        let adjustedMonth = month + (month > 2 ? -3 : 9)
        let dayOfYear = (153 * adjustedMonth + 2) / 5 + day - 1
        let dayOfEra = yearOfEra * 365 + yearOfEra / 4 - yearOfEra / 100 + dayOfYear
        return era * 146_097 + dayOfEra - 719_468
    }
}
