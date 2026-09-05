package setu

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

// 本地下载再发送的相关常量：超时、体积上限与并发数。
const (
	// setuImageDownloadTimeout 单张原图下载超时（事件整体另有框架 5 分钟上限）。
	setuImageDownloadTimeout = 30 * time.Second
	// setuImageMaxBytes 单张原图体积上限，超限直接判失败（防 base64 后消息过大发不出去）。
	setuImageMaxBytes = 20 << 20
	// setuImageDownloadConcurrency 图片下载并发数，避免多连发时打爆图床。
	setuImageDownloadConcurrency = 3
	// pixiv 系图床常校验 Referer，不带会被 403，插件侧下载时必须带上；
	// 以前直发 URL 是 NapCat 侧去拉（不带 Referer），这就是经常发送失败的主因。
	setuImageReferer   = "https://www.pixiv.net/"
	setuImageUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 AniaBot-setu/1.1"
)

// setuDownload 单张图的下载结果：b64 为空表示失败，看 err。
type setuDownload struct {
	meta *PixivMeta
	url  string
	b64  string
	size int
	err  error
}

// downloadSetuImages 并发把原图下载到本地内存并转好 base64。
// 顺序与输入 metas 一致；全部失败/部分失败都如实返回，由调用方决定提示文案，绝不静默吞掉。
func (p *SetuPlugin) downloadSetuImages(ctx context.Context, metas []*PixivMeta) []setuDownload {
	out := make([]setuDownload, len(metas))
	var wg sync.WaitGroup
	sem := make(chan struct{}, setuImageDownloadConcurrency)
	for i, m := range metas {
		wg.Add(1)
		go func(idx int, meta *PixivMeta) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out[idx] = setuDownload{meta: meta, err: ctx.Err()}
				return
			}
			raw := ""
			if meta != nil {
				raw = rewriteImageHost(meta.ImageURL(), p.cfg.ImageProxy)
			}
			if strings.TrimSpace(raw) == "" {
				out[idx] = setuDownload{meta: meta, url: raw, err: fmt.Errorf("图片地址为空")}
				return
			}
			dctx, cancel := context.WithTimeout(ctx, setuImageDownloadTimeout)
			defer cancel()
			body, err := p.downloadImageBytes(dctx, raw)
			if err != nil {
				out[idx] = setuDownload{meta: meta, url: raw, err: err}
				return
			}
			out[idx] = setuDownload{meta: meta, url: raw, b64: base64.StdEncoding.EncodeToString(body), size: len(body)}
		}(i, m)
	}
	wg.Wait()
	return out
}

// downloadImageBytes 下载单张图片字节。优先用框架注入的 RestyClient
// （统一超时/代理），未注入时退回标准库；两种路径都带 Referer+UA 并做体积上限。
// 注意：本函数不记日志（调用方统一记），只返回可直接展示的错误。
func (p *SetuPlugin) downloadImageBytes(ctx context.Context, url string) ([]byte, error) {
	if p.RestyClient != nil {
		resp, err := p.RestyClient.R().
			SetContext(ctx).
			SetHeader("Referer", setuImageReferer).
			SetHeader("User-Agent", setuImageUserAgent).
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
		if len(body) > setuImageMaxBytes {
			return nil, fmt.Errorf("图片过大（%.1fMB，上限 %dMB），已跳过", float64(len(body))/(1<<20), setuImageMaxBytes/(1<<20))
		}
		return body, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", setuImageReferer)
	req.Header.Set("User-Agent", setuImageUserAgent)
	client := &http.Client{Timeout: setuImageDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("图床返回 %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, setuImageMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片失败：%w", err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("图床返回空内容")
	}
	if len(body) > setuImageMaxBytes {
		return nil, fmt.Errorf("图片过大（超 %dMB），已跳过", setuImageMaxBytes/(1<<20))
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
		return "下载超时（30 秒），图床太慢或网络被墙，稍后再试"
	case strings.Contains(s, "403"):
		return "图床拒绝访问（403），换 image_proxy 镜像或稍后再试"
	case strings.Contains(s, "404"):
		return "原图已失效（404），换个 tag 试试"
	case strings.Contains(s, "图片过大"):
		return s
	case strings.Contains(s, "空内容") || strings.Contains(s, "图片地址为空"):
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
