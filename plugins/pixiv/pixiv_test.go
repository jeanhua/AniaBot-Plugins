package pixiv

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseKeywordAndPage(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		keyword string
		page    int
	}{
		{"空参数", nil, "", 1},
		{"纯关键词", []string{"少女前线"}, "少女前线", 1},
		{"带页码", []string{"少女前线", "2"}, "少女前线", 2},
		{"多词关键词+页码", []string{"少女前线", "同人", "3"}, "少女前线 同人", 3},
		{"页码超上限被忽略当关键词", []string{"少女前线", "999"}, "少女前线 999", 1},
		{"页码非法被忽略", []string{"少女前线", "abc"}, "少女前线 abc", 1},
		{"单独数字是关键词不是页码", []string{"123"}, "123", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kw, page := parseKeywordAndPage(c.args)
			if kw != c.keyword || page != c.page {
				t.Fatalf("parseKeywordAndPage(%v) = (%q, %d), want (%q, %d)", c.args, kw, page, c.keyword, c.page)
			}
		})
	}
}

func TestParseIDAndPage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		id   int64
		page int
	}{
		{"空参数", nil, 0, 0},
		{"非数字", []string{"abc"}, 0, 0},
		{"负数", []string{"-1"}, 0, 0},
		{"仅ID", []string{"12345"}, 12345, 1},
		{"ID+页码", []string{"12345", "3"}, 12345, 3},
		{"页码非法回退1", []string{"12345", "x"}, 12345, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, page := parseIDAndPage(c.args)
			if id != c.id || page != c.page {
				t.Fatalf("parseIDAndPage(%v) = (%d, %d), want (%d, %d)", c.args, id, page, c.id, c.page)
			}
		})
	}
}

func TestParseRankArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		mode    string
		wantR18 bool
	}{
		{"默认日榜", nil, "day", false},
		{"周榜", []string{"周"}, "week", false},
		{"月榜", []string{"month"}, "month", false},
		{"日榜+r18", []string{"日", "r18"}, "day", true},
		{"R-18 写法", []string{"week", "R-18"}, "week", true},
		{"非法词忽略", []string{"随便"}, "day", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mode, r18 := parseRankArgs(c.args)
			if mode != c.mode || r18 != c.wantR18 {
				t.Fatalf("parseRankArgs(%v) = (%q, %v), want (%q, %v)", c.args, mode, r18, c.mode, c.wantR18)
			}
		})
	}
}

func TestResolveRankMode(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		mode        string
		wantR18     bool
		apiMode     string
		deny        bool
	}{
		{"safe日榜", "safe", "day", false, "day", false},
		{"safe要R18被拒", "safe", "day", true, "", true},
		{"r18分级自动升级", "r18", "day", false, "day_r18", false},
		{"mixed周R18", "mixed", "week", true, "week_r18", false},
		{"月榜无R18", "mixed", "month", true, "", true},
		{"r18分级月榜不升级", "r18", "month", false, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			apiMode, _, denyMsg := resolveRankMode(c.contentType, c.mode, c.wantR18)
			if apiMode != c.apiMode || (denyMsg != "") != c.deny {
				t.Fatalf("resolveRankMode(%q,%q,%v) = (%q,%q), want apiMode=%q deny=%v",
					c.contentType, c.mode, c.wantR18, apiMode, denyMsg, c.apiMode, c.deny)
			}
		})
	}
}

func TestMatchContentType(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		xRestrict   int
		want        bool
	}{
		{"safe全年龄", "safe", 0, true},
		{"safe R18", "safe", 2, false},
		{"safe R18G", "safe", 1, false},
		{"mixed全过", "mixed", 2, true},
		{"r18分级收R18", "r18", 2, true},
		{"r18分级拒全年龄", "r18", 0, false},
		{"未知分级按safe", "whatever", 2, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchContentType(c.contentType, c.xRestrict); got != c.want {
				t.Fatalf("matchContentType(%q, %d) = %v, want %v", c.contentType, c.xRestrict, got, c.want)
			}
		})
	}
}

func TestNextOffset(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want int
	}{
		{"空", "", -1},
		{"无offset", "https://app-api.pixiv.net/v1/search/illust?word=a", -1},
		{"带offset", "https://app-api.pixiv.net/v1/search/illust?word=a&offset=30", 30},
		{"offset为0视为无下一页", "https://app-api.pixiv.net/v1/ranking/illust?mode=day&offset=0", -1},
		{"offset非法", "https://app-api.pixiv.net/v1/x?offset=abc", -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nextOffset(c.url); got != c.want {
				t.Fatalf("nextOffset(%q) = %d, want %d", c.url, got, c.want)
			}
		})
	}
}

func TestClientHash(t *testing.T) {
	// md5("test") 的公知向量，验证拼接与 hex 编码正确。
	if got := clientHashWith("", "test"); got != "098f6bcd4621d373cade4e832627b4f6" {
		t.Fatalf("clientHashWith = %q, want 已知 md5 向量", got)
	}
	if got := clientHash("2026-01-01T00:00:00.000000"); len(got) != 32 {
		t.Fatalf("clientHash 长度 = %d, want 32", len(got))
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("少女前线同人图", 20); got != "少女前线同人图" {
		t.Fatalf("truncate 未超长不应截断, got %q", got)
	}
	if got := truncate("少女前线同人图", 4); got != "少女前线…" {
		t.Fatalf("truncate 超长应截断加省略号, got %q", got)
	}
	if got := truncate("  x  ", 10); got != "x" {
		t.Fatalf("truncate 应去空白, got %q", got)
	}
	if got := truncate("abc", 0); got != "" {
		t.Fatalf("truncate n<1 应返回空, got %q", got)
	}
}

func TestFormatCount(t *testing.T) {
	cases := map[int]string{
		0:     "0",
		9999:  "9999",
		10000: "1.0w",
		23456: "2.3w",
	}
	for in, want := range cases {
		if got := formatCount(in); got != want {
			t.Fatalf("formatCount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderIllustListMarksR18(t *testing.T) {
	visible := true
	items := []illust{
		{ID: 1, Title: "普通图", User: pixivUser{Name: "画师A"}, TotalBookmarks: 23456, XRestrict: 0, Visible: &visible},
		{ID: 2, Title: "涩涩图", User: pixivUser{Name: "画师B"}, TotalBookmarks: 100, XRestrict: 2, Visible: &visible},
	}
	out := renderIllustList("📊 测试", items, "提示尾巴")
	if !strings.Contains(out, "ID:1") || !strings.Contains(out, "ID:2") {
		t.Fatalf("列表应包含作品 ID:\n%s", out)
	}
	if !strings.Contains(out, "2.3w") {
		t.Fatalf("收藏数应缩写为 2.3w:\n%s", out)
	}
	if !strings.Contains(out, "🔞") {
		t.Fatalf("R18 作品应带 🔞 标记:\n%s", out)
	}
	if !strings.Contains(out, "提示尾巴") {
		t.Fatalf("列表应包含提示尾巴:\n%s", out)
	}
}

func TestIllustVisible(t *testing.T) {
	falsy := false
	if (&illust{}).visible() != true {
		t.Fatal("visible 字段缺失应视为可见（排行榜不带该字段）")
	}
	if (&illust{Visible: &falsy}).visible() != false {
		t.Fatal("visible=false 应视为不可见")
	}
}

func TestPageCandidates(t *testing.T) {
	cases := []struct {
		name string
		it   illust
		want string // 首个候选应为原图地址
	}{
		{
			"单页带原图",
			illust{MetaSinglePage: struct {
				OriginalImageURL string `json:"original_image_url"`
			}{OriginalImageURL: "https://i.pximg.net/img-original/1.jpg"}, ImageURLs: imageURLs{Large: "https://i.pximg.net/c/1200/1.jpg"}},
			"https://i.pximg.net/img-original/1.jpg",
		},
		{
			"多页取对应页",
			illust{PageCount: 2, MetaPages: []illustPage{
				{ImageURLs: imageURLs{Original: "https://i.pximg.net/p0.jpg", Large: "https://i.pximg.net/p0_l.jpg"}},
				{ImageURLs: imageURLs{Original: "https://i.pximg.net/p1.jpg", Large: "https://i.pximg.net/p1_l.jpg"}},
			}},
			"https://i.pximg.net/p1.jpg", // page 下标 1
		},
		{
			"排行数据无原图回退large",
			illust{ImageURLs: imageURLs{Large: "https://i.pximg.net/c/1200/r.jpg", Medium: "https://i.pximg.net/c/600/r.jpg"}},
			"https://i.pximg.net/c/1200/r.jpg",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cands := c.it.pageCandidates(1)
			if len(cands) == 0 || cands[0] != c.want {
				t.Fatalf("pageCandidates 首候选 = %v, want 首个为 %q", cands, c.want)
			}
		})
	}
}

func TestValidateProxy(t *testing.T) {
	for _, ok := range []string{"http://127.0.0.1:7890", "https://p.example.com", "socks5://127.0.0.1:1080", "socks5h://a.b"} {
		if err := validateProxy(ok); err != nil {
			t.Fatalf("validateProxy(%q) 不应报错: %v", ok, err)
		}
	}
	for _, bad := range []string{"ftp://x", "127.0.0.1:7890", "://"} {
		if err := validateProxy(bad); err == nil {
			t.Fatalf("validateProxy(%q) 应报错", bad)
		}
	}
}

func TestParseAPIErrorMessage(t *testing.T) {
	body := []byte(`{"error":{"message":"Rate Limit","reason":"","user_message":"-error-","validation_errors":{}}}`)
	if got := parseAPIErrorMessage(body, "fb"); got != "-error-" {
		t.Fatalf("应优先 user_message, got %q", got)
	}
	if got := parseAPIErrorMessage([]byte(`not-json`), "fb"); got != "fb" {
		t.Fatalf("解析失败应返回 fallback, got %q", got)
	}
	if got := parseAPIErrorMessage([]byte(`{"error":{"reason":"OAuth error"}}`), "fb"); got != "OAuth error" {
		t.Fatalf("应取 reason, got %q", got)
	}
}

func TestFlexString(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"数字 id（作品列表返回这种）", `{"id":12345}`, "12345"},
		{"字符串 id（登录响应返回这种）", `{"id":"678"}`, "678"},
		{"null", `{"id":null}`, ""},
		{"字段缺失", `{}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var u struct {
				ID flexString `json:"id"`
			}
			if err := json.Unmarshal([]byte(c.json), &u); err != nil {
				t.Fatalf("解析 %s 不应报错: %v", c.json, err)
			}
			if string(u.ID) != c.want {
				t.Fatalf("解析 %s = %q, want %q", c.json, u.ID, c.want)
			}
		})
	}
}

func TestPixivUserIDNumberAndString(t *testing.T) {
	// 复现线上问题：作品列表里 user.id 是数字，pixivUser 必须能吃下。
	var resp struct {
		Illusts []illust `json:"illusts"`
	}
	body := `{"illusts":[{"id":100,"title":"t","user":{"id":9876543210,"name":"画师"}}]}`
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("数字 user.id 解析失败: %v", err)
	}
	if len(resp.Illusts) != 1 || resp.Illusts[0].User.ID != "9876543210" {
		t.Fatalf("user.id 解析结果不对: %+v", resp.Illusts)
	}
}
