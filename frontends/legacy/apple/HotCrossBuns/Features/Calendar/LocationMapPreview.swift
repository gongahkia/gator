import SwiftUI
import MapKit
import CoreLocation

// Inline map thumbnail next to a location field. Renders a MapKit pin for
// speed (no network, no key) and offers two actions in the top-right corner:
//
//  - "Expand" — opens LocationFullView in a sheet. That sheet loads the
//    Google Maps Embed iframe when the GOOGLE_MAPS_EMBED_API_KEY xcconfig is
//    set; otherwise it shows a larger MapKit fallback.
//  - "Apple Maps" — native handoff via MKMapItem.openInMaps.
//
// `isEditable` governs whether the expand sheet shows a text field or a
// read-only header. The detail view passes a constant binding; the edit sheet
// passes a mutable binding to its `@State var location`.
struct LocationMapPreview: View {
    @Binding var locationText: String
    let isEditable: Bool

    @State private var coordinate: CLLocationCoordinate2D?
    @State private var resolvedLocation: String = ""
    @State private var isGeocoding = false
    @State private var isPresentingFullView = false

    // Binding initialiser — used by editable callers (AddEventSheet).
    init(locationText: Binding<String>, isEditable: Bool = true) {
        self._locationText = locationText
        self.isEditable = isEditable
    }

    // Convenience for read-only callers (EventDetailView).
    init(locationText: String) {
        self._locationText = .constant(locationText)
        self.isEditable = false
    }

    var body: some View {
        Group {
            if let coord = coordinate {
                Button {
                    isPresentingFullView = true
                } label: {
                    Map(initialPosition: .region(region(for: coord))) {
                        Marker(resolvedLocation.isEmpty ? locationText : resolvedLocation, coordinate: coord)
                            .tint(AppColor.ember)
                    }
                    .allowsHitTesting(false)
                }
                .buttonStyle(.plain)
                .frame(height: 140)
                .clipShape(RoundedRectangle(cornerRadius: 10))
                .overlay(
                    RoundedRectangle(cornerRadius: 10)
                        .strokeBorder(AppColor.cardStroke, lineWidth: 0.6)
                )
                .contentShape(RoundedRectangle(cornerRadius: 10))
                .overlay(alignment: .topLeading) {
                    expandChip
                }
                .overlay(alignment: .topTrailing) {
                    mapsChip(coord: coord)
                }
                .accessibilityLabel("Open full map")
                .accessibilityHint(fullMapAccessibilityHint)
            } else if isGeocoding {
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Looking up location…")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .hcbScaledPadding(.vertical, 4)
            }
        }
        .task(id: locationText) {
            await geocode()
        }
        .sheet(isPresented: $isPresentingFullView) {
            LocationFullView(locationText: $locationText, isEditable: isEditable)
        }
    }

    private var fullMapAccessibilityHint: String {
        let label = resolvedLocation.isEmpty ? locationText : resolvedLocation
        return label.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? "Shows the location in a larger map view."
            : "Shows \(label) in a larger map view."
    }

    private var expandChip: some View {
        Button {
            isPresentingFullView = true
        } label: {
            Label("Expand", systemImage: "arrow.up.left.and.arrow.down.right")
                .hcbFont(.caption, weight: .semibold)
                .hcbScaledPadding(.horizontal, 8)
                .hcbScaledPadding(.vertical, 4)
                .background(
                    RoundedRectangle(cornerRadius: 8)
                        .fill(.thinMaterial)
                )
        }
        .buttonStyle(.plain)
        .hcbScaledPadding(6)
        .help("Open full map")
    }

    private func mapsChip(coord: CLLocationCoordinate2D) -> some View {
        Button {
            openInMaps(coordinate: coord, label: locationText)
        } label: {
            Label("Maps", systemImage: "arrow.up.right.square")
                .hcbFont(.caption, weight: .semibold)
                .hcbScaledPadding(.horizontal, 8)
                .hcbScaledPadding(.vertical, 4)
                .background(
                    RoundedRectangle(cornerRadius: 8)
                        .fill(.thinMaterial)
                )
        }
        .buttonStyle(.plain)
        .hcbScaledPadding(6)
        .help("Open in Apple Maps")
    }

    private func region(for coord: CLLocationCoordinate2D) -> MKCoordinateRegion {
        MKCoordinateRegion(
            center: coord,
            span: MKCoordinateSpan(latitudeDelta: 0.02, longitudeDelta: 0.02)
        )
    }

    private func geocode() async {
        let trimmed = locationText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            coordinate = nil
            resolvedLocation = ""
            return
        }
        // Debounce: short delay so each keystroke doesn't fire a geocode.
        try? await Task.sleep(for: .milliseconds(400))
        if Task.isCancelled { return }
        isGeocoding = true
        defer { isGeocoding = false }
        let geocoder = CLGeocoder()
        do {
            let placemarks = try await geocoder.geocodeAddressString(trimmed)
            if let first = placemarks.first, let location = first.location {
                coordinate = location.coordinate
                resolvedLocation = [first.name, first.locality, first.administrativeArea]
                    .compactMap { $0 }
                    .joined(separator: ", ")
            } else {
                coordinate = nil
            }
        } catch {
            coordinate = nil
        }
    }

    private func openInMaps(coordinate: CLLocationCoordinate2D, label: String) {
        let placemark = MKPlacemark(coordinate: coordinate)
        let item = MKMapItem(placemark: placemark)
        item.name = label.isEmpty ? "Location" : label
        item.openInMaps()
    }
}
