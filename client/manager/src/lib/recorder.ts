/**
 * （和 client/app/src/lib/recorder.ts 是同一份，改动时两边一起改。）
 *
 * 录一段话，交给服务端转文字（MiniMax asr-1.0）。
 *
 * 识别服务认 m4a / mp3 / wav / ogg，不认 webm：
 *  - iPhone Safari 和新版 Chrome 的 MediaRecorder 能录 audio/mp4（AAC），体积小，优先用；
 *  - Firefox 录 ogg/opus；
 *  - 都不行（只会录 webm 的浏览器）就自己从麦克风取样，编码成 16kHz 单声道 WAV。
 */

export interface Recording {
  blob: Blob;
  extension: string;
  seconds: number;
}

export const MAX_SECONDS = 300;

const recorderTypes: [string, string][] = [
  ["audio/mp4", "m4a"],
  ["audio/ogg;codecs=opus", "ogg"],
  ["audio/ogg", "ogg"],
];

export function canRecord(): boolean {
  return typeof navigator !== "undefined" && !!navigator.mediaDevices?.getUserMedia;
}

export class VoiceRecorder {
  private stream: MediaStream | null = null;
  private context: AudioContext | null = null;
  private recorder: MediaRecorder | null = null;
  private chunks: Blob[] = [];
  private samples: Float32Array[] = [];
  private extension = "wav";
  private startedAt = 0;
  private levelTimer = 0;

  /** 开始录音；onLevel 按 0-1 回调音量，用来画音量条。没给麦克风权限会抛错。 */
  async start(onLevel?: (level: number) => void) {
    this.stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
    const AudioContextClass = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    this.context = new AudioContextClass();
    await this.context.resume();
    const source = this.context.createMediaStreamSource(this.stream);

    const analyser = this.context.createAnalyser();
    analyser.fftSize = 512;
    source.connect(analyser);
    const buffer = new Uint8Array(analyser.fftSize);
    this.levelTimer = window.setInterval(() => {
      analyser.getByteTimeDomainData(buffer);
      let peak = 0;
      for (let i = 0; i < buffer.length; i++) peak = Math.max(peak, Math.abs(buffer[i] - 128));
      onLevel?.(Math.min(1, peak / 64));
    }, 80);

    const type = typeof MediaRecorder !== "undefined" ? recorderTypes.find(([mime]) => MediaRecorder.isTypeSupported(mime)) : undefined;
    if (type) {
      this.extension = type[1];
      this.recorder = new MediaRecorder(this.stream, { mimeType: type[0] });
      this.chunks = [];
      this.recorder.ondataavailable = (event) => event.data.size && this.chunks.push(event.data);
      this.recorder.start(1000);
    } else {
      // ScriptProcessor 虽然过时，但 iOS 和各家浏览器都还支持，够录一段话用
      this.extension = "wav";
      const processor = this.context.createScriptProcessor(4096, 1, 1);
      this.samples = [];
      processor.onaudioprocess = (event) => this.samples.push(new Float32Array(event.inputBuffer.getChannelData(0)));
      source.connect(processor);
      processor.connect(this.context.destination);
    }
    this.startedAt = Date.now();
  }

  elapsed(): number {
    return this.startedAt ? (Date.now() - this.startedAt) / 1000 : 0;
  }

  async stop(): Promise<Recording> {
    const seconds = this.elapsed();
    let blob: Blob;
    if (this.recorder) {
      const recorder = this.recorder;
      blob = await new Promise<Blob>((resolve) => {
        recorder.onstop = () => resolve(new Blob(this.chunks, { type: recorder.mimeType }));
        recorder.stop();
      });
    } else {
      blob = encodeWAV(this.samples, this.context?.sampleRate ?? 48000);
    }
    this.release();
    return { blob, extension: this.extension, seconds };
  }

  cancel() {
    try {
      this.recorder?.stop();
    } catch {
      /* 已经停了 */
    }
    this.release();
  }

  private release() {
    window.clearInterval(this.levelTimer);
    this.stream?.getTracks().forEach((track) => track.stop());
    void this.context?.close().catch(() => {});
    this.stream = null;
    this.context = null;
    this.recorder = null;
    this.startedAt = 0;
  }
}

/** 降采样到 16kHz、16 位单声道 WAV：一分钟约 1.9MB。 */
function encodeWAV(chunks: Float32Array[], sourceRate: number): Blob {
  const targetRate = 16000;
  const total = chunks.reduce((sum, chunk) => sum + chunk.length, 0);
  const merged = new Float32Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    merged.set(chunk, offset);
    offset += chunk.length;
  }
  const ratio = sourceRate / targetRate;
  const length = Math.floor(merged.length / ratio);
  const view = new DataView(new ArrayBuffer(44 + length * 2));
  const text = (at: number, value: string) => {
    for (let i = 0; i < value.length; i++) view.setUint8(at + i, value.charCodeAt(i));
  };
  text(0, "RIFF");
  view.setUint32(4, 36 + length * 2, true);
  text(8, "WAVEfmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, targetRate, true);
  view.setUint32(28, targetRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  text(36, "data");
  view.setUint32(40, length * 2, true);
  for (let i = 0; i < length; i++) {
    // 取区间平均，比直接抽点少些混叠
    const start = Math.floor(i * ratio);
    const end = Math.min(merged.length, Math.floor((i + 1) * ratio));
    let sum = 0;
    for (let j = start; j < end; j++) sum += merged[j];
    const sample = Math.max(-1, Math.min(1, sum / Math.max(1, end - start)));
    view.setInt16(44 + i * 2, sample < 0 ? sample * 0x8000 : sample * 0x7fff, true);
  }
  return new Blob([view], { type: "audio/wav" });
}
