package music

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// defaultAPIBase GD音乐台开放 API 地址，使用时须注明出处 music.gdstudio.xyz。
const defaultAPIBase = "https://music-api.gdstudio.xyz/api.php"

// validSources API 支持的全部音源；其中 netease/joox/bilibili 为文档标注的稳定音源。
var validSources = map[string]bool{
	"netease": true, "tencent": true, "kuwo": true, "tidal": true, "qobuz": true,
	"joox": true, "bilibili": true, "apple": true, "ytmusic": true, "spotify": true,
}

// validBitrates 音质可选值：740 为 16bit 无损，999 为 24bit 无损。
var validBitrates = map[string]bool{"128": true, "192": true, "320": true, "740": true, "999": true}

// gdMusicClient GD音乐台 API 客户端，复用框架注入的共享 resty 客户端。
type gdMusicClient struct {
	base string
	http *resty.Client
}

// newGDClient 构建 API 客户端；注入客户端为空时退回自建（带超时兜底）。
func newGDClient(base string, injected *resty.Client) *gdMusicClient {
	if strings.TrimSpace(base) == "" {
		base = defaultAPIBase
	}
	rc := injected
	if rc == nil {
		rc = resty.New().SetTimeout(30 * time.Second)
	}
	return &gdMusicClient{base: base, http: rc}
}

// fetch 调一次 API 并返回响应体。接口地址固定为 api.php，全部参数走 query。
func (c *gdMusicClient) fetch(ctx context.Context, params map[string]string) ([]byte, error) {
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(params).Get(c.base)
	if err != nil {
		return nil, fmt.Errorf("请求音乐 API 失败: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("音乐 API 返回 HTTP %d: %s", resp.StatusCode(), truncate(string(resp.Body()), 160))
	}
	return resp.Body(), nil
}

// flexStr 容忍字符串/数字/null 的字段：不同音源会把 id、br、size 等返回成数字。
type flexStr string

func (f flexStr) String() string { return string(f) }

func (f *flexStr) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexStr(s)
		return nil
	}
	*f = flexStr(b)
	return nil
}

// track 搜索结果条目；pic_id/lyric_id 可能为 null 或数字，后续接口按原样回传。
type track struct {
	ID      flexStr  `json:"id"`
	Name    string   `json:"name"`
	Artist  []string `json:"artist"`
	Album   string   `json:"album"`
	PicID   flexStr  `json:"pic_id"`
	LyricID flexStr  `json:"lyric_id"`
	Source  string   `json:"source"`
}

// artistLine 歌手展示串，多歌手用 / 连接。
func (t *track) artistLine() string {
	return strings.Join(t.Artist, "/")
}

// searchSongs 关键字搜索，支持歌名、歌手、专辑名；page 从 1 起。
func (c *gdMusicClient) searchSongs(ctx context.Context, source, keyword string, count, page int) ([]*track, error) {
	if page < 1 {
		page = 1
	}
	body, err := c.fetch(ctx, map[string]string{
		"types":  "search",
		"source": source,
		"name":   keyword,
		"count":  strconv.Itoa(count),
		"pages":  strconv.Itoa(page),
	})
	if err != nil {
		return nil, err
	}
	var tracks []*track
	if jerr := json.Unmarshal(body, &tracks); jerr != nil {
		// 正常返回是数组；解析失败大概率是接口返回了错误对象，尽量把错误信息带出来。
		var obj map[string]any
		if oerr := json.Unmarshal(body, &obj); oerr == nil {
			for _, key := range []string{"message", "error", "msg"} {
				if s, _ := obj[key].(string); s != "" {
					return nil, fmt.Errorf("音乐 API 错误: %s", s)
				}
			}
		}
		return nil, fmt.Errorf("解析搜索结果失败: %w", jerr)
	}
	return tracks, nil
}

// songURLResult 播放直链结果；br 为实际音质，size 单位 KB。
type songURLResult struct {
	URL  flexStr `json:"url"`
	BR   flexStr `json:"br"`
	Size flexStr `json:"size"`
}

// songURL 按 track id 获取播放直链。
func (c *gdMusicClient) songURL(ctx context.Context, source, id, br string) (*songURLResult, error) {
	body, err := c.fetch(ctx, map[string]string{
		"types": "url", "source": source, "id": id, "br": br,
	})
	if err != nil {
		return nil, err
	}
	var out songURLResult
	if jerr := json.Unmarshal(body, &out); jerr != nil {
		return nil, fmt.Errorf("解析播放链接失败: %w", jerr)
	}
	if strings.TrimSpace(out.URL.String()) == "" {
		return nil, fmt.Errorf("音源未返回播放链接（歌曲可能下架或音源暂不可用）")
	}
	return &out, nil
}

// coverURL 按 pic_id 获取封面图直链。
func (c *gdMusicClient) coverURL(ctx context.Context, source, picID string) (string, error) {
	body, err := c.fetch(ctx, map[string]string{
		"types": "pic", "source": source, "id": picID, "size": "300",
	})
	if err != nil {
		return "", err
	}
	var out struct {
		URL flexStr `json:"url"`
	}
	if jerr := json.Unmarshal(body, &out); jerr != nil {
		return "", fmt.Errorf("解析封面失败: %w", jerr)
	}
	return out.URL.String(), nil
}

// download 下载音频文件到内存，返回 (数据, Content-Type, 错误)。
func (c *gdMusicClient) download(ctx context.Context, url string) ([]byte, string, error) {
	resp, err := c.http.R().SetContext(ctx).Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("请求音频失败: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, "", fmt.Errorf("下载音频失败：HTTP %d", resp.StatusCode())
	}
	body := resp.Body()
	if len(body) == 0 {
		return nil, "", fmt.Errorf("音频内容为空")
	}
	return body, resp.Header().Get("Content-Type"), nil
}

// lyricResult 歌词：lyric 为原语种 LRC，tlyric 为中文翻译 LRC（可能为空）。
type lyricResult struct {
	Lyric  string `json:"lyric"`
	TLyric string `json:"tlyric"`
}

// lyric 按 lyric_id 获取歌词，lyric_id 缺失时一般与曲目 id 相同。
func (c *gdMusicClient) lyric(ctx context.Context, source, id string) (*lyricResult, error) {
	body, err := c.fetch(ctx, map[string]string{
		"types": "lyric", "source": source, "id": id,
	})
	if err != nil {
		return nil, err
	}
	var out lyricResult
	if jerr := json.Unmarshal(body, &out); jerr != nil {
		return nil, fmt.Errorf("解析歌词失败: %w", jerr)
	}
	return &out, nil
}

// truncate 按字符截断长文本，避免日志/提示里塞进整段响应。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
