package setu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 索引与详情地址默认值（与配置默认值保持一致，配置被清空时兜底）。
const (
	defaultIndexURL = "https://cdn.jsdelivr.net/gh/Mabbs/pixiv-index/index.json"
	defaultDataBase = "https://cdn.jsdelivr.net/gh/Mabbs/pixiv-index/data/"
)

// PixivMeta Mabbs 索引中的单图详情（data/<pid>_<p>.json）。
type PixivMeta struct {
	Pid        int64    `json:"pid"`
	P          int      `json:"p"`
	Uid        int64    `json:"uid"`
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	R18        bool     `json:"r18"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Tags       []string `json:"tags"`
	Ext        string   `json:"ext"`
	AiType     int      `json:"aiType"`
	UploadDate int64    `json:"uploadDate"`
	Urls       struct {
		Regular string `json:"regular"`
	} `json:"urls"`
	URL string `json:"url"`
}

// ImageURL 图片直链（索引自带的代理地址，开箱即用）。
func (m *PixivMeta) ImageURL() string { return m.Urls.Regular }

// ArtworkURL 作品主页，方便去 Pixiv 给画师点赞。
func (m *PixivMeta) ArtworkURL() string { return fmt.Sprintf("https://www.pixiv.net/artworks/%d", m.Pid) }

// indexCache 索引内存缓存：index.json 只在启动/定时刷新/兜底时拉取，
// 绝不在每次 /setu 请求时全量拉取；详情 data/*.json 按需拉取命中的几个。
type indexCache struct {
	mu        sync.RWMutex
	files     []string
	updatedAt time.Time
}

func (c *indexCache) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.files)
}

func (c *indexCache) snapshot() ([]string, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.files, c.updatedAt
}

// pickRandomFiles 随机抽 n 个不重复的文件名（seen 用于跨轮去重，可为 nil）。
func (c *indexCache) pickRandomFiles(n int, seen map[string]bool) []string {
	files, _ := c.snapshot()
	if len(files) == 0 || n <= 0 {
		return nil
	}
	out := make([]string, 0, n)
	guard := 0
	for len(out) < n && guard < n*10 {
		guard++
		f := files[rand.Intn(len(files))]
		if seen != nil {
			if seen[f] {
				continue
			}
			seen[f] = true
		}
		out = append(out, f)
	}
	return out
}

// httpGetJSON 发 GET 请求并解析 JSON。优先用框架注入的 RestyClient
// （统一超时/代理），未注入时退回标准库，保证单测与异常环境可用。
func (p *SetuPlugin) httpGetJSON(ctx context.Context, url string, v any) error {
	if p.RestyClient != nil {
		resp, err := p.RestyClient.R().SetContext(ctx).Get(url)
		if err != nil {
			return err
		}
		if resp.IsError() {
			return fmt.Errorf("http %s", resp.Status())
		}
		if err := json.Unmarshal(resp.Body(), v); err != nil {
			return fmt.Errorf("解析响应失败：%w", err)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AniaBot-setu/1.0")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("解析响应失败：%w", err)
	}
	return nil
}

// reloadIndex 全量拉取索引并整体替换缓存（调用方负责单飞行控制）。
func (p *SetuPlugin) reloadIndex(ctx context.Context) error {
	indexURL := strings.TrimSpace(p.cfg.IndexURL)
	if indexURL == "" {
		indexURL = defaultIndexURL
	}
	var files []string
	if err := p.httpGetJSON(ctx, indexURL, &files); err != nil {
		return fmt.Errorf("拉取索引失败：%w", err)
	}
	// 过滤空条目，防脏数据。
	clean := make([]string, 0, len(files))
	for _, f := range files {
		if strings.TrimSpace(f) != "" {
			clean = append(clean, f)
		}
	}
	if len(clean) == 0 {
		return fmt.Errorf("索引为空，拒绝替换缓存")
	}
	p.index.mu.Lock()
	p.index.files = clean
	p.index.updatedAt = time.Now()
	p.index.mu.Unlock()
	return nil
}

// ensureIndex 索引为空时同步拉取一次（loadMu 保证同时只有一个拉取在飞）。
func (p *SetuPlugin) ensureIndex(ctx context.Context) error {
	if p.index.size() > 0 {
		return nil
	}
	p.loadMu.Lock()
	defer p.loadMu.Unlock()
	if p.index.size() > 0 {
		return nil
	}
	return p.reloadIndex(ctx)
}

// refreshLoop 定时刷新索引缓存的后台循环，退出由 ctx 控制。
func (p *SetuPlugin) refreshLoop(ctx context.Context) {
	if p.cfg.RefreshHours <= 0 {
		return
	}
	ticker := time.NewTicker(time.Duration(p.cfg.RefreshHours) * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			if err := p.reloadIndex(fctx); err != nil {
				p.Logger.Warn("涩图索引定时刷新失败，继续使用旧缓存", "error", err)
			} else {
				p.Logger.Info("涩图索引定时刷新完成", "size", p.index.size())
			}
			cancel()
		}
	}
}

// dataBase 详情地址前缀兜底（保证末尾带 /）。
func (p *SetuPlugin) dataBase() string {
	base := strings.TrimSpace(p.cfg.DataBase)
	if base == "" {
		base = defaultDataBase
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base
}

// fetchDetail 拉取单个详情文件。
func (p *SetuPlugin) fetchDetail(ctx context.Context, file string) (*PixivMeta, error) {
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var meta PixivMeta
	if err := p.httpGetJSON(dctx, p.dataBase()+file, &meta); err != nil {
		return nil, err
	}
	if meta.Pid == 0 || meta.ImageURL() == "" {
		return nil, fmt.Errorf("详情数据不完整：%s", file)
	}
	return &meta, nil
}

// setuFilter 搜索过滤条件：r18 取 "0"/"1"/"2"，tags 为用户 tag（AND 关系）。
type setuFilter struct {
	tags []string
	r18  string
}

// matchR18 详情是否符合 R18 过滤。
func matchR18(m *PixivMeta, r18 string) bool {
	switch r18 {
	case "0":
		return !m.R18
	case "1":
		return m.R18
	default:
		return true
	}
}

// matchTags 用户 tag 是否全部命中（大小写不敏感的子串匹配，
// 范围覆盖标签、标题与作者，方便中日双语标签互相命中）。
func matchTags(m *PixivMeta, want []string) bool {
	if len(want) == 0 {
		return true
	}
	hay := make([]string, 0, len(m.Tags)+2)
	for _, t := range m.Tags {
		hay = append(hay, strings.ToLower(t))
	}
	hay = append(hay, strings.ToLower(m.Title), strings.ToLower(m.Author))
	for _, w := range want {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		hit := false
		for _, h := range hay {
			if h != "" && strings.Contains(h, w) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// fetchBatch 并发拉取一批详情（最多 5 并发），失败的 individual 直接丢弃。
func (p *SetuPlugin) fetchBatch(ctx context.Context, files []string) []*PixivMeta {
	out := make([]*PixivMeta, 0, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for _, f := range files {
		wg.Add(1)
		go func(file string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			m, err := p.fetchDetail(ctx, file)
			if err != nil {
				return
			}
			mu.Lock()
			out = append(out, m)
			mu.Unlock()
		}(f)
	}
	wg.Wait()
	return out
}

// searchSetu 在缓存索引中随机抽取并按条件筛选，返回命中的详情。
// 最多拉取 maxTries 个详情（tag 越多命中越难），调用方用返回的 tried
// 在“没搜到”时告诉用户翻了多少候选。
func (p *SetuPlugin) searchSetu(ctx context.Context, f setuFilter, need int) (found []*PixivMeta, tried int, err error) {
	if err := p.ensureIndex(ctx); err != nil {
		return nil, 0, err
	}
	if need <= 0 {
		need = 1
	}
	maxTries := p.cfg.MaxSearchTries
	if maxTries <= 0 {
		maxTries = 15
	}
	seen := make(map[string]bool)
	for len(found) < need && tried < maxTries {
		select {
		case <-ctx.Done():
			return found, tried, ctx.Err()
		default:
		}
		batch := 5
		if len(f.tags) == 0 {
			batch = need - len(found)
			if batch <= 0 {
				break
			}
			if batch > 5 {
				batch = 5
			}
		}
		remain := maxTries - tried
		if batch > remain {
			batch = remain
		}
		files := p.index.pickRandomFiles(batch, seen)
		if len(files) == 0 {
			break
		}
		tried += len(files)
		for _, m := range p.fetchBatch(ctx, files) {
			if !matchR18(m, f.r18) || !matchTags(m, f.tags) {
				continue
			}
			dup := false
			for _, e := range found {
				if e.Pid == m.Pid && e.P == m.P {
					dup = true
					break
				}
			}
			if !dup {
				found = append(found, m)
				if len(found) >= need {
					break
				}
			}
		}
	}
	return found, tried, nil
}
