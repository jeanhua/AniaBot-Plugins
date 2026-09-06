package pixiv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/net/proxy"
)

// Pixiv App API 地址与端点。路径以成熟开源客户端（PixEz/pixivpy）为准——
// 排行是 /v1/illust/ranking 而不是 /v1/ranking/illust，相关作品用 v2，
// pixiv 会不定期挪动端点，失联时先来这里对照。
const (
	pixivAPIBase = "https://app-api.pixiv.net"

	epSearchIllust      = "/v1/search/illust"
	epIllustRanking     = "/v1/illust/ranking"
	epIllustDetail      = "/v1/illust/detail"
	epIllustRelated     = "/v2/illust/related"
	epIllustRecommended = "/v1/illust/recommended"
	epUserIllusts       = "/v1/user/illusts"
	epUserDetail        = "/v1/user/detail"
)

// 通用列表响应。
type illustListResp struct {
	Illusts []illust `json:"illusts"`
	NextURL string   `json:"next_url"`
}

// 作品详情响应。
type illustDetailResp struct {
	Illust illust `json:"illust"`
}

// 画师统计信息。
type userProfile struct {
	TotalIllusts int `json:"total_illusts"`
	TotalManga   int `json:"total_manga"`
}

// 画师详情响应。
type userDetailResp struct {
	User    pixivUser   `json:"user"`
	Profile userProfile `json:"profile"`
}

// newPixivHTTPClient 构建 Pixiv 专用 HTTP 客户端：未配代理时复用框架注入的
// 共享 RestyClient；配了代理则新建（resty 不支持 socks5，需自建 DialContext），
// API 请求与图片下载共用，保证都走代理。
func newPixivHTTPClient(injected *resty.Client, proxyURL string) *resty.Client {
	if injected != nil && strings.TrimSpace(proxyURL) == "" {
		return injected
	}
	rc := resty.New().SetTimeout(60 * time.Second)
	applyProxy(rc, proxyURL)
	return rc
}

// applyProxy 给 resty 客户端挂代理：http(s) 走 SetProxy，socks5 自建 Transport
// （模式与框架 telegram 适配器一致）。
func applyProxy(rc *resty.Client, proxyURL string) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return
	}
	if strings.HasPrefix(proxyURL, "socks5://") || strings.HasPrefix(proxyURL, "socks5h://") {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return
		}
		d, err := proxy.FromURL(u, proxy.Direct)
		if err != nil {
			return
		}
		if cd, ok := d.(proxy.ContextDialer); ok {
			rc.SetTransport(&http.Transport{DialContext: cd.DialContext})
		}
		return
	}
	rc.SetProxy(proxyURL)
}

// validateProxy 校验代理地址格式，Start 时提前发现配错。
func validateProxy(proxyURL string) error {
	u, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return nil
	}
	return fmt.Errorf("不支持的代理协议 %q（支持 http/https/socks5）", u.Scheme)
}

// apiGet 带登录态的 GET：自动补 token 与 App 头；401 时强制刷新 token 重试一次。
func (p *PixivPlugin) apiGet(ctx context.Context, path string, query map[string]string, out any) error {
	if p.http == nil {
		p.http = newPixivHTTPClient(p.RestyClient, p.cfg.Proxy)
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := p.ensureToken(ctx)
		if err != nil {
			return err
		}
		clientTime := time.Now().Format("2006-01-02T15:04:05.000000")
		req := p.http.R().SetContext(ctx).
			SetHeader("Authorization", "Bearer "+tok).
			SetHeader("X-Client-Time", clientTime).
			SetHeader("X-Client-Hash", clientHash(clientTime)).
			SetHeader("App-OS", pixivAppOS).
			SetHeader("App-OS-Version", pixivAppOSVersion).
			SetHeader("App-Version", pixivAppVersion).
			SetHeader("Accept-Language", "zh-CN").
			SetHeader("User-Agent", pixivAppUserAgent)
		if query != nil {
			req = req.SetQueryParams(query)
		}
		resp, err := req.Get(pixivAPIBase + path)
		if err != nil {
			return fmt.Errorf("请求失败：%w", err)
		}
		if resp.StatusCode() == http.StatusUnauthorized {
			p.invalidateToken()
			lastErr = fmt.Errorf("登录态已过期（401）")
			continue
		}
		if resp.IsError() {
			if resp.StatusCode() == http.StatusForbidden {
				return fmt.Errorf("请求被 Pixiv 拒绝（403），可能触发了限流，稍后再试")
			}
			return fmt.Errorf("Pixiv 接口错误（%s）：%s", resp.Status(), parseAPIErrorMessage(resp.Body(), resp.Status()))
		}
		if err := json.Unmarshal(resp.Body(), out); err != nil {
			return fmt.Errorf("解析响应失败：%w", err)
		}
		return nil
	}
	return lastErr
}

// nextOffset 从 next_url 里解析 offset，-1 表示没有下一页。
func nextOffset(nextURL string) int {
	nextURL = strings.TrimSpace(nextURL)
	if nextURL == "" {
		return -1
	}
	u, err := url.Parse(nextURL)
	if err != nil {
		return -1
	}
	v := u.Query().Get("offset")
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return -1
	}
	return n
}

// matchContentType 分级过滤：x_restrict 0=全年龄 1=R-18G 2=R-18。
func matchContentType(contentType string, xRestrict int) bool {
	switch contentType {
	case "mixed":
		return true
	case "r18":
		return xRestrict != 0
	default:
		return xRestrict == 0
	}
}

// contentTypeName 分级配置的中文说明。
func contentTypeName(contentType string) string {
	switch contentType {
	case "mixed":
		return "mixed（不过滤）"
	case "r18":
		return "r18（仅 R18）"
	default:
		return "safe（仅全年龄）"
	}
}

// collectIllusts 拉取列表并做分级/可见性过滤，最多补 3 页凑满 listSize。
// 首页失败返回错误；补页失败带已有结果返回（记日志）。
func (p *PixivPlugin) collectIllusts(ctx context.Context, listSize, startOffset int, fetch func(offset int) ([]illust, int, error)) ([]illust, error) {
	var out []illust
	offset := startOffset
	for page := 0; page < 3; page++ {
		items, next, err := fetch(offset)
		if err != nil {
			if len(out) == 0 {
				return nil, err
			}
			p.Logger.Warn("Pixiv 列表补页失败，返回已有结果", "error", err, "got", len(out))
			return out, nil
		}
		for i := range items {
			if items[i].visible() && matchContentType(p.cfg.ContentType, items[i].XRestrict) {
				out = append(out, items[i])
			}
		}
		if len(out) >= listSize || next < 0 {
			break
		}
		offset = next
	}
	if len(out) > listSize {
		out = out[:listSize]
	}
	return out, nil
}

// apiSearchIllust 关键词搜索插画（tag 部分匹配，按发布时间倒序）。
func (p *PixivPlugin) apiSearchIllust(ctx context.Context, word string, offset int) ([]illust, int, error) {
	var resp illustListResp
	q := map[string]string{
		"word":          word,
		"search_target": "partial_match_for_tags",
		"sort":          "date_desc",
		"filter":        "for_ios",
	}
	if offset > 0 {
		q["offset"] = strconv.Itoa(offset)
	}
	if err := p.apiGet(ctx, epSearchIllust, q, &resp); err != nil {
		return nil, -1, err
	}
	return resp.Illusts, nextOffset(resp.NextURL), nil
}

// apiRankingIllust 插画排行榜，mode 形如 day/week/month/day_r18/week_r18。
func (p *PixivPlugin) apiRankingIllust(ctx context.Context, mode string, offset int) ([]illust, int, error) {
	var resp illustListResp
	q := map[string]string{"mode": mode, "filter": "for_ios"}
	if offset > 0 {
		q["offset"] = strconv.Itoa(offset)
	}
	if err := p.apiGet(ctx, epIllustRanking, q, &resp); err != nil {
		return nil, -1, err
	}
	return resp.Illusts, nextOffset(resp.NextURL), nil
}

// apiIllustDetail 作品详情。
func (p *PixivPlugin) apiIllustDetail(ctx context.Context, id int64) (illust, error) {
	var resp illustDetailResp
	q := map[string]string{
		"illust_id": strconv.FormatInt(id, 10),
		"filter":    "for_ios",
	}
	if err := p.apiGet(ctx, epIllustDetail, q, &resp); err != nil {
		return illust{}, err
	}
	return resp.Illust, nil
}

// apiUserIllusts 画师插画作品列表。
func (p *PixivPlugin) apiUserIllusts(ctx context.Context, userID string, offset int) ([]illust, int, error) {
	var resp illustListResp
	q := map[string]string{
		"user_id": userID,
		"type":    "illust",
		"filter":  "for_ios",
	}
	if offset > 0 {
		q["offset"] = strconv.Itoa(offset)
	}
	if err := p.apiGet(ctx, epUserIllusts, q, &resp); err != nil {
		return nil, -1, err
	}
	return resp.Illusts, nextOffset(resp.NextURL), nil
}

// apiRelatedIllust 相关（相似）作品。
func (p *PixivPlugin) apiRelatedIllust(ctx context.Context, id int64, offset int) ([]illust, int, error) {
	var resp illustListResp
	q := map[string]string{
		"illust_id": strconv.FormatInt(id, 10),
		"filter":    "for_ios",
	}
	if offset > 0 {
		q["offset"] = strconv.Itoa(offset)
	}
	if err := p.apiGet(ctx, epIllustRelated, q, &resp); err != nil {
		return nil, -1, err
	}
	return resp.Illusts, nextOffset(resp.NextURL), nil
}

// apiRecommendedIllust 为登录账号个性化推荐。
func (p *PixivPlugin) apiRecommendedIllust(ctx context.Context, offset int) ([]illust, int, error) {
	var resp illustListResp
	q := map[string]string{
		"content_type":            "illust",
		"include_ranking_illusts": "true",
		"filter":                  "for_ios",
	}
	if offset > 0 {
		q["offset"] = strconv.Itoa(offset)
	}
	if err := p.apiGet(ctx, epIllustRecommended, q, &resp); err != nil {
		return nil, -1, err
	}
	return resp.Illusts, nextOffset(resp.NextURL), nil
}

// apiUserDetail 画师详情（名字与作品统计，失败由调用方降级处理）。
func (p *PixivPlugin) apiUserDetail(ctx context.Context, userID string) (userDetailResp, error) {
	var resp userDetailResp
	q := map[string]string{"user_id": userID}
	if err := p.apiGet(ctx, epUserDetail, q, &resp); err != nil {
		return userDetailResp{}, err
	}
	return resp, nil
}
