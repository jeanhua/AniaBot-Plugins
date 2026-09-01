#!/usr/bin/env bash
# 从 plugins/*/plugin.json 生成 index.json（聚合索引，供插件市场低配额读取）。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/index.json"

manifests=("$ROOT"/plugins/*/plugin.json)
if [ ! -e "${manifests[0]}" ]; then
  echo '{"plugins":[]}' > "$OUT"
  echo "index.json 已生成（无插件）"
  exit 0
fi

jq -s '{plugins: map({
  id: .id,
  name: .name,
  description: .description,
  author: .author,
  version: .version,
  platforms: (.platforms // []),
  tags: (.tags // []),
  api_version: (.api_version // 1),
  min_framework: (.min_framework // ""),
  readme: (.readme // "README.md"),
  icon: (.icon // "")
})}' "${manifests[@]}" > "$OUT"
echo "index.json 已生成：$(jq '.plugins | length' "$OUT") 个插件"
