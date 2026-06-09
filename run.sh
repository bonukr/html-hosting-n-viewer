#!/usr/bin/env bash
set -euo pipefail

# 스크립트 위치를 기준으로 프로젝트 루트로 이동
cd "$(dirname "$0")"

PORT="${PORT:-8080}"
BIN="./webserver"
LOG_FILE="${LOG_FILE:-webserver.log}"
PID_FILE="${PID_FILE:-webserver.pid}"

# stop 인자 처리: 데몬 중지
if [ "${1:-}" = "stop" ]; then
  if [ -f "$PID_FILE" ]; then
    PID="$(cat "$PID_FILE")"
    echo "[run.sh] 데몬 중지 (PID=${PID})"
    sudo kill "$PID" 2>/dev/null || true
    rm -f "$PID_FILE"
  else
    echo "[run.sh] 실행 중인 데몬이 없습니다."
  fi
  exit 0
fi

# 이미 실행 중인지 확인
if [ -f "$PID_FILE" ] && sudo kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
  echo "[run.sh] 이미 실행 중입니다 (PID=$(cat "$PID_FILE")). 중지하려면: ./run.sh stop"
  exit 0
fi

echo "[run.sh] 빌드 중..."
go build -o "$BIN" .

echo "[run.sh] PORT=${PORT} 으로 백그라운드 데몬 실행합니다."
# setsid 로 새 세션을 만들어 터미널과 완전히 분리(데몬화)하고, sudo 로 80 포트 권한을 얻는다.
# sudo 는 환경변수를 초기화하므로 PORT 를 명령에 직접 전달한다.
sudo setsid env PORT="$PORT" "$BIN" >>"$LOG_FILE" 2>&1 < /dev/null &

# 래퍼(sudo/env) 가 아닌, 실제로 포트를 리스닝하는 서버 PID 를 찾아 기록한다.
SRV_PID=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  sleep 0.5
  SRV_PID="$(sudo ss -ltnpH "sport = :$PORT" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -n 1 || true)"
  [ -n "$SRV_PID" ] && break
done

if [ -n "$SRV_PID" ]; then
  echo "$SRV_PID" | sudo tee "$PID_FILE" >/dev/null
  sudo chown "$(id -un)" "$PID_FILE" 2>/dev/null || true
  echo "[run.sh] 데몬 시작됨 (PID=${SRV_PID}), 로그: ${LOG_FILE}"
else
  echo "[run.sh] 데몬 시작 실패. 로그를 확인하세요: ${LOG_FILE}"
  exit 1
fi
