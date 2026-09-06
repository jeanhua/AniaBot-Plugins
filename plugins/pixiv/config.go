package pixiv

// pixivConfig 插件配置。实现 plugin.ConfigSchemaProvider 后，
// 面板「配置管理」会自动渲染表单，框架在 Start 前填充到 p.cfg。
// 与其他插件保持一致：整个插件共用一个 group（插件名），面板侧边栏只出现一项。
type pixivConfig struct {
	Enable bool `cfg:"plugin.pixiv.enable" label:"启用 Pixiv" group:"Pixiv" default:"true" help:"关闭后不响应任何 /pixiv 指令"`

	RefreshToken string `cfg:"plugin.pixiv.refresh_token" label:"Refresh Token" type:"password" sensitive:"true" group:"Pixiv" help:"Pixiv OAuth refresh_token（长期有效），获取方法见插件 README；留空则提示未登录"`

	Proxy string `cfg:"plugin.pixiv.proxy" label:"网络代理" group:"Pixiv" default:"" help:"访问 Pixiv 用的代理，支持 http:// 或 socks5://，国内直连不可达；API 与图片下载都走它，留空则直连"`

	ContentType string `cfg:"plugin.pixiv.content_type" label:"内容分级" type:"select" options:"safe,mixed,r18" group:"Pixiv" default:"safe" help:"safe=仅全年龄，mixed=不过滤，r18=仅 R18；safe 下请求 R18 榜单会被拒绝"`

	ListSize     int `cfg:"plugin.pixiv.list_size" label:"列表条数" group:"Pixiv" default:"5" help:"搜索/排行/推荐/画师/相关列表返回的条目数，1~10"`
	PreviewCount int `cfg:"plugin.pixiv.preview_count" label:"列表附图数" group:"Pixiv" default:"1" help:"列表命令附带发送的预览图张数，0~3，0 只发文字列表防刷屏"`

	CooldownSec int `cfg:"plugin.pixiv.cooldown_sec" label:"个人冷却(秒)" group:"Pixiv" default:"20" help:"同一用户两次请求的最小间隔，管理员可旁路"`
	DailyLimit  int `cfg:"plugin.pixiv.daily_limit" label:"每日限量(每人)" group:"Pixiv" default:"30" help:"每人每天最多请求几次功能命令，0 表示不限量；调太高可能触发 Pixiv 风控"`

	AllowGroups  []string `cfg:"plugin.pixiv.allow_groups" label:"放行群聊(正则)" group:"Pixiv" help:"允许使用的群，每行一条正则，对群号与完整 ID 同时匹配；留空=不放行任何群，填 .* 放行所有群"`
	AllowFriends []string `cfg:"plugin.pixiv.allow_friends" label:"放行好友(正则)" group:"Pixiv" help:"允许使用的好友，每行一条正则；留空=不放行任何好友，填 .* 放行所有好友"`
	AdminBypass  bool     `cfg:"plugin.pixiv.admin_bypass" label:"管理员旁路" group:"Pixiv" default:"true" help:"管理员不受放行名单、冷却与每日限量限制"`
	SilentDeny   bool     `cfg:"plugin.pixiv.silent_deny" label:"非放行会话保持沉默" group:"Pixiv" default:"false" help:"开启后，在未放行的群/好友里收到 /pixiv 装作没看见（交给后续插件）；关闭则回复拒绝提示"`
}

// ConfigSchema 声明配置结构体（面板表单 + 默认值 + Start 前自动填充）。
// 铁律：必须返回同一个指针，且不能在里面用任何注入字段。
func (p *PixivPlugin) ConfigSchema() any { return &p.cfg }
