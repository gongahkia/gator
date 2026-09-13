import Foundation

// Resolves the Google Maps Embed API key from the app bundle.
//
// The key is plumbed in via xcconfig → project.yml → Info.plist:
//   apps/apple/Configuration/GoogleOAuth.xcconfig
//     GOOGLE_MAPS_EMBED_API_KEY = AIza...
//   apps/apple/project.yml
//     info.properties.GoogleMapsEmbedAPIKey: $(GOOGLE_MAPS_EMBED_API_KEY)
//
// If the key is absent, `embedAPIKey` returns nil and the caller falls back
// to a large MapKit view — never a broken iframe.
enum GoogleMapsConfig {
    // Info.plist key populated by the xcconfig variable. Kept out of source
    // so rotating the key doesn't require a code change.
    static let infoPlistKey = "GoogleMapsEmbedAPIKey"
    // Placeholder shipped in GoogleOAuth.example.xcconfig — explicitly rejected
    // so an unconfigured clone doesn't load Google with a bogus key and get
    // an OVER_QUERY_LIMIT page.
    private static let placeholder = "your-maps-embed-api-key"

    static var embedAPIKey: String? {
        guard let raw = Bundle.main.object(forInfoDictionaryKey: infoPlistKey) as? String else {
            return nil
        }
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        guard trimmed != placeholder else { return nil }
        // If the xcconfig substitution didn't happen, the value is literally
        // "$(GOOGLE_MAPS_EMBED_API_KEY)" — reject that too.
        guard trimmed.contains("$(") == false else { return nil }
        return trimmed
    }

    // Returns a Maps Embed API URL for the given free-text location, or nil
    // when either the key is missing or the location is blank. Callers should
    // treat nil as "no iframe available — use the MapKit fallback".
    static func embedURL(for locationText: String) -> URL? {
        guard let key = embedAPIKey else { return nil }
        let trimmed = locationText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        var comps = URLComponents(string: "https://www.google.com/maps/embed/v1/place")
        comps?.queryItems = [
            URLQueryItem(name: "key", value: key),
            URLQueryItem(name: "q", value: trimmed)
        ]
        return comps?.url
    }

    // Web URL the "Open in Google Maps" button hands off to the browser. Never
    // requires an API key — just a standard search URL.
    static func webSearchURL(for locationText: String) -> URL? {
        let trimmed = locationText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        var comps = URLComponents(string: "https://www.google.com/maps/search/")
        comps?.queryItems = [URLQueryItem(name: "api", value: "1"), URLQueryItem(name: "query", value: trimmed)]
        return comps?.url
    }
}
