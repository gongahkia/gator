import AppKit
import Observation
import SwiftUI
import Testing
@testable import GatorMac

@MainActor private final class PetSurfaceFixture: NativeDesktopPetSurface {
    var requests: [NativeMascotRequest] = []
    var frames: [CGRect] = []
    var closes = 0
    var onShow: (() -> Void)?
    func show(_ request: NativeMascotRequest, frame: CGRect) {
        requests.append(request); frames.append(frame); onShow?()
    }
    func close() { closes += 1 }
}

@MainActor @Observable private final class PetSignalFixture {
    var signal: NativeMascotSignal? = .init(owner: "A", connected: true)
    var closing = false
}

private func petScreen(_ id: String = "primary", x: Double = 0, y: Double = 0) -> NativeDesktopPetScreen {
    .init(id: id, name: id, frame: CGRect(x: x, y: y, width: 1470, height: 956),
          visible: CGRect(x: x, y: y + 40, width: 1470, height: 883))
}

@MainActor private func petDefaults() throws -> (UserDefaults, String) {
    let suite = "gator.desktopPet.test." + UUID().uuidString
    return (try #require(UserDefaults(suiteName: suite)), suite)
}

@Test func desktopPetPlacementUsesExistingExpandedNotchEnvelope() throws {
    var screen = petScreen()
    screen.safeInsets = NSEdgeInsets(top: 32, left: 0, bottom: 0, right: 0)
    screen.notch = NotchGeometry(frame: screen.frame, cameraWidth: 179, cameraHeight: 32)
    let frame = try #require(screen.petFrame)
    #expect(frame == CGRect(x: 699, y: 626, width: 72, height: 72))
    #expect(!frame.intersects(try #require(screen.notch).panelFrame(expanded: true)))
}

@Test func desktopPetGeometryHandlesNegativeOriginsAndMissingDisplays() throws {
    let primary = petScreen(), secondary = petScreen("secondary", x: -1920, y: 956)
    for screen in [primary, secondary] {
        let frame = try #require(screen.petFrame)
        #expect(screen.visible.contains(frame))
        #expect(frame.midX == screen.frame.midX)
    }
    #expect(NativeDesktopPetScreen.selected([primary, secondary], preferred: "secondary")?.id == "secondary")
    #expect(NativeDesktopPetScreen.selected([primary, secondary], preferred: "removed")?.id == "primary")
    #expect(NativeDesktopPetScreen.selected([], preferred: "secondary") == nil)
    let tiny = NativeDesktopPetScreen(id: "tiny", name: "tiny", frame: CGRect(x: 0, y: 0, width: 80, height: 80), visible: CGRect(x: 0, y: 0, width: 80, height: 80))
    #expect(tiny.petFrame == nil)
}

@Test @MainActor func desktopPetDefaultsOffAndDoesNotReadAnOwnerOrCreateAPresentation() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    var reads = 0, creates = 0
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { reads += 1; return .init(owner: "A", connected: true) }, isClosing: { false }, screens: { [petScreen()] }, makeSurface: { creates += 1; return PetSurfaceFixture() })
    defer { pet.stop() }
    pet.reconcile()
    #expect(reads == 0 && creates == 0)
    #expect(defaults.object(forKey: NativeDesktopPetPreference.shown) == nil)
}

@Test @MainActor func desktopPetSettingsPersistSeparatelyFromInChatVisibility() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    struct Preferences {
        @AppStorage var desktop: Bool
        @AppStorage var chat: Bool
        init(_ defaults: UserDefaults) {
            _desktop = AppStorage(wrappedValue: false, NativeDesktopPetPreference.shown, store: defaults)
            _chat = AppStorage(wrappedValue: true, NativeMascotPreference.shown, store: defaults)
        }
    }
    let preferences = Preferences(defaults)
    for desktop in [false, true] { for chat in [false, true] {
        preferences.desktop = desktop; preferences.chat = chat
        let reopened = Preferences(defaults)
        #expect(reopened.desktop == desktop && reopened.chat == chat)
    } }
    #expect(Set((defaults.persistentDomain(forName: suite) ?? [:]).keys) == Set([NativeDesktopPetPreference.shown, NativeMascotPreference.shown]))
}

@Test @MainActor func desktopPetUsesOneSurfaceAndOnlyTheSelectedOwnersPublicSignal() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    let fixture = PetSignalFixture(), surface = PetSurfaceFixture()
    var creates = 0
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { fixture.signal }, isClosing: { fixture.closing }, screens: { [petScreen()] }, makeSurface: { creates += 1; return surface })
    defer { pet.stop() }
    fixture.signal = .init(owner: "A", connected: true, working: true); pet.reconcile()
    #expect(surface.requests.last?.expression == .working)
    fixture.signal = .init(owner: "A", connected: true, working: true, publicTextRevision: 4); pet.reconcile()
    #expect(surface.requests.last?.expression == .responding)
    fixture.signal = .init(owner: "A", connected: true, working: true, attention: true, publicTextRevision: 5); pet.reconcile()
    #expect(surface.requests.last?.expression == .attention)
    fixture.signal = .init(owner: "B", connected: true, completedRun: "older-completion"); pet.reconcile()
    #expect(surface.requests.last?.owner == "B" && surface.requests.last?.expression == .neutral)
    fixture.signal = .init(owner: "B", connected: false); pet.reconcile()
    #expect(surface.requests.last?.expression == .disconnected)
    #expect(creates == 1)
}

@Test @MainActor func desktopPetSharesStyleAndStaticPreferenceWithoutReadingChatVisibility() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    defaults.set(false, forKey: NativeMascotPreference.shown)
    defaults.set(false, forKey: NativeMascotPreference.animated)
    defaults.set("classic", forKey: "gator.mascot.style")
    let surface = PetSurfaceFixture()
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { .init(owner: "A", connected: true) }, isClosing: { false }, screens: { [petScreen()] }, makeSurface: { surface })
    defer { pet.stop() }
    #expect(surface.requests.last?.mode == .emoji)
    #expect(surface.requests.last?.motionAllowed == false)
    #expect(surface.requests.last?.reduceMotion == NSWorkspace.shared.accessibilityDisplayShouldReduceMotion)
}

@Test @MainActor func desktopPetTuckPersistsAndCannotBeUndoneByStateOrWake() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    let fixture = PetSignalFixture(), surface = PetSurfaceFixture()
    var creates = 0
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { fixture.signal }, isClosing: { false }, screens: { [petScreen()] }, makeSurface: { creates += 1; return surface })
    defer { pet.stop() }
    defaults.set(false, forKey: NativeDesktopPetPreference.shown); pet.reconcile()
    fixture.signal = .init(owner: "A", connected: true, completedRun: "confirmed")
    NSWorkspace.shared.notificationCenter.post(name: NSWorkspace.screensDidWakeNotification, object: nil)
    for _ in 0..<20 { pet.reconcile() }
    #expect(creates == 1 && surface.closes == 1)
    #expect(!defaults.bool(forKey: NativeDesktopPetPreference.shown))
    let reopened = try #require(UserDefaults(suiteName: suite))
    #expect(!reopened.bool(forKey: NativeDesktopPetPreference.shown))
}

@Test @MainActor func desktopPetClosingAndStopReleaseTheSurfaceWithoutChangingEnablePreference() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    let fixture = PetSignalFixture(), surface = PetSurfaceFixture()
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { fixture.signal }, isClosing: { fixture.closing }, screens: { [petScreen()] }, makeSurface: { surface })
    fixture.closing = true; pet.reconcile()
    #expect(surface.closes == 1)
    fixture.closing = false; pet.reconcile()
    #expect(surface.requests.count == 2)
    pet.stop(); pet.stop(); pet.reconcile()
    #expect(surface.closes == 2 && surface.requests.count == 2)
    #expect(defaults.bool(forKey: NativeDesktopPetPreference.shown))
}

@Test @MainActor func desktopPetDisplayFallbackDoesNotOverwriteTheChosenIdentity() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    defaults.set("secondary", forKey: NativeDesktopPetPreference.display)
    let surface = PetSurfaceFixture()
    var screens = [petScreen(), petScreen("secondary", x: -1920)]
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { .init(owner: "A", connected: true) }, isClosing: { false }, screens: { screens }, makeSurface: { surface })
    defer { pet.stop() }
    #expect(surface.frames.last == screens[1].petFrame)
    screens.removeLast(); pet.reconcile()
    #expect(surface.frames.last == screens[0].petFrame)
    #expect(defaults.string(forKey: NativeDesktopPetPreference.display) == "secondary")
    screens = []; pet.reconcile(); #expect(surface.closes == 1)
}

@Test(.timeLimit(.minutes(1))) @MainActor func desktopPetObservationUpdatesAfterCurrentOwnerReplacement() async throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    let fixture = PetSignalFixture(), surface = PetSurfaceFixture()
    let (stream, continuation) = AsyncStream<Void>.makeStream(bufferingPolicy: .bufferingNewest(1))
    surface.onShow = { continuation.yield(()) }
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { fixture.signal }, isClosing: { fixture.closing }, screens: { [petScreen()] }, makeSurface: { surface })
    defer { pet.stop(); continuation.finish() }
    var iterator = stream.makeAsyncIterator()
    _ = await iterator.next()
    fixture.signal = .init(owner: "replacement", connected: true, working: true)
    _ = await iterator.next()
    #expect(surface.requests.last?.owner == "replacement")
    #expect(surface.requests.last?.expression == .working)
}

@Test @MainActor func desktopPetRepeatedTuckReleasesItsPresentation() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    weak var last: PetSurfaceFixture?
    var creates = 0
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { .init(owner: "A", connected: true) }, isClosing: { false }, screens: { [petScreen()] }, makeSurface: {
        let surface = PetSurfaceFixture(); last = surface; creates += 1; return surface
    })
    defer { pet.stop() }
    for _ in 0..<100 {
        defaults.set(true, forKey: NativeDesktopPetPreference.shown); pet.reconcile()
        #expect(last != nil)
        defaults.set(false, forKey: NativeDesktopPetPreference.shown); pet.reconcile()
        #expect(last == nil)
    }
    #expect(creates == 100)
}

@Test @MainActor func desktopPetSleepAndWakeRespectThePersistedEnableState() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    let surface = PetSurfaceFixture()
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { .init(owner: "A", connected: true) }, isClosing: { false }, screens: { [petScreen()] }, makeSurface: { surface })
    defer { pet.stop() }
    NSWorkspace.shared.notificationCenter.post(name: NSWorkspace.screensDidSleepNotification, object: nil); pet.reconcile()
    #expect(surface.closes == 1 && surface.requests.count == 1)
    NSWorkspace.shared.notificationCenter.post(name: NSWorkspace.screensDidWakeNotification, object: nil); pet.reconcile()
    #expect(surface.requests.count == 2)
    defaults.set(false, forKey: NativeDesktopPetPreference.shown); pet.reconcile()
    NSWorkspace.shared.notificationCenter.post(name: NSWorkspace.screensDidWakeNotification, object: nil); pet.reconcile()
    #expect(surface.requests.count == 2 && !defaults.bool(forKey: NativeDesktopPetPreference.shown))
}

@Test @MainActor func desktopPresenceOffStopsOverlayWithoutReenablingOrClearingVisibilityChoices() throws {
    let (defaults, suite) = try petDefaults(); defer { defaults.removePersistentDomain(forName: suite) }
    defaults.set(true, forKey: NativeDesktopPetPreference.shown)
    defaults.set(false, forKey: NativeMascotPreference.shown)
    defaults.set("off", forKey: NativeMascotPreference.mode)
    var reads = 0, creates = 0
    let surface = PetSurfaceFixture()
    let pet = NativeDesktopPet(defaults: defaults, readSignal: { reads += 1; return .init(owner: "A", connected: true) }, isClosing: { false }, screens: { [petScreen()] }, makeSurface: { creates += 1; return surface })
    defer { pet.stop() }
    #expect(reads == 0 && creates == 0)
    defaults.set("reactions", forKey: NativeMascotPreference.mode); pet.reconcile()
    #expect(surface.requests.last?.mode == .reactions && creates == 1)
    defaults.set("off", forKey: NativeMascotPreference.mode); pet.reconcile()
    #expect(surface.closes == 1 && defaults.bool(forKey: NativeDesktopPetPreference.shown))
    defaults.set(false, forKey: NativeDesktopPetPreference.shown)
    defaults.set("emoji", forKey: NativeMascotPreference.mode); pet.reconcile()
    #expect(creates == 1 && !defaults.bool(forKey: NativeDesktopPetPreference.shown))
}
