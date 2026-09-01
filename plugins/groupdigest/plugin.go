// Package groupdigest 群刊插件：群消息累计达到阈值后，自动调用 AI 生成群刊，
// 以 Markdown 文件或渲染图片的形式发送到群。
package groupdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jeanhua/AniaBot/bot/component/aichat"
	"github.com/jeanhua/AniaBot/common/bot"
	"github.com/jeanhua/AniaBot/common/model/command"
	"github.com/jeanhua/AniaBot/common/model/message"
	"github.com/jeanhua/AniaBot/common/msgchain"
	"github.com/jeanhua/AniaBot/common/plugin"
	"github.com/jeanhua/AniaBot/common/plugininfo"
	"github.com/jeanhua/AniaBot/common/storage"
	"github.com/spf13/viper"
)

// persistEvery 每累计多少条消息把群状态落盘一次；重启最多丢失最近几条。
const persistEvery = 10

// 群刊管理命令（@机器人后输入，/digest 与中文别名均可）。
const (
	cmdDigest     = "digest"
	cmdDigestCN   = "群刊"
	cmdStatus     = "status"
	cmdStatusCN   = "状态"
	cmdList       = "list"
	cmdListCN     = "列表"
	cmdClear      = "clear"
	cmdClearCN    = "清空"
	cmdStatusFull = "群刊状态"
	cmdListFull   = "群刊列表"
	cmdClearFull  = "群刊清空"
)

// groupDigestConfig 群刊插件配置。实现 plugin.ConfigSchemaProvider 后，
// 面板「配置管理」自动渲染表单，框架在 Start 前填充到 p.cfg。
type groupDigestConfig struct {
	Enable      bool     `cfg:"plugin.groupdigest.enable" label:"启用群刊" group:"群刊" default:"true" help:"关闭后插件不再计数与生成群刊"`
	GroupIDs    []string `cfg:"plugin.groupdigest.group_ids" label:"作用群聊列表" group:"群刊" help:"要生成群刊的群 ID 列表（逗号分隔；QQ 可直接填群号，其他平台需带前缀，如 fs:oc_xxx / tg:-100xxx / dc:频道ID）；留空则不对任何群生效"`
	Prompt      string   `cfg:"plugin.groupdigest.prompt" label:"系统提示词" type:"text" group:"群刊" default:"你是一名群刊编辑，负责把群聊记录整理成一份简洁、有条理、有趣的群刊。\n要求：\n1. 用 Markdown 格式输出，标题为「群聊精选」\n2. 提炼热点话题、精彩发言、问答与有趣瞬间，按主题归类\n3. 每个要点附上发言者昵称，避免逐条罗列全部消息\n4. 语言自然活泼，适合在群里传播"`
	Threshold   int      `cfg:"plugin.groupdigest.threshold" label:"触发消息数" group:"群刊" default:"100" help:"群消息累计达到该条数后自动生成一期群刊"`
	MaxMessages int      `cfg:"plugin.groupdigest.max_messages" label:"喂给 AI 的最大消息数" group:"群刊" default:"200" help:"参与生成的最近消息数量上限，超出只保留最近的部分"`
	SendMode    string   `cfg:"plugin.groupdigest.send_mode" label:"发送形式" type:"select" options:"md,image" group:"群刊" default:"md" help:"md=发送 Markdown 文件；image=先经 md2img 服务渲染成 PNG 图片再发送"`
	MD2ImgURL   string   `cfg:"plugin.groupdigest.md2img_url" label:"md2img 服务地址" group:"群刊" default:"http://127.0.0.1:3000" help:"image 模式使用的渲染服务地址（docker run -d -p 3000:3000 jeanhua/md2img-api:latest）"`
	CooldownMin int      `cfg:"plugin.groupdigest.cooldown_minutes" label:"生成冷却(分钟)" group:"群刊" default:"0" help:"生成一期后至少间隔该分钟数才生成下一期；0 表示不限制"`
}

// GroupDigestPlugin 群刊插件定义：嵌入 plugin.Meta 获得默认实现，只覆盖需要的方法。
type GroupDigestPlugin struct {
	plugin.Meta
	cfg groupDigestConfig

	// chat 复用的群刊 AI 会话（每次生成前清空历史）；nil 表示 AI 参数未配置，
	// 群刊生成不可用（不影响插件其余部分）。
	chat *aichat.ChatBot
	// maxToken 输出 token 上限（<=0 表示不传该参数）。
	maxToken int

	// groupSet 规范化后的作用群 ID 集合（message.FromString 统一 qq: 前缀）。
	groupSet map[string]struct{}
	// store 持久化存储（digest 子命名空间）；nil 表示持久层不可用，退回纯内存。
	store storage.PersistentStorage
	// states 按群隔离的计数与消息缓冲。
	states sync.Map // groupID -> *groupState
}

// NewPlugin 构造函数（plugin.json 的 entry.constructor 默认指向这里）。
func NewPlugin() *GroupDigestPlugin {
	p := &GroupDigestPlugin{}
	p.Name = "群刊"
	p.HelpWords = "群消息达到阈值后自动生成群刊；@我发送 /群刊状态 可查看收集进度"
	p.AdminOnly = false
	p.ShowFor = plugininfo.ShowForGroup
	p.Author = "jeanhua"
	p.Version = "1.1.0"
	p.Order = plugin.LevelNormal
	return p
}

// ConfigSchema 声明配置结构体（面板表单 + 默认值 + Start 前自动填充）。
func (p *GroupDigestPlugin) ConfigSchema() any { return &p.cfg }

// Start 初始化：构建作用群集合、持久化存储与群刊 AI 会话（复用 AI 对话插件配置）。
func (p *GroupDigestPlugin) Start(ctx context.Context, cfg *viper.Viper) error {
	p.groupSet = normalizeGroupIDs(p.cfg.GroupIDs)
	// 框架已按插件名做基础命名空间隔离，这里再开 digest 子命名空间
	if p.PersistentStorage != nil {
		p.store = p.PersistentStorage.Clone("digest")
	}

	if p.cfg.Threshold <= 0 {
		p.cfg.Threshold = 100
	}
	if p.cfg.MaxMessages <= 0 {
		p.cfg.MaxMessages = 200
	}
	if p.cfg.SendMode == "" {
		p.cfg.SendMode = "md"
	}

	// 与 AI 对话插件共用同一套模型配置（plugin.ai_chat_bot.*）
	baseURL := cfg.GetString("plugin.ai_chat_bot.base_url")
	apiKey := cfg.GetString("plugin.ai_chat_bot.api_key")
	model := cfg.GetString("plugin.ai_chat_bot.model")
	if baseURL == "" || apiKey == "" || model == "" {
		p.Logger.Warn("未配置 AI 对话参数（plugin.ai_chat_bot.base_url/api_key/model），群刊生成功能不可用")
		return nil
	}

	opts := []aichat.LLMClientOption{aichat.WithAPIFormat(cfg.GetString("plugin.ai_chat_bot.api_format"))}
	if maxAttempts := cfg.GetInt("plugin.ai_chat_bot.retry.max_attempts"); maxAttempts > 1 {
		baseDelay := time.Duration(cfg.GetInt("plugin.ai_chat_bot.retry.base_delay_sec")) * time.Second
		if baseDelay <= 0 {
			baseDelay = 2 * time.Second
		}
		opts = append(opts, aichat.WithRetry(maxAttempts, baseDelay))
	}
	if maxToken := cfg.GetInt("plugin.ai_chat_bot.max_token"); maxToken > 0 {
		p.maxToken = maxToken
	}

	chat, err := aichat.NewChatBot(baseURL, apiKey, model, p.cfg.Prompt, 0, nil, nil,
		aichat.WithClientOptions(opts...))
	if err != nil {
		// 初始化失败只降级为不可用，不让整个插件启动失败
		p.Logger.Error("创建群刊 AI 会话失败，群刊生成功能不可用", "error", err.Error())
		return nil
	}
	p.chat = chat
	p.Logger.Info("群刊插件已初始化",
		"groups", len(p.groupSet), "threshold", p.cfg.Threshold, "send_mode", p.cfg.SendMode,
		"persist", p.store != nil)
	return nil
}

// OnGroupMsg 群聊消息事件：先处理管理命令，再按群累计消息，达到阈值后异步触发群刊生成。
// 管理命令返回 false 停止传播；普通消息始终返回 true，不阻断后续插件（如 AI 对话）。
func (p *GroupDigestPlugin) OnGroupMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	if !p.cfg.Enable {
		return true, nil
	}
	if cmd.Mention && p.tryHandleCommand(b, cmd, msg) {
		return false, nil
	}
	gid := msg.GroupId.String()
	if !p.isTargetGroup(gid) {
		return true, nil
	}

	st := p.stateFor(gid)
	text := renderMessageText(msg)
	if text == "" {
		text = "[消息]"
	}
	t := time.Now()
	if msg.Time > 0 {
		t = time.Unix(int64(msg.Time), 0)
	}
	nick := msg.Sender.Card
	if nick == "" {
		nick = msg.Sender.Nickname
	}
	if nick == "" {
		nick = "用户"
	}
	st.add(digestMessage{Time: t, Nickname: nick, Text: text}, p.cfg.MaxMessages)
	// 按固定间隔落盘，减少高频写入
	if st.count%persistEvery == 0 {
		p.persistState(gid, st)
	}

	if snapshot := st.tryTrigger(p.cfg.Threshold, time.Duration(p.cfg.CooldownMin)*time.Minute, time.Now()); len(snapshot) > 0 {
		// 触发后计数与缓冲已重置，立即落盘
		p.persistState(gid, st)
		p.triggerDigest(b, msg.GroupId, snapshot)
	}
	return true, nil
}

// tryHandleCommand 处理群刊管理命令（@机器人）；返回 true 表示已处理并应停止传播。
func (p *GroupDigestPlugin) tryHandleCommand(b bot.Bot, cmd command.Command, msg message.Message) bool {
	switch cmd.Name {
	case cmdStatusFull:
		p.replyStatus(b, msg.GroupId)
		return true
	case cmdListFull:
		p.replyList(b, msg.GroupId, cmd.Args)
		return true
	case cmdClearFull:
		p.clearState(b, msg.GroupId)
		return true
	case cmdDigest, cmdDigestCN:
		if len(cmd.Args) == 0 {
			p.replyText(b, msg.GroupId, "用法：/digest status（状态）| list [n]（最近消息）| clear（清空）\n中文别名：/群刊状态、/群刊列表、/群刊清空")
			return true
		}
		switch cmd.Args[0] {
		case cmdStatus, cmdStatusCN:
			p.replyStatus(b, msg.GroupId)
		case cmdList, cmdListCN:
			p.replyList(b, msg.GroupId, cmd.Args[1:])
		case cmdClear, cmdClearCN:
			p.clearState(b, msg.GroupId)
		default:
			p.replyText(b, msg.GroupId, "未知子命令："+cmd.Args[0]+"，可用 status / list / clear")
		}
		return true
	}
	return false
}

// replyStatus 回复当前群刊收集状态。
func (p *GroupDigestPlugin) replyStatus(b bot.Bot, gid message.QID) {
	if !p.isTargetGroup(gid.String()) {
		p.replyText(b, gid, "本群未在群刊作用列表中（plugin.groupdigest.group_ids），不会收集消息。")
		return
	}
	st := p.stateFor(gid.String())
	st.mu.Lock()
	count, msgs, lastGen, generating := st.count, len(st.messages), st.lastGen, st.generating
	st.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("📊 群刊状态\n")
	fmt.Fprintf(&sb, "计数：%d / %d\n", count, p.cfg.Threshold)
	fmt.Fprintf(&sb, "已收集消息：%d 条\n", msgs)
	if generating {
		sb.WriteString("正在生成：是\n")
	} else {
		sb.WriteString("正在生成：否\n")
	}
	if lastGen.IsZero() {
		sb.WriteString("最近生成：暂无\n")
	} else {
		fmt.Fprintf(&sb, "最近生成：%s\n", lastGen.Format("01-02 15:04:05"))
	}
	if p.cfg.CooldownMin > 0 && !lastGen.IsZero() {
		remain := time.Duration(p.cfg.CooldownMin)*time.Minute - time.Since(lastGen)
		if remain > 0 {
			fmt.Fprintf(&sb, "冷却剩余：约 %.0f 分钟\n", remain.Minutes())
		} else {
			sb.WriteString("冷却剩余：无\n")
		}
	}
	fmt.Fprintf(&sb, "发送形式：%s", p.cfg.SendMode)
	p.replyText(b, gid, strings.TrimSpace(sb.String()))
}

// replyList 回复最近收集的 n 条消息（默认 10，上限 50）。
func (p *GroupDigestPlugin) replyList(b bot.Bot, gid message.QID, args []string) {
	if !p.isTargetGroup(gid.String()) {
		p.replyText(b, gid, "本群未在群刊作用列表中（plugin.groupdigest.group_ids），不会收集消息。")
		return
	}
	n := 10
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = min(v, 50)
		}
	}
	st := p.stateFor(gid.String())
	st.mu.Lock()
	msgs := append([]digestMessage(nil), st.messages...)
	st.mu.Unlock()
	if len(msgs) == 0 {
		p.replyText(b, gid, "还没有收集到群消息。")
		return
	}
	if n > len(msgs) {
		n = len(msgs)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "📝 最近 %d 条已收集消息（共 %d 条）：\n", n, len(msgs))
	for _, m := range msgs[len(msgs)-n:] {
		fmt.Fprintf(&sb, "· [%s] %s：%s\n", m.Time.Format("01-02 15:04"), m.Nickname, truncateRunes(m.Text, 60))
	}
	p.replyText(b, gid, strings.TrimSpace(sb.String()))
}

// clearState 清空当前群收集的计数与消息缓冲。
func (p *GroupDigestPlugin) clearState(b bot.Bot, gid message.QID) {
	if !p.isTargetGroup(gid.String()) {
		p.replyText(b, gid, "本群未在群刊作用列表中（plugin.groupdigest.group_ids），不会收集消息。")
		return
	}
	st := p.stateFor(gid.String())
	st.mu.Lock()
	st.count = 0
	st.messages = nil
	st.mu.Unlock()
	p.persistState(gid.String(), st)
	p.replyText(b, gid, "已清空该群收集的群刊消息，计数重新开始。")
}

// replyText 发送一条纯文本群消息。
func (p *GroupDigestPlugin) replyText(b bot.Bot, gid message.QID, text string) {
	chain := msgchain.Builder().Group().Text(text)
	b.SendGroupMsg(gid, chain.Build())
}

// persistState 把指定群的计数与缓冲快照写入持久化存储（digest:g:<群ID>）。
func (p *GroupDigestPlugin) persistState(gid string, st *groupState) {
	if p.store == nil {
		return
	}
	st.mu.Lock()
	ps := persistedState{Count: st.count, Messages: st.messages, LastGen: st.lastGen}
	st.mu.Unlock()
	data, err := json.Marshal(&ps)
	if err != nil {
		p.Logger.Warn("序列化群刊状态失败", "group", gid, "error", err.Error())
		return
	}
	if !p.store.SetString(context.Background(), "g:"+gid, string(data)) {
		p.Logger.Warn("持久化群刊状态失败", "group", gid)
	}
}

// isTargetGroup 判断群是否在作用列表中。
func (p *GroupDigestPlugin) isTargetGroup(gid string) bool {
	if len(p.groupSet) == 0 {
		return false
	}
	_, ok := p.groupSet[gid]
	return ok
}

// stateFor 获取（或惰性创建）指定群的计数状态，首次访问时从持久层恢复。
func (p *GroupDigestPlugin) stateFor(gid string) *groupState {
	v, _ := p.states.LoadOrStore(gid, &groupState{})
	st := v.(*groupState)
	st.ensureLoaded(p.store, gid)
	return st
}

// triggerDigest 异步生成并发送群刊；b.Go 提供崩溃恢复，且不阻塞消息分发。
func (p *GroupDigestPlugin) triggerDigest(b bot.Bot, gid message.QID, snapshot []digestMessage) {
	b.Go("群刊-"+gid.String(), func() {
		// OnGroupMsg 的 ctx 在返回后即被取消，这里使用独立超时上下文
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		st := p.stateFor(gid.String())
		defer func() {
			st.finish(time.Now())
			p.persistState(gid.String(), st)
		}()

		md, err := p.generateDigest(ctx, b, gid, snapshot)
		if err != nil {
			p.notifyError(b, gid, err)
			return
		}
		if err := p.deliver(ctx, b, gid, md); err != nil {
			p.notifyError(b, gid, err)
		}
	})
}

// digestMessage 一条参与群刊生成的群消息。
type digestMessage struct {
	Time     time.Time `json:"time"`
	Nickname string    `json:"nickname"`
	Text     string    `json:"text"`
}

// persistedState 落盘用的群状态快照（digest 子命名空间，键 g:<群ID>）。
type persistedState struct {
	Count    int             `json:"count"`
	Messages []digestMessage `json:"messages,omitempty"`
	LastGen  time.Time       `json:"last_gen,omitempty"`
}

// groupState 单个群的消息计数状态（mutex 保护，允许并发消息触发）。
type groupState struct {
	mu         sync.Mutex
	loaded     bool
	count      int
	messages   []digestMessage
	lastGen    time.Time
	generating bool
}

// ensureLoaded 首次访问时从持久化存储恢复状态（只执行一次）。
func (st *groupState) ensureLoaded(store storage.PersistentStorage, gid string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.loaded {
		return
	}
	st.loaded = true
	if store == nil {
		return
	}
	var ps persistedState
	if !store.Get(context.Background(), "g:"+gid, &ps) {
		return
	}
	st.count = ps.Count
	st.messages = ps.Messages
	st.lastGen = ps.LastGen
}

// add 累计一条消息，缓冲只保留最近 max 条。
func (st *groupState) add(m digestMessage, max int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.count++
	st.messages = append(st.messages, m)
	if len(st.messages) > max {
		st.messages = st.messages[len(st.messages)-max:]
	}
}

// tryTrigger 判断是否该生成群刊：达到阈值、无生成中任务且不在冷却期内。
// 领取本次消息快照并重置计数；返回 nil 表示不触发。
func (st *groupState) tryTrigger(threshold int, cooldown time.Duration, now time.Time) []digestMessage {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.count < threshold || st.generating {
		return nil
	}
	if cooldown > 0 && now.Sub(st.lastGen) < cooldown {
		return nil
	}
	st.generating = true
	st.count = 0
	msgs := st.messages
	st.messages = nil
	return msgs
}

// finish 标记本次生成结束并记录时间。
func (st *groupState) finish(now time.Time) {
	st.mu.Lock()
	st.generating = false
	st.lastGen = now
	st.mu.Unlock()
}

// normalizeGroupIDs 规范化作用群列表：QQ 纯数字统一加 qq: 前缀，其余平台 ID 原样保留。
func normalizeGroupIDs(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		set[message.FromString(id).String()] = struct{}{}
	}
	return set
}

// renderMessageText 把消息段渲染成可供 AI 阅读的纯文本。
func renderMessageText(msg message.Message) string {
	return strings.TrimSpace(msg.FriendlyText(false, message.WithNoSenderPrefix()))
}

// truncateRunes 按字符数截断文本，超长加省略号。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
