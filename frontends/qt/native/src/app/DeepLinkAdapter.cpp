#include "app/DeepLinkAdapter.h"

#include <QCoreApplication>
#include <QEvent>
#include <QFileOpenEvent>
#include <QSet>

#include <utility>

namespace hcb {
namespace {

constexpr qsizetype kMaximumEntityIdLength = 256;

[[nodiscard]] std::optional<QString> entityId(const QUrl& url) {
  const QString path = url.path(QUrl::FullyDecoded);
  if (path.isEmpty() || path == QStringLiteral("/")) {
    return QString{};
  }
  if (!path.startsWith(QLatin1Char('/')) || path.indexOf(QLatin1Char('/'), 1) >= 0) {
    return std::nullopt;
  }
  const QString id = path.mid(1).trimmed();
  if (id.isEmpty() || id.size() > kMaximumEntityIdLength || id == QStringLiteral(".") ||
      id == QStringLiteral("..") || id.contains(QChar::Null)) {
    return std::nullopt;
  }
  return id;
}

[[nodiscard]] std::optional<DeepLinkDestination> destinationForHost(QStringView host) {
  if (host == u"task" || host == u"tasks") {
    return DeepLinkDestination::Tasks;
  }
  if (host == u"event" || host == u"calendar") {
    return DeepLinkDestination::Calendar;
  }
  if (host == u"note" || host == u"notes") {
    return DeepLinkDestination::Notes;
  }
  if (host == u"settings") {
    return DeepLinkDestination::Settings;
  }
  return std::nullopt;
}

} // namespace

std::optional<DeepLink> DeepLinkAdapter::parse(const QUrl& url) {
  if (!url.isValid() ||
      url.scheme().compare(QStringLiteral("hotcrossbuns"), Qt::CaseInsensitive) != 0 ||
      url.hasQuery() || url.hasFragment() || !url.userInfo().isEmpty() || url.port() != -1) {
    return std::nullopt;
  }
  const std::optional<DeepLinkDestination> destination =
      destinationForHost(url.host().toCaseFolded());
  if (!destination.has_value()) {
    return std::nullopt;
  }
  const std::optional<QString> id = entityId(url);
  if (!id.has_value() || (*destination == DeepLinkDestination::Settings && !id->isEmpty())) {
    return std::nullopt;
  }
  return DeepLink{.destination = *destination, .entityId = *id};
}

std::vector<DeepLink> DeepLinkAdapter::parseLaunchArguments(const QStringList& arguments) {
  std::vector<DeepLink> links;
  QSet<QString> seen;
  for (const QString& argument : arguments) {
    const QString candidate = argument.trimmed();
    if (candidate.isEmpty() || seen.contains(candidate)) {
      continue;
    }
    const QUrl url(candidate, QUrl::StrictMode);
    const std::optional<DeepLink> link = parse(url);
    if (!link.has_value()) {
      continue;
    }
    seen.insert(candidate);
    links.push_back(*link);
  }
  return links;
}

QString DeepLinkAdapter::pageName(DeepLinkDestination destination) {
  switch (destination) {
  case DeepLinkDestination::Tasks:
    return QStringLiteral("Tasks");
  case DeepLinkDestination::Calendar:
    return QStringLiteral("Calendar");
  case DeepLinkDestination::Notes:
    return QStringLiteral("Notes");
  case DeepLinkDestination::Settings:
    return QStringLiteral("Settings");
  }
  return {};
}

void DeepLinkDispatcher::setHandler(Handler handler) {
  handler_ = std::move(handler);
  if (!handler_) {
    return;
  }
  for (const DeepLink& link : pending_) {
    handler_(link);
  }
  pending_.clear();
}

bool DeepLinkDispatcher::handle(const QUrl& url) {
  const std::optional<DeepLink> link = DeepLinkAdapter::parse(url);
  if (!link.has_value()) {
    return false;
  }
  handle(*link);
  return true;
}

void DeepLinkDispatcher::handle(DeepLink link) {
  if (!handler_) {
    pending_.push_back(std::move(link));
    return;
  }
  handler_(link);
}

DeepLinkFileOpenEventFilter::DeepLinkFileOpenEventFilter(DeepLinkDispatcher& dispatcher,
                                                         QObject* parent)
    : QObject(parent), dispatcher_(dispatcher) {}

bool DeepLinkFileOpenEventFilter::eventFilter(QObject* watched, QEvent* event) {
  if (watched != QCoreApplication::instance() || event->type() != QEvent::FileOpen) {
    return false;
  }
  const auto* openEvent = static_cast<const QFileOpenEvent*>(event);
  static_cast<void>(dispatcher_.handle(openEvent->url()));
  return false;
}

} // namespace hcb
