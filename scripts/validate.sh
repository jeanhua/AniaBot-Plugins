#!/usr/bin/env bash
# 插件仓库校验：plugin.json 合法性 + index.json 生成 + 可选编译校验。
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
for manifest in "$ROOT"/plugins/*/plugin.json; do
  [ -e "$manifest" ] || continue
  found=1
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
done
[ "$found" -eq 1 ] || { echo "未找到任何插件"; exit 1; }

echo "== 2. 生成 index.json =="
bash "$ROOT/scripts/build-index.sh"

SRC="${ANIA_SRC:-}"
if [ -n "$SRC" ] && [ -d "$SRC" ]; then
  echo "== 3. 编译校验（AniaBot 源码：$SRC）=="
  mkdir -p "$SRC/custom/plugins"
  for manifest in "$ROOT"/plugins/*/plugin.json; do
    [ -e "$manifest" ] || continue
    id="$(basename "$(dirname "$manifest")")"
    rm -rf "$SRC/custom/plugins/$id"
    cp -r "$ROOT/plugins/$id" "$SRC/custom/plugins/$id"
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

echo "== 4. 检查 index.json 是否已提交（git 仓库中执行）=="
if [ -d "$ROOT/.git" ]; then
  if ! (cd "$ROOT" && git diff --quiet -- index.json); then
    echo "index.json 与插件不一致，请运行 bash scripts/build-index.sh 后提交变更"
    exit 1
  fi
fi
echo "校验通过"
