import XCTest

final class HelpLocalizationTests: XCTestCase {
    private static let expectedLocalizedLanguages: Set<String> = [
        "ms",
        "ta",
        "zh-Hans",
        "id",
        "vi",
        "th",
        "ja",
        "ko",
        "zh-Hant",
        "hi"
    ]
    private static let expectedKnownRegions: Set<String> = expectedLocalizedLanguages.union(["Base", "en"])

    func testHelpCopyUsesLocalizedLookups() throws {
        let source = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Help/HelpView.swift"))

        XCTAssertNil(source.range(of: #"HelpSectionData\(\s*title:\s*""#, options: .regularExpression))
        XCTAssertNil(source.range(of: #"\.init\(title:\s*""#, options: .regularExpression))
        XCTAssertNil(source.range(of: #"detail:\s*""#, options: .regularExpression))
        XCTAssertNil(source.range(of: #"ContentUnavailableView\(\s*""#, options: .regularExpression))
    }

    func testHelpLocalizedKeysExistForSupportedLanguages() throws {
        let catalogURL = repoRoot.appending(path: "apps/apple/Resources/Localizable.xcstrings")
        let data = try Data(contentsOf: catalogURL)
        let catalog = try JSONDecoder().decode(StringCatalog.self, from: data)

        let sourceURLs = [
            "apps/apple/HotCrossBuns/Features/Help/HelpView.swift",
            "apps/apple/HotCrossBuns/Features/QuickAdd/NaturalLanguageTaskParser.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/NaturalLanguageEventParser.swift",
            "apps/apple/HotCrossBuns/App/HCBChord.swift",
            "apps/apple/HotCrossBuns/App/HCBDeepLinkRouter.swift"
        ].map { repoRoot.appending(path: $0) }

        let keys = try sourceURLs.reduce(into: Set<String>()) { partial, url in
            let source = try String(contentsOf: url)
            partial.formUnion(Self.localizedKeys(in: source))
        }

        for key in keys {
            let entry = try XCTUnwrap(catalog.strings[key], "Missing catalog key: \(key)")
            let languages = Set(entry.localizations.keys)
            XCTAssertTrue(
                Self.expectedLocalizedLanguages.isSubset(of: languages),
                "Missing localization for \(key): \(Self.expectedLocalizedLanguages.subtracting(languages))"
            )
        }
    }

    func testStringCatalogIncludesAsiaFirstLanguageBatchForEveryLocalizedKey() throws {
        let catalogURL = repoRoot.appending(path: "apps/apple/Resources/Localizable.xcstrings")
        let data = try Data(contentsOf: catalogURL)
        let catalog = try JSONDecoder().decode(StringCatalog.self, from: data)

        for (key, entry) in catalog.strings {
            let languages = Set(entry.localizations.keys)
            XCTAssertTrue(
                Self.expectedLocalizedLanguages.isSubset(of: languages),
                "Missing localization for \(key): \(Self.expectedLocalizedLanguages.subtracting(languages))"
            )
        }
    }

    func testXcodeProjectKnowsSupportedRegions() throws {
        let projectURL = repoRoot.appending(path: "apps/apple/HotCrossBuns.xcodeproj/project.pbxproj")
        let project = try String(contentsOf: projectURL)
        let knownRegions = try XCTUnwrap(Self.knownRegions(in: project))

        for region in Self.expectedKnownRegions {
            XCTAssertTrue(knownRegions.contains(region), "Missing known region: \(region)")
        }
    }

    private static func localizedKeys(in source: String) -> Set<String> {
        var keys = Set<String>()
        for pattern in [
            #"hcbHelpString\("((?:\\.|[^"\\])*)"\)"#,
            #"String\(localized:\s*"((?:\\.|[^"\\])*)"\)"#
        ] {
            let regex = try! NSRegularExpression(pattern: pattern)
            let range = NSRange(source.startIndex..<source.endIndex, in: source)
            for match in regex.matches(in: source, range: range) {
                guard let keyRange = Range(match.range(at: 1), in: source) else { continue }
                var key = String(source[keyRange])
                    .replacingOccurrences(of: #"\""#, with: #"""#)
                    .replacingOccurrences(of: #"\n"#, with: "\n")
                key = key.replacingOccurrences(of: #"\(entry.title)"#, with: "%@")
                keys.insert(key)
            }
        }
        keys.insert("Task quick-add: %@")
        keys.insert("Event quick-add: %@")
        return keys
    }

    private static func knownRegions(in project: String) -> Set<String>? {
        let pattern = #"knownRegions = \(\s*([\s\S]*?)\s*\);"#
        let regex = try! NSRegularExpression(pattern: pattern)
        let range = NSRange(project.startIndex..<project.endIndex, in: project)
        guard
            let match = regex.firstMatch(in: project, range: range),
            let knownRegionsRange = Range(match.range(at: 1), in: project)
        else {
            return nil
        }

        return Set(
            project[knownRegionsRange]
                .split(separator: "\n")
                .map { line in
                    String(line)
                        .trimmingCharacters(in: .whitespacesAndNewlines)
                        .trimmingCharacters(in: CharacterSet(charactersIn: ",\""))
                }
                .filter { !$0.isEmpty }
        )
    }

    private var repoRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}

private struct StringCatalog: Decodable {
    var strings: [String: StringCatalogEntry]
}

private struct StringCatalogEntry: Decodable {
    var localizations: [String: StringCatalogLocalization]
}

private struct StringCatalogLocalization: Decodable {}
