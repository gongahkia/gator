#include "core/UnifiedLocalSearchRanker.h"

#include <algorithm>

namespace hcb {
namespace {

constexpr int kExactPhraseScore = 1'000;
constexpr int kPrefixPhraseScore = 800;
constexpr int kContainsPhraseScore = 500;
constexpr int kExactTokenScore = 100;
constexpr int kPrefixTokenScore = 70;
constexpr int kContainsTokenScore = 30;
constexpr int kTitleWeight = 10;
constexpr int kDetailWeight = 4;

[[nodiscard]] QString normalized(QString value) { return value.trimmed().toCaseFolded(); }

[[nodiscard]] QStringList queryTokens(const QString& query) {
  return normalized(query).split(u' ', Qt::SkipEmptyParts);
}

[[nodiscard]] int phraseScore(const QString& field, const QString& phrase) {
  if (phrase.isEmpty() || field.isEmpty()) {
    return 0;
  }
  if (field == phrase) {
    return kExactPhraseScore;
  }
  if (field.startsWith(phrase)) {
    return kPrefixPhraseScore;
  }
  return field.contains(phrase) ? kContainsPhraseScore : 0;
}

[[nodiscard]] int tokenScore(const QString& field, const QString& token) {
  if (field.isEmpty() || token.isEmpty()) {
    return 0;
  }
  if (field == token) {
    return kExactTokenScore;
  }
  if (field.startsWith(token)) {
    return kPrefixTokenScore;
  }
  return field.contains(token) ? kContainsTokenScore : 0;
}

[[nodiscard]] int fieldScore(const QString& title,
                             const QString& detail,
                             const QString& phrase,
                             const QStringList& tokens) {
  const int phraseMatch =
      phraseScore(title, phrase) * kTitleWeight + phraseScore(detail, phrase) * kDetailWeight;
  int tokenMatches = 0;
  for (const QString& token : tokens) {
    const int score =
        tokenScore(title, token) * kTitleWeight + tokenScore(detail, token) * kDetailWeight;
    if (score == 0) {
      return -1;
    }
    tokenMatches += score;
  }
  return phraseMatch + tokenMatches;
}

[[nodiscard]] bool comesBefore(const LocalSearchRankedResult& left,
                               const LocalSearchRankedResult& right) {
  if (left.score != right.score) {
    return left.score > right.score;
  }
  if (left.resource != right.resource) {
    return left.resource < right.resource;
  }
  const int title = QString::compare(left.title, right.title, Qt::CaseInsensitive);
  return title != 0 ? title < 0 : left.id < right.id;
}

} // namespace

QList<LocalSearchRankedResult> UnifiedLocalSearchRanker::rank(
    QString query, QList<LocalSearchCandidate> candidates, int limit) const {
  const QString phrase = normalized(std::move(query));
  const QStringList tokens = queryTokens(phrase);
  QList<LocalSearchRankedResult> results;
  results.reserve(candidates.size());
  for (const LocalSearchCandidate& candidate : candidates) {
    const QString id = candidate.id.trimmed();
    const QString title = candidate.title.trimmed();
    if (id.isEmpty() || title.isEmpty()) {
      continue;
    }
    const int score = fieldScore(normalized(title), normalized(candidate.detail), phrase, tokens);
    if (score < 0) {
      continue;
    }
    results.append({.resource = candidate.resource,
                    .id = id,
                    .title = title,
                    .detail = candidate.detail,
                    .scheduledAt = candidate.scheduledAt,
                    .score = score,
                    .isUndatedTask = candidate.isUndatedTask});
  }
  std::sort(results.begin(), results.end(), comesBefore);
  if (limit > 0 && results.size() > limit) {
    results.resize(limit);
  }
  return results;
}

} // namespace hcb
