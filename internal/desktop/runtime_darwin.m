#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <stdint.h>
#include <string.h>

static void gator_copy_string(NSString *value, char *destination, int size) {
  if (destination == NULL || size < 1) return;
  destination[0] = '\0';
  if (value == nil) return;
  const char *utf8 = [value UTF8String];
  if (utf8 == NULL) return;
  strncpy(destination, utf8, (size_t)size - 1);
  destination[size - 1] = '\0';
}

int gator_desktop_accessibility_trusted(void) {
  return AXIsProcessTrusted() ? 1 : 0;
}

int gator_desktop_front_window(uint32_t *window_id, int *pid, char *bundle_id, int bundle_size, char *title, int title_size) {
  @autoreleasepool {
    CFArrayRef windows = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
    if (windows == NULL) return 0;
    int found = 0;
    for (NSDictionary *window in (__bridge NSArray *)windows) {
      NSNumber *layer = window[(id)kCGWindowLayer];
      NSNumber *number = window[(id)kCGWindowNumber];
      NSNumber *ownerPID = window[(id)kCGWindowOwnerPID];
      if (layer == nil || [layer intValue] != 0 || number == nil || ownerPID == nil) continue;
      NSRunningApplication *application = [NSRunningApplication runningApplicationWithProcessIdentifier:[ownerPID intValue]];
      if (application == nil || application.bundleIdentifier == nil) continue;
      if (window_id) *window_id = (uint32_t)[number unsignedIntValue];
      if (pid) *pid = [ownerPID intValue];
      gator_copy_string(application.bundleIdentifier, bundle_id, bundle_size);
      gator_copy_string(window[(id)kCGWindowName], title, title_size);
      found = 1;
      break;
    }
    CFRelease(windows);
    return found;
  }
}

int gator_desktop_click(double x, double y) {
  CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, CGPointMake(x, y), kCGMouseButtonLeft);
  CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, CGPointMake(x, y), kCGMouseButtonLeft);
  if (down == NULL || up == NULL) { if (down) CFRelease(down); if (up) CFRelease(up); return 0; }
  CGEventPost(kCGHIDEventTap, down);
  CGEventPost(kCGHIDEventTap, up);
  CFRelease(down);
  CFRelease(up);
  return 1;
}

int gator_desktop_type(const char *value) {
  @autoreleasepool {
    if (value == NULL) return 0;
    NSString *text = [NSString stringWithUTF8String:value];
    if (text == nil) return 0;
    NSUInteger length = [text length];
    if (length == 0 || length > 8192) return 0;
    UniChar *characters = calloc(length, sizeof(UniChar));
    if (characters == NULL) return 0;
    [text getCharacters:characters range:NSMakeRange(0, length)];
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, 0, true);
    if (event == NULL) { free(characters); return 0; }
    CGEventKeyboardSetUnicodeString(event, (UniCharCount)length, characters);
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    free(characters);
    return 1;
  }
}

int gator_desktop_press(int keycode) {
  CGEventRef down = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, true);
  CGEventRef up = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, false);
  if (down == NULL || up == NULL) { if (down) CFRelease(down); if (up) CFRelease(up); return 0; }
  CGEventPost(kCGHIDEventTap, down);
  CGEventPost(kCGHIDEventTap, up);
  CFRelease(down);
  CFRelease(up);
  return 1;
}

int gator_desktop_focused_field_sensitive(int pid) {
  AXUIElementRef application = AXUIElementCreateApplication((pid_t)pid);
  if (application == NULL) return -1;
  CFTypeRef focused = NULL;
  AXError error = AXUIElementCopyAttributeValue(application, kAXFocusedUIElementAttribute, &focused);
  CFRelease(application);
  if (error != kAXErrorSuccess || focused == NULL) return 0;
  CFTypeRef subrole = NULL;
  error = AXUIElementCopyAttributeValue((AXUIElementRef)focused, kAXSubroleAttribute, &subrole);
  CFRelease(focused);
  if (error != kAXErrorSuccess || subrole == NULL) return 0;
  int sensitive = CFEqual(subrole, kAXSecureTextFieldSubrole) ? 1 : 0;
  CFRelease(subrole);
  return sensitive;
}
