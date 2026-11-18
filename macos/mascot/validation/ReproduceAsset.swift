// compile together with the original A1 authoring source; emit only B to a new path
import Foundation
import SceneKit
@main struct ReproduceAsset {
    static func main() throws {
        // A1 exports after the size-study renders. SceneKit lazily materializes
        // primitive meshes during rendering, so reproduce that sequence too.
        let stage = Stage(directions[1])
        for (open, blink, tilt, concern): (CGFloat, CGFloat, CGFloat, CGFloat) in [(0,0,0,0),(0.85,0,0,0),(0,1,0,0),(0.12,0,-0.055,-0.10),(0,0,0.05,0.7)] {
            stage.character.pose(open: open, blink: blink, tilt: tilt, concern: concern)
            _ = stage.image()
            if open == 0.85 || (open == 0 && blink == 0 && concern == 0) { _ = stage.image(quarter: true) }
        }
        stage.character.pose(open: 0, blink: 0); stage.character.root.eulerAngles.y = 0
        for pixels in [16,24,32,48,64,96] { _ = stage.image(size: pixels) }
        let counts = try exportUSD(stage.character, URL(fileURLWithPath: CommandLine.arguments[1]))
        print(counts)
        if CommandLine.arguments.count > 2 {
            let archived = try SCNScene(url: URL(fileURLWithPath: CommandLine.arguments[2]))
            for name in ["Head_Pivot", "Jaw_Pivot", "Eye_Left", "Eye_Right", "Tongue", "Mouth_Interior"] {
                guard archived.rootNode.childNode(withName: name, recursively: true) != nil else { throw NSError(domain: "SceneArchiveMissingNode", code: 1) }
            }
            print("Original editable SCN archive loaded; head, jaw, both eyes and mouth parts present")
        }
    }
}
