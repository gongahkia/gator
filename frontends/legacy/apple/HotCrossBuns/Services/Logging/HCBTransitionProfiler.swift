import Foundation
import os
import SwiftUI

@MainActor
enum HCBTransitionProfiler {
    private static let log = OSLog(subsystem: "com.gongahkia.hotcrossbuns.mac", category: "transition")
    private static var storedMeasurements: [String: HCBTransitionMeasurement] = [:]

    static var isEnabled: Bool {
        HCBPerformanceTelemetry.isEnabled
    }

    static func start(
        _ name: String,
        metadata: [String: String] = [:]
    ) -> HCBTransitionMeasurement? {
        guard isEnabled else { return nil }
        let measurement = HCBTransitionMeasurement(name: name, metadata: metadata, log: log)
        measurement.begin()
        return measurement
    }

    static func startStored(
        key: String,
        name: String,
        metadata: [String: String] = [:]
    ) {
        guard let measurement = start(name, metadata: metadata) else { return }
        storedMeasurements[key] = measurement
    }

    static func markFirstContent(for key: String, metadata: [String: String] = [:]) {
        storedMeasurements[key]?.markFirstContent(metadata: metadata)
    }

    static func markSettled(for key: String, metadata: [String: String] = [:]) {
        storedMeasurements[key]?.markSettled(metadata: metadata)
        storedMeasurements[key] = nil
    }

    static func cancelStored(key: String, metadata: [String: String] = [:]) {
        storedMeasurements[key]?.cancel(metadata: metadata)
        storedMeasurements[key] = nil
    }
}

enum HCBTransitionKeys {
    static let settings = "window.settings"
    static let commandPalette = "panel.commandPalette"

    static func window(_ id: String) -> String {
        "window.\(id)"
    }
}

@MainActor
final class HCBTransitionMeasurement {
    private let name: String
    private let metadata: [String: String]
    private let log: OSLog
    private let signpostID: OSSignpostID
    private let started = DispatchTime.now().uptimeNanoseconds
    private var didBegin = false
    private var didMarkFirstContent = false
    private var didEnd = false

    init(name: String, metadata: [String: String], log: OSLog) {
        self.name = name
        self.metadata = metadata
        self.log = log
        self.signpostID = OSSignpostID(log: log)
    }

    func begin() {
        guard didBegin == false else { return }
        didBegin = true
        let context = contextString(metadata)
        os_signpost(.begin, log: log, name: "HCBTransition", signpostID: signpostID, "%{public}s %{public}s", name, context)
        logInfo("transition.start \(name)", metadata: metadata)
    }

    func markFirstContent(metadata extraMetadata: [String: String] = [:]) {
        guard didBegin, didMarkFirstContent == false, didEnd == false else { return }
        didMarkFirstContent = true
        let elapsed = elapsedMilliseconds
        let merged = mergedMetadata(extraMetadata, elapsed: elapsed)
        let context = contextString(merged)
        os_signpost(.event, log: log, name: "HCBTransitionFirstContent", signpostID: signpostID, "%{public}s %{public}s", name, context)
        logInfo("transition.firstContent \(name) elapsed_ms=\(Self.format(elapsed))", metadata: merged)
    }

    func markSettled(metadata extraMetadata: [String: String] = [:]) {
        guard didBegin, didEnd == false else { return }
        didEnd = true
        let elapsed = elapsedMilliseconds
        var merged = mergedMetadata(extraMetadata, elapsed: elapsed)
        merged["budget"] = elapsed <= 1_000 ? "p95-target" : "over-1000ms"
        let context = contextString(merged)
        os_signpost(.end, log: log, name: "HCBTransition", signpostID: signpostID, "%{public}s %{public}s", name, context)
        logInfo("transition.settled \(name) elapsed_ms=\(Self.format(elapsed))", metadata: merged)
    }

    func cancel(metadata extraMetadata: [String: String] = [:]) {
        guard didBegin, didEnd == false else { return }
        var merged = mergedMetadata(extraMetadata, elapsed: elapsedMilliseconds)
        merged["outcome"] = "cancelled"
        didEnd = true
        let context = contextString(merged)
        os_signpost(.end, log: log, name: "HCBTransition", signpostID: signpostID, "%{public}s %{public}s", name, context)
        logInfo("transition.cancelled \(name) elapsed_ms=\(Self.format(elapsedMilliseconds))", metadata: merged)
    }

    func scheduleSettled(after delay: Duration = .milliseconds(250), metadata: [String: String] = [:]) {
        Task { @MainActor [weak self] in
            do {
                try await Task.sleep(for: delay)
            } catch {
                return
            }
            self?.markSettled(metadata: metadata)
        }
    }

    private var elapsedMilliseconds: Double {
        Double(DispatchTime.now().uptimeNanoseconds - started) / 1_000_000
    }

    private func mergedMetadata(_ extraMetadata: [String: String], elapsed: Double) -> [String: String] {
        var merged = metadata.merging(extraMetadata) { _, new in new }
        merged["elapsed_ms"] = Self.format(elapsed)
        return merged
    }

    private func contextString(_ metadata: [String: String]) -> String {
        metadata
            .sorted { $0.key < $1.key }
            .map { "\($0.key)=\($0.value)" }
            .joined(separator: " ")
    }

    private func logInfo(_ message: String, metadata: [String: String]) {
        AppLogger.info(message, category: .perf, metadata: metadata)
    }

    private static func format(_ value: Double) -> String {
        String(format: "%.2f", value)
    }
}

private struct HCBTransitionFirstContentModifier: ViewModifier {
    let measurement: HCBTransitionMeasurement?
    let metadata: [String: String]

    private var measurementID: ObjectIdentifier? {
        measurement.map(ObjectIdentifier.init)
    }

    func body(content: Content) -> some View {
        content
            .onAppear(perform: markFirstContent)
            .onChange(of: measurementID) { _, _ in
                markFirstContent()
            }
    }

    private func markFirstContent() {
        guard let measurement else { return }
        Task { @MainActor in
            await Task.yield()
            measurement.markFirstContent(metadata: metadata)
        }
    }
}

private struct HCBStoredTransitionContentModifier: ViewModifier {
    let key: String
    let metadata: [String: String]

    func body(content: Content) -> some View {
        content.onAppear {
            guard HCBTransitionProfiler.isEnabled else { return }
            Task { @MainActor in
                await Task.yield()
                HCBTransitionProfiler.markFirstContent(for: key, metadata: metadata)
                HCBTransitionProfiler.markSettled(for: key, metadata: metadata)
            }
        }
    }
}

extension View {
    func hcbTransitionFirstContent(
        _ measurement: HCBTransitionMeasurement?,
        metadata: [String: String] = [:]
    ) -> some View {
        modifier(HCBTransitionFirstContentModifier(measurement: measurement, metadata: metadata))
    }

    func hcbStoredTransitionContent(
        key: String,
        metadata: [String: String] = [:]
    ) -> some View {
        modifier(HCBStoredTransitionContentModifier(key: key, metadata: metadata))
    }
}
