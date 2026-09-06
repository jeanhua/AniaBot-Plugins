package pixiv

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// flexString 兼容 Pixiv 接口对同一字段时而返回数字、时而返回字符串的不一致
// （典型：user.id 在登录响应里是字符串，在作品列表里是数字）。
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	// 数字字面量按原文保存（id 均为十进制，无需再加工）。
	*s = flexString(b)
	return nil
}

// Pixiv App API 数据模型：只解析插件用得到的字段，其余忽略。

// pixivUser 作品作者/登录用户信息。
type pixivUser struct {
	ID      flexString `json:"id"`
	Name    string     `json:"name"`
	Account string     `json:"account"`
}

// imageURLs 同一张图的不同尺寸地址（original 只在详情/搜索等完整数据里出现，
// 排行榜等列表数据常缺失）。
type imageURLs struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
}

// illustTag 作品标签。
type illustTag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

// illustSeries 连载系列标题。
type illustSeries struct {
	Title string `json:"title"`
}

// illustPage 多页作品中的单页地址集合。
type illustPage struct {
	ImageURLs imageURLs `json:"image_urls"`
}

// illust Pixiv 插画/漫画/动图作品。
type illust struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Type           string        `json:"type"` // illust / manga / ugoira
	ImageURLs      imageURLs     `json:"image_urls"`
	User           pixivUser     `json:"user"`
	Tags           []illustTag   `json:"tags"`
	CreateDate     string        `json:"create_date"`
	PageCount      int           `json:"page_count"`
	XRestrict      int           `json:"x_restrict"` // 0=全年龄 1=R-18G 2=R-18
	Series         *illustSeries `json:"series"`
	MetaSinglePage struct {
		OriginalImageURL string `json:"original_image_url"`
	} `json:"meta_single_page"`
	MetaPages      []illustPage `json:"meta_pages"`
	TotalView      int          `json:"total_view"`
	TotalBookmarks int          `json:"total_bookmarks"`
	Visible        *bool        `json:"visible"` // 指针：接口不带该字段时视为可见（排行榜不返回 visible）
}

// visible 作品是否可见（已删除/不可见作品为 false）。
func (i *illust) visible() bool { return i.Visible == nil || *i.Visible }

// isR18 是否 R18（含 R-18G）。
func (i *illust) isR18() bool { return i.XRestrict != 0 }

// webURL 作品页链接。
func (i *illust) webURL() string { return fmt.Sprintf("https://www.pixiv.net/artworks/%d", i.ID) }

// tagNames 取前 max 个标签名，max<=0 表示全部。
func (i *illust) tagNames(max int) []string {
	out := make([]string, 0, len(i.Tags))
	for _, t := range i.Tags {
		if t.Name == "" {
			continue
		}
		out = append(out, t.Name)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

// pageCandidates 第 page 页（0 基）的下载候选地址，按优先级排列：
// 原图优先，下载失败逐个回退到压缩图。
func (i *illust) pageCandidates(page int) []string {
	if i.PageCount > 1 && page >= 0 && page < len(i.MetaPages) {
		urls := i.MetaPages[page].ImageURLs
		return trimEmpty(urls.Original, urls.Large, urls.Medium)
	}
	if u := i.MetaSinglePage.OriginalImageURL; u != "" {
		return trimEmpty(u, i.ImageURLs.Large, i.ImageURLs.Medium)
	}
	// 列表/排行数据常缺原图地址，用 large 压缩图兜底。
	return trimEmpty(i.ImageURLs.Large, i.ImageURLs.Medium)
}

// previewCandidates 列表预览的下载候选（第 1 页即可）。
func (i *illust) previewCandidates() []string { return i.pageCandidates(0) }

// trimEmpty 过滤空串，保证调用方拿到的候选列表不含空地址。
func trimEmpty(urls ...string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if u != "" {
			out = append(out, u)
		}
	}
	return out
}
