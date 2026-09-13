#pragma once

#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QNetworkRequest>

#include <algorithm>
#include <utility>

namespace hcb::test {

struct MockNetworkResponse final {
  int status{200};
  QByteArray body;
  QNetworkReply::NetworkError error{QNetworkReply::NoError};
  QList<QPair<QByteArray, QByteArray>> headers;
};

class MockNetworkReply final : public QNetworkReply {
public:
  MockNetworkReply(MockNetworkResponse response, QObject* parent) : QNetworkReply(parent) {
    setAttribute(QNetworkRequest::HttpStatusCodeAttribute, response.status);
    for (const auto& [name, value] : response.headers) {
      setRawHeader(name, value);
    }
    if (response.error != QNetworkReply::NoError) {
      setError(response.error, QStringLiteral("mock network error"));
    }
    body_ = std::move(response.body);
    open(QIODevice::ReadOnly | QIODevice::Unbuffered);
    QMetaObject::invokeMethod(
        this,
        [this] {
          emit readyRead();
          emit finished();
        },
        Qt::QueuedConnection);
  }

  void abort() override {}

  [[nodiscard]] qint64 bytesAvailable() const override {
    return body_.size() - offset_ + QNetworkReply::bytesAvailable();
  }

  [[nodiscard]] bool isSequential() const override { return true; }

protected:
  qint64 readData(char* data, qint64 maximumSize) override {
    if (offset_ >= body_.size()) {
      return -1;
    }
    const qint64 count = std::min(maximumSize, body_.size() - offset_);
    std::copy_n(body_.constData() + offset_, count, data);
    offset_ += count;
    return count;
  }

private:
  QByteArray body_;
  qsizetype offset_{0};
};

struct CapturedNetworkRequest final {
  QNetworkAccessManager::Operation operation;
  QNetworkRequest request;
  QByteArray body;
};

class MockNetworkAccessManager final : public QNetworkAccessManager {
public:
  using QNetworkAccessManager::QNetworkAccessManager;

  void enqueue(MockNetworkResponse response) { responses_.append(std::move(response)); }

  [[nodiscard]] const QList<CapturedNetworkRequest>& requests() const { return requests_; }

protected:
  QNetworkReply* createRequest(Operation operation,
                               const QNetworkRequest& request,
                               QIODevice* outgoingData) override {
    requests_.append({.operation = operation,
                      .request = request,
                      .body = outgoingData != nullptr ? outgoingData->readAll() : QByteArray()});
    MockNetworkResponse response;
    if (!responses_.isEmpty()) {
      response = responses_.takeFirst();
    } else {
      response.status = 500;
      response.error = QNetworkReply::UnknownServerError;
    }
    return new MockNetworkReply(std::move(response), this);
  }

private:
  QList<MockNetworkResponse> responses_;
  QList<CapturedNetworkRequest> requests_;
};

} // namespace hcb::test
