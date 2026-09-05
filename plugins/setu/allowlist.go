package setu

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/jeanhua/AniaBot/common/model/message"
)

// matchAllowlist 正则放行名单匹配：每条 pattern 先尝试精确匹配，
// 再作为正则对完整 ID（如 qq:123456）与裸 ID（如 123456）同时匹配。
// 纯数字 pattern 会被规范化为 qq: 前缀后再比对，跨平台 ID 原样处理。
func matchAllowlist(patterns []string, qid message.QID) bool {
	full := qid.String()
	bare := qid.TrimQQPrefix()
	cands := []string{full}
	if bare != "" && bare != full {
		cands = append(cands, bare)
	}
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		for _, c := range cands {
			if pat == c {
				return true
			}
		}
		if message.FromString(pat).String() == full {
			return true
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		for _, c := range cands {
			if re.MatchString(c) {
				return true
			}
		}
	}
	return false
}

// invalidPatterns 找出配错了的正则，Start 时打日志提醒管理员。
func invalidPatterns(patterns []string) []string {
	var bad []string
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if _, err := regexp.Compile(pat); err != nil {
			bad = append(bad, pat)
		}
	}
	return bad
}

// countRe 数量写法：3 / 3连 / 3张 / 3发 / 3枚 / 3个 / 3次 / x3。
var countRe = regexp.MustCompile(`^(?:[xX×](\d+)|(\d+)(?:连|张|发|枚|个|次)?)$`)

// parseCountToken 解析单个数量 token，超上限时截断到 maxCount。
func parseCountToken(s string, maxCount int) (int, bool) {
	if maxCount <= 0 {
		maxCount = 1
	}
	m := countRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	numStr := m[1]
	if numStr == "" {
		numStr = m[2]
	}
	n, err := strconv.Atoi(numStr)
	if err != nil || n < 1 {
		return 0, false
	}
	if n > maxCount {
		n = maxCount
	}
	return n, true
}

// setuRequest /setu 参数解析结果。
type setuRequest struct {
	count     int
	tags      []string
	wantR18   bool
	wantSafe  bool
	isHelp    bool
	isStatus  bool
	isRefresh bool
}

// parseSetuArgs 解析 /setu 参数：首个 token 可能是 help/status/refresh/数量，
// 其余为 tag；r18/safe 类 token 被吃掉转为 R18 意图，不进入 tag。
func parseSetuArgs(args []string, maxCount int) setuRequest {
	r := setuRequest{count: 1}
	if len(args) == 0 {
		return r
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "help", "h", "帮助", "?", "？":
		r.isHelp = true
		return r
	case "status", "st", "状态":
		r.isStatus = true
		return r
	case "refresh", "reload", "刷新":
		r.isRefresh = true
		return r
	}
	rest := args
	if n, ok := parseCountToken(args[0], maxCount); ok {
		r.count = n
		rest = args[1:]
	}
	for _, a := range rest {
		t := strings.TrimSpace(a)
		if t == "" {
			continue
		}
		switch strings.ToLower(t) {
		case "r18", "r-18", "r18!", "🔞":
			r.wantR18 = true
			continue
		case "safe", "safety", "全年龄", "全年龄向", "green", "绿":
			r.wantSafe = true
			continue
		}
		r.tags = append(r.tags, t)
	}
	if len(r.tags) > 5 {
		r.tags = r.tags[:5]
	}
	return r
}

// resolveR18 结合全局模式与本次意图算出实际的 r18 参数。
// 返回 ("0"/"1"/"2", 是否拒绝)。mode 0 下还敢要 R18 直接拒绝并调侃。
func resolveR18(mode string, req setuRequest) (string, bool) {
	switch mode {
	case "0":
		if req.wantR18 {
			return "", true
		}
		return "0", false
	case "1":
		return "1", false
	default:
		if req.wantR18 && !req.wantSafe {
			return "1", false
		}
		if req.wantSafe && !req.wantR18 {
			return "0", false
		}
		return "2", false
	}
}

// rewriteImageHost 按配置替换图片 host（如索引域名打不开时切镜像），
// proxy 留空则原样返回。
func rewriteImageHost(rawURL, proxy string) string {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" || rawURL == "" {
		return rawURL
	}
	proxy = strings.TrimPrefix(strings.TrimPrefix(proxy, "https://"), "http://")
	proxy = strings.TrimSuffix(proxy, "/")
	if proxy == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	u.Host = proxy
	return u.String()
}

// displayName 发送者展示昵称（群名片优先）。
func displayName(msg message.Message) string {
	name := strings.TrimSpace(msg.Sender.Card)
	if name == "" {
		name = strings.TrimSpace(msg.Sender.Nickname)
	}
	if name == "" {
		return "绅士"
	}
	return name
}
