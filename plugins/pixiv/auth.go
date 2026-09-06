package pixiv

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Pixiv App API 的 OAuth 常量：client_id/secret 与签名密钥均为 pixivpy 等
// 开源客户端的公知值（非本插件私有密钥）；Pixiv 改版导致登录失效时只需更新这里。
const (
	pixivOAuthURL          = "https://oauth.secure.pixiv.net/auth/token"
	pixivOAuthClientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	pixivOAuthClientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	pixivHashSecret        = "28c1fdd170a5204386cb1313c7077b34f83e4aaf4aa829ce78c231e05b0bae2c"
	pixivAppUserAgent      = "PixivAndroidApp/5.0.233 (Android 11; Pixel 5)"
	pixivAppOS             = "ios"
	pixivAppOSVersion      = "14.6"
)

// authResp /auth/token 的响应。
type authResp struct {
	AccessToken  string    `json:"access_token"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token"`
	User         pixivUser `json:"user"`
}

// apiError Pixiv 错误响应的通用结构。
type apiError struct {
	Error struct {
		Message     string `json:"message"`
		Reason      string `json:"reason"`
		UserMessage string `json:"user_message"`
	} `json:"error"`
}

// pixivSession 登录态缓存：access_token 约 1 小时过期，惰性刷新（并发只刷一次）。
type pixivSession struct {
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
	userID      string
	userName    string
}

// clientHashWith 计算请求签名 md5(clientTime + secret)，Pixiv 服务端按同样规则校验。
// secret 抽成参数方便单测（真实调用固定传 pixivHashSecret）。
func clientHashWith(clientTime, secret string) string {
	sum := md5.Sum([]byte(clientTime + secret))
	return hex.EncodeToString(sum[:])
}

// clientHash App API 请求签名。
func clientHash(clientTime string) string { return clientHashWith(clientTime, pixivHashSecret) }

// ensureToken 取可用的 access_token：为空或临近过期（<5 分钟）时自动刷新。
// 刷新期间持锁，天然单飞行，并发请求不会重复登录。
func (p *PixivPlugin) ensureToken(ctx context.Context) (string, error) {
	s := &p.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accessToken != "" && time.Until(s.expiresAt) > 5*time.Minute {
		return s.accessToken, nil
	}
	if err := p.refreshSessionLocked(ctx); err != nil {
		return "", err
	}
	return s.accessToken, nil
}

// invalidateToken 登录态失效（如 401）时清空，下次请求会重新刷新。
func (p *PixivPlugin) invalidateToken() {
	s := &p.session
	s.mu.Lock()
	s.accessToken = ""
	s.expiresAt = time.Time{}
	s.mu.Unlock()
}

// refreshSessionLocked 用 refresh_token 换新 access_token（调用方已持 session 锁）。
func (p *PixivPlugin) refreshSessionLocked(ctx context.Context) error {
	token := strings.TrimSpace(p.cfg.RefreshToken)
	if token == "" {
		return fmt.Errorf("未配置 refresh_token")
	}
	if p.http == nil {
		p.http = newPixivHTTPClient(p.RestyClient, p.cfg.Proxy)
	}
	clientTime := time.Now().Format("2006-01-02T15:04:05.000000")
	resp, err := p.http.R().SetContext(ctx).
		SetHeader("X-Client-Time", clientTime).
		SetHeader("X-Client-Hash", clientHash(clientTime)).
		SetHeader("App-OS", pixivAppOS).
		SetHeader("App-OS-Version", pixivAppOSVersion).
		SetHeader("User-Agent", pixivAppUserAgent).
		SetFormData(map[string]string{
			"client_id":      pixivOAuthClientID,
			"client_secret":  pixivOAuthClientSecret,
			"grant_type":     "refresh_token",
			"refresh_token":  token,
			"include_policy": "1",
			"get_secure_url": "1",
		}).Post(pixivOAuthURL)
	if err != nil {
		return fmt.Errorf("登录请求失败：%w", err)
	}
	if resp.IsError() {
		if strings.Contains(string(resp.Body()), "invalid_grant") {
			return fmt.Errorf("refresh_token 已失效，请重新获取并在面板更新")
		}
		return fmt.Errorf("登录失败（%s）：%s", resp.Status(), parseAPIErrorMessage(resp.Body(), resp.Status()))
	}
	var ar authResp
	if err := json.Unmarshal(resp.Body(), &ar); err != nil {
		return fmt.Errorf("解析登录响应失败：%w", err)
	}
	if ar.AccessToken == "" {
		return fmt.Errorf("登录响应缺少 access_token")
	}
	s := &p.session
	s.accessToken = ar.AccessToken
	ttl := time.Duration(ar.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	s.expiresAt = time.Now().Add(ttl)
	if ar.User.ID != "" {
		s.userID = ar.User.ID
	}
	if ar.User.Name != "" {
		s.userName = ar.User.Name
	}
	return nil
}

// parseAPIErrorMessage 从 Pixiv 错误响应体里提取可读信息，取不到用 fallback。
func parseAPIErrorMessage(body []byte, fallback string) string {
	var ae apiError
	if err := json.Unmarshal(body, &ae); err == nil {
		switch {
		case ae.Error.UserMessage != "":
			return ae.Error.UserMessage
		case ae.Error.Reason != "":
			return ae.Error.Reason
		case ae.Error.Message != "":
			return ae.Error.Message
		}
	}
	return fallback
}
