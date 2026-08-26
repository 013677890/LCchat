#!/bin/sh
set -e
set -u

CONNECT_URL="${KAFKA_CONNECT_URL:-http://kafka-connect:8083}"
CONNECTOR_NAME="${DEBEZIUM_CONNECTOR_NAME:-lcchat-outbox-connector}"
BOOTSTRAP_SERVERS="${KAFKA_BROKERS:-kafka:9092}"
DB_HOST="${DEBEZIUM_MYSQL_HOST:-mysql}"
DB_PORT="${DEBEZIUM_MYSQL_PORT:-3306}"
DB_USER="${DEBEZIUM_MYSQL_USER:-debezium}"
DB_PASSWORD="${DEBEZIUM_MYSQL_PASSWORD:-debezium}"
DB_NAME="${DEBEZIUM_MYSQL_DATABASE:-chat_server}"
DB_SERVER_ID="${DEBEZIUM_MYSQL_SERVER_ID:-5401}"
TOPIC_PREFIX="${DEBEZIUM_TOPIC_PREFIX:-lcchat}"
OUTBOX_TABLE="${DEBEZIUM_OUTBOX_TABLE:-chat_server.outbox_events}"
SCHEMA_HISTORY_TOPIC="${DEBEZIUM_SCHEMA_HISTORY_TOPIC:-lcchat.schema-history}"
READY_TIMEOUT_SECONDS="${KAFKA_CONNECT_READY_TIMEOUT_SECONDS:-120}"

echo "[cdc-init] waiting for Kafka Connect: ${CONNECT_URL}"
deadline=$(( $(date +%s) + READY_TIMEOUT_SECONDS ))
while true; do
    if curl -fsS "${CONNECT_URL}/connectors" >/dev/null 2>&1; then
        break
    fi

    if [ "$(date +%s)" -ge "${deadline}" ]; then
        echo "[cdc-init] Kafka Connect not ready before timeout" >&2
        exit 1
    fi
    sleep 2
done

case "${CONNECTOR_NAME}" in
    ''|*[!A-Za-z0-9._-]*)
        echo "[cdc-init] connector name contains unsupported characters" >&2
        exit 1
        ;;
esac

# 当前 Connect worker 专用于 LCChat Outbox，只允许存在本脚本管理的唯一 connector。
# 多个 source connector 同时读取 outbox_events 会把同一 binlog 事件重复写入业务 Topic。
connectors_payload="$(curl -fsS "${CONNECT_URL}/connectors")"
compact_connectors="$(printf '%s' "${connectors_payload}" | tr -d '[:space:]')"
case "${compact_connectors}" in
    '[]'|"[\"${CONNECTOR_NAME}\"]")
        ;;
    *)
        echo "[cdc-init] unexpected connector set; expected none or only ${CONNECTOR_NAME}: ${compact_connectors}" >&2
        exit 1
        ;;
esac

cat <<EOF >/tmp/outbox-connector.json
{
  "connector.class": "io.debezium.connector.mysql.MySqlConnector",
  "tasks.max": "1",
  "database.hostname": "${DB_HOST}",
  "database.port": "${DB_PORT}",
  "database.user": "${DB_USER}",
  "database.password": "${DB_PASSWORD}",
  "database.server.id": "${DB_SERVER_ID}",
  "topic.prefix": "${TOPIC_PREFIX}",
  "database.include.list": "${DB_NAME}",
  "table.include.list": "${OUTBOX_TABLE}",
  "include.schema.changes": "false",
  "snapshot.mode": "when_needed",
  "tombstones.on.delete": "false",
  "key.converter": "org.apache.kafka.connect.storage.StringConverter",
  "value.converter": "org.apache.kafka.connect.json.JsonConverter",
  "value.converter.schemas.enable": "false",
  "schema.history.internal.kafka.bootstrap.servers": "${BOOTSTRAP_SERVERS}",
  "schema.history.internal.kafka.topic": "${SCHEMA_HISTORY_TOPIC}",
  "transforms": "outbox",
  "transforms.outbox.type": "io.debezium.transforms.outbox.EventRouter",
  "transforms.outbox.route.by.field": "event_type",
  "transforms.outbox.route.topic.replacement": "\${routedByValue}",
  "transforms.outbox.table.field.event.key": "entity_id",
  "transforms.outbox.table.field.event.payload": "payload",
  "transforms.outbox.table.expand.json.payload": "true",
  "transforms.outbox.table.field.event.id": "id",
  "transforms.outbox.table.fields.additional.placement": "event_type:header:eventType,entity_id:header:entityId,created_at:header:createdAt"
}
EOF

status_url="${CONNECT_URL}/connectors/${CONNECTOR_NAME}/status"
connector_was_stopped=false
status_probe_file="/tmp/outbox-connector-status-probe.json"
stop_response_file="/tmp/outbox-connector-stop-response.json"
status_probe_http_code="$(
    curl -sS \
      -o "${status_probe_file}" \
      -w '%{http_code}' \
      "${status_url}"
)"
case "${status_probe_http_code}" in
    200)
        # 先用 GET 明确证明 connector 存在，再调用 stop。不能把 stop 的 404
        # 猜成“不存在”：它也可能表示当前 Connect 没有暴露固定版本所需的 stop API。
        # 这种环境必须启动失败，禁止绕过 STOPPED 屏障做向前/向后兼容。
        stop_http_code="$(
            curl -sS \
              -o "${stop_response_file}" \
              -w '%{http_code}' \
              -X PUT \
              "${CONNECT_URL}/connectors/${CONNECTOR_NAME}/stop"
        )"
        case "${stop_http_code}" in
            200|202|204)
                ;;
            *)
                echo "[cdc-init] failed to stop existing connector; HTTP ${stop_http_code}" >&2
                if [ -s "${stop_response_file}" ]; then
                    cat "${stop_response_file}" >&2
                    echo >&2
                fi
                exit 1
                ;;
        esac
        connector_was_stopped=true
        echo "[cdc-init] existing connector found; waiting for STOPPED barrier"
        ;;
    404)
        echo "[cdc-init] connector does not exist yet; creating a new one"
        ;;
    *)
        echo "[cdc-init] failed to determine whether connector exists; HTTP ${status_probe_http_code}" >&2
        if [ -s "${status_probe_file}" ]; then
            cat "${status_probe_file}" >&2
            echo >&2
        fi
        exit 1
        ;;
esac

if [ "${connector_was_stopped}" = "true" ]; then
    # Kafka Connect 通过 status topic 异步传播状态。必须先观察到 STOPPED 且 tasks
    # 已清空，才能证明旧 generation 已经退出；否则 PUT 后立即读到的 RUNNING
    # 可能仍属于旧配置，并在新 task 随后 FAILED 前错误放行业务服务。
    deadline=$(( $(date +%s) + READY_TIMEOUT_SECONDS ))
    while true; do
        status_payload=""
        if status_payload="$(curl -fsS "${status_url}")"; then
            compact_status="$(printf '%s' "${status_payload}" | tr -d '[:space:]')"
            stopped_state_count="$(printf '%s' "${compact_status}" |
                awk -F '"state":"STOPPED"' '{print NF - 1}')"
            case "${compact_status}" in
                *'"tasks":[]'*)
                    if [ "${stopped_state_count}" -eq 1 ]; then
                        echo "[cdc-init] existing connector reached STOPPED barrier"
                        break
                    fi
                    ;;
            esac
        fi

        if [ "$(date +%s)" -ge "${deadline}" ]; then
            echo "[cdc-init] connector did not reach STOPPED before timeout; last status: ${status_payload}" >&2
            exit 1
        fi
        sleep 2
    done
fi

echo "[cdc-init] registering connector: ${CONNECTOR_NAME}"
curl -fsS -X PUT \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  --data @/tmp/outbox-connector.json \
  "${CONNECT_URL}/connectors/${CONNECTOR_NAME}/config" >/dev/null

if [ "${connector_was_stopped}" = "true" ]; then
    # 更新 stopped connector 的配置后显式恢复；resume 对 STOPPED 是当前固定契约，
    # 不通过 restart/pause 等旧语义做兼容猜测。
    curl -fsS -X PUT "${CONNECT_URL}/connectors/${CONNECTOR_NAME}/resume" >/dev/null
fi

echo
echo "[cdc-init] connector registered successfully"
deadline=$(( $(date +%s) + READY_TIMEOUT_SECONDS ))
while true; do
    status_payload=""
    if status_payload="$(curl -fsS "${status_url}")"; then
        # Kafka Connect 的 /status 在 connector/task 已 FAILED 时仍返回 HTTP 200，
        # 因此不能把 curl 成功当成 CDC 可用。去掉结构空白后显式检查 connector
        # 与 tasks.max=1 对应的 task-0；严格 v2 依赖 connector 真正 RUNNING，
        # 否则 expand.json.payload 没有生效却会让所有业务服务继续启动。
        compact_status="$(printf '%s' "${status_payload}" | tr -d '[:space:]')"
        case "${compact_status}" in
            *'"state":"FAILED"'*)
                echo "[cdc-init] connector or task entered FAILED state: ${status_payload}" >&2
                exit 1
                ;;
        esac
        # JSON Object 的字段顺序不属于 Kafka Connect REST 契约，不能假设 state
        # 一定紧跟在 connector 或 task id 后面。tasks.max 固定为 1，因此合法就绪
        # 状态必须恰好出现两个 RUNNING state（connector + task-0），同时包含 task id 0。
        # 这种判断不依赖对象字段顺序，也不会把只有 connector RUNNING、task 尚未创建
        # 的中间状态误报为成功。
        running_state_count="$(printf '%s' "${compact_status}" |
            awk -F '"state":"RUNNING"' '{print NF - 1}')"
        has_connector=false
        has_task_zero=false
        case "${compact_status}" in
            *'"connector":{'*) has_connector=true ;;
        esac
        case "${compact_status}" in
            *'"id":0'*) has_task_zero=true ;;
        esac
        if [ "${has_connector}" = "true" ] &&
           [ "${has_task_zero}" = "true" ] &&
           [ "${running_state_count}" -eq 2 ]; then
            printf '%s\n' "${status_payload}"
            echo "[cdc-init] connector and all configured tasks are RUNNING"
            break
        fi
    fi

    if [ "$(date +%s)" -ge "${deadline}" ]; then
        echo "[cdc-init] connector/tasks not RUNNING before timeout; last status: ${status_payload}" >&2
        exit 1
    fi
    sleep 2
done
