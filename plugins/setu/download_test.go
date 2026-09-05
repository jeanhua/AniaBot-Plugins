package setu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDownloadImageBytesSuccess 图床替身：缺 Referer 直接 403（复刻 pixiv 系图床行为），
// 以此验证插件确实带了 Referer+UA（以前直发 URL 就是栽在这里）。
func TestDownloadImageBytesSuccess(t *testing.T) {
	img := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != setuImageReferer {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "need ua", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(img)
	}))
	defer srv.Close()
	p := &SetuPlugin{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	body, err := p.downloadImageBytes(ctx, srv.URL+"/a.png")
	if err != nil {
		t.Fatalf("下载失败：%v", err)
	}
	if len(body) != len(img) {
		t.Fatalf("字节数不对：期望 %d 实际 %d", len(img), len(body))
	}
}

// TestDownloadImageBytesHTTPError 图床 404 时必须返回带状态码的错误（调用方翻译成提示）。
func TestDownloadImageBytesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	p := &SetuPlugin{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := p.downloadImageBytes(ctx, srv.URL+"/gone.jpg"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("期望 404 错误，实际 %v", err)
	}
}

// TestDownloadSetuImagesMixed 一好一坏混发：顺序保持、一成一败、互不影响。
func TestDownloadSetuImagesMixed(t *testing.T) {
	img := []byte("fake-image-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(img)
	}))
	defer srv.Close()
	good := &PixivMeta{Pid: 111, Title: "好图", Author: "A"}
	good.Urls.Regular = srv.URL + "/good.jpg"
	bad := &PixivMeta{Pid: 222, Title: "坏图", Author: "B"}
	bad.Urls.Regular = "http://127.0.0.1:1/bad.jpg" // 保留端口必拒连，失败又快又稳
	p := &SetuPlugin{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	got := p.downloadSetuImages(ctx, []*PixivMeta{good, bad})
	if len(got) != 2 {
		t.Fatalf("期望 2 个结果，实际 %d", len(got))
	}
	if got[0].err != nil || got[0].b64 == "" {
		t.Errorf("好图应下载成功，实际 err=%v", got[0].err)
	}
	if got[1].err == nil {
		t.Error("坏图应下载失败")
	}
	if got[0].meta.Pid != 111 || got[1].meta.Pid != 222 {
		t.Error("结果顺序与输入不一致")
	}
}

// TestShortDownloadErr 错误翻译表：超时/403/404/过大等必须说人话。
func TestShortDownloadErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"超时", context.DeadlineExceeded, "超时"},
		{"403", errors.New("图床返回 403 Forbidden"), "403"},
		{"404", errors.New("图床返回 404 Not Found"), "404"},
		{"过大原样", errors.New("图片过大（超 20MB），已跳过"), "图片过大"},
		{"空错误", nil, ""},
	}
	for _, c := range cases {
		if got := shortDownloadErr(c.err); !strings.Contains(got, c.want) {
			t.Errorf("%s：期望包含 %q，实际 %q", c.name, c.want, got)
		}
	}
}

// TestFailTexts 失败文案必须带 pid 与作品页链接（兜底可点），空输入返回空。
func TestFailTexts(t *testing.T) {
	m := &PixivMeta{Pid: 12345, Title: "T", Author: "A"}
	f := []setuDownload{{meta: m, url: "http://x/y.jpg", err: errors.New("图床返回 403 Forbidden")}}
	if s := allDownloadFailText(f); !strings.Contains(s, "12345") || !strings.Contains(s, "artworks/12345") || !strings.Contains(s, "403") {
		t.Errorf("全失败文案缺关键信息：%s", s)
	}
	if s := downloadFailSummary(f); !strings.Contains(s, "12345") {
		t.Errorf("部分失败文案缺 pid：%s", s)
	}
	if s := sendFailSummary(f); !strings.Contains(s, "artworks/12345") {
		t.Errorf("发送失败文案缺作品页：%s", s)
	}
	if downloadFailSummary(nil) != "" || sendFailSummary(nil) != "" {
		t.Error("空输入应返回空字符串")
	}
}
