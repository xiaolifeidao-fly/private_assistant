/**
 * 朗读（浏览器自带的语音合成，不走服务端）。长辈模式里回答可以读出来听。
 *
 * iOS 要求第一次朗读发生在用户点按里，否则之后自动朗读没声音：
 * 在按下说话按钮时调一次 unlockSpeech()。
 */

const AUTO_KEY = "pa_elder_autoread";

export function canSpeak(): boolean {
  return typeof window !== "undefined" && "speechSynthesis" in window;
}

let unlocked = false;
export function unlockSpeech() {
  if (unlocked || !canSpeak()) return;
  unlocked = true;
  const utterance = new SpeechSynthesisUtterance("");
  utterance.volume = 0;
  window.speechSynthesis.speak(utterance);
}

/** 去掉 Markdown 记号和记忆编号 [mem_xxx]，免得念出「星号」「mem 下划线」。 */
export function speakableText(text: string): string {
  return text
    .replace(/\[(mem|note)_[A-Za-z0-9]+\]/g, "")
    .replace(/[*#>`_~|]/g, "")
    .replace(/^\s*[-•]\s+/gm, "")
    .replace(/\n{2,}/g, "\n")
    .trim();
}

function chineseVoice(): SpeechSynthesisVoice | undefined {
  const voices = window.speechSynthesis.getVoices();
  return voices.find((v) => v.lang === "zh-CN" && v.localService) ?? voices.find((v) => v.lang.startsWith("zh"));
}

/** 读一段话；onEnd 在读完或被打断时回调。 */
export function speak(text: string, onEnd?: () => void) {
  if (!canSpeak()) return;
  const content = speakableText(text);
  window.speechSynthesis.cancel();
  if (!content) {
    onEnd?.();
    return;
  }
  const utterance = new SpeechSynthesisUtterance(content);
  utterance.lang = "zh-CN";
  utterance.rate = 0.9; // 慢一点，老人家听得清
  const voice = chineseVoice();
  if (voice) utterance.voice = voice;
  utterance.onend = () => onEnd?.();
  utterance.onerror = () => onEnd?.();
  window.speechSynthesis.speak(utterance);
}

export function stopSpeaking() {
  if (canSpeak()) window.speechSynthesis.cancel();
}

/** 说完话后自动把回答读出来，默认开。 */
export function autoRead(): boolean {
  try {
    return localStorage.getItem(AUTO_KEY) !== "0";
  } catch {
    return true;
  }
}

export function setAutoRead(on: boolean) {
  try {
    localStorage.setItem(AUTO_KEY, on ? "1" : "0");
  } catch {
    /* 隐私模式下不可用 */
  }
}
