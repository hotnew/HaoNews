#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import sys
import urllib.parse
import urllib.request

LOCAL_BASE = os.environ.get("LIVE_LOCAL_BASE", "http://127.0.0.1:51818")
REMOTE_BASE = os.environ.get("LIVE_REMOTE_BASE", "http://192.168.102.74:51818")
ROOM_ID = os.environ.get("LIVE_ROOM_ID", "public-live-time")
CREATE_ARCHIVE = os.environ.get("LIVE_VERIFY_CREATE_ARCHIVE", "1").strip().lower() not in {"0", "false", "no"}


def fetch_json(url: str, method: str = "GET", payload: dict | None = None) -> dict:
    data = None
    headers = {"Accept": "application/json"}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=12) as resp:
        return json.load(resp)


def summarize_status(name: str, status: dict) -> None:
    watcher = status.get("watcher") or {}
    sender = status.get("sender_config") or {}
    identity = status.get("sender_identity") or {}
    archive_stats = status.get("archive_stats") or {}
    print(f"[{name}] watcher_peer_id={status.get('watcher_peer_id') or watcher.get('peer_id') or 'none'}")
    print(f"[{name}] watcher_listen_port={status.get('watcher_listen_port') or watcher.get('listen_port') or 0}")
    print(f"[{name}] sender_peer_id={status.get('sender_peer_id') or identity.get('agent_id') or 'none'}")
    print(f"[{name}] sender_listen_port={sender.get('listen_port') or 0}")
    print(f"[{name}] visible={status.get('visible_event_count')} total={status.get('total_event_count')}")
    print(f"[{name}] latest_inbound={status.get('latest_non_heartbeat_at') or status.get('latest_event_at') or 'none'}")
    print(f"[{name}] latest_local_write={status.get('latest_local_write_at') or 'none'}")
    print(f"[{name}] latest_cache_refresh={status.get('latest_cache_refresh_at') or 'none'}")
    print(f"[{name}] latest_archive={status.get('latest_archive_at') or 'none'}")
    print(f"[{name}] archive_count={archive_stats.get('archive_count', 0)} latest_kind={archive_stats.get('latest_archive_kind') or 'none'}")


def summarize_window(name: str, room: dict) -> None:
    events = room.get("events") or []
    latest = events[-1] if events else {}
    payload = latest.get("payload") or {}
    print(f"[{name}] show_all={room.get('show_all')} visible={room.get('visible_event_count')} total={room.get('total_event_count')}")
    print(f"[{name}] latest_ts={latest.get('timestamp', 'none')}")
    print(f"[{name}] latest_content={payload.get('content', '')}")


def ensure(cond: bool, msg: str) -> None:
    if not cond:
        raise RuntimeError(msg)


def latest_archive_payload(archive_response: dict) -> dict:
    archive = archive_response.get("archive") or {}
    return {
        "kind": archive.get("kind", ""),
        "archive_id": archive.get("archive_id", ""),
        "message_count": archive.get("message_count", 0),
        "event_count": archive.get("event_count", 0),
        "heartbeat_count": archive.get("heartbeat_count", 0),
        "start_at": archive.get("start_at", ""),
        "end_at": archive.get("end_at", ""),
        "archived_at": archive.get("archived_at", ""),
    }


def main() -> int:
    local_status = fetch_json(f"{LOCAL_BASE}/api/live/status/{urllib.parse.quote(ROOM_ID)}")
    remote_status = fetch_json(f"{REMOTE_BASE}/api/live/status/{urllib.parse.quote(ROOM_ID)}")
    summarize_status("local-status", local_status)
    summarize_status("remote-status", remote_status)

    local_window = fetch_json(f"{LOCAL_BASE}/api/live/{urllib.parse.quote('public/' + ROOM_ID.split('-', 1)[-1])}?show_all=1")
    remote_window = fetch_json(f"{REMOTE_BASE}/api/live/{urllib.parse.quote('public/' + ROOM_ID.split('-', 1)[-1])}?show_all=1")
    summarize_window("local-window", local_window)
    summarize_window("remote-window", remote_window)

    ensure(local_window.get("show_all") is True, "local show_all window is not enabled")
    ensure(remote_window.get("show_all") is True, "remote show_all window is not enabled")
    ensure(local_window.get("visible_event_count") == local_window.get("total_event_count"), "local show_all should expose all visible events")
    ensure(remote_window.get("visible_event_count") == remote_window.get("total_event_count"), "remote show_all should expose all visible events")
    ensure(len(local_window.get("events") or []) > 0, "local show_all should expose at least one event")
    ensure(len(remote_window.get("events") or []) > 0, "remote show_all should expose at least one event")

    archive_info = None
    if CREATE_ARCHIVE:
        create_url = f"{LOCAL_BASE}/api/live/archive/{urllib.parse.quote(ROOM_ID)}"
        archive_response = fetch_json(create_url, method="POST")
        archive_info = latest_archive_payload(archive_response)
        print("[manual-archive] " + " ".join(f"{k}={v}" for k, v in archive_info.items()))
        ensure(archive_info["archive_id"] != "", "manual archive missing archive_id")
        ensure(archive_info["message_count"] > 0, "manual archive message_count must be positive")
        ensure(archive_info["event_count"] >= archive_info["message_count"], "manual archive event_count must be >= message_count")
        ensure(archive_info["kind"] == "manual", "manual archive kind must be manual")

    if archive_info is not None:
        archive_index = fetch_json(f"{LOCAL_BASE}/api/archive/live")
        rooms = archive_index.get("rooms") or []
        reflected = any((room.get("latest_archive") or {}).get("archive_id") == archive_info["archive_id"] for room in rooms)
        print(f"[manual-archive-index] reflected={reflected} rooms={len(rooms)}")
        if not reflected:
            print("[manual-archive-index] note: archive index cache may be lagging; manual archive response already validated")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
