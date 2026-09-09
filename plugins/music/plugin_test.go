package music

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseMusicArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want musicAction
	}{
		{"无参数给帮助", nil, musicAction{kind: "help"}},
		{"英文help", []string{"help"}, musicAction{kind: "help"}},
		{"中文帮助", []string{"帮助"}, musicAction{kind: "help"}},
		{"多词关键词搜索", []string{"周杰伦", "晴天"}, musicAction{kind: "search", keyword: "周杰伦 晴天"}},
		{"单词关键词搜索", []string{"晴天"}, musicAction{kind: "search", keyword: "晴天"}},
		{"纯序号点播", []string{"3"}, musicAction{kind: "pick", index: 3}},
		{"选序号", []string{"选", "2"}, musicAction{kind: "pick", index: 2}},
		{"播放序号", []string{"播放", "12"}, musicAction{kind: "pick", index: 12}},
		{"选加关键词当搜索", []string{"选", "晴天"}, musicAction{kind: "search", keyword: "晴天"}},
		{"选无参数给帮助", []string{"选"}, musicAction{kind: "help"}},
		{"歌词带序号", []string{"歌词", "5"}, musicAction{kind: "lyric", index: 5}},
		{"词带序号", []string{"词", "1"}, musicAction{kind: "lyric", index: 1}},
		{"歌词缺序号", []string{"歌词"}, musicAction{kind: "lyric"}},
		{"英文lyric", []string{"lyric", "1"}, musicAction{kind: "lyric", index: 1}},
		{"下一页", []string{"下一页"}, musicAction{kind: "next"}},
		{"下页别名", []string{"下页"}, musicAction{kind: "next"}},
		{"上一页", []string{"上一页"}, musicAction{kind: "prev"}},
		{"跳页", []string{"页", "3"}, musicAction{kind: "page", index: 3}},
		{"英文page", []string{"page", "2"}, musicAction{kind: "page", index: 2}},
		{"跳页缺页码", []string{"page"}, musicAction{kind: "page"}},
		{"序号非法当关键词", []string{"第3首"}, musicAction{kind: "search", keyword: "第3首"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMusicArgs(tc.args)
			if got != tc.want {
				t.Fatalf("parseMusicArgs(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseIndex(t *testing.T) {
	if _, ok := parseIndex([]string{"3"}); !ok {
		t.Fatal("单正整数应合法")
	}
	if _, ok := parseIndex([]string{"0"}); ok {
		t.Fatal("0 应非法")
	}
	if _, ok := parseIndex([]string{"a"}); ok {
		t.Fatal("非数字应非法")
	}
	if _, ok := parseIndex([]string{"1", "2"}); ok {
		t.Fatal("多个参数应非法")
	}
	if _, ok := parseIndex(nil); ok {
		t.Fatal("空参数应非法")
	}
}

func TestStripTimestamps(t *testing.T) {
	in := "[ti:晴天]\n[ar:周杰伦]\n[00:04.31]故事的小黄花\n[00:12.55][00:50.13]从出生那年就飘着\n\n[01:02:03] 童年 enjoyed\n"
	want := "故事的小黄花\n从出生那年就飘着\n童年 enjoyed"
	if got := stripTimestamps(in); got != want {
		t.Fatalf("stripTimestamps() = %q, want %q", got, want)
	}
}

func TestFlexStrUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want flexStr
	}{
		{`"12345"`, flexStr("12345")},
		{`12345`, flexStr("12345")},
		{`3.14`, flexStr("3.14")},
		{`"abc"`, flexStr("abc")},
		{`null`, flexStr("")},
	}
	for _, tc := range cases {
		var f flexStr
		if err := json.Unmarshal([]byte(tc.in), &f); err != nil {
			t.Fatalf("Unmarshal(%s) 出错: %v", tc.in, err)
		}
		if f != tc.want {
			t.Fatalf("Unmarshal(%s) = %q, want %q", tc.in, f, tc.want)
		}
	}
}

func TestUnmarshalSearch(t *testing.T) {
	data := `[
		{"id":186016,"name":"晴天","artist":["周杰伦"],"album":"叶惠美","pic_id":"109951169160045389","lyric_id":186016,"source":"netease"},
		{"id":"8812","name":"Lemon","artist":["米津玄師"],"album":null,"pic_id":null,"lyric_id":null,"source":"joox"}
	]`
	var tracks []*track
	if err := json.Unmarshal([]byte(data), &tracks); err != nil {
		t.Fatalf("解析搜索结果出错: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("应解析出 2 条，实际 %d 条", len(tracks))
	}
	first := tracks[0]
	if first.ID != flexStr("186016") || first.Name != "晴天" || first.Album != "叶惠美" {
		t.Fatalf("第一条解析错误: %+v", first)
	}
	if first.artistLine() != "周杰伦" {
		t.Fatalf("artistLine() = %q, want 周杰伦", first.artistLine())
	}
	second := tracks[1]
	if second.ID != flexStr("8812") || second.Album != "" || second.PicID != flexStr("") {
		t.Fatalf("null 字段应落为空串: %+v", second)
	}
	if second.artistLine() != "米津玄師" {
		t.Fatalf("artistLine() = %q, want 米津玄師", second.artistLine())
	}
}

func TestUnmarshalSongURL(t *testing.T) {
	data := `{"url":"https://example.com/a.mp3","br":320,"size":"10240"}`
	var out songURLResult
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		t.Fatalf("解析播放链接出错: %v", err)
	}
	if out.URL != flexStr("https://example.com/a.mp3") || out.BR != flexStr("320") || out.Size != flexStr("10240") {
		t.Fatalf("播放链接解析错误: %+v", out)
	}
}

func TestTrackLine(t *testing.T) {
	cases := []struct {
		name  string
		track track
		want  string
	}{
		{"完整信息", track{Name: "晴天", Artist: []string{"周杰伦"}, Album: "叶惠美"}, "晴天 - 周杰伦 [叶惠美]"},
		{"无专辑", track{Name: "晴天", Artist: []string{"周杰伦"}}, "晴天 - 周杰伦"},
		{"无歌手", track{Name: "晴天", Album: "叶惠美"}, "晴天 [叶惠美]"},
		{"多歌手", track{Name: "屋顶", Artist: []string{"周杰伦", "温岚"}}, "屋顶 - 周杰伦/温岚"},
		{"仅歌名", track{Name: "晴天"}, "晴天"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trackLine(&tc.track); got != tc.want {
				t.Fatalf("trackLine() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"正常名字", "晴天 - 周杰伦", "晴天 - 周杰伦"},
		{"非法字符剔除", `a/b\c:d*e?f"g<h>i|j`, "abcdefghij"},
		{"空名字兜底", "  ", "未命名"},
		{"控制字符剔除", "歌\n名\t!", "歌名!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeFilename(tc.in); got != tc.want {
				t.Fatalf("sanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	if r := []rune(sanitizeFilename(strings.Repeat("长", 200))); len(r) != 80 {
		t.Fatalf("超长文件名应截断到 80 字符，实际 %d", len(r))
	}
}

func TestAudioExt(t *testing.T) {
	cases := []struct {
		ct string
		br string
		want string
	}{
		{"audio/flac", "999", ".flac"},
		{"", "740", ".flac"},
		{"audio/mpeg", "320", ".mp3"},
		{"video/mp4", "320", ".m4a"},
		{"application/octet-stream", "128", ".mp3"},
		{"", "", ".mp3"},
	}
	for _, tc := range cases {
		if got := audioExt(tc.ct, tc.br); got != tc.want {
			t.Fatalf("audioExt(%q, %q) = %q, want %q", tc.ct, tc.br, got, tc.want)
		}
	}
}

func TestParseSizeBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"11161644", 11161644},
		{"0", 0},
		{"", 0},
		{"abc", 0},
		{"-5", 0},
	}
	for _, tc := range cases {
		if got := parseSizeBytes(tc.in); got != tc.want {
			t.Fatalf("parseSizeBytes(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestBuildAudioName(t *testing.T) {
	tr := &track{Name: "晴/天", Artist: []string{"周杰伦"}, Album: "叶惠美"}
	if got := buildAudioName(tr, "audio/mpeg", "320"); got != "晴天 - 周杰伦.mp3" {
		t.Fatalf("buildAudioName() = %q", got)
	}
	if got := buildAudioName(&track{Name: "Lemon"}, "", "999"); got != "Lemon.flac" {
		t.Fatalf("buildAudioName() = %q", got)
	}
}

func TestRateLimiter(t *testing.T) {
	r := newRateLimiter(time.Minute, 2)
	if ok, _ := r.allow(); !ok {
		t.Fatal("第 1 次应放行")
	}
	if ok, _ := r.allow(); !ok {
		t.Fatal("第 2 次应放行")
	}
	if ok, wait := r.allow(); ok {
		t.Fatal("第 3 次应拒绝")
	} else if wait <= 0 || wait > time.Minute {
		t.Fatalf("等待时长应在窗口内: %v", wait)
	}
}

func TestHelpText(t *testing.T) {
	s := helpText(10)
	for _, want := range []string{"/点歌 关键词", "/点歌 下一页", "/点歌 序号", "/点歌 歌词 序号", creditLine} {
		if !strings.Contains(s, want) {
			t.Fatalf("帮助文本缺少 %q:\n%s", want, s)
		}
	}
}

// TestKeyboardRows 列表键盘：序号点播按钮每行 5 个（回调数据 pick:<序号>，
// 带插件前缀），末行翻页按钮按页码出现。
func TestKeyboardRows(t *testing.T) {
	p := NewPlugin()
	sess := &searchSession{keyword: "test", page: 2, tracks: make([]*track, 12)}
	for i := range sess.tracks {
		sess.tracks[i] = &track{Name: fmt.Sprintf("t%d", i+1)}
	}
	rows := p.keyboardRows(sess)
	// 12 首 → 3 行序号（5+5+2）+ 1 行翻页
	if len(rows) != 4 {
		t.Fatalf("行数 = %d, want 4: %+v", len(rows), rows)
	}
	if len(rows[0]) != 5 || len(rows[2]) != 2 {
		t.Fatalf("序号按钮布局不符: %+v", rows)
	}
	first := rows[0][0]
	if first.Text != "1" || first.Data != "音乐点歌:pick:1" {
		t.Fatalf("首个点播按钮不符: %+v", first)
	}
	last := rows[2][1]
	if last.Text != "12" || last.Data != "音乐点歌:pick:12" {
		t.Fatalf("末个点播按钮不符: %+v", last)
	}
	// 第 2 页：上一页 + 下一页
	pg := rows[3]
	if len(pg) != 2 || pg[0].Data != "音乐点歌:pg:1" || pg[1].Data != "音乐点歌:pg:3" {
		t.Fatalf("翻页按钮不符: %+v", pg)
	}

	// 第 1 页短列表：只有序号行 + 下一页
	sess1 := &searchSession{keyword: "test", page: 1, tracks: []*track{{Name: "a"}, {Name: "b"}}}
	rows1 := p.keyboardRows(sess1)
	if len(rows1) != 2 || len(rows1[0]) != 2 || len(rows1[1]) != 1 || rows1[1][0].Data != "音乐点歌:pg:2" {
		t.Fatalf("第 1 页键盘不符: %+v", rows1)
	}
}

// TestParsePickPayload 点播按钮载荷解析。
func TestParsePickPayload(t *testing.T) {
	if n, ok := parsePickPayload("pick:3"); !ok || n != 3 {
		t.Fatalf("pick:3 → %d %v, want 3 true", n, ok)
	}
	if _, ok := parsePickPayload("pg:3"); ok {
		t.Fatal("翻页载荷不应解析为点播")
	}
	if _, ok := parsePickPayload("pick:0"); ok {
		t.Fatal("序号 0 不合法")
	}
	if _, ok := parsePickPayload("pick:x"); ok {
		t.Fatal("非数字不合法")
	}
}
