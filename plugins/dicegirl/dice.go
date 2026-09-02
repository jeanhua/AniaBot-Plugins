// Package dicegirl 骰娘插件：TRPG 掷骰 / COC 检定 / 理智检定 / 今日人品。
//
// 本文件实现骰子表达式（xdy / d% / 取骰 k / 四则运算）的解析与投掷。
package dicegirl

import (
	"fmt"
	"strconv"
	"strings"
)

// diceLimits 单次掷骰的防滥用限制。
type diceLimits struct {
	maxDice      int // 每组骰子数量上限
	maxFaces     int // 骰子面数上限
	maxGroups    int // 表达式中的骰组数量上限（如 2d6+1d4 为两组）
	maxTotalDice int // 全部骰组骰子总数上限
}

// node 骰子表达式 AST 节点。
type node interface {
	// roll 投掷并返回数值与可读的分解串（如 "(4+2)+3"）。
	roll(f func(int) int) (int, string)
	// maxValue 表达式可能的最大值（理智检定大失败扣除用）。
	maxValue() int
	// hasDice 是否包含至少一个骰子。
	hasDice() bool
}

// numNode 常数节点。
type numNode int

func (n numNode) roll(func(int) int) (int, string) {
	return int(n), strconv.Itoa(int(n))
}

func (n numNode) maxValue() int { return int(n) }

func (n numNode) hasDice() bool { return false }

// diceNode 一组骰子：count 个 faces 面骰。
type diceNode struct {
	count    int
	faces    int
	keepHigh bool // true=取最大，false=取最小（kl）
	keep     int  // 保留的骰子个数；0 表示全保留
}

func (d *diceNode) roll(f func(int) int) (int, string) {
	rolls := make([]int, d.count)
	for i := range rolls {
		rolls[i] = f(d.faces) + 1
	}
	if d.keep == 0 {
		if d.count == 1 {
			return rolls[0], strconv.Itoa(rolls[0])
		}
		sum := 0
		parts := make([]string, len(rolls))
		for i, v := range rolls {
			sum += v
			parts[i] = strconv.Itoa(v)
		}
		return sum, "(" + strings.Join(parts, "+") + ")"
	}

	// 取骰：先升序排序，再按方向取前/后 keep 个。
	sorted := make([]int, len(rolls))
	copy(sorted, rolls)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	var kept []int
	if d.keepHigh {
		kept = sorted[len(sorted)-d.keep:]
	} else {
		kept = sorted[:d.keep]
	}
	sum := 0
	keptParts := make([]string, len(kept))
	for i, v := range kept {
		sum += v
		keptParts[i] = strconv.Itoa(v)
	}
	orig := make([]string, len(rolls))
	for i, v := range rolls {
		orig[i] = strconv.Itoa(v)
	}
	dir := "大"
	if !d.keepHigh {
		dir = "小"
	}
	return sum, fmt.Sprintf("取%s%d：[%s]（原始[%s]）", dir, d.keep, strings.Join(keptParts, "+"), strings.Join(orig, ","))
}

func (d *diceNode) maxValue() int {
	if d.keep == 0 {
		return d.count * d.faces
	}
	return d.keep * d.faces
}

func (d *diceNode) hasDice() bool { return true }

// binNode 二元运算节点。
type binNode struct {
	op          byte
	left, right node
}

func (b *binNode) roll(f func(int) int) (int, string) {
	lv, ls := b.left.roll(f)
	rv, rs := b.right.roll(f)
	switch b.op {
	case '+':
		return lv + rv, ls + "+" + rs
	case '-':
		return lv - rv, ls + "-" + rs
	case '*':
		return lv * rv, ls + "*" + rs
	case '/':
		return lv / rv, ls + "/" + rs
	}
	return 0, ""
}

func (b *binNode) maxValue() int {
	switch b.op {
	case '+':
		return b.left.maxValue() + b.right.maxValue()
	case '-':
		return b.left.maxValue() - b.right.maxValue()
	case '*':
		return b.left.maxValue() * b.right.maxValue()
	case '/':
		return b.left.maxValue() / b.right.maxValue()
	}
	return 0
}

func (b *binNode) hasDice() bool {
	return b.left.hasDice() || b.right.hasDice()
}

// exprParser 递归下降解析器。语法：
//
//	expr  := term (('+'|'-') term)*
//	term  := factor (('*'|'/'|'x'|'X'|'×') factor)*
//	factor:= 整数 | (整数? 'd' ('%'|整数) ('k'('h'|'l')? 整数?)?)
//
// k = 取最大（Dice! 风格），kh = 取最大，kl = 取最小；
// 省略保留数量时默认保留 count-1 个（如 4d6k 等价于 4d6kh3）。
type exprParser struct {
	src    string
	pos    int
	limits diceLimits
	groups int
	dice   int
}

// parseDiceExpression 解析骰子表达式；成功时节点已完整消费输入。
func parseDiceExpression(input string, limits diceLimits) (node, error) {
	// 全角乘除号统一为半角，方便中文输入法用户
	input = strings.ReplaceAll(input, "×", "*")
	input = strings.ReplaceAll(input, "÷", "/")
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("表达式为空")
	}
	p := &exprParser{src: input, limits: limits}
	n, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("无法解析表达式「%s」（剩余 %q）", input, p.src[p.pos:])
	}
	return n, nil
}

func (p *exprParser) parseExpr() (node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return left, nil
		}
		c := p.src[p.pos]
		if c != '+' && c != '-' {
			return left, nil
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &binNode{op: c, left: left, right: right}
	}
}

func (p *exprParser) parseTerm() (node, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return left, nil
		}
		c := p.src[p.pos]
		if c != '*' && c != '/' && c != 'x' && c != 'X' && c != '×' {
			return left, nil
		}
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		if c == '/' {
			if rn, ok := right.(numNode); ok && rn == 0 {
				return nil, fmt.Errorf("除数不能为 0")
			}
		}
		left = &binNode{op: c, left: left, right: right}
	}
}

func (p *exprParser) parseFactor() (node, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("表达式不完整")
	}
	c := p.src[p.pos]
	if c >= '0' && c <= '9' {
		count, err := p.readInt()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		// 形如 2d6：数字后紧跟 d/D 视为骰子数量
		if p.pos < len(p.src) && (p.src[p.pos] == 'd' || p.src[p.pos] == 'D') {
			return p.parseDice(count)
		}
		return numNode(count), nil
	}
	if c == 'd' || c == 'D' {
		return p.parseDice(1)
	}
	if c == '(' {
		return nil, fmt.Errorf("暂不支持括号")
	}
	return nil, fmt.Errorf("无法识别的字符 %q", string(c))
}

// parseDice 解析 d 之后的面数与可选的取骰后缀；count 已读入。
func (p *exprParser) parseDice(count int) (node, error) {
	if p.limits.maxDice > 0 && (count < 1 || count > p.limits.maxDice) {
		return nil, fmt.Errorf("每组骰子数量需在 1~%d 之间（当前 %d）", p.limits.maxDice, count)
	}
	p.pos++ // 跳过 d/D
	faces := 0
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == '%' {
		faces = 100
		p.pos++
	} else {
		if p.pos >= len(p.src) || p.src[p.pos] < '0' || p.src[p.pos] > '9' {
			return nil, fmt.Errorf("骰子缺少面数（如 d6、d100、d%%）")
		}
		v, err := p.readInt()
		if err != nil {
			return nil, err
		}
		faces = v
	}
	if p.limits.maxFaces > 0 && (faces < 2 || faces > p.limits.maxFaces) {
		return nil, fmt.Errorf("骰子面数需在 2~%d 之间（当前 %d）", p.limits.maxFaces, faces)
	}
	p.groups++
	if p.limits.maxGroups > 0 && p.groups > p.limits.maxGroups {
		return nil, fmt.Errorf("骰组数量超过上限 %d", p.limits.maxGroups)
	}
	p.dice += count
	if p.limits.maxTotalDice > 0 && p.dice > p.limits.maxTotalDice {
		return nil, fmt.Errorf("骰子总数超过上限 %d", p.limits.maxTotalDice)
	}

	nd := &diceNode{count: count, faces: faces}
	p.skipSpace()
	if p.pos >= len(p.src) || (p.src[p.pos] != 'k' && p.src[p.pos] != 'K') {
		return nd, nil
	}
	if count < 2 {
		return nil, fmt.Errorf("取骰需要至少 2 个骰子")
	}
	p.pos++ // 跳过 k/K
	p.skipSpace()
	if p.pos < len(p.src) && (p.src[p.pos] == 'h' || p.src[p.pos] == 'H') {
		nd.keepHigh = true
		p.pos++
	} else if p.pos < len(p.src) && (p.src[p.pos] == 'l' || p.src[p.pos] == 'L') {
		nd.keepHigh = false
		p.pos++
	} else {
		// 裸 k：Dice! 语义，取最大
		nd.keepHigh = true
	}
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		v, err := p.readInt()
		if err != nil {
			return nil, err
		}
		nd.keep = v
	} else {
		nd.keep = count - 1
	}
	if nd.keep < 1 || nd.keep >= count {
		return nil, fmt.Errorf("取骰数量需在 1~%d 之间", count-1)
	}
	return nd, nil
}

func (p *exprParser) readInt() (int, error) {
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("缺少数字")
	}
	v, err := strconv.Atoi(p.src[start:p.pos])
	if err != nil {
		return 0, fmt.Errorf("数字过大")
	}
	return v, nil
}

func (p *exprParser) skipSpace() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

// splitExprAndReason 从按空白切分后的参数中拆出“表达式 + 掷骰原因”。
// 从最长前缀开始尝试解析（如 "2d6"、"侦查"、"2d6 侦查" 拆为 "2d6" + "侦查"），
// 解析成功即返回，剩余 token 作为原因。
func splitExprAndReason(tokens []string, limits diceLimits) (expr string, reason string, err error) {
	if len(tokens) == 0 {
		return "", "", nil
	}
	for i := len(tokens); i >= 1; i-- {
		cand := strings.Join(tokens[:i], "")
		if _, e := parseDiceExpression(cand, limits); e == nil {
			return cand, strings.Join(tokens[i:], " "), nil
		}
	}
	return "", "", fmt.Errorf("无法识别掷骰表达式，请参考：2d6、2d6+3、d%%、3d6k2")
}

// rollNode 投掷 AST 并返回数值与分解串。
func rollNode(n node, f func(int) int) (int, string) {
	return n.roll(f)
}
