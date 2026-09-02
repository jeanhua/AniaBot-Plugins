package dicegirl

import (
	"math/rand"
	"strings"
	"testing"
)

func testLimits() diceLimits {
	return diceLimits{maxDice: 100, maxFaces: 1000, maxGroups: 20, maxTotalDice: 500}
}

// fixedRoller 返回固定序列的随机函数：依次返回 n-1, n-2, ...（模拟掷出 1,2,3...）。
// fixedRoller 无论骰面如何都掷出该骰子的最大值（如 d6→6、d%→100）。
func fixedRoller(n int) func(int) int {
	return func(face int) int { return face - 1 }
}

func TestParseBasic(t *testing.T) {
	cases := []struct {
		in      string
		wantVal int
	}{
		{"d6", 6},
		{"1d6", 6},
		{"2d6+3", 6 + 6 + 3},
		{"2d6-1", 6 + 6 - 1},
		{"3d6*10", 6 * 3 * 10},
		{"d%", 100},
		{"1d4+1d6+2", 4 + 6 + 2},
	}
	for _, c := range cases {
		n, err := parseDiceExpression(c.in, testLimits())
		if err != nil {
			t.Fatalf("%s 解析失败: %v", c.in, err)
		}
		got, _ := rollNode(n, fixedRoller(6))
		if got != c.wantVal {
			t.Errorf("%s: 期望 %d, 实际 %d", c.in, c.wantVal, got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{"", "d0", "101d6", "d1001", "abc", "2d6+", "2d6(", "1d6/0"}
	for _, in := range cases {
		if _, err := parseDiceExpression(in, testLimits()); err == nil {
			t.Errorf("%q 应解析失败", in)
		}
	}
}

func TestKeepDice(t *testing.T) {
	// 固定掷出 1,2,3,4,5,6... 序列需要可控源：用序列返回函数模拟掷出 6,5,4,3,2,1。
	seq := []int{6, 5, 4, 3, 2, 1}
	i := 0
	f := func(int) int {
		v := seq[i]
		i++
		return v - 1
	}
	n, err := parseDiceExpression("4d6k3", testLimits())
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	got, detail := rollNode(n, f)
	// 掷出 6,5,4,3，取最大 3 个 = 6+5+4=15
	if got != 15 {
		t.Errorf("4d6k3 期望 15, 实际 %d (%s)", got, detail)
	}
	if detail == "" {
		t.Error("取骰应输出分解")
	}
}

func TestSplitExprAndReason(t *testing.T) {
	lim := testLimits()
	expr, reason, err := splitExprAndReason([]string{"2d6", "侦查"}, lim)
	if err != nil || expr != "2d6" || reason != "侦查" {
		t.Fatalf("拆分失败: expr=%q reason=%q err=%v", expr, reason, err)
	}
	expr, reason, err = splitExprAndReason([]string{"2d6", "+", "3", "挥砍"}, lim)
	if err != nil || expr != "2d6+3" || reason != "挥砍" {
		t.Fatalf("拆分失败: expr=%q reason=%q err=%v", expr, reason, err)
	}
	expr, reason, err = splitExprAndReason([]string{"d100"}, lim)
	if err != nil || expr != "d100" || reason != "" {
		t.Fatalf("拆分失败: expr=%q reason=%q err=%v", expr, reason, err)
	}
}

func TestCocCheck(t *testing.T) {
	cases := []struct {
		roll, rating int
		want         string
	}{
		{1, 60, "大成功"},
		{100, 60, "大失败"},
		{97, 40, "大失败"},
		{97, 60, "失败"},
		{12, 60, "极难成功"},
		{13, 60, "困难成功"},
		{30, 60, "困难成功"},
		{31, 60, "成功"},
		{60, 60, "成功"},
		{61, 60, "失败"},
	}
	for _, c := range cases {
		_, got := cocCheck(c.roll, c.rating)
		if got != c.want {
			t.Errorf("D100=%d/%d: 期望 %s, 实际 %s", c.roll, c.rating, c.want, got)
		}
	}
}

func TestMaxValue(t *testing.T) {
	n, err := parseDiceExpression("1d4+1d6+3", testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if n.maxValue() != 4+6+3 {
		t.Errorf("maxValue 期望 %d, 实际 %d", 4+6+3, n.maxValue())
	}
	keep, err := parseDiceExpression("4d6k3", testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if keep.maxValue() != 18 {
		t.Errorf("4d6k3 maxValue 期望 18, 实际 %d", keep.maxValue())
	}
}

func TestJrrpDeterministic(t *testing.T) {
	a := jrrpValue("qq:123", "2026-09-02")
	b := jrrpValue("qq:123", "2026-09-02")
	c := jrrpValue("qq:456", "2026-09-02")
	if a != b {
		t.Errorf("同用户同日应稳定: %d != %d", a, b)
	}
	if a == c {
		t.Errorf("不同用户同日应大概率不同: %d == %d", a, c)
	}
	if a < 1 || a > 100 {
		t.Errorf("人品值应在 1~100: %d", a)
	}
}

func TestHelpTextAndArg(t *testing.T) {
	if !isHelpArg([]string{"help"}) || !isHelpArg([]string{"帮助"}) || !isHelpArg([]string{"用法"}) {
		t.Error("help/帮助/用法 应识别为帮助请求")
	}
	if isHelpArg(nil) || isHelpArg([]string{"2d6"}) || isHelpArg([]string{"侦查", "70"}) {
		t.Error("普通参数不应识别为帮助请求")
	}
	for _, kw := range []string{"/r", "/ra", "/sc", "/jrrp"} {
		if !strings.Contains(helpText, kw) {
			t.Errorf("帮助文本应包含 %s", kw)
		}
	}
}

func TestRollDeterministic(t *testing.T) {
	r1 := rand.New(rand.NewSource(42))
	r2 := rand.New(rand.NewSource(42))
	n, err := parseDiceExpression("2d6+3", testLimits())
	if err != nil {
		t.Fatal(err)
	}
	v1, d1 := rollNode(n, r1.Intn)
	v2, d2 := rollNode(n, r2.Intn)
	if v1 != v2 || d1 != d2 {
		t.Errorf("相同种子应得到相同结果: %d/%s vs %d/%s", v1, d1, v2, d2)
	}
}
