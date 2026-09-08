package music

// musicConfig 音乐点歌插件配置。实现 plugin.ConfigSchemaProvider 后，
// 面板「配置管理」会自动渲染表单，框架在 Start 前填充到 p.cfg。
type musicConfig struct {
	Enable bool `cfg:"plugin.music.enable" label:"启用点歌" group:"音乐" default:"true" help:"关闭后不响应任何点歌指令"`

	APIBase string `cfg:"plugin.music.api_base" label:"API 地址" group:"音乐" default:"https://music-api.gdstudio.xyz/api.php" help:"GD音乐台开放 API 地址，一般无需修改"`
	Source  string `cfg:"plugin.music.source" label:"默认音源" type:"select" options:"netease,joox,bilibili,tencent,kuwo,tidal,qobuz,apple,ytmusic,spotify" group:"音乐" default:"netease" help:"搜索与点播使用的音源，netease/joox/bilibili 为当前稳定音源，其余可能间歇不可用"`
	Bitrate string `cfg:"plugin.music.bitrate" label:"音质" type:"select" options:"128,192,320,740,999" group:"音乐" default:"320" help:"点播音质 kbps，740 为 16bit 无损、999 为 24bit 无损；音源不支持高音质时可能获取失败，换低音质即可"`

	SendMode string `cfg:"plugin.music.send_mode" label:"发送方式" type:"select" options:"file,card,text" group:"音乐" default:"file" help:"file=下载音频并以文件发送（默认）；card=QQ 音乐卡片，发送失败自动降级为文件；text=只发播放链接"`
	MaxSizeMB int `cfg:"plugin.music.max_size_mb" label:"下载上限(MB)" group:"音乐" default:"20" help:"点播下载的音频大小上限，超过则改为发送播放链接；无损音质通常 30~80MB，需要无损请调大，0 为不限制"`
	DownloadTimeoutSec int `cfg:"plugin.music.download_timeout_sec" label:"下载超时(秒)" group:"音乐" default:"180" help:"下载音频的超时时间，30~600 秒"`

	SearchCount int `cfg:"plugin.music.search_count" label:"每页条数" group:"音乐" default:"10" help:"每次搜索返回的候选数量（1~30），也是翻页的页大小；整页结果时提示可翻页"`

	SessionMin  int `cfg:"plugin.music.session_min" label:"选歌有效期(分钟)" group:"音乐" default:"10" help:"搜索结果列表的有效期，过期后需要重新搜索"`
	CooldownSec int `cfg:"plugin.music.cooldown_sec" label:"搜索冷却(秒)" group:"音乐" default:"15" help:"同一用户两次搜索的最小间隔，0 表示不限制；点播/歌词只受全局配额约束"`

	RateLimit5Min int `cfg:"plugin.music.rate_limit_5min" label:"全局频率上限(每5分钟)" group:"音乐" default:"40" help:"全体用户 5 分钟内最多发起的 API 请求次数，官方上限 50，留出余量防止被限流"`

	LyricStripTimestamps bool `cfg:"plugin.music.lyric_strip_timestamps" label:"歌词去掉时间轴" group:"音乐" default:"true" help:"发送歌词时去掉 [00:00.00] 类时间轴与头部信息标记，更省篇幅"`
}

// ConfigSchema 声明配置结构体（面板表单 + 默认值 + Start 前自动填充）。
func (p *MusicPlugin) ConfigSchema() any { return &p.cfg }
