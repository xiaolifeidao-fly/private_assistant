"use client";

import { useEffect, useRef, useState } from "react";
import dayjs from "dayjs";
import { DatePicker, ImageViewer, TextArea, Toast } from "antd-mobile";
import { CalendarClock, Camera, ClipboardPaste, ImagePlus, Images, Mic, PenLine, Sparkles, X } from "lucide-react";
import { useRouter } from "next/navigation";
import { api, postForm } from "@/lib/api";
import { compressImage } from "@/lib/image";
import { enqueue, isNetworkError, outboxSize } from "@/lib/outbox";
import type { Note } from "@/lib/types";
import { personLabel, usePersons, useMemoryTypes } from "@/lib/family";
import { personTone, typeTone } from "@/lib/tone";
import { Button, Chips, PageHeader, Segmented, Tip } from "@/components/ui";
import { PickedPhotos } from "@/components/photos";
import { VoiceButton } from "@/components/voice-input";

// 速记里常用的类型放前面；其余类型在管理端录入。名称内置一份，断网时也能显示中文。
const quickTypes: [string, string][] = [
  ["quote", "原话"],
  ["need", "诉求"],
  ["commitment", "我的承诺"],
  ["event", "事件"],
  ["preference", "喜好厌恶"],
  ["habit", "生活习惯"],
  ["trait", "性格特质"],
  ["taboo", "雷区"],
  ["custom", "规矩习俗"],
  ["health", "健康"],
  ["routine", "作息安排"],
];
const modeHints: Record<string, string> = {
  capture: "说一段、拍张照，助理帮你整理",
  quick: "十秒记下一句话",
  paste: "聊天截图交给助理慢慢整理",
};
const importanceLabels =["随口", "一般", "在意", "重要", "很重要"];
const DRAFT_KEY = "pa_note_draft";
const PASTE_DRAFT_KEY = "pa_paste_draft";
const CAPTURE_DRAFT_KEY = "pa_capture_draft";
const MAX_IMAGES = 9;

/**
 * 速记页三种模式：
 *  - 随手录：打字或用键盘听写说一段，可以带照片（记东西放哪），交给大模型清洗、整理成记忆；
 *  - 记一条：10 秒内记下一句话，选「关于谁」和「是什么」，提交即生效；
 *  - 整理聊天：粘贴聊天记录或选截图，后台自动整理成多条记忆，没把握的进「今天」待确认。
 * 页面上任何位置粘贴图片（⌘V / 长按粘贴）都会切到「整理聊天」并加进截图。
 */
export default function NotePage() {
  const [mode, setMode] = useState("capture");
  const [pasted, setPasted] = useState<File[]>([]);

  useEffect(() => {
    const onPaste = (event: ClipboardEvent) => {
      const files = clipboardImages(event.clipboardData);
      if (!files.length) return;
      // 同时带文字且正粘在输入框里时，让文字照常粘进去，图片另外收下
      const editable = event.target instanceof HTMLElement && event.target.closest("textarea, input, [contenteditable]");
      if (!(editable && event.clipboardData?.getData("text/plain"))) event.preventDefault();
      setMode("paste");
      setPasted((list) => [...list, ...files]);
    };
    document.addEventListener("paste", onPaste);
    return () => document.removeEventListener("paste", onPaste);
  }, []);

  return (
    <div>
      <PageHeader eyebrow={modeHints[mode]} title="速记" />
      <div style={{ padding: "6px 20px 0" }}>
        <Segmented
          value={mode}
          onChange={setMode}
          options={[
            { value: "capture", label: <span className="note-seg"><Mic size={15} strokeWidth={2.2} />随手记</span> },
            { value: "quick", label: <span className="note-seg"><PenLine size={15} strokeWidth={2.2} />记一句</span> },
            { value: "paste", label: <span className="note-seg"><Images size={15} strokeWidth={2.2} />聊天截图</span> },
          ]}
        />
      </div>
      <Tip id="note-modes" title="三种记法，挑顺手的用" style={{ marginTop: 14 }}>
        <b>随手记</b>：说一段、拍张照，最省事。<b>记一句</b>：自己选好是谁、什么事。<b>聊天截图</b>：把微信聊天截下来交给助理。
      </Tip>
      <div key={mode} className="pa-enter">
        {mode === "capture" ? <CaptureForm /> : mode === "quick" ? <QuickNoteForm /> : <PasteForm pasted={pasted} onPastedTaken={() => setPasted([])} />}
      </div>
      <style jsx global>{`
        .note-submit { padding: 30px 20px 0; }
        .note-seg { display: inline-flex; align-items: center; gap: 5px; }
        .note-tools { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin-top: 12px; }
        .note-tool {
          appearance: none;
          height: 84px;
          border-radius: var(--pa-radius);
          border: 0.5px solid var(--pa-line);
          background: var(--pa-surface);
          box-shadow: var(--pa-shadow-sm);
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          gap: 7px;
          font-size: 13.5px;
          font-weight: 600;
          color: var(--pa-text);
          cursor: pointer;
          transition: transform 0.2s var(--pa-spring), background 0.15s;
        }
        .note-tool:active { transform: scale(0.95); background: var(--pa-surface-2); }
        .note-tool:disabled, .note-tool[data-disabled="true"] { opacity: 0.4; pointer-events: none; }
        .note-tool-icon { width: 38px; height: 38px; border-radius: 19px; display: grid; place-items: center; background: var(--tone-bg); color: var(--tone-fg); }
        .note-tools + .pa-field-hint, .picked-photos + .note-tools { margin-top: 12px; }
        .pa-textarea-wrap + .picked-photos { margin-top: 12px; padding-left: 0; }
        .note-foot { font-size: 12.5px; color: var(--pa-text-3); text-align: center; margin-top: 10px; line-height: 1.6; padding: 0 12px 4px; }
        .note-time { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
        .note-shots { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; }
        .note-shot { position: relative; aspect-ratio: 1; border-radius: 12px; overflow: hidden; background: var(--pa-fill); }
        .note-shot img { width: 100%; height: 100%; object-fit: cover; }
        .note-shot-remove {
          position: absolute;
          top: 4px;
          right: 4px;
          width: 22px;
          height: 22px;
          border-radius: 11px;
          border: 0;
          display: grid;
          place-items: center;
          background: rgba(0, 0, 0, 0.55);
          color: #fff;
          -webkit-backdrop-filter: blur(6px);
          backdrop-filter: blur(6px);
          cursor: pointer;
        }
        .note-shot-add {
          aspect-ratio: 1;
          border-radius: 12px;
          border: 1.5px dashed var(--pa-line-strong);
          background: var(--pa-surface);
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          gap: 4px;
          color: var(--pa-text-3);
          font-size: 11.5px;
          cursor: pointer;
          transition: background 0.15s;
        }
        .note-shot-add:active { background: var(--pa-surface-2); }
        button.note-shot-add { font: inherit; padding: 0; }
      `}</style>
    </div>
  );
}

function TimeChips({ value, onChange }: { value: Date; onChange: (value: Date) => void }) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const isToday = dayjs(value).isSame(dayjs(), "day");
  const isYesterday = dayjs(value).isSame(dayjs().subtract(1, "day"), "day");
  const custom = !isToday && !isYesterday;
  return (
    <div className="note-time">
      <button type="button" className="pa-chip" data-size="sm" data-selected={isToday} onClick={() => onChange(new Date())}>今天</button>
      <button type="button" className="pa-chip" data-size="sm" data-selected={isYesterday} onClick={() => onChange(dayjs().subtract(1, "day").toDate())}>昨天</button>
      <button type="button" className="pa-chip" data-size="sm" data-selected={custom} onClick={() => setPickerOpen(true)}>
        <CalendarClock size={14} />
        <span className="pa-num">{dayjs(value).format(custom ? "M月D日 HH:mm" : "HH:mm")}</span>
      </button>
      <DatePicker
        title="什么时候的事"
        visible={pickerOpen}
        precision="minute"
        value={value}
        max={new Date()}
        onClose={() => setPickerOpen(false)}
        onConfirm={onChange}
      />
    </div>
  );
}

function QuickNoteForm() {
  const persons = usePersons();
  const types = useMemoryTypes();
  const [subject, setSubject] = useState<string[]>(["wife"]);
  const [type, setType] = useState<string[]>(["quote"]);
  const [content, setContent] = useState("");
  const [when, setWhen] = useState<Date>(new Date());
  const [importance, setImportance] = useState(3);
  const [saving, setSaving] = useState(false);
  const [offline, setOffline] = useState(0);

  useEffect(() => {
    setOffline(outboxSize());
    try {
      setContent(localStorage.getItem(DRAFT_KEY) || "");
    } catch {
      /* ignore */
    }
  }, []);
  useEffect(() => {
    try {
      if (content) localStorage.setItem(DRAFT_KEY, content);
      else localStorage.removeItem(DRAFT_KEY);
    } catch {
      /* ignore */
    }
  }, [content]);

  const hint = types.find((t) => t.type === type[0])?.hint;
  const personOptions = [
    ...persons.filter((p) => p.isCore && p.personKey !== "me"),
    ...persons.filter((p) => !p.isCore),
  ].map((p) => ({ label: personLabel(persons, p.personKey), value: p.personKey, tone: personTone(p.personKey, persons) }));
  personOptions.push({ label: "全家", value: "", tone: "slate" });

  const submit = async () => {
    const text = content.trim();
    if (!text) {
      Toast.show({ content: "先写点内容" });
      return;
    }
    setSaving(true);
    let body: Record<string, unknown> = {};
    try {
      const attrs: Record<string, string> = {};
      if (type[0] === "quote" && subject[0]) attrs.speaker = subject[0];
      if (type[0] === "commitment") Object.assign(attrs, { promisedTo: subject[0] ?? "", status: "open" });
      if (type[0] === "need") attrs.status = "open";
      body = {
        type: type[0],
        subjectPerson: subject[0] ?? "",
        title: "",
        content: text,
        importance,
        occurredAt: when.toISOString(),
        attrs: Object.keys(attrs).length ? attrs : undefined,
      };
      await api.post("/memories", body);
      reset();
      Toast.show({ icon: "success", content: "记下了" });
    } catch (error) {
      if (isNetworkError(error)) {
        enqueue("/memories", body);
        setOffline(outboxSize());
        reset();
        Toast.show({ content: "没网，先存在手机上，联网后自动提交" });
      } else {
        Toast.show({ content: (error as Error).message });
      }
    } finally {
      setSaving(false);
    }
  };

  function reset() {
    setContent("");
    setImportance(3);
    setWhen(new Date());
  }

  return (
    <div>
      <div className="pa-field">
        <div className="pa-label">关于谁</div>
        <Chips scroll options={personOptions} value={subject} onChange={setSubject} />
      </div>

      <div className="pa-field">
        <div className="pa-label">是什么</div>
        <Chips
          size="sm"
          rows={2}
          options={quickTypes.map(([value, label]) => ({ value, label, tone: typeTone(value) }))}
          value={type}
          onChange={setType}
        />
        {hint && <div className="pa-field-hint">{hint}</div>}
      </div>

      <div className="pa-field">
        <div className="pa-label">内容</div>
        <div className="pa-textarea-wrap">
          <TextArea
            value={content}
            onChange={setContent}
            placeholder={type[0] === "quote" ? "她的原话，尽量一字不差" : "发生了什么 / 她在意什么 / 我答应了什么"}
            rows={3}
            autoSize={{ minRows: 3, maxRows: 10 }}
          />
          {content && <div className="pa-textarea-foot">{content.length} 字</div>}
        </div>
      </div>

      <div className="pa-field">
        <div className="pa-label">什么时候</div>
        <TimeChips value={when} onChange={setWhen} />
      </div>

      <div className="pa-field">
        <div className="pa-label">有多重要<small>{importanceLabels[importance - 1]}</small></div>
        <div className="pa-importance">
          {importanceLabels.map((label, index) => (
            <button key={label} type="button" data-on={index < importance} data-current={index + 1 === importance} onClick={() => setImportance(index + 1)}>
              {index + 1}
            </button>
          ))}
        </div>
      </div>

      <div className="note-submit">
        <Button block variant="accent" loading={saving} disabled={!content.trim()} onClick={submit}>记下来</Button>
        <div className="note-foot">说不清类型也没关系，直接去和助理说「记一下：……」</div>
        {offline > 0 && <div className="note-foot" style={{ color: "var(--pa-warning)" }}>手机上还有 {offline} 条离线速记，联网后会自动提交</div>}
      </div>
    </div>
  );
}

/**
 * 随手录：想到什么说什么（iPhone 键盘上的麦克风可以直接听写），东西放在哪就拍张照。
 * 不用选类型，提交后由大模型清洗：认人、换算日期、拆成几条记忆、和旧记忆去重，没把握的进「今天」待确认。
 */
function CaptureForm() {
  const router = useRouter();
  const persons = usePersons();
  const [text, setText] = useState("");
  const [about, setAbout] = useState<string[]>([]);
  const [images, setImages] = useState<PickedImage[]>([]);
  const [when, setWhen] = useState<Date>(new Date());
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    try {
      setText(localStorage.getItem(CAPTURE_DRAFT_KEY) || "");
    } catch {
      /* ignore */
    }
  }, []);
  useEffect(() => {
    try {
      if (text) localStorage.setItem(CAPTURE_DRAFT_KEY, text);
      else localStorage.removeItem(CAPTURE_DRAFT_KEY);
    } catch {
      /* ignore */
    }
  }, [text]);
  const imagesRef = useRef(images);
  imagesRef.current = images;
  useEffect(() => () => imagesRef.current.forEach((image) => URL.revokeObjectURL(image.url)), []);

  const pick = async (files: File[]) => {
    const room = MAX_IMAGES - imagesRef.current.length;
    if (files.length > room) Toast.show({ content: `一次最多 ${MAX_IMAGES} 张，多出来的没加上` });
    const picked: PickedImage[] = [];
    for (const file of files.slice(0, Math.max(room, 0))) {
      try {
        const blob = await compressImage(file);
        picked.push({ blob, url: URL.createObjectURL(blob) });
      } catch (error) {
        Toast.show({ content: (error as Error).message });
      }
    }
    setImages((list) => [...list, ...picked]);
  };

  const submit = async () => {
    if (!text.trim() && images.length === 0) {
      Toast.show({ content: "说点什么，或者拍张照" });
      return;
    }
    const form = new FormData();
    form.set("text", text);
    form.set("channel", "capture");
    form.set("aboutPersons", about.join(","));
    form.set("occurredAt", when.toISOString());
    images.forEach((image, index) => form.append("images", image.blob, `${index}.jpg`));
    setSaving(true);
    try {
      await postForm<Note>("/notes", form);
      images.forEach((image) => URL.revokeObjectURL(image.url));
      setText("");
      setImages([]);
      setAbout([]);
      setWhen(new Date());
      Toast.show({ icon: "success", content: "已交给助理整理" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setSaving(false);
    }
  };

  const personOptions = persons
    .filter((p) => p.personKey !== "me")
    .map((p) => ({ label: personLabel(persons, p.personKey), value: p.personKey, tone: personTone(p.personKey, persons) }));

  return (
    <div>
      <div className="pa-field">
        <div className="pa-label">想记什么</div>
        <div className="pa-textarea-wrap">
          <TextArea
            value={text}
            onChange={setText}
            placeholder={images.length ? "这是什么、干什么用、放在哪……" : "发生了什么、谁说了什么、东西放哪了……想到哪说到哪"}
            rows={4}
            autoSize={{ minRows: 4, maxRows: 14 }}
          />
          {text && <div className="pa-textarea-foot">{text.length} 字</div>}
        </div>
        <PickedPhotos urls={images.map((image) => image.url)} onRemove={(index) => setImages((list) => {
          URL.revokeObjectURL(list[index].url);
          return list.filter((_, i) => i !== index);
        })} />
        <div className="note-tools">
          <VoiceButton className="note-tool" onText={(spoken) => setText((current) => (current ? `${current}\n${spoken}` : spoken))}>
            <span className="note-tool-icon" data-tone="clay"><Mic size={20} strokeWidth={2} /></span>
            说一段
          </VoiceButton>
          <label className="note-tool" data-disabled={images.length >= MAX_IMAGES}>
            <span className="note-tool-icon" data-tone="sky"><Camera size={20} strokeWidth={2} /></span>
            拍照
            <input type="file" accept="image/*" capture="environment" hidden disabled={images.length >= MAX_IMAGES} onChange={(event) => { void pick(Array.from(event.target.files ?? [])); event.target.value = ""; }} />
          </label>
          <label className="note-tool" data-disabled={images.length >= MAX_IMAGES}>
            <span className="note-tool-icon" data-tone="plum"><ImagePlus size={20} strokeWidth={2} /></span>
            相册
            <input type="file" accept="image/*" multiple hidden disabled={images.length >= MAX_IMAGES} onChange={(event) => { void pick(Array.from(event.target.files ?? [])); event.target.value = ""; }} />
          </label>
        </div>
        <div className="pa-field-hint">懒得打字就点「说一段」（最长 5 分钟），说得乱也没关系。东西放哪了就拍一张，以后能翻出原图。</div>
      </div>

      <div className="pa-field">
        <div className="pa-label">关于谁<small>可不选，助理会自己认</small></div>
        <Chips scroll multiple options={personOptions} value={about} onChange={setAbout} />
      </div>

      <div className="pa-field">
        <div className="pa-label">什么时候的事</div>
        <TimeChips value={when} onChange={setWhen} />
      </div>

      <div className="note-submit">
        <Button block variant="accent" loading={saving} disabled={!text.trim() && !images.length} onClick={submit}><Sparkles size={17} />交给助理整理</Button>
        <div className="note-foot">
          半分钟左右整理好，有把握的直接记下，没把握的放进「今天」等你确认。
        </div>
        <div className="note-foot"><button type="button" className="pa-link-btn" onClick={() => router.push("/today")}>看整理结果</button></div>
      </div>
    </div>
  );
}

function clipboardImages(data: DataTransfer | null): File[] {
  if (!data) return [];
  return Array.from(data.items)
    .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
    .map((item) => item.getAsFile())
    .filter((file): file is File => !!file);
}

interface PickedImage {
  blob: Blob;
  url: string;
}

function PasteForm({ pasted, onPastedTaken }: { pasted: File[]; onPastedTaken: () => void }) {
  const router = useRouter();
  const persons = usePersons();
  const [about, setAbout] = useState<string[]>(["wife"]);
  const [text, setText] = useState("");
  const [images, setImages] = useState<PickedImage[]>([]);
  const [when, setWhen] = useState<Date>(new Date());
  const [saving, setSaving] = useState(false);
  const [preview, setPreview] = useState<number | null>(null);

  useEffect(() => {
    try {
      setText(localStorage.getItem(PASTE_DRAFT_KEY) || "");
    } catch {
      /* ignore */
    }
  }, []);
  useEffect(() => {
    try {
      if (text) localStorage.setItem(PASTE_DRAFT_KEY, text);
      else localStorage.removeItem(PASTE_DRAFT_KEY);
    } catch {
      /* ignore */
    }
  }, [text]);
  // 只在离开页面时释放预览图；不能挂在 images 上，否则每次增删都会把还在显示的图释放掉
  const imagesRef = useRef(images);
  imagesRef.current = images;
  useEffect(() => () => imagesRef.current.forEach((image) => URL.revokeObjectURL(image.url)), []);

  const pick = async (files: File[]) => {
    if (!files.length) return;
    const room = MAX_IMAGES - imagesRef.current.length;
    if (files.length > room) Toast.show({ content: `一次最多 ${MAX_IMAGES} 张，多出来的没加上` });
    const picked: PickedImage[] = [];
    for (const file of files.slice(0, room)) {
      try {
        const blob = await compressImage(file);
        picked.push({ blob, url: URL.createObjectURL(blob) });
      } catch (error) {
        Toast.show({ content: (error as Error).message });
      }
    }
    setImages((list) => [...list, ...picked]);
  };

  // 页面级粘贴收到的图片（可能是在「记一条」时粘贴、刚切过来）
  const takenRef = useRef<File[]>([]);
  useEffect(() => {
    if (!pasted.length || takenRef.current === pasted) return;
    takenRef.current = pasted;
    onPastedTaken();
    void pick(pasted);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pasted]);

  // 手机上没有 ⌘V，读剪贴板要用户点一下；iOS 会弹出系统「粘贴」确认
  const [canReadClipboard, setCanReadClipboard] = useState(false);
  useEffect(() => setCanReadClipboard(typeof navigator !== "undefined" && !!navigator.clipboard?.read), []);
  const pasteFromClipboard = async () => {
    try {
      const files: File[] = [];
      for (const item of await navigator.clipboard.read()) {
        const type = item.types.find((t) => t.startsWith("image/"));
        if (type) files.push(new File([await item.getType(type)], `粘贴.${type.split("/")[1]}`, { type }));
      }
      if (files.length) await pick(files);
      else Toast.show({ content: "剪贴板里没有图片，先截图或复制一张图" });
    } catch {
      Toast.show({ content: "读不了剪贴板，可以长按下面的输入框粘贴" });
    }
  };

  const submit = async () => {
    if (!text.trim() && images.length === 0) {
      Toast.show({ content: "粘贴聊天内容，或者选几张截图" });
      return;
    }
    const form = new FormData();
    form.set("text", text);
    form.set("channel", images.length ? "screenshot" : "paste");
    form.set("aboutPersons", about.join(","));
    form.set("occurredAt", when.toISOString());
    images.forEach((image, index) => form.append("images", image.blob, `${index}.jpg`));
    setSaving(true);
    try {
      await postForm<Note>("/notes", form);
      setText("");
      setImages([]);
      Toast.show({ icon: "success", content: "已提交，整理好后在「今天」里看" });
      router.push("/today");
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    } finally {
      setSaving(false);
    }
  };

  const personOptions = persons
    .filter((p) => p.personKey !== "me")
    .map((p) => ({ label: personLabel(persons, p.personKey), value: p.personKey, tone: personTone(p.personKey, persons) }));

  return (
    <div>
      <div className="pa-field">
        <div className="pa-label">和谁的聊天 / 关于谁<small>可多选</small></div>
        <Chips scroll multiple options={personOptions} value={about} onChange={setAbout} />
      </div>

      <div className="pa-field">
        <div className="pa-label">截图<small>{images.length} / {MAX_IMAGES}</small></div>
        <div className="note-shots">
          {images.map((image, index) => (
            <div key={image.url} className="note-shot">
              <img src={image.url} alt="" onClick={() => setPreview(index)} />
              <button
                type="button"
                className="note-shot-remove"
                aria-label="移除"
                onClick={() => {
                  URL.revokeObjectURL(image.url);
                  setImages((list) => list.filter((_, i) => i !== index));
                }}
              >
                <X size={13} strokeWidth={2.6} />
              </button>
            </div>
          ))}
          {images.length < MAX_IMAGES && (
            <label className="note-shot-add">
              <ImagePlus size={22} strokeWidth={1.7} />
              添加截图
              <input type="file" accept="image/*" multiple hidden onChange={(event) => { void pick(Array.from(event.target.files ?? [])); event.target.value = ""; }} />
            </label>
          )}
          {images.length < MAX_IMAGES && canReadClipboard && (
            <button type="button" className="note-shot-add" onClick={() => void pasteFromClipboard()}>
              <ClipboardPaste size={22} strokeWidth={1.7} />
              粘贴截图
            </button>
          )}
        </div>
        <div className="pa-field-hint">多条消息直接截图最省事；截图后可直接点「粘贴截图」，或在下面输入框里粘贴</div>
      </div>
      <ImageViewer.Multi images={images.map((i) => i.url)} visible={preview !== null} defaultIndex={preview ?? 0} onClose={() => setPreview(null)} />

      <div className="pa-field">
        <div className="pa-label">或者粘贴文字</div>
        <div className="pa-textarea-wrap">
          <TextArea value={text} onChange={setText} placeholder="粘贴聊天记录，或者把刚才发生的事写下来，越原汁原味越好" rows={4} autoSize={{ minRows: 4, maxRows: 14 }} />
          {text && <div className="pa-textarea-foot">{text.length} 字</div>}
        </div>
      </div>

      <div className="pa-field">
        <div className="pa-label">大概什么时候的事</div>
        <TimeChips value={when} onChange={setWhen} />
        <div className="pa-field-hint">聊天里的「昨天」「下周」会按这个时间换算</div>
      </div>

      <div className="note-submit">
        <Button block variant="accent" loading={saving} disabled={!text.trim() && !images.length} onClick={submit}><Sparkles size={17} />交给助理整理</Button>
        <div className="note-foot">一般半分钟到一分钟。有把握的直接记下，没把握的放进「今天」等你确认</div>
      </div>
    </div>
  );
}
