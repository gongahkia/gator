pragma Singleton

import QtQuick

QtObject {
    property int appearanceMode: 0 // 0 system, 1 light, 2 dark
    property int visualDensity: 1 // 0 compact, 1 standard, 2 comfortable
    property int paletteMode: 0 // 0 system, 1 violet, 2 blue, 3 green, 4 rose, 5 amber
    property string accentColor: ""
    property string fontFamily: ""
    property int fontScale: 1 // 0 small, 1 standard, 2 large, 3 extra large
    readonly property var systemPalette: SystemPalette {
        colorGroup: SystemPalette.Active
    }

    readonly property bool forceDark: appearanceMode === 2
    readonly property bool forceLight: appearanceMode === 1
    readonly property bool systemIsDark: systemPalette.window.hslLightness < 0.5
    readonly property bool darkPalette: forceDark || (!forceLight && systemIsDark)
    readonly property bool usesPalettePreset: paletteMode !== 0
    readonly property color presetBackground: {
        if (darkPalette) {
            return paletteMode === 2 ? "#0d1117" : paletteMode === 3 ? "#101510"
                 : paletteMode === 4 ? "#1a1015" : paletteMode === 5 ? "#1b150b" : "#1d1b20"
        }
        return paletteMode === 2 ? "#f8faff" : paletteMode === 3 ? "#f6fbf5"
             : paletteMode === 4 ? "#fff8fa" : paletteMode === 5 ? "#fffbeb" : "#fffbfe"
    }
    readonly property color presetSurface: {
        if (darkPalette) {
            return paletteMode === 2 ? "#161b22" : paletteMode === 3 ? "#18201a"
                 : paletteMode === 4 ? "#24161d" : paletteMode === 5 ? "#271e0f" : "#242126"
        }
        return "#ffffff"
    }
    readonly property color presetAccent: darkPalette
        ? (paletteMode === 2 ? "#58a6ff" : paletteMode === 3 ? "#56d364"
           : paletteMode === 4 ? "#ff7eb6" : paletteMode === 5 ? "#f2cc60" : "#d0bcff")
        : (paletteMode === 2 ? "#0969da" : paletteMode === 3 ? "#1a7f37"
           : paletteMode === 4 ? "#bf3989" : paletteMode === 5 ? "#b45309" : "#6750a4")
    readonly property color background: usesPalettePreset ? presetBackground
                                                     : forceDark ? "#1d1b20"
                                                     : forceLight ? "#fffbfe" : systemPalette.window
    readonly property color surface: usesPalettePreset ? presetSurface
                                               : forceDark ? "#242126"
                                               : forceLight ? "#ffffff" : systemPalette.base
    readonly property color textPrimary: usesPalettePreset || forceDark
                                         ? (darkPalette ? "#e6e1e5" : "#1d1b20")
                                         : forceLight ? "#1d1b20" : systemPalette.windowText
    readonly property color textSecondary: usesPalettePreset || forceDark
                                           ? (darkPalette ? "#cac4d0" : "#625b71")
                                           : forceLight ? "#625b71" : systemPalette.placeholderText
    readonly property color accent: accentColor.length === 7 ? accentColor
                                        : usesPalettePreset ? presetAccent
                                        : forceDark ? "#d0bcff" : forceLight ? "#6750a4"
                                        : systemPalette.highlight
    function linearSrgb(channel) {
        return channel <= 0.04045 ? channel / 12.92 : Math.pow((channel + 0.055) / 1.055, 2.4)
    }
    readonly property color onAccent: {
        const luminance = 0.2126 * linearSrgb(accent.r) + 0.7152 * linearSrgb(accent.g) +
                          0.0722 * linearSrgb(accent.b)
        return luminance > 0.179 ? "#1d1b20" : "#ffffff"
    }
    readonly property color destructive: "#b3261e"
    readonly property color calendarFallback: darkPalette ? "#8ab4f8" : "#0b57d0"
    readonly property int navigationWidth: 220
    readonly property real densityScale: visualDensity === 0 ? 0.82 : visualDensity === 2 ? 1.18 : 1
    readonly property real fontScaleFactor: fontScale === 0 ? 0.9 : fontScale === 2 ? 1.12
                                                                  : fontScale === 3 ? 1.25 : 1
    readonly property int spacingSmall: Math.round(8 * densityScale)
    readonly property int spacingMedium: Math.round(12 * densityScale)
    readonly property int spacingLarge: Math.round(16 * densityScale)
    readonly property int titleFontSize: Math.round(20 * densityScale * fontScaleFactor)
    readonly property int labelFontSize: Math.round(16 * densityScale * fontScaleFactor)
    readonly property int bodyFontSize: Math.round(14 * densityScale * fontScaleFactor)
    readonly property int timelineHourHeight: Math.round(48 * densityScale)

    function calendarColor(colorId, fallback) {
        const colors = {
            "1": "#7986cb", "2": "#33b679", "3": "#8e24aa", "4": "#e67c73",
            "5": "#f6bf26", "6": "#f4511e", "7": "#039be5", "8": "#616161",
            "9": "#3f51b5", "10": "#0b8043", "11": "#d50000"
        }
        return colors[colorId] || fallback || calendarFallback
    }
}
