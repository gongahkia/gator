import CoreTransferable
import Foundation

struct SubtaskDragPayload: Codable, Transferable {
    let taskID: String

    static var transferRepresentation: some TransferRepresentation {
        ProxyRepresentation(exporting: { "hcb-subtask:\($0.taskID)" }, importing: { raw in
            let prefix = "hcb-subtask:"
            guard raw.hasPrefix(prefix) else {
                throw CocoaError(.coderReadCorrupt)
            }
            return SubtaskDragPayload(taskID: String(raw.dropFirst(prefix.count)))
        })
    }
}
