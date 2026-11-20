import AppKit
@MainActor final class Field: NSTextField {
    var invalidations = 0
    override func invalidateIntrinsicContentSize() { invalidations += 1; super.invalidateIntrinsicContentSize() }
}
@main struct Probe {
 @MainActor static func main() {
let field = Field(labelWithString: "🐊")
field.font = .systemFont(ofSize: 30)
field.invalidations = 0
for _ in 0..<100 { field.stringValue = "🐊"; field.font = .systemFont(ofSize: 30) }
print("100 unchanged text/font assignments caused \(field.invalidations) intrinsic-size invalidations")

 }
}
