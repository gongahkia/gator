import { ElectronNotificationScheduler } from "../electronNotificationScheduler";

export class WindowsNotificationScheduler extends ElectronNotificationScheduler {
  constructor(currentPlatform: NodeJS.Platform | string = process.platform) {
    super("win32", currentPlatform);
  }
}
