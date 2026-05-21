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
  "schema.history.internal.kafka.bootstrap.servers": "${BOOTSTRAP_SERVERS}",
  "schema.history.internal.kafka.topic": "${SCHEMA_HISTORY_TOPIC}",
  "transforms": "outbox",
  "transforms.outbox.type": "io.debezium.transforms.outbox.EventRouter",
  "transforms.outbox.route.by.field": "event_type",
  "transforms.outbox.route.topic.replacement": "\${routedByValue}",
  "transforms.outbox.table.field.event.key": "entity_id",
  "transforms.outbox.table.field.event.payload": "payload",
  "transforms.outbox.table.field.event.id": "id",
  "transforms.outbox.table.fields.additional.placement": "event_type:header:eventType,entity_id:header:entityId,created_at:header:createdAt"
}
EOF

echo "[cdc-init] registering connector: ${CONNECTOR_NAME}"
curl -fsS -X PUT \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  --data @/tmp/outbox-connector.json \
  "${CONNECT_URL}/connectors/${CONNECTOR_NAME}/config"

echo
echo "[cdc-init] connector registered successfully"
status_url="${CONNECT_URL}/connectors/${CONNECTOR_NAME}/status"
deadline=$(( $(date +%s) + READY_TIMEOUT_SECONDS ))
while true; do
    if curl -fsS "${status_url}"; then
        echo
        break
    fi

    if [ "$(date +%s)" -ge "${deadline}" ]; then
        echo "[cdc-init] connector status not ready before timeout" >&2
        exit 1
    fi
    sleep 2
done
