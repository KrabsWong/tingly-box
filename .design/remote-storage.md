# Remote 存储重设计

Remote 子系统（IM bot 远程控制）的状态持久化方案。记录现状问题、目标 schema、
分阶段迁移路径。

目录：

1. 现状盘点
2. 问题清单（带证据）
3. 设计原则
4. 目标 schema
5. 抽象与依赖方向
6. 数据迁移
7. 分阶段落地
8. 待决策事项

---

## 1. 现状盘点

Remote 的状态散落在四种载体上，只有 bot 设置进了主库：

| 数据 | 载体 | 位置 | 并发/持久性 |
|---|---|---|---|
| Chat（项目绑定、配对、白名单、bash cwd、当前 agent、agent state） | `pkg/jsonstore` 整文件 | `~/.tingly-box/bot_chats.json` | 多实例各持内存副本，整文件覆盖 |
| Session（执行会话 + 消息历史） | `pkg/jsonstore` 整文件 | `~/.tingly-box/bot_sessions.json` | 更新只置 dirty，不落盘 |
| SmartGuide 对话历史 | 裸 `os.WriteFile` | `<dataDir>/sessions/<chatID>-smartguide.json` | 每 chat 一个文件，0644 |
| Bot 设置 | SQLite / GORM ✅ | `db/tingly.db` → `imbot_settings` | 单连接 + WAL |
| Scenario bindings | SQLite 里的 JSON text 列 | `imbot_settings.scenarios` | 读-改-写整个 blob |
| Pairing code | 纯内存 | `imbot/security` | 重启即失 |
| Audit | 纯内存 | `remote/audit.Logger` | 从不自动落盘 |

也就是说：**主库已经在那儿了，remote 是唯一还在用 JSON 文件当数据库的子系统。**

## 2. 问题清单

### P0-1 多实例并发写整文件 → 静默丢数据

`runBotWithSettings` 里每个 bot 各自 `NewChatStoreJSON(dataPath)`
（`internal/remote_control/bot/manager.go:32`），而 `dataPath` 是全局共享的
`bot_chats.json`（`internal/server/module/imbot/manager.go:123`）。
`Manager.ChatStore()`（`manager.go:281`）给 CLI 又开一个。

`jsonstore` 只在 `New()` 时 `load()` 一次，之后各实例各持一份内存 map，
每次写都是 `MarshalIndent` 整个 map + rename 覆盖
（`pkg/jsonstore/store.go:162`）。结果：

> 开了 telegram 和 feishu 两个 bot。telegram 侧用户 `/cd` 绑了项目 →
> 写入文件。之后 feishu 侧任何一次写（配对、切 agent）都会用它启动时的
> 旧快照整体覆盖，telegram 那条绑定凭空消失。

跨进程（server + CLI 同时操作）同理，且没有文件锁。

### P0-2 Session 的更新根本不落盘

`Manager.Update()` → `store.Set()` 只把 `dirty` 置 true
（`remote/session/manager.go:288`, `pkg/jsonstore/store.go:231`）。真正写盘只发生在：

- `Create` / `CreateWithID`（显式 `ForceSave`）
- `Manager.Stop()` → `store.Close()`

也就是说状态流转（running → completed）、response、每一条 message，
在下一次「新建会话」或「优雅关闭」之前都只在内存里。进程被 kill / 崩溃 →
全部丢失，重启后看到的是一堆停在 `pending` 的僵尸会话。

附带：manager 靠 `if jsonStore, ok := m.store.(*SessionStoreJSON)` 类型断言
来触发落盘（`manager.go:136`, `manager.go:179`）—— 接口没抽干净，
换实现就悄悄失去持久化。

### P0-3 chatID 直接拼进文件名

`smart_guide.SessionStore.path()` = `filepath.Join(s.dir, chatID+"-smartguide.json")`
（`internal/remote_control/smart_guide/session_store.go:37`），chatID 直接来自平台。
Telegram 是纯数字，但飞书 `open_chat_id`、WhatsApp JID 含 `@`、部分平台含 `/`。
没有任何 sanitize → 路径穿越 + 文件名冲突。同一目录还是 0644（其他状态文件是 0600）。

### P1-1 全表扫描代替索引

所有查询都是 O(n) 线性遍历：`FindByChatAgentProject`、`ListByChat`
（`remote/session/json_store.go:84,111`）、`ListChatsByOwner`、`IsWhitelisted`、
`ListWhitelistedGroups`（`chat_store.go:446,483,559`）。

更糟的是写放大：`Session.Messages []Message` 内联在会话里，
append 一条消息要重新 marshal 整个 sessions 文件 → 消息数增长下是 O(n²)。

### P1-2 没有 schema，也没有迁移路径

`jsonstore` 的 `version` 只做「文件版本 > 代码版本就报错」，没有升级钩子
（`store.go:139`）。字段增删全靠 `omitempty` 碰运气。

`Session` 结构体连 json tag 都没有（`remote/session/manager.go:38-53`），
持久化的 key 是 `"ID"` / `"ChatID"` 这种 Go 字段名 —— 任何一次字段重命名
都是一次静默的数据丢失。

### P1-3 同一份状态有两个家

- `Chat.AgentState []byte`（chats.json 里的 blob，`chat_store.go:124`）和
  `smart_guide.SessionStore`（每 chat 一个文件）都在存会话/handoff 状态。
- `bot.BotSetting`（`chat_store.go:22`）和 `db.Settings`
  （`internal/data/db/imbot_settings_store.go:21`）是两个手工同步的结构体，
  已经漂移：`BotSetting` 有 `Token`/`Verbose` 而 `Settings` 没有 `Verbose`，
  `ImBotSettingsRecord` 有 `Debug` 而两个 DTO 都没有。
- `Chat.ProjectPath` + `Chat.ProjectHistory []string` 是同一个事实的两份表示，
  靠 `pushProjectHistory` 手工维持一致（`chat_store.go:387`）。

### P1-4 Scenario bindings 是 SQLite 里的 JSON blob

`imbot_settings.scenarios` 存一整个 binding 列表的 JSON 文本。
后果是 `remote/binding/binding.go` 里要手写 `rawBinding map[string]json.RawMessage`
解析器（`binding.go:194`）来保住未知字段，`SetScenarioEnabled` 要在原始
`[]map[string]json.RawMessage` 上做读-改-写（`binding.go:161`）才不丢字段，
`Resolver.Resolve` 要遍历所有 bot、逐个 parse blob 才能匹配一个
(scenario, event)（`binding.go:124`）。查询、约束、部分更新全都做不了。

### P2-1 审计不落盘

`remote/audit.Logger` 只有内存环形缓冲（`logger.go:41`），
唯一的落盘入口是手动调用的 `ExportJSONToFile`（`logger.go:195`）。
而 pairing code reveal 这类「每次都审计」的安全事件重启即蒸发。

### P2-2 UX：这些状态在产品里不可见

`internal/server/module/imbot/routes.go` 全部是 bot 设置的 CRUD，
**没有任何接口能列出 chat、session、绑定历史或配对状态**。
用户在 UI 上看不到「这个 bot 现在配对了谁、绑在哪个项目、有哪些活跃会话」，
只能去 shell 里翻 JSON 文件。

对照 `.design/ux-principles.md`：违反「surface the artifact for the next action」
（配对/绑定这些产物是下一步操作的入口，却没有出口）和
「diagnostics must traverse the real path」（诊断「为什么 bot 不回我」时
真实路径上的状态全是黑箱）。这条是这次重设计真正的产品动机 ——
存储换成表之后，这些出口才做得出来。

## 3. 设计原则

1. **单库单连接。** 不再新造 store，并入已有的 `internal/data/db.StoreManager`
   （单 `*gorm.DB` + WAL + busy_timeout + AutoMigrate）。remote 不再自己持有文件路径。
2. **一份状态一个家。** 关系用行和索引表达，不用 blob 嵌套。
3. **JSON 不是禁忌，JSON 文件当数据库才是。** 真正开放的字段
   （binding options、session context）继续用 JSON 列 —— 那是「值」，不是「表」。
4. **每张表都要有出口。** schema 落地的同时给 API + UI，否则只是把黑箱换了个格式。

## 4. 目标 schema

```
remote_chats
  chat_id            TEXT PK
  platform           TEXT
  project_path       TEXT
  owner_id           TEXT          idx(owner_id, platform)
  is_paired          BOOL
  paired_bot_uuid    TEXT          idx
  paired_sender_id   TEXT
  paired_at          DATETIME
  is_whitelisted     BOOL          idx
  whitelisted_by     TEXT
  bash_cwd           TEXT
  current_agent      TEXT
  verbose            BOOL NULL     -- 三态保留
  created_at / updated_at

remote_chat_projects              -- 取代 Chat.ProjectHistory []string
  chat_id            TEXT          idx(chat_id, last_used_at DESC)
  project_path       TEXT
  last_used_at       DATETIME
  PK(chat_id, project_path)        -- 去重变成主键，上限变成 DELETE

remote_sessions
  id                 TEXT PK
  chat_id            TEXT
  agent              TEXT
  project            TEXT
  status             TEXT          idx(status)
  request / response / error  TEXT
  permission_mode    TEXT
  context            JSON          -- 开放字段，保持 JSON 列
  created_at / last_activity / expires_at
  idx(chat_id, agent, project, last_activity DESC)   -- FindByChatAgentProject 一次索引查询

remote_session_messages           -- 取代 Session.Messages []Message
  session_id         TEXT          idx(session_id, seq)
  seq                INTEGER
  role / content / summary  TEXT
  created_at         DATETIME
  PK(session_id, seq)              -- append 变 INSERT，retention 变 DELETE WHERE

remote_agent_state                -- 取代 Chat.AgentState + <chatID>-smartguide.json
  chat_id            TEXT
  agent              TEXT
  state              JSON/BLOB
  updated_at
  PK(chat_id, agent)               -- 顺带消灭 P0-3 的路径穿越

remote_bindings                   -- 取代 imbot_settings.scenarios JSON 列
  bot_uuid           TEXT          idx(bot_uuid)
  scenario           TEXT          idx(scenario)
  chat_id            TEXT
  enabled            BOOL NULL     -- 三态保留（nil = 兼容旧行为「视为开」）
  events             JSON
  options            JSON          -- 真正开放的部分，保持 JSON
  PK(bot_uuid, scenario)

remote_audit                      -- 取代内存环形缓冲
  id / timestamp(idx) / level / action / user_id / client_ip
  session_id / request_id / success / message / details JSON / duration_ms
```

`Resolver.Resolve` 从「遍历所有 bot × parse blob」变成
`WHERE scenario = ? AND enabled IS NOT FALSE`。

## 5. 抽象与依赖方向

- `remote/store`（新包）只定义领域接口：`ChatStore` / `SessionStore` /
  `AgentStateStore` / `BindingStore` / `AuditStore`。
- 实现落在 `internal/data/db`，由 `StoreManager` 统一初始化并注入。
- **`remote/*` 不 import gorm，也不再接受文件路径参数** —— 现在
  `NewChatStoreJSON(filePath)` / `NewSessionStoreJSON(filePath)` 这种签名
  把存储介质泄漏给了调用方，是「per-bot 各开一个实例」的根因。
- 删掉 `manager.go` 里的 `*SessionStoreJSON` 类型断言：写入即事务提交，
  `ForceSave` 这个概念消失。
- `bot.BotSetting` 与 `db.Settings` 合并成一个类型，消除手工同步。
- 现有 `ChatStoreInterface`（`chat_store.go:135`）方法签名基本可以原样保留，
  这让 P1 能做到「换实现不改调用方」。

## 6. 数据迁移

复用 `internal/data/db/migrations/` 的既有模式（见 `migrate_imbot_credentials.go`）：

- 启动时一次性 importer：`bot_chats.json` / `bot_sessions.json` /
  `<dataDir>/sessions/*.json` → 建表 → 逐条 upsert。
- 幂等：以 migration marker 行判定，重复启动不重复导入。
- 旧文件**重命名**为 `.migrated` 而非删除，留一个回滚窗口。
- 导入失败不阻塞启动：记 error 日志 + 保留原文件，让用户能拿到数据。
- `Session` 没有 json tag，导入时按现有的大写 key（`"ID"`/`"ChatID"`…）读，
  这段读逻辑随迁移代码一起在若干版本后删除。

## 7. 分阶段落地

**P0 · 止血**（不改 schema，可独立合入、独立发版）✅ 已完成
- `Manager` 持有唯一 `ChatStore` 实例，`runBotWithSettings` 接收而非新建 → 修 P0-1
- session 写入后立即持久化；顺带修好从未被调用的 `sessionMgr.Stop()` → 修 P0-2
- `smart_guide.SessionStore` 文件名 sanitize/hash + 0600 → 修 P0-3

P0 修不掉的部分（留给 P1）：**跨进程**的整文件覆盖。共享实例只在单进程内有效，
CLI 的 `remote run`（`internal/command/remote.go` 的 standalone 路径，单进程单 bot）
与 server 同时操作同一份 `bot_chats.json` 时仍会互相覆盖 —— jsonstore 没有文件锁，
也没有写前重读。这个只有换到 SQLite（WAL + busy_timeout）才真正解决。

**P1 · 落表**
- schema + AutoMigrate + importer
- ChatStore / SessionStore 切 SQLite 实现，`ChatStoreInterface` 签名不变，现有测试全绿
- 消息拆到 `remote_session_messages`

**P2 · 拆 blob**
- `ProjectHistory` / `AgentState` / `scenarios` → 独立表
- `BotSetting` 与 `db.Settings` 合并
- 废弃 `pkg/jsonstore`（届时零调用方）

**P3 · 出口**（真正兑现 UX 动机）
- `GET /api/v1/remote/chats`、`/remote/chats/:id`、`/remote/sessions`
  + swagger 定义 + `task codegen`
- UI：bot 详情里的「当前状态」面板 —— 配对了谁、绑到哪个项目、活跃会话、最近审计
- audit 落盘 + retention

## 8. 待决策事项

1. **范围**：只做 P0+P1，还是一路做到 P3？
2. **SmartGuide 历史进不进库**：单 chat 的 anthropic message 数组可能到 MB 级。
   倾向进 `remote_agent_state` + retention；另一选择是留文件但只修 sanitize。
3. **迁移策略**：静默自动迁移，还是保留旧文件只读回退窗口 + 显式提示？
4. **`pkg/jsonstore` 是否直接废弃**：全迁完就只剩零调用方。
