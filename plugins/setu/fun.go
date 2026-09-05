package setu

import (
	"fmt"
	"math/rand"
	"strings"
)

// 趣味文案池：涩图插件的灵魂。调用方用 pickRand 随机取一条。

var summonLines = []string{
	"少女祈祷中……锵锵！%s 要的 %d 张好东西来了~",
	"从 Pixiv 深处偷来了 %s 要的 %d 张，接好！",
	"嘿嘿，%s 的 %d 连发车了，坐稳！",
	"芜湖，起飞！给 %s 的 %d 张~",
	"嘘……小声点，%s 的 %d 张到了",
	"P站老司机发车，%s 的 %d 连请查收！",
}

var denyMessages = []string{
	"这里还不是涩涩开放区哦，让管理员把本群/你加入放行名单后再来~",
	"⛔ 非开放区，涩图发射失败……（管理员在面板配置管理里放行本群/好友即可）",
	"不可以涩涩！（本群还不在放行名单里，快去抱管理员大腿）",
	"雷达显示此处不宜涩涩，换个已放行的地方再试试~",
}

var cooldownMessages = []string{
	"慢一点啦，贤者时间还没过（剩余 %ds），喝口水再来~",
	"太快了太快了！冷却中（剩余 %ds），先聊会天~",
	"涩图过载警告！%ds 后再来找我~",
}

var dailyMessages = []string{
	"今天的配额用光啦（%d/%d），明天再来，适度涩涩有益健康~",
	"被榨干了……今日额度 %d/%d 已用完，早点休息~",
}

var emptyMessages = []string{
	"在 %d 个候选中没找到 [%s] 相关的涩图，换个 tag 试试~",
	"翻了 %d 张都没找到 [%s]，P站少女表示没听过这个 XP……",
	"翻了 %d 张都没找到 [%s]，太冷门了，试试更通用的词？",
}

var errorMessages = []string{
	"Pixiv 少女们躲起来了（索引/网络开小差），稍后再试~",
	"连接 P站失败，可能是索引还没加载好或网络波动，过会再来~",
}

var r18SealedMessages = []string{
	"🔞 R18 被封印了，本机器人只提供全年龄内容（管理员可在配置里切换 R18 模式）",
	"封印解除失败！当前为全年龄模式，想要 R18 请找管理员~",
}

var footers = []string{
	"⚠️ R18 内容，请确保已成年并遵守群规与法律法规",
	"💡 @我 /setu 白丝 可按 tag 搜索，/setu help 看全部玩法",
	"🍵 多喝热水，适度涩涩",
	"⭐ 喜欢的话去 Pixiv 给画师点个赞吧",
	"",
}

func pickRand(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[rand.Intn(len(list))]
}

func summonLine(nick string, n int) string {
	return fmt.Sprintf(pickRand(summonLines), nick, n)
}

// captionLine 单图的配文：标题/画师/pid/标签，主打一个信息齐全。
func captionLine(idx, total int, m *PixivMeta) string {
	var b strings.Builder
	if total > 1 {
		fmt.Fprintf(&b, "【%d/%d】", idx, total)
	}
	title := m.Title
	if title == "" {
		title = "无题"
	}
	fmt.Fprintf(&b, "🖼️《%s》 by %s", title, m.Author)
	if m.R18 {
		b.WriteString(" 🔞")
	}
	fmt.Fprintf(&b, "\n🔗 https://www.pixiv.net/artworks/%d", m.Pid)
	if m.P > 0 {
		fmt.Fprintf(&b, "（第 %d 张）", m.P+1)
	}
	tags := m.Tags
	if len(tags) > 8 {
		tags = tags[:8]
	}
	if len(tags) > 0 {
		fmt.Fprintf(&b, "\n🏷️ %s", strings.Join(tags, " "))
	}
	return b.String()
}

const helpTextClean = `涩图插件 /setu 玩法：
/setu                  随机来一张
/setu 白丝             按 tag 搜索（空格分隔多个 tag 为 AND，可混中日文）
/setu 3 白丝           多连发（1~5，支持 3连/3张/x3 写法）
/setu r18 /setu safe   本次指定 R18/全年龄（受全局 R18 模式约束）
/setu status           查看放行状态、剩余额度与索引情况
/setu refresh          管理员：立即刷新索引缓存
/setu help             显示本帮助

群聊请 @我再发，私聊直接发就行。
只有放行名单里的群/好友能开车，其余地方会被拒之门外；
tag 搜不到时换个通用词试试。适度涩涩，注意身体🍵`

// 以下为下载/发送失败时的面向用户文案：必须说清哪张、为什么、怎么办，绝不静默。

// setuTitle 作品标题展示（空标题回退“无题”，失败文案里也保持可读）。
func setuTitle(m *PixivMeta) string {
	if m == nil || strings.TrimSpace(m.Title) == "" {
		return "无题"
	}
	return strings.TrimSpace(m.Title)
}

// downloadFailSummary 部分下载失败时的汇总（一句话 + 每张原因），拼在成功消息末尾。
func downloadFailSummary(failed []setuDownload) string {
	if len(failed) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "⚠️ %d 张下载失败，只发出了成功的部分", len(failed))
	for _, f := range failed {
		var pid int64
		if f.meta != nil {
			pid = f.meta.Pid
		}
		fmt.Fprintf(&b, "\n· pid %d：%s", pid, shortDownloadErr(f.err))
	}
	b.WriteString("\n💡 稍后再试，或让管理员换 image_proxy 镜像")
	return b.String()
}

// allDownloadFailText 全部下载失败时的单独回复（含作品页链接兜底，至少能点进去看）。
func allDownloadFailText(failed []setuDownload) string {
	var b strings.Builder
	b.WriteString("😭 图片都下载失败了，一张也没发出来")
	for _, f := range failed {
		if f.meta != nil {
			fmt.Fprintf(&b, "\n·《%s》：%s\n  作品页：%s", setuTitle(f.meta), shortDownloadErr(f.err), f.meta.ArtworkURL())
		} else {
			fmt.Fprintf(&b, "\n· 未知图片：%s", shortDownloadErr(f.err))
		}
	}
	b.WriteString("\n💡 稍后再试，或让管理员换 image_proxy（如 i.pixiv.cat）")
	return b.String()
}

// sendFailSummary 图已下载但 Bot 返回失败时的汇总（多为 NapCat 网络波动、风控或消息过大）。
func sendFailSummary(failed []setuDownload) string {
	if len(failed) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "😭 %d 张图已下载但发送失败（Bot 返回失败），可能是网络波动、风控或图片过大", len(failed))
	for _, f := range failed {
		if f.meta != nil {
			fmt.Fprintf(&b, "\n·《%s》作品页：%s", setuTitle(f.meta), f.meta.ArtworkURL())
		}
	}
	b.WriteString("\n💡 作品页可点进去看；持续失败请让管理员看日志排查")
	return b.String()
}
