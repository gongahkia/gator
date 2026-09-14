#pragma once

#include "core/AppError.h"
#include "core/PythonBridgeClient.h"

#include <QProcess>
#include <QTemporaryDir>

#include <optional>
#include <variant>

namespace hcb {

using PythonBridgeLaunchResult = std::variant<PythonBridgeConnection, AppError>;

class PythonBridgeProcessLauncher final {
public:
  explicit PythonBridgeProcessLauncher(QString executable = {});
  ~PythonBridgeProcessLauncher();
  PythonBridgeProcessLauncher(const PythonBridgeProcessLauncher&) = delete;
  PythonBridgeProcessLauncher& operator=(const PythonBridgeProcessLauncher&) = delete;

  [[nodiscard]] PythonBridgeLaunchResult start();
  [[nodiscard]] QString descriptorPath() const;

private:
  void stop();

  QString executable_;
  QTemporaryDir directory_;
  QProcess process_;
  QString descriptorPath_;
};

} // namespace hcb
