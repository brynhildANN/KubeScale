#!/usr/bin/env bash
# 本地开发入口；配置文件是可信的 Bash 环境变量文件。
set -euo pipefail
set +m

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
usage() {
  cat <<'HELP'
用法：scripts/dev.sh [--env 文件]
  检查集群与 Prometheus，启动后端和前端
  --env  Bash 配置文件；默认 backend/.env.local（存在时读取）
环境变量：KUBESCALE_FRONTEND_PORT（默认 5173）及后端 KUBESCALE_* 配置。
按 Ctrl+C 停止本次启动的所有进程。需要 Bash 4.3+ 与 Linux setsid。
HELP
}
fail() { printf '错误：%s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "缺少 $1，请先安装。"; }
env_file="$ROOT/backend/.env.local"
explicit_env=false
while (($#)); do
  case "$1" in
    --env) (($# >= 2)) || fail '--env 需要文件路径'; env_file=$2; explicit_env=true; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; fail "未知参数：$1" ;;
  esac
done
((BASH_VERSINFO[0] > 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] >= 3))) || fail '需要 Bash 4.3+'
if [[ -f $env_file ]]; then
  set -a
  source "$(cd -- "$(dirname -- "$env_file")" && pwd)/$(basename -- "$env_file")"
  set +a
elif [[ $explicit_env == true ]]; then
  fail '指定的配置文件不存在'
fi
for cmd in node pnpm curl setsid; do need "$cmd"; done
[[ -d $ROOT/frontend/node_modules ]] || fail '前端依赖未安装，请先在 frontend 目录运行 pnpm install'
frontend_port=${KUBESCALE_FRONTEND_PORT:-5173}
check_port() {
  node --input-type=module - "$1" "$2" <<'JS'
import net from 'node:net';
const [host, rawPort] = process.argv.slice(2);
const port = Number(rawPort);
if (!/^\d+$/.test(rawPort) || port < 1 || port > 65535) {
  console.error('错误：端口必须是 1～65535 的整数'); process.exit(1);
}
const server = net.createServer();
server.once('error', () => { console.error(`错误：无法监听 ${host}:${port}，请检查地址或端口占用`); process.exit(1); });
server.listen(port, host, () => server.close());
JS
}
check_port 127.0.0.1 "$frontend_port"
for cmd in go kubectl; do need "$cmd"; done
namespaces=${KUBESCALE_NAMESPACES:-}
[[ -n ${namespaces//[[:space:],]/} ]] || fail '请设置 KUBESCALE_NAMESPACES'
export KUBESCALE_LISTEN=${KUBESCALE_LISTEN:-127.0.0.1:8080}
export KUBESCALE_PROMETHEUS_URL=${KUBESCALE_PROMETHEUS_URL:-http://127.0.0.1:9090}
# 开发入口固定回环监听，避免意外公开管理接口。
[[ $KUBESCALE_LISTEN == 127.0.0.1:* ]] || fail '开发脚本要求 KUBESCALE_LISTEN=127.0.0.1:<端口>'
backend_port=${KUBESCALE_LISTEN##*:}
[[ $backend_port != "$frontend_port" ]] || fail '前后端端口不能相同'
check_port 127.0.0.1 "$backend_port"
kubectl --request-timeout=8s get --raw=/version >/dev/null || fail '无法连接 Kubernetes，请检查 kubeconfig 与当前 context'
curl --noproxy '*' --fail --silent --show-error --connect-timeout 3 --max-time 8 "${KUBESCALE_PROMETHEUS_URL%/}/-/ready" >/dev/null || fail 'Prometheus 未就绪，请检查地址或先手动 port-forward'

pids=()
work_dir=''
cleanup() {
  trap '' INT TERM
  local pid
  for pid in "${pids[@]}"; do kill -TERM -- "-$pid" 2>/dev/null || true; done
  # 给服务留出退出时间，再清理未退出的子进程。
  for ((i=0; i<30; i++)); do
    local alive=false
    for pid in "${pids[@]}"; do kill -0 -- "-$pid" 2>/dev/null && alive=true; done
    [[ $alive == true ]] || break
    sleep 0.1
  done
  for pid in "${pids[@]}"; do kill -KILL -- "-$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; done
  [[ -z $work_dir ]] || rm -rf -- "$work_dir"
  return 0
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
wait_ready() {
  local pid=$1 url=$2 name=$3
  for ((i=0; i<60; i++)); do
    kill -0 "$pid" 2>/dev/null || fail "$name 已退出，请查看上方日志"
    if curl --noproxy '*' --fail --silent --connect-timeout 1 --max-time 1 "$url" >/dev/null; then return; fi
    sleep 0.5
  done
  fail "$name 启动超时，请查看上方日志"
}
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/kubescale-dev.XXXXXX")
printf '编译后端…\n'
(cd "$ROOT/backend" && go build -o "$work_dir/kubescale" ./cmd/kubescale)
cd "$ROOT/backend"
setsid "$work_dir/kubescale" &
pids+=("$!")
wait_ready "${pids[0]}" "http://$KUBESCALE_LISTEN/healthz" 后端
export KUBESCALE_DEV_BACKEND="http://$KUBESCALE_LISTEN"
# 覆盖 Vite 本地配置，确保前端经当前后端代理访问。
export VITE_API_BASE_URL=/api/v1 VITE_ENABLE_SSE=false
cd "$ROOT/frontend"
setsid pnpm dev --host 127.0.0.1 --port "$frontend_port" --strictPort &
pids+=("$!")
wait_ready "${pids[${#pids[@]}-1]}" "http://127.0.0.1:$frontend_port/" 前端
printf '\nKubeScale 已启动：http://127.0.0.1:%s\n按 Ctrl+C 停止。\n' "$frontend_port"
# 任一服务退出都停止其余服务；异常退出保留退出码。
status=0
wait -n "${pids[@]}" || status=$?
exit "$status"
