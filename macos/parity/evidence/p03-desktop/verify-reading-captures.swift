// Headless, read-only OCR validation of the actual delivered window captures.
import Foundation
import Vision
import ImageIO
var evidence: [[String: Any]] = []
for path in CommandLine.arguments.dropFirst() {
    let request = VNRecognizeTextRequest()
    request.recognitionLevel = .accurate
    request.usesLanguageCorrection = false
    try VNImageRequestHandler(url: URL(fileURLWithPath: path)).perform([request])
    let matches = (request.results ?? []).compactMap { observation -> [String: Any]? in
        guard let text = observation.topCandidates(1).first?.string,
              text.trimmingCharacters(in: .whitespacesAndNewlines) == "Retained note 0005-15" else { return nil }
        let b = observation.boundingBox
        return ["text": text, "top": Double(1 - b.maxY), "height": Double(b.height), "left": Double(b.minX)]
    }.sorted { ($0["top"] as! Double) < ($1["top"] as! Double) }
    let messageMatches = matches.filter { ($0["top"] as! Double) > 0.20 && ($0["top"] as! Double) < 0.65 }
    guard messageMatches.count == 1, let heading = messageMatches.first,
          let top = heading["top"] as? Double,
          top > 0.20, top < 0.65 else {
        throw NSError(domain: "ReadingCapture", code: 1, userInfo: [NSLocalizedDescriptionKey: "The message heading was not uniquely recognized in the transcript region in \(path): \(matches)"])
    }
    evidence.append(["path": path, "matches": matches, "messageHeading": heading])
}
guard evidence.count == 2,
      abs((evidence[0]["messageHeading"] as! [String: Any])["top"] as! Double - ((evidence[1]["messageHeading"] as! [String: Any])["top"] as! Double)) < 0.06 else {
    throw NSError(domain: "ReadingCapture", code: 2, userInfo: [NSLocalizedDescriptionKey: "The heading position changed beyond the observed row allowance."])
}
let data = try JSONSerialization.data(withJSONObject: ["result": "PASS", "captures": evidence], options: [.prettyPrinted, .sortedKeys])
print(String(decoding: data, as: UTF8.self))
