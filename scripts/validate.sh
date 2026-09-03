#!/usr/bin/env bash
# 插件仓库校验：plugin.json 合法性 + 生成 index.json / README 插件列表 + 可选编译校验。
#
# 用法：
#   bash scripts/validate.sh                     # 只做元信息校验
#   ANIA_SRC=/path/to/AniaBot bash scripts/validate.sh   # 额外编译校验
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

command -v jq >/dev/null 2>&1 || { echo "错误：需要 jq"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "错误：需要 Go"; exit 1; }

echo "== 1. 校验 plugin.json =="
found=0

# 校验单个插件目录（plugins/ 与 examples/ 共用同一套格式要求；
# examples/ 仅作为开发示例，不进入插件市场索引与列表）。
check_manifest() {
  local manifest="$1"
  local dir id
  dir="$(dirname "$manifest")"
  id="$(basename "$dir")"
  if ! jq -e --arg id "$id" '
    (.id == $id)
    and (.id | test("^[a-z0-9_-]{2,64}$"))
    and ((.name | type) == "string" and (.name | length) > 0)
    and ((.description | type) == "string" and (.description | length) > 0)
    and ((.author | type) == "string" and (.author | length) > 0)
    and ((.version | type) == "string" and (.version | length) > 0)
  ' "$manifest" >/dev/null; then
    echo "校验失败：$manifest（id 须与目录名一致，name/description/author/version 必填）"
    exit 1
  fi
  [ -f "$dir/README.md" ] || { echo "校验失败：$dir 缺少 README.md"; exit 1; }
  if ! ls "$dir"/*.go >/dev/null 2>&1; then
    echo "校验失败：$dir 缺少 Go 源码"; exit 1
  fi
  echo "  ok: $id"
}

for manifest in "$ROOT"/plugins/*/plugin.json; do
  [ -e "$manifest" ] || continue
  found=1
  check_manifest "$manifest"
done
# 示例插件（examples/）不进市场，但同样校验格式并参与编译，避免教程代码失效
for manifest in "$ROOT"/examples/*/plugin.json; do
  [ -e "$manifest" ] || continue
  id="$(basename "$(dirname "$manifest")")"
  if [ -d "$ROOT/plugins/$id" ]; then
    echo "校验失败：examples/$id 与市场插件 plugins/$id 同名，示例插件不能占用已在市场的插件 id"
    exit 1
  fi
  found=1
  check_manifest "$manifest"
done
[ "$found" -eq 1 ] || { echo "未找到任何插件"; exit 1; }

echo "== 2. 生成 index.json 与 README 插件列表 =="
bash "$ROOT/scripts/build-index.sh"
bash "$ROOT/scripts/build-readme.sh"

SRC="${ANIA_SRC:-}"
if [ -n "$SRC" ] && [ -d "$SRC" ]; then
  echo "== 3. 编译校验（AniaBot 源码：$SRC）=="
  mkdir -p "$SRC/custom/plugins"
  # 市场插件
  for manifest in "$ROOT"/plugins/*/plugin.json; do
    [ -e "$manifest" ] || continue
    id="$(basename "$(dirname "$manifest")")"
    rm -rf "$SRC/custom/plugins/$id"
    cp -r "$ROOT/plugins/$id" "$SRC/custom/plugins/$id"
  done
  # 示例插件：与市场插件同样编译，保证示例代码与当前框架 API 保持可用
  for manifest in "$ROOT"/examples/*/plugin.json; do
    [ -e "$manifest" ] || continue
    id="$(basename "$(dirname "$manifest")")"
    rm -rf "$SRC/custom/plugins/$id"
    cp -r "$ROOT/examples/$id" "$SRC/custom/plugins/$id"
  done
  (
    cd "$SRC"
    go mod tidy
    go vet ./custom/plugins/...
    go build ./custom/plugins/...
  )
else
  echo "== 3. 未提供 ANIA_SRC，跳过编译校验 =="
fi

echo "== 4. 检查自动生成文件是否已提交（git 仓库中执行）=="
if [ -d "$ROOT/.git" ]; then
  if ! (cd "$ROOT" && git diff --quiet -- index.json README.md); then
    if [ "${ALLOW_AUTOGEN_DIFF:-0}" = "1" ]; then
      echo "提示：index.json / README 插件列表有差异，无需手动处理——合并到 main 后 CI 会自动重新生成并提交"
    else
      echo "index.json / README 插件列表与插件不一致，请运行 bash scripts/build-index.sh && bash scripts/build-readme.sh 后提交变更"
      exit 1
    fi
  fi
fi
echo "校验通过"
