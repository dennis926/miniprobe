package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ─────────────────────────────────────────────────────────────────────────────
// Data Model
// ─────────────────────────────────────────────────────────────────────────────

// AgentInfo holds all system metrics reported by a connected agent.
type AgentInfo struct {
	ID          string    `json:"id"`
	Hostname    string    `json:"hostname"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	CPU         float64   `json:"cpu"`
	MemTotal    uint64    `json:"mem_total"`
	MemUsed     uint64    `json:"mem_used"`
	MemPercent  float64   `json:"mem_percent"`
	DiskTotal   uint64    `json:"disk_total"`
	DiskUsed    uint64    `json:"disk_used"`
	DiskPercent float64   `json:"disk_percent"`
	NetIn       uint64    `json:"net_in"`
	NetOut      uint64    `json:"net_out"`
	NetInRate   uint64    `json:"net_in_rate"`
	NetOutRate  uint64    `json:"net_out_rate"`
	Uptime      uint64    `json:"uptime"`
	IP          string    `json:"ip"`
	Load1       float64   `json:"load1"`
	Load5       float64   `json:"load5"`
	Load15      float64   `json:"load15"`
	LastSeen    time.Time `json:"last_seen"`
	Online      bool      `json:"online"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Server
// ─────────────────────────────────────────────────────────────────────────────

// Server manages all agent connections and in-memory metrics store.
type Server struct {
	agents map[string]*AgentInfo
	mu     sync.RWMutex
	token  string
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// NewServer creates a Server and starts the background offline-checker.
func NewServer(token string) *Server {
	s := &Server{
		agents: make(map[string]*AgentInfo),
		token:  token,
	}
	go s.offlineChecker()
	return s
}

// offlineChecker marks agents that haven't reported in 30 s as offline.
func (s *Server) offlineChecker() {
	for range time.Tick(10 * time.Second) {
		s.mu.Lock()
		for _, a := range s.agents {
			if time.Since(a.LastSeen) > 30*time.Second {
				a.Online = false
			}
		}
		s.mu.Unlock()
	}
}

// handleWS accepts WebSocket connections from agents.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if s.token != "" && r.URL.Query().Get("token") != s.token {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()
	log.Printf("[+] Agent connected from %s", r.RemoteAddr)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[-] Agent disconnected: %s", r.RemoteAddr)
			break
		}
		var info AgentInfo
		if err := json.Unmarshal(msg, &info); err != nil {
			log.Printf("[!] JSON parse error: %v", err)
			continue
		}
		info.LastSeen = time.Now()
		info.Online = true
		if info.IP == "" {
			info.IP = r.RemoteAddr
		}
		s.mu.Lock()
		s.agents[info.ID] = &info
		s.mu.Unlock()
	}
}

// handleAgents serves all agent data as a JSON array.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	s.mu.RLock()
	agents := make([]*AgentInfo, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	s.mu.RUnlock()
	// Online nodes first, then alphabetical by hostname.
	sort.Slice(agents, func(i, j int) bool {
		if agents[i].Online != agents[j].Online {
			return agents[i].Online
		}
		return agents[i].Hostname < agents[j].Hostname
	})
	json.NewEncoder(w).Encode(agents)
}

// handleDashboard serves the embedded HTML dashboard.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

// ─────────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	port  := flag.Int("port",  8080,        "Listening port")
	token := flag.String("token", "miniprobe", "Agent auth token")
	flag.Parse()

	s := NewServer(*token)
	http.HandleFunc("/", s.handleDashboard)
	http.HandleFunc("/ws", s.handleWS)
	http.HandleFunc("/api/agents", s.handleAgents)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("  MiniProbe Server v1.0")
	log.Printf("  Listen    : %s", addr)
	log.Printf("  Token     : %s", *token)
	log.Printf("  Dashboard : http://0.0.0.0%s", addr)
	log.Printf("  Agent WS  : ws://<HOST>%s/ws?token=%s", addr, *token)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Fatal(http.ListenAndServe(addr, nil))
}

// ─────────────────────────────────────────────────────────────────────────────
// Embedded Dashboard (no backticks inside — safe as Go raw string literal)
// ─────────────────────────────────────────────────────────────────────────────

const dashboardHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>MiniProbe 监控面板</title>
<style>
/* ── Reset ── */
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

/* ── Tokens ── */
:root {
  --bg:       #0f172a;
  --surface:  #1e293b;
  --surface2: #273548;
  --border:   #334155;
  --text:     #e2e8f0;
  --muted:    #94a3b8;
  --green:    #22c55e;
  --yellow:   #eab308;
  --red:      #ef4444;
  --blue:     #3b82f6;
}

/* ── Base ── */
body {
  font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
}

/* ── Header ── */
.header {
  background: linear-gradient(135deg, #1e293b 0%, #0f172a 100%);
  border-bottom: 1px solid var(--border);
  padding: 13px 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: sticky;
  top: 0;
  z-index: 100;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 11px;
}
.header-left h1 {
  font-size: 1.25rem;
  font-weight: 700;
  background: linear-gradient(135deg, #38bdf8, #818cf8);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}
.stats {
  display: flex;
  gap: 16px;
  align-items: center;
}
.stat {
  font-size: .82rem;
  color: var(--muted);
  display: flex;
  align-items: center;
  gap: 5px;
}
.stat b { color: var(--text); font-size: .9rem; }

/* ── Dots ── */
.dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}
.dot-green  { background: var(--green); box-shadow: 0 0 6px var(--green); }
.dot-red    { background: var(--red); }
.dot-pulse  {
  background: var(--green);
  box-shadow: 0 0 8px var(--green);
  animation: pulse 2s ease-in-out infinite;
}
@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50%       { opacity: .55; transform: scale(1.35); }
}

/* ── Toolbar ── */
.toolbar {
  padding: 14px 24px 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.refresh-hint { font-size: .78rem; color: var(--muted); }
.search {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 7px 14px;
  color: var(--text);
  font-size: .87rem;
  outline: none;
  width: 220px;
  transition: border-color .2s;
}
.search::placeholder { color: var(--muted); }
.search:focus { border-color: var(--blue); }

/* ── Grid ── */
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(330px, 1fr));
  gap: 16px;
  padding: 16px 24px 48px;
}

/* ── Card ── */
.card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 18px 20px 16px;
  transition: transform .18s, box-shadow .18s;
  position: relative;
  overflow: hidden;
}
.card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 32px rgba(0,0,0,.38);
}
.card.online  { border-left: 3px solid var(--green); }
.card.offline { border-left: 3px solid var(--red); opacity: .66; }

/* ── Card Header ── */
.card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}
.card-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: .95rem;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 65%;
}
.badge {
  font-size: .67rem;
  padding: 2px 9px;
  border-radius: 9999px;
  font-weight: 600;
  flex-shrink: 0;
}
.badge-on  { background: rgba(34,197,94,.13);  color: var(--green); border: 1px solid rgba(34,197,94,.35); }
.badge-off { background: rgba(239,68,68,.13);  color: var(--red);   border: 1px solid rgba(239,68,68,.35); }

/* ── Meta row ── */
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  margin-bottom: 14px;
}
.meta-item {
  font-size: .75rem;
  color: var(--muted);
}
.meta-item span { color: var(--text); }

/* ── Metrics bars ── */
.metrics { display: flex; flex-direction: column; gap: 9px; }
.metric  { display: flex; flex-direction: column; gap: 3px; }
.metric-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.metric-label { font-size: .72rem; color: var(--muted); }
.metric-val   { font-size: .76rem; font-weight: 600; }
.bar-track {
  background: var(--surface2);
  border-radius: 9999px;
  height: 5px;
  overflow: hidden;
}
.bar-fill {
  height: 5px;
  border-radius: 9999px;
  transition: width .55s ease;
}
.bar-green  { background: linear-gradient(90deg, #16a34a, #22c55e); }
.bar-yellow { background: linear-gradient(90deg, #ca8a04, #eab308); }
.bar-red    { background: linear-gradient(90deg, #dc2626, #ef4444); }

/* ── Network row ── */
.net-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 11px;
  padding-top: 10px;
  border-top: 1px solid var(--border);
  font-size: .76rem;
  flex-wrap: wrap;
  gap: 4px;
}
.net-item { display: flex; align-items: center; gap: 4px; }
.net-item .lbl { color: var(--muted); }
.net-item .val { color: var(--text); font-weight: 500; }

/* ── Load row ── */
.load-row {
  display: flex;
  gap: 10px;
  margin-top: 7px;
  font-size: .71rem;
  color: var(--muted);
}
.load-row span { color: var(--text); }

/* ── Empty state ── */
.empty {
  grid-column: 1 / -1;
  text-align: center;
  padding: 80px 24px;
  color: var(--muted);
}
.empty h2 { font-size: 1.1rem; color: var(--text); margin-bottom: 8px; }

/* ── Footer ── */
.footer {
  text-align: center;
  padding: 14px;
  font-size: .74rem;
  color: var(--muted);
  border-top: 1px solid var(--border);
}

/* ── Responsive ── */
@media (max-width: 600px) {
  .header  { padding: 10px 14px; }
  .grid    { padding: 12px 14px 28px; }
  .toolbar { padding: 10px 14px 0; }
  .search  { width: 150px; }
  .load-row { display: none; }
  .stats .stat:last-child { display: none; }
}
</style>
</head>
<body>

<div class="header">
  <div class="header-left">
    <div class="dot dot-pulse"></div>
    <h1>MiniProbe 监控面板</h1>
  </div>
  <div class="stats">
    <div class="stat">总计 <b id="stat-total">0</b></div>
    <div class="stat"><span class="dot dot-green"></span> 在线 <b id="stat-online">0</b></div>
    <div class="stat"><span class="dot dot-red"></span> 离线 <b id="stat-offline">0</b></div>
  </div>
</div>

<div class="toolbar">
  <div class="refresh-hint">自动刷新 &bull; <span id="countdown">3</span>s 后更新</div>
  <input class="search" type="text" id="search-box"
         placeholder="搜索主机名 / IP..."
         oninput="onSearch()">
</div>

<div class="grid" id="main-grid"></div>

<div class="footer">Powered by <b>MiniProbe</b> &nbsp;|&nbsp; 简单轻量的服务器监控探针</div>

<script>
var allAgents = [];
var searchKw  = '';
var cdSecs    = 3;
var cdTimer   = null;

function onSearch() {
  searchKw = document.getElementById('search-box').value.toLowerCase();
  renderCards();
}

// ─── Format helpers ─────────────────────────────────────────────────────────
function fmtBytes(b) {
  b = b || 0;
  if (b < 1024)           return b + ' B';
  if (b < 1048576)        return (b / 1024).toFixed(1) + ' KB';
  if (b < 1073741824)     return (b / 1048576).toFixed(1) + ' MB';
  return (b / 1073741824).toFixed(2) + ' GB';
}
function fmtRate(b) {
  b = b || 0;
  if (b < 1024)       return b + ' B/s';
  if (b < 1048576)    return (b / 1024).toFixed(1) + ' KB/s';
  return (b / 1048576).toFixed(1) + ' MB/s';
}
function fmtUptime(s) {
  s = s || 0;
  var d = Math.floor(s / 86400);
  var h = Math.floor((s % 86400) / 3600);
  var m = Math.floor((s % 3600) / 60);
  if (d > 0) return d + '天 ' + h + '时';
  if (h > 0) return h + '时 ' + m + '分';
  return m + '分';
}
function barCls(p) {
  return p >= 90 ? 'bar-red' : p >= 70 ? 'bar-yellow' : 'bar-green';
}
function valStyle(p) {
  return p >= 90 ? 'color:#ef4444' : p >= 70 ? 'color:#eab308' : 'color:#22c55e';
}
function esc(s) {
  return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}
function pct(p) { return Math.min(Math.max(p || 0, 0), 100); }

// ─── Build one card ──────────────────────────────────────────────────────────
function buildCard(a) {
  var on    = a.online;
  var css   = on ? 'online' : 'offline';
  var bdg   = on
    ? '<span class="badge badge-on">在线</span>'
    : '<span class="badge badge-off">离线</span>';
  var dotC  = on ? 'dot-pulse' : 'dot-red';
  var cpu   = a.cpu         || 0;
  var mp    = a.mem_percent  || 0;
  var dp    = a.disk_percent || 0;
  var hasLoad = (a.load1 !== undefined && a.load1 !== null);

  var loadHtml = '';
  if (hasLoad) {
    loadHtml = '<div class="load-row">负载均值:' +
      ' <span>' + (a.load1  || 0).toFixed(2) + '</span>' +
      ' <span>' + (a.load5  || 0).toFixed(2) + '</span>' +
      ' <span>' + (a.load15 || 0).toFixed(2) + '</span>' +
    '</div>';
  }

  return '<div class="card ' + css + '">' +

    '<div class="card-head">' +
      '<div class="card-name">' +
        '<span class="dot ' + dotC + '"></span>' +
        esc(a.hostname) +
      '</div>' +
      bdg +
    '</div>' +

    '<div class="meta">' +
      '<div class="meta-item">&#127760; <span>' + esc(a.ip || 'N/A') + '</span></div>' +
      '<div class="meta-item">&#128187; <span>' + esc(a.os || 'Unknown') + '</span></div>' +
      '<div class="meta-item">&#9201; <span>' + fmtUptime(a.uptime) + '</span></div>' +
    '</div>' +

    '<div class="metrics">' +

      '<div class="metric">' +
        '<div class="metric-row">' +
          '<span class="metric-label">&#9889; CPU</span>' +
          '<span class="metric-val" style="' + valStyle(cpu) + '">' + cpu.toFixed(1) + '%</span>' +
        '</div>' +
        '<div class="bar-track">' +
          '<div class="bar-fill ' + barCls(cpu) + '" style="width:' + pct(cpu) + '%"></div>' +
        '</div>' +
      '</div>' +

      '<div class="metric">' +
        '<div class="metric-row">' +
          '<span class="metric-label">&#129504; 内存 ' +
            fmtBytes(a.mem_used) + ' / ' + fmtBytes(a.mem_total) + '</span>' +
          '<span class="metric-val" style="' + valStyle(mp) + '">' + mp.toFixed(1) + '%</span>' +
        '</div>' +
        '<div class="bar-track">' +
          '<div class="bar-fill ' + barCls(mp) + '" style="width:' + pct(mp) + '%"></div>' +
        '</div>' +
      '</div>' +

      '<div class="metric">' +
        '<div class="metric-row">' +
          '<span class="metric-label">&#128190; 磁盘 ' +
            fmtBytes(a.disk_used) + ' / ' + fmtBytes(a.disk_total) + '</span>' +
          '<span class="metric-val" style="' + valStyle(dp) + '">' + dp.toFixed(1) + '%</span>' +
        '</div>' +
        '<div class="bar-track">' +
          '<div class="bar-fill ' + barCls(dp) + '" style="width:' + pct(dp) + '%"></div>' +
        '</div>' +
      '</div>' +

    '</div>' +

    '<div class="net-row">' +
      '<div class="net-item">&#8595; <span class="lbl">入站</span> <span class="val">' + fmtRate(a.net_in_rate)  + '</span></div>' +
      '<div class="net-item">&#8593; <span class="lbl">出站</span> <span class="val">' + fmtRate(a.net_out_rate) + '</span></div>' +
      '<div class="net-item">&#128230; <span class="lbl">总入</span> <span class="val">' + fmtBytes(a.net_in)    + '</span></div>' +
    '</div>' +

    loadHtml +

  '</div>';
}

// ─── Render grid ─────────────────────────────────────────────────────────────
function renderCards() {
  var list = allAgents.filter(function(a) {
    if (!searchKw) return true;
    return (a.hostname || '').toLowerCase().indexOf(searchKw) !== -1 ||
           (a.ip       || '').indexOf(searchKw)               !== -1 ||
           (a.os       || '').toLowerCase().indexOf(searchKw) !== -1;
  });

  var on  = list.filter(function(a) { return a.online; }).length;
  document.getElementById('stat-total').textContent   = list.length;
  document.getElementById('stat-online').textContent  = on;
  document.getElementById('stat-offline').textContent = list.length - on;

  var grid = document.getElementById('main-grid');
  if (list.length === 0) {
    grid.innerHTML =
      '<div class="empty">' +
        '<h2>&#127760; 暂无监控节点</h2>' +
        '<p>请在目标机器上运行 <code>agent.py</code></p>' +
      '</div>';
  } else {
    grid.innerHTML = list.map(buildCard).join('');
  }
}

// ─── Countdown ───────────────────────────────────────────────────────────────
function resetCountdown() {
  cdSecs = 3;
  document.getElementById('countdown').textContent = cdSecs;
  if (cdTimer) clearInterval(cdTimer);
  cdTimer = setInterval(function() {
    cdSecs = cdSecs > 1 ? cdSecs - 1 : 3;
    document.getElementById('countdown').textContent = cdSecs;
  }, 1000);
}

// ─── Fetch & refresh ─────────────────────────────────────────────────────────
async function refresh() {
  try {
    var resp  = await fetch('/api/agents');
    allAgents = await resp.json();
    renderCards();
  } catch (e) {
    console.warn('MiniProbe fetch error:', e);
  }
  resetCountdown();
}

setInterval(refresh, 3000);
refresh();
</script>
</body>
</html>`
