#pragma once

#include <QHash>
#include <QReadWriteLock>
#include <QString>
#include <QStringView>

#include <any>
#include <concepts>
#include <cstdint>
#include <optional>
#include <typeindex>
#include <type_traits>
#include <utility>

namespace hcb {

template <typename T>
concept SettingsValueType = std::same_as<T, std::remove_cvref_t<T>> && std::copy_constructible<T> &&
                            std::equality_comparable<T>;

template <SettingsValueType T> class SettingsKey final {
public:
  SettingsKey(QStringView name, T defaultValue)
      : name_(name.toString()), defaultValue_(std::move(defaultValue)) {}

  [[nodiscard]] const QString& name() const noexcept { return name_; }
  [[nodiscard]] const T& defaultValue() const noexcept { return defaultValue_; }

private:
  QString name_;
  T defaultValue_;
};

enum class SettingsRegistrationResult : std::uint8_t {
  Registered,
  AlreadyRegistered,
  DefaultValueMismatch,
  ValueTypeMismatch
};

enum class SettingsWriteResult : std::uint8_t {
  Changed,
  Unchanged,
  UnknownKey,
  ValueTypeMismatch
};

class SettingsRegistry final {
public:
  template <SettingsValueType T>
  [[nodiscard]] SettingsRegistrationResult registerKey(const SettingsKey<T>& key) {
    const std::any defaultValue{key.defaultValue()};
    QWriteLocker lock(&lock_);
    const auto existing = settings_.constFind(key.name());
    if (existing == settings_.cend()) {
      settings_.insert(key.name(),
                       RegisteredSetting{typeid(T), defaultValue, defaultValue, &equals<T>});
      return SettingsRegistrationResult::Registered;
    }
    if (existing->type != typeid(T)) {
      return SettingsRegistrationResult::ValueTypeMismatch;
    }
    return existing->equals(existing->defaultValue, defaultValue)
               ? SettingsRegistrationResult::AlreadyRegistered
               : SettingsRegistrationResult::DefaultValueMismatch;
  }

  template <SettingsValueType T>
  [[nodiscard]] std::optional<T> value(const SettingsKey<T>& key) const {
    QReadLocker lock(&lock_);
    const auto existing = settings_.constFind(key.name());
    if (existing == settings_.cend() || existing->type != typeid(T)) {
      return std::nullopt;
    }
    const T* typedValue = std::any_cast<T>(&existing->value);
    return typedValue == nullptr ? std::nullopt : std::optional<T>{*typedValue};
  }

  template <SettingsValueType T>
  [[nodiscard]] SettingsWriteResult set(const SettingsKey<T>& key, std::type_identity_t<T> value) {
    std::any nextValue{std::move(value)};
    QWriteLocker lock(&lock_);
    auto existing = settings_.find(key.name());
    if (existing == settings_.end()) {
      return SettingsWriteResult::UnknownKey;
    }
    if (existing->type != typeid(T)) {
      return SettingsWriteResult::ValueTypeMismatch;
    }
    if (existing->equals(existing->value, nextValue)) {
      return SettingsWriteResult::Unchanged;
    }
    existing->value = std::move(nextValue);
    return SettingsWriteResult::Changed;
  }

  template <SettingsValueType T>
  [[nodiscard]] SettingsWriteResult reset(const SettingsKey<T>& key) {
    QWriteLocker lock(&lock_);
    auto existing = settings_.find(key.name());
    if (existing == settings_.end()) {
      return SettingsWriteResult::UnknownKey;
    }
    if (existing->type != typeid(T)) {
      return SettingsWriteResult::ValueTypeMismatch;
    }
    if (existing->equals(existing->value, existing->defaultValue)) {
      return SettingsWriteResult::Unchanged;
    }
    existing->value = existing->defaultValue;
    return SettingsWriteResult::Changed;
  }

private:
  struct RegisteredSetting final {
    std::type_index type;
    std::any defaultValue;
    std::any value;
    bool (*equals)(const std::any&, const std::any&);
  };

  template <SettingsValueType T> static bool equals(const std::any& left, const std::any& right) {
    return std::any_cast<const T&>(left) == std::any_cast<const T&>(right);
  }

  mutable QReadWriteLock lock_;
  QHash<QString, RegisteredSetting> settings_;
};

} // namespace hcb
