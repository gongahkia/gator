#pragma once

#include <QString>
#include <QStringView>

namespace hcb {

class SecretRedactor final {
public:
  [[nodiscard]] static QString redactText(QStringView value, qsizetype maximumLength = 500);
  [[nodiscard]] static QString redactKey(QStringView key);
  [[nodiscard]] static bool isSensitiveKey(QStringView key);
};

} // namespace hcb
