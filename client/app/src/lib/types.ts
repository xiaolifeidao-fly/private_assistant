export interface Principal {
  accountKey: string;
  username: string;
  displayName: string;
  realm: string;
  role: string;
  familyKey: string;
}

export interface Account {
  accountKey: string;
  realm: "manager" | "app";
  username: string;
  displayName: string;
  role: string;
  familyKey: string;
  personKey: string;
  status: string;
  lastLoginAt: string | null;
  createdAt: string;
}

export interface Person {
  personKey: string;
  name: string;
  roleLabel: string;
  aliases: string[];
  gender: string;
  birthday: string;
  lunarBirthday: string;
  birthdayBy: "solar" | "lunar";
  homePlaceKey: string;
  side: string;
  importance: number;
  isCore: boolean;
  profile: string;
  avatar: string;
  sort: number;
  status: string;
}

export interface Relation {
  relationKey: string;
  fromPerson: string;
  toPerson: string;
  relation: string;
  addressAs: string;
  childAddressAs: string;
  closeness: number;
  notes: string;
  sensitivities: string;
}

export interface Place {
  placeKey: string;
  name: string;
  kind: string;
  address: string;
  dialect: string;
  description: string;
  customs: string;
  travelTips: string;
  sort: number;
}

export interface MemoryType {
  type: string;
  label: string;
  hint: string;
  decays: boolean;
}

export interface Memory {
  memoryKey: string;
  type: string;
  typeLabel: string;
  scope: string;
  subjectPerson: string;
  placeKey: string;
  persons: string[];
  title: string;
  content: string;
  attrs: Record<string, unknown> | null;
  importance: number;
  confidence: number;
  status: "pending" | "active" | "superseded" | "archived" | "rejected";
  occurredAt: string | null;
  validFrom: string | null;
  validTo: string | null;
  supersedesKey: string;
  sourceNoteKey: string;
  createdBy: string;
  useCount: number;
  lastUsedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SearchHit {
  memory: Memory;
  score: number;
  vector: number;
  text: number;
}

export interface Bot {
  botKey: string;
  name: string;
  kind: "assistant" | "persona";
  personKey: string;
  avatar: string;
  description: string;
  greeting: string;
  systemPrompt: string;
  routeTask: string;
  tools: string[];
  memoryScope: "all" | "person";
  enabled: boolean;
  sort: number;
  version: number;
  updatedAt: string;
}

export interface Provider {
  providerKey: string;
  name: string;
  kind: "anthropic" | "openai";
  baseUrl: string;
  apiKey?: string;
  hasApiKey: boolean;
  enabled: boolean;
}

export interface RouteConfig {
  task: string;
  providerKey: string;
  model: string;
  maxTokens: number;
  effort: string;
  temperature: number | null;
  description: string;
}

export interface LlmCall {
  callKey: string;
  task: string;
  providerKey: string;
  model: string;
  refKey: string;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  latencyMs: number;
  stopReason: string;
  status: string;
  error: string;
  createdAt: string;
}

export interface Conversation {
  conversationKey: string;
  accountKey: string;
  botKey: string;
  title: string;
  status: string;
  messageCount: number;
  lastMessageAt: string | null;
  createdAt: string;
}

export interface Block {
  type: "text" | "tool_use" | "tool_result";
  text?: string;
  toolUseId?: string;
  toolName?: string;
  input?: unknown;
  isError?: boolean;
}

export interface ChatMessage {
  messageKey: string;
  turnKey: string;
  segment: number;
  role: "user" | "assistant" | "tool" | "notice";
  content: string;
  blocks: Block[] | null;
  model: string;
  usage: Record<string, number> | null;
  meta: {
    citations?: { cited: string[]; unknown?: string[] };
    /** 这一轮开始前，对话里涉及的人的人设卡有了新版本 */
    personaUpdates?: { person: string; from: number; to: number; changes: string }[];
    /** 我这句话带的照片（原始记录）和当场整理出的记忆 */
    attachment?: { noteKey: string; photos: number; status: string; summary?: string; memories?: string[]; error?: string };
    /** 回答里引用到的记忆所附的照片 */
    photos?: { memoryKey: string; title: string; noteKey: string; index: number }[];
  } | null;
  status: string;
  error: string;
  feedback: string;
  feedbackNote: string;
  createdAt: string;
}

export interface Paged<T> {
  items: T[];
  total: number;
}

export interface NoteResult {
  summary: string;
  added: string[] | null;
  pending: string[] | null;
  updated: string[] | null;
  duplicates: string[] | null;
  unknownPersons: string[] | null;
}

export interface Note {
  noteKey: string;
  channel: string;
  rawText: string;
  attachments: { path: string; mediaType: string; size: number }[];
  aboutPersons: string[];
  occurredAt: string | null;
  extractStatus: "queued" | "running" | "done" | "failed";
  extractAttempts: number;
  extractedAt: string | null;
  result: NoteResult | null;
  extractError: string;
  createdBy: string;
  createdAt: string;
}

export interface PersonaCard {
  cardKey: string;
  personKey: string;
  version: number;
  content: string;
  changes: string;
  memoryCount: number;
  memoryCutoff: string | null;
  status: "active" | "history";
  createdBy: "auto" | "manual" | "rollback";
  createdAt: string;
}

export interface Reminder {
  reminderKey: string;
  kind: "manual" | "birthday" | "commitment" | "dated" | "festival" | "trip" | "daily";
  title: string;
  body: string;
  eventAt: string | null;
  remindAt: string;
  status: "scheduled" | "sending" | "sent" | "failed" | "dismissed" | "cancelled";
  sourceType: string;
  sourceKey: string;
  targetUrl: string;
  attempts: number;
  lastError: string;
  sentAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Trip {
  tripKey: string;
  placeKey: string;
  title: string;
  startAt: string;
  endAt: string | null;
  people: string[];
  notes: string;
  status: "planned" | "done" | "cancelled";
  brief?: string;
  createdAt: string;
  updatedAt: string;
}
