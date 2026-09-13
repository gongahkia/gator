import Foundation
import CoreTransferable
import UniformTypeIdentifiers

struct DraggedTask: Codable, Transferable, Equatable, Sendable {
    let taskID: String
    let taskListID: String
    let title: String

    static var transferRepresentation: some TransferRepresentation {
        CodableRepresentation(contentType: .json)
    }
}

struct DraggedEvent: Codable, Transferable, Equatable, Sendable {
    let eventID: String
    let calendarID: String
    let durationMinutes: Int
    let isAllDay: Bool

    static var transferRepresentation: some TransferRepresentation {
        CodableRepresentation(contentType: .json)
    }
}
