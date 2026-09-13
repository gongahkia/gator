#include "core/SecretRedactor.h"

#include <QRegularExpression>

namespace hcb {
namespace {

constexpr QStringView kRedactedValue = u"[redacted]";

const QRegularExpression& sensitiveKeyPattern() {
  static const QRegularExpression pattern(
      QStringLiteral(
          R"((?:access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|mcp[_-]?token|bearer[_-]?token|api[_-]?key|password|credential|secret|token|authorization))"),
      QRegularExpression::CaseInsensitiveOption);
  return pattern;
}

const QRegularExpression& secretAssignmentPattern() {
  static const QRegularExpression pattern(
      QStringLiteral(
          R"(\b([A-Za-z0-9_-]*(?:access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|mcp[_-]?token|bearer[_-]?token|api[_-]?key|password|credential|secret|token|authorization)[A-Za-z0-9_-]*)\b\s*([:=])\s*(?!\[redacted\])(?:"[^"]*"|'[^']*'|[^"',\s)}\]]+))"),
      QRegularExpression::CaseInsensitiveOption);
  return pattern;
}

const QRegularExpression& jsonSecretPattern() {
  static const QRegularExpression pattern(
      QStringLiteral(
          R"((['"])(access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|mcp[_-]?token|bearer[_-]?token|api[_-]?key|password|credential|secret|token|authorization)\1\s*:\s*(['"])(?:(?!\3).)*\3)"),
      QRegularExpression::CaseInsensitiveOption);
  return pattern;
}

const QRegularExpression& bearerPattern() {
  static const QRegularExpression pattern(QStringLiteral(R"(\bBearer\s+[A-Za-z0-9._~+/=-]+)"),
                                          QRegularExpression::CaseInsensitiveOption);
  return pattern;
}

const QRegularExpression& authorizationHeaderPattern() {
  static const QRegularExpression pattern(
      QStringLiteral(
          R"(\b((?:Proxy-)?Authorization)\s*:\s*(?:Basic|Bearer)\s+[A-Za-z0-9._~+/=-]+)"),
      QRegularExpression::CaseInsensitiveOption);
  return pattern;
}

const QRegularExpression& oauthQueryPattern() {
  static const QRegularExpression pattern(
      QStringLiteral(R"(\b(code|code_verifier|codeVerifier|state)=([^&\s]+))"),
      QRegularExpression::CaseInsensitiveOption);
  return pattern;
}

const QRegularExpression& newlinePattern() {
  static const QRegularExpression pattern(QStringLiteral(R"([\r\n]+)"));
  return pattern;
}

QString replaceAssignments(const QString& input) {
  QString output;
  qsizetype previousEnd = 0;
  QRegularExpressionMatchIterator matches = secretAssignmentPattern().globalMatch(input);
  while (matches.hasNext()) {
    const QRegularExpressionMatch match = matches.next();
    output.append(input.mid(previousEnd, match.capturedStart() - previousEnd));
    output.append(match.capturedView(1));
    output.append(match.capturedView(2));
    output.append(kRedactedValue);
    previousEnd = match.capturedEnd();
  }
  output.append(input.mid(previousEnd));
  return output;
}

QString replaceJsonSecrets(const QString& input) {
  QString output;
  qsizetype previousEnd = 0;
  QRegularExpressionMatchIterator matches = jsonSecretPattern().globalMatch(input);
  while (matches.hasNext()) {
    const QRegularExpressionMatch match = matches.next();
    const QStringView quote = match.capturedView(1);
    output.append(input.mid(previousEnd, match.capturedStart() - previousEnd));
    output.append(quote);
    output.append(match.capturedView(2));
    output.append(quote);
    output.append(QStringLiteral(": "));
    output.append(match.capturedView(3));
    output.append(kRedactedValue);
    output.append(match.capturedView(3));
    previousEnd = match.capturedEnd();
  }
  output.append(input.mid(previousEnd));
  return output;
}

QString replaceOAuthQueryParameters(const QString& input) {
  QString output;
  qsizetype previousEnd = 0;
  QRegularExpressionMatchIterator matches = oauthQueryPattern().globalMatch(input);
  while (matches.hasNext()) {
    const QRegularExpressionMatch match = matches.next();
    output.append(input.mid(previousEnd, match.capturedStart() - previousEnd));
    output.append(match.capturedView(1));
    output.append(u'=');
    output.append(kRedactedValue);
    previousEnd = match.capturedEnd();
  }
  output.append(input.mid(previousEnd));
  return output;
}

QString replaceAuthorizationHeaders(const QString& input) {
  QString output;
  qsizetype previousEnd = 0;
  QRegularExpressionMatchIterator matches = authorizationHeaderPattern().globalMatch(input);
  while (matches.hasNext()) {
    const QRegularExpressionMatch match = matches.next();
    output.append(input.mid(previousEnd, match.capturedStart() - previousEnd));
    output.append(match.capturedView(1));
    output.append(QStringLiteral(": "));
    output.append(kRedactedValue);
    previousEnd = match.capturedEnd();
  }
  output.append(input.mid(previousEnd));
  return output;
}

} // namespace

QString SecretRedactor::redactText(QStringView value, qsizetype maximumLength) {
  QString redacted = value.toString();
  redacted = replaceAuthorizationHeaders(redacted);
  redacted.replace(bearerPattern(), QStringLiteral("Bearer [redacted]"));
  redacted = replaceJsonSecrets(redacted);
  redacted = replaceAssignments(redacted);
  redacted = replaceOAuthQueryParameters(redacted);
  redacted.replace(newlinePattern(), QStringLiteral(" "));
  return redacted.trimmed().left(qMax<qsizetype>(maximumLength, 0));
}

QString SecretRedactor::redactKey(QStringView key) {
  return isSensitiveKey(key) ? kRedactedValue.toString() : redactText(key, 120);
}

bool SecretRedactor::isSensitiveKey(QStringView key) {
  return sensitiveKeyPattern().match(key.toString()).hasMatch();
}

} // namespace hcb
