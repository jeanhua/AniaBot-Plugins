// Package dicegirl 骰娘插件：为群聊/私聊提供 TRPG 掷骰能力。
//
// 支持三种入口：
//  1. 群聊 @机器人 + /r /ra /sc /jrrp；
//  2. 私聊 /r /ra /sc /jrrp；
//  3. 群聊/私聊直接发送以 . 或 。或 ! 或 ！ 开头的裸指令（如 .r 2d6、.ra 70）。
package dicegirl

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jeanhua/AniaBot/common/bot"
	"github.com/jeanhua/AniaBot/common/model/command"
	"github.com/jeanhua/AniaBot/common/model/message"
	"github.com/jeanhua/AniaBot/common/msgchain"
	"github.com/jeanhua/AniaBot/common/plugin"
	"github.com/jeanhua/AniaBot/common/plugininfo"
)

// diceConfig 骰娘插件配置。实现 plugin.ConfigSchemaProvider 后，
// 面板「配置管理」会自动渲染表单，框架在 Start 前填充到 p.cfg。
type diceConfig struct {
	Enable       bool `cfg:"plugin.dicegirl.enable" label:"启用骰娘" group:"骰娘" default:"true" help:"关闭后不响应任何骰娘指令"`
	EnableLegacy bool `cfg:"plugin.dicegirl.enable_legacy" label:"启用裸指令" group:"骰娘" default:"true" help:"允许直接发送 .r/.ra/.sc/.jrrp 等以点号开头的指令（无需 @ 机器人）"`
	Persona      bool `cfg:"plugin.dicegirl.persona" label:"卖萌语气" group:"骰娘" default:"true" help:"回复末尾追加 ~、♪ 等口癖"`
	MaxDice      int  `cfg:"plugin.dicegirl.max_dice" label:"每组最大骰数" group:"骰娘" default:"100" help:"单个骰组数量上限（如 2d6 的 2）"`
	MaxFaces     int  `cfg:"plugin.dicegirl.max_faces" label:"最大骰面" group:"骰娘" default:"1000" help:"骰子面数上限"`
	MaxGroups    int  `cfg:"plugin.dicegirl.max_groups" label:"最大骰组数" group:"骰娘" default:"20" help:"单个表达式里骰组数量上限（2d6+1d4 为两组）"`
	MaxTotalDice int  `cfg:"plugin.dicegirl.max_total_dice" label:"总骰数上限" group:"骰娘" default:"500" help:"单个表达式允许的骰子总数上限"`
}

// DiceGirlPlugin 骰娘插件定义：嵌入 plugin.Meta 获得默认实现。
type DiceGirlPlugin struct {
	plugin.Meta
	cfg diceConfig
}

// NewPlugin 构造函数（plugin.json 的 entry.constructor 默认指向这里）。
func NewPlugin() *DiceGirlPlugin {
	p := &DiceGirlPlugin{}
	p.Name = "骰娘"
	p.HelpWords = "@我 /r 2d6+3 掷骰、/ra 70 检定、/sc 1/1d6 60 理智、/jrrp 人品；.help 或 @我 /骰子 看详细玩法"
	p.AdminOnly = false
	p.ShowFor = plugininfo.ShowForGroup | plugininfo.ShowForFriend
	p.Author = "jeanhua"
	p.Version = "1.0.0"
	p.Order = plugin.LevelNormal
	return p
}

// ConfigSchema 声明配置结构体（面板表单 + 默认值 + Start 前自动填充）。
func (p *DiceGirlPlugin) ConfigSchema() any { return &p.cfg }

// limits 由配置构造本次掷骰限制。
func (p *DiceGirlPlugin) limits() diceLimits {
	return diceLimits{
		maxDice:      p.cfg.MaxDice,
		maxFaces:     p.cfg.MaxFaces,
		maxGroups:    p.cfg.MaxGroups,
		maxTotalDice: p.cfg.MaxTotalDice,
	}
}

// OnGroupMsg 群聊消息事件：返回 (是否继续传播, 错误)。
func (p *DiceGirlPlugin) OnGroupMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	if !p.cfg.Enable {
		return true, nil
	}
	// 框架命令（@机器人后发送 /xxx）
	if cmd.Name != "" {
		if !cmd.Mention {
			return true, nil
		}
		if p.dispatch(ctx, b, cmd.Name, cmd.Args, msg, true, true) {
			return false, nil
		}
		return true, nil
	}
	// 裸指令（.r/.ra/.sc/.jrrp 等，无需 @）
	if p.cfg.EnableLegacy && p.dispatchLegacy(ctx, b, msg, true) {
		return false, nil
	}
	return true, nil
}

// OnFriendMsg 私聊消息事件：与群聊一致，命令不要求 @。
func (p *DiceGirlPlugin) OnFriendMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	if !p.cfg.Enable {
		return true, nil
	}
	if cmd.Name != "" {
		if p.dispatch(ctx, b, cmd.Name, cmd.Args, msg, false, false) {
			return false, nil
		}
		return true, nil
	}
	if p.cfg.EnableLegacy && p.dispatchLegacy(ctx, b, msg, false) {
		return false, nil
	}
	return true, nil
}

// dispatch 处理框架命令。返回是否已消费该消息。
func (p *DiceGirlPlugin) dispatch(ctx context.Context, b bot.Bot, name string, args []string, msg message.Message, group, mentioned bool) bool {
	cmdCtx := diceContext{group: group, mentioned: mentioned, nick: displayName(msg), senderID: msg.Sender.UserId, groupID: msg.GroupId}
	switch strings.ToLower(name) {
	case "r", "roll", "掷骰", "骰":
		p.sendReply(b, cmdCtx, p.rollMessage(cmdCtx.nick, args))
		return true
	case "ra", "check", "检定":
		p.sendReply(b, cmdCtx, p.raMessage(cmdCtx.nick, args))
		return true
	case "sc", "理智":
		p.sendReply(b, cmdCtx, p.scMessage(cmdCtx.nick, args))
		return true
	case "jrrp", "人品":
		p.sendReply(b, cmdCtx, p.jrrpMessage(cmdCtx.nick, cmdCtx.senderID.String()))
		return true
	case "dice", "骰子", "骰娘", "帮助":
		p.sendReply(b, cmdCtx, helpText)
		return true
	}
	return false
}

// dispatchLegacy 解析 ".r 2d6 原因" 形式的裸指令。返回是否已消费该消息。
func (p *DiceGirlPlugin) dispatchLegacy(ctx context.Context, b bot.Bot, msg message.Message, group bool) bool {
	text, mentioned := extractText(msg)
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	if r != '.' && r != '。' && r != '!' && r != '！' {
		return false
	}
	_, size := utf8.DecodeRuneInString(text)
	body := strings.TrimSpace(text[size:])
	if body == "" {
		return false
	}

	word := body
	rest := ""
	if idx := strings.IndexAny(body, " \t\r\n"); idx >= 0 {
		word = body[:idx]
		rest = strings.TrimSpace(body[idx+1:])
	}
	lower := strings.ToLower(word)

	cmdCtx := diceContext{group: group, mentioned: mentioned && group, nick: displayName(msg), senderID: msg.Sender.UserId, groupID: msg.GroupId}

	switch {
	case lower == "r" || lower == "roll" || lower == "掷骰":
		p.sendReply(b, cmdCtx, p.rollMessage(cmdCtx.nick, strings.Fields(rest)))
		return true
	case strings.HasPrefix(lower, "r") && len(lower) > 1 && isDiceStart(lower[1:]):
		// 紧凑形式 .r2d6 或 .rd6
		args := word[1:]
		if rest != "" {
			args += " " + rest
		}
		p.sendReply(b, cmdCtx, p.rollMessage(cmdCtx.nick, strings.Fields(args)))
		return true

	case lower == "ra" || lower == "check" || lower == "检定":
		p.sendReply(b, cmdCtx, p.raMessage(cmdCtx.nick, strings.Fields(rest)))
		return true
	case strings.HasPrefix(lower, "ra") && len(lower) > 2 && isDigit(lower[2]):
		// 紧凑形式 .ra70
		args := word[2:]
		if rest != "" {
			args += " " + rest
		}
		p.sendReply(b, cmdCtx, p.raMessage(cmdCtx.nick, strings.Fields(args)))
		return true

	case lower == "sc" || lower == "理智":
		p.sendReply(b, cmdCtx, p.scMessage(cmdCtx.nick, strings.Fields(rest)))
		return true
	case strings.HasPrefix(lower, "sc") && len(lower) > 2 && isDiceStart(lower[2:]):
		// 紧凑形式 .sc1/1d6 60
		args := word[2:]
		if rest != "" {
			args += " " + rest
		}
		p.sendReply(b, cmdCtx, p.scMessage(cmdCtx.nick, strings.Fields(args)))
		return true

	case lower == "help" || lower == "帮助":
		p.sendReply(b, cmdCtx, helpText)
		return true

	case lower == "jrrp" || lower == "人品":
		p.sendReply(b, cmdCtx, p.jrrpMessage(cmdCtx.nick, cmdCtx.senderID.String()))
		return true
	}
	return false
}

// isDiceStart 判断去掉命令前缀后的串是否以数字或 d/D 开头（骰子表达式合法开头）。
func isDiceStart(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= '0' && c <= '9' || c == 'd' || c == 'D'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// looksLikeDice 粗略判断 token 是否像骰子表达式（含 d/D/% 且含数字或 %）。
func looksLikeDice(s string) bool {
	if !strings.ContainsAny(s, "dD%") {
		return false
	}
	return strings.ContainsAny(s, "0123456789%")
}

// isHelpArg 判断参数是否请求查看玩法说明。
func isHelpArg(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "help", "帮助", "用法", "怎么玩":
		return true
	}
	return false
}

// diceContext 一次指令回复的上下文。
type diceContext struct {
	group     bool
	mentioned bool
	nick      string
	senderID  message.QID
	groupID   message.QID
}

// sendReply 发送回复消息；群聊且用户 @ 了机器人时回 @。
func (p *DiceGirlPlugin) sendReply(b bot.Bot, c diceContext, text string) {
	if text == "" {
		return
	}
	if strings.HasPrefix(text, "🎲 ") {
		text = p.decorate(text)
	}
	if c.group {
		chain := msgchain.Builder().Group()
		if c.mentioned {
			chain.Mention(c.senderID)
			text = " " + text
		}
		chain.Text(text)
		b.SendGroupMsg(c.groupID, chain.Build())
		return
	}
	chain := msgchain.Builder().Friend()
	chain.Text(text)
	b.SendFriendMsg(c.senderID, chain.Build())
}

// rollMessage 普通掷骰：/r 2d6+3 [原因]。
func (p *DiceGirlPlugin) rollMessage(nick string, args []string) string {
	if isHelpArg(args) {
		return helpText
	}
	lim := p.limits()
	if len(args) == 0 {
		args = []string{"d100"}
	}
	expr, reason, err := splitExprAndReason(args, lim)
	if err != nil {
		return hint + err.Error()
	}
	nd, err := parseDiceExpression(expr, lim)
	if err != nil {
		return hint + err.Error()
	}
	if !nd.hasDice() {
		return hint + "掷骰表达式里需要至少一个骰子，例如 /r d10 或 /r 2d6+3"
	}
	val, detail := rollNode(nd, rand.Intn)
	msg := fmt.Sprintf("🎲 %s 掷骰 %s", nick, expr)
	if reason != "" {
		msg += "（" + reason + "）"
	}
	msg += "：" + detail + " = " + strconv.Itoa(val)
	return msg
}

// raMessage COC 7e 技能检定：/ra [技能名] 技能值 或 /ra 技能值 [技能名]。
func (p *DiceGirlPlugin) raMessage(nick string, args []string) string {
	if len(args) == 0 {
		return usageText
	}
	if isHelpArg(args) {
		return helpText
	}
	rating := 0
	skillParts := make([]string, 0, len(args))
	for _, a := range args {
		if looksLikeDice(a) {
			return hint + "检定指令不接受骰子表达式，掷骰请使用 /r（如 /r " + a + "）"
		}
		if v, e := strconv.Atoi(a); e == nil {
			if rating != 0 {
				return hint + "只能提供一个技能值"
			}
			rating = v
			continue
		}
		skillParts = append(skillParts, a)
	}
	if rating < 1 || rating > 99 {
		return hint + "技能值需为 1~99 的整数"
	}
	skill := strings.TrimSpace(strings.Join(skillParts, " "))
	if skill == "" {
		skill = "检定"
	}
	roll := rand.Intn(100) + 1
	lv, label := cocCheck(roll, rating)
	msg := fmt.Sprintf("🎲 %s「%s」检定：D100=%d/%d %s", nick, skill, roll, rating, label)
	if lv == levelSuccess {
		msg += fmt.Sprintf("（困难 %d / 极难 %d）", rating/2, rating/5)
	}
	return msg
}

// scMessage 理智检定：/sc 成功损失/失败损失 [当前SAN] [原因]。
func (p *DiceGirlPlugin) scMessage(nick string, args []string) string {
	if len(args) == 0 {
		return usageText
	}
	if isHelpArg(args) {
		return helpText
	}
	// 找到形如 1/1d6 的损失表达式 token
	lossIdx := -1
	for i, a := range args {
		if strings.Contains(a, "/") {
			lossIdx = i
			break
		}
	}
	if lossIdx < 0 {
		return usageText
	}
	lossParts := strings.SplitN(args[lossIdx], "/", 2)
	successExpr, failureExpr := lossParts[0], lossParts[1]
	if successExpr == "" || failureExpr == "" {
		return hint + "理智损失格式应为 成功损失/失败损失，例如 /sc 1/1d6 60"
	}

	san := -1
	reasonParts := make([]string, 0)
	for _, a := range args[lossIdx+1:] {
		if v, e := strconv.Atoi(a); e == nil {
			if san >= 0 {
				return hint + "只能提供一个当前 SAN 值"
			}
			san = v
			continue
		}
		reasonParts = append(reasonParts, a)
	}
	if san < 0 || san > 999 {
		return hint + "请提供当前 SAN 值（0~999），例如 /sc 1/1d6 60"
	}

	lim := p.limits()
	successNode, err := parseDiceExpression(successExpr, lim)
	if err != nil {
		return hint + "成功损失表达式无效：" + err.Error()
	}
	failureNode, err := parseDiceExpression(failureExpr, lim)
	if err != nil {
		return hint + "失败损失表达式无效：" + err.Error()
	}

	roll := rand.Intn(100) + 1
	lv, label := cocCheck(roll, san)
	var loss int
	switch {
	case lv == levelFumble:
		loss = failureNode.maxValue()
	case lv == levelFail:
		loss, _ = rollNode(failureNode, rand.Intn)
	default:
		loss, _ = rollNode(successNode, rand.Intn)
	}
	newSan := san - loss
	if newSan < 0 {
		newSan = 0
	}

	msg := fmt.Sprintf("🎲 %s 理智检定：D100=%d/%d %s，理智损失 %d，SAN %d→%d", nick, roll, san, label, loss, san, newSan)
	if lv == levelFumble {
		msg += "（呜……大失败，按最大损失扣除）"
	} else if newSan == 0 {
		msg += "（理智归零了……）"
	}
	if reason := strings.TrimSpace(strings.Join(reasonParts, " ")); reason != "" {
		msg += "（" + reason + "）"
	}
	return msg
}

// jrrpMessage 今日人品：按“日期 + 用户ID”确定性生成 1~100。
func (p *DiceGirlPlugin) jrrpMessage(nick, userID string) string {
	v := jrrpValue(userID, nowDate())
	msg := fmt.Sprintf("🎲 %s 今日人品：%d/100", nick, v)
	switch {
	case v >= 95:
		msg += " 欧气爆棚，去买张彩票吧！"
	case v >= 80:
		msg += " 运气不错，诸事皆宜~"
	case v >= 60:
		msg += " 中规中矩，稳步前进。"
	case v >= 40:
		msg += " 平平淡淡才是真。"
	case v >= 20:
		msg += " 今天宜苟，不宜浪。"
	default:
		msg += " 非酋认证……摸摸头，明天会好的。"
	}
	return msg
}

// nowDate 便于测试替换的日期函数。
var nowDate = func() string {
	return timeNow().Format("2006-01-02")
}

var timeNow = time.Now

const (
	hint      = "💡 "
	usageText = "骰娘用法：\n/r 2d6+3 [原因] 掷骰（支持 d%、3d6k2、+-*/）\n/ra 侦查 70 或 /ra 70 技能检定\n/sc 1/1d6 60 [原因] 理智检定\n/jrrp 查看今日人品\n群聊请 @我，也可以直接发 .r/.ra/.sc/.jrrp，发送 .help 查看详细说明"
	helpText  = `骰娘怎么玩？

【1. 掷骰】
  @我 /r 2d6+3 挥砍    或直接 .r 2d6 挥砍
  · 不带表达式默认掷 D100
  · d% 等价于 d100；3d6k2 = 3 个 d6 取最大 2 个
  · 支持 + - * /（x、× 也是乘号），例如 1d4+1d6+2

【2. COC 技能检定】
  @我 /ra 侦查 70    或 .ra 侦查 70
  · 自动掷 D100 判定：
    1 = 大成功；≤技能/5 = 极难成功；≤技能/2 = 困难成功
    ≤技能 = 成功；100 = 大失败；技能<50 时 96~100 也是大失败

【3. 理智检定】
  /sc 成功损失/失败损失 当前SAN [原因]
  例：/sc 1/1d6 60 直面外神
  · 成功扣 1 点、失败扣 1d6 点，大失败按最大损失扣
  · 群聊请 @我 再发送

【4. 今日人品】
  /jrrp   查看今日人品值（每天更新，无需存储）

小贴士：群聊中 / 开头的命令需要 @我；以 .（或 。!！）开头的裸指令不需要 @。`
)

// personaSuffixes 卖萌口癖词库。
var personaSuffixes = []string{"~", "～", "♪", "☆", "喵~", "！", "……"}

// decorate 根据配置在消息末尾追加口癖。
func (p *DiceGirlPlugin) decorate(s string) string {
	if !p.cfg.Persona || s == "" {
		return s
	}
	return s + personaSuffixes[rand.Intn(len(personaSuffixes))]
}

// displayName 发送者展示昵称。
func displayName(msg message.Message) string {
	name := strings.TrimSpace(msg.Sender.Card)
	if name == "" {
		name = strings.TrimSpace(msg.Sender.Nickname)
	}
	if name == "" {
		return "冒险者"
	}
	return name
}

// extractText 提取消息中的纯文本（忽略 @、图片等段），返回 (文本, 是否@机器人)。
func extractText(msg message.Message) (string, bool) {
	var b strings.Builder
	mentioned := false
	for _, seg := range msg.Message {
		switch seg.Type {
		case message.SegmentText:
			if t, ok := seg.Data["text"].(string); ok {
				b.WriteString(t)
			}
		case message.SegmentMention:
			if qq, ok := seg.Data["qq"].(string); ok && qq != "all" {
				if message.FromString(qq) == message.FromString(msg.SelfId.String()) {
					mentioned = true
				}
			}
		}
	}
	return b.String(), mentioned
}
