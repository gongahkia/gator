import XCTest
@testable import HotCrossBunsMac

final class GoogleMapsConfigTests: XCTestCase {
    // embedURL returns nil without a key — verifies the fallback contract that
    // keeps unconfigured builds on MapKit instead of rendering an OVER_QUERY_LIMIT
    // page. embedAPIKey itself is read from Info.plist; in the test bundle the
    // key isn't populated, so we exercise the nil path here.
    func testEmbedURLNilWhenNoKey() throws {
        // Only valid to run when the test bundle doesn't have the key set —
        // which is the default for local CLI builds. If a developer has set
        // the key in their local xcconfig, the test bundle may still inherit
        // it; skip cleanly in that case instead of false-failing.
        try XCTSkipIf(GoogleMapsConfig.embedAPIKey != nil, "Maps Embed key present in test bundle — skipping nil-path test.")
        XCTAssertNil(GoogleMapsConfig.embedURL(for: "Apple Park"))
    }

    func testEmbedURLNilWhenLocationBlank() {
        // Blank location yields nil regardless of key state — callers should
        // never see a Google embed URL with an empty q= parameter.
        XCTAssertNil(GoogleMapsConfig.embedURL(for: ""))
        XCTAssertNil(GoogleMapsConfig.embedURL(for: "   "))
    }

    func testWebSearchURLEncodesQuery() {
        let url = GoogleMapsConfig.webSearchURL(for: "Apple Park, Cupertino")
        XCTAssertNotNil(url)
        let str = url!.absoluteString
        XCTAssertTrue(str.hasPrefix("https://www.google.com/maps/search/"))
        XCTAssertTrue(str.contains("api=1"))
        // Comma and spaces must be percent-encoded so the URL round-trips
        // correctly through NSWorkspace.open.
        XCTAssertTrue(str.contains("Apple%20Park") || str.contains("Apple+Park") || str.contains("Apple%20Park,%20Cupertino"))
    }

    func testWebSearchURLNilWhenBlank() {
        XCTAssertNil(GoogleMapsConfig.webSearchURL(for: ""))
        XCTAssertNil(GoogleMapsConfig.webSearchURL(for: "   "))
    }
}
