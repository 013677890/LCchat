import base64
import json
import os
import subprocess
import sys
import tempfile
import time
import uuid
from dataclasses import dataclass
from pathlib import Path

import requests
import websocket

ROOT = Path(__file__).resolve().parent.parent
BASE_URL = os.getenv("LCCHAT_BASE_URL", "http://127.0.0.1:8088")
HOST_HEADER = os.getenv("LCCHAT_HOST_HEADER", "lcchat.local")
REQUEST_TIMEOUT = int(os.getenv("LCCHAT_REQUEST_TIMEOUT", "10"))
RATE_LIMIT_RETRY_COUNT = int(os.getenv("LCCHAT_RATE_LIMIT_RETRY_COUNT", "6"))
RATE_LIMIT_RETRY_BASE_DELAY = float(os.getenv("LCCHAT_RATE_LIMIT_RETRY_BASE_DELAY", "0.25"))

EXPECTED_ENDPOINTS = {
    ("GET", "/health"),
    ("GET", "/metrics"),
    ("POST", "/api/v1/public/user/login"),
    ("POST", "/api/v1/public/user/login-by-code"),
    ("POST", "/api/v1/public/user/register"),
    ("POST", "/api/v1/public/user/send-verify-code"),
    ("POST", "/api/v1/public/user/reset-password"),
    ("POST", "/api/v1/public/user/refresh-token"),
    ("POST", "/api/v1/public/user/verify-code"),
    ("POST", "/api/v1/public/user/parse-qrcode"),
    ("GET", "/api/v1/auth/user/profile"),
    ("PUT", "/api/v1/auth/user/profile"),
    ("GET", "/api/v1/auth/user/profile/:userUuid"),
    ("GET", "/api/v1/auth/user/search"),
    ("POST", "/api/v1/auth/user/avatar"),
    ("GET", "/api/v1/auth/user/qrcode"),
    ("POST", "/api/v1/auth/user/batch-profile"),
    ("GET", "/api/v1/auth/user/devices"),
    ("DELETE", "/api/v1/auth/user/devices/:deviceId"),
    ("GET", "/api/v1/auth/user/online-status/:userUuid"),
    ("POST", "/api/v1/auth/user/batch-online-status"),
    ("POST", "/api/v1/auth/user/change-password"),
    ("POST", "/api/v1/auth/user/change-email"),
    ("POST", "/api/v1/auth/user/delete-account"),
    ("POST", "/api/v1/auth/user/logout"),
    ("POST", "/api/v1/auth/friend/apply"),
    ("GET", "/api/v1/auth/friend/apply-list"),
    ("GET", "/api/v1/auth/friend/apply/sent"),
    ("POST", "/api/v1/auth/friend/apply/handle"),
    ("GET", "/api/v1/auth/friend/apply/unread"),
    ("POST", "/api/v1/auth/friend/apply/read"),
    ("GET", "/api/v1/auth/friend/list"),
    ("POST", "/api/v1/auth/friend/sync"),
    ("POST", "/api/v1/auth/friend/delete"),
    ("POST", "/api/v1/auth/friend/remark"),
    ("POST", "/api/v1/auth/friend/tag"),
    ("POST", "/api/v1/auth/friend/check"),
    ("POST", "/api/v1/auth/friend/relation"),
    ("POST", "/api/v1/auth/blacklist"),
    ("GET", "/api/v1/auth/blacklist"),
    ("DELETE", "/api/v1/auth/blacklist/:userUuid"),
    ("POST", "/api/v1/auth/blacklist/check"),
    ("POST", "/api/v1/auth/messages/send"),
    ("GET", "/api/v1/auth/messages/pull"),
    ("POST", "/api/v1/auth/messages/get-by-ids"),
    ("POST", "/api/v1/auth/messages/recall"),
    ("GET", "/api/v1/auth/conversations"),
    ("POST", "/api/v1/auth/conversations/mark-read"),
    ("DELETE", "/api/v1/auth/conversations/:convId"),
    ("PATCH", "/api/v1/auth/conversations/settings"),
    ("POST", "/api/v1/auth/groups"),
    ("GET", "/api/v1/auth/groups"),
    ("GET", "/api/v1/auth/groups/search"),
    ("GET", "/api/v1/auth/groups/join-applications"),
    ("GET", "/api/v1/auth/groups/:groupUuid"),
    ("PATCH", "/api/v1/auth/groups/:groupUuid"),
    ("PUT", "/api/v1/auth/groups/:groupUuid/notice"),
    ("POST", "/api/v1/auth/groups/:groupUuid/apply"),
    ("DELETE", "/api/v1/auth/groups/:groupUuid/apply"),
    ("GET", "/api/v1/auth/groups/:groupUuid/my-join-application"),
    ("GET", "/api/v1/auth/groups/:groupUuid/join-requests"),
    ("GET", "/api/v1/auth/groups/:groupUuid/join-requests/pending-count"),
    ("GET", "/api/v1/auth/groups/:groupUuid/join-requests/reviewed"),
    ("POST", "/api/v1/auth/groups/:groupUuid/join-requests/:applyId/review"),
    ("POST", "/api/v1/auth/groups/:groupUuid/transfer-owner"),
    ("POST", "/api/v1/auth/groups/:groupUuid/leave"),
    ("PATCH", "/api/v1/auth/groups/:groupUuid/my-nickname"),
    ("PATCH", "/api/v1/auth/groups/:groupUuid/mute-setting"),
    ("DELETE", "/api/v1/auth/groups/:groupUuid"),
    ("POST", "/api/v1/auth/groups/:groupUuid/members"),
    ("GET", "/api/v1/auth/groups/:groupUuid/members/search"),
    ("GET", "/api/v1/auth/groups/:groupUuid/members"),
    ("DELETE", "/api/v1/auth/groups/:groupUuid/members/:userUuid"),
    ("PATCH", "/api/v1/auth/groups/:groupUuid/members/:userUuid/nickname"),
    ("PATCH", "/api/v1/auth/groups/:groupUuid/members/:userUuid/mute"),
    ("PATCH", "/api/v1/auth/groups/:groupUuid/members/:userUuid/role"),
    ("GET", "/api/v1/auth/groups/:groupUuid/member-ids"),
}

COVERED_ENDPOINTS: set[tuple[str, str]] = set()
CONNECT_EXPECTED_ENDPOINTS = {
    ("GET", "/health"),
    ("GET", "/metrics"),
    ("GET", "/ws"),
}
CONNECT_COVERED_ENDPOINTS: set[tuple[str, str]] = set()

CONNECT_HTTP_BASE_URL = os.getenv("LCCHAT_CONNECT_HTTP_BASE_URL", "http://127.0.0.1:8081")
CONNECT_WS_BASE_URL = os.getenv("LCCHAT_CONNECT_WS_BASE_URL", "ws://127.0.0.1:8081")
CONNECT_WS_ORIGIN = os.getenv("LCCHAT_CONNECT_WS_ORIGIN", "http://127.0.0.1")
VALID_PNG_BYTES = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg=="
)

class TestFailure(RuntimeError):
    pass

@dataclass
class ResultItem:
    name: str
    status: str
    detail: str

class Recorder:
    def __init__(self) -> None:
        self.items: list[ResultItem] = []

    def ok(self, name: str, detail: str = "") -> None:
        self.items.append(ResultItem(name, "PASS", detail))
        print(f"[PASS] {name}: {detail}")

    def warn(self, name: str, detail: str = "") -> None:
        self.items.append(ResultItem(name, "WARN", detail))
        print(f"[WARN] {name}: {detail}")

    def fail(self, name: str, detail: str = "") -> None:
        self.items.append(ResultItem(name, "FAIL", detail))
        print(f"[FAIL] {name}: {detail}")

    def summary(self) -> tuple[int, int, int]:
        passed = sum(1 for item in self.items if item.status == "PASS")
        warned = sum(1 for item in self.items if item.status == "WARN")
        failed = sum(1 for item in self.items if item.status == "FAIL")
        return passed, warned, failed

def mark_endpoint(method: str, path: str) -> None:
    COVERED_ENDPOINTS.add((method.upper(), path))

def run_cmd(args: list[str]) -> str:
    proc = subprocess.run(
        args,
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise TestFailure(
            f"command failed: {' '.join(args)}\nstdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
        )
    return proc.stdout.strip()

def redis_set(key: str, value: str, expire_seconds: int = 600) -> None:
    run_cmd(
        [
            "docker",
            "compose",
            "exec",
            "-T",
            "redis",
            "redis-cli",
            "SET",
            key,
            value,
            "EX",
            str(expire_seconds),
        ]
    )


def build_headers(*, token: str | None = None, device_id: str | None = None) -> dict[str, str]:
    headers: dict[str, str] = {}
    if HOST_HEADER:
        headers["Host"] = HOST_HEADER
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if device_id:
        headers["X-Device-ID"] = device_id
    return headers

def request_json(
    method: str,
    path: str,
    *,
    route_path: str | None = None,
    token: str | None = None,
    device_id: str | None = None,
    json_body: dict | None = None,
    params: dict | None = None,
    files: dict | None = None,
) -> tuple[requests.Response, dict]:
    mark_endpoint(method, route_path or path)

    headers = build_headers(token=token, device_id=device_id)

    for attempt in range(RATE_LIMIT_RETRY_COUNT):
        response = requests.request(
            method,
            f"{BASE_URL}{path}",
            headers=headers,
            json=json_body,
            params=params,
            files=files,
            timeout=REQUEST_TIMEOUT,
        )

        try:
            body = response.json()
        except ValueError as exc:
            raise TestFailure(f"{method} {path} returned non-JSON body: {response.text}") from exc

        # 全量黑盒会在极短时间内连续命中很多接口，本地网关的全局 IP 限流可能短暂返回 429。
        # 这种情况属于测试噪声，不是单个业务接口真实失败，因此在这里做有限次退避重试。
        if response.status_code != 429 and body.get("code") != 10005:
            return response, body
        if attempt == RATE_LIMIT_RETRY_COUNT - 1:
            return response, body
        time.sleep(RATE_LIMIT_RETRY_BASE_DELAY * (attempt + 1))

    raise TestFailure(f"{method} {path} exhausted unexpected retry loop")

def ensure_success(name: str, response: requests.Response, body: dict) -> dict:
    if response.status_code != 200:
        raise TestFailure(f"{name} http status={response.status_code}, body={body}")
    if body.get("code") != 0:
        raise TestFailure(f"{name} business code={body.get('code')}, body={body}")
    return body.get("data") or {}

def wait_until(name: str, fn, timeout: float = 20.0, interval: float = 1.0):
    deadline = time.time() + timeout
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            return fn()
        except Exception as exc:  # noqa: BLE001
            last_error = exc
            time.sleep(interval)
    raise TestFailure(f"{name} timeout: {last_error}")

def expect_contains(items: list[dict], key: str, value: str, name: str) -> dict:
    for item in items:
        if item.get(key) == value:
            return item
    raise TestFailure(f"{name} missing item with {key}={value}: {items}")

def expect_string_contains(items: list[str], value: str, name: str) -> None:
    if value not in items:
        raise TestFailure(f"{name} missing item {value}: {items}")

def mark_connect_endpoint(method: str, path: str) -> None:
    CONNECT_COVERED_ENDPOINTS.add((method.upper(), path))

def test_connect_interfaces(recorder: Recorder, token: str, device_id: str) -> None:
    mark_connect_endpoint("GET", "/health")
    health_response = requests.get(
        f"{CONNECT_HTTP_BASE_URL}/health",
        headers=build_headers(),
        timeout=REQUEST_TIMEOUT,
    )
    health_body = health_response.json()
    if health_response.status_code != 200 or health_body.get("status") != "ok":
        raise TestFailure(f"connect health unexpected: http={health_response.status_code}, body={health_body}")
    recorder.ok("connect-health", str(health_body.get("online_connections", 0)))

    mark_connect_endpoint("GET", "/metrics")
    metrics_response = requests.get(f"{CONNECT_HTTP_BASE_URL}/metrics", timeout=REQUEST_TIMEOUT)
    if metrics_response.status_code != 200 or "connect_online_connections" not in metrics_response.text:
        raise TestFailure(
            f"connect metrics unexpected: http={metrics_response.status_code}, body={metrics_response.text[:200]}"
        )
    recorder.ok("connect-metrics", "metrics reachable")

    mark_connect_endpoint("GET", "/ws")
    ws_url = f"{CONNECT_WS_BASE_URL}/ws?token={token}&device_id={device_id}"
    ws = websocket.create_connection(ws_url, timeout=REQUEST_TIMEOUT, origin=CONNECT_WS_ORIGIN)
    try:
        ws.settimeout(REQUEST_TIMEOUT)
        ws.send_binary(b"\x0a\theartbeat")
        frame = ws.recv()
        if not isinstance(frame, (bytes, bytearray)):
            raise TestFailure(f"connect ws heartbeat returned non-binary payload: {frame!r}")
        if b"heartbeat_ack" not in frame:
            raise TestFailure(f"connect ws heartbeat ack mismatch: {frame!r}")
        recorder.ok("connect-ws", "heartbeat_ack")
    finally:
        ws.close()

def test_group_interfaces(recorder: Recorder, token_a: str, token_b: str, user_a: str, user_b: str, suffix: str) -> None:
    group1_name = f"grp-mgmt-{suffix}"
    group2_name = f"grp-apply-{suffix}"
    group3_name = f"grp-cancel-{suffix}"
    group1_avatar = f"https://example.com/{suffix}/g1.png"
    group1_avatar_new = f"https://example.com/{suffix}/g1-new.png"
    group_notice = f"notice-{suffix}"

    response, body = request_json(
        "POST",
        "/api/v1/auth/groups",
        token=token_a,
        json_body={"name": group1_name, "avatar": group1_avatar},
    )
    group1_uuid = ensure_success("group1-create", response, body)["groupUuid"]
    recorder.ok("group1-create", group1_uuid)

    response, body = request_json(
        "POST",
        "/api/v1/auth/groups",
        token=token_a,
        json_body={"name": group2_name, "avatar": f"https://example.com/{suffix}/g2.png"},
    )
    group2_uuid = ensure_success("group2-create", response, body)["groupUuid"]
    recorder.ok("group2-create", group2_uuid)

    response, body = request_json(
        "POST",
        "/api/v1/auth/groups",
        token=token_a,
        json_body={"name": group3_name, "avatar": f"https://example.com/{suffix}/g3.png"},
    )
    group3_uuid = ensure_success("group3-create", response, body)["groupUuid"]
    recorder.ok("group3-create", group3_uuid)

    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group1_uuid}",
        route_path="/api/v1/auth/groups/:groupUuid",
        token=token_a,
        json_body={"name": f"{group1_name}-v2", "avatar": group1_avatar_new, "addMode": 1},
    )
    ensure_success("group1-update-info", response, body)
    recorder.ok("group1-update-info", group1_uuid)

    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group2_uuid}",
        route_path="/api/v1/auth/groups/:groupUuid",
        token=token_a,
        json_body={"addMode": 1},
    )
    ensure_success("group2-update-info", response, body)

    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group3_uuid}",
        route_path="/api/v1/auth/groups/:groupUuid",
        token=token_a,
        json_body={"addMode": 1},
    )
    ensure_success("group3-update-info", response, body)

    response, body = request_json("GET", "/api/v1/auth/groups", token=token_a)
    groups = ensure_success("group-list", response, body).get("groups", [])
    expect_contains(groups, "groupUuid", group1_uuid, "group-list")
    expect_contains(groups, "groupUuid", group2_uuid, "group-list")
    expect_contains(groups, "groupUuid", group3_uuid, "group-list")
    recorder.ok("group-list", "three groups visible")

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group1_uuid}",
        route_path="/api/v1/auth/groups/:groupUuid",
        token=token_a,
    )
    group_info = ensure_success("group-info", response, body)
    if group_info.get("name") != f"{group1_name}-v2":
        raise TestFailure(f"group-info name mismatch: {group_info}")
    recorder.ok("group-info", group1_uuid)

    response, body = request_json(
        "PUT",
        f"/api/v1/auth/groups/{group1_uuid}/notice",
        route_path="/api/v1/auth/groups/:groupUuid/notice",
        token=token_a,
        json_body={"notice": group_notice},
    )
    ensure_success("group-notice", response, body)
    recorder.ok("group-notice", group_notice)

    response, body = request_json(
        "POST",
        f"/api/v1/auth/groups/{group1_uuid}/members",
        route_path="/api/v1/auth/groups/:groupUuid/members",
        token=token_a,
        json_body={"userUuids": [user_b]},
    )
    ensure_success("group-add-member", response, body)
    recorder.ok("group-add-member", user_b)

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group1_uuid}/members",
        route_path="/api/v1/auth/groups/:groupUuid/members",
        token=token_a,
    )
    members = ensure_success("group-members", response, body).get("members", [])
    expect_contains(members, "userUuid", user_a, "group-members")
    expect_contains(members, "userUuid", user_b, "group-members")
    recorder.ok("group-members", "owner+member")

    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group1_uuid}/my-nickname",
        route_path="/api/v1/auth/groups/:groupUuid/my-nickname",
        token=token_a,
        json_body={"groupNickname": f"A-g1-{suffix}"},
    )
    ensure_success("group-my-nickname", response, body)
    recorder.ok("group-my-nickname", f"A-g1-{suffix}")

    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group1_uuid}/members/{user_b}/nickname",
        route_path="/api/v1/auth/groups/:groupUuid/members/:userUuid/nickname",
        token=token_a,
        json_body={"groupNickname": f"B-g1-{suffix}"},
    )
    ensure_success("group-member-nickname", response, body)
    recorder.ok("group-member-nickname", user_b)

    mute_until = int(time.time() * 1000) + 3600_000
    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group1_uuid}/members/{user_b}/mute",
        route_path="/api/v1/auth/groups/:groupUuid/members/:userUuid/mute",
        token=token_a,
        json_body={"muteUntil": mute_until},
    )
    ensure_success("group-member-mute", response, body)
    recorder.ok("group-member-mute", user_b)

    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group1_uuid}/mute-setting",
        route_path="/api/v1/auth/groups/:groupUuid/mute-setting",
        token=token_a,
        json_body={"muteAll": True},
    )
    ensure_success("group-mute-setting", response, body)
    recorder.ok("group-mute-setting", "true")

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group1_uuid}/members/search",
        route_path="/api/v1/auth/groups/:groupUuid/members/search",
        token=token_a,
        params={"keyword": f"B-g1-{suffix}", "page": 1, "pageSize": 20},
    )
    search_members = ensure_success("group-member-search", response, body).get("members", [])
    expect_contains(search_members, "userUuid", user_b, "group-member-search")
    recorder.ok("group-member-search", user_b)

    response, body = request_json(
        "GET",
        "/api/v1/auth/groups/search",
        token=token_a,
        params={"keyword": f"{group1_name}-v2", "page": 1, "pageSize": 20},
    )
    search_groups = ensure_success("group-search", response, body).get("groups", [])
    expect_contains(search_groups, "groupUuid", group1_uuid, "group-search")
    recorder.ok("group-search", group1_uuid)

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group1_uuid}/member-ids",
        route_path="/api/v1/auth/groups/:groupUuid/member-ids",
        token=token_a,
    )
    member_ids = ensure_success("group-member-ids", response, body).get("userUuids", [])
    expect_string_contains(member_ids, user_a, "group-member-ids")
    expect_string_contains(member_ids, user_b, "group-member-ids")
    recorder.ok("group-member-ids", "owner+member")

    response, body = request_json(
        "PATCH",
        f"/api/v1/auth/groups/{group1_uuid}/members/{user_b}/role",
        route_path="/api/v1/auth/groups/:groupUuid/members/:userUuid/role",
        token=token_a,
        json_body={"role": 1},
    )
    ensure_success("group-member-role", response, body)
    recorder.ok("group-member-role", user_b)

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group1_uuid}/members",
        route_path="/api/v1/auth/groups/:groupUuid/members",
        token=token_a,
    )
    members_after_role = ensure_success("group-members-after-role", response, body).get("members", [])
    role_item = expect_contains(members_after_role, "userUuid", user_b, "group-members-after-role")
    if role_item.get("role") != 1:
        raise TestFailure(f"group member role mismatch: {role_item}")
    recorder.ok("group-members-after-role", user_b)

    response, body = request_json(
        "DELETE",
        f"/api/v1/auth/groups/{group1_uuid}/members/{user_b}",
        route_path="/api/v1/auth/groups/:groupUuid/members/:userUuid",
        token=token_a,
    )
    ensure_success("group-remove-member", response, body)
    recorder.ok("group-remove-member", user_b)

    response, body = request_json(
        "POST",
        f"/api/v1/auth/groups/{group1_uuid}/members",
        route_path="/api/v1/auth/groups/:groupUuid/members",
        token=token_a,
        json_body={"userUuids": [user_b]},
    )
    ensure_success("group-add-member-again", response, body)
    recorder.ok("group-add-member-again", user_b)

    response, body = request_json(
        "POST",
        f"/api/v1/auth/groups/{group1_uuid}/transfer-owner",
        route_path="/api/v1/auth/groups/:groupUuid/transfer-owner",
        token=token_a,
        json_body={"targetUserUuid": user_b},
    )
    ensure_success("group-transfer-owner", response, body)
    recorder.ok("group-transfer-owner", user_b)

    response, body = request_json(
        "DELETE",
        f"/api/v1/auth/groups/{group1_uuid}",
        route_path="/api/v1/auth/groups/:groupUuid",
        token=token_b,
    )
    ensure_success("group-dismiss", response, body)
    recorder.ok("group-dismiss", group1_uuid)

    response, body = request_json(
        "POST",
        f"/api/v1/auth/groups/{group2_uuid}/apply",
        route_path="/api/v1/auth/groups/:groupUuid/apply",
        token=token_b,
        json_body={"reason": f"apply-{suffix}"},
    )
    apply2 = ensure_success("group2-apply", response, body)
    if apply2.get("joinedDirectly"):
        raise TestFailure(f"group2 apply should require review: {apply2}")
    group2_apply_id = apply2.get("applyId")
    recorder.ok("group2-apply", str(group2_apply_id))

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group2_uuid}/my-join-application",
        route_path="/api/v1/auth/groups/:groupUuid/my-join-application",
        token=token_b,
    )
    my_apply2 = ensure_success("group2-my-join-application", response, body)
    if not my_apply2.get("hasApplication"):
        raise TestFailure(f"group2 my-join-application missing: {my_apply2}")
    recorder.ok("group2-my-join-application", str(group2_apply_id))

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group2_uuid}/join-requests/pending-count",
        route_path="/api/v1/auth/groups/:groupUuid/join-requests/pending-count",
        token=token_a,
    )
    pending2 = ensure_success("group2-pending-count", response, body)
    if pending2.get("count", 0) < 1:
        raise TestFailure(f"group2 pending count unexpected: {pending2}")
    recorder.ok("group2-pending-count", str(pending2.get("count")))

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group2_uuid}/join-requests",
        route_path="/api/v1/auth/groups/:groupUuid/join-requests",
        token=token_a,
        params={"page": 1, "pageSize": 20},
    )
    join_requests2 = ensure_success("group2-join-requests", response, body).get("items", [])
    expect_contains(join_requests2, "applyId", group2_apply_id, "group2-join-requests")
    recorder.ok("group2-join-requests", str(group2_apply_id))

    response, body = request_json(
        "POST",
        f"/api/v1/auth/groups/{group2_uuid}/join-requests/{group2_apply_id}/review",
        route_path="/api/v1/auth/groups/:groupUuid/join-requests/:applyId/review",
        token=token_a,
        json_body={"action": 1, "remark": "ok"},
    )
    ensure_success("group2-review", response, body)
    recorder.ok("group2-review", str(group2_apply_id))

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group2_uuid}/join-requests/reviewed",
        route_path="/api/v1/auth/groups/:groupUuid/join-requests/reviewed",
        token=token_a,
        params={"page": 1, "pageSize": 20, "status": 1},
    )
    reviewed2 = ensure_success("group2-reviewed", response, body).get("items", [])
    reviewed_item2 = expect_contains(reviewed2, "applyId", group2_apply_id, "group2-reviewed")
    if reviewed_item2.get("status") != 1:
        raise TestFailure(f"group2 reviewed status mismatch: {reviewed_item2}")
    recorder.ok("group2-reviewed", str(group2_apply_id))

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group2_uuid}/my-join-application",
        route_path="/api/v1/auth/groups/:groupUuid/my-join-application",
        token=token_b,
    )
    my_apply2_done = ensure_success("group2-my-join-application-after-review", response, body)
    if not my_apply2_done.get("hasApplication"):
        raise TestFailure(f"group2 my-join-application after review missing: {my_apply2_done}")
    recorder.ok("group2-my-join-application-after-review", str(group2_apply_id))

    response, body = request_json(
        "POST",
        f"/api/v1/auth/groups/{group2_uuid}/leave",
        route_path="/api/v1/auth/groups/:groupUuid/leave",
        token=token_b,
    )
    ensure_success("group2-leave", response, body)
    recorder.ok("group2-leave", group2_uuid)

    response, body = request_json(
        "POST",
        f"/api/v1/auth/groups/{group3_uuid}/apply",
        route_path="/api/v1/auth/groups/:groupUuid/apply",
        token=token_b,
        json_body={"reason": f"cancel-{suffix}"},
    )
    apply3 = ensure_success("group3-apply", response, body)
    if apply3.get("joinedDirectly"):
        raise TestFailure(f"group3 apply should require review: {apply3}")
    group3_apply_id = apply3.get("applyId")
    recorder.ok("group3-apply", str(group3_apply_id))

    response, body = request_json(
        "DELETE",
        f"/api/v1/auth/groups/{group3_uuid}/apply",
        route_path="/api/v1/auth/groups/:groupUuid/apply",
        token=token_b,
    )
    ensure_success("group3-cancel-apply", response, body)
    recorder.ok("group3-cancel-apply", str(group3_apply_id))

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group3_uuid}/join-requests",
        route_path="/api/v1/auth/groups/:groupUuid/join-requests",
        token=token_a,
        params={"page": 1, "pageSize": 20},
    )
    pending3 = ensure_success("group3-join-requests-after-cancel", response, body).get("items", [])
    if pending3:
        raise TestFailure(f"group3 join requests should be empty after cancel: {pending3}")
    recorder.ok("group3-join-requests-after-cancel", "empty")

    response, body = request_json(
        "GET",
        f"/api/v1/auth/groups/{group3_uuid}/join-requests/pending-count",
        route_path="/api/v1/auth/groups/:groupUuid/join-requests/pending-count",
        token=token_a,
    )
    pending3_count = ensure_success("group3-pending-count-after-cancel", response, body)
    if pending3_count.get("count", 0) != 0:
        raise TestFailure(f"group3 pending count after cancel unexpected: {pending3_count}")
    recorder.ok("group3-pending-count-after-cancel", "0")

    response, body = request_json(
        "GET",
        "/api/v1/auth/groups/join-applications",
        token=token_b,
        params={"page": 1, "pageSize": 20},
    )
    my_apps = ensure_success("group-join-applications", response, body).get("items", [])
    expect_contains(my_apps, "groupUuid", group2_uuid, "group-join-applications")
    recorder.ok("group-join-applications", group2_uuid)

def main() -> int:
    recorder = Recorder()
    suffix = uuid.uuid4().hex[:6]

    email_a = f"a{suffix}@example.com"
    email_b = f"b{suffix}@example.com"
    new_email_a = f"na{suffix}@example.com"
    nickname_a = f"A{suffix}"
    nickname_b = f"B{suffix}"
    nickname_a2 = f"AU{suffix[:4]}"
    phone_seed = int(suffix, 16) % 1_000_000_000
    telephone_a = f"13{phone_seed:09d}"
    telephone_b = f"13{(phone_seed + 1) % 1_000_000_000:09d}"
    password_a = "Passw0rd1"
    password_b = "Passw0rd2"
    password_a2 = "Passw0rd3"
    password_b2 = "ResetPwd4"

    device_a1 = f"dev-a1-{suffix}"
    device_a2 = f"dev-a2-{suffix}"
    device_a3 = f"dev-a3-{suffix}"
    device_a4 = f"dev-a4-{suffix}"
    device_a5 = f"dev-a5-{suffix}"
    device_b1 = f"dev-b1-{suffix}"
    device_b2 = f"dev-b2-{suffix}"
    device_b3 = f"dev-b3-{suffix}"
    device_code = f"dev-code-{suffix}"

    code_register = "111111"
    code_login = "222222"
    code_reset = "333333"
    code_change_email = "444444"

    try:
        response, body = request_json("GET", "/health")
        if response.status_code == 200 and body.get("status") == "ok":
            recorder.ok("health", "gateway healthy")
        else:
            raise TestFailure(f"health body unexpected: {body}")

        mark_endpoint("GET", "/metrics")
        metrics_response = requests.get(
            f"{BASE_URL}/metrics",
            headers=build_headers(),
            timeout=REQUEST_TIMEOUT,
        )
        if metrics_response.status_code != 200 or not metrics_response.text.strip():
            raise TestFailure(
                f"metrics unexpected: http={metrics_response.status_code}, body={metrics_response.text[:200]}"
            )
        recorder.ok("metrics", "metrics reachable")

        redis_set(f"user:verify_code:{email_a}:1", code_register)
        response, body = request_json(
            "POST",
            "/api/v1/public/user/verify-code",
            json_body={"email": email_a, "verifyCode": code_register, "type": 1},
        )
        data = ensure_success("verify-code", response, body)
        if not data.get("valid"):
            raise TestFailure(f"verify-code returned invalid: {data}")
        recorder.ok("verify-code", "manual redis seeded code verified")

        response, body = request_json(
            "POST",
            "/api/v1/public/user/register",
            json_body={
                "email": email_a,
                "password": password_a,
                "verifyCode": code_register,
                "nickname": nickname_a,
                "telephone": telephone_a,
            },
        )
        data = ensure_success("register-a", response, body)
        user_a = data["userUuid"]
        recorder.ok("register-a", user_a)

        redis_set(f"user:verify_code:{email_b}:1", code_register)
        response, body = request_json(
            "POST",
            "/api/v1/public/user/register",
            json_body={
                "email": email_b,
                "password": password_b,
                "verifyCode": code_register,
                "nickname": nickname_b,
                "telephone": telephone_b,
            },
        )
        data = ensure_success("register-b", response, body)
        user_b = data["userUuid"]
        recorder.ok("register-b", user_b)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/login",
            device_id=device_a1,
            json_body={
                "account": email_a,
                "password": password_a,
                "deviceInfo": {"deviceName": "A1", "platform": "Web", "appVersion": "1.0.0"},
            },
        )
        data = ensure_success("login-a1", response, body)
        token_a1 = data["accessToken"]
        refresh_a1 = data["refreshToken"]
        recorder.ok("login-a1", device_a1)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/login",
            device_id=device_a2,
            json_body={
                "account": email_a,
                "password": password_a,
                "deviceInfo": {"deviceName": "A2", "platform": "Web", "appVersion": "1.0.0"},
            },
        )
        ensure_success("login-a2", response, body)
        recorder.ok("login-a2", device_a2)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/login",
            device_id=device_b1,
            json_body={
                "account": email_b,
                "password": password_b,
                "deviceInfo": {"deviceName": "B1", "platform": "Web", "appVersion": "1.0.0"},
            },
        )
        data = ensure_success("login-b1", response, body)
        token_b1 = data["accessToken"]
        recorder.ok("login-b1", device_b1)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/refresh-token",
            device_id=device_a1,
            json_body={"uuid": user_a, "device_id": device_a1, "refreshToken": refresh_a1},
        )
        data = ensure_success("refresh-token", response, body)
        token_a1 = data["accessToken"]
        recorder.ok("refresh-token", "refreshed access token")

        test_connect_interfaces(recorder, token_a1, device_a1)

        def get_profile_a() -> dict:
            response, body = request_json("GET", "/api/v1/auth/user/profile", token=token_a1)
            return ensure_success("get-profile-a", response, body)

        profile_a = wait_until("wait profile a", get_profile_a)
        if profile_a.get("userInfo", {}).get("uuid") != user_a:
            raise TestFailure(f"profile a uuid mismatch: {profile_a}")
        recorder.ok("get-profile-a", user_a)

        def get_profile_b_from_a() -> dict:
            response, body = request_json(
                "GET",
                f"/api/v1/auth/user/profile/{user_b}",
                route_path="/api/v1/auth/user/profile/:userUuid",
                token=token_a1,
            )
            return ensure_success("get-profile-b", response, body)

        profile_b = wait_until("wait profile b", get_profile_b_from_a)
        if profile_b.get("userInfo", {}).get("uuid") != user_b:
            raise TestFailure(f"profile b uuid mismatch: {profile_b}")
        recorder.ok("get-other-profile", user_b)

        response, body = request_json(
            "GET",
            "/api/v1/auth/user/search",
            token=token_a1,
            params={"keyword": nickname_b, "page": 1, "pageSize": 20},
        )
        data = ensure_success("search-user", response, body)
        expect_contains(data.get("items", []), "uuid", user_b, "search-user")
        recorder.ok("search-user", nickname_b)

        response, body = request_json("GET", "/api/v1/auth/user/qrcode", token=token_a1)
        data = ensure_success("get-qrcode", response, body)
        qr_code = data["qrCode"]
        qr_token = qr_code.rstrip("/").rsplit("/", 1)[-1]
        recorder.ok("get-qrcode", qr_code)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/parse-qrcode",
            json_body={"token": qr_token},
        )
        data = ensure_success("parse-qrcode", response, body)
        if data.get("uuid") != user_a:
            raise TestFailure(f"parse-qrcode uuid mismatch: {data}")
        recorder.ok("parse-qrcode", user_a)

        response, body = request_json(
            "POST",
            "/api/v1/auth/user/batch-profile",
            token=token_a1,
            json_body={"userUuids": [user_a, user_b]},
        )
        data = ensure_success("batch-profile", response, body)
        expect_contains(data.get("users", []), "uuid", user_a, "batch-profile")
        expect_contains(data.get("users", []), "uuid", user_b, "batch-profile")
        recorder.ok("batch-profile", "returned two users")

        response, body = request_json("GET", "/api/v1/auth/user/devices", token=token_a1)
        data = ensure_success("device-list", response, body)
        expect_contains(data.get("devices", []), "deviceId", device_a1, "device-list")
        expect_contains(data.get("devices", []), "deviceId", device_a2, "device-list")
        recorder.ok("device-list", "two devices visible")

        response, body = request_json(
            "DELETE",
            f"/api/v1/auth/user/devices/{device_a2}",
            route_path="/api/v1/auth/user/devices/:deviceId",
            token=token_a1,
        )
        ensure_success("kick-device", response, body)
        recorder.ok("kick-device", device_a2)

        response, body = request_json(
            "GET",
            f"/api/v1/auth/user/online-status/{user_b}",
            route_path="/api/v1/auth/user/online-status/:userUuid",
            token=token_a1,
        )
        ensure_success("online-status", response, body)
        recorder.ok("online-status", user_b)

        response, body = request_json(
            "POST",
            "/api/v1/auth/user/batch-online-status",
            token=token_a1,
            json_body={"userUuids": [user_a, user_b]},
        )
        data = ensure_success("batch-online-status", response, body)
        expect_contains(data.get("users", []), "userUuid", user_a, "batch-online-status")
        expect_contains(data.get("users", []), "userUuid", user_b, "batch-online-status")
        recorder.ok("batch-online-status", "returned two users")

        response, body = request_json(
            "PUT",
            "/api/v1/auth/user/profile",
            token=token_a1,
            json_body={"nickname": nickname_a2, "signature": "blackbox-profile"},
        )
        ensure_success("update-profile", response, body)
        recorder.ok("update-profile", nickname_a2)

        def login_by_code_after_profile_change() -> dict:
            # 验证码登录成功后会消费验证码，因此每次重试前都重新写入测试验证码。
            redis_set(f"user:verify_code:{email_a}:2", code_login)
            response, body = request_json(
                "POST",
                "/api/v1/public/user/login-by-code",
                device_id=device_code,
                json_body={
                    "email": email_a,
                    "verifyCode": code_login,
                    "deviceInfo": {"deviceName": "ACode", "platform": "Web", "appVersion": "1.0.0"},
                },
            )
            data = ensure_success("login-by-code", response, body)
            if data.get("userInfo", {}).get("nickname") != nickname_a2:
                raise TestFailure(f"nickname not propagated yet: {data}")
            return data

        login_code_data = wait_until("wait profile_display_changed nickname", login_by_code_after_profile_change)
        recorder.ok("login-by-code", login_code_data.get("userInfo", {}).get("nickname", ""))

        with tempfile.NamedTemporaryFile("wb", suffix=".png", delete=False) as temp_file:
            temp_file.write(VALID_PNG_BYTES)
            avatar_path = temp_file.name

        try:
            mark_endpoint("POST", "/api/v1/auth/user/avatar")
            with open(avatar_path, "rb") as avatar_file:
                response = requests.post(
                    f"{BASE_URL}/api/v1/auth/user/avatar",
                    headers=build_headers(token=token_a1),
                    files={"avatar": ("avatar.png", avatar_file, "image/png")},
                    timeout=REQUEST_TIMEOUT,
                )
        finally:
            if os.path.exists(avatar_path):
                os.unlink(avatar_path)
        body = response.json()
        data = ensure_success("upload-avatar", response, body)
        avatar_url = data["avatarUrl"]
        recorder.ok("upload-avatar", avatar_url)

        def login_after_avatar_change() -> dict:
            response, body = request_json(
                "POST",
                "/api/v1/public/user/login",
                device_id=device_a3,
                json_body={
                    "account": email_a,
                    "password": password_a,
                    "deviceInfo": {"deviceName": "A3", "platform": "Web", "appVersion": "1.0.0"},
                },
            )
            data = ensure_success("login-after-avatar", response, body)
            if data.get("userInfo", {}).get("avatar") != avatar_url:
                raise TestFailure(f"avatar not propagated yet: {data}")
            return data

        login_avatar_data = wait_until("wait profile_display_changed avatar", login_after_avatar_change)
        token_a3 = login_avatar_data["accessToken"]
        recorder.ok("login-after-avatar", avatar_url)

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/apply",
            token=token_a1,
            json_body={"targetUuid": user_b, "reason": "hello", "source": "blackbox"},
        )
        data = ensure_success("friend-apply", response, body)
        apply_id = data["applyId"]
        recorder.ok("friend-apply", str(apply_id))

        def unread_apply() -> dict:
            response, body = request_json("GET", "/api/v1/auth/friend/apply/unread", token=token_b1)
            data = ensure_success("friend-apply-unread", response, body)
            if data.get("unreadCount", 0) < 1:
                raise TestFailure(f"unread count not ready: {data}")
            return data

        unread_data = wait_until("wait unread apply", unread_apply)
        recorder.ok("friend-apply-unread", str(unread_data.get("unreadCount")))

        response, body = request_json(
            "GET",
            "/api/v1/auth/friend/apply-list",
            token=token_b1,
            params={"page": 1, "pageSize": 20, "status": 0},
        )
        data = ensure_success("friend-apply-list", response, body)
        expect_contains(data.get("items", []), "applyId", apply_id, "friend-apply-list")
        recorder.ok("friend-apply-list", str(apply_id))

        response, body = request_json(
            "GET",
            "/api/v1/auth/friend/apply/sent",
            token=token_a1,
            params={"page": 1, "pageSize": 20, "status": 0},
        )
        data = ensure_success("friend-apply-sent", response, body)
        expect_contains(data.get("items", []), "applyId", apply_id, "friend-apply-sent")
        recorder.ok("friend-apply-sent", str(apply_id))

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/apply/read",
            token=token_b1,
            json_body={"applyIds": [apply_id]},
        )
        ensure_success("friend-apply-read", response, body)
        recorder.ok("friend-apply-read", str(apply_id))

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/apply/handle",
            token=token_b1,
            json_body={"applyId": apply_id, "action": 1, "remark": "ok"},
        )
        ensure_success("friend-apply-handle", response, body)
        recorder.ok("friend-apply-handle", "accepted")

        response, body = request_json(
            "GET",
            "/api/v1/auth/friend/list",
            token=token_a1,
            params={"page": 1, "pageSize": 20},
        )
        data = ensure_success("friend-list", response, body)
        friend_item = expect_contains(data.get("items", []), "uuid", user_b, "friend-list")
        friend_version = data.get("version", 0)
        recorder.ok("friend-list", friend_item.get("uuid", ""))

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/check",
            token=token_a1,
            json_body={"userUuid": user_a, "peerUuid": user_b},
        )
        data = ensure_success("friend-check", response, body)
        if not data.get("isFriend"):
            raise TestFailure(f"friend-check false: {data}")
        recorder.ok("friend-check", "true")

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/relation",
            token=token_a1,
            json_body={"userUuid": user_a, "peerUuid": user_b},
        )
        data = ensure_success("friend-relation", response, body)
        if not data.get("isFriend"):
            raise TestFailure(f"friend-relation false: {data}")
        recorder.ok("friend-relation", data.get("relation", ""))

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/remark",
            token=token_a1,
            json_body={"userUuid": user_b, "remark": "bb-remark"},
        )
        ensure_success("friend-remark", response, body)
        recorder.ok("friend-remark", "bb-remark")

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/tag",
            token=token_a1,
            json_body={"userUuid": user_b, "groupTag": "bb-tag"},
        )
        ensure_success("friend-tag", response, body)
        recorder.ok("friend-tag", "bb-tag")

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/sync",
            token=token_a1,
            json_body={"version": 0, "limit": 100},
        )
        data = ensure_success("friend-sync", response, body)
        if not data.get("changes"):
            raise TestFailure(f"friend-sync empty: {data}")
        recorder.ok("friend-sync", str(data.get("latestVersion")))

        response, body = request_json(
            "POST",
            "/api/v1/auth/messages/send",
            token=token_a1,
            json_body={
                "clientMsgId": f"msg-{suffix}",
                "convType": 1,
                "targetUuid": user_b,
                "msgType": 1,
                "content": json.dumps({"text": "hello blackbox"}),
            },
        )
        data = ensure_success("message-send", response, body)
        conv_id = data["convId"]
        msg_id = data["msgId"]
        msg_seq = data["seq"]
        recorder.ok("message-send", msg_id)

        def get_conversations_b() -> dict:
            response, body = request_json(
                "GET",
                "/api/v1/auth/conversations",
                token=token_b1,
                params={"pageSize": 50},
            )
            data = ensure_success("conversations-list", response, body)
            expect_contains(data.get("conversations", []), "convId", conv_id, "conversations-list")
            return data

        data = wait_until("wait conversation b", get_conversations_b)
        recorder.ok("conversations-list", conv_id)

        response, body = request_json(
            "GET",
            "/api/v1/auth/messages/pull",
            token=token_b1,
            params={"convId": conv_id, "limit": 20, "direction": 1},
        )
        data = ensure_success("message-pull", response, body)
        expect_contains(data.get("messages", []), "msgId", msg_id, "message-pull")
        recorder.ok("message-pull", msg_id)

        response, body = request_json(
            "POST",
            "/api/v1/auth/messages/get-by-ids",
            token=token_a1,
            json_body={"convId": conv_id, "msgIds": [msg_id]},
        )
        data = ensure_success("message-get-by-ids", response, body)
        expect_contains(data.get("messages", []), "msgId", msg_id, "message-get-by-ids")
        recorder.ok("message-get-by-ids", msg_id)

        response, body = request_json(
            "POST",
            "/api/v1/auth/conversations/mark-read",
            token=token_b1,
            json_body={"convId": conv_id, "readSeq": msg_seq},
        )
        ensure_success("conversation-mark-read", response, body)
        recorder.ok("conversation-mark-read", str(msg_seq))

        response, body = request_json(
            "PATCH",
            "/api/v1/auth/conversations/settings",
            token=token_b1,
            json_body={"convId": conv_id, "mute": True, "pin": True},
        )
        ensure_success("conversation-settings", response, body)
        recorder.ok("conversation-settings", "mute+pin")

        response, body = request_json(
            "POST",
            "/api/v1/auth/messages/recall",
            token=token_a1,
            json_body={"convId": conv_id, "msgId": msg_id},
        )
        ensure_success("message-recall", response, body)
        recorder.ok("message-recall", msg_id)

        test_group_interfaces(recorder, token_a1, token_b1, user_a, user_b, suffix)

        def pull_recalled_message() -> dict:
            response, body = request_json(
                "GET",
                "/api/v1/auth/messages/pull",
                token=token_b1,
                params={"convId": conv_id, "limit": 20, "direction": 1},
            )
            data = ensure_success("message-pull-after-recall", response, body)
            item = expect_contains(data.get("messages", []), "msgId", msg_id, "message-pull-after-recall")
            if item.get("status") != 1:
                raise TestFailure(f"message not recalled yet: {item}")
            return data

        wait_until("wait message recall visible", pull_recalled_message)
        recorder.ok("message-pull-after-recall", "status=1")

        response, body = request_json(
            "DELETE",
            f"/api/v1/auth/conversations/{conv_id}",
            route_path="/api/v1/auth/conversations/:convId",
            token=token_b1,
        )
        ensure_success("conversation-delete", response, body)
        recorder.ok("conversation-delete", conv_id)

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/delete",
            token=token_a1,
            json_body={"userUuid": user_b},
        )
        ensure_success("friend-delete", response, body)
        recorder.ok("friend-delete", user_b)

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/check",
            token=token_a1,
            json_body={"userUuid": user_a, "peerUuid": user_b},
        )
        data = ensure_success("friend-check-after-delete", response, body)
        if data.get("isFriend"):
            raise TestFailure(f"friend-check-after-delete true: {data}")
        recorder.ok("friend-check-after-delete", "false")

        response, body = request_json(
            "POST",
            "/api/v1/auth/friend/sync",
            token=token_a1,
            json_body={"version": friend_version, "limit": 100},
        )
        data = ensure_success("friend-sync-after-delete", response, body)
        recorder.ok("friend-sync-after-delete", str(data.get("latestVersion")))

        response, body = request_json(
            "POST",
            "/api/v1/auth/blacklist",
            token=token_a1,
            json_body={"targetUuid": user_b},
        )
        ensure_success("blacklist-add", response, body)
        recorder.ok("blacklist-add", user_b)

        response, body = request_json("GET", "/api/v1/auth/blacklist", token=token_a1, params={"page": 1, "pageSize": 20})
        data = ensure_success("blacklist-list", response, body)
        expect_contains(data.get("items", []), "uuid", user_b, "blacklist-list")
        recorder.ok("blacklist-list", user_b)

        response, body = request_json(
            "POST",
            "/api/v1/auth/blacklist/check",
            token=token_a1,
            json_body={"userUuid": user_a, "targetUuid": user_b},
        )
        data = ensure_success("blacklist-check", response, body)
        if not data.get("isBlacklist"):
            raise TestFailure(f"blacklist-check false: {data}")
        recorder.ok("blacklist-check", "true")

        response, body = request_json(
            "DELETE",
            f"/api/v1/auth/blacklist/{user_b}",
            route_path="/api/v1/auth/blacklist/:userUuid",
            token=token_a1,
        )
        ensure_success("blacklist-remove", response, body)
        recorder.ok("blacklist-remove", user_b)

        response, body = request_json(
            "POST",
            "/api/v1/auth/blacklist/check",
            token=token_a1,
            json_body={"userUuid": user_a, "targetUuid": user_b},
        )
        data = ensure_success("blacklist-check-after-remove", response, body)
        if data.get("isBlacklist"):
            raise TestFailure(f"blacklist-check-after-remove true: {data}")
        recorder.ok("blacklist-check-after-remove", "false")

        response, body = request_json(
            "POST",
            "/api/v1/auth/user/change-password",
            token=token_a3,
            json_body={"oldPassword": password_a, "newPassword": password_a2},
        )
        ensure_success("change-password", response, body)
        recorder.ok("change-password", "updated")

        response, body = request_json(
            "POST",
            "/api/v1/public/user/login",
            device_id=device_a4,
            json_body={
                "account": email_a,
                "password": password_a2,
                "deviceInfo": {"deviceName": "A4", "platform": "Web", "appVersion": "1.0.0"},
            },
        )
        data = ensure_success("login-after-change-password", response, body)
        token_a4 = data["accessToken"]
        recorder.ok("login-after-change-password", device_a4)

        redis_set(f"user:verify_code:{new_email_a}:4", code_change_email)
        response, body = request_json(
            "POST",
            "/api/v1/auth/user/change-email",
            token=token_a4,
            json_body={"newEmail": new_email_a, "verifyCode": code_change_email},
        )
        data = ensure_success("change-email", response, body)
        if data.get("email") != new_email_a:
            raise TestFailure(f"change-email mismatch: {data}")
        recorder.ok("change-email", new_email_a)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/login",
            device_id=device_a5,
            json_body={
                "account": new_email_a,
                "password": password_a2,
                "deviceInfo": {"deviceName": "A5", "platform": "Web", "appVersion": "1.0.0"},
            },
        )
        data = ensure_success("login-after-change-email", response, body)
        token_a5 = data["accessToken"]
        recorder.ok("login-after-change-email", new_email_a)

        redis_set(f"user:verify_code:{email_b}:3", code_reset)
        response, body = request_json(
            "POST",
            "/api/v1/public/user/reset-password",
            json_body={"email": email_b, "verifyCode": code_reset, "newPassword": password_b2},
        )
        ensure_success("reset-password", response, body)
        recorder.ok("reset-password", email_b)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/login",
            device_id=device_b2,
            json_body={
                "account": email_b,
                "password": password_b2,
                "deviceInfo": {"deviceName": "B2", "platform": "Web", "appVersion": "1.0.0"},
            },
        )
        data = ensure_success("login-after-reset-password", response, body)
        token_b2 = data["accessToken"]
        recorder.ok("login-after-reset-password", device_b2)

        response, body = request_json(
            "POST",
            "/api/v1/auth/user/delete-account",
            token=token_b2,
            json_body={"password": password_b2, "reason": "blackbox cleanup"},
        )
        ensure_success("delete-account", response, body)
        recorder.ok("delete-account", user_b)

        def wait_profile_deleted() -> dict:
            response, body = request_json(
                "GET",
                f"/api/v1/auth/user/profile/{user_b}",
                route_path="/api/v1/auth/user/profile/:userUuid",
                token=token_a5,
            )
            if response.status_code != 200:
                raise TestFailure(f"profile delete http mismatch: {response.status_code}, {body}")
            if body.get("code") == 0:
                raise TestFailure(f"profile still exists: {body}")
            return body

        wait_until("wait account.deleted profile cleanup", wait_profile_deleted)
        recorder.ok("account-deleted-profile-cleanup", user_b)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/login",
            device_id=device_b3,
            json_body={
                "account": email_b,
                "password": password_b2,
                "deviceInfo": {"deviceName": "B3", "platform": "Web", "appVersion": "1.0.0"},
            },
        )
        if response.status_code == 200 and body.get("code") != 0:
            recorder.ok("login-after-delete-account", f"blocked with code={body.get('code')}")
        else:
            raise TestFailure(f"login-after-delete-account unexpected: http={response.status_code}, body={body}")

        response, body = request_json(
            "POST",
            "/api/v1/auth/user/logout",
            token=token_a5,
            json_body={"deviceId": device_a5},
        )
        ensure_success("logout", response, body)
        recorder.ok("logout", device_a5)

        response, body = request_json(
            "POST",
            "/api/v1/public/user/send-verify-code",
            json_body={"email": f"mail{suffix}@example.com", "type": 1},
        )
        if response.status_code == 200 and body.get("code") == 0:
            recorder.ok("send-verify-code", "email sending configured")
        else:
            recorder.warn(
                "send-verify-code",
                f"blocked by email config or smtp: http={response.status_code}, code={body.get('code')}, message={body.get('message')}",
            )

        missing_connect = sorted(CONNECT_EXPECTED_ENDPOINTS - CONNECT_COVERED_ENDPOINTS)
        extra_connect = sorted(CONNECT_COVERED_ENDPOINTS - CONNECT_EXPECTED_ENDPOINTS)
        if missing_connect or extra_connect:
            raise TestFailure(f"connect endpoint coverage mismatch: missing={missing_connect}, extra={extra_connect}")
        recorder.ok("connect-endpoint-coverage", f"covered={len(CONNECT_COVERED_ENDPOINTS)}")

        missing = sorted(EXPECTED_ENDPOINTS - COVERED_ENDPOINTS)
        extra = sorted(COVERED_ENDPOINTS - EXPECTED_ENDPOINTS)
        if missing or extra:
            raise TestFailure(f"endpoint coverage mismatch: missing={missing}, extra={extra}")
        recorder.ok("endpoint-coverage", f"covered={len(COVERED_ENDPOINTS)}")

    except Exception as exc:  # noqa: BLE001
        recorder.fail("scenario", str(exc))

    passed, warned, failed = recorder.summary()
    print(f"\nSummary: pass={passed}, warn={warned}, fail={failed}")
    return 1 if failed else 0

if __name__ == "__main__":
    sys.exit(main())
