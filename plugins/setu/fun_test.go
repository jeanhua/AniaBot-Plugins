package setu

import (
	"fmt"
	"strings"
	"testing"
)

// TestMessageTemplateFormat 校验所有带占位符的文案池与调用方的实参顺序/类型一致，
// 防止 fmt.Sprintf 打出 %!d(string=…)/%!s(int=…) 这类格式错乱。
func TestMessageTemplateFormat(t *testing.T) {
	cases := []struct {
		name string
		list []string
		args []any
		want []string
	}{
		{"summonLines", summonLines, []any{"绅士", 3}, []string{"绅士", "3"}},
		{"cooldownMessages", cooldownMessages, []any{5}, []string{"5"}},
		{"dailyMessages", dailyMessages, []any{10, 10}, []string{"10/10"}},
		{"emptyMessages", emptyMessages, []any{15, "白丝"}, []string{"15", "白丝"}},
	}
	for _, c := range cases {
		for _, tmpl := range c.list {
			got := fmt.Sprintf(tmpl, c.args...)
			if strings.Contains(got, "%!") {
				t.Errorf("%s 模板 %q 格式化出错：%s", c.name, tmpl, got)
				continue
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("%s 模板 %q 格式化后缺少 %q：%s", c.name, tmpl, w, got)
				}
			}
		}
	}
}
