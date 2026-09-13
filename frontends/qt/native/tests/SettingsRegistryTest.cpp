#include <QtTest>

#include "core/SettingsRegistry.h"

class SettingsRegistryTest final : public QObject {
  Q_OBJECT

private slots:
  void readsRegisteredDefaults();
  void writesAndResetsRegisteredValues();
  void supportsCustomValueTypes();
  void rejectsUnknownAndConflictingKeys();
};

void SettingsRegistryTest::readsRegisteredDefaults() {
  hcb::SettingsRegistry settings;
  const hcb::SettingsKey<bool> animationsEnabled{u"appearance/animations-enabled", true};
  const hcb::SettingsKey<QString> timeZone{u"calendar/default-time-zone", QStringLiteral("UTC")};

  QVERIFY(settings.registerKey(animationsEnabled) == hcb::SettingsRegistrationResult::Registered);
  QVERIFY(settings.registerKey(timeZone) == hcb::SettingsRegistrationResult::Registered);

  const std::optional<bool> animationsValue = settings.value(animationsEnabled);
  const std::optional<QString> timeZoneValue = settings.value(timeZone);
  QVERIFY(animationsValue == std::optional<bool>{true});
  QVERIFY(timeZoneValue == std::optional<QString>{QStringLiteral("UTC")});
}

void SettingsRegistryTest::writesAndResetsRegisteredValues() {
  hcb::SettingsRegistry settings;
  const hcb::SettingsKey<std::int64_t> notificationLeadMinutes{u"notifications/lead-minutes", 10};
  QVERIFY(settings.registerKey(notificationLeadMinutes) ==
          hcb::SettingsRegistrationResult::Registered);

  QVERIFY(settings.set(notificationLeadMinutes, 20) == hcb::SettingsWriteResult::Changed);
  QVERIFY(settings.set(notificationLeadMinutes, 20) == hcb::SettingsWriteResult::Unchanged);
  const std::optional<std::int64_t> updatedValue = settings.value(notificationLeadMinutes);
  QVERIFY(updatedValue == std::optional<std::int64_t>{20});

  QVERIFY(settings.reset(notificationLeadMinutes) == hcb::SettingsWriteResult::Changed);
  QVERIFY(settings.reset(notificationLeadMinutes) == hcb::SettingsWriteResult::Unchanged);
  const std::optional<std::int64_t> resetValue = settings.value(notificationLeadMinutes);
  QVERIFY(resetValue == std::optional<std::int64_t>{10});
}

void SettingsRegistryTest::supportsCustomValueTypes() {
  enum class SyncMode : std::uint8_t {
    Manual,
    Balanced
  };

  hcb::SettingsRegistry settings;
  const hcb::SettingsKey<SyncMode> syncMode{u"sync/mode", SyncMode::Balanced};
  QVERIFY(settings.registerKey(syncMode) == hcb::SettingsRegistrationResult::Registered);
  QVERIFY(settings.set(syncMode, SyncMode::Manual) == hcb::SettingsWriteResult::Changed);
  QVERIFY(settings.value(syncMode) == std::optional<SyncMode>{SyncMode::Manual});
}

void SettingsRegistryTest::rejectsUnknownAndConflictingKeys() {
  hcb::SettingsRegistry settings;
  const hcb::SettingsKey<bool> notificationsEnabled{u"notifications/enabled", false};
  const hcb::SettingsKey<QString> conflictingType{u"notifications/enabled", QStringLiteral("no")};
  const hcb::SettingsKey<bool> conflictingDefault{u"notifications/enabled", true};
  const hcb::SettingsKey<bool> unknownKey{u"notifications/unknown", false};

  QVERIFY(settings.registerKey(notificationsEnabled) ==
          hcb::SettingsRegistrationResult::Registered);
  QVERIFY(settings.registerKey(notificationsEnabled) ==
          hcb::SettingsRegistrationResult::AlreadyRegistered);
  QVERIFY(settings.registerKey(conflictingType) ==
          hcb::SettingsRegistrationResult::ValueTypeMismatch);
  QVERIFY(settings.registerKey(conflictingDefault) ==
          hcb::SettingsRegistrationResult::DefaultValueMismatch);
  QVERIFY(settings.set(conflictingType, QStringLiteral("yes")) ==
          hcb::SettingsWriteResult::ValueTypeMismatch);
  QVERIFY(settings.set(unknownKey, true) == hcb::SettingsWriteResult::UnknownKey);
  QVERIFY(!settings.value(conflictingType).has_value());
  QVERIFY(!settings.value(unknownKey).has_value());
}

QTEST_MAIN(SettingsRegistryTest)
#include "SettingsRegistryTest.moc"
