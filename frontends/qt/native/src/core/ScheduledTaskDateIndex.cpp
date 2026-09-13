#include "core/ScheduledTaskDateIndex.h"

#include <QDate>
#include <QVariantMap>

#include <algorithm>

namespace hcb {

ScheduledTaskDateIndex::ScheduledTaskDateIndex(QObject* parent) : QObject(parent) {}

int ScheduledTaskDateIndex::revision() const { return revision_; }

void ScheduledTaskDateIndex::setTasks(QVariantList tasks) {
  QHash<QString, QVariantList> byDate;
  for (const QVariant& value : tasks) {
    const QVariantMap task = value.toMap();
    const QString date = task.value(QStringLiteral("dueAt")).toString().left(10);
    if (!QDate::fromString(date, Qt::ISODate).isValid()) {
      continue;
    }
    byDate[date].append(value);
  }
  if (tasks_ == tasks && tasksByDate_ == byDate) {
    return;
  }
  tasks_ = std::move(tasks);
  tasksByDate_ = std::move(byDate);
  ++revision_;
  emit changed();
}

QVariantList ScheduledTaskDateIndex::tasksForDate(const QString& date) const {
  const QString normalized = date.left(10);
  return QDate::fromString(normalized, Qt::ISODate).isValid()
             ? tasksByDate_.value(normalized)
             : QVariantList{};
}

QVariantList ScheduledTaskDateIndex::tasksForRange(const QString& firstDate,
                                                    const QString& lastDate) const {
  QDate first = QDate::fromString(firstDate.left(10), Qt::ISODate);
  QDate last = QDate::fromString(lastDate.left(10), Qt::ISODate);
  if (!first.isValid() || !last.isValid()) {
    return {};
  }
  if (first > last) {
    std::swap(first, last);
  }
  QVariantList result;
  for (QDate date = first; date <= last; date = date.addDays(1)) {
    const QVariantList bucket = tasksByDate_.value(date.toString(Qt::ISODate));
    result.append(bucket);
  }
  return result;
}

} // namespace hcb
