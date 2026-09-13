#include "app/NativeReminderNotifier.h"

#import <UserNotifications/UserNotifications.h>

#include <QMetaObject>
#include <QPointer>

static NSString* const HCBReminderCategory = @"hcb.calendar.reminder";
static NSString* const HCBReminderSnoozeAction = @"hcb.calendar.reminder.snooze";
static NSString* const HCBReminderDismissAction = @"hcb.calendar.reminder.dismiss";
static NSString* const HCBReminderIdentifierKey = @"hcbReminderIdentifier";

@interface HCBReminderDelegate : NSObject <UNUserNotificationCenterDelegate>
@property(nonatomic, assign) hcb::NativeReminderNotifier* owner;
@end

@implementation HCBReminderDelegate

- (void)userNotificationCenter:(UNUserNotificationCenter*)center
 didReceiveNotificationResponse:(UNNotificationResponse*)response
          withCompletionHandler:(void (^)(void))completionHandler {
  NSString* identifier = response.notification.request.content.userInfo[HCBReminderIdentifierKey];
  hcb::NativeReminderNotifier* owner = self.owner;
  if (owner != nullptr && identifier != nil) {
    const QString reminderIdentifier = QString::fromUtf8(identifier.UTF8String);
    hcb::ReminderAction action = hcb::ReminderAction::Dismiss;
    if ([response.actionIdentifier isEqualToString:HCBReminderSnoozeAction]) {
      action = hcb::ReminderAction::SnoozeTenMinutes;
    }
    QPointer<hcb::NativeReminderNotifier> notifier(owner);
    QMetaObject::invokeMethod(owner, [notifier, reminderIdentifier, action] {
      if (notifier != nullptr) {
        notifier->handleAction(reminderIdentifier, action);
      }
    }, Qt::QueuedConnection);
  }
  completionHandler();
}

- (void)userNotificationCenter:(UNUserNotificationCenter*)center
       willPresentNotification:(UNNotification*)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions options))completionHandler {
  completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList |
                    UNNotificationPresentationOptionSound);
}

@end

namespace hcb {

class NativeReminderNotifier::NativeReminderNotifierPrivate final {
public:
  HCBReminderDelegate* __strong delegate{nil};
};

NativeReminderNotifier::NativeReminderNotifier(QObject* parent) : QObject(parent) {
  state_ = new NativeReminderNotifierPrivate;
  state_->delegate = [[HCBReminderDelegate alloc] init];
  state_->delegate.owner = this;
  UNUserNotificationCenter* center = [UNUserNotificationCenter currentNotificationCenter];
  [center setDelegate:state_->delegate];
  UNNotificationAction* snooze = [UNNotificationAction actionWithIdentifier:HCBReminderSnoozeAction
                                                                        title:@"Snooze 10 minutes"
                                                                      options:UNNotificationActionOptionNone];
  UNNotificationAction* dismiss = [UNNotificationAction actionWithIdentifier:HCBReminderDismissAction
                                                                         title:@"Dismiss"
                                                                       options:UNNotificationActionOptionDestructive];
  UNNotificationCategory* category = [UNNotificationCategory categoryWithIdentifier:HCBReminderCategory
                                                                              actions:@[snooze, dismiss]
                                                                    intentIdentifiers:@[]
                                                                              options:UNNotificationCategoryOptionCustomDismissAction];
  [center setNotificationCategories:[NSSet setWithObject:category]];
}

NativeReminderNotifier::~NativeReminderNotifier() {
  if (state_ != nullptr) {
    state_->delegate.owner = nullptr;
  }
  delete state_;
  state_ = nullptr;
}

void NativeReminderNotifier::requestAuthorization() {
  UNUserNotificationCenter* center = [UNUserNotificationCenter currentNotificationCenter];
  QPointer<NativeReminderNotifier> notifier(this);
  [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionBadge |
                                          UNAuthorizationOptionSound)
                        completionHandler:[notifier](BOOL granted, NSError* error) {
    const QString message = error != nil
                                ? QStringLiteral("Calendar reminder permission failed")
                                : granted ? QStringLiteral("Calendar reminders are enabled")
                                          : QStringLiteral("Calendar reminders are disabled");
    if (notifier != nullptr) {
      QMetaObject::invokeMethod(notifier, [notifier, message] {
        if (notifier != nullptr) {
          emit notifier->statusChanged(message);
        }
      }, Qt::QueuedConnection);
    }
  }];
}

void NativeReminderNotifier::schedule(NativeReminderNotification notification) {
  if (notification.identifier.isEmpty() || notification.title.trimmed().isEmpty() ||
      notification.body.trimmed().isEmpty() || !notification.deliverAt.isValid()) {
    return;
  }
  const QDateTime local = notification.deliverAt.toLocalTime();
  NSDateComponents* components = [[NSDateComponents alloc] init];
  components.year = local.date().year();
  components.month = local.date().month();
  components.day = local.date().day();
  components.hour = local.time().hour();
  components.minute = local.time().minute();
  components.second = local.time().second();
  UNMutableNotificationContent* content = [[UNMutableNotificationContent alloc] init];
  content.title = [NSString stringWithUTF8String:notification.title.toUtf8().constData()];
  content.body = [NSString stringWithUTF8String:notification.body.toUtf8().constData()];
  content.sound = [UNNotificationSound defaultSound];
  content.categoryIdentifier = HCBReminderCategory;
  NSString* identifier = [NSString stringWithUTF8String:notification.identifier.toUtf8().constData()];
  content.userInfo = @{HCBReminderIdentifierKey : identifier};
  UNCalendarNotificationTrigger* trigger =
      [UNCalendarNotificationTrigger triggerWithDateMatchingComponents:components repeats:NO];
  UNNotificationRequest* request = [UNNotificationRequest requestWithIdentifier:identifier
                                                                          content:content
                                                                          trigger:trigger];
  QPointer<NativeReminderNotifier> notifier(this);
  [[UNUserNotificationCenter currentNotificationCenter]
      addNotificationRequest:request
         withCompletionHandler:[notifier](NSError* error) {
           if (error != nil && notifier != nullptr) {
             QMetaObject::invokeMethod(notifier, [notifier] {
               if (notifier != nullptr) {
                 emit notifier->statusChanged(QStringLiteral("Calendar reminder could not be scheduled"));
               }
             }, Qt::QueuedConnection);
           }
         }];
}

void NativeReminderNotifier::cancel(QString identifier) {
  if (!identifier.startsWith(QStringLiteral("hcb.reminder."))) {
    return;
  }
  [[UNUserNotificationCenter currentNotificationCenter]
      removePendingNotificationRequestsWithIdentifiers:@[[NSString stringWithUTF8String:identifier.toUtf8().constData()]]];
  [[UNUserNotificationCenter currentNotificationCenter]
      removeDeliveredNotificationsWithIdentifiers:@[[NSString stringWithUTF8String:identifier.toUtf8().constData()]]];
}

void NativeReminderNotifier::handleAction(QString identifier, ReminderAction action) {
  emit actionRequested(std::move(identifier), action);
}

} // namespace hcb
