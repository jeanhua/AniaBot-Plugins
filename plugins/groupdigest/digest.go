package groupdigest

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jeanhua/AniaBot/bot/component/aichat"
	"github.com/jeanhua/AniaBot/bot/component/llmtool"
	"github.com/jeanhua/AniaBot/common/bot"
	"github.com/jeanhua/AniaBot/common/model/message"
	"github.com/jeanhua/AniaBot/common/msgchain"
)

// generateDigest 用 AI 根据消息快照生成 Markdown 群刊正文。
func (p *GroupDigestPlugin) generateDigest(ctx context.Context, b bot.Bot, gid message.QID, msgs []digestMessage) (string, error) {
	if p.chat == nil {
		return "", errors.New("AI 对话未配置，无法生成群刊")
	}

	var sb strings.Builder
	sb.WriteString("请根据以下群聊记录生成一期群刊。")
	if info, ok := b.GetGroupDetail(gid); ok && info != nil && info.GroupName != "" {
		sb.WriteString("群名：" + info.GroupName + "。")
	}
	fmt.Fprintf(&sb, "共 %d 条消息（按时间顺序）：\n\n", len(msgs))
	shown := 0
	for i, m := range msgs {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(&sb, "%d. [%s] %s：%s\n", i+1, m.Time.Format("01-02 15:04"), m.Nickname, text)
		shown++
	}
	if shown == 0 {
		return "", errors.New("没有可用的群聊消息文本")
	}
	sb.WriteString("\n请直接输出 Markdown 格式的群刊正文，不要输出与正文无关的解释。")

	// 复用同一个 ChatBot，生成前清空历史，保证每期群刊互不串上下文
	if err := p.chat.ClearHistory(ctx); err != nil {
		return "", fmt.Errorf("重置 AI 会话失败: %w", err)
	}
	// 只传空参数：不设输出上限与采样参数，跟随模型 API 默认
	opts := aichat.ChatOptions{}
	resp, _, err := p.chat.Chat(ctx, sb.String(), llmtool.CallBackFuncs{}, opts)
	if err != nil {
		return "", fmt.Errorf("AI 生成群刊失败: %w", err)
	}
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return "", errors.New("AI 返回的群刊内容为空")
	}
	return resp, nil
}

// deliver 按配置的发送形式把群刊发到群：md 文件或 md2img 渲染图片。
func (p *GroupDigestPlugin) deliver(ctx context.Context, b bot.Bot, gid message.QID, md string) error {
	if strings.EqualFold(p.cfg.SendMode, "image") {
		return p.sendImage(ctx, b, gid, md)
	}
	return p.sendMarkdownFile(b, gid, md)
}

// sendMarkdownFile 把群刊正文作为 Markdown 文件发送。
func (p *GroupDigestPlugin) sendMarkdownFile(b bot.Bot, gid message.QID, md string) error {
	name := fmt.Sprintf("群刊-%s.md", time.Now().Format("20060102-150405"))
	b64 := base64.StdEncoding.EncodeToString([]byte(md))
	chain := msgchain.Builder().Group().Text("📰 本期群刊已生成～").FileBase64(name, b64)
	if _, ok := b.SendGroupMsg(gid, chain.Build()); !ok {
		return errors.New("发送群刊 Markdown 文件失败")
	}
	return nil
}

// sendImage 调用本地 md2img-api 服务把 Markdown 渲染成 PNG 图片发送。
func (p *GroupDigestPlugin) sendImage(ctx context.Context, b bot.Bot, gid message.QID, md string) error {
	if p.RestyClient == nil {
		return errors.New("HTTP 客户端不可用，无法调用 md2img 服务")
	}
	baseURL := strings.TrimSpace(p.cfg.MD2ImgURL)
	if baseURL == "" {
		return errors.New("未配置 md2img 服务地址（plugin.groupdigest.md2img_url）")
	}
	renderURL := strings.TrimRight(baseURL, "/") + "/render"

	resp, err := p.RestyClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"markdown": md}).
		Post(renderURL)
	if err != nil {
		return fmt.Errorf("调用 md2img 服务失败: %w", err)
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("md2img 服务返回异常状态码 %d", resp.StatusCode())
	}
	body := resp.Body()
	if len(body) == 0 {
		return errors.New("md2img 服务返回空内容")
	}

	b64 := base64.StdEncoding.EncodeToString(body)
	chain := msgchain.Builder().Group().Text("📰 本期群刊已生成～").ImageBase64(b64)
	if _, ok := b.SendGroupMsg(gid, chain.Build()); !ok {
		return errors.New("发送群刊图片失败")
	}
	return nil
}

// notifyError 记录失败日志并简短告知群成员。
func (p *GroupDigestPlugin) notifyError(b bot.Bot, gid message.QID, err error) {
	p.Logger.Error("生成群刊失败", "group", gid.String(), "error", err.Error())
	chain := msgchain.Builder().Group().Text("群刊生成失败：" + err.Error())
	b.SendGroupMsg(gid, chain.Build())
}
