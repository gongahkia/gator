import { ElectronNotificationScheduler } from "../electronNotificationScheduler";

export class NotificationScheduler extends ElectronNotificationScheduler {
  constructor(currentPlatform: NodeJS.Platform | string = process.platform) {
    super("darwin", currentPlatform);
  }
}
