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

export interface Skill {
  skillKey: string;
  name: string;
  whenToUse: string;
  body: string;
  botKeys: string[];
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

export interface TurnMeta {
  retrieval?: { skipped: boolean; hits: number; deduped: number; clipped: number };
  safety?: { signals?: string[]; helpMentioned?: boolean; noticeAdded?: boolean };
  segment?: { rolled: boolean; from: number; to: number; tokens?: number; summaryChars?: number; lost?: string[] };
  personaUpdates?: { person: string; from: number; to: number; changes: string }[];
  citations?: { cited: string[]; unknown?: string[] };
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
  meta: TurnMeta | null;
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
  createdBy: "auto" | "manual" | "rollback" | "chat";
  createdAt: string;
}

/** 对话补充人设卡的一条消息（不落库，前端每轮带上完整记录）。 */
export interface PersonaTurn {
  role: "user" | "assistant";
  text: string;
}

export interface Reminder {
  reminderKey: string;
  kind: string;
  title: string;
  body: string;
  eventAt: string | null;
  remindAt: string;
  status: string;
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
  status: string;
  brief?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SafetySettings {
  crisisWords: string[];
  excludePhrases: string[];
  helpWords: string[];
  notice: string;
}

export interface SafetyHit {
  messageKey: string;
  conversationKey: string;
  content: string;
  signals: string[];
  helpMentioned: boolean | null;
  noticeAdded: boolean;
  createdAt: string;
}

export interface SkillSuggestion {
  body: string;
  changes: string[];
  gaps: string[];
  memories: number;
}

export interface SegmentRoll {
  messageKey: string;
  conversationKey: string;
  title: string;
  to: number;
  tokens: number;
  summaryChars: number;
  lost: string[];
  createdAt: string;
}

export interface SkillExample {
  exampleKey: string;
  skillKey: string;
  situation: string;
  goodResponse: string;
  badResponse: string;
  why: string;
  status: "pending" | "approved" | "rejected";
  source: string;
  sourceMessageKey: string;
  createdAt: string;
}

export interface MemoryMergeSuggestion {
  mergeKey: string;
  a: Memory | null;
  b: Memory | null;
  similarity: number;
  status: string;
  keptKey: string;
  createdAt: string;
}

export interface JobRun {
  job: string;
  period: string;
  result: string;
  createdAt: string;
}
