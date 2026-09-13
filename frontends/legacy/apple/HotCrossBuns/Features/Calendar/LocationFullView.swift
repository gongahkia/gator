import SwiftUI
import MapKit
import CoreLocation
import WebKit
import AppKit

// Expanded-map sheet presented from LocationMapPreview. Uses the Google Maps
// Embed API iframe when GOOGLE_MAPS_EMBED_API_KEY is configured; falls back
// to a large MapKit view when the key is absent so builds without Maps setup
// still render a map (never a broken iframe).
//
// Editing:
//  - When `isEditable` is true the header surfaces a text field bound to the
//    caller's location string. A 500ms debounce re-geocodes and re-loads the
//    Google embed so address edits reflect live.
//  - When false the header is a title-only label (detail-view read-only path).
//
// External hand-off:
//  - "Apple Maps" opens MKMapItem.openInMaps (native handoff).
//  - "Google Maps" opens maps.google.com/search?query=… in the default browser.
struct LocationFullView: View {
    @Environment(\.dismiss) private var dismiss
    @Binding var locationText: String
    let isEditable: Bool

    @State private var coordinate: CLLocationCoordinate2D?
    @State private var resolvedLabel: String = ""
    @State private var isGeocoding = false
    @State private var debouncedText: String

    init(locationText: Binding<String>, isEditable: Bool) {
        self._locationText = locationText
        self.isEditable = isEditable
        self._debouncedText = State(initialValue: locationText.wrappedValue)
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                header
                Divider()
                mapContent
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .navigationTitle("Location")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
                ToolbarItemGroup {
                    Button {
                        openInAppleMaps()
                    } label: {
                        Label("Apple Maps", systemImage: "map")
                    }
                    .disabled(coordinate == nil)
                    Button {
                        openInGoogleMaps()
                    } label: {
                        Label("Google Maps", systemImage: "safari")
                    }
                    .disabled(debouncedText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
        }
        .hcbScaledFrame(minWidth: 720, idealWidth: 920, minHeight: 560, idealHeight: 720)
        .onChange(of: locationText) { _, newValue in
            // Debounce user typing so we don't thrash the geocoder / iframe
            // load on every keystroke.
            Task {
                try? await Task.sleep(for: .milliseconds(500))
                if newValue == locationText {
                    debouncedText = newValue
                }
            }
        }
        .task(id: debouncedText) { await geocode() }
    }

    // MARK: - header

    @ViewBuilder
    private var header: some View {
        HStack(alignment: .center, spacing: 10) {
            Image(systemName: "mappin.and.ellipse")
                .foregroundStyle(AppColor.ember)
            if isEditable {
                TextField("Location", text: $locationText)
                    .textFieldStyle(.roundedBorder)
                    .font(.system(.body, design: .rounded))
            } else {
                Text(locationText.isEmpty ? "No location" : locationText)
                    .font(.system(.body, design: .rounded, weight: .medium))
                    .foregroundStyle(AppColor.ink)
                    .lineLimit(2)
                Spacer(minLength: 0)
            }
            if isGeocoding {
                ProgressView().controlSize(.small)
            }
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 12)
    }

    // MARK: - map content

    @ViewBuilder
    private var mapContent: some View {
        let trimmed = debouncedText.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty {
            emptyState
        } else if let url = GoogleMapsConfig.embedURL(for: trimmed) {
            // Preferred path: Google Maps Embed iframe.
            GoogleMapsEmbed(url: url)
        } else if let coord = coordinate {
            // Fallback: large MapKit view. Same pin + label the preview shows
            // but at full sheet size. Matches the "never a broken iframe"
            // invariant when the Maps Embed key isn't configured.
            Map(initialPosition: .region(region(for: coord))) {
                Marker(resolvedLabel.isEmpty ? trimmed : resolvedLabel, coordinate: coord)
                    .tint(AppColor.ember)
            }
            .overlay(alignment: .bottomLeading) {
                Text("Google Maps key not configured — showing Apple Maps.")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .hcbScaledPadding(.horizontal, 8)
                    .hcbScaledPadding(.vertical, 4)
                    .background(Capsule().fill(.thinMaterial))
                    .hcbScaledPadding(12)
            }
        } else if isGeocoding {
            ProgressView("Looking up location…")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            ContentUnavailableView(
                "Couldn't find that location",
                systemImage: "mappin.slash",
                description: Text("Try a more specific address.")
            )
        }
    }

    @ViewBuilder
    private var emptyState: some View {
        ContentUnavailableView(
            "No location set",
            systemImage: "mappin.slash",
            description: Text(isEditable ? "Type an address above to see it on the map." : "This event has no location.")
        )
    }

    // MARK: - geocode

    private func geocode() async {
        let trimmed = debouncedText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            coordinate = nil
            resolvedLabel = ""
            return
        }
        isGeocoding = true
        defer { isGeocoding = false }
        let geocoder = CLGeocoder()
        do {
            let placemarks = try await geocoder.geocodeAddressString(trimmed)
            if let first = placemarks.first, let location = first.location {
                coordinate = location.coordinate
                resolvedLabel = [first.name, first.locality, first.administrativeArea]
                    .compactMap { $0 }
                    .joined(separator: ", ")
            } else {
                coordinate = nil
            }
        } catch {
            coordinate = nil
        }
    }

    private func region(for coord: CLLocationCoordinate2D) -> MKCoordinateRegion {
        MKCoordinateRegion(
            center: coord,
            span: MKCoordinateSpan(latitudeDelta: 0.02, longitudeDelta: 0.02)
        )
    }

    // MARK: - external handoff

    private func openInAppleMaps() {
        guard let coord = coordinate else { return }
        let item = MKMapItem(placemark: MKPlacemark(coordinate: coord))
        item.name = locationText.isEmpty ? "Location" : locationText
        item.openInMaps()
    }

    private func openInGoogleMaps() {
        guard let url = GoogleMapsConfig.webSearchURL(for: locationText) else { return }
        NSWorkspace.shared.open(url)
    }
}

// WKWebView host for the Google Maps Embed iframe URL. Background set to clear
// so the sheet's material shines through if the page is slow to paint.
private struct GoogleMapsEmbed: NSViewRepresentable {
    let url: URL

    func makeNSView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        let view = WKWebView(frame: .zero, configuration: config)
        view.setValue(false, forKey: "drawsBackground")
        view.load(URLRequest(url: url))
        return view
    }

    func updateNSView(_ view: WKWebView, context: Context) {
        // Only reload when the URL actually changes — avoids flicker on
        // harmless SwiftUI re-renders.
        if view.url?.absoluteString != url.absoluteString {
            view.load(URLRequest(url: url))
        }
    }
}
