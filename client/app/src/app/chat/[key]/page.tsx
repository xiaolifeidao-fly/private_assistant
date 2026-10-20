"use client";

import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Dialog, TextArea, Toast } from "antd-mobile";
import type { TextAreaRef } from "antd-mobile";
import { ArrowDown, ArrowUp, AtSign, Camera, Check, Copy, Info, LifeBuoy, PanelLeft, Sparkles, SquarePen, ThumbsDown, ThumbsUp, TriangleAlert, UserRoundPen, X } from "lucide-react";
import { AuthGate } from "@/components/auth-gate";
import { CommandMenu, commandsFor, filterCommands, findMention } from "@/components/command-menu";
import type { Command } from "@/components/command-menu";
import { ConversationDrawer } from "@/components/conversation-drawer";
import { MemoryPopup } from "@/components/memory-popup";
import { NoteSheet } from "@/components/note-sheet";
import { PhotoStrip, PickedPhotos } from "@/components/photos";
import { useDictation, VoiceButton } from "@/components/voice-input";
import { useInputHistory } from "@/lib/input-history";
import { isSendKey } from "@/lib/keys";
import { RichText } from "@/components/rich-text";
import { Avatar, NavBar } from "@/components/ui";
import { api, postForm, streamMessage } from "@/lib/api";
import { compressImage } from "@/lib/image";
import { useOnResume } from "@/lib/events";
import { personLabel, usePersons } from "@/lib/family";
import { botTone } from "@/lib/tone";
import type { ChatMessage, Conversation, Note } from "@/lib/types";

interface BotSummary {
  botKey: string;
  kind: string;
  name: string;
  avatar: string;
  greeting: string;
}

interface StreamEvent {
  type: "user_message" | "status" | "delta" | "assistant_message" | "tool_message" | "notice" | "error" | "done";
  text?: string;
  message?: ChatMessage;
  error?: string;
}

const toolText: Record<string, string> = {
  search_memory: "翻了翻记忆",
  get_person: "看了人物档案",
  get_relations: "看了亲戚关系",
  get_place: "看了老家的规矩",
  list_open_items: "查了没办完的事",
  remember: "记下来了",
  ask_persona: "让分身预演了一下",
  list_upcoming: "查了近期提醒",
  create_reminder: "建好了提醒",
  cancel_reminder: "取消了提醒",
  diet_overview: "看了吃饭记录",
  load_skill: "翻了做事手册",
  now: "看了看时间",
};

// 空对话时给的开场建议，点一下填进输入框
const MAX_PHOTOS = 9;
const MAX_TEXT_FILE = 200 << 10;
const TEXT_EXTENSIONS = /\.(txt|md|markdown|csv|tsv|json|log|xml|ya?ml|ini|conf|html?|srt|vtt)$/i;

// Safari、从网页复制的图片往往只在 items 里，files 是空的
function clipboardFiles(data: DataTransfer | null): File[] {
  if (!data) return [];
  if (data.files.length) return Array.from(data.files);
  return Array.from(data.items)
    .filter((item) => item.kind === "file")
    .map((item) => item.getAsFile())
    .filter((file): file is File => !!file);
}

function isTextFile(file: File) {
  return file.type.startsWith("text/") || file.type === "application/json" || TEXT_EXTENSIONS.test(file.name);
}

const starters: Record<string, string[]> = {
  remind: ["明早 8 点提醒我给妈打电话", "周五下班前交报销", "最近有哪些提醒？", "提醒我："],
  recall: ["护照和户口本放哪了？", "上次修空调是哪天？", "她说过想要什么礼物？", "记一下："],
  assistant: ["她最近在意什么？", "下周回老家要注意什么？", "记一下：", "我答应过的事还有哪些没做？"],
  wife: ["我打算这么跟你说：", "你最近为什么不太开心？"],
  mom: ["我打算这么跟你说：", "带孩子的事你怎么看？"],
  daughter: ["她这个年纪该怎么引导？", "最近她喜欢什么？"],
};

export default function ChatPage() {
  return (
    <AuthGate>
      <Chat />
    </AuthGate>
  );
}

/**
 * iOS 主屏 App 里键盘弹起时只缩小 visualViewport，不缩布局视口，
 * 输入框会被顶出屏幕。这里让容器高度直接跟随 visualViewport。
 */
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

function Chat() {
  const { key } = useParams<{ key: string }>();
  const router = useRouter();
  const height = useViewportHeight();
  const [conversation, setConversation] = useState<Conversation | null>(null);
  const [bot, setBot] = useState<BotSummary | null>(null);
  const [drawer, setDrawer] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[] | null>(null);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [streamText, setStreamText] = useState("");
  const [memoryKey, setMemoryKey] = useState<string | null>(null);
  const [openNote, setOpenNote] = useState<Note | null>(null);
  const showNote = useCallback((key: string) => {
    api.get<Note>(`/notes/${key}`).then(setOpenNote).catch((error) => Toast.show({ content: (error as Error).message }));
  }, []);
  // 所有对话都可以带照片：选好的照片、发出去还没被服务端收下时的本机预览、整理进度
  const [photos, setPhotos] = useState<{ blob: Blob; url: string }[]>([]);
  const [pendingUser, setPendingUser] = useState<{ text: string; urls: string[] } | null>(null);
  const [statusText, setStatusText] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<TextAreaRef>(null);
  // 输入「@」弹出快捷指令（记一下、提醒我……），选中后替换掉「@xx」
  const [mention, setMention] = useState<{ start: number; query: string } | null>(null);
  // 选中的指令单独显示成一块标签放在输入框最前面，发送时拼在话前头；光标在最前时按一下回退整块删掉
  const [chip, setChip] = useState<{ label: string; text: string } | null>(null);
  const [activeCommand, setActiveCommand] = useState(0);
  // 用户往上翻看历史时不要被流式输出拽回底部
  const stickRef = useRef(true);
  const canRehearse = bot?.botKey === "wife" || bot?.botKey === "mom";
  // 所有对话都能带照片（拍照、相册或粘贴）
  const canAttach = !!bot;

  const pickPhotos = async (files: File[]) => {
    const room = MAX_PHOTOS - photos.length;
    if (files.length > room) Toast.show({ content: `一次最多 ${MAX_PHOTOS} 张` });
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

  // 粘贴：图片当照片带上；文本类文件读出内容接到草稿后面；纯文字交给输入框自己处理
  const onPaste = (event: ClipboardEvent) => {
    const files = clipboardFiles(event.clipboardData);
    if (!files.length) return;
    event.preventDefault();
    const images = files.filter((file) => file.type.startsWith("image/"));
    const others = files.filter((file) => !file.type.startsWith("image/"));
    if (images.length) {
      if (canAttach) void pickPhotos(images);
    }
    void (async () => {
      for (const file of others) {
        if (!isTextFile(file)) {
          Toast.show({ content: `${file.name} 暂不支持，可以粘贴图片或文本文件` });
          continue;
        }
        if (file.size > MAX_TEXT_FILE) {
          Toast.show({ content: `${file.name} 太大了（最多 ${MAX_TEXT_FILE >> 10}KB）` });
          continue;
        }
        const content = (await file.text()).trim();
        if (!content) continue;
        setDraft((current) => `${current ? `${current}\n\n` : ""}【${file.name}】\n${content}`);
      }
    })();
  };

  // 挂在 document 上：光标不在输入框里（比如刚点了消息区）直接 ⌘V 也能收到
  const onPasteRef = useRef(onPaste);
  onPasteRef.current = onPaste;
  useEffect(() => {
    const listener = (event: ClipboardEvent) => onPasteRef.current(event);
    document.addEventListener("paste", listener);
    return () => document.removeEventListener("paste", listener);
  }, []);

  const loadMessages = useCallback(async () => {
    const list = await api.get<ChatMessage[]>(`/conversations/${key}/messages`);
    setMessages(list);
  }, [key]);

  useEffect(() => {
    (async () => {
      try {
        const [current, bots] = await Promise.all([
          api.get<Conversation>(`/conversations/${key}`),
          api.get<BotSummary[]>("/bots"),
          loadMessages(),
        ]);
        setConversation(current);
        setBot(bots.find((b) => b.botKey === current.botKey) ?? null);
      } catch (error) {
        setMessages((list) => list ?? []);
        Toast.show({ content: (error as Error).message });
      }
    })();
    // 从首页快捷入口进来时带着替你写好的开头（?q=），否则用本机保存的草稿
    const prompt = new URLSearchParams(window.location.search).get("q");
    if (prompt) {
      setDraft(prompt);
      setChip(null);
      window.history.replaceState(window.history.state, "", window.location.pathname);
      return;
    }
    try {
      setDraft(localStorage.getItem(`pa_draft_${key}`) || "");
      const saved = localStorage.getItem(`pa_draft_cmd_${key}`);
      setChip(saved ? JSON.parse(saved) : null);
    } catch {
      /* ignore */
    }
  }, [key, loadMessages]);

  // 回答在后台写完了（锁屏、切走 App 时连接会断），回来时以服务端为准；正在发送就交给 send 收尾
  const sendingRef = useRef(false);
  sendingRef.current = sending;
  useOnResume(() => {
    if (!sendingRef.current) loadMessages().catch(() => {});
  }, 5_000);

  useEffect(() => {
    try {
      if (draft) localStorage.setItem(`pa_draft_${key}`, draft);
      else localStorage.removeItem(`pa_draft_${key}`);
      if (chip) localStorage.setItem(`pa_draft_cmd_${key}`, JSON.stringify(chip));
      else localStorage.removeItem(`pa_draft_cmd_${key}`);
    } catch {
      /* ignore */
    }
  }, [draft, chip, key]);

  useLayoutEffect(() => {
    const element = scrollRef.current;
    if (element && stickRef.current) element.scrollTop = element.scrollHeight;
  }, [messages, streamText, height, sending]);

  // 往上翻得比较远时，右下角出现「回到最新」
  const [farFromBottom, setFarFromBottom] = useState(false);
  const onScroll = () => {
    const element = scrollRef.current;
    if (!element) return;
    const distance = element.scrollHeight - element.scrollTop - element.clientHeight;
    stickRef.current = distance < 80;
    setFarFromBottom(distance > 400);
  };
  const toBottom = () => {
    const element = scrollRef.current;
    if (!element) return;
    stickRef.current = true;
    element.scrollTo({ top: element.scrollHeight, behavior: "smooth" });
  };

  // 说的话边识别边写进输入框，说完可以改了再发
  const dictation = useDictation({ value: draft, onChange: setDraft, disabled: sending });
  const inputHistory = useInputHistory(draft, setDraft);
  // 没打字时右边的大按钮是麦克风（说话最省事），打了字变成发送；说话期间不换，免得按钮在手指下跳
  const bigVoice = useRef(true);
  if (!dictation.busy) bigVoice.current = !draft.trim() && !chip && !photos.length;

  const send = async (spoken?: string) => {
    const typed = spoken ?? draft;
    const command = chip;
    const text = ((command?.text ?? "") + typed).trim();
    const picked = photos;
    if ((!text && !picked.length) || sending) return;
    setSending(true);
    inputHistory.remember(text);
    setDraft("");
    setChip(null);
    setPhotos([]);
    setStreamText("");
    setStatusText("");
    if (picked.length) setPendingUser({ text, urls: picked.map((p) => p.url) });
    stickRef.current = true;
    let accepted = false;
    let noteKey: string | undefined;
    try {
      if (picked.length) {
        // 照片先存成原始记录（OSS），发消息时服务端当场整理成记忆，再回答
        setStatusText("正在上传照片…");
        const form = new FormData();
        form.set("text", text);
        form.set("channel", "capture");
        form.set("inline", "1");
        form.set("occurredAt", new Date().toISOString());
        picked.forEach((photo, index) => form.append("images", photo.blob, `${index}.jpg`));
        noteKey = (await postForm<Note>("/notes", form)).noteKey;
        setStatusText("正在看照片…");
      }
      await streamMessage<StreamEvent>(key, { text, noteKey }, (event) => {
        switch (event.type) {
          case "user_message":
            accepted = true;
            setPendingUser(null);
            setStatusText("");
            setMessages((list) => [...(list ?? []), event.message!]);
            break;
          case "status":
            setStatusText(event.text ?? "");
            break;
          case "tool_message":
          case "notice":
            setMessages((list) => [...(list ?? []), event.message!]);
            break;
          case "delta":
            setStreamText((value) => value + (event.text ?? ""));
            break;
          case "assistant_message":
            setStreamText("");
            setMessages((list) => [...(list ?? []), event.message!]);
            break;
          case "error":
            Toast.show({ content: event.error || "出错了" });
            break;
        }
      });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
      // 服务端还没收到这句话（断网、登录失效等），把原文放回输入框，免得重打；照片没传上去的也放回去
      if (!accepted) {
        // 指令标签一起放回去；这期间又选了别的指令或打了字就不覆盖
        setChip((current) => current ?? command);
        setDraft((current) => (spoken && current ? `${current}${typed.trim()}` : current || typed.trim()));
      }
      if (!noteKey && picked.length) setPhotos((list) => (list.length ? list : picked));
    } finally {
      setSending(false);
      setStreamText("");
      setStatusText("");
      setPendingUser(null);
      if (noteKey || accepted) picked.forEach((photo) => URL.revokeObjectURL(photo.url));
      // 以服务端为准重新拉一次（断网时回答仍在服务端写完）
      loadMessages().catch(() => {});
    }
  };

  const newConversation = async () => {
    if (!bot) return;
    if (messages?.length === 0) {
      Toast.show({ content: "已经是新对话了" });
      return;
    }
    try {
      const next = await api.post<Conversation>("/conversations", { botKey: bot.botKey });
      router.replace(`/chat/${next.conversationKey}`);
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  };

  const feedback = useCallback(async (message: ChatMessage, value: "up" | "down") => {
    let note = "";
    if (value === "down") {
      const result = await new Promise<string | null>((resolve) => {
        let input = "";
        Dialog.confirm({
          title: "哪里不对？",
          content: (
            <div style={{ background: "var(--pa-fill)", borderRadius: 12, padding: "10px 12px", marginTop: 4 }}>
              <TextArea
                placeholder="比如：她其实在意的是……；我实际是这么说的，结果……"
                autoSize={{ minRows: 3, maxRows: 6 }}
                onChange={(value) => (input = value)}
              />
            </div>
          ),
          confirmText: "提交",
          cancelText: "取消",
          onConfirm: () => resolve(input),
          onCancel: () => resolve(null),
        });
      });
      if (result === null) return;
      note = result;
    }
    try {
      await api.post(`/messages/${message.messageKey}/feedback`, { feedback: value, note });
      setMessages((list) => (list ?? []).map((m) => (m.messageKey === message.messageKey ? { ...m, feedback: value, feedbackNote: note } : m)));
      Toast.show({ content: value === "up" ? "收到，会多这样回答" : "收到，会用来改进" });
    } catch (error) {
      Toast.show({ content: (error as Error).message });
    }
  }, []);

  const tone = bot ? botTone(bot.botKey) : undefined;
  const avatar = bot?.avatar ?? "🤖";
  const showTitle = conversation && conversation.title !== "新对话";
  const suggestions = bot ? starters[bot.botKey] ?? (bot.kind === "assistant" ? starters.assistant : []) : [];
  const empty = messages !== null && messages.length === 0 && !sending;
  const commandList = mention ? filterCommands(commandsFor(bot?.botKey, bot?.kind), mention.query) : [];

  const onDraftChange = (value: string) => {
    setDraft(value);
    const caret = inputRef.current?.nativeElement?.selectionStart ?? value.length;
    const next = findMention(value, caret);
    setMention(next);
    if (next?.query !== mention?.query) setActiveCommand(0);
  };

  const pickCommand = (command: Command) => {
    if (!mention) return;
    const end = mention.start + 1 + mention.query.length;
    const next = draft.slice(0, mention.start) + draft.slice(end);
    const caret = mention.start;
    setDraft(next);
    setChip({ label: command.label, text: command.text });
    setMention(null);
    requestAnimationFrame(() => {
      const element = inputRef.current?.nativeElement;
      if (!element) return;
      element.focus();
      element.setSelectionRange(caret, caret);
    });
  };

  const onDraftKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (isSendKey(event)) {
      event.preventDefault();
      setMention(null);
      if (!dictation.busy) void send();
      return;
    }
    // 光标在最前面（或者没打字）时按回退：整块删掉指令标签，不用一个字一个字删
    const element = event.currentTarget;
    if (chip && event.key === "Backspace" && !event.nativeEvent.isComposing && element.selectionStart === 0 && element.selectionEnd === 0) {
      event.preventDefault();
      setChip(null);
      return;
    }
    if (!commandList.length) {
      if (!dictation.busy) inputHistory.onKeyDown(event);
      return;
    }
    if (event.nativeEvent.isComposing) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActiveCommand((index) => (index + step + commandList.length) % commandList.length);
    } else if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      pickCommand(commandList[Math.min(activeCommand, commandList.length - 1)]);
    } else if (event.key === "Escape") {
      event.preventDefault();
      setMention(null);
    }
  };

  return (
    <div className="chat-screen pa-page" style={{ height: height ? `${height}px` : "100dvh" }}>
      <NavBar
        leading={
          <button className="pa-icon-btn" onClick={() => setDrawer(true)} aria-label="最近的对话">
            <PanelLeft size={22} strokeWidth={1.9} />
          </button>
        }
        left={bot && <Avatar tone={tone} size={32}>{bot.avatar}</Avatar>}
        title={bot?.name ?? ""}
        subtitle={showTitle ? conversation.title : bot?.kind === "persona" ? "基于记忆的模拟" : "随时问，随时记"}
        right={bot && (
          <button className="pa-icon-btn" onClick={newConversation} aria-label="新对话">
            <SquarePen size={20} strokeWidth={1.9} />
          </button>
        )}
      />

      <div ref={scrollRef} className="pa-scroll chat-body" onScroll={onScroll}>
        {bot?.kind === "persona" && (
          <div className="chat-notice">
            <Info size={13} strokeWidth={2.2} />
            {bot.botKey === "daughter" ? "育儿档案顾问，不会假装成孩子说话" : "基于记忆的模拟，不是她本人，以真人沟通为准"}
          </div>
        )}

        {messages === null && <div className="pa-loading"><i /><i /><i /></div>}

        {empty && bot && (
          <div className="chat-intro">
            <Avatar tone={tone} size={64} shape="squircle">{bot.avatar}</Avatar>
            <div className="chat-intro-name">{bot.name}</div>
            <div className="chat-intro-sub">
              {bot.kind === "persona" ? "把想说的话先说给 TA 听听，也可以发照片" : "打字、说话、拍照都可以"}
            </div>
          </div>
        )}

        {empty && bot?.greeting && (
          <AssistantTurn avatar={avatar} name={bot.name} tone={tone}>
            <RichText text={bot.greeting} />
          </AssistantTurn>
        )}

        {messages?.map((message, index) => (
          <MessageView
            key={message.messageKey}
            message={message}
            avatar={avatar}
            name={bot?.name ?? ""}
            tone={tone}
            firstOfTurn={isFirstOfTurn(messages, index)}
            isFinal={isFinalAnswer(messages, index)}
            onMemory={setMemoryKey}
            onNote={showNote}
            onFeedback={feedback}
          />
        ))}

        {pendingUser && (
          <div className="chat-user chat-user-col">
            <div className="chat-user-photos">
              {pendingUser.urls.map((url) => <img key={url} src={url} alt="" />)}
            </div>
            {pendingUser.text && <div className="chat-user-bubble">{pendingUser.text}</div>}
          </div>
        )}

        {sending && (
          <AssistantTurn avatar={avatar} name={bot?.name ?? ""} tone={tone} showHead={!lastIsAssistantSide(messages) || !!pendingUser}>
            {streamText ? (
              <RichText text={streamText} streaming />
            ) : (
              <div className="chat-thinking"><span className="pa-typing"><i /><i /><i /></span>{statusText || "正在想"}</div>
            )}
          </AssistantTurn>
        )}

        {empty && suggestions.length > 0 && (
          <>
            <div className="chat-starters-title">可以这样问，点一下填进输入框</div>
            <div className="chat-starters">
              {suggestions.map((text, index) => (
                <button key={text} className="chat-starter" style={{ animationDelay: `${120 + index * 60}ms` }} onClick={() => setDraft(text)}>
                  <Sparkles size={14} strokeWidth={2} />
                  {text}
                </button>
              ))}
            </div>
          </>
        )}
      </div>

      <div className="chat-composer">
        <button className="chat-to-bottom" data-show={farFromBottom} onClick={toBottom} aria-label="回到最新">
          <ArrowDown size={18} strokeWidth={2.2} />
        </button>
        {canRehearse && !draft && !chip && !empty && (
          <button className="chat-rehearse" onClick={() => setChip({ label: "预演一句话", text: "我打算这么跟你说：" })}>
            <Sparkles size={13} strokeWidth={2.2} />
            预演一句我想说的话
          </button>
        )}
        <CommandMenu items={commandList} active={activeCommand} onPick={pickCommand} onHover={setActiveCommand} />
        <PickedPhotos
          urls={photos.map((p) => p.url)}
          onRemove={(index) => setPhotos((list) => {
            URL.revokeObjectURL(list[index].url);
            return list.filter((_, i) => i !== index);
          })}
        />
        <div className="chat-input" data-attach={canAttach} {...dictation.holdProps}>
          {canAttach && (
            <label className="chat-attach" aria-label="拍照或选照片">
              <Camera size={20} strokeWidth={1.9} />
              <input type="file" accept="image/*" multiple hidden onChange={(event) => { void pickPhotos(Array.from(event.target.files ?? [])); event.target.value = ""; }} />
            </label>
          )}
          {chip && (
            <button
              className="chat-chip"
              // 不抢焦点，键盘保持弹着
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => { setChip(null); inputRef.current?.focus(); }}
              aria-label={`去掉指令「${chip.label}」`}
            >
              <AtSign size={12} strokeWidth={2.6} />
              {chip.label}
              <X size={12} strokeWidth={2.6} className="chat-chip-x" />
            </button>
          )}
          <TextArea
            ref={inputRef}
            value={draft}
            onChange={onDraftChange}
            onKeyDown={onDraftKeyDown}
            // 晚一点关：手机上点菜单项时输入框会先失焦
            onBlur={() => setTimeout(() => setMention(null), 200)}
            placeholder={
              chip ? (chip.text.endsWith("：") ? "接着说…（按回退可以去掉前面的指令）" : "可以直接发，也可以补一句")
              : photos.length ? (
                bot?.kind === "persona" ? "想跟 TA 说说这张照片什么…"
                  : bot?.botKey === "remind" ? "要提醒什么？照片里的时间也会帮你看"
                  : bot?.botKey === "recall" ? "说说这是什么、干什么用、放在哪（按住这里就能说）"
                  : "想问这张照片什么，或者直接发"
              )
                : bot?.kind === "persona" ? "想对 TA 说什么…"
                : bot?.botKey === "recall" ? "问「…放哪了」，或拍张照记东西"
                : bot?.botKey === "remind" ? "说要做的事和时间，比如「明晚 8 点给女儿洗澡」"
                : "说点什么，输入 @ 选「记一下」等指令"
            }
            readOnly={dictation.busy}
            rows={1}
            autoSize={{ minRows: 1, maxRows: 6 }}
          />
          {dictation.supported && !sending && bigVoice.current ? (
            <VoiceButton className="chat-send chat-voice" dictation={dictation} />
          ) : (
            <>
              {/* 已经写了开头（比如「记一下：」）或选了照片，接着用说的 */}
              <VoiceButton dictation={dictation} disabled={sending} />
              <button className="chat-send" disabled={(!draft.trim() && !chip && !photos.length) || sending || dictation.busy} onClick={() => void send()} aria-label="发送">
                {sending ? <span className="pa-btn-spin" /> : <ArrowUp size={19} strokeWidth={2.6} />}
              </button>
            </>
          )}
        </div>
      </div>

      {dictation.overlay}
      <MemoryPopup memoryKey={memoryKey} onClose={() => setMemoryKey(null)} />
      <ConversationDrawer open={drawer} onClose={() => setDrawer(false)} activeKey={key} showHome />
      <NoteSheet note={openNote} onClose={() => setOpenNote(null)} onChanged={() => {}} onMemory={(key) => { setOpenNote(null); setMemoryKey(key); }} />

      <style jsx global>{`
        .chat-screen {
          display: flex;
          flex-direction: column;
          background: var(--pa-bg);
          overflow: hidden;
        }
        .chat-body { padding: 14px 20px 20px; }
        .chat-notice {
          display: flex;
          align-items: center;
          justify-content: center;
          gap: 6px;
          margin: 0 auto 10px;
          width: fit-content;
          max-width: 100%;
          padding: 6px 12px;
          border-radius: 14px;
          background: var(--pa-warning-soft);
          color: var(--pa-warning);
          font-size: 12px;
          font-weight: 500;
          text-align: center;
          line-height: 1.45;
        }
        .chat-turn { margin: 18px 0 6px; animation: pa-enter 0.35s var(--pa-ease) both; }
        .chat-turn-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
        .chat-turn-name { font-size: 13px; font-weight: 600; color: var(--pa-text-2); }
        .chat-user { display: flex; justify-content: flex-end; margin: 18px 0 4px; animation: pa-enter 0.3s var(--pa-ease) both; }
        .chat-user-bubble {
          max-width: 82%;
          padding: 10px 15px;
          border-radius: 20px 20px 6px 20px;
          background: var(--pa-ink-btn);
          color: var(--pa-ink-btn-text);
          font-size: 16px;
          line-height: 1.6;
          white-space: pre-wrap;
          word-break: break-word;
        }
        .chat-tools { display: flex; flex-wrap: wrap; gap: 6px; margin: 10px 0 2px; }
        .chat-tool {
          display: inline-flex;
          align-items: center;
          gap: 5px;
          height: 24px;
          padding: 0 9px;
          border-radius: 12px;
          background: var(--pa-fill);
          color: var(--pa-text-3);
          font-size: 12px;
          font-weight: 500;
        }
        .chat-tool[data-error="true"] { color: var(--pa-danger); background: var(--pa-danger-soft); }
        .chat-error {
          display: flex;
          gap: 8px;
          align-items: flex-start;
          margin: 12px 0;
          padding: 10px 12px;
          border-radius: 12px;
          background: var(--pa-danger-soft);
          color: var(--pa-danger);
          font-size: 13.5px;
          line-height: 1.5;
        }
        .chat-persona {
          margin: 14px auto 4px;
          max-width: 92%;
          padding: 8px 12px;
          border-radius: 12px;
          background: var(--pa-accent-soft);
          color: var(--pa-accent);
          font-size: 12.5px;
          line-height: 1.55;
          cursor: pointer;
        }
        .chat-persona-head { display: flex; gap: 6px; align-items: flex-start; }
        .chat-persona-head svg { flex-shrink: 0; margin-top: 2px; }
        .chat-persona-body { margin-top: 6px; color: var(--pa-text); font-size: 13.5px; }
        .chat-persona-link { display: block; margin-top: 6px; padding: 0; border: 0; background: none; color: var(--pa-accent); font-weight: 600; font-size: 13px; cursor: pointer; }
        .chat-help {
          display: flex;
          gap: 10px;
          align-items: flex-start;
          margin: 12px 0;
          padding: 12px 14px;
          border-radius: 14px;
          background: var(--pa-warning-soft);
          color: var(--pa-text);
          font-size: 14px;
          line-height: 1.6;
        }
        .chat-help svg { color: var(--pa-warning); }
        .chat-cite-warn {
          display: inline-flex;
          gap: 5px;
          align-items: center;
          margin-top: 6px;
          font-size: 12px;
          color: var(--pa-warning);
        }
        .chat-actions { display: flex; gap: 2px; margin: 8px 0 0 -8px; }
        .chat-action {
          height: 32px;
          min-width: 32px;
          padding: 0 8px;
          border: 0;
          border-radius: 16px;
          background: none;
          color: var(--pa-text-3);
          display: inline-flex;
          align-items: center;
          justify-content: center;
          gap: 4px;
          font-size: 12.5px;
          cursor: pointer;
          transition: background 0.15s, color 0.15s;
        }
        .chat-action:active { background: var(--pa-fill-strong); }
        .chat-action[data-on="true"] { color: var(--pa-accent); }
        .chat-thinking { display: flex; align-items: center; gap: 10px; color: var(--pa-text-3); font-size: 14px; height: 28px; }
        .pa-typing { display: inline-flex; gap: 4px; }
        .pa-typing i { width: 6px; height: 6px; }
        .chat-starters { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; margin-top: 10px; }
        .chat-starter {
          display: inline-flex;
          align-items: center;
          gap: 7px;
          padding: 9px 14px;
          border-radius: 18px;
          border: 0.5px solid var(--pa-line-strong);
          background: var(--pa-surface);
          color: var(--pa-text);
          font-size: 14px;
          cursor: pointer;
          transition: transform 0.15s, background 0.15s;
          animation: pa-enter 0.4s var(--pa-ease) both;
        }
        .chat-starter svg { color: var(--pa-accent); }
        .chat-starter:active { transform: scale(0.97); background: var(--pa-surface-2); }
        .chat-composer {
          z-index: 2;
          flex-shrink: 0;
          padding: 8px 12px calc(10px + env(safe-area-inset-bottom));
          background: linear-gradient(180deg, transparent 0%, var(--pa-bg) 14px);
        }
        .chat-rehearse {
          display: inline-flex;
          align-items: center;
          gap: 5px;
          margin: 0 0 8px 4px;
          padding: 6px 12px;
          border: 0;
          border-radius: 15px;
          background: var(--pa-accent-soft);
          color: var(--pa-accent);
          font-size: 13px;
          font-weight: 600;
          cursor: pointer;
        }
        .chat-input {
          display: flex;
          align-items: flex-end;
          gap: 8px;
          padding: 6px 6px 6px 16px;
          border-radius: 24px;
          background: var(--pa-elevated);
          border: 0.5px solid var(--pa-line-strong);
          box-shadow: var(--pa-shadow);
          transition: border-color 0.2s;
        }
        .chat-input:focus-within { border-color: color-mix(in srgb, var(--pa-accent) 50%, var(--pa-line-strong)); }
        .chat-input[data-attach="true"] { padding-left: 6px; }
        .chat-attach {
          flex-shrink: 0;
          width: 36px;
          height: 36px;
          border-radius: 18px;
          display: grid;
          place-items: center;
          color: var(--pa-text-2);
          cursor: pointer;
        }
        .chat-attach:active { background: var(--pa-fill-strong); }
        .chat-user-col { flex-direction: column; align-items: flex-end; gap: 6px; }
        .chat-user-photos { display: flex; gap: 6px; flex-wrap: wrap; justify-content: flex-end; max-width: 82%; }
        .chat-user-photos img { width: 76px; height: 76px; object-fit: cover; border-radius: 14px; opacity: 0.85; }
        .chat-input .adm-text-area { padding: 7px 0; }
        .chat-chip {
          flex-shrink: 0;
          align-self: flex-end;
          margin-bottom: 5px;
          height: 28px;
          padding: 0 7px 0 8px;
          display: inline-flex;
          align-items: center;
          gap: 3px;
          border: 0;
          border-radius: 14px;
          background: var(--pa-accent);
          color: var(--pa-accent-ink);
          font-size: 13.5px;
          font-weight: 650;
          white-space: nowrap;
          cursor: pointer;
          animation: pa-enter 0.18s var(--pa-ease) both;
        }
        .chat-chip-x { opacity: 0.75; margin-left: 1px; }
        .chat-chip:active { opacity: 0.8; }
        .chat-input .adm-text-area-element { line-height: 1.5; color: var(--pa-text); }
        .chat-input .adm-text-area-element::placeholder { color: var(--pa-text-3); }
        .chat-send {
          flex-shrink: 0;
          width: 36px;
          height: 36px;
          border-radius: 18px;
          border: 0;
          display: grid;
          place-items: center;
          background: var(--pa-accent);
          color: var(--pa-accent-ink);
          cursor: pointer;
          transition: transform 0.15s, opacity 0.2s, background 0.2s;
        }
        .chat-send:active { transform: scale(0.9); }
        .chat-send:not(:disabled) { animation: pa-pop 0.35s var(--pa-spring); }
        .chat-voice { background: var(--pa-ink-btn); color: var(--pa-ink-btn-text); }
        .chat-composer { position: relative; }
        .chat-to-bottom {
          position: absolute;
          right: 16px;
          top: -48px;
          width: 38px;
          height: 38px;
          border-radius: 19px;
          border: 0.5px solid var(--pa-line-strong);
          background: color-mix(in srgb, var(--pa-elevated) 88%, transparent);
          -webkit-backdrop-filter: blur(12px);
          backdrop-filter: blur(12px);
          box-shadow: var(--pa-shadow);
          color: var(--pa-text);
          display: grid;
          place-items: center;
          cursor: pointer;
          opacity: 0;
          transform: translateY(8px) scale(0.9);
          pointer-events: none;
          transition: opacity 0.2s, transform 0.3s var(--pa-spring);
        }
        .chat-to-bottom[data-show="true"] { opacity: 1; transform: none; pointer-events: auto; }
        .chat-intro { display: flex; flex-direction: column; align-items: center; text-align: center; padding: 18px 0 4px; animation: pa-enter 0.5s var(--pa-ease) both; }
        .chat-intro-name { font-family: var(--pa-font-display); font-size: 22px; font-weight: 700; margin-top: 12px; }
        .chat-intro-sub { font-size: 13px; color: var(--pa-text-3); margin-top: 3px; }
        .chat-starters-title { font-size: 12.5px; color: var(--pa-text-3); margin: 22px 0 0 2px; animation: pa-enter 0.4s var(--pa-ease) 80ms both; }
        .chat-send:disabled { background: var(--pa-fill-strong); color: var(--pa-text-3); cursor: default; transform: none; }
      `}</style>
    </div>
  );
}

/** 一轮回答里最后一条有正文的 assistant 消息才显示反馈按钮（后面跟的提示卡不算）。 */
function isFinalAnswer(messages: ChatMessage[], index: number): boolean {
  const message = messages[index];
  if (message.role !== "assistant" || message.status !== "done" || !message.content) return false;
  const next = messages[index + 1];
  return !next || next.turnKey !== message.turnKey || next.role === "notice";
}

/** 助理这一侧（工具 + 回答）连续出现时，只在第一条上显示头像和名字。 */
function isFirstOfTurn(messages: ChatMessage[], index: number): boolean {
  const previous = messages[index - 1];
  return !previous || previous.role === "user";
}

function lastIsAssistantSide(messages: ChatMessage[] | null): boolean {
  const last = messages?.[messages.length - 1];
  return !!last && last.role !== "user";
}

// 流式输出期间整页每个字都会重渲染，历史消息只要属性没变就跳过
const MessageView = memo(function MessageView({ message, avatar, name, tone, firstOfTurn, isFinal, onMemory, onNote, onFeedback }: {
  message: ChatMessage;
  avatar: string;
  name: string;
  tone?: ReturnType<typeof botTone>;
  firstOfTurn: boolean;
  isFinal: boolean;
  onMemory: (key: string) => void;
  onNote: (key: string) => void;
  onFeedback: (message: ChatMessage, value: "up" | "down") => void;
}) {
  if (message.role === "user") {
    return (
      <>
        {message.meta?.personaUpdates?.map((update) => <PersonaUpdateNotice key={update.person} update={update} />)}
        <div className="chat-user chat-user-col">
          {message.meta?.attachment && (
            <PhotoStrip
              size="sm"
              align="end"
              photos={Array.from({ length: message.meta.attachment.photos }, (_, index) => ({ noteKey: message.meta!.attachment!.noteKey, index }))}
            />
          )}
          <div className="chat-user-bubble">{message.content}</div>
        </div>
      </>
    );
  }
  if (message.role === "notice") {
    return (
      <div className="chat-help">
        <LifeBuoy size={16} style={{ flexShrink: 0, marginTop: 2 }} />
        <div style={{ whiteSpace: "pre-wrap" }}>{message.content}</div>
      </div>
    );
  }
  if (message.role === "tool") {
    const names = Array.from(new Set((message.blocks ?? []).map((block) => (block.isError ? "查询出错" : toolText[block.toolName ?? ""] ?? block.toolName ?? ""))));
    return (
      <AssistantTurn avatar={avatar} name={name} tone={tone} showHead={firstOfTurn} tight>
        <div className="chat-tools">
          {names.map((text) => (
            <span key={text} className="chat-tool" data-error={text === "查询出错"}>
              {text === "查询出错" ? <TriangleAlert size={12} /> : <Check size={12} strokeWidth={2.6} />}
              {text}
            </span>
          ))}
        </div>
      </AssistantTurn>
    );
  }
  if (message.status === "error") {
    return (
      <AssistantTurn avatar={avatar} name={name} tone={tone} showHead={firstOfTurn} tight>
        <div className="chat-error"><TriangleAlert size={16} style={{ flexShrink: 0, marginTop: 2 }} />没有回答成功：{message.error}</div>
      </AssistantTurn>
    );
  }
  if (!message.content) return null;
  return (
    <AssistantTurn avatar={avatar} name={name} tone={tone} showHead={firstOfTurn}>
      <RichText text={message.content} onMemory={onMemory} onNote={onNote} />
      {message.meta?.photos?.length ? (
        <PhotoStrip photos={message.meta.photos.map((photo) => ({ noteKey: photo.noteKey, index: photo.index, caption: photo.title }))} />
      ) : null}
      {message.meta?.citations?.unknown?.length ? (
        <div className="chat-cite-warn">
          <TriangleAlert size={12} />
          有 {message.meta.citations.unknown.length} 处引用找不到出处，这部分别当真
        </div>
      ) : null}
      {isFinal && <AnswerActions message={message} onFeedback={onFeedback} />}
    </AssistantTurn>
  );
});

/** 对话进行中，某人的人设卡更新了：助理这一轮已经知道变化，这里也告诉我。点开看全部变化。 */
function PersonaUpdateNotice({ update }: { update: { person: string; from: number; to: number; changes: string } }) {
  const persons = usePersons();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  return (
    <div className="chat-persona" onClick={() => setOpen(!open)}>
      <div className="chat-persona-head">
        <UserRoundPen size={13} strokeWidth={2.2} />
        <span>{personLabel(persons, update.person)}的人设有了新变化（第 {update.to} 版），这一轮已按新的理解来回答</span>
      </div>
      {open && (
        <div className="chat-persona-body">
          {update.changes || "（没有写变化说明）"}
          <button className="chat-persona-link" onClick={(event) => { event.stopPropagation(); router.push(`/person/${update.person}#changes`); }}>看 TA 的全部变化</button>
        </div>
      )}
    </div>
  );
}

function AnswerActions({ message, onFeedback }: { message: ChatMessage; onFeedback: (message: ChatMessage, value: "up" | "down") => void }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(message.content.replace(/\s*\[mem_[0-9a-f]+(?:[,，、\s]+mem_[0-9a-f]+)*\]|\s*\[note_[0-9a-f]+\]/g, ""));
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      Toast.show({ content: "复制失败" });
    }
  };
  return (
    <div className="chat-actions">
      <button className="chat-action" onClick={copy} aria-label="复制">
        {copied ? <Check size={16} /> : <Copy size={16} />}
      </button>
      <button className="chat-action" data-on={message.feedback === "up"} onClick={() => onFeedback(message, "up")} aria-label="有用">
        <ThumbsUp size={16} fill={message.feedback === "up" ? "currentColor" : "none"} />
      </button>
      <button className="chat-action" data-on={message.feedback === "down"} onClick={() => onFeedback(message, "down")} aria-label="不对">
        <ThumbsDown size={16} fill={message.feedback === "down" ? "currentColor" : "none"} />
      </button>
    </div>
  );
}

function AssistantTurn({ avatar, name, tone, showHead = true, tight, children }: {
  avatar: string;
  name: string;
  tone?: ReturnType<typeof botTone>;
  showHead?: boolean;
  tight?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="chat-turn" style={!showHead ? { marginTop: tight ? 4 : 10 } : undefined}>
      {showHead && (
        <div className="chat-turn-head">
          <Avatar tone={tone} size={26}>{avatar}</Avatar>
          <span className="chat-turn-name">{name}</span>
        </div>
      )}
      {children}
    </div>
  );
}
