import { ElectronNotificationScheduler } from "../electronNotificationScheduler";

export class LinuxNotificationScheduler extends ElectronNotificationScheduler {
  constructor(currentPlatform: NodeJS.Platform | string = process.platform) {
    super("linux", currentPlatform);
  }
}
