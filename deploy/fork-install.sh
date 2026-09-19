#!/usr/bin/env bash
# 一键安装 / 更新本 fork（yuanzc188/3x-ui，含端口转发二开）。
#
#   首次安装 / 更新到最新 Release：
#     bash <(curl -Ls https://raw.githubusercontent.com/yuanzc188/3x-ui/main/deploy/fork-install.sh)
#   安装指定版本：
#     bash <(curl -Ls https://raw.githubusercontent.com/yuanzc188/3x-ui/main/deploy/fork-install.sh) v3.8.5-fw1
#
# 原理：下载本 fork 的上游 install.sh（1600 行、久经考验：依赖、systemd、证书、
# 保留 /etc/x-ui 数据库与 bin/ 自定义文件），只把下载源改成本 fork 的 Release，
# 因为 fork 的 Release 是 prerelease，"latest" 解析不到，所以这里自己解析 tag。
#
# 更新：重新执行本脚本即可。不要用面板自带的 `x-ui update`，那会装回上游版本。
#
# 可选环境变量（透传给 install.sh 的无人值守模式）：
#   XUI_USERNAME XUI_PASSWORD XUI_PANEL_PORT XUI_WEB_BASE_PATH
#   XUI_SSL_MODE=none|ip|domain XUI_DOMAIN XUI_ACME_EMAIL
set -euo pipefail

REPO="${XUI_FORK_REPO:-yuanzc188/3x-ui}"
REF="${XUI_FORK_REF:-main}"
API="https://api.github.com/repos/${REPO}"

[[ $EUID -eq 0 ]] || { echo "请用 root 运行"; exit 1; }
command -v curl >/dev/null || { apt-get update -qq && apt-get install -y -qq curl; }

tag="${1:-}"
if [[ -z "$tag" ]]; then
    # 取最新一个已发布的 Release（含 prerelease）
    tag=$(curl -fsSL --retry 3 "${API}/releases?per_page=1" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
    [[ -n "$tag" ]] || { echo "解析不到 ${REPO} 的 Release，请确认 GitHub Actions 已构建完成"; exit 1; }
fi
echo "==> 安装 ${REPO} ${tag}"

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
curl -fsSL --retry 3 "https://raw.githubusercontent.com/${REPO}/${REF}/install.sh" -o "$tmp"
sed -i "s#MHSanaei/3x-ui#${REPO}#g" "$tmp"
bash "$tmp" "$tag"

# 管理脚本里的仓库地址也指向 fork（x-ui.sh 从同一 tag 下载，内容还是上游的）
for f in /usr/local/x-ui/x-ui.sh /usr/bin/x-ui; do
    [[ -f "$f" ]] && sed -i "s#MHSanaei/3x-ui#${REPO}#g" "$f" || true
done

echo
echo "==> 完成。面板信息：x-ui settings   |   更新：重新执行本脚本"
