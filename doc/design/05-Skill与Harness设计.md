# 05 · Skill 与 Harness

M0–M4 把「能记、能查、能提醒」做齐了。这一篇解决的是回答质量的四个问题：

| 问题 | 现象 | 这一篇的做法 |
|---|---|---|
| 准确率 | 所有场景共用一段大提示词；引用的 `[mem_xxx]` 没人核对 | Skill 按场景装载；回答后校验引用 |
| 可用性 | 「提到离婚、动手、自伤要建议专业帮助」只写在提示词里 | 代码规则兜底，漏了就补一张固定提示卡 |
| 可维护性 | 想调「回老家」只能改整段提示词，牵一发动全身；改完不知道变好还是变坏 | 每个 Skill 独立编辑、独立版本；评测集回归 |
| 上下文可控 | 历史无限增长、每轮固定检索 12 条、工具结果不限长 | 上下文预算 + 会话分段 |

原则不变：**只追加、不改写**。会话一旦开始，系统提示词和已发出的历史一个字节都不动（Claude 的 thinking 签名和提示词缓存都依赖这一点）。下面每一项设计都先过这一关。

---

## 1. Skill：按场景装载的做事手册

### 1.1 是什么

Skill 是一段「遇到某类事该怎么想、先查什么、怎么说、别做什么」的手册。系统提示词里只放**目录**（每条一行：什么时候用），正文由模型在需要时调用 `load_skill` 读取。

```
系统提示词（会话开始冻结）
  ├─ 机器人提示词：身份、三件事、立场、引用格式      ← 瘦身，只留通用部分
  ├─ 家庭档案 / 关系 / 人设卡 / 常驻记忆
  └─ 做事手册目录：
       - mother_wife：婆媳冲突 —— 我妈和老婆有摩擦、我夹在中间时
       - hometown_visit：回老家准备 —— 要回老家、走亲戚、过年过节前
       …
对话历史（只追加）
  user:      <turn_context>… + 我老婆和我妈又因为孩子穿衣服吵起来了
  assistant: tool_use load_skill{name: mother_wife}
  tool:      # 婆媳冲突（v1）…正文…                ← 读进来就留在历史里
  assistant: tool_use search_memory …
  …
```

为什么正文走工具结果而不是拼进系统提示词：

- 系统提示词冻结后不能改；工具结果是追加进历史的，天然符合只追加。
- 读过一次，整个会话里都在，后面不用重读；也会被提示词缓存覆盖。
- 没用到的手册不占上下文。8 本手册全文约 6k 字，目录只要 400 字。

### 1.2 存储

`pa_skill`（迁移 `0006_skill_harness.sql`）：

| 字段 | 说明 |
|---|---|
| skill_key | 唯一键，也是 `load_skill` 的参数，如 `mother_wife` |
| name | 显示名 |
| when_to_use | 一句话「什么时候用」，进目录 |
| body | 正文（Markdown） |
| bot_keys | 哪些机器人能用，JSON 数组；默认只给 `assistant` |
| enabled / sort / version | 正文或适用条件变了版本 +1 |

出厂内容写在 `service/skill/defaults.go`，启动时只补缺不覆盖（和机器人默认值同一套做法），管理端可以「恢复默认」。

### 1.3 运行时规则

- 目录只给**有 `load_skill` 工具**且**在 bot_keys 里**的机器人；随系统提示词一起冻结。
- `load_skill` 执行时再查一次：手册存在、启用、对这个机器人开放，否则返回错误。
- 手册正文以 `# 名称（v3）` 开头，回看对话时能对上是哪一版。
- 会话进行中改了手册：已经读进历史的保持原样；之后新读的是新版。

### 1.4 第一批手册

| key | 名称 | 何时用 |
|---|---|---|
| mother_wife | 婆媳冲突 | 我妈和老婆有摩擦、我夹在中间 |
| wife_repair | 让老婆感到被在乎 | 她觉得我不关心、不尊重、记不住她的话；要道歉或修复 |
| hometown_visit | 回老家准备 | 回老家、走亲戚、过年过节前 |
| gifts_favors | 人情往来 | 送礼、随份子、红白喜事、借钱、帮忙 |
| commitment_followup | 承诺跟进 | 盘点我答应过的事、她提过的诉求 |
| parenting_split | 带孩子的分歧 | 老婆和我妈在带孩子上意见不同 |
| rehearsal | 话术预演 | 想好了要说的话，先请分身预演 |
| crisis | 危机信号 | 冷战很久、提离婚、动手、有人情绪低落或想伤害自己 |

助理提示词随之瘦身：通用的「出主意五段式」和立场留在提示词里；`ask_persona` 的用法挪进 `rehearsal`，危机处理挪进 `crisis` 并有代码兜底（见 2.2）。

> 已有库里的助理提示词不会被自动覆盖。到管理端「机器人 → 私人助理 → 恢复默认提示词」再保存即可换成新版；不换也能用，目录照样会拼进去。

---

## 2. Harness：回答前后的程序化把关

都在 `service/chat` 里，三个新文件：`budget.go`（上下文预算）、`guard.go`（校验与兜底）、`segment.go`（会话分段）。

### 2.1 引用校验

回答结束后，扫描正文里的 `mem_xxx`，和「这段对话里模型见过的记忆编号」比对：

- 见过的范围 = 冻结的系统提示词 + 当前分段全部历史（turn_context、工具结果、分段摘要）。
- 没见过的编号记进这条消息的 `meta.citations.unknown`，管理端标红；手机端点开这类编号时提示「这条引用没有出处」。
- 不改正文（已经流式发出去了，也不能改历史）。

### 2.2 危机兜底

- 用户这句话命中危机关键词（离婚、动手/打我、不想活、自杀、伤害自己、抑郁……）时：
  1. 在这一轮 `<turn_context>` 里加一行 `<safety>`，要求先读 `crisis` 手册；
  2. 回答结束后检查正文里是否给出了求助建议（咨询、专业、热线、医生……）；
  3. 没有的话，追加一条 `role=notice` 的固定提示卡（心理援助热线 12356、紧急情况 110/120、婚姻家庭咨询）。
- `notice` 只给人看，不进模型历史。
- 关键词命中只是「可能」，所以只有模型漏了才补卡，避免「她闺蜜离婚了」这种话也弹卡。

### 2.3 上下文预算

| 部分 | 现在 | 改成 |
|---|---|---|
| 每轮检索 | 固定 12 条 | 寒暄（好的、谢谢、嗯……，8 个字以内）跳过；已在常驻记忆里的去重；总长上限 4000 字 |
| 工具结果 | 不限 | 单个结果上限 6000 字，超出截断并提示「缩小范围再查」 |
| 历史 | 全量回放 | 超过预算后分段（2.4） |

每轮把这些决定写进用户消息的 `meta.retrieval`：检索了几条、去重了几条、是否跳过。

### 2.4 会话分段（长对话压缩）

- 每轮开始前看上一条 assistant 消息的输入 token（输入 + 缓存读 + 缓存写）。超过 **60k** 时，先把当前分段压缩成交接摘要（路由 `chat.summary`），然后 `segment + 1`。
- 新分段的第一条用户消息 = `<previous_summary>…</previous_summary>` + turn_context + 原话。
- `history()` 只回放当前分段。旧分段原样留在库里，手机端和管理端照常显示（管理端画一条分隔线）。
- 系统提示词不变，所以缓存仍然命中；新分段是全新的历史前缀，不存在改写旧 thinking 块的问题。
- 摘要失败就不分段，继续用全量历史，并记日志——宁可贵一点，不能丢上下文。

摘要要求写清：已确认的事实（带记忆编号）、我做过的决定、没办完的事、读过哪些手册。

### 2.5 每轮记录（meta）

`pa_message.meta` JSON：

```jsonc
// 用户消息
{ "retrieval": { "skipped": false, "hits": 9, "deduped": 3 },
  "safety": { "signals": ["离婚"] },
  "segment": { "rolled": true, "from": 0, "to": 1 } }
// 最终回答
{ "citations": { "cited": ["mem_a…"], "unknown": ["mem_zz…"] },
  "safety": { "helpMentioned": false, "noticeAdded": true } }
```

读过哪些手册看工具消息即可，不重复记。

---

## 3. 评测集

没有评测就无法判断改了提示词或手册之后是变好还是变坏，所以先建评测集再调内容。

### 3.1 结构

独立的 Go 模块（加进了 `go.work`），不往 `service` 里塞数据库驱动和评测代码：

```
server/eval/
  fixture.json        虚构的家庭数据（人物用迁移里的默认成员，这里补档案、规矩和 15 条记忆）
  cases/*.json        用例（assistant.json 10 条、persona.json 2 条）
  main.go             入口：检查库名 → 迁移 → 清空并装 fixture → 跑用例 → 写报告
  fixture.go / runner.go / report.go
  configs/application.example.properties
  reports/            每次的报告（不进 git，里面有完整对话）
```

每次运行前清空记忆、对话、人设卡、提醒，并把机器人和手册**重置成代码里的出厂内容**——评测的是仓库里的提示词和手册。想评测管理端里改过的版本，先把改动同步回 `bot/defaults.go`、`skill/defaults.go`。

用例：

```json
{
  "name": "婆媳-孩子穿衣",
  "bot": "assistant",
  "turns": ["我妈又说老婆给孩子穿少了，老婆当场就不高兴了，我该怎么办"],
  "expect": {
    "tools": ["search_memory"],
    "skills": ["mother_wife"],
    "cites": ["@wife_kids_clothes"],
    "mustNotContain": ["你妈说得对", "你老婆太敏感"],
    "rubric": ["没有评判谁对谁错", "建议先私下分别沟通，而不是当场裁判", "给出了可以直接说出口的话"]
  }
}
```

- `@xxx` 指 fixture 里的记忆别名，装数据时换成真实编号。
- 所有用例自动附加两条硬断言：无未知引用、没有失败消息。
- `rubric` 由模型打分（路由 `eval.judge`，结构化输出 `{items:[{rubric, pass, reason}]}`）。

### 3.2 运行

必须用**独立的库**（库名必须含 `eval`，否则拒绝运行，防止清掉真实数据）：

```sql
CREATE DATABASE private_assistant_eval DEFAULT CHARSET utf8mb4;
```

```bash
cp server/eval/configs/application.example.properties server/eval/configs/application.properties
```

填好 `sqlconn` 和 `llm.anthropic.api_key` 后：

```bash
cd server/eval && go run .
```

- `-case 婆媳`：只跑名字包含它的用例
- `-repeat 3`：每个用例跑三遍，看稳定性
- `-skip-judge`：只跑程序断言，不调用模型打分

全部通过退出码为 0，否则为 1，可以直接接 CI。

### 3.3 从点踩生成用例

管理端对话记录里，被点踩的回答多一个「转成评测用例」按钮：把这轮之前的用户原话和你的反馈（作为 rubric）生成用例 JSON 复制到剪贴板。**里面是真实对话**，提交进仓库前要改写成 fixture 里的虚构内容。

---

## 4. 改动清单

| 位置 | 改动 |
|---|---|
| `migrations/sql/0006_skill_harness.sql` | `pa_skill`；`pa_message.meta / segment`；`pa_conversation.segment`；`load_skill` 加给助理；路由 `chat.summary`、`eval.judge` |
| `service/skill` | 新包：存取、出厂手册 |
| `service/bot` | `load_skill` 工具名；助理提示词瘦身 |
| `service/chat` | `load_skill` 工具、目录拼接；`budget.go`、`guard.go`、`segment.go`；`notice` 消息 |
| `server/eval` | 评测模块 + fixture + 首批 12 条用例 |
| `manager-api` | `/skills` 增改查、恢复默认；评测用例导出 |
| `client/manager` | 「做事手册」页；对话记录显示 meta、分段线、导出用例 |
| `client/app` | 渲染 notice 卡；可疑引用提示 |

## 5. 不做的

- 不做通用 agent 框架、不引入 MCP：只有一个人用，现有工具循环够了。
- 不自动改写正文、不重试生成：校验只记录和补卡，保持只追加。
- 不用模型做危机判断：关键词规则可预期，误报的代价只是一次检查。
