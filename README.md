# AniaBot-Plugins

AniaBot 的官方插件市场仓库。

本仓库只存放**插件源码与元信息**，不包含 AniaBot 框架代码。插件通过 AniaBot 面板的「插件市场」在线安装（下载源码 → 编译 → 重启），也可以手动克隆到本地 AniaBot 源码树的 `custom/plugins/<id>` 目录使用。

## 目录结构

```
plugins/
  <plugin-id>/
    plugin.json   # 插件元信息（必填）
    README.md     # 插件介绍（必填）
    *.go          # 插件源码（必填，不带 go.mod）
docs/
  plugin-spec.md  # plugin.json 规范
scripts/
  validate.sh     # 本地校验脚本（与 CI 一致）
index.json        # 聚合索引（由 scripts/build-index.sh 生成，CI 在合并到 main 后自动同步，无需手改）
```

## 插件列表

| ID | 名称 | 作者 | 版本 | 说明 |
| --- | --- | --- | --- | --- |
| [example](plugins/example) | 示例插件 | AniaBot | 1.0.0 | 插件开发入门示例 |
| [groupdigest](plugins/groupdigest) | 群刊 | jeanhua | 1.3.1 | 群消息达到阈值后自动用 AI 生成群刊，可发送 Markdown 文件或渲染图片 |
| [dicegirl](plugins/dicegirl) | 骰娘 | jeanhua | 1.0.0 | TRPG 骰娘：掷骰表达式 /r、COC 7e 技能检定 /ra、理智检定 /sc、今日人品 /jrrp |
| [antiwithdrawal](plugins/antiwithdrawal) | 防撤回 | jeanhua | 1.0.0 | QQ 群防撤回：缓存群消息并以合并转发回顾 /explore，撤回也能看 |

## 提交插件

1. Fork 本仓库
2. 在 `plugins/` 下新建 `<plugin-id>/` 目录，包含 `plugin.json`、`README.md` 与源码（格式见 [docs/plugin-spec.md](docs/plugin-spec.md)，可参考 [example](plugins/example)）
3. 运行 `bash scripts/validate.sh`（需本地有 Go 1.25+ 与 AniaBot 源码，或直接依赖 CI）
4. 提交 Pull Request，CI 会自动校验；维护者人工审查后会合并

提交前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 本地开发

> 仓库根目录的 `go.mod` / `go.sum` 仅用于**本地开发与 IDE 解析**（让 gopls 能识别
> `github.com/jeanhua/AniaBot/...` 框架包，消除编辑器里的导入报错），**不参与插件安装与 CI**：
> 市场安装只解压 `plugins/<id>/`，`scripts/validate.sh` 也是在 AniaBot 源码树内编译，
> 两者都不会读取这两个文件。插件本身依然不带自己的 go.mod。

```bash
# 0.（IDE 支持，可选但推荐）两个仓库放到同一父目录，直接打开 AniaBot-Plugins 即可：
#    ../AniaBot  ← go.mod 的 replace 指向这里
#    ../AniaBot-Plugins
#    在插件仓库根执行 go build ./... / go test ./... 可独立校验全部插件

# 1. 克隆 AniaBot 与本仓库
git clone https://github.com/jeanhua/AniaBot.git
git clone https://github.com/jeanhua/AniaBot-Plugins.git

# 2. 把正在开发的插件软链/复制到 AniaBot 源码树
cp -r AniaBot-Plugins/plugins/my-plugin AniaBot/custom/plugins/my-plugin

# 3. 在 AniaBot 中生成注册代码并运行
cd AniaBot
go run ./tools/plugingen
go run cmd/main.go
```

```bash
# 1. 克隆 AniaBot 与本仓库
git clone https://github.com/jeanhua/AniaBot.git
git clone https://github.com/jeanhua/AniaBot-Plugins.git

# 2. 把正在开发的插件软链/复制到 AniaBot 源码树
cp -r AniaBot-Plugins/plugins/my-plugin AniaBot/custom/plugins/my-plugin

# 3. 在 AniaBot 中生成注册代码并运行
cd AniaBot
go run ./tools/plugingen
go run cmd/main.go
```

## 相关链接

| 链接 | 说明 |
| --- | --- |
| [AniaBot 框架仓库](https://github.com/jeanhua/AniaBot) | AniaBot 主仓库（框架代码、插件市场功能） |
| [AniaBot 文档站点](https://jeanhua.github.io/AniaBot/) | AniaBot 完整文档 |
| [插件系统概览](https://jeanhua.github.io/AniaBot/plugin/overview) | 插件开发入门：插件如何加载与执行 |
| [第一个插件](https://jeanhua.github.io/AniaBot/plugin/first-plugin) | 从零开发第一个插件 |
| [插件开发教程](https://jeanhua.github.io/AniaBot/plugin/tutorial) | 完整插件开发实战 |
| [插件市场使用指南](https://jeanhua.github.io/AniaBot/guide/plugin-marketplace) | 面板在线安装 / 升级 / 卸载插件 |
| [插件规范（本仓库）](docs/plugin-spec.md) | plugin.json 元信息规范 |

开发插件前，建议先阅读 AniaBot 的[插件开发文档](https://jeanhua.github.io/AniaBot/plugin/overview)，了解 `plugin.Meta`、消息事件、`msgchain` 消息构造器等基础 API，再参考本仓库的 [example](plugins/example) 示例。

## 安全与信任模型

**安装插件 = 在 Bot 所在机器上执行插件代码（与 Bot 同进程）**。请只安装你信任的插件。本仓库所有插件都经过维护者人工审查（重点：网络请求、文件读写、进程执行、凭据访问、第三方依赖），但无法保证第三方依赖与未来版本绝对安全。面板安装时会再次提示风险，默认情况下插件市场功能是关闭的。
