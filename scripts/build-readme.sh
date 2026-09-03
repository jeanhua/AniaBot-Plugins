#!/usr/bin/env bash
# 从 plugins/*/plugin.json 生成 README.md 的「插件列表」表格。
# 与 index.json 一样：CI 在合并到 main 后自动重新生成并提交，无需手动维护。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="$ROOT/README.md"
BEGIN='<!-- PLUGIN-LIST:BEGIN -->'
END='<!-- PLUGIN-LIST:END -->'

if ! grep -qF -- "$BEGIN" "$README" || ! grep -qF -- "$END" "$README"; then
  echo "错误：README.md 缺少自动生成标记（$BEGIN 与 $END）" >&2
  exit 1
fi

manifests=("$ROOT"/plugins/*/plugin.json)
if [ ! -e "${manifests[0]}" ]; then
  echo "错误：plugins/ 下没有任何插件，无法生成插件列表" >&2
  exit 1
fi

# 生成表格正文：与 index.json 同序（按插件目录名排序）
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
printf '%s\n' '| ID | 名称 | 作者 | 版本 | 说明 |' > "$tmp"
printf '%s\n' '| --- | --- | --- | --- | --- |' >> "$tmp"

sanitize() {
  # 折叠多行字段并转义 Markdown 表格分隔符（兼容 CRLF）
  tr -d '\r' | tr '\n' ' ' | sed 's/[[:space:]]*$//; s/\\/\\\\/g; s/|/\\|/g'
}

for manifest in "${manifests[@]}"; do
  id="$(jq -r '.id' "$manifest")"
  name="$(jq -r '.name' "$manifest" | sanitize)"
  author="$(jq -r '.author' "$manifest" | sanitize)"
  version="$(jq -r '.version' "$manifest" | sanitize)"
  desc="$(jq -r '.description' "$manifest" | sanitize)"
  printf '| [%s](plugins/%s) | %s | %s | %s | %s |\n' \
    "$id" "$id" "$name" "$author" "$version" "$desc" >> "$tmp"
done

# 用新表格替换两个标记之间的内容
awk -v begin="$BEGIN" -v end="$END" -v table="$tmp" '
  { sub(/\r$/, "") }
  $0 == begin {
    print
    while ((getline line < table) > 0) print line
    close(table)
    in_table = 1
    next
  }
  in_table && $0 == end { in_table = 0 }
  !in_table { print }
' "$README" > "$README.new"
mv "$README.new" "$README"

echo "README.md 插件列表已生成：$(( $(wc -l < "$tmp") - 2 )) 个插件"
