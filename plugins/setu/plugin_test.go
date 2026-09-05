package setu

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jeanhua/AniaBot/common/model/message"
)

func TestMatchAllowlist(t *testing.T) {
	group := message.FromString("qq:123456")
	cases := []struct {
		name     string
		patterns []string
		qid      message.QID
		want     bool
	}{
		{"精确裸号", []string{"123456"}, group, true},
		{"精确全ID", []string{"qq:123456"}, group, true},
		{"不相关", []string{"654321"}, group, false},
		{"空名单拒绝", nil, group, false},
		{"空字符串忽略", []string{"", "  "}, group, false},
		{"前缀正则", []string{`^qq:123.*`}, group, true},
		{"裸号正则", []string{`^123`}, group, true},
		{"全放行", []string{".*"}, group, true},
		{"无效正则不炸", []string{"[["}, group, false},
		{"多条之一命中", []string{"999", `123456$`}, group, true},
		{"飞书ID精确", []string{"fs:oc_abc"}, message.FromString("fs:oc_abc"), true},
		{"TG负号ID", []string{"tg:-100123"}, message.FromString("tg:-100123"), true},
	}
	for _, c := range cases {
		if got := matchAllowlist(c.patterns, c.qid); got != c.want {
			t.Errorf("%s：patterns=%v qid=%s，期望 %v 实际 %v", c.name, c.patterns, c.qid, c.want, got)
		}
	}
}

func TestParseCountToken(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"3", 3, true},
		{"1", 1, true},
		{"3连", 3, true},
		{"2张", 2, true},
		{"x3", 3, true},
		{"X2", 2, true},
		{"×3", 3, true},
		{"10", 3, true}, // 超上限截断（maxCount=3）
		{"0", 0, false},
		{"白丝", 0, false},
		{"", 0, false},
		{"3连发", 0, false},
	}
	for _, c := range cases {
		got, ok := parseCountToken(c.in, 3)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseCountToken(%q)=(%d,%v)，期望 (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseSetuArgs(t *testing.T) {
	r := parseSetuArgs(nil, 3)
	if r.count != 1 || len(r.tags) != 0 || r.isHelp || r.isStatus || r.isRefresh {
		t.Fatalf("空参数解析错误：%+v", r)
	}
	r = parseSetuArgs([]string{"help"}, 3)
	if !r.isHelp {
		t.Fatalf("help 未识别：%+v", r)
	}
	r = parseSetuArgs([]string{"状态"}, 3)
	if !r.isStatus {
		t.Fatalf("状态 未识别：%+v", r)
	}
	r = parseSetuArgs([]string{"refresh"}, 3)
	if !r.isRefresh {
		t.Fatalf("refresh 未识别：%+v", r)
	}
	r = parseSetuArgs([]string{"3", "白丝", "萝莉"}, 3)
	if r.count != 3 || len(r.tags) != 2 || r.tags[0] != "白丝" {
		t.Fatalf("数量+tag 解析错误：%+v", r)
	}
	r = parseSetuArgs([]string{"白丝", "r18"}, 3)
	if r.count != 1 || !r.wantR18 || len(r.tags) != 1 || r.tags[0] != "白丝" {
		t.Fatalf("r18 意图解析错误：%+v", r)
	}
	r = parseSetuArgs([]string{"2连", "safe"}, 3)
	if r.count != 2 || !r.wantSafe || len(r.tags) != 0 {
		t.Fatalf("safe 意图解析错误：%+v", r)
	}
}

func TestResolveR18(t *testing.T) {
	if got, sealed := resolveR18("0", setuRequest{wantR18: true}); !sealed || got != "" {
		t.Errorf("全年龄模式要 R18 应拒绝，实际 (%q,%v)", got, sealed)
	}
	if got, sealed := resolveR18("0", setuRequest{}); sealed || got != "0" {
		t.Errorf("全年龄默认应为 0，实际 (%q,%v)", got, sealed)
	}
	if got, sealed := resolveR18("1", setuRequest{wantSafe: true}); sealed || got != "1" {
		t.Errorf("R18 模式应恒为 1，实际 (%q,%v)", got, sealed)
	}
	if got, _ := resolveR18("2", setuRequest{wantR18: true}); got != "1" {
		t.Errorf("混合模式+r18 应为 1，实际 %q", got)
	}
	if got, _ := resolveR18("2", setuRequest{wantSafe: true}); got != "0" {
		t.Errorf("混合模式+safe 应为 0，实际 %q", got)
	}
	if got, _ := resolveR18("2", setuRequest{}); got != "2" {
		t.Errorf("混合模式默认应为 2，实际 %q", got)
	}
}

func TestMatchTagsAndR18(t *testing.T) {
	m := &PixivMeta{
		Title:  "测试标题",
		Author: "画师A",
		R18:    true,
		Tags:   []string{"R-18", "白丝", "アズールレーン"},
	}
	if !matchR18(m, "1") || matchR18(m, "0") || !matchR18(m, "2") {
		t.Error("R18 过滤错误")
	}
	for _, want := range [][]string{{"白丝"}, {"白丝", "R-18"}, {"アズール"}, {"画师"}, nil} {
		if !matchTags(m, want) {
			t.Errorf("tags %v 应命中", want)
		}
	}
	if matchTags(m, []string{"不存在的tag"}) {
		t.Error("冷门 tag 不应命中")
	}
	if matchTags(m, []string{"白丝", "不存在的tag"}) {
		t.Error("AND 关系：缺一不可")
	}
}

func TestRewriteImageHost(t *testing.T) {
	raw := "https://i.pixiv.re/img-master/img/2022/07/26/06/02/29/100001853_p0_master1200.jpg"
	if got := rewriteImageHost(raw, ""); got != raw {
		t.Errorf("空代理应原样返回，实际 %s", got)
	}
	if got := rewriteImageHost(raw, "i.pixiv.cat"); !strings.HasPrefix(got, "https://i.pixiv.cat/") {
		t.Errorf("代理替换失败，实际 %s", got)
	}
	if got := rewriteImageHost("not a url", "i.pixiv.cat"); got != "not a url" {
		t.Errorf("非法 URL 应原样返回，实际 %s", got)
	}
}

func TestPixivMetaUnmarshal(t *testing.T) {
	raw := `{"pid":100001853,"p":0,"uid":5271609,"title":"标题","author":"画师","r18":true,"width":100,"height":100,"tags":["R-18","白丝"],"ext":"jpg","aiType":0,"uploadDate":1658782949000,"urls":{"regular":"https://i.pixiv.re/xxx.jpg"},"url":"/xxx.jpg"}`
	var m PixivMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("详情解析失败：%v", err)
	}
	if m.Pid != 100001853 || !m.R18 || m.ImageURL() == "" {
		t.Fatalf("详情字段错误：%+v", m)
	}
	if m.ArtworkURL() != "https://www.pixiv.net/artworks/100001853" {
		t.Errorf("作品链接错误：%s", m.ArtworkURL())
	}
}

func TestCaptionLine(t *testing.T) {
	m := &PixivMeta{Pid: 1, Title: "T", Author: "A", R18: true, Tags: []string{"a", "b"}}
	got := captionLine(1, 1, m)
	for _, want := range []string{"《T》", "A", "🔞", "artworks/1", "a b"} {
		if !strings.Contains(got, want) {
			t.Errorf("配文缺少 %q，实际：%s", want, got)
		}
	}
}
