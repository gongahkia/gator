#pragma once

#include <QObject>
#include <QHash>
#include <QVariantList>

namespace hcb {

class ScheduledTaskDateIndex final : public QObject {
  Q_OBJECT
  Q_PROPERTY(int revision READ revision NOTIFY changed)

public:
  explicit ScheduledTaskDateIndex(QObject* parent = nullptr);

  [[nodiscard]] int revision() const;
  void setTasks(QVariantList tasks);

  Q_INVOKABLE QVariantList tasksForDate(const QString& date) const;
  Q_INVOKABLE QVariantList tasksForRange(const QString& firstDate, const QString& lastDate) const;

signals:
  void changed();

private:
  QVariantList tasks_;
  QHash<QString, QVariantList> tasksByDate_;
  int revision_{0};
};

} // namespace hcb
