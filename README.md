# 家庭关系助理（private_assistant）

一个只给自己用的「家庭关系外脑」：记住家里人说过的话、在意的事和老家的人情规矩，在需要的时候帮我想清楚怎么做、怎么说。

- 设计方案：[doc/design/01-总体设计.md](doc/design/01-总体设计.md)
- 实现说明：[M0 + M1](doc/design/02-M1实现说明.md) · [M2 + M3](doc/design/03-M2M3实现说明.md) · [M4](doc/design/04-M4实现说明.md) · [Skill 与 Harness](doc/design/05-Skill与Harness设计.md) · [越用越准](doc/design/06-越用越准.md) · [万能通](doc/design/07-万能通.md)

## 目录

```
server/                 Go 1.24，go.work 多模块
  common/               启动、统一响应、鉴权中间件、迁移执行器、JSON 列类型
  migrations/           建表 SQL（嵌入二进制，启动时自动执行）
  service/              领域层：identity / family / memory / bot / skill / setting / llm / chat / intake / persona / reminder / insight，wire 负责装配
  eval/                 回答质量评测集：虚构家庭数据 + 用例，跑在单独的 *_eval 库上
  data/                 截图等附件（没配 OSS 时；运行时生成，不进 git）
  manager-api/          管理端接口 :10013
  app-api/              手机端接口 :10012（对话走 SSE 流式）
client/
  manager/              管理端：Next.js 14 + antd 5，端口 7905
  app/                  手机端 PWA：Next.js 14 + antd-mobile 5，端口 7906
```

## 本地运行

### 1. 数据库

用服务器上现有的 MySQL（5.7.8+ 或 8.x 都行，需要 ngram 全文解析器，MySQL 自带）。先建库：

```sql
CREATE DATABASE private_assistant DEFAULT CHARSET utf8mb4;
```

表结构不用手动建：两个服务启动时都会执行 `server/migrations/sql` 里还没跑过的脚本（用 `GET_LOCK` 串行化，同时启动也没问题）。

### 2. 后端配置

```bash
cp server/manager-api/configs/application.example.properties server/manager-api/configs/application.properties
```

```bash
cp server/app-api/configs/application.example.properties server/app-api/configs/application.properties
```

两个文件里都要填：
- `sqlconn`：指向同一个库，例如 `user:pass@tcp(你的服务器:3306)/private_assistant?charset=utf8mb4&parseTime=True&loc=Local`
- `push.vapid_public_key` / `push.vapid_private_key` / `push.vapid_subject`：需要 iPhone Web Push 时填，两个服务必须相同

manager-api 里额外填：
- `bootstrap.admin_username` / `bootstrap.admin_password`：管理端首个管理员（只在还没有任何管理员时创建）
- `llm.anthropic.api_key`：Claude 的 API Key（也可以启动后在管理端「模型」里配）
- `llm.embedding.base_url` / `llm.embedding.api_key`：向量化服务（OpenAI 兼容 `/embeddings`，例如通义 `https://dashscope.aliyuncs.com/compatible-mode/v1` + 模型 `text-embedding-v3`）。不配也能用，检索退化为全文 + 结构化

app-api 里额外填：
- `bootstrap.app_username` / `bootstrap.app_password`：手机端账号（也可以之后在管理端「账号」里建）

附件（截图、照片）：配了 `storage.oss.*`（阿里云 OSS 私有桶）就存 OSS，否则存 `storage.dir`。两个文件要配成一样；切到 OSS 后 `storage.dir` 里的旧附件仍能读到。见 [07 · 万能通 §3](doc/design/07-万能通.md)。

配置项都可以用环境变量覆盖：`PA_` + 大写、点换下划线，例如 `PA_SQLCONN`、`PA_LLM_ANTHROPIC_API_KEY`。

### 3. 启动

```bash
cd server/manager-api && go run .
```

```bash
cd server/app-api && go run .
```

```bash
cd client/manager && npm install && npm run dev
```

```bash
cd client/app && npm install && npm run dev
```

- 管理端：http://localhost:7905/assistant-manager
- 手机端：http://localhost:7906/assistant（手机和电脑同一局域网时，用电脑 IP 访问）
- 访问前缀是 next.config.mjs 里的 `basePath`，构建期定死，`BASE_PATH=/xxx ./build.sh` 可改，设成空字符串就挂在根路径。
- 后端接口前缀是 application.properties 的 `server.context_path`（app-api `/assistant`、manager-api `/assistant-manager`），要和前端 `BASE_PATH` 一致。

### 4. 第一次用

1. 登录管理端，到「模型」确认 `anthropic` 提供商有 API Key，点各条路由的「测试」。
2. 「家庭成员」里把占位的「老婆 / 妈妈 / 女儿 / 岳父 / 岳母」改成真实名字、生日、别名，再把两边老家的亲戚加进来，在「人物关系」里写清称呼、要点和雷区。
3. 「记忆」里先录几条最重要的：她的性格、雷区、最近在意的事、你答应过的事。
4. 手机上打开手机端，登录后用 Safari「分享 → 添加到主屏幕」。

## 运维脚本

`server/app-api`、`server/manager-api`、`client/app`、`client/manager` 下各有一套：

| 脚本 | 作用 |
| --- | --- |
| `build.sh` | 编译（Go 出 `bin/<名字>`，前端出 `.next`） |
| `start.sh` / `stop.sh` | 后台起停，pid 在 `run/`，日志在 `logs/`；不会去杀不是自己起的进程 |
| `redev.sh` | 改完代码一键重编重启 |
| `package.sh` | 打部署包：后端 `<名字>-linux-amd64.tar.gz`，前端 `pa-app/pa-manager-linux-x64.tar.gz` |
| `unpack-release.sh` | 和 tar 包放在部署机同一目录下执行，解包到同级目录 |
| `deploy.sh` | 和 tar 包、`unpack-release.sh` 放同一目录，一条命令按 stop → 解包 → start 换版本；后端缺 `application.properties` 时只解包不启动 |

- 后端包不带 `configs/application.properties`，首次部署从 example 复制；`storage.dir=../data` 落在解包目录的上一级，两个服务放同一个父目录就自然共享。
- 前端 `/api` 的转发目标是**构建期**定死的：后端不在同机默认端口时，打包前带上 `APP_API=http://… ./package.sh` / `MANAGER_API=…`。
- 前端部署包在部署机首次 `start.sh` 时执行 `npm ci --omit=dev`，之后只在 lockfile 变化时重装。

## 部署要点

- **必须 HTTPS**：iOS 的「添加到主屏幕」以后的推送、Service Worker 都要求 HTTPS。
- 前端和后端接口都带访问前缀（手机端 `/assistant`、管理端 `/assistant-manager`），nginx 按前缀原样转发，**不要**去掉前缀。
- nginx 把 `/assistant/api` 直接转给后端，**对话接口要关缓冲**，否则流式输出会变成一次性返回：

```nginx
location /assistant/api/ {
    proxy_pass http://127.0.0.1:10012;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_buffering off;
    proxy_read_timeout 900s;
}
location /assistant/ {
    proxy_pass http://127.0.0.1:7906;
}
```

- 管理端不要暴露到公网（内网 / VPN / IP 白名单）。
- 每天加密备份数据库。

## 测试

```bash
cd server/service && go test ./...
```

```bash
cd server/common && go test ./...
```
