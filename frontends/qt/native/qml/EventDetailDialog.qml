import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Event"
    primaryText: "Edit event"
    secondaryText: "Close"
    destructiveText: "Delete event"
    property var event: ({})
    property var calendarSourceModel: null
    signal editRequested(var event)
    signal deleteRequested(string eventId, string title, string recurrenceRule,
                           string recurringRemoteId, string originalStartAt)
    signal externalLinkRequested(string url)

    function openForEvent(value) {
        event = value || ({})
        open()
    }

    function scheduleLabel() {
        if (event.allDay === true) return "All day"
        const start = new Date(event.startAt)
        const end = new Date(event.endAt)
        if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime())) return "Time unavailable"
        return Qt.locale().toString(start, "ddd, d MMM · HH:mm") + "–" + Qt.locale().toString(end, "HH:mm")
    }

    function calendarTitle() {
        if (calendarSourceModel !== null && typeof calendarSourceModel.calendarTitle === "function") {
            const value = calendarSourceModel.calendarTitle(event.calendarId || "")
            if (value.length > 0) return value
        }
        return "Calendar"
    }

    function attendeeCount() {
        try { return JSON.parse(event.attendeeEmailsJson || "[]").length } catch (error) { return 0 }
    }

    onPrimaryAction: editRequested(event)

    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.spacingMedium

        ExternalLinkText {
            Layout.fillWidth: true
            plainText: event.title || "Untitled event"
            font.pixelSize: Theme.titleFontSize
            font.bold: true
            onLinkRequested: url => root.externalLinkRequested(url)
        }
        Label { text: root.calendarTitle(); color: Theme.textSecondary }
        Label { text: root.scheduleLabel(); color: Theme.textSecondary }
        ExternalLinkText {
            Layout.fillWidth: true
            visible: event.location && event.location.length > 0
            plainText: event.location || ""
            color: Theme.textSecondary
            onLinkRequested: url => root.externalLinkRequested(url)
        }
        Label { visible: event.recurrenceRule && event.recurrenceRule.length > 0; text: "Repeats"; color: Theme.textSecondary }
        Label { visible: root.attendeeCount() > 0; text: root.attendeeCount() + " guest" + (root.attendeeCount() === 1 ? "" : "s"); color: Theme.textSecondary }
        Label { visible: event.conferenceJson && event.conferenceJson.length > 2; text: "Google Meet attached"; color: Theme.textSecondary }
        Label { visible: event.attachmentsJson && event.attachmentsJson.length > 2; text: "Attachments added"; color: Theme.textSecondary }

        Label { visible: event.description && event.description.length > 0; text: "Description"; font.bold: true }
        ExternalLinkText {
            Layout.fillWidth: true
            visible: event.description && event.description.length > 0
            plainText: event.description || ""
            onLinkRequested: url => root.externalLinkRequested(url)
        }
    }

    onDestructiveAction: root.deleteRequested(event.id || "", event.title || "", event.recurrenceRule || "",
                                               event.recurringRemoteId || "", event.originalStartAt || "")
}
