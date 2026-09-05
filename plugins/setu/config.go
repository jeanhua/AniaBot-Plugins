package setu

// setuConfig 涩图插件配置。实现 plugin.ConfigSchemaProvider 后，
// 面板「配置管理」会自动渲染表单，框架在 Start 前填充到 p.cfg。
type setuConfig struct {
	Enable bool `cfg:"plugin.setu.enable" label:"启用涩图" group:"基础设置" default:"true" help:"关闭后不响应任何涩图指令"`

	IndexURL string `cfg:"plugin.setu.index_url" label:"索引地址" group:"基础设置" default:"https://cdn.jsdelivr.net/gh/Mabbs/pixiv-index/index.json" help:"Pixiv 图片索引地址，只在启动与定时刷新时拉取一次并缓存在内存里，不会每次请求都拉"`
	DataBase string `cfg:"plugin.setu.data_base" label:"详情地址前缀" group:"基础设置" default:"https://cdn.jsdelivr.net/gh/Mabbs/pixiv-index/data/" help:"详情 JSON 所在目录，末尾带 /，每次请求时只按需拉取命中的几个详情文件"`
	RefreshHours int `cfg:"plugin.setu.refresh_hours" label:"索引刷新间隔(小时)" group:"基础设置" default:"12" help:"索引缓存刷新间隔，0 表示仅启动时加载一次不再自动刷新"`

	R18Mode string `cfg:"plugin.setu.r18_mode" label:"R18 模式" type:"select" options:"0,1,2" group:"内容设置" default:"1" help:"0=仅全年龄，1=仅 R18，2=混合。当前索引基本全是 R18 涩图，选 0 会经常搜不到；请务必配合放行名单使用"`
	DefaultTags []string `cfg:"plugin.setu.default_tags" label:"默认标签" group:"内容设置" help:"不带 tag 参数时默认使用的标签，每行一个；留空则纯随机"`
	ImageProxy string `cfg:"plugin.setu.image_proxy" label:"图片代理域名" group:"内容设置" default:"" help:"留空则直接使用索引自带的图片地址；若打不开可填 i.pixiv.cat 等镜像域名，只替换 host 部分"`

	MaxCount int `cfg:"plugin.setu.max_count" label:"单次最多几连" group:"频率限制" default:"3" help:"一次最多返回几张图，1~5，超出会被截断"`
	MaxSearchTries int `cfg:"plugin.setu.max_search_tries" label:"tag 搜索尝试数" group:"频率限制" default:"15" help:"按 tag 搜索时最多拉取几个详情做筛选，越大越容易命中但越慢"`
	CooldownSec int `cfg:"plugin.setu.cooldown_sec" label:"个人冷却(秒)" group:"频率限制" default:"30" help:"同一用户两次涩图请求的最小间隔，管理员可旁路"`
	DailyLimit int `cfg:"plugin.setu.daily_limit" label:"每日限量(每人)" group:"频率限制" default:"20" help:"每人每天最多来几张，0 表示不限量"`

	AllowGroups []string `cfg:"plugin.setu.allow_groups" label:"放行群聊(正则)" group:"放行名单" help:"允许使用涩图的群，每行一条正则，对群号与完整 ID 同时匹配；留空=一个群都不放行，填 .* 放行所有群"`
	AllowFriends []string `cfg:"plugin.setu.allow_friends" label:"放行好友(正则)" group:"放行名单" help:"允许使用涩图的好友，每行一条正则；留空=一个好友都不放行，填 .* 放行所有好友"`
	SilentDeny bool `cfg:"plugin.setu.silent_deny" label:"非放行会话保持沉默" group:"放行名单" default:"false" help:"开启后，在未放行的群/好友里收到 /setu 装作没看见（交给后续插件）；关闭则回一句调侃的拒绝"`
	AdminBypass bool `cfg:"plugin.setu.admin_bypass" label:"管理员旁路" group:"放行名单" default:"true" help:"管理员不受放行名单、冷却与每日限量限制"`
}

// ConfigSchema 声明配置结构体（面板表单 + 默认值 + Start 前自动填充）。
func (p *SetuPlugin) ConfigSchema() any { return &p.cfg }
