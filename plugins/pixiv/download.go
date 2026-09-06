package pixiv

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 下载常量：超时、体积上限与并发数。
const (
	// pixivImageDownloadTimeout 单张图片下载超时（事件整体另有框架 5 分钟上限）。
	pixivImageDownloadTimeout = 30 * time.Second
	// pixivImageMaxBytes 单张体积上限，超限判失败（防 base64 后消息过大发不出去）。
	pixivImageMaxBytes = 20 << 20
	// pixivImageDownloadConcurrency 并发下载数，避免打爆图床。
	pixivImageDownloadConcurrency = 3
	// pixiv 图床校验 Referer，不带必 403，插件侧下载必须带上。
	pixivImageReferer   = "https://www.pixiv.net/"
	pixivImageUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 AniaBot-pixiv/1.0"
)

// pixivDownload 单张图的下载结果：b64 为空表示失败，原因看 err。
type pixivDownload struct {
	url  string
	b64  string
	size int
	err  error
}

// downloadImages 并发下载多张图并转 base64，结果顺序与输入一致；
// candidates 按优先级排列（原图在前），失败逐个回退。绝不静默吞错。
func (p *PixivPlugin) downloadImages(ctx context.Context, candidates [][]string) []pixivDownload {
	out := make([]pixivDownload, len(candidates))
	var wg sync.WaitGroup
	sem := make(chan struct{}, pixivImageDownloadConcurrency)
	for i, cands := range candidates {
		wg.Add(1)
		go func(idx int, cands []string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out[idx] = pixivDownload{err: ctx.Err()}
				return
			}
			b64, size, url, err := p.downloadOne(ctx, cands)
			out[idx] = pixivDownload{url: url, b64: b64, size: size, err: err}
		}(i, cands)
	}
	wg.Wait()
	return out
}

// downloadOne 按候选顺序下载单张图，全部失败返回最后一个错误。
func (p *PixivPlugin) downloadOne(ctx context.Context, cands []string) (b64 string, size int, url string, err error) {
	var lastErr error
	for _, u := range cands {
		if strings.TrimSpace(u) == "" {
			continue
		}
		dctx, cancel := context.WithTimeout(ctx, pixivImageDownloadTimeout)
		body, derr := p.downloadImageBytes(dctx, u)
		cancel()
		if derr != nil {
			lastErr = derr
			continue
		}
		return base64.StdEncoding.EncodeToString(body), len(body), u, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("图片地址为空")
	}
	return "", 0, "", lastErr
}

// downloadImageBytes 下载单张图片字节。优先用插件客户端（统一代理），
// 未初始化时退回标准库直连；两种路径都带 Referer+UA 并做体积上限。
// 不记日志（调用方统一记），只返回可直接展示的错误。
func (p *PixivPlugin) downloadImageBytes(ctx context.Context, url string) ([]byte, error) {
	if p.http != nil {
		resp, err := p.http.R().
			SetContext(ctx).
			SetHeader("Referer", pixivImageReferer).
			SetHeader("User-Agent", pixivImageUserAgent).
			Get(url)
		if err != nil {
			return nil, err
		}
		if resp.IsError() {
			return nil, fmt.Errorf("图床返回 %s", resp.Status())
		}
		body := resp.Body()
		if len(body) == 0 {
			return nil, fmt.Errorf("图床返回空内容")
		}
		if len(body) > pixivImageMaxBytes {
			return nil, fmt.Errorf("图片过大（%.1fMB，上限 %dMB），已跳过", float64(len(body))/(1<<20), pixivImageMaxBytes/(1<<20))
		}
		return body, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", pixivImageReferer)
	req.Header.Set("User-Agent", pixivImageUserAgent)
	client := &http.Client{Timeout: pixivImageDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("图床返回 %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, pixivImageMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片失败：%w", err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("图床返回空内容")
	}
	if len(body) > pixivImageMaxBytes {
		return nil, fmt.Errorf("图片过大（超 %dMB），已跳过", pixivImageMaxBytes/(1<<20))
	}
	return body, nil
}

// shortDownloadErr 把下载错误翻译成用户能看懂的一句话（日志里仍记原始 err）。
func shortDownloadErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "deadline exceeded") || strings.Contains(s, "Client.Timeout") || strings.Contains(s, "timeout") || strings.Contains(s, "Timeout"):
		return "下载超时（30 秒），图床太慢或代理不通，稍后再试"
	case strings.Contains(s, "403"):
		return "图床拒绝访问（403），检查 proxy 配置或稍后再试"
	case strings.Contains(s, "404"):
		return "原图已失效（404）"
	case strings.Contains(s, "图片过大"), strings.Contains(s, "空内容"), strings.Contains(s, "图片地址为空"):
		return s
	case strings.Contains(s, "canceled") || strings.Contains(s, "cancel"):
		return "下载被取消，稍后再试"
	default:
		if len(s) > 120 {
			s = s[:120] + "……"
		}
		return "下载失败：" + s
	}
}
