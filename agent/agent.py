#!/usr/bin/env python3
"""
MiniProbe Agent
===============
轻量级服务器监控客户端：自动安装依赖、自动注册、自动重连。

Usage:
    python3 agent.py -s ws://SERVER_IP:8080/ws -t YOUR_TOKEN

Options:
    -s / --server    服务端 WebSocket URL  (默认: ws://localhost:8080/ws)
    -t / --token     认证 Token            (默认: miniprobe)
    -i / --interval  上报间隔（秒）         (默认: 3)
"""

# ─── Stage 0: bootstrap imports ───────────────────────────────────────────────
import subprocess
import sys
import os


def _ensure(pip_name: str, import_name: str = None) -> None:
    """Install a package if not already importable."""
    try:
        __import__(import_name or pip_name)
    except ImportError:
        print(f"[*] Installing {pip_name} ...", flush=True)
        subprocess.check_call(
            [sys.executable, "-m", "pip", "install", pip_name, "-q"],
        )


_ensure("psutil")
_ensure("websocket-client", "websocket")

# ─── Stage 1: real imports ────────────────────────────────────────────────────
import argparse
import json
import platform
import socket
import threading
import time
import uuid

import psutil
import websocket  # websocket-client

# ─────────────────────────────────────────────────────────────────────────────
# Persistent Machine ID
# ─────────────────────────────────────────────────────────────────────────────

def _get_machine_id() -> str:
    """Return a stable UUID for this machine, persisted to ~/.miniprobe_id."""
    id_file = os.path.join(os.path.expanduser("~"), ".miniprobe_id")
    if os.path.exists(id_file):
        with open(id_file) as f:
            mid = f.read().strip()
        if mid:
            return mid
    mid = str(uuid.uuid4())
    try:
        with open(id_file, "w") as f:
            f.write(mid)
    except OSError:
        pass
    return mid


MACHINE_ID = _get_machine_id()

# ─────────────────────────────────────────────────────────────────────────────
# Metrics Collection
# ─────────────────────────────────────────────────────────────────────────────

_prev_net: psutil._common.snetio = None
_prev_net_time: float = None


def collect() -> dict:
    """Collect current system metrics and return as a JSON-serialisable dict."""
    global _prev_net, _prev_net_time

    # CPU % (non-blocking; returns value since last call — warm-up in main())
    cpu_pct = psutil.cpu_percent(interval=None)

    # Memory
    mem = psutil.virtual_memory()

    # Disk (root on POSIX, C:\ on Windows)
    disk_path = "C:\\" if sys.platform == "win32" else "/"
    try:
        disk = psutil.disk_usage(disk_path)
    except Exception:
        class _FakeDisk:
            total = used = percent = 0
        disk = _FakeDisk()

    # Network I/O rates
    net = psutil.net_io_counters()
    now = time.monotonic()
    net_in_rate = net_out_rate = 0
    if _prev_net is not None and _prev_net_time is not None:
        dt = now - _prev_net_time
        if dt > 0:
            net_in_rate  = max(0, int((net.bytes_recv - _prev_net.bytes_recv) / dt))
            net_out_rate = max(0, int((net.bytes_sent - _prev_net.bytes_sent) / dt))
    _prev_net      = net
    _prev_net_time = now

    # Load averages (Linux / macOS only)
    try:
        load1, load5, load15 = os.getloadavg()
    except AttributeError:
        load1 = load5 = load15 = 0.0

    # Primary outbound IP
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.settimeout(1)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
    except Exception:
        ip = "127.0.0.1"

    return {
        "id":           MACHINE_ID,
        "hostname":     socket.gethostname(),
        "os":           platform.system() + " " + platform.release(),
        "arch":         platform.machine(),
        "cpu":          round(cpu_pct, 1),
        "mem_total":    mem.total,
        "mem_used":     mem.used,
        "mem_percent":  round(mem.percent, 1),
        "disk_total":   disk.total,
        "disk_used":    disk.used,
        "disk_percent": round(float(disk.percent), 1),
        "net_in":       net.bytes_recv,
        "net_out":      net.bytes_sent,
        "net_in_rate":  net_in_rate,
        "net_out_rate": net_out_rate,
        "uptime":       int(time.time() - psutil.boot_time()),
        "ip":           ip,
        "load1":        round(load1,  2),
        "load5":        round(load5,  2),
        "load15":       round(load15, 2),
    }

# ─────────────────────────────────────────────────────────────────────────────
# WebSocket Agent
# ─────────────────────────────────────────────────────────────────────────────

class ProbeAgent:
    """Connects to the MiniProbe server and streams system metrics."""

    def __init__(self, url: str, interval: int) -> None:
        self.url      = url
        self.interval = interval
        self._ws      = None
        self._stop    = False

    # ── Callbacks ──────────────────────────────────────────────────────────────
    def _on_open(self, ws) -> None:
        self._ws = ws
        print(f"[+] Connected: {self.url}", flush=True)

        def _sender() -> None:
            while not self._stop:
                try:
                    data = collect()
                    ws.send(json.dumps(data))
                    print(
                        f"    CPU {data['cpu']:5.1f}%  "
                        f"MEM {data['mem_percent']:5.1f}%  "
                        f"Down {data['net_in_rate'] // 1024:5d} KB/s  "
                        f"Up {data['net_out_rate'] // 1024:5d} KB/s",
                        flush=True,
                    )
                except Exception as exc:
                    print(f"[!] Send error: {exc}", flush=True)
                    break
                time.sleep(self.interval)

        threading.Thread(target=_sender, daemon=True).start()

    def _on_error(self, ws, error) -> None:
        print(f"[!] WebSocket error: {error}", flush=True)

    def _on_close(self, ws, code, msg) -> None:
        print(f"[-] Disconnected (code={code}). Retry in 5 s …", flush=True)

    # ── Main loop (auto-reconnect) ─────────────────────────────────────────────
    def run(self) -> None:
        print("=" * 54, flush=True)
        print("  MiniProbe Agent", flush=True)
        print(f"  Machine ID : {MACHINE_ID}", flush=True)
        print(f"  Hostname   : {socket.gethostname()}", flush=True)
        print(f"  Server     : {self.url}", flush=True)
        print(f"  Interval   : {self.interval} s", flush=True)
        print("=" * 54, flush=True)

        while not self._stop:
            ws_app = websocket.WebSocketApp(
                self.url,
                on_open=self._on_open,
                on_error=self._on_error,
                on_close=self._on_close,
            )
            ws_app.run_forever(ping_interval=20, ping_timeout=10)
            if not self._stop:
                print("[*] Reconnecting in 5 s …", flush=True)
                time.sleep(5)

    def stop(self) -> None:
        self._stop = True
        if self._ws:
            try:
                self._ws.close()
            except Exception:
                pass

# ─────────────────────────────────────────────────────────────────────────────
# Entry Point
# ─────────────────────────────────────────────────────────────────────────────

def main() -> None:
    p = argparse.ArgumentParser(
        description="MiniProbe Agent — lightweight server monitoring client",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    p.add_argument("-s", "--server",
                   default="ws://localhost:8080/ws",
                   help="Server WebSocket URL")
    p.add_argument("-t", "--token",
                   default="miniprobe",
                   help="Auth token")
    p.add_argument("-i", "--interval",
                   type=int, default=3,
                   help="Metrics report interval (seconds)")
    args = p.parse_args()

    # Build URL with token
    url = args.server
    sep = "&" if "?" in url else "?"
    url = url + sep + "token=" + args.token

    # Warm-up CPU % (first call always returns 0 — discard it)
    psutil.cpu_percent(interval=1)

    agent = ProbeAgent(url, args.interval)
    try:
        agent.run()
    except KeyboardInterrupt:
        print("\n[*] Stopping agent …", flush=True)
        agent.stop()


if __name__ == "__main__":
    main()
