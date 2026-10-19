/**
 * 边说边转文字。识别服务（MiniMax asr-1.0）只认整段录音，没有流式接口，所以在本机按停顿切句：
 *  - 一直从麦克风取样，按音量判断有没有在说话；
 *  - 说完一句（停顿约 0.6 秒）或一句太长（15 秒）就切下来，编成 16kHz WAV 交给 /speech/transcribe；
 *  - 一句话还没说完时，每 2 秒把这句已说的部分先识别一遍，结果当「临时文字」先显示，整句识别完再替换。
 * 每句按顺序拼回去。录音只用来识别，不保存。
 */
import { postForm } from "@/lib/api";
import { encodeWAV } from "@/lib/recorder";

export const DICTATION_MAX_SECONDS = 300;

const SILENCE_TO_CUT = 0.6;
const MIN_SPEECH = 0.35;
const MAX_SEGMENT = 15;
const PRE_ROLL = 0.3;
const INTERIM_EVERY = 2;
const INTERIM_MIN = 1.2;

interface Segment {
  id: number;
  chunks: Float32Array[];
  seconds: number;
  speech: number;
  silence: number;
  lastInterimAt: number;
  interimBusy: boolean;
}

export interface DictationUpdate {
  /** 按顺序拼好的文字（含还在识别中的临时结果） */
  text: string;
  /** 正在说一句、还没出字 */
  listening: boolean;
  /** 还有几句在识别 */
  pending: number;
}

export class LiveTranscriber {
  private stream: MediaStream | null = null;
  private context: AudioContext | null = null;
  private processor: ScriptProcessorNode | null = null;
  private rate = 48000;
  private startedAt = 0;
  private noise = 0.008;
  private segment: Segment | null = null;
  private nextId = 0;
  private finals = new Map<number, string>();
  private interims = new Map<number, string>();
  private inflight = new Set<Promise<void>>();
  private pendingIds = new Set<number>();
  private failed = false;
  private stopped = false;

  constructor(
    private onUpdate: (update: DictationUpdate) => void,
    private onLevel?: (level: number) => void,
    private onError?: (message: string) => void,
  ) {}

  /** 没给麦克风权限会抛错。 */
  async start() {
    this.stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
    const AudioContextClass = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    this.context = new AudioContextClass();
    await this.context.resume();
    this.rate = this.context.sampleRate;
    const source = this.context.createMediaStreamSource(this.stream);
    // ScriptProcessor 虽然过时，但 iOS 和各家浏览器都还支持
    this.processor = this.context.createScriptProcessor(4096, 1, 1);
    this.processor.onaudioprocess = (event) => this.take(new Float32Array(event.inputBuffer.getChannelData(0)));
    source.connect(this.processor);
    this.processor.connect(this.context.destination);
    this.startedAt = Date.now();
    this.segment = this.newSegment();
  }

  elapsed(): number {
    return this.startedAt ? (Date.now() - this.startedAt) / 1000 : 0;
  }

  /** 停止录音，等所有句子识别完，返回最终文字。 */
  async stop(): Promise<string> {
    if (this.stopped) return this.text();
    this.stopped = true;
    const last = this.segment;
    this.segment = null;
    this.release();
    if (last) this.cut(last);
    this.emit();
    while (this.inflight.size) await Promise.allSettled(Array.from(this.inflight));
    return this.text();
  }

  cancel() {
    this.stopped = true;
    this.segment = null;
    this.release();
  }

  private newSegment(): Segment {
    return { id: this.nextId++, chunks: [], seconds: 0, speech: 0, silence: 0, lastInterimAt: 0, interimBusy: false };
  }

  private take(chunk: Float32Array) {
    const segment = this.segment;
    if (!segment || this.stopped) return;
    let sum = 0;
    for (let i = 0; i < chunk.length; i++) sum += chunk[i] * chunk[i];
    const rms = Math.sqrt(sum / chunk.length);
    const duration = chunk.length / this.rate;
    const speaking = rms > Math.max(0.012, this.noise * 2.5);
    // 噪声底只在没说话时慢慢跟
    if (!speaking) this.noise = this.noise * 0.95 + rms * 0.05;
    this.onLevel?.(Math.min(1, rms * 8));

    segment.chunks.push(chunk);
    segment.seconds += duration;
    if (speaking) {
      segment.speech += duration;
      segment.silence = 0;
    } else {
      segment.silence += duration;
    }

    if (segment.speech === 0) {
      // 还没开口：只留最后一点当「句首」，免得吞掉第一个字
      while (segment.seconds > PRE_ROLL + duration && segment.chunks.length > 1) {
        segment.seconds -= segment.chunks.shift()!.length / this.rate;
      }
      return;
    }

    const ended = segment.silence >= SILENCE_TO_CUT && segment.speech >= MIN_SPEECH;
    if (ended || segment.seconds >= MAX_SEGMENT) {
      this.segment = this.newSegment();
      this.cut(segment);
      this.emit();
      return;
    }
    // 只是咳一声、碰一下：等不到够长的话就当没说
    if (segment.silence >= SILENCE_TO_CUT && segment.speech < MIN_SPEECH) {
      segment.speech = 0;
      return;
    }

    const now = this.elapsed();
    if (!segment.interimBusy && segment.speech >= INTERIM_MIN && now - segment.lastInterimAt >= INTERIM_EVERY) {
      segment.lastInterimAt = now;
      segment.interimBusy = true;
      const snapshot = encodeWAV(segment.chunks.slice(), this.rate);
      this.track(
        transcribe(snapshot)
          .then((text) => {
            if (text && !this.finals.has(segment.id)) this.interims.set(segment.id, text);
          })
          .catch(() => {})
          .finally(() => {
            segment.interimBusy = false;
            this.emit();
          }),
      );
    }
    this.emit();
  }

  /** 一句说完：整句识别，结果替换临时文字。 */
  private cut(segment: Segment) {
    if (segment.speech < MIN_SPEECH) return;
    const audio = encodeWAV(segment.chunks, this.rate);
    this.pendingIds.add(segment.id);
    this.track(
      transcribe(audio)
        .then((text) => {
          this.finals.set(segment.id, text);
          this.interims.delete(segment.id);
        })
        .catch((error: Error) => {
          if (!this.failed) this.onError?.(error.message);
          this.failed = true;
        })
        .finally(() => {
          this.pendingIds.delete(segment.id);
          this.emit();
        }),
    );
  }

  private track(task: Promise<void>) {
    this.inflight.add(task);
    void task.finally(() => this.inflight.delete(task));
  }

  private text(): string {
    // 整句结果到了用整句的，没到先用临时的
    const ids = new Set(Array.from(this.finals.keys()).concat(Array.from(this.interims.keys())));
    return Array.from(ids)
      .sort((a, b) => a - b)
      .map((id) => this.finals.get(id) ?? this.interims.get(id) ?? "")
      .reduce(joinText, "");
  }

  private emit() {
    this.onUpdate({
      text: this.text(),
      listening: !!this.segment && this.segment.speech > 0,
      pending: this.pendingIds.size,
    });
  }

  private release() {
    if (this.processor) this.processor.onaudioprocess = null;
    this.processor?.disconnect();
    this.stream?.getTracks().forEach((track) => track.stop());
    void this.context?.close().catch(() => {});
    this.processor = null;
    this.stream = null;
    this.context = null;
    this.onLevel?.(0);
  }
}

function transcribe(audio: Blob): Promise<string> {
  const form = new FormData();
  form.append("audio", audio, "voice.wav");
  return postForm<{ text: string }>("/speech/transcribe", form).then((result) => result.text?.trim() ?? "");
}

const PUNCTUATION = /[\s，。！？、；：,.!?;:…“”"'（）()【】\-—]/;
const CJK = /[㐀-鿿]/;

/** 把两段文字接起来：中文之间补个逗号，英文之间补个空格，已有标点就不加。 */
export function joinText(left: string, right: string): string {
  if (!left) return right;
  if (!right) return left;
  const a = left[left.length - 1];
  const b = right[0];
  if (PUNCTUATION.test(a) || PUNCTUATION.test(b)) return left + right;
  if (CJK.test(a) || CJK.test(b)) return `${left}，${right}`;
  return `${left} ${right}`;
}
