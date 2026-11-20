import AppKit
// headless identity validation host: no window, menu, Go process or activation.
let application = NSApplication.shared
application.setActivationPolicy(.prohibited)
application.run()
