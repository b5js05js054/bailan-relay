const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

// 公告「已知」持久化：同一则公告叉掉后本次与下次启动都不再弹；
// 后台改了标题/内容/级别视为新公告，会重新出现。
const ANNOUNCEMENT_DISMISSED_KEY = "bailan.announcement.dismissed";
function loadDismissedAnnouncement() {
    try { return localStorage.getItem(ANNOUNCEMENT_DISMISSED_KEY) || ""; } catch { return ""; }
}
function persistDismissedAnnouncement(key) {
    try { localStorage.setItem(ANNOUNCEMENT_DISMISSED_KEY, key); } catch { /* 存储不可用时仅本次生效 */ }
}

const state = {
    initialized: false,
    native: false,
    admin: false,
    edition: "full",
    page: "home",
    models: [],
    catalogSynced: false,
    missingNotified: "",
    config: {},
    status: {},
    logs: [],
    logCursor: 0,
    logFilter: "all",
    logSearch: "",
    streaming: true,
    pauseIndex: null,
    autoScroll: true,
    noticeSequence: 0,
    statusPolling: false,
    logPolling: false,
    renderFrame: 0,
    announcement: null,
    announcementClosed: loadDismissedAnnouncement(),
    update: null,
    updateDismissed: false,
    updating: false,
    updateProgressTimer: 0,
};

const mockState = {
    config: {
        relay_server_url: "http://127.0.0.1:5001",
        intercept_process: ["Devin.exe", "devin.exe"],
        devin_executable: "E:\\Devin\\Devin.exe",
        relay_auth_token: "",
        target_domains: [],
        target_paths: [],
        override_model: "swe-2-high",
        show_mitmweb: false,
        restart_devin_on_start: true,
    },
    status: {
        proxy_running: true,
        proxy_state: "running",
        proxy_message: "应用级拦截正在运行",
        proxy_has_web: true,
        relay_ok: true,
        relay_detail: "已连接",
        relay_checking: false,
        cert_file_exists: true,
        cert_installed: true,
        updated_at: "14:32:08",
        summary: "本地加速已开启 · 模型 SWE-2 · High",
        activity: {
            online: true,
            queue_count: 0,
            jwt_available: true,
            last_request_path: "",
            last_request_time: "14:31:52",
            is_processing: false,
            current_path: "",
            modify_count: 128,
        },
        notice: null,
        license: { activated: true, valid: true, card_key: "A2C4****DW7C", card_exp: "2026-12-31 23:59", remain_days: 90, message: "ok", billing_enabled: true, balance: 12.5, balance_text: "¥12.5000", currency: "¥" },
    },
    logs: [
        { id: 1, time: "14:31:46", category: "normal", text: "[Manager] 本地组件已就绪" },
        { id: 2, time: "14:31:47", category: "relay", text: "[会话转发] 请求已进入中继服务" },
        { id: 3, time: "14:31:49", category: "success", text: "[会话转发成功] 模型：SWE-2 · High" },
        { id: 4, time: "14:31:52", category: "relay", text: "[会话转发] 已写回响应" },
    ],
    account: { logged_in: false, user: null },
};

const mockApi = {
    async get_bootstrap() {
        return {
            app_title: "白岚 Devin 中继",
            ui_version: "3.0",
            edition: "full",
            admin: true,
            config_path: "C:\\Users\\Administrator\\AppData\\Local\\BaiLan Proxy Manager\\config.json",
            cert_path: "C:\\Users\\Administrator\\.mitmproxy\\mitmproxy-ca-cert.cer",
            models: [
                { id: "swe-2-medium", label: "SWE-2 · Medium", free: true },
                { id: "swe-2-high", label: "SWE-2 · High", free: true },
                { id: "swe-2-max", label: "SWE-2 · Max", free: true },
                { id: "glm-5-2", label: "GLM-5.2", free: true },
            ],
            config: structuredClone(mockState.config),
            status: structuredClone(mockState.status),
            license: structuredClone(mockState.status.license),
            account: structuredClone(mockState.account),
        };
    },
    async user_session() {
        return { ok: true, account: structuredClone(mockState.account) };
    },
    async user_login(email) {
        mockState.account = { logged_in: true, user: { id: 1, email, avatar_url: "favicon.ico", background_url: "" } };
        return { ok: true, message: "登录成功", account: structuredClone(mockState.account) };
    },
    async user_register(payload) {
        mockState.account = { logged_in: true, user: { id: 1, email: payload.email, avatar_url: payload.avatar, background_url: "" } };
        return { ok: true, message: "注册成功", account: structuredClone(mockState.account) };
    },
    async user_set_background(payload) {
        mockState.account.user.background_url = payload.image;
        return { ok: true, message: "背景已更换", account: structuredClone(mockState.account) };
    },
    async user_clear_background() {
        mockState.account.user.background_url = "";
        return { ok: true, message: "已恢复默认背景", account: structuredClone(mockState.account) };
    },
    async user_logout() {
        mockState.account = { logged_in: false, user: null };
        return { ok: true, message: "已退出登录", account: structuredClone(mockState.account) };
    },
    async user_upload_progress() {
        return { ok: true, progress: { phase: "done", sent: 1, total: 1, percent: 100 } };
    },
    async activate_license(code) {
        mockState.status.license = { activated: true, valid: true, card_key: String(code || "").slice(0, 4) + "****", card_exp: "2026-12-31 23:59", remain_days: 90, message: "激活成功" };
        return { ok: true, message: "激活成功（演示）", license: structuredClone(mockState.status.license) };
    },
    async redeem_topup(code) {
        const lic = mockState.status.license;
        lic.balance = Number(lic.balance || 0) + 10;
        lic.balance_text = `${lic.currency || "¥"}${lic.balance.toFixed(4)}`;
        return { ok: true, message: "充值成功 +10（演示）", license: structuredClone(lic) };
    },
    async license_status() {
        return { ok: true, license: structuredClone(mockState.status.license) };
    },
    async deactivate_license() {
        mockState.status.license = { activated: false, valid: false, message: "已清除" };
        return { ok: true, message: "已清除", license: structuredClone(mockState.status.license) };
    },
    async get_status() {
        mockState.status.updated_at = new Date().toLocaleTimeString("zh-CN", { hour12: false });
        return structuredClone(mockState.status);
    },
    async refresh_status() {
        return this.get_status();
    },
    async get_logs(afterId) {
        const entries = mockState.logs.filter((entry) => entry.id > Number(afterId || 0));
        return { entries: structuredClone(entries), latest_id: mockState.logs.at(-1)?.id || 0, buffer_count: mockState.logs.length };
    },
    async set_model(modelId) {
        mockState.config.override_model = modelId;
        return { ok: true, message: `模型路由已切换为 ${modelId}`, config: structuredClone(mockState.config) };
    },
    async set_option(name, value) {
        mockState.config[name] = Boolean(value);
        return { ok: true, config: structuredClone(mockState.config) };
    },
    async save_config(payload) {
        Object.assign(mockState.config, payload);
        return { ok: true, message: "配置已保存", config: structuredClone(mockState.config) };
    },
    async scan_processes(payload) {
        return { ok: true, message: "已扫描应用进程", processes: ["Devin.exe", "devin.exe", "language_server_windows_x64.exe"], executable: payload.devin_executable || "E:\\Devin\\Devin.exe" };
    },
    async browse_devin_executable() {
        return { ok: true, path: "E:\\Devin\\Devin.exe" };
    },
    async test_relay() {
        return { ok: true, detail: "HTTP 200 · 317ms", url: mockState.config.relay_server_url };
    },
    async test_dns(payload) {
        const domains = (payload?.target_domains?.length ? payload.target_domains : mockState.config.target_domains) || [];
        const results = domains.map((domain) => ({ domain, ok: true, ip: "203.0.113.7", via: "223.5.5.5" }));
        return { ok: true, message: `全部 ${results.length} 个域名均可解析真实 IP`, results, servers: ["223.5.5.5", "119.29.29.29"] };
    },
    async start_proxy() {
        mockState.status.proxy_state = "starting";
        mockState.status.proxy_message = "正在启动本地代理";
        setTimeout(() => {
            mockState.status.proxy_running = true;
            mockState.status.proxy_state = "running";
            mockState.status.proxy_message = "应用级拦截正在运行";
            mockState.status.proxy_has_web = true;
        }, 1400);
        return { ok: true, accepted: true, message: "正在启动本地代理" };
    },
    async stop_proxy() {
        mockState.status.proxy_state = "stopping";
        mockState.status.proxy_message = "正在停止拦截并关闭 Devin";
        setTimeout(() => {
            mockState.status.proxy_running = false;
            mockState.status.proxy_state = "idle";
            mockState.status.proxy_message = "拦截已停止，hosts 已恢复，Devin 已关闭";
            mockState.status.proxy_has_web = false;
        }, 800);
        return { ok: true, accepted: true, message: "正在停止拦截并关闭 Devin" };
    },
    async open_mitmweb() {
        return { ok: true, message: "已打开代理页面" };
    },
    async install_certificate() {
        mockState.status.cert_file_exists = true;
        mockState.status.cert_installed = true;
        return { ok: true, message: "证书已安装到系统根证书存储" };
    },
    async uninstall_certificate() {
        mockState.status.cert_installed = false;
        return { ok: true, message: "证书已从系统根证书存储卸载" };
    },
    async load_log_history() {
        return { ok: true, entries: structuredClone(mockState.logs), latest_id: mockState.logs.at(-1)?.id || 0, buffer_count: mockState.logs.length };
    },
    async clear_log_buffer() {
        mockState.logs = [];
        return { ok: true, latest_id: 4, buffer_count: 0 };
    },
    async relaunch_as_admin() {
        return { ok: true, message: "正在以管理员身份重新启动" };
    },
    async quit() {
        return { ok: true, message: "正在退出并恢复 hosts…" };
    },
    async heartbeat() {
        return { ok: true };
    },
    async ping_relay() {
        // 演示：多数为低延迟，偶尔抖动或丢包，便于预览各档颜色。
        await new Promise((resolve) => setTimeout(resolve, 20));
        const roll = Math.random();
        const at = new Date().toLocaleTimeString("zh-CN", { hour12: false });
        if (roll < 0.03) return { ok: false, ms: 0, status: 0, error: "i/o timeout", at };
        const base = roll < 0.75 ? 35 + Math.random() * 70 : roll < 0.92 ? 180 + Math.random() * 180 : 450 + Math.random() * 300;
        return { ok: true, ms: Math.round(base), status: 200, error: "", at };
    },
    async closing() {
        return { ok: true };
    },
    async open_url(url) {
        window.open(url, "_blank", "noopener");
        return { ok: true, message: "已在浏览器打开" };
    },
    async apply_update() {
        return { ok: false, message: "演示环境不支持在线更新" };
    },
    async update_progress() {
        return { ok: true, active: false, phase: "idle", downloaded: 0, total: 0, percent: 0, message: "" };
    },
    async get_usage(days, page, pageSize) {
        await new Promise((resolve) => setTimeout(resolve, 380));
        const lic = mockState.status.license || {};
        if (!lic.activated) return { ok: false, reason: "not_activated", message: "请先激活卡密后再查看用量" };
        days = Math.min(90, Math.max(1, Number(days) || 30));
        page = Math.max(1, Number(page) || 1);
        pageSize = Math.min(100, Math.max(1, Number(pageSize) || 20));
        const round6 = (value) => Number(value.toFixed(6));
        const sum = (list, key) => list.reduce((total, item) => total + item[key], 0);
        const all = mockUsageRecords();
        const todayStart = Date.parse(`${beijingDate(Date.now())}T00:00:00+08:00`);
        const dates = Array.from({ length: days }, (_, i) => beijingDate(todayStart - (days - 1 - i) * 86400000));
        const dateIndex = new Map(dates.map((date, i) => [date, i]));
        const inRange = all.filter((item) => dateIndex.has(beijingDate(Date.parse(item.time))));
        const byModel = new Map();
        inRange.forEach((item) => {
            let series = byModel.get(item.model);
            if (!series) {
                series = {
                    model: item.model, label: item.label, free: item.free, cost: 0, total_calls: 0, total_tokens: 0,
                    daily: new Array(days).fill(0), calls: new Array(days).fill(0), tokens: new Array(days).fill(0),
                };
                byModel.set(item.model, series);
            }
            const i = dateIndex.get(beijingDate(Date.parse(item.time)));
            const tokens = item.prompt_tokens + item.completion_tokens;
            series.cost += item.cost;
            series.daily[i] += item.cost;
            series.calls[i] += 1;
            series.tokens[i] += tokens;
            series.total_calls += 1;
            series.total_tokens += tokens;
        });
        const series = [...byModel.values()]
            .sort((a, b) => b.cost - a.cost)
            .map((item) => ({ ...item, cost: round6(item.cost), daily: item.daily.map(round6) }));
        const today = all.filter((item) => Date.parse(item.time) >= todayStart);
        const prompt = sum(all, "prompt_tokens");
        const completion = sum(all, "completion_tokens");
        const cacheRead = sum(all, "cache_read_tokens");
        const saved = all.reduce((total, item) => {
            const pin = MOCK_USAGE_MODELS.find((model) => model.model === item.model)?.pin || 0;
            return total + (item.cache_read_tokens * pin * 0.9) / 1e6;
        }, 0);
        const tokensOf = (list) => sum(list, "prompt_tokens") + sum(list, "completion_tokens");
        const freeAll = all.filter((item) => item.free);
        const freeRange = inRange.filter((item) => item.free);
        const paidRange = inRange.filter((item) => !item.free);
        return {
            ok: true,
            data: {
                ok: true,
                currency: lic.currency || "¥",
                billing_enabled: lic.billing_enabled !== false,
                balance: Number(lic.balance || 0),
                summary: {
                    total_cost: round6(sum(all, "cost")), total_requests: all.length,
                    free_requests: freeAll.length, paid_requests: all.length - freeAll.length,
                    today_cost: round6(sum(today, "cost")), today_requests: today.length, today_tokens: tokensOf(today),
                    total_tokens: prompt + completion, prompt_tokens: prompt, completion_tokens: completion,
                    cache_read_tokens: cacheRead, cache_write_tokens: 0,
                    cache_hit_rate: prompt ? cacheRead / prompt : 0, cache_saved: round6(saved),
                },
                range: {
                    days, start: dates[0], end: dates.at(-1), cost: round6(sum(inRange, "cost")), requests: inRange.length,
                    tokens: tokensOf(inRange), free_requests: freeRange.length, paid_requests: paidRange.length,
                    free_tokens: tokensOf(freeRange), paid_tokens: tokensOf(paidRange),
                },
                chart: { dates, series },
                records: { total: inRange.length, page, page_size: pageSize, items: inRange.slice((page - 1) * pageSize, page * pageSize) },
            },
        };
    },
};

// 演示用量：9 个模型（>7 个以预览「其他」合并），90 天固定种子随机记录。
const MOCK_USAGE_MODELS = [
    { model: "claude-opus-5-5-forge", label: "Claude Opus 5.5", weight: 40, pin: 5, pout: 25 },
    { model: "claude-fable-5-1-forge", label: "Claude Fable 5.1", weight: 18, pin: 3, pout: 15 },
    { model: "claude-fable-5-1-thinking-high", label: "Claude Fable 5.1 Thinking", weight: 8, pin: 3, pout: 15 },
    { model: "claude-opus-5-5-high", label: "Claude Opus 5.5 High", weight: 6, pin: 5, pout: 25 },
    { model: "gpt-5-2-codex", label: "GPT-5.2 Codex", weight: 5, pin: 1.25, pout: 10 },
    { model: "gemini-3-pro", label: "Gemini 3 Pro", weight: 4, pin: 2, pout: 12 },
    { model: "claude-sonnet-5-5", label: "Claude Sonnet 5.5", weight: 4, pin: 3, pout: 15 },
    { model: "glm-5-2-pro", label: "GLM-5.2 Pro", weight: 6, pin: 0, pout: 0, free: true },
    { model: "kimi-k3", label: "Kimi K3", weight: 5, pin: 0, pout: 0, free: true },
];

function beijingDate(ms) {
    return new Date(ms + 8 * 3600 * 1000).toISOString().slice(0, 10);
}

function mockRandom(seed) {
    let s = seed >>> 0;
    return () => {
        s = (s + 0x6d2b79f5) >>> 0;
        let t = s;
        t = Math.imul(t ^ (t >>> 15), t | 1);
        t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

function mockUsageRecords() {
    if (mockState.usageRecords) return mockState.usageRecords;
    const rand = mockRandom(20261010);
    const totalWeight = MOCK_USAGE_MODELS.reduce((total, model) => total + model.weight, 0);
    const pick = () => {
        let roll = rand() * totalWeight;
        return MOCK_USAGE_MODELS.find((model) => (roll -= model.weight) < 0) || MOCK_USAGE_MODELS[0];
    };
    const now = Date.now();
    const todayStart = Date.parse(`${beijingDate(now)}T00:00:00+08:00`);
    const records = [];
    for (let ago = 89; ago >= 0; ago -= 1) {
        const dayStart = todayStart - ago * 86400000;
        const span = Math.min(86400000, now - dayStart);
        const count = ago === 0 ? 2 : (rand() < 0.3 ? 0 : 1 + Math.floor(rand() * 5));
        for (let i = 0; i < count; i += 1) {
            const model = pick();
            const prompt = Math.round(20000 + rand() * 180000);
            const cacheRead = Math.round(prompt * (0.75 + rand() * 0.22));
            const completion = Math.round(300 + rand() * 6000);
            const cost = ((prompt - cacheRead) * model.pin + cacheRead * model.pin * 0.1 + completion * model.pout) / 1e6;
            records.push({
                time: new Date(dayStart + rand() * span).toISOString(),
                model: model.model, label: model.label, free: Boolean(model.free),
                prompt_tokens: prompt, completion_tokens: completion, cache_read_tokens: cacheRead, cache_write_tokens: 0,
                cost: Number(cost.toFixed(6)), estimated: rand() < 0.08,
            });
        }
    }
    records.sort((a, b) => b.time.localeCompare(a.time));
    mockState.usageRecords = records;
    return records;
}

function nativeApi() {
    return window.pywebview?.api || null;
}

async function callApi(method, ...args) {
    const api = nativeApi();
    state.native = Boolean(api);
    if (api && typeof api[method] === "function") {
        return api[method](...args);
    }
    if (typeof mockApi[method] === "function") {
        return mockApi[method](...args);
    }
    throw new Error(`API 不存在：${method}`);
}

function errorMessage(error) {
    if (!error) return "未知错误";
    if (typeof error === "string") return error;
    return error.message || error.detail || JSON.stringify(error);
}

function setButtonBusy(button, busy) {
    if (!button) return;
    button.classList.toggle("loading", busy);
    button.disabled = busy;
}

function setBadge(element, text, tone = "neutral") {
    if (!element) return;
    element.textContent = text;
    element.classList.remove("neutral", "success", "warning", "danger", "accent");
    element.classList.add(tone);
}

function toast(message, tone = "success", timeout = 3600) {
    const item = document.createElement("div");
    item.className = `toast ${tone}`;
    const copy = document.createElement("span");
    copy.textContent = message;
    item.append(copy);
    $("#toastStack").append(item);
    setTimeout(() => {
        item.classList.add("removing");
        setTimeout(() => item.remove(), 220);
    }, timeout);
}

function showNotice(message, tone = "success", action = false) {
    const notice = $("#globalNotice");
    notice.classList.remove("error", "warning");
    if (tone === "error" || tone === "warning") notice.classList.add(tone);
    $("#noticeMessage").textContent = message;
    $("#noticeAction").hidden = !action;
    const icon = $(".notice-icon use", notice);
    icon.setAttribute("href", tone === "success" ? "#i-check" : "#i-alert");
    notice.hidden = false;
}

function announcementKey(announcement) {
    return announcement ? `${announcement.level || ""}|${announcement.title || ""}|${announcement.content || ""}` : "";
}

function renderAnnouncement(announcement) {
    state.announcement = announcement || null;
    const banner = $("#announcementBanner");
    if (!banner) return;
    if (!state.announcement || !state.announcement.content || announcementKey(state.announcement) === state.announcementClosed) {
        banner.hidden = true;
        return;
    }
    banner.dataset.level = state.announcement.level || "info";
    $("#announcementTitle").textContent = state.announcement.title || "公告";
    $("#announcementContent").textContent = state.announcement.content;
    banner.hidden = false;
}

// refreshPublishConfig 周期拉取后台发布的模型目录/公告/更新，运行中无需重启即可生效。
async function refreshPublishConfig() {
    try {
        const data = await callApi("client_config");
        if (!data?.ok) return;
        // 模型目录有变动时重建下拉框（保持当前选中项），后台改模型列表约 15s 内生效
        const syncedChanged = Boolean(data.catalog_synced) !== state.catalogSynced;
        state.catalogSynced = Boolean(data.catalog_synced);
        if ((Array.isArray(data.models) && data.models.length
            && JSON.stringify(data.models) !== JSON.stringify(state.models)) || syncedChanged) {
            if (Array.isArray(data.models) && data.models.length) state.models = data.models;
            populateModels(state.config?.override_model || "");
        }
        renderAnnouncement(data.announcement || null);
        renderUpdate(data.update || null);
    } catch (error) { /* 刷新失败保持现状 */ }
}

function updateBlocked() {
    return Boolean(state.update && state.update.has_update && state.update.mandatory);
}

function renderUpdate(update) {
    state.update = update || null;
    const overlay = $("#updateOverlay");
    if (!overlay) return;
    if (!state.update || !state.update.has_update || (state.updateDismissed && !state.update.mandatory)) {
        overlay.hidden = true;
        return;
    }
    $("#updateTitle").textContent = `发现新版本 ${state.update.version || ""}`.trim();
    $("#updateCurrent").textContent = `当前版本 ${state.update.current || "—"}${state.update.mandatory ? " · 此更新为强制更新" : ""}`;
    $("#updateNote").textContent = state.update.note || "";
    if (!state.updating) {
        $("#updateStatus").textContent = state.update.mandatory ? "强制更新：不更新将无法启动拦截。" : "";
        renderUpdateProgress(null);
    }
    $("#updateLaterBtn").hidden = Boolean(state.update.mandatory || state.updating);
    overlay.hidden = false;
}

function formatDownloadBytes(value) {
    const bytes = Math.max(0, Number(value || 0));
    if (bytes < 1024) return `${Math.round(bytes)} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function renderUpdateProgress(progress) {
    const panel = $("#updateProgress");
    if (!panel) return;
    if (!progress) {
        panel.hidden = true;
        return;
    }

    const phase = String(progress.phase || "downloading");
    const total = Math.max(0, Number(progress.total || 0));
    const downloaded = Math.max(0, Number(progress.downloaded || 0));
    const percent = Math.min(100, Math.max(0, Number(progress.percent || 0)));
    const determinate = total > 0 || phase === "installing" || phase === "ready";
    const shownPercent = phase === "ready" ? 100 : percent;
    const track = $("#updateProgressTrack");
    const fill = $("#updateProgressFill");

    panel.hidden = false;
    panel.dataset.phase = phase;
    track.classList.toggle("indeterminate", !determinate && phase === "downloading");
    track.setAttribute("aria-valuenow", String(Math.round(shownPercent)));
    fill.style.width = determinate ? `${shownPercent}%` : "36%";
    $("#updateProgressPercent").textContent = determinate ? `${Math.round(shownPercent)}%` : "下载中";
    $("#updateProgressText").textContent = progress.message || "正在下载更新包…";
    $("#updateProgressBytes").textContent = total > 0
        ? `${formatDownloadBytes(downloaded)} / ${formatDownloadBytes(total)}`
        : (downloaded > 0 ? `已下载 ${formatDownloadBytes(downloaded)}` : "正在连接下载服务器");
}

async function pollUpdateProgress() {
    if (!state.updating) return;
    try {
        const progress = await callApi("update_progress");
        if (progress?.ok) renderUpdateProgress(progress);
    } catch (_error) { /* apply_update 的主请求会报告最终错误 */ }
}

function startUpdateProgressPolling() {
    if (state.updateProgressTimer) clearInterval(state.updateProgressTimer);
    pollUpdateProgress();
    state.updateProgressTimer = window.setInterval(pollUpdateProgress, 250);
}

function stopUpdateProgressPolling() {
    if (!state.updateProgressTimer) return;
    clearInterval(state.updateProgressTimer);
    state.updateProgressTimer = 0;
}

async function handleUpdateNow() {
    if (state.updating) return;
    const button = $("#updateNowBtn");
    const status = $("#updateStatus");
    state.updating = true;
    setButtonBusy(button, true);
    if (button) button.textContent = "正在下载更新…";
    if (status) status.textContent = "正在下载更新包，请稍候…";
    if ($("#updateLaterBtn")) $("#updateLaterBtn").hidden = true;
    renderUpdateProgress({ active: true, phase: "preparing", percent: 0, message: "正在准备更新…" });
    startUpdateProgressPolling();
    try {
        const result = await callApi("apply_update");
        if (!result?.ok) throw new Error(result?.message || "更新失败");
        stopUpdateProgressPolling();
        if (button) button.textContent = "正在重启…";
        renderUpdateProgress({ active: false, phase: "ready", percent: 100, message: "更新包已下载，正在重启…" });
        if (status) status.textContent = result.message || "更新包已下载，程序即将自动重启…";
    } catch (error) {
        stopUpdateProgressPolling();
        state.updating = false;
        setButtonBusy(button, false);
        if (button) button.textContent = "重新更新";
        if ($("#updateLaterBtn")) $("#updateLaterBtn").hidden = Boolean(state.update?.mandatory);
        if (status) status.textContent = `更新失败：${errorMessage(error)}`;
        renderUpdateProgress({ active: false, phase: "error", percent: 0, message: "更新下载失败" });
        toast(`更新失败：${errorMessage(error)}`, "error");
    }
}

function navigate(route) {
    if (!$("#page-" + route)) return;
    state.page = route;
    $$(".nav-item").forEach((item) => item.classList.toggle("active", item.dataset.route === route));
    $$(".page").forEach((page) => page.classList.toggle("active", page.dataset.page === route));
    if (route === "log") {
        scheduleLogRender();
        $("#navLive").hidden = true;
    } else if (state.status.proxy_running) {
        $("#navLive").hidden = false;
    }
    if (route === "usage") loadUsage();
}

function modelLabel(modelId) {
    return state.models.find((model) => model.id === modelId)?.label || modelId || "—";
}

function isFreeModel(modelOrId) {
    const model = typeof modelOrId === "string"
        ? state.models.find((item) => item.id === modelOrId)
        : modelOrId;
    if (model && typeof model.free === "boolean") return model.free;
    const id = String(model?.id || modelOrId || "").toLowerCase();
    return id.startsWith("swe-2") || id === "glm-5-2";
}

// 收费模型的标签：按次计费显示「按次 价格」，按量显示「高级 ×N」，都没有则回退「收费」。
function paidLabel(modelOrId) {
    const model = typeof modelOrId === "string"
        ? state.models.find((item) => item.id === modelOrId)
        : modelOrId;
    if (model?.billing_mode === "request") {
        const price = Number(model.price_request);
        return price > 0 ? `按次 ${Number(price.toFixed(4))}` : "按次";
    }
    const ratio = Number(model?.ratio);
    return ratio > 0 ? `高级 ×${Number(ratio.toFixed(4))}` : "收费";
}

function paidTitle(model) {
    if (model?.billing_mode === "request") {
        const price = Number(model.price_request);
        return `按次计费：每次请求固定扣 ${price > 0 ? price.toFixed(4) : "—"}，从卡密余额扣费`;
    }
    const pin = Number(model?.price_in), pout = Number(model?.price_out);
    let text = !(pin > 0) && !(pout > 0)
        ? "高级模型：按用量从卡密余额扣费"
        : `高级模型：每 100 万 tokens 输入 ${pin.toFixed(4)} / 输出 ${pout.toFixed(4)}，从卡密余额扣费`;
    if (model?.tier_threshold > 0 && model?.tier_ratio > 0) {
        text += `；输入超过 ${formatTokenSpec(model.tier_threshold)} 时整单 ×${model.tier_ratio}`;
    }
    return text;
}

// 输入：token 数；输出：规格简写（200000 → 200K，1000000 → 1M），用于上下文/阶梯阈值。
function formatTokenSpec(n) {
    n = Number(n);
    if (n >= 1e6 && n % 1e5 === 0) return `${n / 1e6}M`;
    if (n >= 1000 && n % 1000 === 0) return `${n / 1000}K`;
    return n.toLocaleString();
}

// 输入：模型；输出：能力摘要，如「200K 上下文 · 思考」，未知能力返回空串。
function modelCaps(model) {
    const caps = [];
    if (model?.context_window > 0) caps.push(`${formatTokenSpec(model.context_window)} 上下文`);
    if (model?.max_output > 0) caps.push(`输出 ${formatTokenSpec(model.max_output)}`);
    if (model?.supports_thinking) caps.push("思考");
    return caps.join(" · ");
}

// 模型系列分类规则：按 ID/展示名前缀归组，数组顺序即菜单分组顺序，未命中统一归「其他」。
const MODEL_SERIES = [
    { title: "Claude 系列", prefixes: ["claude", "anthropic"] },
    { title: "千问 Qwen", prefixes: ["qwen", "qwq", "qvq", "tongyi"] },
    { title: "小米 MiMo", prefixes: ["mimo", "xiaomi"] },
    { title: "Kimi", prefixes: ["kimi", "moonshot"] },
    { title: "GLM", prefixes: ["glm", "chatglm", "zhipu"] },
];

// 输入：模型；输出：命中的系列标题，未识别返回空串（由调用方归入「其他」）。
function modelSeriesTitle(model) {
    const id = String(model?.id || "").toLowerCase();
    const label = String(model?.label || "").toLowerCase();
    for (const series of MODEL_SERIES) {
        if (series.prefixes.some((prefix) => id.startsWith(prefix) || label.startsWith(prefix))) return series.title;
    }
    return "";
}

// 输入：无（读 state.models）；输出：按厂商系列分组的有序数组 [{ title, models }]，
// 组内保持后台目录顺序，空组剔除，「其他」固定垫底。
function groupedModels() {
    const groups = MODEL_SERIES.map((series) => ({ title: series.title, models: [] }));
    groups.push({ title: "其他", models: [] });
    state.models.forEach((model) => {
        const title = modelSeriesTitle(model) || "其他";
        groups.find((group) => group.title === title).models.push(model);
    });
    return groups.filter((group) => group.models.length);
}

// 模型已被后台隐藏/删除：目录已从后台同步成功，且所选模型不在目录里。
function isModelMissing(modelId) {
    return Boolean(state.catalogSynced && modelId && !state.models.some((model) => model.id === modelId));
}

function notifyModelMissing(modelId) {
    if (!isModelMissing(modelId)) {
        state.missingNotified = "";
        return;
    }
    if (state.missingNotified === modelId) return;
    state.missingNotified = modelId;
    const message = `所选模型「${modelId}」已下线或不存在，请重新选择模型，否则对话请求会被拒绝。`;
    showNotice(message, "warning");
    toast(message, "error", 6000);
}

function updateRouteBilling(modelId) {
    const missing = isModelMissing(modelId);
    const free = isFreeModel(modelId);
    const chip = $("#routeBilling");
    if (chip) {
        chip.textContent = missing ? "已下线" : (free ? "免费" : paidLabel(modelId));
        chip.dataset.tier = missing ? "gone" : (free ? "free" : "paid");
    }
    const hint = $("#configModelHint");
    if (hint) {
        hint.textContent = missing
            ? "该模型已在后台下线或不存在，请重新选择模型，否则对话请求会被拒绝。"
            : free
            ? "当前为免费模型。可选范围由后台模型目录下发。"
            : `当前为高级模型：${paidTitle(state.models.find((item) => item.id === modelId)).replace(/^[^：]*：/, "")}，余额不足将无法使用。可选范围由后台模型目录下发。`;
    }
}

// 输入：模型选择器；输出：选择器对应的菜单，菜单挂到 body 后仍可定位。
function getModelPickerMenu(picker) {
    return picker._modelPickerMenu || $(".model-picker-menu", picker);
}

// 输入：模型选择器与菜单；输出：菜单恢复到选择器内部并保持原有 DOM 顺序。
function restoreModelPickerMenu(picker, menu) {
    if (!menu || menu.parentElement === picker) return;
    const select = $("select", picker);
    picker.insertBefore(menu, select || null);
}

function closeModelPickers(except) {
    $$(".model-picker").forEach((picker) => {
        if (picker === except) return;
        picker.classList.remove("open");
        const trigger = $(".model-picker-trigger", picker);
        const menu = getModelPickerMenu(picker);
        if (trigger) trigger.setAttribute("aria-expanded", "false");
        if (menu) {
            menu.hidden = true;
            restoreModelPickerMenu(picker, menu);
        }
    });
}

function positionPickerMenu(picker) {
    const trigger = $(".model-picker-trigger", picker);
    const menu = getModelPickerMenu(picker);
    if (!trigger || !menu) return;
    const rect = trigger.getBoundingClientRect();
    const maxHeight = Math.min(320, Math.max(180, window.innerHeight - 24));
    menu.style.position = "fixed";
    menu.style.left = `${Math.round(rect.left)}px`;
    menu.style.width = `${Math.round(rect.width)}px`;
    menu.style.maxHeight = `${maxHeight}px`;
    const spaceBelow = window.innerHeight - rect.bottom - 12;
    if (spaceBelow < 220 && rect.top > spaceBelow) {
        menu.style.top = "auto";
        menu.style.bottom = `${Math.round(window.innerHeight - rect.top + 6)}px`;
    } else {
        menu.style.top = `${Math.round(rect.bottom + 6)}px`;
        menu.style.bottom = "auto";
    }
}

// 输入：模型菜单与当前选项；输出：仅调整菜单自身的纵向滚动位置。
function scrollSelectedModelIntoMenu(menu, selected) {
    const menuRect = menu.getBoundingClientRect();
    const selectedRect = selected.getBoundingClientRect();
    if (selectedRect.top < menuRect.top) {
        menu.scrollTop -= menuRect.top - selectedRect.top;
    } else if (selectedRect.bottom > menuRect.bottom) {
        menu.scrollTop += selectedRect.bottom - menuRect.bottom;
    }
}

function setPickerOpen(picker, open) {
    const trigger = $(".model-picker-trigger", picker);
    const menu = getModelPickerMenu(picker);
    if (!trigger || !menu) return;
    if (open) {
        closeModelPickers(picker);
        picker.classList.add("open");
        trigger.setAttribute("aria-expanded", "true");
        document.body.appendChild(menu);
        menu.hidden = false;
        positionPickerMenu(picker);
        const selected = $(".model-option.selected", menu);
        if (selected) scrollSelectedModelIntoMenu(menu, selected);
    } else {
        picker.classList.remove("open");
        trigger.setAttribute("aria-expanded", "false");
        menu.hidden = true;
        restoreModelPickerMenu(picker, menu);
    }
}

function paintPickerTrigger(picker, modelId) {
    const model = state.models.find((item) => item.id === modelId);
    const name = $(".model-picker-name", picker);
    const meta = $(".model-picker-meta", picker);
    const chip = $(".model-picker-trigger .billing-chip", picker);
    const free = isFreeModel(model || modelId);
    const missing = isModelMissing(modelId);
    picker.classList.toggle("is-missing", missing);
    if (name) name.textContent = model?.label || modelId || "选择模型";
    if (meta) {
        const caps = modelCaps(model);
        const base = missing ? "该模型已下线或不存在，请重新选择" : (free ? "当前为免费模型" : "当前为高级模型（按余额扣费）");
        meta.textContent = !missing && caps ? `${base} · ${caps}` : base;
    }
    if (chip) {
        chip.textContent = missing ? "已下线" : (free ? "免费" : paidLabel(model || modelId));
        chip.dataset.tier = missing ? "gone" : (free ? "free" : "paid");
    }
}

function appendPickerGroup(menu, title, models, selected) {
    if (!models.length) return;
    const group = document.createElement("div");
    group.className = "model-group";
    const heading = document.createElement("p");
    heading.className = "model-group-title";
    heading.textContent = title;
    group.append(heading);
    models.forEach((model) => {
        const option = document.createElement("button");
        option.type = "button";
        option.className = "model-option";
        option.dataset.id = model.id;
        option.setAttribute("role", "option");
        option.setAttribute("aria-selected", String(model.id === selected));
        if (model.id === selected) option.classList.add("selected");
        const copy = document.createElement("span");
        copy.className = "model-option-copy";
        const name = document.createElement("strong");
        name.textContent = model.label;
        const id = document.createElement("small");
        const caps = modelCaps(model);
        id.textContent = caps ? `${model.id} · ${caps}` : model.id;
        copy.append(name, id);
        const mark = document.createElement("span");
        mark.className = `billing-chip ${isFreeModel(model) ? "free" : "paid"}`;
        mark.textContent = isFreeModel(model) ? "免费" : paidLabel(model);
        if (!isFreeModel(model)) mark.title = paidTitle(model);
        option.append(copy, mark);
        group.append(option);
    });
    menu.append(group);
}

function populateModels(selected) {
    const groups = groupedModels();
    const ordered = groups.flatMap((group) => group.models);
    for (const select of [$("#homeModel"), $("#configModel")]) {
        if (!select) continue;
        select.replaceChildren();
        ordered.forEach((model) => {
            const option = document.createElement("option");
            option.value = model.id;
            option.textContent = model.label;
            select.append(option);
        });
        select.value = selected;
    }
    $$(".model-picker").forEach((picker) => {
        const menu = getModelPickerMenu(picker);
        if (!menu) return;
        menu.replaceChildren();
        groups.forEach((group) => appendPickerGroup(menu, group.title, group.models, selected));
        paintPickerTrigger(picker, selected);
    });
    $("#routeTarget").textContent = selected || "—";
    updateRouteBilling(selected);
    renderChips();
    notifyModelMissing(selected);
}

function applyModelSelection(modelId, persist) {
    $("#homeModel").value = modelId;
    $("#configModel").value = modelId;
    populateModels(modelId);
    closeModelPickers();
    if (persist) changeModel(modelId);
}

function applyConfig(config) {
    state.config = { ...state.config, ...config };
    const cfg = state.config;
    $("#relayUrl").value = cfg.relay_server_url || "";
    $("#devinExecutable").value = cfg.devin_executable || "";
    $("#authToken").value = cfg.relay_auth_token || "";
    if ($("#dnsServers")) $("#dnsServers").value = (cfg.dns_servers || []).join("\n");
    if ($("#stubPaths")) $("#stubPaths").value = (cfg.stub_paths || []).join("\n");
    $("#targetDomains").value = (cfg.target_domains || []).join("\n");
    $("#targetPaths").value = (cfg.target_paths || []).join("\n");
    $("#restartToggle").checked = Boolean(cfg.restart_devin_on_start);
    if (state.models.length) populateModels(cfg.override_model);
    renderChips();
    renderHeartbeat();
}

function renderAdmin(admin) {
    state.admin = admin;
    $("#adminBanner").hidden = admin;
    $("#operatorRole").textContent = admin ? "管理员权限" : "权限受限";
    $("#operatorDot").classList.toggle("admin", admin);
    $("#operatorDot").classList.toggle("limited", !admin);
    $("#installCertBtn").disabled = !admin;
    $("#uninstallCertBtn").disabled = !admin;
    renderChips();
}

function formatActivity(activity) {
    if (!activity || !activity.online) {
        $("#activityServer").textContent = "不可达";
        $("#activityQueue").textContent = "—";
        $("#activityJwt").textContent = "—";
        $("#activityLast").textContent = "—";
        $("#activityCurrent").textContent = "不可达";
        $("#activityTotal").textContent = "累计处理 —";
        return;
    }
    const lastPath = String(activity.last_request_path || "");
    const currentPath = String(activity.current_path || "");
    $("#activityServer").textContent = "在线";
    $("#activityQueue").textContent = activity.queue_count ? `${activity.queue_count} 排队` : "空闲";
    $("#activityJwt").textContent = activity.jwt_available ? "可用" : "不可用";
    $("#activityLast").textContent = lastPath ? `${lastPath.split("/").at(-1)} ${activity.last_request_time || ""}`.trim() : "暂无";
    $("#activityCurrent").textContent = activity.is_processing && currentPath ? currentPath.split("/").at(-1) : "空闲";
    $("#activityTotal").textContent = `累计处理 ${activity.modify_count || 0}`;
}

function renderCertificate(status) {
    const exists = status.cert_file_exists;
    const installed = status.cert_installed;
    const hero = $("#certificateHero");
    if (exists == null || installed == null) {
        hero.dataset.state = "checking";
        $("#certificateTitle").textContent = "正在检查系统信任";
        $("#certificateDescription").textContent = "正在读取证书文件和 Windows 根证书存储。";
        setBadge($("#certificateBadge"), "检测中", "neutral");
        $("#certFileText").textContent = "正在检测";
        $("#certInstallText").textContent = "正在检测";
        setBadge($("#certFileBadge"), "检测中", "neutral");
        setBadge($("#certInstallBadge"), "检测中", "neutral");
        return;
    }
    if (installed) {
        hero.dataset.state = "trusted";
        $("#certificateTitle").textContent = "系统信任已建立";
        $("#certificateDescription").textContent = "本地根证书已安装，HTTPS 应用流量可以进入本地拦截服务。";
        setBadge($("#certificateBadge"), "已信任", "success");
    } else if (exists === false || installed === false) {
        hero.dataset.state = "untrusted";
        $("#certificateTitle").textContent = exists ? "证书尚未安装" : "尚未生成本地证书";
        $("#certificateDescription").textContent = "安装根证书后，HTTPS 拦截才能正常建立连接。";
        setBadge($("#certificateBadge"), "未信任", "warning");
    } else {
        hero.dataset.state = "checking";
    }
    $("#certFileText").textContent = exists ? "本地证书文件可用" : "未找到证书文件";
    $("#certInstallText").textContent = installed ? "已写入受信任的根证书" : "尚未写入系统证书存储";
    setBadge($("#certFileBadge"), exists ? "已存在" : "不存在", exists ? "success" : "danger");
    setBadge($("#certInstallBadge"), installed ? "已安装" : "未安装", installed ? "success" : "danger");
}

function renderStatus(status) {
    if (!status) return;
    state.status = { ...state.status, ...status };
    const current = state.status;
    const running = Boolean(current.proxy_running);
    const operation = current.proxy_state || (running ? "running" : "idle");
    const busy = operation === "starting" || operation === "stopping";
    const board = $("#statusBoard");
    board.dataset.state = running ? "running" : (operation === "error" ? "error" : "idle");
    board.dataset.operation = busy ? operation : "";

    if (operation === "starting") {
        $("#captureTitle").textContent = "正在启动应用级拦截";
        setBadge($("#captureBadge"), "启动中", "accent");
        $("#controlState").textContent = "启动中";
        $("#operationPercent").textContent = "···";
    } else if (operation === "stopping") {
        $("#captureTitle").textContent = "正在停止拦截并关闭 Devin";
        setBadge($("#captureBadge"), "停止中", "warning");
        $("#controlState").textContent = "停止中";
        $("#operationPercent").textContent = "···";
    } else if (running) {
        $("#captureTitle").textContent = "本地拦截已开启";
        setBadge($("#captureBadge"), "运行中", "success");
        $("#controlState").textContent = "运行中";
        $("#operationPercent").textContent = "100%";
    } else if (operation === "error") {
        $("#captureTitle").textContent = "本地拦截启动失败";
        setBadge($("#captureBadge"), "异常", "danger");
        $("#controlState").textContent = "异常";
        $("#operationPercent").textContent = "0%";
    } else {
        $("#captureTitle").textContent = "本地拦截未开启";
        setBadge($("#captureBadge"), "待机", "warning");
        $("#controlState").textContent = "待机";
        $("#operationPercent").textContent = "0%";
    }

    $("#operationMessage").textContent = current.proxy_message || (running ? "应用级拦截正在运行" : "等待启动");
    $("#operationProgress").style.width = busy ? "42%" : (running ? "100%" : "0%");
    $("#updatedAt").textContent = current.updated_at ? `更新 ${current.updated_at}` : "正在检测";
    $("#statusSummary").textContent = current.summary || "正在读取运行状态。";
    setBadge($("#proxyBadge"), running ? "运行中" : (busy ? "处理中" : "未运行"), running ? "success" : (busy ? "accent" : "danger"));
    if (current.relay_checking) {
        setBadge($("#relayBadge"), "检测中", "neutral");
    } else {
        setBadge($("#relayBadge"), current.relay_ok ? "已连接" : "不可达", current.relay_ok ? "success" : "danger");
    }

    renderLicense(current.license);
    renderReconnect(current.relay_reconnect);
    const licenseValid = !current.license || current.license.valid;
    $("#startProxyBtn").disabled = running || busy || !licenseValid || updateBlocked();
    $("#stopProxyBtn").disabled = (!running && operation !== "starting") || operation === "stopping";
    $("#navLive").hidden = state.page === "log" || !running;
    formatActivity(current.activity);
    renderCertificate(current);
    renderChips();

    const notice = current.notice;
    if (notice && Number(notice.sequence || 0) > state.noticeSequence) {
        state.noticeSequence = Number(notice.sequence || 0);
        showNotice(notice.message, notice.tone || "success", notice.action === "log");
        if (notice.action === "log" && notice.auto_navigate) navigate("log");
    }
}

function renderReconnect(info) {
    const box = $("#relayReconnect");
    if (!box) return;
    const r = info || {};
    let view = "";
    if (r.waiting) {
        view = "waiting";
        $("#reconnectTitle").textContent = "服务器正在重启，正在重新连接…";
        const count = Number(r.count) || 1;
        $("#reconnectText").textContent =
            `${count > 1 ? `${count} 个对话` : "对话"}请求已暂存，已等待 ${Number(r.seconds) || 0} 秒（第 ${Number(r.attempts) || 1} 次尝试，最长 ${Number(r.window) || 60} 秒），连上后自动继续，请不要关闭 Devin。`;
    } else if (r.recovered) {
        view = "recovered";
        $("#reconnectTitle").textContent = "已重新连接服务器";
        $("#reconnectText").textContent = "对话已自动继续。";
    } else if (r.failed) {
        view = "failed";
        $("#reconnectTitle").textContent = "服务器暂时无法连接";
        $("#reconnectText").textContent = "重连超时，请稍后在 Devin 里重新发送这条消息。";
    }
    box.hidden = !view;
    if (view) box.dataset.state = view;
}

function renderLicense(license) {
    const panel = $("#licensePanel");
    if (!panel) return;
    const info = license || { activated: false, valid: false };
    const badge = $("#licenseBadge");
    const infoText = $("#licenseInfo");
    const form = $("#licenseForm");
    const meta = $("#licenseMeta");

    panel.dataset.state = info.valid ? "valid" : info.activated ? "invalid" : "pending";

    if (info.valid) {
        setBadge(badge, "已授权", "success");
        const days = Number(info.remain_days || 0);
        infoText.textContent = days > 0
            ? `授权有效，剩余约 ${days} 天。`
            : "授权有效。";
        if (form) form.hidden = true;
        if (meta) {
            meta.hidden = false;
            $("#licenseCode").textContent = info.card_key ? `卡密 ${info.card_key}` : "";
            $("#licenseExpiry").textContent = info.card_exp ? `到期 ${info.card_exp}` : "";
        }
        renderBalance(info, true);
    } else if (info.activated) {
        setBadge(badge, "已失效", "danger");
        infoText.textContent = info.message ? `授权失效：${info.message}` : "授权已失效，请重新激活。";
        if (form) form.hidden = false;
        if (meta) meta.hidden = true;
        renderBalance(info, false);
    } else {
        setBadge(badge, "未激活", "warning");
        infoText.textContent = info.message
            ? info.message
            : "请输入卡密并激活后方可启动拦截。授权与本机绑定，一卡一机。";
        if (form) form.hidden = false;
        if (meta) meta.hidden = true;
        renderBalance(info, false);
    }
}

// 余额与充值入口：仅在授权有效且后台开启余额计费时显示。
function renderBalance(info, authorized) {
    const show = Boolean(authorized && info && info.billing_enabled);
    const label = $("#licenseBalance");
    const redeem = $("#licenseRedeem");
    if (label) {
        label.hidden = !show;
        if (show) label.textContent = `余额 ${info.balance_text || "—"}`;
    }
    if (redeem) redeem.hidden = !show;
}

async function handleRedeemTopup() {
    const input = $("#redeemInput");
    const button = $("#redeemBtn");
    const code = (input?.value || "").trim();
    if (!code) {
        showNotice("请输入充值码", "error");
        return;
    }
    setButtonBusy(button, true);
    try {
        const res = await callApi("redeem_topup", code);
        if (res && res.ok) {
            showNotice(res.message || "充值成功", "success");
            if (input) input.value = "";
        } else {
            showNotice((res && res.message) || "充值失败", "error");
        }
        if (res && res.license) {
            state.status.license = res.license;
            renderLicense(res.license);
        }
    } catch (error) {
        showNotice(`充值失败：${errorMessage(error)}`, "error");
    } finally {
        setButtonBusy(button, false);
    }
}

async function handleActivateLicense() {
    const input = $("#licenseInput");
    const button = $("#activateLicenseBtn");
    const code = (input?.value || "").trim();
    if (!code) {
        showNotice("请输入卡密", "error");
        return;
    }
    setButtonBusy(button, true);
    try {
        const res = await callApi("activate_license", code);
        if (res && res.ok) {
            showNotice(res.message || "激活成功", "success");
            if (input) input.value = "";
            renderLicense(res.license);
            state.status.license = res.license;
            renderStatus(state.status);
            pollStatus(false);
        } else {
            showNotice((res && res.message) || "激活失败", "error");
            if (res) renderLicense(res.license);
        }
    } catch (error) {
        showNotice(`激活失败：${errorMessage(error)}`, "error");
    } finally {
        setButtonBusy(button, false);
    }
}

async function handleChangeLicense() {
    const ok = await confirmAction(
        "更换卡密",
        "确定要清除当前卡密授权吗？清除后需要重新输入卡密激活。",
        "清除",
    );
    if (!ok) return;
    try {
        const res = await callApi("deactivate_license");
        renderLicense(res && res.license);
        state.status.license = res && res.license;
        renderStatus(state.status);
    } catch (error) {
        showNotice(`操作失败：${errorMessage(error)}`, "error");
    }
}

function logMatches(entry) {
    const categoryMap = {
        success: ["success"],
        error: ["error", "blocked"],
        relay: ["relay"],
    };
    if (state.logFilter !== "all" && !categoryMap[state.logFilter]?.includes(entry.category)) return false;
    return !state.logSearch || String(entry.text || "").toLocaleLowerCase().includes(state.logSearch);
}

function scheduleLogRender() {
    if (state.renderFrame) cancelAnimationFrame(state.renderFrame);
    state.renderFrame = requestAnimationFrame(() => {
        state.renderFrame = 0;
        renderLogs();
    });
}

function renderLogs() {
    const source = state.pauseIndex === null ? state.logs : state.logs.slice(0, state.pauseIndex);
    const visible = source.filter(logMatches);
    const container = $("#logLines");
    const fragment = document.createDocumentFragment();
    visible.forEach((entry) => {
        const line = document.createElement("div");
        line.className = `log-line ${entry.category || "normal"}`;
        const time = document.createElement("time");
        time.className = "log-time";
        time.textContent = entry.time || "--:--:--";
        const marker = document.createElement("span");
        marker.className = "log-marker";
        const text = document.createElement("span");
        text.className = "log-text";
        text.textContent = entry.text || "";
        line.title = "点击复制";
        line.append(marker, text, time);
        fragment.append(line);
    });
    container.replaceChildren(fragment);
    renderLogPreview();

    const pending = state.pauseIndex === null ? 0 : Math.max(0, state.logs.length - state.pauseIndex);
    $("#logCount").textContent = `可见 ${visible.length} 行 / 缓冲 ${state.logs.length} 行${pending ? `（待显示 ${pending} 行）` : ""}`;
    const empty = $("#logEmpty");
    empty.hidden = visible.length > 0;
    if (!visible.length) {
        if (pending) {
            $("strong", empty).textContent = "日志显示已暂停";
            $("p", empty).textContent = `已有 ${pending} 条新日志等待显示。`;
        } else if (state.logs.length) {
            $("strong", empty).textContent = "没有匹配的日志";
            $("p", empty).textContent = "请调整类型筛选或搜索关键词。";
        } else {
            $("strong", empty).textContent = "暂无运行记录";
            $("p", empty).textContent = "启动代理后，新日志会实时显示在这里。";
        }
    }
    setBadge($("#logLiveBadge"), state.streaming ? "实时" : "已暂停", state.streaming ? "success" : "warning");
    $("#streamBtn span").textContent = state.streaming ? "暂停显示" : "恢复显示";
    $("#streamBtn use").setAttribute("href", state.streaming ? "#i-pause" : "#i-play");
    if (state.autoScroll && state.streaming) {
        const viewport = $("#logViewport");
        viewport.scrollTop = viewport.scrollHeight;
    }
}

/* ---------- 最近日志（工作台预览）---------- */

const LOG_PREVIEW_COUNT = 6;

function renderLogPreview() {
    const container = $("#logPreview");
    if (!container) return;
    const recent = state.logs.slice(-LOG_PREVIEW_COUNT).reverse();
    const fragment = document.createDocumentFragment();
    recent.forEach((entry) => {
        const line = document.createElement("div");
        line.className = `preview-line ${entry.category || "normal"}`;
        line.title = "点击复制";
        const marker = document.createElement("span");
        marker.className = "log-marker";
        const text = document.createElement("span");
        text.className = "log-text";
        text.textContent = entry.text || "";
        const time = document.createElement("time");
        time.className = "log-time";
        time.textContent = entry.time || "--:--:--";
        line.append(marker, text, time);
        fragment.append(line);
    });
    container.replaceChildren(fragment);
    const empty = $("#logPreviewEmpty");
    if (empty) empty.hidden = recent.length > 0;
}

async function copyLogLine(line) {
    const text = $(".log-text", line)?.textContent || "";
    if (!text) return;
    try {
        await navigator.clipboard.writeText(text);
        toast("已复制该行日志", "success", 1600);
    } catch (_error) {
        toast("复制失败，请手动选择文本", "error", 2200);
    }
}

/* ---------- 状态标签（主控卡片）---------- */

function renderChips() {
    const row = $("#chipRow");
    if (!row) return;
    const chips = [];
    const cfg = state.config || {};
    if (cfg.override_model) {
        if (isModelMissing(cfg.override_model)) {
            chips.push({ text: `模型 ${cfg.override_model} 已下线`, tone: "danger" });
        } else {
            chips.push({ text: `模型 ${modelLabel(cfg.override_model)}`, tone: isFreeModel(cfg.override_model) ? "success" : "warning" });
        }
    }
    if (state.edition !== "customer" && cfg.relay_server_url) {
        let host = cfg.relay_server_url;
        try { host = new URL(cfg.relay_server_url).host; } catch (_error) { /* 使用原文 */ }
        chips.push({ text: `中继 ${host}`, tone: "neutral" });
    }
    if (state.status.proxy_running) chips.push({ text: "hosts 已劫持", tone: "accent" });
    const license = state.status.license;
    if (license && license.valid) {
        const days = Number(license.remain_days);
        chips.push({ text: Number.isFinite(days) && days >= 0 ? `授权有效 · 剩 ${days} 天` : "授权有效", tone: "success" });
    }
    chips.push(state.admin ? { text: "管理员权限", tone: "neutral" } : { text: "权限受限", tone: "danger" });
    row.replaceChildren(...chips.map((chip) => {
        const el = document.createElement("span");
        el.className = `chip ${chip.tone}`;
        el.textContent = chip.text;
        return el;
    }));
}

/* ---------- 心跳监控：中继网络延迟 ---------- */

const HB_INTERVAL_MS = 3000;
const HB_MAX_SAMPLES = 60;
const HB_GOOD_MS = 150;
const HB_FAIR_MS = 400;
const HB_CHART_W = 400;
const HB_CHART_H = 96;

const heartbeatState = { samples: [], busy: false, timer: 0, lastAt: "", unconfigured: false };

/** 延迟分档：good <150ms，fair <400ms，其余为 poor；探测失败为 down。 */
function hbTier(sample) {
    if (!sample || !sample.ok) return "down";
    if (sample.ms < HB_GOOD_MS) return "good";
    if (sample.ms < HB_FAIR_MS) return "fair";
    return "poor";
}

const HB_TIER_TEXT = {
    idle: "等待首次探测",
    good: "网络畅通",
    fair: "延迟一般",
    poor: "延迟较高",
    down: "无法连接",
    unset: "未配置中继地址",
};
const HB_TIER_BADGE = { good: "success", fair: "warning", poor: "danger", down: "danger", idle: "neutral", unset: "neutral" };

function hbChartGeometry(samples) {
    const okMs = samples.filter((s) => s.ok).map((s) => s.ms);
    const ceiling = Math.max(120, Math.ceil((Math.max(0, ...okMs) * 1.2) / 20) * 20);
    const step = HB_CHART_W / (HB_MAX_SAMPLES - 1);
    const offset = HB_MAX_SAMPLES - samples.length;
    const xOf = (index) => (offset + index) * step;
    const yOf = (ms) => HB_CHART_H - 6 - (Math.min(ms, ceiling) / ceiling) * (HB_CHART_H - 14);
    const segments = [];
    let current = [];
    samples.forEach((sample, index) => {
        if (sample.ok) {
            current.push([xOf(index), yOf(sample.ms)]);
        } else if (current.length) {
            segments.push(current);
            current = [];
        }
    });
    if (current.length) segments.push(current);
    return { segments, xOf, step };
}

function hbPaths(segments) {
    let line = "";
    let area = "";
    segments.forEach((points) => {
        const fmt = ([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`;
        if (points.length === 1) {
            const [x, y] = points[0];
            line += `M${fmt([x, y])} L${fmt([x + 0.01, y])} `;
            return;
        }
        line += `M${points.map(fmt).join(" L")} `;
        const first = points[0];
        const last = points[points.length - 1];
        area += `M${first[0].toFixed(1)},${HB_CHART_H} L${points.map(fmt).join(" L")} L${last[0].toFixed(1)},${HB_CHART_H} Z `;
    });
    return { line: line.trim(), area: area.trim() };
}

function renderHeartbeat() {
    const samples = heartbeatState.samples;
    const latest = samples.at(-1);
    let tier = latest ? hbTier(latest) : "idle";
    if (heartbeatState.unconfigured) tier = "unset";

    const panel = $("#heartbeatPanel");
    if (!panel) return;
    panel.dataset.state = tier;
    const mini = $("#hbMini");
    if (mini) mini.dataset.state = tier;

    const showMs = latest && latest.ok && tier !== "unset";
    $("#hbValue").textContent = showMs ? String(latest.ms) : "--";
    $("#hbQuality").textContent = HB_TIER_TEXT[tier];
    $("#hbMiniValue").textContent = showMs ? `${latest.ms}ms` : (tier === "down" ? "离线" : "--");
    setBadge($("#hbBadge"), showMs ? `${latest.ms} ms` : (tier === "down" ? "离线" : (tier === "unset" ? "未配置" : "检测中")), HB_TIER_BADGE[tier]);

    const liveText = $("#hbLiveText");
    if (liveText) {
        liveText.textContent = heartbeatState.lastAt ? `实时 · ${heartbeatState.lastAt}` : "连接中";
    }
    const target = $("#hbTarget");
    if (target) {
        const cfg = state.config || {};
        if (state.edition !== "customer" && cfg.relay_server_url) {
            let host = cfg.relay_server_url;
            try { host = new URL(cfg.relay_server_url).host; } catch (_error) { /* 使用原文 */ }
            target.textContent = `中继 ${host} · 每 ${HB_INTERVAL_MS / 1000} 秒探测`;
        } else {
            target.textContent = `中继延迟 · 每 ${HB_INTERVAL_MS / 1000} 秒探测`;
        }
        if (latest && !latest.ok && latest.error && state.edition !== "customer") target.textContent += ` · ${latest.error}`;
    }

    const okMs = samples.filter((s) => s.ok).map((s) => s.ms);
    const fmt = (value) => (okMs.length ? `${Math.round(value)} ms` : "--");
    $("#hbAvg").textContent = fmt(okMs.reduce((sum, v) => sum + v, 0) / (okMs.length || 1));
    $("#hbMin").textContent = fmt(Math.min(...okMs));
    $("#hbMax").textContent = fmt(Math.max(...okMs));
    const lost = samples.length - okMs.length;
    $("#hbLoss").textContent = samples.length ? `${Math.round((lost / samples.length) * 100)}%` : "0%";
    $("#hbCount").textContent = String(samples.length);

    const { segments, xOf } = hbChartGeometry(samples);
    const paths = hbPaths(segments);
    $("#hbLine").setAttribute("d", paths.line);
    $("#hbArea").setAttribute("d", paths.area);

    const fails = $("#hbFails");
    if (fails) {
        const marks = [];
        samples.forEach((sample, index) => {
            if (sample.ok) return;
            const mark = document.createElement("span");
            mark.style.left = `${(xOf(index) / HB_CHART_W) * 100}%`;
            marks.push(mark);
        });
        fails.replaceChildren(...marks);
    }
}

async function heartbeatProbe() {
    if (heartbeatState.busy) return;
    heartbeatState.busy = true;
    let sample;
    try {
        const result = await callApi("ping_relay");
        heartbeatState.lastAt = result?.at || new Date().toLocaleTimeString("zh-CN", { hour12: false });
        const message = String(result?.error || "");
        // 未配置中继地址：不是网络故障，既不计入采样也不算丢包。
        heartbeatState.unconfigured = !result?.ok && message.includes("未配置");
        if (!heartbeatState.unconfigured) {
            sample = { ok: Boolean(result?.ok), ms: Number(result?.ms) || 0, error: message };
        }
    } catch (error) {
        heartbeatState.unconfigured = false;
        heartbeatState.lastAt = new Date().toLocaleTimeString("zh-CN", { hour12: false });
        sample = { ok: false, ms: 0, error: errorMessage(error) };
    } finally {
        heartbeatState.busy = false;
    }
    if (sample) {
        heartbeatState.samples.push(sample);
        if (heartbeatState.samples.length > HB_MAX_SAMPLES) heartbeatState.samples.shift();
    }
    renderHeartbeat();
}

function startHeartbeatMonitor() {
    if (heartbeatState.timer) return;
    heartbeatProbe();
    heartbeatState.timer = window.setInterval(() => {
        if (!document.hidden) heartbeatProbe();
    }, HB_INTERVAL_MS);
    document.addEventListener("visibilitychange", () => {
        if (!document.hidden) heartbeatProbe();
    });
}

/* ---------- 用量与扣费 ---------- */

const USAGE_PALETTE = ["#3b82f6", "#22c55e", "#60a5fa", "#a855f7", "#f59e0b", "#ec4899", "#14b8a6"];
const USAGE_OTHER_COLOR = "#8b929c";
const USAGE_MAX_SERIES = 7;
const USAGE_REFRESH_MS = 60000;

const usageState = { days: 30, page: 1, pageSize: 20, data: null, seq: 0, chart: null, metric: "cost" };

/** 图表指标：values 为后端 series 中按天的数组字段，total 为排序/图例用的合计字段（free 模型花费为 0，需切到 Token/次数才看得到）。 */
const USAGE_METRICS = {
    cost: { title: "累计花费", values: "daily", total: "cost", format: (v, digits) => formatMoney(v, digits) },
    tokens: { title: "累计 Token", values: "tokens", total: "total_tokens", format: (v) => formatTokens(v) },
    calls: { title: "累计调用次数", values: "calls", total: "total_calls", format: (v) => formatInt(v) },
};

function escapeHtml(value) {
    return String(value ?? "").replace(/[&<>"']/g, (ch) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
}

function formatMoney(value, digits = 4) {
    const n = Number(value) || 0;
    return `${n < 0 ? "-" : ""}${usageState.data?.currency || "¥"}${Math.abs(n).toFixed(digits)}`;
}

function formatRecordCost(value) {
    const n = Number(value) || 0;
    return formatMoney(n, n !== 0 && Math.abs(n) < 0.01 ? 6 : 4);
}

function formatTokens(value) {
    const n = Number(value) || 0;
    const abs = Math.abs(n);
    if (abs >= 1e9) return `${(n / 1e9).toFixed(2)}B`;
    if (abs >= 1e6) return `${(n / 1e6).toFixed(2)}M`;
    if (abs >= 1e3) return `${(n / 1e3).toFixed(1)}K`;
    return String(Math.round(n));
}

function formatInt(value) {
    return Math.round(Number(value) || 0).toLocaleString("en-US");
}

function shortDate(date) {
    const [, month, day] = String(date || "").split("-");
    return month && day ? `${month}/${day}` : String(date || "");
}

function formatRecordTime(value) {
    const time = new Date(value);
    if (Number.isNaN(time.getTime())) return String(value || "—");
    const pad = (n) => String(n).padStart(2, "0");
    return `${pad(time.getMonth() + 1)}-${pad(time.getDate())} ${pad(time.getHours())}:${pad(time.getMinutes())}`;
}

const USAGE_EMPTY_VIEWS = {
    inactive: { icon: "#i-key", title: "尚未激活卡密", text: "激活卡密后即可查看模型调用、Token 消耗与扣费明细。", action: "前往激活" },
    unsupported: { icon: "#i-info", title: "暂不支持用量查询", text: "服务端暂不支持用量查询，请联系管理员升级。", action: "重新检测" },
    error: { icon: "#i-alert", title: "用量加载失败", text: "无法读取用量数据，请稍后重试。", action: "重试" },
};

function showUsageView(view, message = "") {
    $("#usageSkeleton").hidden = view !== "loading";
    $("#usageContent").hidden = view !== "ready";
    const empty = $("#usageEmpty");
    const spec = USAGE_EMPTY_VIEWS[view];
    empty.hidden = !spec;
    if (!spec) return;
    empty.dataset.view = view;
    $(".usage-empty-icon use", empty).setAttribute("href", spec.icon);
    $("#usageEmptyTitle").textContent = spec.title;
    $("#usageEmptyText").textContent = message || spec.text;
    $("#usageEmptyAction span").textContent = spec.action;
}

async function loadUsage({ silent = false } = {}) {
    const seq = ++usageState.seq;
    const button = $("#usageRefreshBtn");
    if (!silent) setButtonBusy(button, true);
    if (usageState.data) $("#usageContent").classList.add("is-refreshing");
    else if (!silent) showUsageView("loading");
    try {
        const result = await callApi("get_usage", usageState.days, usageState.page, usageState.pageSize);
        if (seq !== usageState.seq) return;
        if (!result?.ok) {
            const error = new Error(result?.message || "用量查询失败");
            error.reason = result?.reason || "error";
            throw error;
        }
        const data = result.data || {};
        if (data.ok === false) throw new Error(data.message || data.detail || "用量查询失败");
        usageState.data = data;
        // 先显示内容区再绘图：隐藏状态下图表量不到宽度。
        showUsageView("ready");
        renderUsage(data);
    } catch (error) {
        if (seq !== usageState.seq) return;
        const reason = error.reason || "error";
        if (silent && usageState.data && reason === "error") return;
        usageState.data = null;
        usageState.chart = null;
        $("#usageBalance").hidden = true;
        if (reason === "not_activated") showUsageView("inactive");
        else if (reason === "unsupported") showUsageView("unsupported");
        else showUsageView("error", errorMessage(error));
    } finally {
        if (seq === usageState.seq) {
            setButtonBusy(button, false);
            $("#usageContent").classList.remove("is-refreshing");
        }
    }
}

function renderUsage(data) {
    const summary = data.summary || {};
    const billing = data.billing_enabled !== false;
    const balance = $("#usageBalance");
    balance.hidden = !billing;
    balance.textContent = `余额 ${formatMoney(data.balance)}`;
    $("#usageBillingNote").hidden = billing;
    $("#usageContent").classList.toggle("billing-off", !billing);

    $("#usageTotalCost").textContent = formatMoney(summary.total_cost);
    $("#usageTotalRecords").textContent = `共 ${formatInt(summary.total_requests)} 条记录`;
    $("#usageCacheSaved").textContent = `缓存节省 ${formatMoney(summary.cache_saved)}`;
    $("#usageTodayCost").textContent = formatMoney(summary.today_cost);
    $("#usageTodayRequests").textContent = `${formatInt(summary.today_requests)} 次请求 · ${formatTokens(summary.today_tokens)} Token`;
    const freeRequests = Number(summary.free_requests) || 0;
    $("#usageFreeSplit").textContent = `累计免费 ${formatInt(freeRequests)} 次 · 付费 ${formatInt(summary.paid_requests ?? (summary.total_requests || 0) - freeRequests)} 次`;
    $("#usageTotalTokens").textContent = formatTokens(summary.total_tokens);
    $("#usageTokenSplit").textContent = `输入 ${formatTokens(summary.prompt_tokens)} · 输出 ${formatTokens(summary.completion_tokens)}`;
    const rate = Math.min(1, Math.max(0, Number(summary.cache_hit_rate) || 0));
    $("#usageCacheRate").textContent = `${(rate * 100).toFixed(2)}%`;
    $("#usageCacheBar").style.width = `${(rate * 100).toFixed(2)}%`;

    const range = data.range || {};
    $("#usageRangeText").textContent = range.start
        ? `近 ${range.days || usageState.days} 天（${range.start} 至 ${range.end}）· 花费 ${formatMoney(range.cost)} · ${formatInt(range.requests)} 次请求（免费 ${formatInt(range.free_requests)} / 付费 ${formatInt(range.paid_requests)}）· ${formatTokens(range.tokens)} Token（免费 ${formatTokens(range.free_tokens)} / 付费 ${formatTokens(range.paid_tokens)}）`
        : `近 ${usageState.days} 天`;
    renderUsageChart(data.chart);
    renderUsageRecords(data.records);
}

/**
 * 按当前指标取前 7 个模型（合计降序，0 值模型不画），其余合并为「其他」。
 * 输入：后端 chart {dates, series[]}；输出：{dates, series[{label, total, values, color, cumulative}]}。
 */
function usageChartSeries(chart, metric = USAGE_METRICS[usageState.metric]) {
    const dates = Array.isArray(chart?.dates) ? chart.dates : [];
    const sorted = (Array.isArray(chart?.series) ? chart.series : [])
        .filter((item) => Array.isArray(item?.[metric.values]))
        .map((item) => ({ ...item, total: Number(item[metric.total]) || 0, values: item[metric.values] }))
        .filter((item) => item.total > 0)
        .sort((a, b) => b.total - a.total);
    let list = sorted;
    if (sorted.length > USAGE_MAX_SERIES) {
        const rest = sorted.slice(USAGE_MAX_SERIES);
        list = [...sorted.slice(0, USAGE_MAX_SERIES), {
            model: `${rest.length} 个模型`,
            label: "其他",
            other: true,
            free: rest.every((item) => item.free),
            total: rest.reduce((total, item) => total + item.total, 0),
            values: dates.map((_, i) => rest.reduce((total, item) => total + (Number(item.values[i]) || 0), 0)),
        }];
    }
    const series = list.map((item, index) => {
        let running = 0;
        return {
            ...item,
            color: item.other ? USAGE_OTHER_COLOR : USAGE_PALETTE[index % USAGE_PALETTE.length],
            cumulative: dates.map((_, i) => (running += Number(item.values[i]) || 0)),
        };
    });
    return { dates, series };
}

function usageNiceScale(max) {
    if (!(max > 0)) return { step: 0.25, top: 1 };
    const raw = max / 4;
    const magnitude = 10 ** Math.floor(Math.log10(raw));
    const norm = raw / magnitude;
    const nice = [1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10].find((candidate) => norm <= candidate + 1e-9) || 10;
    return { step: nice * magnitude, top: nice * magnitude * 4 };
}

function renderUsageLegend(series) {
    $("#usageLegend").innerHTML = series.map((item) => `
        <li title="${escapeHtml(item.model || item.label)}">
            <i style="background:${item.color}"></i>
            <span><strong>${escapeHtml(item.label || item.model)}${item.free ? ` <em class="usage-free">免费</em>` : ""}</strong><small>${escapeHtml(USAGE_METRICS[usageState.metric].format(item.total))}</small></span>
        </li>`).join("");
}

function renderUsageChart(chart) {
    const wrap = $("#usageChartWrap");
    const svg = $("#usageChart");
    const metric = USAGE_METRICS[usageState.metric];
    $("#usageChartTitle").textContent = metric.title;
    const { dates, series } = usageChartSeries(chart, metric);
    const hasData = dates.length > 0 && series.length > 0;
    $("#usageChartEmpty").hidden = hasData;
    renderUsageLegend(hasData ? series : []);
    hideUsageTooltip();
    const W = Math.round(wrap.clientWidth);
    const H = Math.round(wrap.clientHeight);
    if (!hasData || !W || !H) {
        svg.innerHTML = "";
        usageState.chart = null;
        return;
    }

    const n = dates.length;
    const stacked = [];
    series.forEach((item, i) => stacked.push(item.cumulative.map((value, j) => value + (i ? stacked[i - 1][j] : 0))));
    const { step, top } = usageNiceScale(Math.max(...stacked.at(-1)));
    let digits = 2;
    while (digits < 6 && Math.abs(Math.round(step * 10 ** digits) - step * 10 ** digits) > 1e-6) digits += 1;
    const ticks = [0, 1, 2, 3, 4].map((k) => ({ value: step * k, label: metric.format(step * k, digits) }));
    const padL = Math.max(44, Math.max(...ticks.map((tick) => tick.label.length)) * 6.8 + 12);
    const padR = 12;
    const padT = 10;
    const padB = 26;
    const plotW = Math.max(10, W - padL - padR);
    const plotH = Math.max(10, H - padT - padB);
    const xOf = (j) => (n === 1 ? padL + plotW / 2 : padL + (j / (n - 1)) * plotW);
    const yOf = (value) => padT + plotH - (value / top) * plotH;
    // 只有一天时把该点拉成横跨整个绘图区的水平带。
    const points = (values) => (n === 1
        ? [[padL, yOf(values[0])], [padL + plotW, yOf(values[0])]]
        : values.map((value, j) => [xOf(j), yOf(value)]));
    const fmt = ([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`;

    let grid = "";
    ticks.forEach((tick, k) => {
        const y = yOf(tick.value).toFixed(1);
        grid += `<line class="usage-grid${k === 0 ? " base" : ""}" x1="${padL}" x2="${W - padR}" y1="${y}" y2="${y}"/>`;
        grid += `<text class="usage-axis" x="${padL - 8}" y="${Number(y) + 4}" text-anchor="end">${escapeHtml(tick.label)}</text>`;
    });
    const labelIndexes = [...new Set([0, Math.floor((n - 1) / 2), n - 1])];
    labelIndexes.forEach((j, k) => {
        const anchor = labelIndexes.length === 1 ? "middle" : k === 0 ? "start" : k === labelIndexes.length - 1 ? "end" : "middle";
        const x = labelIndexes.length === 1 ? padL + plotW / 2 : anchor === "start" ? padL : anchor === "end" ? padL + plotW : xOf(j);
        grid += `<text class="usage-axis" x="${x.toFixed(1)}" y="${H - 6}" text-anchor="${anchor}">${escapeHtml(shortDate(dates[j]))}</text>`;
    });

    let areas = "";
    let lines = "";
    series.forEach((item, i) => {
        const upper = points(stacked[i]);
        const lower = i ? points(stacked[i - 1]) : upper.map(([x]) => [x, yOf(0)]);
        areas += `<path class="usage-area" fill="${item.color}" d="M${upper.map(fmt).join("L")}L${lower.reverse().map(fmt).join("L")}Z"/>`;
        lines += `<path class="usage-line" stroke="${item.color}" d="M${upper.map(fmt).join("L")}"/>`;
    });
    const dots = series.map((item) => `<circle class="usage-dot" r="3.5" fill="${item.color}" cx="0" cy="0"/>`).join("");

    svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
    svg.innerHTML = `${grid}${areas}${lines}<g class="usage-hover" visibility="hidden"><line class="usage-guide" x1="0" x2="0" y1="${padT}" y2="${padT + plotH}"/>${dots}</g>`;
    usageState.chart = { dates, series, stacked, n, W, padL, plotW, xOf, yOf, hover: -1 };
}

function hideUsageTooltip() {
    $("#usageTooltip").hidden = true;
    $(".usage-hover", $("#usageChart"))?.setAttribute("visibility", "hidden");
    if (usageState.chart) usageState.chart.hover = -1;
}

function onUsageChartMove(event) {
    const geo = usageState.chart;
    if (!geo) return;
    const svg = $("#usageChart");
    const rect = svg.getBoundingClientRect();
    if (!rect.width) return;
    const scale = geo.W / rect.width;
    const x = (event.clientX - rect.left) * scale;
    if (x < geo.padL - 12 || x > geo.padL + geo.plotW + 12) {
        hideUsageTooltip();
        return;
    }
    const j = geo.n === 1 ? 0 : Math.round(Math.min(1, Math.max(0, (x - geo.padL) / geo.plotW)) * (geo.n - 1));
    if (j === geo.hover) return;
    geo.hover = j;

    const gx = geo.xOf(j);
    const hover = $(".usage-hover", svg);
    hover.setAttribute("visibility", "visible");
    const guide = $(".usage-guide", hover);
    guide.setAttribute("x1", gx.toFixed(1));
    guide.setAttribute("x2", gx.toFixed(1));
    $$(".usage-dot", hover).forEach((dot, i) => {
        dot.setAttribute("cx", gx.toFixed(1));
        dot.setAttribute("cy", geo.yOf(geo.stacked[i][j]).toFixed(1));
    });

    const tooltip = $("#usageTooltip");
    const { format } = USAGE_METRICS[usageState.metric];
    const rows = geo.series.map((item) => `
        <div class="usage-tip-row"><i style="background:${item.color}"></i><span>${escapeHtml(item.label || item.model)}</span><b>${escapeHtml(format(item.cumulative[j]))}</b></div>`).join("");
    tooltip.innerHTML = `<strong>${escapeHtml(geo.dates[j])}</strong>${rows}
        <div class="usage-tip-row total"><i></i><span>累计合计</span><b>${escapeHtml(format(geo.stacked.at(-1)[j]))}</b></div>`;
    tooltip.hidden = false;
    const px = gx / scale;
    const width = tooltip.offsetWidth;
    const wrapWidth = $("#usageChartWrap").clientWidth;
    let left = px + 14;
    if (left + width > wrapWidth - 4) left = Math.max(4, px - width - 14);
    tooltip.style.left = `${Math.round(left)}px`;
}

function renderUsageRecords(records) {
    const items = Array.isArray(records?.items) ? records.items : [];
    const total = Number(records?.total) || 0;
    const pageSize = Number(records?.page_size) || usageState.pageSize;
    const pages = Math.max(1, Math.ceil(total / pageSize));
    const page = Math.max(1, Number(records?.page) || usageState.page);
    usageState.page = page;
    $("#usageRows").innerHTML = items.length
        ? items.map((item) => {
            const label = item.label || item.model || "—";
            const id = item.model && item.model !== label ? `<small>${escapeHtml(item.model)}</small>` : "";
            const estimated = item.estimated ? `<span class="usage-est" title="上游未返回准确用量，此条费用为估算值">估算</span>` : "";
            return `<tr>
                <td class="time">${escapeHtml(formatRecordTime(item.time))}</td>
                <td class="model"><strong>${escapeHtml(label)}</strong>${id}</td>
                <td class="num">${formatInt(item.prompt_tokens)}</td>
                <td class="num">${formatInt(item.cache_read_tokens)}</td>
                <td class="num">${formatInt(item.completion_tokens)}</td>
                <td class="num cost">${estimated}${item.free ? `<span class="usage-free">免费</span>` : escapeHtml(formatRecordCost(item.cost))}</td>
            </tr>`;
        }).join("")
        : `<tr class="usage-empty-row"><td colspan="6">所选时间范围内暂无调用记录</td></tr>`;
    $("#usagePagerTotal").textContent = `共 ${formatInt(total)} 条`;
    $("#usagePageText").textContent = `第 ${Math.min(page, pages)}/${pages} 页`;
    $("#usagePrevBtn").disabled = page <= 1;
    $("#usageNextBtn").disabled = page >= pages;
}

function changeUsagePage(delta) {
    usageState.page = Math.max(1, usageState.page + delta);
    loadUsage();
}

/* ---------- 顶栏用户菜单 ---------- */

function setUserMenuOpen(open) {
    const menu = $("#userMenu");
    const button = $("#userMenuBtn");
    if (!menu || !button) return;
    menu.hidden = !open;
    button.setAttribute("aria-expanded", String(open));
}

async function pollStatus(force = false) {
    if (state.statusPolling) return;
    state.statusPolling = true;
    try {
        const status = await callApi(force ? "refresh_status" : "get_status");
        renderStatus(status);
    } catch (error) {
        if (force) toast(`状态刷新失败：${errorMessage(error)}`, "error");
    } finally {
        state.statusPolling = false;
    }
}

async function pollLogs() {
    if (state.logPolling) return;
    state.logPolling = true;
    try {
        const result = await callApi("get_logs", state.logCursor, 600);
        const entries = Array.isArray(result?.entries) ? result.entries : [];
        if (entries.length) {
            state.logs.push(...entries);
            if (state.logs.length > 5000) {
                const removed = state.logs.length - 5000;
                state.logs.splice(0, removed);
                if (state.pauseIndex !== null) state.pauseIndex = Math.max(0, state.pauseIndex - removed);
            }
            state.logCursor = Number(result.latest_id || entries.at(-1).id || state.logCursor);
            if (state.streaming) scheduleLogRender();
            else renderLogs();
        } else if (result?.latest_id) {
            state.logCursor = Math.max(state.logCursor, Number(result.latest_id));
        }
    } catch (error) {
        if (state.native) console.error(error);
    } finally {
        state.logPolling = false;
    }
}

function splitLines(value) {
    return String(value || "")
        .split(/\r?\n/)
        .map((item) => item.trim().replace(/^["'`]+|["'`]+$/g, "").trim())
        .filter(Boolean);
}

async function changeModel(modelId) {
    $("#homeModel").value = modelId;
    $("#configModel").value = modelId;
    populateModels(modelId);
    state.config.override_model = modelId;
    try {
        const result = await callApi("set_model", modelId);
        if (!result?.ok) throw new Error(result?.message || "模型设置失败");
        if (result.config) applyConfig(result.config);
        toast(result.message || `模型已切换为 ${modelLabel(modelId)}`);
        pollStatus(true);
    } catch (error) {
        toast(`模型设置失败：${errorMessage(error)}`, "error");
    }
}

async function changeOption(name, value) {
    state.config[name] = value;
    try {
        const result = await callApi("set_option", name, value);
        if (!result?.ok) throw new Error(result?.message || "设置失败");
        if (result.config) state.config = { ...state.config, ...result.config };
        toast(value ? "启动选项已开启" : "启动选项已关闭");
    } catch (error) {
        toast(`选项保存失败：${errorMessage(error)}`, "error");
    }
}

async function handleSaveConfig(event) {
    event.preventDefault();
    const form = $("#configForm");
    if (!form.reportValidity()) return;
    const button = $("#saveConfigBtn");
    setButtonBusy(button, true);
    const payload = {
        relay_server_url: $("#relayUrl").value.trim(),
        devin_executable: $("#devinExecutable").value.trim(),
        relay_auth_token: $("#authToken").value.trim(),
        dns_servers: $("#dnsServers") ? splitLines($("#dnsServers").value) : undefined,
        stub_paths: $("#stubPaths") ? splitLines($("#stubPaths").value) : undefined,
        target_domains: splitLines($("#targetDomains").value),
        target_paths: splitLines($("#targetPaths").value),
        override_model: $("#configModel").value || state.config?.override_model || "",
        show_mitmweb: false,
        restart_devin_on_start: $("#restartToggle").checked,
    };
    try {
        const result = await callApi("save_config", payload);
        if (!result?.ok) throw new Error(result?.message || "保存失败");
        if (result.config) applyConfig(result.config);
        showNotice(result.message || "配置已保存", "success");
        toast("配置文件已更新");
        await pollStatus(true);
    } catch (error) {
        showNotice(`配置保存失败：${errorMessage(error)}`, "error");
    } finally {
        setButtonBusy(button, false);
    }
}

async function handleScanProcesses() {
    const button = $("#scanProcessesBtn");
    setButtonBusy(button, true);
    try {
        const result = await callApi("scan_processes", {
            devin_executable: $("#devinExecutable").value.trim(),
        });
        if (!result?.ok) throw new Error(result?.message || "扫描失败");
        if (result.executable) $("#devinExecutable").value = result.executable;
        toast(result.message || "已自动找到 Devin 程序");
    } catch (error) {
        toast(`扫描失败：${errorMessage(error)}`, "error");
    } finally {
        setButtonBusy(button, false);
    }
}

async function handleBrowseDevin() {
    const button = $("#browseDevinBtn");
    button.disabled = true;
    try {
        const result = await callApi("browse_devin_executable", $("#devinExecutable").value.trim());
        if (result?.path) $("#devinExecutable").value = result.path;
    } catch (error) {
        toast(`无法打开文件选择器：${errorMessage(error)}`, "error");
    } finally {
        button.disabled = false;
    }
}

async function handleRelayTest() {
    const button = $("#testRelayBtn");
    button.disabled = true;
    button.textContent = "测试中…";
    setBadge($("#relayBadge"), "测试中", "warning");
    try {
        const result = await callApi("test_relay");
        setBadge($("#relayBadge"), result.ok ? "已连接" : "不可达", result.ok ? "success" : "danger");
        const message = `中继测试${result.ok ? "成功" : "失败"} · ${result.url || "未配置"} · ${result.detail || ""}`;
        showNotice(message, result.ok ? "success" : "error");
    } catch (error) {
        showNotice(`中继测试失败：${errorMessage(error)}`, "error");
    } finally {
        button.disabled = false;
        button.textContent = "测试连接";
        pollStatus(true);
    }
}

async function handleDnsTest() {
    const button = $("#testDnsBtn");
    if (!button) return;
    setButtonBusy(button, true);
    try {
        const payload = { target_domains: splitLines($("#targetDomains").value) };
        const result = await callApi("test_dns", payload);
        const failed = (result.results || []).filter((item) => !item.ok).map((item) => item.domain);
        const tone = result.ok ? "success" : "error";
        showNotice(result.message || (result.ok ? "域名解析正常" : "部分域名无法解析"), tone);
        if (failed.length) {
            toast(`解析失败：${failed.join("、")}`, "error", 5200);
        } else {
            toast(result.message || "域名解析正常");
        }
    } catch (error) {
        showNotice(`域名解析检测失败：${errorMessage(error)}`, "error");
    } finally {
        setButtonBusy(button, false);
    }
}

async function waitForProxyState(notState, tries = 25) {
    for (let i = 0; i < tries; i += 1) {
        try {
            const status = await callApi("get_status");
            renderStatus(status);
            if (status?.proxy_state !== notState) return status;
        } catch (error) {
            // 停止过程中偶发接口占用，继续重试即可
        }
        await new Promise((resolve) => setTimeout(resolve, 160));
    }
    state.statusPolling = false;
    await pollStatus();
    return state.status;
}

async function handleStartProxy() {
    const button = $("#startProxyBtn");
    setButtonBusy(button, true);
    renderStatus({ ...state.status, proxy_state: "starting", proxy_message: "正在启动本地代理" });
    try {
        const result = await callApi("start_proxy");
        if (!result?.ok) throw new Error(result?.message || "启动失败");
        toast(result.message || "正在启动本地代理");
        await waitForProxyState("starting");
    } catch (error) {
        renderStatus({ ...state.status, proxy_running: false, proxy_state: "error", proxy_message: errorMessage(error) });
        showNotice(`启动失败：${errorMessage(error)}`, "error");
    } finally {
        button.classList.remove("loading");
        state.statusPolling = false;
        await pollStatus();
    }
}

async function handleStopProxy() {
    const button = $("#stopProxyBtn");
    setButtonBusy(button, true);
    renderStatus({ ...state.status, proxy_state: "stopping", proxy_message: "正在停止拦截并关闭 Devin" });
    try {
        const result = await callApi("stop_proxy");
        if (!result?.ok) throw new Error(result?.message || "停止失败");
        toast(result.message || "正在停止拦截并关闭 Devin");
        await waitForProxyState("stopping");
    } catch (error) {
        showNotice(`停止失败：${errorMessage(error)}`, "error");
    } finally {
        button.classList.remove("loading");
        state.statusPolling = false;
        await pollStatus();
    }
}

async function handleCertificate(install) {
    const button = install ? $("#installCertBtn") : $("#uninstallCertBtn");
    if (!install) {
        const confirmed = await confirmAction("卸载根证书", "卸载后 HTTPS 应用流量将无法被本地代理处理。确定继续吗？", "卸载证书");
        if (!confirmed) return;
    }
    setButtonBusy(button, true);
    const resultBox = $("#certificateResult");
    resultBox.hidden = false;
    resultBox.className = "certificate-result";
    resultBox.textContent = install ? "正在安装证书…" : "正在卸载证书…";
    try {
        const result = await callApi(install ? "install_certificate" : "uninstall_certificate");
        if (!result?.ok) throw new Error(result?.message || "证书操作失败");
        resultBox.classList.add("success");
        resultBox.textContent = result.message || "操作已完成";
        showNotice(result.message || "证书操作已完成", "success");
        await pollStatus(true);
    } catch (error) {
        resultBox.classList.add("error");
        resultBox.textContent = errorMessage(error);
        showNotice(`证书操作失败：${errorMessage(error)}`, "error");
    } finally {
        button.classList.remove("loading");
        renderAdmin(state.admin);
    }
}

function confirmAction(title, message, confirmText = "确认") {
    return new Promise((resolve) => {
        const backdrop = $("#modalBackdrop");
        $("#modalTitle").textContent = title;
        $("#modalMessage").textContent = message;
        $("#modalConfirm").textContent = confirmText;
        backdrop.hidden = false;
        const finish = (result) => {
            backdrop.hidden = true;
            $("#modalConfirm").removeEventListener("click", onConfirm);
            $("#modalCancel").removeEventListener("click", onCancel);
            backdrop.removeEventListener("click", onBackdrop);
            resolve(result);
        };
        const onConfirm = () => finish(true);
        const onCancel = () => finish(false);
        const onBackdrop = (event) => {
            if (event.target === backdrop) finish(false);
        };
        $("#modalConfirm").addEventListener("click", onConfirm);
        $("#modalCancel").addEventListener("click", onCancel);
        backdrop.addEventListener("click", onBackdrop);
        $("#modalCancel").focus();
    });
}

async function loadHistory() {
    const button = $("#loadHistoryBtn");
    setButtonBusy(button, true);
    try {
        const result = await callApi("load_log_history");
        if (!result?.ok) throw new Error(result?.message || "没有找到历史日志");
        state.logs = Array.isArray(result.entries) ? result.entries : [];
        state.logCursor = Number(result.latest_id || state.logs.at(-1)?.id || state.logCursor);
        state.pauseIndex = state.streaming ? null : state.logs.length;
        renderLogs();
        toast(result.message || `已加载 ${state.logs.length} 条历史日志`);
    } catch (error) {
        toast(`加载历史失败：${errorMessage(error)}`, "error");
    } finally {
        setButtonBusy(button, false);
    }
}

async function clearLogs() {
    try {
        const result = await callApi("clear_log_buffer");
        state.logs = [];
        state.logCursor = Number(result?.latest_id || state.logCursor);
        state.pauseIndex = state.streaming ? null : 0;
        renderLogs();
    } catch (error) {
        toast(`清空失败：${errorMessage(error)}`, "error");
    }
}

function bindEvents() {
    bindAccountEvents();
    $$(".nav-item").forEach((item) => item.addEventListener("click", () => navigate(item.dataset.route)));
    $("#noticeClose").addEventListener("click", () => $("#globalNotice").hidden = true);
    $("#noticeAction").addEventListener("click", () => navigate("log"));
    if ($("#openLogsBtn")) $("#openLogsBtn").addEventListener("click", () => navigate("log"));
    // 点击任意日志行复制其文本（日志页与工作台预览共用）。
    for (const container of [$("#logLines"), $("#logPreview")]) {
        if (container) container.addEventListener("click", (event) => {
            const line = event.target.closest(".log-line, .preview-line");
            if (line) copyLogLine(line);
        });
    }
    if ($("#hbMini")) $("#hbMini").addEventListener("click", () => {
        navigate("home");
        $("#heartbeatPanel")?.scrollIntoView({ behavior: "smooth", block: "center" });
    });
    if ($("#userMenuBtn")) {
        $("#userMenuBtn").addEventListener("click", (event) => {
            event.stopPropagation();
            setUserMenuOpen($("#userMenu").hidden);
        });
        $("#userMenu").addEventListener("click", (event) => event.stopPropagation());
        document.addEventListener("click", () => setUserMenuOpen(false));
    }
    if ($("#announcementClose")) $("#announcementClose").addEventListener("click", () => {
        state.announcementClosed = announcementKey(state.announcement);
        persistDismissedAnnouncement(state.announcementClosed);
        $("#announcementBanner").hidden = true;
    });
    if ($("#updateNowBtn")) $("#updateNowBtn").addEventListener("click", handleUpdateNow);
    if ($("#updateLaterBtn")) $("#updateLaterBtn").addEventListener("click", () => {
        if (state.update && state.update.mandatory) return;
        state.updateDismissed = true;
        $("#updateOverlay").hidden = true;
    });
    if ($("#cursorhubLink")) {
        $("#cursorhubLink").addEventListener("click", async (event) => {
            event.preventDefault();
            const url = "https://cursorhub.bitheart.top/";
            try {
                const result = await callApi("open_url", url);
                if (!result?.ok) throw new Error(result?.message || "打开失败");
            } catch (error) {
                window.open(url, "_blank", "noopener");
            }
        });
    }
    if ($("#qqGroupBtn")) {
        $("#qqGroupBtn").addEventListener("click", async () => {
            try {
                await navigator.clipboard.writeText("386427683");
                toast("QQ 群号已复制：386427683");
            } catch (error) {
                toast("QQ 交流群：386427683");
            }
        });
    }
    if ($("#quitBtn")) {
        $("#quitBtn").addEventListener("click", async () => {
            const ok = await confirmAction("退出程序", "将停止拦截并恢复 hosts，然后退出程序。确定吗？", "退出");
            if (!ok) return;
            try { await callApi("quit"); } catch (error) { /* 进程退出可能来不及回包 */ }
            showNotice("已退出并恢复 hosts，可关闭此窗口。", "success");
            toast("已退出，可关闭此窗口");
        });
    }
    $("#refreshStatusBtn").addEventListener("click", async () => {
        const button = $("#refreshStatusBtn");
        setButtonBusy(button, true);
        await pollStatus(true);
        setButtonBusy(button, false);
    });
    $$(".model-picker").forEach((picker) => {
        const trigger = $(".model-picker-trigger", picker);
        const menu = $(".model-picker-menu", picker);
        picker._modelPickerMenu = menu;
        trigger.addEventListener("click", (event) => {
            event.stopPropagation();
            setPickerOpen(picker, !picker.classList.contains("open"));
        });
        menu.addEventListener("click", (event) => {
            event.stopPropagation();
            const option = event.target.closest(".model-option");
            if (!option) return;
            applyModelSelection(option.dataset.id, picker.dataset.persist === "true");
        });
    });
    document.addEventListener("click", () => closeModelPickers());
    window.addEventListener("resize", () => {
        $$(".model-picker.open").forEach(positionPickerMenu);
    });
    $("#restartToggle").addEventListener("change", (event) => changeOption("restart_devin_on_start", event.target.checked));
    $("#configForm").addEventListener("submit", handleSaveConfig);
    $("#scanProcessesBtn").addEventListener("click", handleScanProcesses);
    $("#browseDevinBtn").addEventListener("click", handleBrowseDevin);
    $("#toggleTokenBtn").addEventListener("click", () => {
        const input = $("#authToken");
        const visible = input.type === "text";
        input.type = visible ? "password" : "text";
        $("#toggleTokenBtn").lastChild.textContent = visible ? "显示" : "隐藏";
    });
    $("#advancedTrigger").addEventListener("click", () => {
        const trigger = $("#advancedTrigger");
        const open = trigger.getAttribute("aria-expanded") === "true";
        trigger.setAttribute("aria-expanded", String(!open));
        $("#advancedContent").hidden = open;
    });
    $("#testRelayBtn").addEventListener("click", handleRelayTest);
    if ($("#testDnsBtn")) $("#testDnsBtn").addEventListener("click", handleDnsTest);
    $("#startProxyBtn").addEventListener("click", handleStartProxy);
    $("#stopProxyBtn").addEventListener("click", handleStopProxy);
    if ($("#activateLicenseBtn")) $("#activateLicenseBtn").addEventListener("click", handleActivateLicense);
    if ($("#changeLicenseBtn")) $("#changeLicenseBtn").addEventListener("click", handleChangeLicense);
    if ($("#redeemBtn")) $("#redeemBtn").addEventListener("click", handleRedeemTopup);
    if ($("#redeemInput")) $("#redeemInput").addEventListener("keydown", (e) => { if (e.key === "Enter") handleRedeemTopup(); });
    if ($("#licenseInput")) $("#licenseInput").addEventListener("keydown", (e) => { if (e.key === "Enter") handleActivateLicense(); });
    $("#installCertBtn").addEventListener("click", () => handleCertificate(true));
    $("#uninstallCertBtn").addEventListener("click", () => handleCertificate(false));
    $("#relaunchAdminBtn").addEventListener("click", async () => {
        try {
            const result = await callApi("relaunch_as_admin");
            if (!result?.ok) throw new Error(result?.message || "重启失败");
            toast(result.message || "正在重新启动");
        } catch (error) {
            toast(`管理员重启失败：${errorMessage(error)}`, "error");
        }
    });
    $$("[data-log-filter]").forEach((button) => button.addEventListener("click", () => {
        state.logFilter = button.dataset.logFilter;
        $$("[data-log-filter]").forEach((item) => item.classList.toggle("active", item === button));
        scheduleLogRender();
    }));
    $("#logSearch").addEventListener("input", (event) => {
        state.logSearch = event.target.value.trim().toLocaleLowerCase();
        scheduleLogRender();
    });
    $("#streamBtn").addEventListener("click", () => {
        state.streaming = !state.streaming;
        state.pauseIndex = state.streaming ? null : state.logs.length;
        renderLogs();
    });
    $("#loadHistoryBtn").addEventListener("click", loadHistory);
    $("#clearLogsBtn").addEventListener("click", clearLogs);
    $("#autoScroll").addEventListener("change", (event) => {
        state.autoScroll = event.target.checked;
        if (state.autoScroll) renderLogs();
    });
    document.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
            if (!$("#modalBackdrop").hidden) {
                $("#modalCancel").click();
                return;
            }
            closeModelPickers();
            setUserMenuOpen(false);
        }
        if (event.ctrlKey && event.key.toLocaleLowerCase() === "k") {
            event.preventDefault();
            navigate("log");
            $("#logSearch").focus();
        }
        if (event.ctrlKey && ["1", "2", "3", "4", "5"].includes(event.key)) {
            event.preventDefault();
            navigate(["home", "usage", "log", "config", "cert"][Number(event.key) - 1]);
        }
    });
    if ($("#usageRefreshBtn")) $("#usageRefreshBtn").addEventListener("click", () => loadUsage());
    $$("[data-usage-metric]").forEach((button) => button.addEventListener("click", () => {
        usageState.metric = button.dataset.usageMetric;
        $$("[data-usage-metric]").forEach((item) => item.classList.toggle("active", item === button));
        if (usageState.data) renderUsageChart(usageState.data.chart);
    }));
    $$("[data-usage-days]").forEach((button) => button.addEventListener("click", () => {
        const days = Number(button.dataset.usageDays);
        if (days === usageState.days && usageState.data) return;
        usageState.days = days;
        usageState.page = 1;
        $$("[data-usage-days]").forEach((item) => item.classList.toggle("active", item === button));
        loadUsage();
    }));
    if ($("#usagePrevBtn")) $("#usagePrevBtn").addEventListener("click", () => changeUsagePage(-1));
    if ($("#usageNextBtn")) $("#usageNextBtn").addEventListener("click", () => changeUsagePage(1));
    if ($("#usageEmptyAction")) $("#usageEmptyAction").addEventListener("click", () => {
        if ($("#usageEmpty").dataset.view !== "inactive") {
            loadUsage();
            return;
        }
        navigate("home");
        const input = $("#licenseInput");
        if (input && !$("#licenseForm").hidden) input.focus();
    });
    const usageChart = $("#usageChart");
    if (usageChart) {
        usageChart.addEventListener("pointermove", onUsageChartMove);
        usageChart.addEventListener("pointerleave", hideUsageTooltip);
        if ("ResizeObserver" in window) {
            let lastWidth = 0;
            new ResizeObserver(() => {
                const width = $("#usageChartWrap").clientWidth;
                if (!width || width === lastWidth) return;
                lastWidth = width;
                if (usageState.data) renderUsageChart(usageState.data.chart);
            }).observe($("#usageChartWrap"));
        }
    }
}

function applyEdition(edition) {
    state.edition = edition === "customer" ? "customer" : "full";
    const customer = state.edition === "customer";
    document.body.dataset.edition = state.edition;
    document.body.classList.toggle("edition-customer", customer);
    const hide = (selector) => {
        const el = $(selector);
        if (el) el.hidden = customer;
    };
    hide("#testRelayBtn");
    hide("#relaySection");
    hide("#advancedSection");
    const opSmall = $(".operator-card > small");
    if (opSmall) opSmall.textContent = customer ? "客户版 · Devin" : "Local capture · Devin";
    const cfgDesc = $("#configPageDesc");
    if (cfgDesc && customer) cfgDesc.textContent = "配置 Devin 程序与模型替换规则。";
}

async function bootstrap() {
    try {
        const data = await callApi("get_bootstrap");
        if (accountState.loggedIn === null) renderAccount(data.account);
        refreshAccount();
        state.models = Array.isArray(data.models) ? data.models : [];
        state.catalogSynced = Boolean(data.catalog_synced);
        applyEdition(data.edition);
        state.platform = data.platform || "";
        $("#uiVersion").textContent = `UI ${data.ui_version || "3.0"} · v${data.app_version || "—"}`;
        renderAnnouncement(data.announcement || null);
        renderUpdate(data.update || null);
        $("#configPath").textContent = `配置文件位置：${data.config_path || "—"}`;
        $("#certPath").textContent = data.cert_path || "—";
        renderAdmin(Boolean(data.admin));
        applyConfig(data.config || {});
        renderStatus(data.status || {});
        await pollLogs();
    } catch (error) {
        showNotice(`界面初始化失败：${errorMessage(error)}`, "error");
    }
}

function heartbeat() {
    callApi("heartbeat").catch(() => {});
}

function startApplication() {
    if (!state.initialized) {
        state.initialized = true;
        bindEvents();
        renderLogs();
        setInterval(() => pollStatus(false), 3000);
        setInterval(pollLogs, 450);
        setInterval(refreshPublishConfig, 15000);
        setInterval(() => {
            if (state.page === "usage" && !document.hidden) loadUsage({ silent: true });
        }, USAGE_REFRESH_MS);
        startHeartbeatMonitor();
        // 关闭窗口即退出：周期心跳 + 关闭时发信标。刷新会触发 pagehide，
        // 但新页面加载后的心跳会取消退出；最小化不会触发 pagehide，因此不会退出。
        heartbeat();
        setInterval(heartbeat, 2000);
        window.addEventListener("pagehide", () => {
            try { navigator.sendBeacon("/api/closing", "[]"); } catch (error) { /* ignore */ }
        });
    }
    bootstrap();
}

window.addEventListener("pywebviewready", startApplication);
window.addEventListener("DOMContentLoaded", () => {
    setupWindowChrome();
    setTimeout(() => {
        if (!state.initialized) startApplication();
    }, 700);
});

// setupWindowChrome 仅在 WebView2 桌面窗口（Go 注入了 window.bailanWin）里启用自绘标题栏：
// 顶栏空白处拖动 / 双击最大化，顶边 4px 调整高度，右侧最小化 / 最大化 / 关闭。
// 输入：无；输出：无。浏览器打开时没有 bailanWin，保持原样。
function setupWindowChrome() {
    if (typeof window.bailanWin !== "function") return;
    const root = document.documentElement;
    const topbar = $(".topbar");
    const winCall = (action) => window.bailanWin(action).catch(() => false);
    const syncMax = () => winCall("state").then((max) => root.classList.toggle("is-maximized", !!max));
    const isDragArea = (target) => !target.closest("button, a, input, .user-menu, .win-resize-top");

    root.classList.add("framed-app");
    $("#winControls").hidden = false;
    topbar.addEventListener("mousedown", (event) => {
        if (event.button !== 0 || event.detail > 1) return;
        if (event.target.closest(".win-resize-top")) winCall("resize-top");
        else if (isDragArea(event.target)) winCall("drag");
    });
    topbar.addEventListener("dblclick", (event) => {
        if (isDragArea(event.target)) winCall("toggle");
    });
    $("#winControls").addEventListener("click", (event) => {
        const button = event.target.closest("[data-win]");
        if (button) winCall(button.dataset.win);
    });
    window.addEventListener("resize", syncMax);
    syncMax();
}
