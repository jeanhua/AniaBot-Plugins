package pixiv

import (
	"regexp"
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
