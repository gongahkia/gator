import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var agendaModel: null
    property var calendarVisibility: null
    property var selectedEventIds: []
    property bool selectionMode: false
    property string highlightedEventId: ""
    property int revealAttempts: 0
    property alias eventRows: eventRows
    signal eventSelected(string eventId)
    signal eventSelectionRequested(string eventId, bool selected)
    signal eventEditRequested(string eventId, string calendarId, string title, string startAt,
                              string endAt, bool allDay, string description, string location,
                              string startTimeZone, string colorId, string transparency,
                              string visibility, string attendeeEmailsJson, string remindersJson,
                              bool remindersUseDefault, string recurrenceRule,
                              string recurringRemoteId, string originalStartAt, string eventType,
                              string conferenceJson, string attachmentsJson,
                              string guestPermissionsJson, string statusPropertiesJson)
    signal eventDetailRequested(var event)

    function scheduleLabel(startAt, allDay) {
        if (allDay) return "All day"
        const parsed = new Date(startAt)
        if (!Number.isFinite(parsed.getTime())) return "Time unavailable"
        return Qt.locale().toString(parsed, "ddd, d MMM · HH:mm")
    }

    function eventColor(calendarId, colorId) {
        const fallback = calendarVisibility !== null && typeof calendarVisibility.calendarColor === "function"
                       ? calendarVisibility.calendarColor(calendarId) : Theme.calendarFallback
        return Theme.calendarColor(colorId, fallback)
    }

    function selectEvent(eventId) {
        eventSelected(eventId)
    }

    function revealEvent(eventId) {
        highlightedEventId = eventId
        revealAttempts = 0
        revealTimer.restart()
        highlightTimer.restart()
    }

    function formatAgendaDay(value) {
        const date = new Date(value + "T12:00:00")
        return Number.isFinite(date.getTime()) ? Qt.locale().toString(date, "dddd, d MMMM") : value
    }

    function formatAgendaWeek(value) {
        const date = new Date(value + "T12:00:00")
        if (!Number.isFinite(date.getTime())) return value
        date.setDate(date.getDate() - ((date.getDay() + 6) % 7))
        return Qt.locale().toString(date, "d MMMM yyyy")
    }

    Timer {
        id: revealTimer
        interval: 50
        repeat: true
        onTriggered: {
            root.revealAttempts += 1
            if (root.agendaModel === null || typeof root.agendaModel.rowForEvent !== "function") {
                stop()
                return
            }
            const row = root.agendaModel.rowForEvent(root.highlightedEventId)
            if (row >= 0) {
                eventRows.currentIndex = row
                eventRows.positionViewAtIndex(row, ListView.Center)
                const event = typeof root.agendaModel.eventForId === "function"
                              ? root.agendaModel.eventForId(root.highlightedEventId) : ({})
                if (event.id !== undefined && event.id.length > 0) {
                    root.eventDetailRequested(event)
                }
                stop()
            } else if (root.revealAttempts >= 50) {
                stop()
            }
        }
    }

    Timer {
        id: highlightTimer
        interval: 5000
        repeat: false
        onTriggered: root.highlightedEventId = ""
    }

    function requestEdit(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                         startTimeZone, colorId, transparency, visibility, attendeeEmailsJson,
                         remindersJson, remindersUseDefault, recurrenceRule, recurringRemoteId,
                         originalStartAt, eventType, conferenceJson, attachmentsJson,
                         guestPermissionsJson, statusPropertiesJson) {
        eventEditRequested(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                           startTimeZone, colorId, transparency, visibility, attendeeEmailsJson,
                           remindersJson, remindersUseDefault, recurrenceRule, recurringRemoteId,
                           originalStartAt, eventType, conferenceJson, attachmentsJson,
                           guestPermissionsJson, statusPropertiesJson)
    }

    function requestDetail(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                           startTimeZone, colorId, transparency, visibility, attendeeEmailsJson,
                           remindersJson, remindersUseDefault, recurrenceRule, recurringRemoteId,
                           originalStartAt, eventType, conferenceJson, attachmentsJson,
                           guestPermissionsJson, statusPropertiesJson) {
        eventDetailRequested({id: eventId, calendarId: calendarId, title: title, startAt: startAt,
                              endAt: endAt, allDay: allDay, description: description, location: location,
                              startTimeZone: startTimeZone, colorId: colorId, transparency: transparency,
                              visibility: visibility, attendeeEmailsJson: attendeeEmailsJson,
                              remindersJson: remindersJson, remindersUseDefault: remindersUseDefault,
                              recurrenceRule: recurrenceRule, recurringRemoteId: recurringRemoteId,
                              originalStartAt: originalStartAt, eventType: eventType,
                              conferenceJson: conferenceJson, attachmentsJson: attachmentsJson,
                              guestPermissionsJson: guestPermissionsJson,
                              statusPropertiesJson: statusPropertiesJson})
    }

    function isCalendarVisible(calendarId) {
        return calendarVisibility === null || calendarVisibility.isVisible(calendarId)
    }

    function isEventSelected(eventId) {
        return selectedEventIds.indexOf(eventId) >= 0
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingMedium

        Label {
            text: "Agenda"
            font.pixelSize: Theme.titleFontSize
            Accessible.role: Accessible.Heading
            Accessible.name: text
        }

        ListView {
            id: eventRows
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            model: root.agendaModel
            spacing: Theme.spacingSmall
            section.property: "agendaDay"
            section.criteria: ViewSection.FullString
            section.delegate: Column {
                required property string section
                width: ListView.view.width
                spacing: Theme.spacingSmall

                Rectangle {
                    width: parent.width
                    height: 1
                    color: Theme.textSecondary
                    opacity: 0.35
                }

                Label {
                    text: "Week of " + root.formatAgendaWeek(section) + " · " +
                          root.formatAgendaDay(section)
                    font.bold: true
                    color: Theme.textSecondary
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }
            }

            delegate: CalendarEventButton {
                required property string id
                required property string calendarId
                required property string title
                required property string startAt
                required property string endAt
                required property bool allDay
                required property string description
                required property string location
                required property string startTimeZone
                required property string colorId
                required property string transparency
                required property string visibility
                required property string attendeeEmailsJson
                required property string remindersJson
                required property bool remindersUseDefault
                required property string recurrenceRule
                required property string recurringRemoteId
                required property string originalStartAt
                property string eventType: "default"
                property string conferenceJson: ""
                property string attachmentsJson: "[]"
                property string guestPermissionsJson: "{}"
                property string statusPropertiesJson: "{}"
                width: ListView.view.width
                visible: root.isCalendarVisible(calendarId)
                height: visible ? implicitHeight : 0
                enabled: visible
                eventColor: root.eventColor(calendarId, colorId)
                outlined: root.highlightedEventId === id
                text: title + "\n" + root.scheduleLabel(startAt, allDay) +
                      (location.length > 0 ? " · " + location : "")
                accessibleName: title
                accessibleDescription: root.scheduleLabel(startAt, allDay) + ". Calendar " + calendarId
                onClicked: {
                    if (root.selectionMode) {
                        root.eventSelectionRequested(id, !root.isEventSelected(id))
                    } else {
                        root.selectEvent(id)
                        root.requestDetail(id, calendarId, title, startAt, endAt, allDay, description, location,
                                         startTimeZone, colorId, transparency, visibility,
                                         attendeeEmailsJson, remindersJson, remindersUseDefault,
                                         recurrenceRule, recurringRemoteId, originalStartAt, eventType,
                                         conferenceJson, attachmentsJson, guestPermissionsJson,
                                         statusPropertiesJson)
                    }
                }

                CheckBox {
                    anchors.top: parent.top
                    anchors.right: parent.right
                    anchors.margins: Theme.spacingSmall
                    z: 1
                    visible: root.selectionMode
                    checked: root.isEventSelected(id)
                    Accessible.name: "Select " + title
                    Accessible.description: checked ? "Event selected" : "Event not selected"
                    onClicked: root.eventSelectionRequested(id, checked)
                }
            }
        }

        Label {
            Layout.fillWidth: true
            visible: eventRows.count === 0
            text: "No upcoming events."
            color: Theme.textSecondary
            horizontalAlignment: Text.AlignHCenter
        }
    }
}
