// Package music 是插件市场的音乐点歌插件：基于 GD音乐台(music.gdstudio.xyz) 开放 API
// 搜索与点播歌曲，支持多音源、歌词查询，QQ 平台以自定义音乐卡片发送。
package music

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jeanhua/AniaBot/common/bot"
	"github.com/jeanhua/AniaBot/common/model/command"
	"github.com/jeanhua/AniaBot/common/model/message"
	"github.com/jeanhua/AniaBot/common/msgchain"
	"github.com/jeanhua/AniaBot/common/plugin"
	"github.com/jeanhua/AniaBot/common/plugininfo"
	"github.com/spf13/viper"
)

const (
	// creditLine 出处注明（API 使用要求），附在搜索列表与文本链接末尾。
	creditLine = "via GD音乐台 (music.gdstudio.xyz)"
	// jumpURL 音乐卡片点击跳转页；API 未提供单曲页，跳转 GD音乐台主页。
	jumpURL = "https://music.gdstudio.xyz/"
	// lyricFileMaxChars 歌词超过该字符数时改发 .lrc 文件（QQ 平台）。
	lyricFileMaxChars = 3800
)

// MusicPlugin 音乐点歌插件：搜索 → 序号点播 → 音乐卡片/链接 + 歌词。
type MusicPlugin struct {
	plugin.Meta
	cfg musicConfig

	client *gdMusicClient

	mu       sync.Mutex
	sessions map[string]*searchSession // 选歌会话：key = 用户|场景
	cooldown map[string]time.Time      // 个人搜索冷却
	limiter  *rateLimiter              // 全局 API 频率保护（官方 50 次/5 分钟）
}

// searchSession 一次搜索的候选缓存，供序号点歌/查歌词。
type searchSession struct {
	keyword   string
	source    string // 会话创建时的音源，点播/歌词按它请求，避免改配置后 id 对不上
	tracks    []*track
	createdAt time.Time
}

// NewPlugin 构造函数（plugin.json 的 entry.constructor 指向这里）。
func NewPlugin() *MusicPlugin {
	p := &MusicPlugin{
		sessions: make(map[string]*searchSession),
		cooldown: make(map[string]time.Time),
	}
	p.Name = "音乐点歌"
	p.HelpWords = "at 我发送 /点歌 关键词 搜歌，/点歌 序号 下载歌曲发文件，/点歌 歌词 序号 看歌词"
	p.AdminOnly = false
	p.ShowFor = plugininfo.ShowForGroup | plugininfo.ShowForFriend
	p.Author = "jeanhua"
	p.Version = "1.0.0"
	p.Order = plugin.LevelNormal
	return p
}

// Start 初始化：配置兜底、构建 API 客户端与全局限流器。
func (p *MusicPlugin) Start(ctx context.Context, cfg *viper.Viper) error {
	if p.sessions == nil {
		p.sessions = make(map[string]*searchSession)
	}
	if p.cooldown == nil {
		p.cooldown = make(map[string]time.Time)
	}
	if strings.TrimSpace(p.cfg.APIBase) == "" {
		p.cfg.APIBase = defaultAPIBase
	}
	if !validSources[p.cfg.Source] {
		if p.cfg.Source != "" {
			p.Logger.Warn("配置的音源无效，回退为 netease", "source", p.cfg.Source)
		}
		p.cfg.Source = "netease"
	}
	if !validBitrates[p.cfg.Bitrate] {
		if p.cfg.Bitrate != "" {
			p.Logger.Warn("配置的音质无效，回退为 320", "bitrate", p.cfg.Bitrate)
		}
		p.cfg.Bitrate = "320"
	}
	if p.cfg.SendMode != "file" && p.cfg.SendMode != "card" && p.cfg.SendMode != "text" {
		p.cfg.SendMode = "file"
	}
	if p.cfg.MaxSizeMB < 0 {
		p.cfg.MaxSizeMB = 20
	}
	if p.cfg.DownloadTimeoutSec < 30 {
		p.cfg.DownloadTimeoutSec = 180
	}
	if p.cfg.DownloadTimeoutSec > 600 {
		p.cfg.DownloadTimeoutSec = 600
	}
	if p.cfg.SearchCount < 1 {
		p.cfg.SearchCount = 10
	}
	if p.cfg.SearchCount > 30 {
		p.cfg.SearchCount = 30
	}
	if p.cfg.MaxResults < 1 {
		p.cfg.MaxResults = 8
	}
	if p.cfg.MaxResults > 20 {
		p.cfg.MaxResults = 20
	}
	if p.cfg.SessionMin < 1 {
		p.cfg.SessionMin = 10
	}
	if p.cfg.CooldownSec < 0 {
		p.cfg.CooldownSec = 0
	}
	if p.cfg.RateLimit5Min < 1 {
		p.cfg.RateLimit5Min = 40
	}
	p.client = newGDClient(p.cfg.APIBase, p.RestyClient)
	p.limiter = newRateLimiter(5*time.Minute, p.cfg.RateLimit5Min)
	p.Logger.Info("音乐点歌插件已初始化",
		"source", p.cfg.Source,
		"bitrate", p.cfg.Bitrate,
		"send_mode", p.cfg.SendMode,
		"max_size_mb", p.cfg.MaxSizeMB,
		"rate_limit_5min", p.cfg.RateLimit5Min,
	)
	return nil
}

// isMusicCmd 是否点歌命令（含中文别名）。
func isMusicCmd(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "music", "点歌", "音乐":
		return true
	default:
		return false
	}
}

// OnGroupMsg 群聊消息事件：必须 @机器人。
func (p *MusicPlugin) OnGroupMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	if !p.cfg.Enable {
		return true, nil
	}
	if !cmd.Mention || !isMusicCmd(cmd.Name) {
		return true, nil
	}
	p.handleMusic(ctx, b, cmd, msg, true)
	return false, nil
}

// OnFriendMsg 私聊消息事件：无需 @。
func (p *MusicPlugin) OnFriendMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	if !p.cfg.Enable {
		return true, nil
	}
	if !isMusicCmd(cmd.Name) {
		return true, nil
	}
	p.handleMusic(ctx, b, cmd, msg, false)
	return false, nil
}

// handleMusic /点歌 主流程：按子命令分发。
func (p *MusicPlugin) handleMusic(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message, isGroup bool) {
	act := parseMusicArgs(cmd.Args)
	switch act.kind {
	case "help":
		p.replyText(b, msg, isGroup, helpText(p.cfg.SessionMin))
	case "search":
		if !p.passSearchLimits(b, msg, isGroup) {
			return
		}
		p.doSearch(ctx, b, msg, isGroup, act.keyword)
	case "pick":
		if !p.passAPIQuota(b, msg, isGroup) {
			return
		}
		p.doPick(ctx, b, msg, isGroup, act.index)
	case "lyric":
		if act.index <= 0 {
			p.replyText(b, msg, isGroup, "用法：/点歌 歌词 序号，先搜索拿到列表哦")
			return
		}
		if !p.passAPIQuota(b, msg, isGroup) {
			return
		}
		p.doLyric(ctx, b, msg, isGroup, act.index)
	}
}

// musicAction 解析后的子命令：kind 为 help/search/pick/lyric。
type musicAction struct {
	kind    string
	keyword string
	index   int // pick/lyric 用，1 起
}

// parseMusicArgs 解析参数：无参/help → 帮助；纯数字或 选 N → 点播；
// 歌词 N → 查歌词；其余整体作为搜索关键词。
func parseMusicArgs(args []string) musicAction {
	if len(args) == 0 {
		return musicAction{kind: "help"}
	}
	first := strings.TrimSpace(args[0])
	rest := args[1:]
	switch first {
	case "help", "帮助", "-h", "--help":
		return musicAction{kind: "help"}
	case "歌词", "词", "lyric":
		if n, ok := parseIndex(rest); ok {
			return musicAction{kind: "lyric", index: n}
		}
		return musicAction{kind: "lyric"}
	case "选", "播放", "点", "play":
		if n, ok := parseIndex(rest); ok {
			return musicAction{kind: "pick", index: n}
		}
		kw := strings.TrimSpace(strings.Join(rest, " "))
		if kw == "" {
			return musicAction{kind: "help"}
		}
		return musicAction{kind: "search", keyword: kw}
	}
	if n, err := strconv.Atoi(first); err == nil {
		return musicAction{kind: "pick", index: n}
	}
	keyword := strings.TrimSpace(strings.Join(args, " "))
	if keyword == "" {
		return musicAction{kind: "help"}
	}
	return musicAction{kind: "search", keyword: keyword}
}

// parseIndex 解析序号参数：恰好一个正整数才合法。
func parseIndex(args []string) (int, bool) {
	if len(args) != 1 {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// passSearchLimits 搜索前置检查：个人冷却 + 全局 API 配额；不通过时已回复提示。
func (p *MusicPlugin) passSearchLimits(b bot.Bot, msg message.Message, isGroup bool) bool {
	if p.cfg.CooldownSec > 0 {
		if ok, wait := p.takeCooldown(msg.Sender.UserId.String()); !ok {
			p.replyText(b, msg, isGroup, fmt.Sprintf("手速太快啦，冷却 %s 后再来搜歌", humanDur(wait)))
			return false
		}
	}
	return p.passAPIQuota(b, msg, isGroup)
}

// passAPIQuota 全局 API 配额检查（点播/歌词也各消耗 API 次数）。
func (p *MusicPlugin) passAPIQuota(b bot.Bot, msg message.Message, isGroup bool) bool {
	if ok, wait := p.limiter.allow(); !ok {
		p.replyText(b, msg, isGroup, fmt.Sprintf("点歌配额用完了（每 5 分钟限 %d 次），约 %s 后再试", p.cfg.RateLimit5Min, humanDur(wait)))
		return false
	}
	return true
}

// takeCooldown 消费一次搜索冷却，返回 (放行与否, 需等待时长)。
func (p *MusicPlugin) takeCooldown(key string) (bool, time.Duration) {
	now := time.Now()
	window := time.Duration(p.cfg.CooldownSec) * time.Second
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cooldown) > 4096 { // 防止长期运行后无限膨胀
		for k, t := range p.cooldown {
			if now.Sub(t) > window {
				delete(p.cooldown, k)
			}
		}
	}
	if last, ok := p.cooldown[key]; ok {
		if wait := window - now.Sub(last); wait > 0 {
			return false, wait
		}
	}
	p.cooldown[key] = now
	return true, 0
}

// doSearch 关键字搜索并回复候选列表，同时记入选歌会话。
func (p *MusicPlugin) doSearch(ctx context.Context, b bot.Bot, msg message.Message, isGroup bool, keyword string) {
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tracks, err := p.client.searchSongs(sctx, p.cfg.Source, keyword, p.cfg.SearchCount)
	if err != nil {
		p.Logger.Warn("点歌搜索失败", "error", err, "keyword", keyword, "user", msg.Sender.UserId)
		p.replyText(b, msg, isGroup, "搜索失败了："+err.Error())
		return
	}
	if len(tracks) == 0 {
		p.replyText(b, msg, isGroup, fmt.Sprintf("没找到 %q 相关的歌曲，换个关键词试试？", keyword))
		return
	}
	if len(tracks) > p.cfg.MaxResults {
		tracks = tracks[:p.cfg.MaxResults]
	}
	p.storeSession(msg, keyword, tracks)
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎵 为你找到 %q 的候选，回复 /点歌 序号 播放、/点歌 歌词 序号 看歌词（%d 分钟内有效）：\n", keyword, p.cfg.SessionMin)
	for i, t := range tracks {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, trackLine(t))
	}
	sb.WriteString(creditLine)
	p.replyText(b, msg, isGroup, sb.String())
	p.Logger.Info("点歌搜索完成", "keyword", keyword, "source", p.cfg.Source, "results", len(tracks), "user", msg.Sender.UserId, "is_group", isGroup)
}

// storeSession 保存选歌会话，顺手清理过期会话。
func (p *MusicPlugin) storeSession(msg message.Message, keyword string, tracks []*track) {
	exp := time.Duration(p.cfg.SessionMin) * time.Minute
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for k, s := range p.sessions {
		if now.Sub(s.createdAt) > exp {
			delete(p.sessions, k)
		}
	}
	p.sessions[sessionKey(msg)] = &searchSession{
		keyword:   keyword,
		source:    p.cfg.Source,
		tracks:    tracks,
		createdAt: now,
	}
}

// currentSession 取当前会话，过期或不存在返回 false。
func (p *MusicPlugin) currentSession(msg message.Message) (*searchSession, bool) {
	exp := time.Duration(p.cfg.SessionMin) * time.Minute
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.sessions[sessionKey(msg)]
	if !ok || time.Since(s.createdAt) > exp {
		return nil, false
	}
	return s, true
}

// sessionKey 会话键：用户 + 场景（群聊按群隔离，私聊统一）。
func sessionKey(msg message.Message) string {
	if msg.GroupId == "" {
		return msg.Sender.UserId.String() + "|f"
	}
	return msg.Sender.UserId.String() + "|g:" + msg.GroupId.String()
}

// doPick 点播：默认下载音频以文件发送；card 模式发音乐卡片（失败降级为文件）；
// text 模式只发播放链接。各环节失败自动降级，保证用户总能拿到可用的结果。
func (p *MusicPlugin) doPick(ctx context.Context, b bot.Bot, msg message.Message, isGroup bool, index int) {
	sess, ok := p.currentSession(msg)
	if !ok {
		p.replyText(b, msg, isGroup, fmt.Sprintf("没有有效的选歌列表，先发 /点歌 关键词 搜索一下吧"))
		return
	}
	if index < 1 || index > len(sess.tracks) {
		p.replyText(b, msg, isGroup, fmt.Sprintf("序号超出范围（1~%d），看下列表再选吧", len(sess.tracks)))
		return
	}
	t := sess.tracks[index-1]
	sctx, cancel := context.WithTimeout(ctx, time.Duration(p.cfg.DownloadTimeoutSec+30)*time.Second)
	defer cancel()
	res, err := p.client.songURL(sctx, sess.source, t.ID.String(), p.cfg.Bitrate)
	if err != nil {
		p.Logger.Warn("获取歌曲播放链接失败", "error", err, "track", t.Name, "id", t.ID.String(), "user", msg.Sender.UserId)
		p.replyText(b, msg, isGroup, "获取播放链接失败："+err.Error())
		return
	}
	audio := res.URL.String()
	size := parseSizeBytes(res.Size.String())

	title := t.Name
	subtitle := t.artistLine()
	if t.Album != "" {
		if subtitle == "" {
			subtitle = t.Album
		} else {
			subtitle += " - " + t.Album
		}
	}

	switch p.cfg.SendMode {
	case "text":
		p.sendSongLink(b, msg, isGroup, title, subtitle, audio)
		p.Logger.Info("点歌完成", "track", title, "user", msg.Sender.UserId, "is_group", isGroup, "mode", "text")
		return
	case "card":
		cover := p.fetchCover(sctx, sess.source, t)
		if p.sendCard(b, msg, isGroup, title, subtitle, audio, cover) {
			p.Logger.Info("点歌完成", "track", title, "user", msg.Sender.UserId, "is_group", isGroup, "mode", "card")
			return
		}
		// 卡片发送失败 → 降级为下载发文件
	}

	// 文件路径（file 模式，或 card 模式降级）：超过上限直接给链接并说明原因。
	limit := int64(p.cfg.MaxSizeMB) * 1024 * 1024
	if limit > 0 && size > limit {
		p.replyText(b, msg, isGroup, fmt.Sprintf("这首 %.1fMB，超过下载上限 %dMB（无损音质更大，可调大「下载上限」或降低音质），给你直链：\n🔗 %s",
			float64(size)/1024/1024, p.cfg.MaxSizeMB, audio))
		return
	}
	data, ctype, err := p.client.download(sctx, audio)
	if err != nil {
		p.Logger.Warn("音频下载失败，降级为链接", "error", err, "track", title, "user", msg.Sender.UserId)
		p.replyText(b, msg, isGroup, "下载失败了："+err.Error()+"，给你直链：\n🔗 "+audio)
		return
	}
	if limit > 0 && int64(len(data)) > limit {
		p.replyText(b, msg, isGroup, fmt.Sprintf("这首 %.1fMB，超过下载上限 %dMB（可调大「下载上限」或降低音质），给你直链：\n🔗 %s",
			float64(len(data))/1024/1024, p.cfg.MaxSizeMB, audio))
		return
	}
	name := buildAudioName(t, ctype, res.BR.String())
	if p.sendAudioFile(b, msg, isGroup, name, data) {
		p.Logger.Info("点歌完成", "track", title, "user", msg.Sender.UserId, "is_group", isGroup, "mode", "file",
			"size_mb", fmt.Sprintf("%.1f", float64(len(data))/1024/1024))
		return
	}
	// 文件发送失败 → 最后兜底给链接
	p.replyText(b, msg, isGroup, "文件发送失败，给你直链：\n🔗 "+audio)
}

// buildAudioName 组装发送文件名：歌名 - 歌手.ext。
func buildAudioName(t *track, contentType, br string) string {
	name := t.Name
	if a := t.artistLine(); a != "" {
		name += " - " + a
	}
	return sanitizeFilename(name) + audioExt(contentType, br)
}

// fetchCover best-effort 获取封面（卡片模式用），失败不影响点播。
func (p *MusicPlugin) fetchCover(ctx context.Context, source string, t *track) string {
	pid := strings.TrimSpace(t.PicID.String())
	if pid == "" {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	u, err := p.client.coverURL(cctx, source, pid)
	if err != nil {
		p.Logger.Warn("获取封面失败，忽略", "error", err, "track", t.Name)
		return ""
	}
	return u
}

// doLyric 查歌词：默认去时间轴，过长时 QQ 发 .lrc 文件，其余发截断文本。
func (p *MusicPlugin) doLyric(ctx context.Context, b bot.Bot, msg message.Message, isGroup bool, index int) {
	sess, ok := p.currentSession(msg)
	if !ok {
		p.replyText(b, msg, isGroup, "没有有效的选歌列表，先发 /点歌 关键词 搜索一下吧")
		return
	}
	if index < 1 || index > len(sess.tracks) {
		p.replyText(b, msg, isGroup, fmt.Sprintf("序号超出范围（1~%d），看下列表再选吧", len(sess.tracks)))
		return
	}
	t := sess.tracks[index-1]
	lid := strings.TrimSpace(t.LyricID.String())
	if lid == "" {
		lid = t.ID.String()
	}
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, err := p.client.lyric(sctx, sess.source, lid)
	if err != nil {
		p.Logger.Warn("获取歌词失败", "error", err, "track", t.Name, "id", lid, "user", msg.Sender.UserId)
		p.replyText(b, msg, isGroup, "获取歌词失败："+err.Error())
		return
	}
	text := strings.TrimSpace(res.Lyric)
	if text == "" {
		p.replyText(b, msg, isGroup, fmt.Sprintf("%s 没有找到歌词", t.Name))
		return
	}
	if p.cfg.LyricStripTimestamps {
		text = stripTimestamps(text)
	}
	if tr := strings.TrimSpace(res.TLyric); tr != "" {
		if p.cfg.LyricStripTimestamps {
			tr = stripTimestamps(tr)
		}
		text = strings.TrimSpace(text) + "\n————翻译————\n" + strings.TrimSpace(tr)
	}
	if len([]rune(text)) <= lyricFileMaxChars {
		p.replyText(b, msg, isGroup, "🎼 "+t.Name+" 歌词：\n"+text)
		return
	}
	if msg.Platform == "qq" && p.sendLyricFile(b, msg, isGroup, t, text) {
		return
	}
	p.replyText(b, msg, isGroup, fmt.Sprintf("🎼 %s 歌词（过长截断）：\n%s", t.Name, truncate(text, 1800)))
}

// sendLyricFile 歌词过长时以 .lrc 文件发送，发送失败返回 false 由调用方降级。
func (p *MusicPlugin) sendLyricFile(b bot.Bot, msg message.Message, isGroup bool, t *track, text string) bool {
	name := t.Name + ".lrc"
	if a := t.artistLine(); a != "" {
		name = t.Name + " - " + a + ".lrc"
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	var ok bool
	if isGroup {
		_, ok = b.SendGroupMsg(msg.GroupId, msgchain.Builder().Group().FileBase64(name, b64).Build())
	} else {
		_, ok = b.SendFriendMsg(msg.Sender.UserId, msgchain.Builder().Friend().FileBase64(name, b64).Build())
	}
	if !ok {
		p.Logger.Warn("歌词文件发送失败", "track", t.Name, "user", msg.Sender.UserId, "is_group", isGroup)
	}
	return ok
}

// sendCard 发 OneBot v11 自定义音乐卡片，发送失败返回 false 由调用方降级。
func (p *MusicPlugin) sendCard(b bot.Bot, msg message.Message, isGroup bool, title, subtitle, audio, cover string) bool {
	seg := musicCardSeg(title, subtitle, jumpURL, audio, cover)
	var ok bool
	if isGroup {
		_, ok = b.SendGroupMsg(msg.GroupId, msgchain.Builder().Group().Raw(seg).Build())
	} else {
		_, ok = b.SendFriendMsg(msg.Sender.UserId, msgchain.Builder().Friend().Raw(seg).Build())
	}
	if !ok {
		p.Logger.Warn("音乐卡片发送失败，降级", "track", title, "user", msg.Sender.UserId, "is_group", isGroup)
	}
	return ok
}

// sendAudioFile 把下载好的音频以文件发送（base64 直传），失败返回 false 由调用方兜底。
func (p *MusicPlugin) sendAudioFile(b bot.Bot, msg message.Message, isGroup bool, name string, data []byte) bool {
	b64 := base64.StdEncoding.EncodeToString(data)
	var ok bool
	if isGroup {
		_, ok = b.SendGroupMsg(msg.GroupId, msgchain.Builder().Group().FileBase64(name, b64).Build())
	} else {
		_, ok = b.SendFriendMsg(msg.Sender.UserId, msgchain.Builder().Friend().FileBase64(name, b64).Build())
	}
	if !ok {
		p.Logger.Warn("音频文件发送失败", "name", name, "size", len(data), "user", msg.Sender.UserId, "is_group", isGroup)
	}
	return ok
}

// musicCardSeg 构造 OneBot v11 自定义音乐卡片段（type=custom）。
func musicCardSeg(title, content, jump, audio, cover string) message.OB11Segment {
	data := map[string]any{
		"type":    "custom",
		"url":     jump,
		"audio":   audio,
		"title":   title,
		"content": content,
	}
	if cover != "" {
		data["image"] = cover
	}
	return message.OB11Segment{Type: "music", Data: data}
}

// sendSongLink 文本降级：歌名 + 歌手 + 播放直链。
func (p *MusicPlugin) sendSongLink(b bot.Bot, msg message.Message, isGroup bool, title, subtitle, audio string) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎵 %s\n", title)
	if subtitle != "" {
		fmt.Fprintf(&sb, "%s\n", subtitle)
	}
	fmt.Fprintf(&sb, "🔗 %s\n%s", audio, creditLine)
	p.replyText(b, msg, isGroup, sb.String())
}

// replyText 回复文本（群聊带 @）。
func (p *MusicPlugin) replyText(b bot.Bot, msg message.Message, isGroup bool, text string) {
	if isGroup {
		c := msgchain.Builder().Group().Mention(msg.Sender.UserId).Text("\n" + text).Build()
		if _, ok := b.SendGroupMsg(msg.GroupId, c); !ok {
			p.Logger.Warn("群聊回复发送失败", "group", msg.GroupId)
		}
		return
	}
	c := msgchain.Builder().Friend().Text(text).Build()
	if _, ok := b.SendFriendMsg(msg.Sender.UserId, c); !ok {
		p.Logger.Warn("私聊回复发送失败", "user", msg.Sender.UserId)
	}
}

// helpText /点歌 help 的回复。
func helpText(sessionMin int) string {
	return fmt.Sprintf(`🎵 音乐点歌
/点歌 关键词      搜索歌曲（歌名/歌手/专辑）
/点歌 序号        下载列表中的歌曲并发送文件（%d 分钟内有效）
/点歌 选 序号     同上
/点歌 歌词 序号   查看歌词
/点歌 help        查看本帮助
%s，仅供个人学习，请勿商用`, sessionMin, creditLine)
}

// filenameBadRe 文件名中的非法字符（Windows/常见聊天平台均不友好）。
var filenameBadRe = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]`)

// sanitizeFilename 清理文件名非法字符并限制长度。
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(filenameBadRe.ReplaceAllString(strings.TrimSpace(name), ""))
	if r := []rune(name); len(r) > 80 {
		name = string(r[:80])
	}
	if name == "" {
		name = "未命名"
	}
	return name
}

// audioExt 按响应 Content-Type 与音质推断文件扩展名。
func audioExt(contentType, br string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "flac"):
		return ".flac"
	case strings.Contains(ct, "mp4") || strings.Contains(ct, "m4a"):
		return ".m4a"
	case strings.Contains(ct, "ogg"):
		return ".ogg"
	case strings.Contains(ct, "wav"):
		return ".wav"
	case strings.Contains(ct, "mpeg") || strings.Contains(ct, "mp3"):
		return ".mp3"
	}
	if n, err := strconv.Atoi(strings.TrimSpace(br)); err == nil && n >= 740 {
		return ".flac" // 740/999 为无损，容器一般是 flac
	}
	return ".mp3"
}

// parseSizeBytes 解析 API 返回的文件大小（字节），解析失败按 0 处理。
func parseSizeBytes(s string) int64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || n < 0 {
		return 0
	}
	return int64(n)
}

// trackLine 列表里的一行：歌名 - 歌手 [专辑]，缺省部分跳过。
func trackLine(t *track) string {
	line := t.Name
	if a := t.artistLine(); a != "" {
		line += " - " + a
	}
	if t.Album != "" {
		line += " [" + t.Album + "]"
	}
	return line
}

// humanDur 人类可读的等待时长提示。
func humanDur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds())+1)
	}
	return fmt.Sprintf("%d 分 %d 秒", int(d.Minutes()), int(d.Seconds())%60+1)
}

// lrcTimeRe LRC 时间轴标记，如 [01:23.45]、[1:2.3]、[00:12:55]。
var lrcTimeRe = regexp.MustCompile(`\[\d{1,3}:\d{1,2}(?:[.:]\d{1,3})?\]`)

// lrcMetaRe LRC 头部信息标记，如 [ti:歌名]、[ar:歌手]、[offset:500]。
var lrcMetaRe = regexp.MustCompile(`^\[[a-zA-Z]{1,8}:.*\]\s*`)

// stripTimestamps 去掉 LRC 时间轴与头部信息标记，并压缩空行。
func stripTimestamps(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(lrcTimeRe.ReplaceAllString(line, ""))
		line = strings.TrimSpace(lrcMetaRe.ReplaceAllString(line, ""))
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// rateLimiter 滑动窗口限流器：保护官方 API 的频率配额（50 次/5 分钟）。
type rateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   []time.Time
}

func newRateLimiter(window time.Duration, limit int) *rateLimiter {
	return &rateLimiter{window: window, limit: limit}
}

// allow 尝试消费一次配额，返回 (放行与否, 需等待时长)。
func (r *rateLimiter) allow() (bool, time.Duration) {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	valid := r.hits[:0]
	for _, t := range r.hits {
		if now.Sub(t) < r.window {
			valid = append(valid, t)
		}
	}
	r.hits = valid
	if len(r.hits) >= r.limit {
		return false, r.window - now.Sub(r.hits[0])
	}
	r.hits = append(r.hits, now)
	return true, 0
}
