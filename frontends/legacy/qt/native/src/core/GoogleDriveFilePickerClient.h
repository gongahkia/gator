#pragma once

#include "core/GoogleApiError.h"

#include <QList>
#include <QString>

#include <future>
#include <optional>
#include <variant>

namespace hcb {

class GoogleHttpClient;

struct GoogleDriveAttachmentCandidate final {
  QString id;
  QString name;
  QString mimeType;
  QString webViewLink;
  std::optional<QString> iconLink;
};

using GoogleDriveAttachmentCandidatesOrError =
    std::variant<QList<GoogleDriveAttachmentCandidate>, GoogleApiError>;

class GoogleDriveFilePickerClient final {
public:
  explicit GoogleDriveFilePickerClient(GoogleHttpClient& httpClient);

  [[nodiscard]] std::future<GoogleDriveAttachmentCandidatesOrError> search(QString query,
                                                                           QString accessToken);

private:
  GoogleHttpClient& httpClient_;
};

} // namespace hcb
