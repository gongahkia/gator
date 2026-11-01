// Browser-only conversion. No ffmpeg, audio file, network request, or transcript
// execution is involved; the result is bounded mono 16 kHz / 16-bit PCM WAV.
export function pcmWAV(channels: Float32Array[]): Uint8Array {
  const frames = channels[0]?.length ?? 0;
  if (!frames || frames > 120 * 16000 || channels.length > 8 || channels.some(channel => channel.length !== frames)) throw new Error("Record between a moment and two minutes of local audio.");
  const bytes = new Uint8Array(44 + frames * 2), view = new DataView(bytes.buffer);
  const text = (offset: number, value: string) => { for (let i = 0; i < value.length; i++) view.setUint8(offset + i, value.charCodeAt(i)); };
  text(0, "RIFF"); view.setUint32(4, bytes.length - 8, true); text(8, "WAVEfmt "); view.setUint32(16, 16, true); view.setUint16(20, 1, true); view.setUint16(22, 1, true); view.setUint32(24, 16000, true); view.setUint32(28, 32000, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true); text(36, "data"); view.setUint32(40, frames * 2, true);
  for (let i = 0; i < frames; i++) {
    let sample = 0;
    for (const channel of channels) { if (!Number.isFinite(channel[i])) throw new Error("The recorded audio contains invalid samples."); sample += channel[i] / channels.length; }
    sample = Math.max(-1, Math.min(1, sample)); view.setInt16(44 + i * 2, Math.round(sample * (sample < 0 ? 32768 : 32767)), true);
  }
  return bytes;
}
export async function localRecording(blob: Blob): Promise<Uint8Array> {
  if (!blob.size || blob.size > 8 * 1024 * 1024 || !globalThis.OfflineAudioContext) throw new Error("Local audio preparation is unavailable; no recording was uploaded.");
  // decodeAudioData resamples to its context rate. An offline context never
  // opens an output device or plays the recorded clip.
  const decoder = new OfflineAudioContext(1, 1, 16000);
  const audio = await decoder.decodeAudioData(await blob.arrayBuffer());
  if (audio.sampleRate !== 16000 || !Number.isFinite(audio.duration) || audio.duration > 120) throw new Error("Local recordings must be at most two minutes at 16 kHz.");
  return pcmWAV(Array.from({ length: audio.numberOfChannels }, (_, i) => audio.getChannelData(i)));
}
