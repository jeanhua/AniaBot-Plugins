package eew

// eewConfig 地震预警插件的高级个性化配置结构体
type eewConfig struct {
	// 基础设置
	Enable         bool     `cfg:"plugin.eew.enable" label:"开启地震预警" group:"地震预警" default:"true"`
	Source         string   `cfg:"plugin.eew.source" label:"预警数据源" type:"select" options:"sc_eew,cenc_eew,jma_eew,fj_eew,cq_eew,all_eew" group:"地震预警" help:"预警数据提供源：sc_eew (四川省地震局，默认)、cenc_eew (中国地震台网)、jma_eew (日本气象厅)、fj_eew (福建省地震局)、cq_eew (重庆市地震局)、all_eew (全量源)" default:"sc_eew"`
	ConnectionType string   `cfg:"plugin.eew.connection_type" label:"连接模式" type:"select" options:"auto,websocket,http_polling" group:"地震预警" help:"auto: WebSocket优先，若被Cloudflare拦截(403)自动切换为HTTP轮询保底(默认); websocket: 仅WebSocket; http_polling: 仅HTTP轮询" default:"auto"`
	PollInterval   int      `cfg:"plugin.eew.poll_interval" label:"HTTP轮询间隔(秒)" group:"地震预警" help:"HTTP轮询模式下的拉取间隔，默认 2 秒" default:"2"`
	MaxAgeMinutes  int      `cfg:"plugin.eew.max_age_minutes" label:"历史预警时限(分钟)" group:"地震预警" help:"忽略超过此分钟数的历史旧预警(0表示不限制)，默认 10 分钟" default:"10"`
	MinMagnitude   float64  `cfg:"plugin.eew.min_magnitude" label:"最小推送震级" group:"地震预警" help:"低于此震级的预警将被过滤不推送" default:"3.0"`
	MinIntensity   int      `cfg:"plugin.eew.min_intensity" label:"最小预估烈度" group:"地震预警" help:"预估烈度低于此值的预警不推送(0表示不过滤)" default:"0"`
	PushStrategy   string   `cfg:"plugin.eew.push_strategy" label:"推送策略" type:"select" options:"first_and_final,all,first_only" group:"地震预警" help:"first_and_final: 仅推送首报与最终报(默认); all: 逐报全量推送; first_only: 仅推送首报" default:"first_and_final"`
	Groups         []string `cfg:"plugin.eew.groups" label:"推送群号列表" group:"地震预警" help:"接收预警推送的群 ID，每行一个（QQ为纯群号或qq:群号，其他平台如tg:-100xxx）"`
	Friends        []string `cfg:"plugin.eew.friends" label:"推送好友QQ列表" group:"地震预警" help:"接收预警推送的好友 ID，每行一个（QQ为纯QQ号或qq:QQ号，其他平台如tg:123xxx）"`

	// 气象定时播报
	WeatherCronEnable  bool     `cfg:"plugin.eew.weather_cron_enable" label:"开启气象定时播报" group:"地震预警" default:"false"`
	WeatherCron        string   `cfg:"plugin.eew.weather_cron" label:"Cron 表达式" group:"地震预警" help:"Cron 表达式，例如 '0 8,12,18 * * *' 表示每天早8点、中午12点、晚6点播报" default:"0 8,12,18 * * *"`
	WeatherCronGroups  []string `cfg:"plugin.eew.weather_cron_groups" label:"播报群号列表" group:"地震预警" help:"接收气象定时播报的群 ID，每行一个；留空则不向任何群推送"`
	WeatherCronFriends []string `cfg:"plugin.eew.weather_cron_friends" label:"播报好友QQ列表" group:"地震预警" help:"接收气象定时播报的好友 ID，每行一个；留空则不向任何好友推送"`

	// 地区关注设置
	FocusMode     string   `cfg:"plugin.eew.focus_mode" label:"地区过滤模式" type:"select" options:"all_regions,keyword_only" group:"地震预警" help:"all_regions: 推送全区域地震(默认); keyword_only: 仅推送包含指定关键词的地区" default:"all_regions"`
	FocusKeywords []string `cfg:"plugin.eew.focus_keywords" label:"关注地区关键词" group:"地震预警" help:"例如：四川,成都,宜宾。设置后若开启关键词模式，仅震中包含这些词时才推送"`

	// 本地位置与烈度测算
	LocationEnable    bool    `cfg:"plugin.eew.location_enable" label:"开启本地测算" group:"地震预警" default:"false"`
	LocationName      string  `cfg:"plugin.eew.location_name" label:"本地地名备注" group:"地震预警" help:"如 成都、北京海淀 等，显示在预警卡片中" default:"本地"`
	LocationLat       float64 `cfg:"plugin.eew.location_lat" label:"本地纬度" group:"地震预警" help:"例如 30.6586 (北纬为正，南纬为负)" default:"0"`
	LocationLng       float64 `cfg:"plugin.eew.location_lng" label:"本地经度" group:"地震预警" help:"例如 104.0648 (东经为正，西经为负)" default:"0"`
	MinLocalIntensity float64 `cfg:"plugin.eew.min_local_intensity" label:"最小本地预估烈度" group:"地震预警" help:"本地预估烈度低于此值时不推送(0表示不过滤)" default:"0"`

	// 消息样式个性化
	CustomHeader    string `cfg:"plugin.eew.custom_header" label:"卡片顶部标题" group:"地震预警" help:"自定义预警消息卡片的顶部标题" default:"🚨【地震预警】🚨"`
	ShowCoordinates bool   `cfg:"plugin.eew.show_coordinates" label:"显示震中经纬度" group:"地震预警" default:"true"`
	ShowDepth       bool   `cfg:"plugin.eew.show_depth" label:"显示震源深度" group:"地震预警" default:"true"`
	ShowIntensity   bool   `cfg:"plugin.eew.show_intensity" label:"显示震中烈度" group:"地震预警" default:"true"`
	ShowTime        bool   `cfg:"plugin.eew.show_time" label:"显示发震/发报时间" group:"地震预警" default:"true"`
	ShowEventID     bool   `cfg:"plugin.eew.show_event_id" label:"显示事件 ID" group:"地震预警" default:"false"`

	// 强震与夜间静音
	AtAll             bool    `cfg:"plugin.eew.at_all" label:"强震时 @全体成员" group:"地震预警" default:"false"`
	AtAllMinMagnitude float64 `cfg:"plugin.eew.at_all_min_magnitude" label:"@全体成员 震级门槛" group:"地震预警" help:"开启强震 @全体成员 且震级达到此门槛时，在群消息中加入 @全体成员" default:"5.5"`
	QuietHoursEnable  bool    `cfg:"plugin.eew.quiet_hours_enable" label:"开启夜间免打扰" group:"地震预警" default:"false"`
	QuietHoursStart   string  `cfg:"plugin.eew.quiet_hours_start" label:"免打扰起始时间" group:"地震预警" help:"24小时制，如 23:00" default:"23:00"`
	QuietHoursEnd     string  `cfg:"plugin.eew.quiet_hours_end" label:"免打扰结束时间" group:"地震预警" help:"24小时制，如 07:00" default:"07:00"`
	QuietMinMagnitude float64 `cfg:"plugin.eew.quiet_min_magnitude" label:"免打扰时段放行震级" group:"地震预警" help:"在免打扰时段内，震级 ≥ 此门槛的大地震仍允许强行推送" default:"5.0"`
}

// ConfigSchema 实现 plugin.ConfigSchemaProvider
func (p *EEWPlugin) ConfigSchema() any {
	return &p.cfg
}
