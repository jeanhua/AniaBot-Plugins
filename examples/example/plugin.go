// Package example 是插件市场示例插件：at 机器人发送 /example 回复问候语。
package example

import (
	"context"

	"github.com/jeanhua/AniaBot/common/bot"
	"github.com/jeanhua/AniaBot/common/model/command"
	"github.com/jeanhua/AniaBot/common/model/message"
	"github.com/jeanhua/AniaBot/common/msgchain"
	"github.com/jeanhua/AniaBot/common/plugin"
	"github.com/jeanhua/AniaBot/common/plugininfo"
)

// ExamplePlugin 插件定义：嵌入 plugin.Meta 获得默认实现，只需覆盖需要的方法。
type ExamplePlugin struct {
	plugin.Meta
}

// NewPlugin 构造函数（plugin.json 的 entry.constructor 默认指向这里）。
func NewPlugin() *ExamplePlugin {
	p := &ExamplePlugin{}
	p.Name = "示例插件"
	p.HelpWords = "at 我发送 /example 触发问候"
	p.AdminOnly = false
	p.ShowFor = plugininfo.ShowForGroup | plugininfo.ShowForFriend
	p.Author = "jeanhua"
	p.Version = "1.0.0"
	p.Order = plugin.LevelNormal
	return p
}

// OnGroupMsg 群聊消息事件：返回 (是否继续传播, 错误)。
func (p *ExamplePlugin) OnGroupMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	if !cmd.Mention || cmd.Name != "example" {
		return true, nil
	}
	builder := msgchain.Builder().Group()
	builder.Text("Hello, AniaBot!")
	b.SendGroupMsg(msg.GroupId, builder.Build())
	// 返回 false：本插件已处理，不再向后续插件传播
	return false, nil
}

// OnFriendMsg 私聊消息事件：与群聊一致。
func (p *ExamplePlugin) OnFriendMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	if cmd.Name != "example" {
		return true, nil
	}
	builder := msgchain.Builder().Friend()
	builder.Text("Hello, AniaBot!")
	b.SendFriendMsg(msg.Sender.UserId, builder.Build())
	return false, nil
}
