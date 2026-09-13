import XCTest
@testable import HotCrossBunsMac

final class GuestsSectionEmailTests: XCTestCase {
    func testAcceptsValidEmails() {
        XCTAssertTrue(GuestsSection.isPlausibleEmail("a@example.com"))
        XCTAssertTrue(GuestsSection.isPlausibleEmail("first.last@sub.example.co.uk"))
        XCTAssertTrue(GuestsSection.isPlausibleEmail("name+tag@domain.io"))
    }

    func testRejectsCommonBadPastes() {
        XCTAssertFalse(GuestsSection.isPlausibleEmail("@channel"))
        XCTAssertFalse(GuestsSection.isPlausibleEmail("@email"))
        XCTAssertFalse(GuestsSection.isPlausibleEmail("name@"))
        XCTAssertFalse(GuestsSection.isPlausibleEmail("no-at-sign.com"))
        XCTAssertFalse(GuestsSection.isPlausibleEmail("no-tld@domain"))
        XCTAssertFalse(GuestsSection.isPlausibleEmail("two@@at.com"))
        XCTAssertFalse(GuestsSection.isPlausibleEmail("with space@example.com"))
        XCTAssertFalse(GuestsSection.isPlausibleEmail("name@domain .com"))
    }

    func testRejectsEmpty() {
        XCTAssertFalse(GuestsSection.isPlausibleEmail(""))
    }

    func testGuestParticipantProfileUsesDisplayNameInitials() {
        let profile = GuestParticipantProfile.make(
            email: "ada@example.com",
            displayName: "Ada Lovelace"
        )

        XCTAssertEqual(profile.title, "Ada Lovelace")
        XCTAssertEqual(profile.subtitle, "ada@example.com")
        XCTAssertEqual(profile.initials, "AL")
    }

    func testGuestParticipantProfileFallsBackToEmailLocalPart() {
        let profile = GuestParticipantProfile.make(email: "first.last+team@example.com")

        XCTAssertEqual(profile.title, "First Last Team")
        XCTAssertEqual(profile.subtitle, "first.last+team@example.com")
        XCTAssertEqual(profile.initials, "FL")
    }
}
