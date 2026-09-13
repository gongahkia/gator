#include "app/LinuxCredentialAdapter.h"

#include <QCryptographicHash>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonParseError>

#if defined(Q_OS_LINUX)
#include <QDBusArgument>
#include <QDBusConnection>
#include <QDBusInterface>
#include <QDBusMetaType>
#include <QDBusObjectPath>
#include <QDBusPendingReply>
#include <QDBusVariant>
#include <QMap>
#include <QMetaType>
#include <QVariantMap>
#endif

#include <future>
#include <optional>
#include <utility>
#include <variant>

#if defined(Q_OS_LINUX)
namespace hcb {

using SecretServiceAttributes = QMap<QString, QString>;
using SecretServicePaths = QList<QDBusObjectPath>;

struct SecretServiceSecret final {
  QDBusObjectPath session;
  QByteArray parameters;
  QByteArray value;
  QString contentType;
};

QDBusArgument& operator<<(QDBusArgument& argument, const SecretServiceSecret& secret) {
  argument.beginStructure();
  argument << secret.session << secret.parameters << secret.value << secret.contentType;
  argument.endStructure();
  return argument;
}

const QDBusArgument& operator>>(const QDBusArgument& argument, SecretServiceSecret& secret) {
  argument.beginStructure();
  argument >> secret.session >> secret.parameters >> secret.value >> secret.contentType;
  argument.endStructure();
  return argument;
}

} // namespace hcb

Q_DECLARE_METATYPE(hcb::SecretServiceAttributes)
Q_DECLARE_METATYPE(hcb::SecretServicePaths)
Q_DECLARE_METATYPE(hcb::SecretServiceSecret)
#endif

namespace hcb {
namespace {

constexpr qsizetype kMaximumAccountIdLength = 256;
constexpr qsizetype kMaximumTokenLength = 8'192;

[[nodiscard]] AppError configurationError(QString message) {
  return AppError(AppErrorCode::Configuration, std::move(message));
}

template <typename Result> [[nodiscard]] std::future<Result> readyFuture(Result result) {
  std::promise<Result> completion;
  std::future<Result> future = completion.get_future();
  completion.set_value(std::move(result));
  return future;
}

template <typename Result, typename Operation>
[[nodiscard]] std::future<Result> runOnWorker(Operation operation, Result fallback) {
  try {
    return std::async(std::launch::async,
                      [operation = std::move(operation), fallback = std::move(fallback)]() mutable {
                        try {
                          return operation();
                        } catch (...) {
                          return fallback;
                        }
                      });
  } catch (...) {
    return readyFuture(std::move(fallback));
  }
}

[[nodiscard]] bool isValidAccountId(const QString& accountId) {
  return !accountId.isEmpty() && accountId == accountId.trimmed() &&
         accountId.size() <= kMaximumAccountIdLength && !accountId.contains(QChar::Null);
}

[[nodiscard]] bool isValidToken(const QString& token) {
  return !token.isEmpty() && token.size() <= kMaximumTokenLength && !token.contains(QChar::Null);
}

[[nodiscard]] bool isValidCredential(const OAuthStoredCredential& credential) {
  return isValidToken(credential.accessToken) &&
         (!credential.refreshToken.has_value() || isValidToken(*credential.refreshToken));
}

[[nodiscard]] OAuthCredentialReadResult unsupportedRead() {
  return configurationError(QStringLiteral("Linux Secret Service is unavailable on this platform"));
}

[[nodiscard]] OAuthCredentialSaveResult unsupportedSave() {
  return configurationError(QStringLiteral("Linux Secret Service is unavailable on this platform"));
}

[[nodiscard]] OAuthCredentialDeleteResult unsupportedDelete() {
  return configurationError(QStringLiteral("Linux Secret Service is unavailable on this platform"));
}

#if defined(Q_OS_LINUX)

constexpr qsizetype kMaximumCredentialPayloadLength = 70'000;
constexpr char kSecretServiceName[] = "org.freedesktop.secrets";
constexpr char kSecretServicePath[] = "/org/freedesktop/secrets";
constexpr char kSecretServiceInterface[] = "org.freedesktop.Secret.Service";
constexpr char kSecretItemInterface[] = "org.freedesktop.Secret.Item";
constexpr char kSecretCollectionInterface[] = "org.freedesktop.Secret.Collection";
constexpr char kSecretSessionInterface[] = "org.freedesktop.Secret.Session";
constexpr char kSecretApplication[] = "dev.hotcrossbuns.native";
constexpr char kSecretCredentialLabel[] = "Hot Cross Buns OAuth credential";

struct SecretServiceSearchResult final {
  SecretServicePaths unlocked;
  SecretServicePaths locked;
};

using SecretServiceSearch = std::variant<SecretServiceSearchResult, AppError>;
using SecretServiceSessionResult = std::variant<QDBusObjectPath, AppError>;
using SecretServiceCollection = std::variant<QDBusObjectPath, AppError>;

[[nodiscard]] AppError secretServiceError(QString operation) {
  return configurationError(
      QStringLiteral("Linux Secret Service %1 failed").arg(std::move(operation)));
}

template <typename... Values> [[nodiscard]] bool waitForReply(QDBusPendingReply<Values...>& reply) {
  reply.waitForFinished();
  return !reply.isError();
}

void registerSecretServiceTypes() {
  static const QMetaType attributes = qDBusRegisterMetaType<SecretServiceAttributes>();
  static const QMetaType paths = qDBusRegisterMetaType<SecretServicePaths>();
  static const QMetaType secret = qDBusRegisterMetaType<SecretServiceSecret>();
  static_cast<void>(attributes);
  static_cast<void>(paths);
  static_cast<void>(secret);
}

[[nodiscard]] bool isRootPath(const QDBusObjectPath& path) {
  return path.path() == QLatin1String("/");
}

[[nodiscard]] bool isUsablePath(const QDBusObjectPath& path) {
  return !path.path().isEmpty() && !isRootPath(path);
}

[[nodiscard]] SecretServiceAttributes credentialAttributes(const QString& accountId) {
  const QByteArray digest =
      QCryptographicHash::hash(accountId.toUtf8(), QCryptographicHash::Sha256).toHex();
  return {{QStringLiteral("application"), QString::fromLatin1(kSecretApplication)},
          {QStringLiteral("kind"), QStringLiteral("oauth")},
          {QStringLiteral("account"), QString::fromLatin1(digest)}};
}

[[nodiscard]] QByteArray credentialPayload(const OAuthStoredCredential& credential) {
  QJsonObject object;
  object.insert(QStringLiteral("accessToken"), credential.accessToken);
  if (credential.refreshToken.has_value()) {
    object.insert(QStringLiteral("refreshToken"), *credential.refreshToken);
  }
  return QJsonDocument(object).toJson(QJsonDocument::Compact);
}

[[nodiscard]] OAuthCredentialReadResult decodeCredential(const QByteArray& payload) {
  if (payload.isEmpty() || payload.size() > kMaximumCredentialPayloadLength) {
    return configurationError(QStringLiteral("Linux Secret Service credential is invalid"));
  }
  QJsonParseError parseError;
  const QJsonDocument document = QJsonDocument::fromJson(payload, &parseError);
  if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
    return configurationError(QStringLiteral("Linux Secret Service credential is invalid"));
  }
  const QJsonObject object = document.object();
  const QJsonValue accessToken = object.value(QStringLiteral("accessToken"));
  const QJsonValue refreshToken = object.value(QStringLiteral("refreshToken"));
  if (!accessToken.isString() || (!refreshToken.isUndefined() && !refreshToken.isString())) {
    return configurationError(QStringLiteral("Linux Secret Service credential is invalid"));
  }
  OAuthStoredCredential credential{.accessToken = accessToken.toString(),
                                   .refreshToken = std::nullopt};
  if (refreshToken.isString()) {
    credential.refreshToken = refreshToken.toString();
  }
  if (!isValidCredential(credential)) {
    return configurationError(QStringLiteral("Linux Secret Service credential is invalid"));
  }
  return std::optional<OAuthStoredCredential>(std::move(credential));
}

class SecretServiceSessionGuard final {
public:
  SecretServiceSessionGuard(QDBusConnection connection, QDBusObjectPath path)
      : connection_(std::move(connection)), path_(std::move(path)) {}
  SecretServiceSessionGuard(const SecretServiceSessionGuard&) = delete;
  SecretServiceSessionGuard& operator=(const SecretServiceSessionGuard&) = delete;
  ~SecretServiceSessionGuard() {
    QDBusInterface session(QString::fromLatin1(kSecretServiceName),
                           path_.path(),
                           QString::fromLatin1(kSecretSessionInterface),
                           connection_);
    session.setInteractiveAuthorizationAllowed(false);
    QDBusPendingReply<> reply = session.asyncCall(QStringLiteral("Close"));
    reply.waitForFinished();
  }

private:
  QDBusConnection connection_;
  QDBusObjectPath path_;
};

[[nodiscard]] SecretServiceSearch searchCredentials(QDBusInterface& service,
                                                    const QString& accountId) {
  QDBusPendingReply<SecretServicePaths, SecretServicePaths> reply =
      service.asyncCallWithArgumentList(QStringLiteral("SearchItems"),
                                        {QVariant::fromValue(credentialAttributes(accountId))});
  if (!waitForReply(reply)) {
    return secretServiceError(QStringLiteral("search"));
  }
  return SecretServiceSearchResult{.unlocked = reply.argumentAt<0>(),
                                   .locked = reply.argumentAt<1>()};
}

[[nodiscard]] SecretServiceSessionResult openPlainSession(QDBusInterface& service) {
  QDBusPendingReply<QDBusVariant, QDBusObjectPath> reply = service.asyncCallWithArgumentList(
      QStringLiteral("OpenSession"),
      {QVariant(QStringLiteral("plain")), QVariant::fromValue(QDBusVariant(QVariant(QString())))});
  if (!waitForReply(reply)) {
    return secretServiceError(QStringLiteral("session open"));
  }
  const QDBusVariant output = reply.argumentAt<0>();
  const QVariant outputValue = output.variant();
  const QDBusObjectPath session = reply.argumentAt<1>();
  if (outputValue.metaType() != QMetaType::fromType<QString>() ||
      !outputValue.toString().isEmpty() || !isUsablePath(session)) {
    return configurationError(QStringLiteral("Linux Secret Service session is invalid"));
  }
  return session;
}

[[nodiscard]] SecretServiceCollection defaultCollection(QDBusInterface& service) {
  QDBusPendingReply<QDBusObjectPath> reply = service.asyncCallWithArgumentList(
      QStringLiteral("ReadAlias"), {QVariant(QStringLiteral("default"))});
  if (!waitForReply(reply)) {
    return secretServiceError(QStringLiteral("default collection lookup"));
  }
  const QDBusObjectPath collection = reply.value();
  return isUsablePath(collection) ? SecretServiceCollection(collection)
                                  : SecretServiceCollection(configurationError(QStringLiteral(
                                        "Linux Secret Service default collection is unavailable")));
}

[[nodiscard]] SecretServiceSecret secretFor(const QDBusObjectPath& session,
                                            const OAuthStoredCredential& credential) {
  return {.session = session,
          .parameters = {},
          .value = credentialPayload(credential),
          .contentType = QStringLiteral("application/json")};
}

[[nodiscard]] OAuthCredentialReadResult readCredential(const QString& accountId) {
  registerSecretServiceTypes();
  const QDBusConnection connection = QDBusConnection::sessionBus();
  if (!connection.isConnected()) {
    return configurationError(
        QStringLiteral("Linux Secret Service is unavailable for this session"));
  }
  QDBusInterface service(QString::fromLatin1(kSecretServiceName),
                         QString::fromLatin1(kSecretServicePath),
                         QString::fromLatin1(kSecretServiceInterface),
                         connection);
  service.setInteractiveAuthorizationAllowed(false);
  if (!service.isValid()) {
    return configurationError(
        QStringLiteral("Linux Secret Service is unavailable for this session"));
  }
  const SecretServiceSearch search = searchCredentials(service, accountId);
  if (std::holds_alternative<AppError>(search)) {
    return std::get<AppError>(search);
  }
  const SecretServiceSearchResult& matches = std::get<SecretServiceSearchResult>(search);
  if (!matches.locked.isEmpty()) {
    return configurationError(QStringLiteral("Linux Secret Service credential requires unlock"));
  }
  if (matches.unlocked.isEmpty()) {
    return std::optional<OAuthStoredCredential>{};
  }
  if (matches.unlocked.size() != 1 || !isUsablePath(matches.unlocked.front())) {
    return configurationError(QStringLiteral("Linux Secret Service credential is ambiguous"));
  }
  const SecretServiceSessionResult opened = openPlainSession(service);
  if (std::holds_alternative<AppError>(opened)) {
    return std::get<AppError>(opened);
  }
  const QDBusObjectPath session = std::get<QDBusObjectPath>(opened);
  const SecretServiceSessionGuard sessionGuard(connection, session);
  QDBusInterface item(QString::fromLatin1(kSecretServiceName),
                      matches.unlocked.front().path(),
                      QString::fromLatin1(kSecretItemInterface),
                      connection);
  item.setInteractiveAuthorizationAllowed(false);
  QDBusPendingReply<SecretServiceSecret> reply =
      item.asyncCallWithArgumentList(QStringLiteral("GetSecret"), {QVariant::fromValue(session)});
  if (!waitForReply(reply)) {
    return secretServiceError(QStringLiteral("read"));
  }
  const SecretServiceSecret secret = reply.value();
  if (secret.session.path() != session.path() || !secret.parameters.isEmpty() ||
      secret.contentType != QLatin1String("application/json")) {
    return configurationError(QStringLiteral("Linux Secret Service credential is invalid"));
  }
  return decodeCredential(secret.value);
}

[[nodiscard]] OAuthCredentialSaveResult saveCredential(const QString& accountId,
                                                       const OAuthStoredCredential& credential) {
  registerSecretServiceTypes();
  const QByteArray payload = credentialPayload(credential);
  if (payload.isEmpty() || payload.size() > kMaximumCredentialPayloadLength) {
    return configurationError(QStringLiteral("Linux Secret Service credential is too large"));
  }
  const QDBusConnection connection = QDBusConnection::sessionBus();
  if (!connection.isConnected()) {
    return configurationError(
        QStringLiteral("Linux Secret Service is unavailable for this session"));
  }
  QDBusInterface service(QString::fromLatin1(kSecretServiceName),
                         QString::fromLatin1(kSecretServicePath),
                         QString::fromLatin1(kSecretServiceInterface),
                         connection);
  service.setInteractiveAuthorizationAllowed(false);
  if (!service.isValid()) {
    return configurationError(
        QStringLiteral("Linux Secret Service is unavailable for this session"));
  }
  const SecretServiceSearch search = searchCredentials(service, accountId);
  if (std::holds_alternative<AppError>(search)) {
    return std::get<AppError>(search);
  }
  const SecretServiceSearchResult& matches = std::get<SecretServiceSearchResult>(search);
  if (!matches.locked.isEmpty()) {
    return configurationError(QStringLiteral("Linux Secret Service credential requires unlock"));
  }
  if (matches.unlocked.size() > 1 ||
      (matches.unlocked.size() == 1 && !isUsablePath(matches.unlocked.front()))) {
    return configurationError(QStringLiteral("Linux Secret Service credential is ambiguous"));
  }
  const SecretServiceSessionResult opened = openPlainSession(service);
  if (std::holds_alternative<AppError>(opened)) {
    return std::get<AppError>(opened);
  }
  const QDBusObjectPath session = std::get<QDBusObjectPath>(opened);
  const SecretServiceSessionGuard sessionGuard(connection, session);
  const SecretServiceSecret secret = secretFor(session, credential);
  if (matches.unlocked.size() == 1) {
    QDBusInterface item(QString::fromLatin1(kSecretServiceName),
                        matches.unlocked.front().path(),
                        QString::fromLatin1(kSecretItemInterface),
                        connection);
    item.setInteractiveAuthorizationAllowed(false);
    QDBusPendingReply<> reply =
        item.asyncCallWithArgumentList(QStringLiteral("SetSecret"), {QVariant::fromValue(secret)});
    return waitForReply(reply)
               ? OAuthCredentialSaveResult(std::monostate{})
               : OAuthCredentialSaveResult(secretServiceError(QStringLiteral("save")));
  }
  const SecretServiceCollection collection = defaultCollection(service);
  if (std::holds_alternative<AppError>(collection)) {
    return std::get<AppError>(collection);
  }
  QVariantMap properties;
  properties.insert(QStringLiteral("org.freedesktop.Secret.Item.Label"),
                    QString::fromLatin1(kSecretCredentialLabel));
  properties.insert(QStringLiteral("org.freedesktop.Secret.Item.Attributes"),
                    QVariant::fromValue(credentialAttributes(accountId)));
  QDBusInterface collectionInterface(QString::fromLatin1(kSecretServiceName),
                                     std::get<QDBusObjectPath>(collection).path(),
                                     QString::fromLatin1(kSecretCollectionInterface),
                                     connection);
  collectionInterface.setInteractiveAuthorizationAllowed(false);
  QDBusPendingReply<QDBusObjectPath, QDBusObjectPath> reply =
      collectionInterface.asyncCallWithArgumentList(
          QStringLiteral("CreateItem"),
          {QVariant(properties), QVariant::fromValue(secret), QVariant(true)});
  if (!waitForReply(reply)) {
    return secretServiceError(QStringLiteral("save"));
  }
  return isUsablePath(reply.argumentAt<0>()) && isRootPath(reply.argumentAt<1>())
             ? OAuthCredentialSaveResult(std::monostate{})
             : OAuthCredentialSaveResult(configurationError(
                   QStringLiteral("Linux Secret Service credential save requires a prompt")));
}

[[nodiscard]] OAuthCredentialDeleteResult eraseCredential(const QString& accountId) {
  registerSecretServiceTypes();
  const QDBusConnection connection = QDBusConnection::sessionBus();
  if (!connection.isConnected()) {
    return configurationError(
        QStringLiteral("Linux Secret Service is unavailable for this session"));
  }
  QDBusInterface service(QString::fromLatin1(kSecretServiceName),
                         QString::fromLatin1(kSecretServicePath),
                         QString::fromLatin1(kSecretServiceInterface),
                         connection);
  service.setInteractiveAuthorizationAllowed(false);
  if (!service.isValid()) {
    return configurationError(
        QStringLiteral("Linux Secret Service is unavailable for this session"));
  }
  const SecretServiceSearch search = searchCredentials(service, accountId);
  if (std::holds_alternative<AppError>(search)) {
    return std::get<AppError>(search);
  }
  const SecretServiceSearchResult& matches = std::get<SecretServiceSearchResult>(search);
  if (!matches.locked.isEmpty()) {
    return configurationError(QStringLiteral("Linux Secret Service credential requires unlock"));
  }
  if (matches.unlocked.isEmpty()) {
    return std::monostate{};
  }
  if (matches.unlocked.size() != 1 || !isUsablePath(matches.unlocked.front())) {
    return configurationError(QStringLiteral("Linux Secret Service credential is ambiguous"));
  }
  QDBusInterface item(QString::fromLatin1(kSecretServiceName),
                      matches.unlocked.front().path(),
                      QString::fromLatin1(kSecretItemInterface),
                      connection);
  item.setInteractiveAuthorizationAllowed(false);
  QDBusPendingReply<QDBusObjectPath> reply = item.asyncCall(QStringLiteral("Delete"));
  if (!waitForReply(reply)) {
    return secretServiceError(QStringLiteral("erase"));
  }
  return isRootPath(reply.value())
             ? OAuthCredentialDeleteResult(std::monostate{})
             : OAuthCredentialDeleteResult(configurationError(
                   QStringLiteral("Linux Secret Service credential erase requires a prompt")));
}

#endif

} // namespace

std::future<OAuthCredentialReadResult> LinuxCredentialAdapter::read(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(OAuthCredentialReadResult(
        configurationError(QStringLiteral("OAuth credential account identifier is invalid"))));
  }
#if defined(Q_OS_LINUX)
  return runOnWorker<OAuthCredentialReadResult>(
      [accountId = std::move(accountId)] { return readCredential(accountId); }, unsupportedRead());
#else
  static_cast<void>(accountId);
  return readyFuture(unsupportedRead());
#endif
}

std::future<OAuthCredentialSaveResult>
LinuxCredentialAdapter::save(QString accountId, OAuthStoredCredential credential) {
  if (!isValidAccountId(accountId) || !isValidCredential(credential)) {
    return readyFuture(OAuthCredentialSaveResult(
        configurationError(QStringLiteral("OAuth credential input is invalid"))));
  }
#if defined(Q_OS_LINUX)
  return runOnWorker<OAuthCredentialSaveResult>(
      [accountId = std::move(accountId), credential = std::move(credential)] {
        return saveCredential(accountId, credential);
      },
      unsupportedSave());
#else
  static_cast<void>(accountId);
  static_cast<void>(credential);
  return readyFuture(unsupportedSave());
#endif
}

std::future<OAuthCredentialDeleteResult> LinuxCredentialAdapter::erase(QString accountId) {
  if (!isValidAccountId(accountId)) {
    return readyFuture(OAuthCredentialDeleteResult(
        configurationError(QStringLiteral("OAuth credential account identifier is invalid"))));
  }
#if defined(Q_OS_LINUX)
  return runOnWorker<OAuthCredentialDeleteResult>(
      [accountId = std::move(accountId)] { return eraseCredential(accountId); },
      unsupportedDelete());
#else
  static_cast<void>(accountId);
  return readyFuture(unsupportedDelete());
#endif
}

} // namespace hcb
