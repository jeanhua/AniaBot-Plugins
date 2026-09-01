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

## 提交插件

1. Fork 本仓库
2. 在 `plugins/` 下新建 `<plugin-id>/` 目录，包含 `plugin.json`、`README.md` 与源码（格式见 [docs/plugin-spec.md](docs/plugin-spec.md)，可参考 [example](plugins/example)）
3. 运行 `bash scripts/validate.sh`（需本地有 Go 1.25+ 与 AniaBot 源码，或直接依赖 CI）
4. 提交 Pull Request，CI 会自动校验；维护者人工审查后会合并

提交前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 本地开发

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

## 安全与信任模型

**安装插件 = 在 Bot 所在机器上执行插件代码（与 Bot 同进程）**。请只安装你信任的插件。本仓库所有插件都经过维护者人工审查（重点：网络请求、文件读写、进程执行、凭据访问、第三方依赖），但无法保证第三方依赖与未来版本绝对安全。面板安装时会再次提示风险，默认情况下插件市场功能是关闭的。
