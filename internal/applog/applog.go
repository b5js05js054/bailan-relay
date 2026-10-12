// Package applog 提供内存环形日志缓冲 + 按日期切分的文件日志，
// 并对每行日志做分类（供前端筛选）。对应旧版 WebAppController 的日志逻辑。
package applog

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bailan-relay/internal/config"
)

const maxRecords = 5000

// Record 是一条日志记录。
type Record struct {
	ID       int    `json:"id"`
	Time     string `json:"time"`
	Category string `json:"category"`
	Text     string `json:"text"`
}

// Logger 是线程安全的日志器。
type Logger struct {
	mu       sync.Mutex
	records  []Record
	nextID   int
	file     *os.File
	fileDate string
}

// New 创建日志器。
func New() *Logger {
	_ = os.MkdirAll(config.LogDir(), 0o755)
	return &Logger{nextID: 1}
}

// Log 追加一行日志（会做分类、噪声过滤，并写入文件）。
func (l *Logger) Log(text string) {
	for _, line := range strings.Split(text, "\n") {
		l.appendLine(line)
	}
}

func (l *Logger) appendLine(text string) {
	clean := strings.TrimRight(text, "\r\n")
	if clean == "" || IsNoisy(clean) {
		return
	}
	if config.IsCustomer() {
		clean = sanitizeForCustomer(clean)
		if strings.TrimSpace(clean) == "" {
			return
		}
	}
	now := time.Now().Format("15:04:05")
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeFileLocked(now, clean)
	l.records = append(l.records, Record{
		ID:       l.nextID,
		Time:     now,
		Category: Classify(clean),
		Text:     clean,
	})
	l.nextID++
	if len(l.records) > maxRecords {
		l.records = l.records[len(l.records)-maxRecords:]
	}
}

func (l *Logger) writeFileLocked(now, text string) {
	date := time.Now().Format("20060102")
	if l.file == nil || l.fileDate != date {
		if l.file != nil {
			_ = l.file.Close()
		}
		_ = os.MkdirAll(config.LogDir(), 0o755)
		path := filepath.Join(config.LogDir(), "proxy_"+date+".log")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		l.file = f
		l.fileDate = date
	}
	_, _ = l.file.WriteString(now + " | " + text + "\n")
}

// GetLogs 返回 id > after 的日志（最多 limit 条）。
func (l *Logger) GetLogs(after, limit int) map[string]any {
	if after < 0 {
		after = 0
	}
	if limit < 1 {
		limit = 600
	}
	if limit > 1000 {
		limit = 1000
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var pending []Record
	for _, r := range l.records {
		if r.ID > after {
			pending = append(pending, r)
		}
	}
	entries := pending
	if len(entries) > limit {
		entries = entries[:limit]
	}
	latestID := after
	if l.nextID-1 > latestID {
		latestID = l.nextID - 1
	}
	if len(entries) > 0 {
		latestID = entries[len(entries)-1].ID
	}
	if entries == nil {
		entries = []Record{}
	}
	return map[string]any{
		"entries":      entries,
		"latest_id":    latestID,
		"buffer_count": len(l.records),
		"has_more":     len(pending) > len(entries),
	}
}

// Clear 清空内存缓冲。
func (l *Logger) Clear() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = nil
	return map[string]any{"ok": true, "latest_id": l.nextID - 1, "buffer_count": 0}
}

// LoadHistory 读取最新日志文件的最后 500 行并替换内存缓冲。
func (l *Logger) LoadHistory() map[string]any {
	path := latestLogFile()
	if path == "" {
		return map[string]any{"ok": false, "message": "没有找到历史日志文件"}
	}
	f, err := os.Open(path)
	if err != nil {
		return map[string]any{"ok": false, "message": "读取日志失败：" + err.Error()}
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, strings.TrimRight(scanner.Text(), "\r\n"))
	}
	if len(lines) > 500 {
		lines = lines[len(lines)-500:]
	}
	now := time.Now().Format("15:04:05")
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = nil
	for _, line := range lines {
		if line == "" || IsNoisy(line) {
			continue
		}
		if config.IsCustomer() {
			line = sanitizeForCustomer(line)
			if strings.TrimSpace(line) == "" {
				continue
			}
		}
		l.records = append(l.records, Record{
			ID:       l.nextID,
			Time:     now,
			Category: Classify(line),
			Text:     line,
		})
		l.nextID++
	}
	entries := append([]Record{}, l.records...)
	return map[string]any{
		"ok":           true,
		"message":      "已加载 " + itoa(len(entries)) + " 条历史日志",
		"entries":      entries,
		"latest_id":    l.nextID - 1,
		"buffer_count": len(entries),
	}
}

func latestLogFile() string {
	entries, err := os.ReadDir(config.LogDir())
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "proxy_") && strings.HasSuffix(e.Name(), ".log") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return filepath.Join(config.LogDir(), names[0])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// IsNoisy 过滤 mitm 握手/断开/ping 等无关噪声（沿用旧版关键词）。
func IsNoisy(line string) bool {
	text := strings.ToLower(line)
	tokens := []string{
		"client connect", "client disconnect",
		"server connect", "server disconnect",
		"websocket ping", "websocket pong", "received websocket",
		"alpn mismatch",
		"[blocked] recordanalyticsevent",
		"[blocked] batchrecordanalyticsevents",
		"[blocked] recordevent",
		"[blocked] recordasynctelemetry",
	}
	for _, t := range tokens {
		if strings.Contains(text, t) {
			return true
		}
	}
	return false
}

var (
	custOnce  sync.Once
	custDrops []string
	custPairs [][2]string
)

func custInit() {
	// 含实现细节（域名清单/源站直连/DNS/路径/监听端口）的整行直接丢弃。
	custDrops = []string{
		"目标域名", "拦截方式", "[PASSTHROUGH", "[passthrough",
		"[DNS]", "[放行]", "[BLOCKED]", "监听",
	}
	d := config.Default()
	add := func(from, to string) {
		if strings.TrimSpace(from) != "" {
			custPairs = append(custPairs, [2]string{from, to})
		}
	}
	add(d.RelayServerURL, "中继服务")
	if u, err := url.Parse(d.RelayServerURL); err == nil {
		add(u.Host, "中继服务")
		add(u.Hostname(), "中继服务")
	}
	add(d.RelayAuthToken, "***")
	for _, dom := range config.DefaultTargetDomains {
		add(dom, "目标服务")
	}
	for _, p := range config.DefaultTargetPaths {
		add(p, "接口")
	}
}

// sanitizeForCustomer 客户版下删除/脱敏会暴露核心信息的日志行。
// 返回空字符串表示该行应被丢弃。
func sanitizeForCustomer(line string) string {
	custOnce.Do(custInit)
	for _, t := range custDrops {
		if strings.Contains(line, t) {
			return ""
		}
	}
	out := line
	for _, p := range custPairs {
		out = strings.ReplaceAll(out, p[0], p[1])
	}
	return out
}

// Classify 给日志行分类：error/blocked/success/relay/passthrough/normal。
func Classify(text string) string {
	folded := strings.ToLower(text)
	if strings.Contains(folded, "error") || strings.Contains(text, "失败") || strings.Contains(text, "异常") {
		return "error"
	}
	if strings.Contains(folded, "[blocked]") {
		return "blocked"
	}
	for _, t := range []string{"[会话转发成功]", "[拦截成功]", "代理启动成功", "已自动重启"} {
		if strings.Contains(text, t) {
			return "success"
		}
	}
	if strings.Contains(text, "→ relay") || strings.Contains(text, "[WindsurfRelay]") || strings.Contains(text, "已启动") {
		return "relay"
	}
	if strings.Contains(folded, "[passthrough]") {
		return "passthrough"
	}
	for _, t := range []string{"已保存", "已停止", "已安装", "已卸载"} {
		if strings.Contains(text, t) {
			return "success"
		}
	}
	return "normal"
}
