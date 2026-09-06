package pixiv

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 渲染插件回复文案：纯文本，QQ/飞书/TG 等平台通用。

// truncate 按 rune 截断，超出加省略号。
func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if n < 1 {
		return ""
	}
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// formatCount 数字缩写：过万显示 x.xw（中文语境的“万”）。
func formatCount(n int) string {
	if n >= 10000 {
		return fmt.Sprintf("%.1fw", float64(n)/10000)
	}
	return strconv.Itoa(n)
}

// formatDate 把 Pixiv 的 RFC3339 时间转成短格式。
func formatDate(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// renderIllustList 渲染列表类回复：标题 + 逐条作品 + 操作提示。
func renderIllustList(title string, items []illust, hint string) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	for idx := range items {
		it := &items[idx]
		mark := ""
		if it.isR18() {
			mark = " 🔞"
		}
		fmt.Fprintf(&b, "%d. %s | %s | ID:%d | ❤%s%s\n",
			idx+1, truncate(it.Title, 20), truncate(it.User.Name, 12), it.ID, formatCount(it.TotalBookmarks), mark)
	}
	if hint != "" {
		b.WriteString(hint)
	}
	return b.String()
}

// renderIllustCard 渲染单张作品卡片（/pixiv 图），page 为正在看的页码（1 基）。
func renderIllustCard(it *illust, page int) string {
	var b strings.Builder
	title := truncate(it.Title, 40)
	if it.isR18() {
		title += " 🔞"
	}
	fmt.Fprintf(&b, "🎨 %s\n", title)
	fmt.Fprintf(&b, "👤 %s（ID:%s）\n", it.User.Name, it.User.ID)
	if tags := it.tagNames(6); len(tags) > 0 {
		fmt.Fprintf(&b, "🏷 %s\n", strings.Join(tags, "、"))
	}
	kind := illustTypeName(it.Type)
	if it.PageCount > 1 {
		fmt.Fprintf(&b, "📄 %s · 共 %d 页（第 %d 页）｜❤%s｜👁%s\n",
			kind, it.PageCount, page, formatCount(it.TotalBookmarks), formatCount(it.TotalView))
	} else {
		fmt.Fprintf(&b, "📄 %s｜❤%s｜👁%s\n", kind, formatCount(it.TotalBookmarks), formatCount(it.TotalView))
	}
	if it.Series != nil && it.Series.Title != "" {
		fmt.Fprintf(&b, "📚 系列：%s\n", truncate(it.Series.Title, 30))
	}
	if t := formatDate(it.CreateDate); t != "" {
		fmt.Fprintf(&b, "🕐 %s\n", t)
	}
	b.WriteString("🔗 ")
	b.WriteString(it.webURL())
	if it.PageCount > 1 {
		fmt.Fprintf(&b, "\n💡 /pixiv 图 %d <页码> 翻页", it.ID)
	}
	return b.String()
}

// illustTypeName 作品类型中文名。
func illustTypeName(t string) string {
	switch t {
	case "manga":
		return "漫画"
	case "ugoira":
		return "动图"
	default:
		return "插画"
	}
}

// renderUserHeader 画师信息抬头（画师详情接口失败时由调用方降级省略）。
func renderUserHeader(d *userDetailResp) string {
	if d.User.ID == "" {
		return ""
	}
	s := fmt.Sprintf("👤 %s（ID:%s）", truncate(d.User.Name, 20), d.User.ID)
	if d.Profile.TotalIllusts > 0 || d.Profile.TotalManga > 0 {
		s += fmt.Sprintf(" · 插画 %d · 漫画 %d", d.Profile.TotalIllusts, d.Profile.TotalManga)
	}
	return s
}

// pixivHelpText /pixiv 帮助文案。
const pixivHelpText = `🎨 Pixiv 插件（登录后可用，群聊需 @机器人）
/pixiv 搜索 <关键词> [页码] — 搜索插画
/pixiv 图 <作品ID> [页码] — 查看作品并发图，多图可翻页
/pixiv 排行 [日|周|月] [r18] — 插画排行榜
/pixiv 推荐 — 为登录账号个性化推荐
/pixiv 画师 <用户ID> [页码] — 画师近期插画
/pixiv 相关 <作品ID> — 相似作品
/pixiv 状态 — 登录与使用状态
ID 就是 pixiv 网页链接里的数字（artworks/12345、users/678）
分级、限流与放行名单在面板「配置管理」→ Pixiv 里调整`

// pixivNotLoginText 未配置 refresh_token 时的统一提示。
const pixivNotLoginText = "还没登录 Pixiv：请管理员在面板「配置管理」→ Pixiv 里填入 Refresh Token（获取方法见插件 README），配置后立即生效"

// pixivDenyText 非放行会话的拒绝提示。
const pixivDenyText = "这里不在 Pixiv 插件的放行名单里，请联系管理员在面板配置 allow_groups / allow_friends"

// pixivAPIErrText 把 API 错误翻译成用户能看懂的提示（原始错误进日志）。
func pixivAPIErrText(err error) string {
	if err == nil {
		return "Pixiv 请求失败"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "refresh_token"):
		return "Pixiv 登录已失效：" + s
	case strings.Contains(s, "403"), strings.Contains(s, "限流"):
		return "请求太频繁被 Pixiv 限流了，稍等一分钟再试"
	case strings.Contains(s, "deadline exceeded"), strings.Contains(s, "Timeout"), strings.Contains(s, "timeout"):
		return "连接 Pixiv 超时：请检查面板里的 proxy 代理配置（国内直连不可达）"
	default:
		if len(s) > 140 {
			s = s[:140] + "……"
		}
		return "Pixiv 请求失败：" + s
	}
}
