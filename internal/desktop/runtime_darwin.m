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

int gator_desktop_front_window(uint32_t *window_id, int *pid, double *x, double *y, double *width, double *height, char *bundle_id, int bundle_size, char *title, int title_size) {
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
      CGRect bounds = CGRectZero;
      NSDictionary *boundsDictionary = window[(id)kCGWindowBounds];
      if (boundsDictionary == nil || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)boundsDictionary, &bounds) || bounds.size.width <= 0 || bounds.size.height <= 0) continue;
      if (window_id) *window_id = (uint32_t)[number unsignedIntValue];
      if (pid) *pid = [ownerPID intValue];
      if (x) *x = bounds.origin.x;
      if (y) *y = bounds.origin.y;
      if (width) *width = bounds.size.width;
      if (height) *height = bounds.size.height;
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

int gator_desktop_double_click(double x, double y) {
  CGPoint point = CGPointMake(x, y);
  CGEventRef down1 = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
  CGEventRef up1 = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
  CGEventRef down2 = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
  CGEventRef up2 = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
  if (down1 == NULL || up1 == NULL || down2 == NULL || up2 == NULL) {
    if (down1) CFRelease(down1); if (up1) CFRelease(up1); if (down2) CFRelease(down2); if (up2) CFRelease(up2);
    return 0;
  }
  CGEventSetIntegerValueField(down1, kCGMouseEventClickState, 1);
  CGEventSetIntegerValueField(up1, kCGMouseEventClickState, 1);
  CGEventSetIntegerValueField(down2, kCGMouseEventClickState, 2);
  CGEventSetIntegerValueField(up2, kCGMouseEventClickState, 2);
  CGEventPost(kCGHIDEventTap, down1); CGEventPost(kCGHIDEventTap, up1);
  CGEventPost(kCGHIDEventTap, down2); CGEventPost(kCGHIDEventTap, up2);
  CFRelease(down1); CFRelease(up1); CFRelease(down2); CFRelease(up2);
  return 1;
}

int gator_desktop_drag(const double *xs, const double *ys, int count) {
  if (xs == NULL || ys == NULL || count < 2 || count > 128) return 0;
  CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, CGPointMake(xs[0], ys[0]), kCGMouseButtonLeft);
  if (down == NULL) return 0;
  CGEventPost(kCGHIDEventTap, down);
  CFRelease(down);
  int completed = 1;
  int last = 0;
  for (int index = 1; index < count; index++) {
    CGEventRef drag = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDragged, CGPointMake(xs[index], ys[index]), kCGMouseButtonLeft);
    if (drag == NULL) { completed = 0; break; }
    CGEventPost(kCGHIDEventTap, drag);
    CFRelease(drag);
    last = index;
  }
  CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, CGPointMake(xs[last], ys[last]), kCGMouseButtonLeft);
  if (up == NULL) return 0;
  CGEventPost(kCGHIDEventTap, up);
  CFRelease(up);
  return completed;
}

int gator_desktop_move(double x, double y) {
  CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, CGPointMake(x, y), kCGMouseButtonLeft);
  if (move == NULL) return 0;
  CGEventPost(kCGHIDEventTap, move);
  CFRelease(move);
  return 1;
}

int gator_desktop_scroll(double x, double y, int delta_x, int delta_y) {
  if (!gator_desktop_move(x, y)) return 0;
  CGEventRef scroll = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, delta_y, delta_x);
  if (scroll == NULL) return 0;
  CGEventPost(kCGHIDEventTap, scroll);
  CFRelease(scroll);
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
