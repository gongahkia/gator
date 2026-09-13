#pragma once

#include <QObject>
#include <QString>
#include <QStringList>
#include <QUrl>

#include <functional>
#include <optional>
#include <vector>

namespace hcb {

enum class DeepLinkDestination : unsigned char {
  Tasks,
  Calendar,
  Notes,
  Settings,
};

struct DeepLink final {
  DeepLinkDestination destination;
  QString entityId;
};

class DeepLinkAdapter final {
public:
  [[nodiscard]] static std::optional<DeepLink> parse(const QUrl& url);
  [[nodiscard]] static std::vector<DeepLink> parseLaunchArguments(const QStringList& arguments);
  [[nodiscard]] static QString pageName(DeepLinkDestination destination);
};

class DeepLinkDispatcher final {
public:
  using Handler = std::function<void(const DeepLink& link)>;

  void setHandler(Handler handler);
  void handle(DeepLink link);
  [[nodiscard]] bool handle(const QUrl& url);

private:
  Handler handler_;
  std::vector<DeepLink> pending_;
};

class DeepLinkFileOpenEventFilter final : public QObject {
public:
  explicit DeepLinkFileOpenEventFilter(DeepLinkDispatcher& dispatcher, QObject* parent = nullptr);

protected:
  bool eventFilter(QObject* watched, QEvent* event) override;

private:
  DeepLinkDispatcher& dispatcher_;
};

} // namespace hcb
