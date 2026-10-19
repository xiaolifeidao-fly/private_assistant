"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Toast } from "antd-mobile";
import { Camera, ChevronLeft, Keyboard, Mic, Send, Volume2, VolumeX, X } from "lucide-react";
import { HoldToTalk } from "@/components/elder/hold-to-talk";
import { useDictation } from "@/components/voice-input";
import { PhotoStrip } from "@/components/photos";
import { RichText } from "@/components/rich-text";
import { api, postForm, streamMessage } from "@/lib/api";
import { elderMode, todayConversation, withPrefix } from "@/lib/elder";
import { compressImage } from "@/lib/image";
import { canRecord } from "@/lib/recorder";
import { autoRead, canSpeak, setAutoRead, speak, stopSpeaking } from "@/lib/speak";
import type { ChatMessage, Note } from "@/lib/types";
import { isSendKey } from "@/lib/keys";

interface StreamEvent {
  type: "user_message" | "status" | "delta" | "assistant_message" | "tool_message" | "notice" | "error" | "done";
  text?: string;
  message?: ChatMessage;
  error?: string;
}

const MAX_PHOTOS = 3;

/** 键盘弹起时让页面跟着可见区域缩，输入框不被挡住（同对话页）。 */
function useViewportHeight() {
  const [height, setHeight] = useState<number | null>(null);
  useEffect(() => {
    const viewport = window.visualViewport;
    if (!viewport) return;
    const update = () => {
      setHeight(viewport.height);
      window.scrollTo(0, 0);
    };
    update();
    viewport.addEventListener("resize", update);
    return () => viewport.removeEventListener("resize", update);
  }, []);
  return height;
}

/** 显示时去掉自动加上的「记一下：」 */
function shownUserText(text: string) {
  return text.replace(/^记一下：/, "");
}

export default function ElderModePage() {
  const { mode } = useParams<{ mode: string }>();
  const router = useRouter();
  const config = elderMode(mode);
  const height = useViewportHeight();
  const [conversation, setConversation] = useState("");
  const [messages, setMessages] = useState<ChatMessage[] | null>(null);
  const [sending, setSending] = useState(false);
  const [streamText, setStreamText] = useState("");
  const [statusText, setStatusText] = useState("");
  const [pendingText, setPendingText] = useState("");
  const [typing, setTyping] = useState(false);
  const [draft, setDraft] = useState("");
  // 打字模式下按住输入框也能说，字会出现在框里
  const dictation = useDictation({ value: draft, onChange: setDraft, disabled: sending });
  const [voice, setVoice] = useState(false);
  const [reading, setReading] = useState(false);
  const [readingKey, setReadingKey] = useState("");
  const [photos, setPhotos] = useState<{ blob: Blob; url: string }[]>([]);
  const scrollRef = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!config) router.replace("/elder");
  }, [config, router]);

  useEffect(() => {
    const supported = canRecord();
    setVoice(supported);
    setTyping(!supported); // 用不了麦克风就直接打字
    setReading(autoRead() && canSpeak());
    return () => stopSpeaking();
  }, []);

  const loadMessages = useCallback(async (key: string) => {
    setMessages(await api.get<ChatMessage[]>(`/conversations/${key}/messages`));
  }, []);

  useEffect(() => {
    if (!config) return;
    (async () => {
      try {
        const key = await todayConversation(config);
        setConversation(key);
        await loadMessages(key);
      } catch (error) {
        setMessages([]);
        Toast.show({ content: (error as Error).message });
      }
    })();
  }, [config, loadMessages]);

  useLayoutEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, streamText, pendingText, statusText]);

  const read = (message: ChatMessage) => {
    if (readingKey === message.messageKey) {
      stopSpeaking();
      setReadingKey("");
      return;
    }
    setReadingKey(message.messageKey);
    speak(message.content, () => setReadingKey((current) => (current === message.messageKey ? "" : current)));
  };

  const send = async (raw: string) => {
    const text = raw.trim();
    const picked = photos;
    if (!config || !conversation || sending || (!text && !picked.length)) return;
    stopSpeaking();
    setSending(true);
    setDraft("");
    setPhotos([]);
    setPendingText(text || "（照片）");
    setStatusText("");
    let accepted = false;
    let noteKey: string | undefined;
    let answer: ChatMessage | null = null;
    try {
      if (picked.length) {
        setStatusText("正在传照片…");
        const form = new FormData();
        form.set("text", text);
        form.set("channel", "capture");
        form.set("inline", "1");
        form.set("occurredAt", new Date().toISOString());
        picked.forEach((photo, index) => form.append("images", photo.blob, `${index}.jpg`));
        noteKey = (await postForm<Note>("/notes", form)).noteKey;
        setStatusText("正在看照片…");
      }
      await streamMessage<StreamEvent>(conversation, { text: withPrefix(config, text), noteKey }, (event) => {
        switch (event.type) {
          case "user_message":
            accepted = true;
            setPendingText("");
            setStatusText("");
            setMessages((list) => [...(list ?? []), event.message!]);
            break;
          case "status":
            setStatusText(event.text ?? "");
            break;
          case "delta":
            setStreamText((value) => value + (event.text ?? ""));
            break;
          case "assistant_message":
            setStreamText("");
            answer = event.message!;
            setMessages((list) => [...(list ?? []), event.message!]);
            break;
          case "error":
            Toast.show({ content: "出了点问题，请再说一遍" });
            break;
        }
      });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
      if (!accepted && !noteKey) {
        setDraft(text);
        setPhotos(picked);
      }
    } finally {
      setSending(false);
      setStreamText("");
      setStatusText("");
      setPendingText("");
      if (noteKey || accepted) picked.forEach((photo) => URL.revokeObjectURL(photo.url));
      loadMessages(conversation).catch(() => {});
    }
    const final = answer as ChatMessage | null;
    if (final?.content && reading) {
      setReadingKey(final.messageKey);
      speak(final.content, () => setReadingKey(""));
    }
  };

  const pickPhotos = async (files: File[]) => {
    const room = MAX_PHOTOS - photos.length;
    const picked: { blob: Blob; url: string }[] = [];
    for (const file of files.slice(0, Math.max(room, 0))) {
      try {
        const blob = await compressImage(file);
        picked.push({ blob, url: URL.createObjectURL(blob) });
      } catch (error) {
        Toast.show({ content: (error as Error).message });
      }
    }
    setPhotos((list) => [...list, ...picked]);
  };

  const toggleReading = () => {
    const next = !reading;
    setReading(next);
    setAutoRead(next);
    if (!next) {
      stopSpeaking();
      setReadingKey("");
    }
    Toast.show({ content: next ? "回答会读给您听" : "不读出来了" });
  };

  if (!config) return null;
  const Icon = config.icon;
  // 只显示说的和答的，查记忆、建提醒这些过程不显示
  const visible = (messages ?? []).filter((m) => (m.role === "user" || m.role === "assistant") && m.content.trim());

  return (
    <div className="el-chat" data-tone={config.tone} style={height ? { height } : undefined}>
      <header className="el-bar">
        <button className="el-back" onClick={() => router.push("/elder")} aria-label="返回">
          <ChevronLeft size={34} strokeWidth={2.6} />
          <span>返回</span>
        </button>
        <div className="el-bar-title">
          <Icon size={26} strokeWidth={2.2} />
          {config.title}
        </div>
        {canSpeak() ? (
          <button className="el-read-toggle" data-on={reading} onClick={toggleReading} aria-label={reading ? "关掉朗读" : "打开朗读"}>
            {reading ? <Volume2 size={26} strokeWidth={2.2} /> : <VolumeX size={26} strokeWidth={2.2} />}
            <span>{reading ? "读出来" : "不读"}</span>
          </button>
        ) : (
          <span style={{ width: 88 }} />
        )}
      </header>

      <div className="el-msgs" ref={scrollRef}>
        <div className="el-msg" data-role="assistant">
          <div className="el-bubble">{config.greeting}</div>
        </div>
        {messages && visible.length === 0 && !pendingText && (
          <div className="el-examples">
            <div className="el-examples-title">可以这样说（点一下就{config.prefix ? "记" : "问"}）：</div>
            {config.examples.map((example) => (
              <button key={example} className="el-example" onClick={() => void send(example)} disabled={sending || !conversation}>
                “{example}”
              </button>
            ))}
          </div>
        )}
        {!messages && <div className="el-loading"><span className="pa-btn-spin" /> 正在打开…</div>}
        {visible.map((message) =>
          message.role === "user" ? (
            <div key={message.messageKey} className="el-msg" data-role="user">
              <div className="el-bubble">{shownUserText(message.content)}</div>
            </div>
          ) : (
            <div key={message.messageKey} className="el-msg" data-role="assistant">
              <div className="el-bubble">
                <RichText text={message.content} />
                {message.meta?.photos?.length ? (
                  <PhotoStrip photos={message.meta.photos.map((photo) => ({ noteKey: photo.noteKey, index: photo.index, caption: photo.title }))} />
                ) : null}
              </div>
              {canSpeak() && (
                <button className="el-listen" data-on={readingKey === message.messageKey} onClick={() => read(message)}>
                  {readingKey === message.messageKey ? <><VolumeX size={22} strokeWidth={2.2} />停下</> : <><Volume2 size={22} strokeWidth={2.2} />听一听</>}
                </button>
              )}
            </div>
          ),
        )}
        {pendingText && (
          <div className="el-msg" data-role="user">
            <div className="el-bubble">{pendingText}</div>
          </div>
        )}
        {sending && (
          <div className="el-msg" data-role="assistant">
            <div className="el-bubble">
              {streamText ? <RichText text={streamText} streaming /> : <span className="el-thinking"><span className="pa-btn-spin" />{statusText || "正在想…"}</span>}
            </div>
          </div>
        )}
      </div>

      <footer className="el-input">
        {photos.length > 0 && (
          <div className="el-picked">
            {photos.map((photo, index) => (
              <div key={photo.url} className="el-picked-item">
                <img src={photo.url} alt="" />
                <button onClick={() => setPhotos((list) => list.filter((_, i) => i !== index))} aria-label="不要这张"><X size={20} strokeWidth={2.6} /></button>
              </div>
            ))}
            <span className="el-picked-hint">再说一下这是什么、放在哪</span>
          </div>
        )}
        {typing && (
          <textarea
            className="el-textarea"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="在这里打字"
            onKeyDown={(event) => { if (isSendKey(event) && !dictation.busy) { event.preventDefault(); void send(draft); } }}
            readOnly={dictation.busy}
            {...dictation.holdProps}
            rows={2}
            enterKeyHint="send"
          />
        )}
        <div className="el-input-row">
          {config.photos && (
            <button className="el-side-btn" onClick={() => fileRef.current?.click()} disabled={sending || photos.length >= MAX_PHOTOS} aria-label="拍照">
              <Camera size={28} strokeWidth={2.1} />
              <span>拍照</span>
            </button>
          )}
          {typing ? (
            <button className="el-send" onClick={() => void send(draft)} disabled={sending || dictation.busy || (!draft.trim() && !photos.length)}>
              <Send size={28} strokeWidth={2.3} />
              <span>发送</span>
            </button>
          ) : (
            <HoldToTalk onText={(text) => void send(text)} disabled={sending || !conversation} />
          )}
          {voice && (
            <button className="el-side-btn" onClick={() => setTyping(!typing)} aria-label={typing ? "改成说话" : "改成打字"}>
              {typing ? <Mic size={28} strokeWidth={2.1} /> : <Keyboard size={28} strokeWidth={2.1} />}
              <span>{typing ? "说话" : "打字"}</span>
            </button>
          )}
        </div>
        {!typing && photos.length > 0 && (
          <button className="el-photo-only" onClick={() => void send("")} disabled={sending}>不说了，直接发照片</button>
        )}
      </footer>
      {dictation.overlay}
      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        capture="environment"
        hidden
        onChange={(event) => {
          const files = Array.from(event.target.files ?? []);
          event.target.value = "";
          if (files.length) void pickPhotos(files);
        }}
      />
    </div>
  );
}
