// Shared by host validation and renderer controls. Keep in sync with the Go
// attachment boundary; tests compare the constants across both runtimes.
export const uploadLimits = Object.freeze({ count: 12, file: 8 * 1024 * 1024, image: 4 * 1024 * 1024, total: 16 * 1024 * 1024 });
export const attachmentLimit = name => /\.(?:png|jpe?g|webp)$/i.test(name) ? uploadLimits.image : uploadLimits.file;
export const attachmentLimitsHint = "Up to 12 files · documents 8 MiB each · images 4 MiB each · 16 MiB total";
export const attachmentLimitsError = "Attach up to 12 files, with documents up to 8 MiB, images up to 4 MiB, and 16 MiB total.";
