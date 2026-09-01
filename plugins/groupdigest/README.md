# 群刊（groupdigest）

群消息累计达到一定数量后，自动调用 AI 生成一期群刊，以 **Markdown 文件**或 **渲染图片** 的形式发送到群。

## 功能

- 在配置的群聊中累计消息数量，达到阈值后自动生成群刊
- 复用 AI 对话插件的模型配置（`plugin.ai_chat_bot.*`），无需单独配置 LLM
- 生成内容由可配置的系统提示词控制
- 两种发送形式：
  - `md`：发送 Markdown 文件（无需额外服务，默认）
  - `image`：先调用本地 `md2img-api` 容器把 Markdown 渲染成 PNG 图片再发送
- 支持冷却时间，避免群内频繁生成
- **状态持久化**：计数与最近消息每 10 条、触发生成、生成结束时自动落盘（SQLite/MySQL），重启后进度不丢（异常退出最多丢失最近几条）

## 管理命令

在群内 @ 机器人发送（命令本身不计入群刊计数）：

| 命令 | 说明 |
| --- | --- |
| `/digest status`（或 `/群刊状态`） | 查看当前收集进度：计数/阈值、缓冲消息数、是否生成中、最近生成时间、冷却剩余 |
| `/digest list [n]`（或 `/群刊列表`） | 查看最近 n 条已收集消息（默认 10，上限 50） |
| `/digest clear`（或 `/群刊清空`） | 清空当前群的计数与消息缓冲，重新开始收集 |

## 配置

在面板「配置管理」中配置（修改后需重启 Bot 生效）：

| 配置键 | 说明 | 默认值 |
| --- | --- | --- |
| `plugin.groupdigest.enable` | 是否启用群刊 | `true` |
| `plugin.groupdigest.group_ids` | 作用群聊列表（逗号分隔） | 空（不对任何群生效） |
| `plugin.groupdigest.prompt` | 生成群刊的系统提示词 | 内置默认提示词 |
| `plugin.groupdigest.threshold` | 触发消息数 | `100` |
| `plugin.groupdigest.max_messages` | 喂给 AI 的最大消息数 | `200` |
| `plugin.groupdigest.send_mode` | 发送形式：`md` / `image` | `md` |
| `plugin.groupdigest.md2img_url` | md2img 渲染服务地址 | `http://127.0.0.1:3000` |
| `plugin.groupdigest.cooldown_minutes` | 生成冷却（分钟），`0` 不限制 | `0` |

### 群 ID 写法

- **QQ**：直接填群号（如 `123456`）或带 `qq:` 前缀（`qq:123456`），二者等价
- **飞书**：`fs:oc_xxx`
- **Telegram**：`tg:-100xxxx`
- **Discord**：`dc:频道ID`
- **QQ 官方**：`qo:群开放ID`

> 群 ID 可从面板日志或 AI 对话的会话 ID（`g:<群ID>`）中查看。

## 依赖

- **AI 对话插件**：群刊复用其模型配置（`plugin.ai_chat_bot.base_url` / `api_key` / `model` / `api_format`）。未配置 API Key 时插件仍可安装运行，但不会生成群刊（日志会提示）
- **md2img-api**（仅 `image` 模式）：需先在本地启动容器：

```bash
docker run -d -p 3000:3000 --name md2img-api jeanhua/md2img-api:latest
```

服务地址在 `plugin.groupdigest.md2img_url` 配置（默认 `http://127.0.0.1:3000`）。

## 行为说明

- 插件只统计**作用群列表**中的群消息；达到阈值后异步生成（不阻塞消息处理），生成期间新消息继续累计
- 每次生成使用独立的 AI 上下文（生成前清空历史），互不干扰
- 发送形式为图片时若 md2img 服务不可用，会向群内发送失败提示并记录日志

## 平台支持

QQ / 飞书 / Telegram / Discord / QQ 官方均支持（Markdown 文件与图片均通过框架统一消息段发送）。
