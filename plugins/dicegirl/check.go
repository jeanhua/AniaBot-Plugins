// 本文件实现 COC 7e 检定分级与「今日人品」确定性随机逻辑。
package dicegirl

import (
	"hash/fnv"
)

// checkLevel COC 7e 检定结果分级（数值从低到高）。
type checkLevel int

const (
	levelFumble   checkLevel = iota // 大失败
	levelFail                       // 失败
	levelSuccess                    // 成功
	levelHard                       // 困难成功
	levelExtreme                    // 极难成功
	levelCritical                   // 大成功
)

// cocCheck 按 COC 7e 规则书判定一次 D100 检定结果并返回文本标签。
//
// 规则（与主流骰娘一致）：
//   - 1 = 大成功
//   - 100 = 大失败；技能 <50 时 96~100 均为大失败
//   - ≤ 技能值/5（向下取整）= 极难成功
//   - ≤ 技能值/2（向下取整）= 困难成功
//   - ≤ 技能值 = 成功，否则失败
func cocCheck(roll, rating int) (checkLevel, string) {
	if roll == 100 || (roll >= 96 && rating < 50) {
		return levelFumble, "大失败"
	}
	if roll == 1 {
		return levelCritical, "大成功"
	}
	switch {
	case roll <= rating/5:
		return levelExtreme, "极难成功"
	case roll <= rating/2:
		return levelHard, "困难成功"
	case roll <= rating:
		return levelSuccess, "成功"
	default:
		return levelFail, "失败"
	}
}

// jrrpValue 由“日期 + 用户 ID”生成确定的 1~100 人品值。
func jrrpValue(userID, day string) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(userID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(day))
	return int(h.Sum64()%100) + 1
}
