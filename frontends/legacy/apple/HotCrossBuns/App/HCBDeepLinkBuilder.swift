import Foundation

enum HCBDeepLinkBuilder {
    static func taskURL(for task: TaskMirror) -> URL {
        taskURL(id: task.id)
    }

    static func taskURL(id: TaskMirror.ID) -> URL {
        resourceURL(host: "task", id: id)
    }

    static func eventURL(for event: CalendarEventMirror) -> URL {
        eventURL(id: event.id)
    }

    static func eventURL(id: CalendarEventMirror.ID) -> URL {
        resourceURL(host: "event", id: id)
    }

    private static func resourceURL(host: String, id: String) -> URL {
        var components = URLComponents()
        components.scheme = HCBDeepLinkRouter.scheme
        components.host = host
        components.percentEncodedPath = "/" + percentEncodedPathSegment(id)

        guard let url = components.url else {
            preconditionFailure("Invalid Hot Cross Buns deep link for \(host)")
        }
        return url
    }

    private static func percentEncodedPathSegment(_ value: String) -> String {
        var allowed = CharacterSet.urlPathAllowed
        allowed.remove(charactersIn: "/?#[]@!$&'()*+,;=%")
        return value.addingPercentEncoding(withAllowedCharacters: allowed) ?? value
    }
}
