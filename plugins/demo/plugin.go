// Package demo 是 CI 自动同步索引的端到端测试插件。
package demo

import (
	"context"

	"github.com/jeanhua/AniaBot/common/bot"
	"github.com/jeanhua/AniaBot/common/model/command"
	"github.com/jeanhua/AniaBot/common/model/message"
	"github.com/jeanhua/AniaBot/common/plugin"
)

type DemoPlugin struct {
	plugin.Meta
}

func NewPlugin() *DemoPlugin {
	p := &DemoPlugin{}
	p.Name = "演示插件"
	p.HelpWords = "测试用"
	p.Author = "jeanhua"
	p.Version = "1.0.0"
	return p
}

func (p *DemoPlugin) OnGroupMsg(ctx context.Context, b bot.Bot, cmd command.Command, msg message.Message) (bool, error) {
	return true, nil
}
