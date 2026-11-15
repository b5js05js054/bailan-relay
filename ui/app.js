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
    logShown: 120,
    logArchive: [],
    logFileSkip: 0,
    logFileHasMore: false,
    logLoading: false,
    logHistoryBootstrapped: false,
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
        devin_executable: "",
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
        license: null,
    },
    // 演示钱包：效益包到期时间（毫秒，null 表示没开通过）和余额分开记；为 null 表示本机还没有钱包。
    wallet: { key: "A2C4****DW7C", packUntil: Date.now() + 46.5 * 86400000, balance: 12.5 },
    logs: [
        { id: 1, time: "14:31:46", category: "normal", text: "[Manager] 本地组件已就绪" },
        { id: 2, time: "14:31:47", category: "relay", text: "[会话转发] 请求已进入中继服务" },
        { id: 3, time: "14:31:49", category: "success", text: "[会话转发成功] 模型：SWE-2 · High" },
        { id: 4, time: "14:31:52", category: "relay", text: "[会话转发] 已写回响应" },
    ],
    account: { logged_in: false, user: null },
};

const MOCK_SHOP_PRODUCTS = [
    { id: 1, name: "体验包", days: 1, price: 0.5, description: "先试一天" },
    { id: 2, name: "周包", days: 7, price: 3, description: "" },
    { id: 3, name: "月包", days: 30, price: 9.9, description: "折合每天最划算" },
    { id: 4, name: "季包", days: 90, price: 26, description: "" },
];

// 输入：给用户看的提示。输出：按演示钱包算出的授权状态，字段与 Go 端 license.Status() 一致。
function mockLicense(message = "ok") {
    const wallet = mockState.wallet;
    if (!wallet) return { activated: false, valid: false, pack_active: false, pack_expired: false, message };
    const now = Date.now();
    const active = Boolean(wallet.packUntil && wallet.packUntil > now);
    const pad = (n) => String(n).padStart(2, "0");
    const until = wallet.packUntil ? new Date(wallet.packUntil) : null;
    return {
        activated: true,
        valid: !wallet.packUntil || active || wallet.balance > 0,
        pack_active: active,
        pack_expired: Boolean(wallet.packUntil) && !active,
        card_key: wallet.key,
        card_exp: until ? `${until.getFullYear()}-${pad(until.getMonth() + 1)}-${pad(until.getDate())} ${pad(until.getHours())}:${pad(until.getMinutes())}` : "",
        remain_days: active ? Math.floor((wallet.packUntil - now) / 86400000) : 0,
        message,
        billing_enabled: true,
        balance: wallet.balance,
        balance_text: `¥${wallet.balance.toFixed(4)}`,
        currency: "¥",
    };
}

// 输入：提示。输出：刷新后的演示授权状态副本，同时写回 mockState.status。
function mockSyncLicense(message) {
    mockState.status.license = mockLicense(message);
    return structuredClone(mockState.status.license);
}

mockSyncLicense("ok");

const mockApi = {
    async get_bootstrap() {
        return {
            app_title: "DevinHub",
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
    async user_set_avatar(payload) {
        mockState.account.user.avatar_url = payload.image;
        return { ok: true, message: "头像已更换", account: structuredClone(mockState.account) };
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
        const wallet = mockState.wallet || (mockState.wallet = { key: String(code || "").slice(0, 4) + "****", packUntil: null, balance: 0 });
        wallet.packUntil = Math.max(wallet.packUntil || 0, Date.now()) + 30 * 86400000;
        return { ok: true, message: "兑换成功：效益包 +30 天（演示）", license: mockSyncLicense("ok") };
    },
    async redeem_topup(code) {
        const wallet = mockState.wallet || (mockState.wallet = { key: String(code || "").slice(0, 4) + "****", packUntil: null, balance: 0 });
        wallet.balance = Number((wallet.balance + 10).toFixed(6));
        return { ok: true, message: "充值成功 +10（演示）", license: mockSyncLicense("ok") };
    },
    async license_status() {
        return { ok: true, license: mockSyncLicense(mockState.status.license?.message) };
    },
    async deactivate_license() {
        mockState.wallet = null;
        return { ok: true, message: "已清除", license: mockSyncLicense("已清除") };
    },
    async shop_info() {
        await new Promise((resolve) => setTimeout(resolve, 280));
        const wallet = mockState.wallet;
        return {
            ok: true,
            data: {
                ok: true,
                currency: "¥",
                billing_enabled: true,
                products: structuredClone(MOCK_SHOP_PRODUCTS),
                wallet: {
                    has_wallet: Boolean(wallet),
                    pack_active: Boolean(wallet?.packUntil && wallet.packUntil > Date.now()),
                    pack_expires_at: wallet?.packUntil ? new Date(wallet.packUntil).toISOString() : null,
                    balance: wallet ? wallet.balance : 0,
                },
            },
        };
    },
    async shop_buy(productId) {
        await new Promise((resolve) => setTimeout(resolve, 320));
        const product = MOCK_SHOP_PRODUCTS.find((item) => item.id === Number(productId));
        const wallet = mockState.wallet;
        if (!product) return { ok: false, message: "该效益包已下架，请刷新商城后重试", license: mockSyncLicense() };
        if (!wallet || wallet.balance < product.price) {
            return { ok: false, message: `余额不足：「${product.name}」售价 ¥${product.price}，当前余额 ¥${wallet ? wallet.balance : 0}，请先充值`, license: mockSyncLicense() };
        }
        wallet.balance = Number((wallet.balance - product.price).toFixed(6));
        wallet.packUntil = Math.max(wallet.packUntil || 0, Date.now()) + product.days * 86400000;
        return { ok: true, message: `已开通「${product.name}」，效益包延长 ${product.days} 天（演示）`, license: mockSyncLicense("ok") };
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
    async set_model_option(modelId, effort, context) {
        mockState.config.model_options = { ...mockState.config.model_options, [modelId]: { effort, context } };
        return { ok: true, message: "已保存模型选项", config: structuredClone(mockState.config) };
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
        return { ok: true, message: "已扫描应用进程", processes: ["Devin.exe", "devin.exe", "language_server_windows_x64.exe"], executable: payload.devin_executable || "" };
    },
    async browse_devin_executable() {
        return { ok: true, path: "" };
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
        return {
            ok: true,
            entries: structuredClone(mockState.logs),
            latest_id: mockState.logs.at(-1)?.id || 0,
            buffer_count: mockState.logs.length,
            has_more: false,
            file_lines_loaded: mockState.logs.length,
        };
    },
    async load_log_chunk(skip, limit) {
        const all = structuredClone(mockState.logs);
        const end = Math.max(0, all.length - Number(skip || 0));
        const start = Math.max(0, end - Number(limit || 200));
        const chunk = all.slice(start, end).map((e) => ({ ...e, id: 0 }));
        return { ok: true, entries: chunk, has_more: start > 0, count: chunk.length };
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
        if (!lic.activated) return { ok: false, reason: "not_activated", message: "请先在商城开通效益包或充值余额" };
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
        const cacheWrite = sum(all, "cache_write_tokens");
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
                    input_tokens: Math.max(prompt - cacheRead - cacheWrite, 0),
                    cache_read_tokens: cacheRead, cache_write_tokens: cacheWrite,
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
    async get_pool_status() {
        await new Promise((resolve) => setTimeout(resolve, 280));
        const lic = mockState.status.license || {};
        if (!lic.activated) return { ok: false, reason: "not_activated", message: "请先在商城开通效益包或充值余额" };
        const hour = new Date(Date.now() + 3 * 3600000).toISOString();
        const week = new Date(Date.now() + 2 * 86400000).toISOString();
        return {
            ok: true,
            data: {
                stale: false,
                fetched_at: new Date().toISOString(),
                summary: { total: 3, active: 1, paused: 1, unavailable: 1, high: 2 },
                items: [
                    { id: "wcdkp2m2vqel", name: "ma***72", status: "active", state: "active", high: true, max_utilization: 99, groups: ["ccmax2"], observed_at: new Date().toISOString(), windows: [{ kind: "five_hour", utilization: 3, resets_at: hour }, { kind: "seven_day", utilization: 99, resets_at: week }, { kind: "seven_day_fable", utilization: 55, resets_at: week }] },
                    { id: "gnchezmzu5xn", name: "wi***91", status: "paused", state: "paused", high: true, max_utilization: 97, groups: ["ccmax2"], observed_at: new Date(Date.now() - 86400000).toISOString(), windows: [{ kind: "five_hour", utilization: 0, resets_at: null }, { kind: "seven_day", utilization: 97, resets_at: hour }, { kind: "seven_day_fable", utilization: 87, resets_at: hour }] },
                    { id: "haau3m3ltatk", name: "az***73", status: "unavailable", state: "unavailable", high: false, max_utilization: 40, groups: [], observed_at: new Date().toISOString(), windows: [{ kind: "five_hour", utilization: 11, resets_at: hour }, { kind: "seven_day", utilization: 40, resets_at: week }] },
                ],
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
            const cacheWrite = rand() < 0.4 ? Math.round(prompt * rand() * 0.03) : 0;
            const cacheRead = Math.round((prompt - cacheWrite) * (0.75 + rand() * 0.22));
            const completion = Math.round(300 + rand() * 6000);
            const cost = ((prompt - cacheRead - cacheWrite) * model.pin + cacheRead * model.pin * 0.1 + cacheWrite * model.pin * 1.25 + completion * model.pout) / 1e6;
            records.push({
                time: new Date(dayStart + rand() * span).toISOString(),
                model: model.model, label: model.label, free: Boolean(model.free),
                prompt_tokens: prompt, completion_tokens: completion,
                input_tokens: Math.max(prompt - cacheRead - cacheWrite, 0),
                cache_read_tokens: cacheRead, cache_write_tokens: cacheWrite,
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

function mainScroller() {
    return $("#appScrollFrame") || $(".workspace");
}

function routeScroller(route) {
    if (route === "models") return $("#modelList");
    if (route === "log") return $("#logViewport");
    if (route === "home") return $("#logPreview");
    if (route === "pool") return $("#poolScroll");
    return mainScroller();
}

function pageScrollTop(route) {
    const current = route || state.page;
    const frame = mainScroller();
    const inner = routeScroller(current);
    let top = frame?.scrollTop || 0;
    if (inner && inner !== frame) top = Math.max(top, inner.scrollTop || 0);
    if (current === "usage") top = Math.max(top, $("#usageTableWrap")?.scrollTop || 0);
    return top;
}

function syncTopbarScroll() {
    const shell = $("#appShell");
    const scrolled = pageScrollTop() > 4;
    shell?.classList.toggle("is-scrolled", scrolled);
    shell?.classList.toggle("is-top", !scrolled);
}

function placeNavIndicator(instant) {
    const nav = $(".navigation");
    const indicator = $("#navIndicator");
    const active = $(".nav-item.active");
    if (!nav || !indicator) return;
    // 进入个人页时滑块停在原位再淡出，回来从原位滑走，避免突然消失。
    if (instant) indicator.style.transition = "none";
    indicator.style.opacity = active ? "1" : "0";
    if (active) {
        indicator.style.width = `${active.offsetWidth}px`;
        indicator.style.transform = `translate3d(${active.offsetLeft}px, 0, 0)`;
    }
    if (instant) {
        indicator.getBoundingClientRect();
        indicator.style.transition = "";
    }
}

function pinLeavingPage(page) {
    const parent = page.parentElement;
    if (!parent) return;
    const rect = page.getBoundingClientRect();
    const parentRect = parent.getBoundingClientRect();
    page.style.top = `${rect.top - parentRect.top}px`;
    page.style.left = `${rect.left - parentRect.left}px`;
    page.style.width = `${rect.width}px`;
    page.style.height = `${rect.height}px`;
    page.style.right = "auto";
    page.style.margin = "0";
}

function clearPagePin(page) {
    page.style.top = "";
    page.style.left = "";
    page.style.width = "";
    page.style.height = "";
    page.style.right = "";
    page.style.margin = "";
}

function navigate(route) {
    const next = $("#page-" + route);
    if (!next) return;
    const same = state.page === route && next.classList.contains("active");
    if (same) {
        placeNavIndicator(true);
        return;
    }
    const prev = $(".page.active");
    state.page = route;
    if (route !== "pool") stopPoolPolling();
    $$(".nav-item").forEach((item) => item.classList.toggle("active", item.dataset.route === route));
    $("#brandChip")?.classList.toggle("is-active", route === "profile");
    placeNavIndicator(same);
    $$(".page.is-leaving").forEach((page) => {
        page.classList.remove("is-leaving");
        clearPagePin(page);
    });
    const present = () => {
        next.classList.remove("is-entering");
        next.classList.add("active");
        next.getBoundingClientRect();
        next.classList.add("is-entering");
        setTimeout(() => next.classList.remove("is-entering"), 700);
        if (route === "usage") {
            loadUsage();
            placeUsageThumbs(true);
        }
        if (route === "pool") {
            loadPoolStatus();
            startPoolPolling();
        }
        if (route === "shop") loadShop();
        if (route === "models") requestAnimationFrame(() => {
            fillModelList();
            placeModelFilterPills(false);
        });
        const frame = mainScroller();
        if (frame) frame.scrollTop = 0;
        syncTopbarScroll();
    };
    if (prev && prev !== next) {
        pinLeavingPage(prev);
        prev.classList.remove("active", "is-entering");
        prev.classList.add("is-leaving");
        let settled = false;
        const finish = (event) => {
            if (settled || (event && event.target !== prev)) return;
            settled = true;
            prev.removeEventListener("animationend", finish);
            prev.classList.remove("is-leaving");
            clearPagePin(prev);
            present();
        };
        prev.addEventListener("animationend", finish);
        setTimeout(finish, 180);
    } else {
        present();
    }
    if (route === "log") {
        ensureLogHistory();
        scheduleLogRender();
    }
}

const MODEL_BRAND = {
    gpt: "GPT", openai: "OpenAI", claude: "Claude", deepseek: "DeepSeek",
    glm: "GLM", chatglm: "ChatGLM", kimi: "Kimi", mimo: "MiMo",
    gemini: "Gemini", grok: "Grok", qwen: "Qwen", nemotron: "Nemotron", longcat: "LongCat",
};
function officialModelName(model) {
    const label = String(model?.label || "").trim();
    const rawID = String(model?.id || "").trim();
    if (label && label.toLowerCase() !== rawID.toLowerCase()) return label;
    let id = String(model?.id || model?.label || "").trim().toLowerCase();
    id = id.replace(/-20\d{6}$/, "").replace(/-\d{6}$/, "");
    const parts = id.split("-").filter(Boolean);
    if (parts.at(-1) === "free") parts.pop();
    const words = [];
    for (let i = 0; i < parts.length; i += 1) {
        const part = parts[i];
        const next = parts[i + 1];
        if (/^\d+$/.test(part) && next && /^\d+$/.test(next) && next.length <= 2) {
            words.push(`${part}.${next}`);
            i += 1;
            continue;
        }
        if (MODEL_BRAND[part]) words.push(MODEL_BRAND[part]);
        else if (/^v\d/i.test(part)) words.push(part.replace(/^v/i, "V"));
        else if (/^k\d/i.test(part)) words.push(part.replace(/^k/i, "K"));
        else if (part === "vl" || part === "ai") words.push(part.toUpperCase());
        else if (/^\d/.test(part)) words.push(part.replace(/[a-z]+$/i, (tail) => tail.toUpperCase()));
        else words.push(part.charAt(0).toUpperCase() + part.slice(1));
    }
    return words.join(" ") || id;
}

function modelLabel(modelId) {
    const model = state.models.find((item) => item.id === modelId);
    if (model) return officialModelName(model);
    return modelId ? officialModelName({ id: modelId }) : "—";
}

function formatGroupRate(value) {
    const n = Number(value);
    if (!Number.isFinite(n)) return "";
    return `×${Number(n.toFixed(4))}`;
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
    const name = String(model?.category_name || "").trim();
    const mult = Number(model?.category_multiplier);
    if (name && mult > 0) return `${name} ${formatGroupRate(mult)}`;
    const ratio = Number(model?.ratio);
    return ratio > 0 ? formatGroupRate(ratio) : "收费";
}

function modelTags(model) {
    if (isFreeModel(model)) return `<span class="model-tag is-free">免费</span>`;
    const tags = [];
    if (model?.billing_mode === "request") {
        const price = Number(model.price_request);
        tags.push(`<span class="model-tag is-group" title="按次计费">按次</span>`);
        if (price > 0) tags.push(`<span class="model-tag is-rate" title="每次价格">${escapeHtml(String(Number(price.toFixed(4))))}</span>`);
        return tags.join("");
    }
    const name = String(model?.category_name || "").trim();
    const mult = Number(model?.category_multiplier);
    if (name) tags.push(`<span class="model-tag is-group" title="分组">${escapeHtml(name)}</span>`);
    if (mult > 0) tags.push(`<span class="model-tag is-rate" title="倍率">${escapeHtml(formatGroupRate(mult))}</span>`);
    if (!tags.length) {
        const ratio = Number(model?.ratio);
        tags.push(ratio > 0
            ? `<span class="model-tag is-rate" title="倍率">${escapeHtml(formatGroupRate(ratio))}</span>`
            : `<span class="model-tag is-group">收费</span>`);
    }
    return tags.join("");
}

function modelTierLine(model) {
    const threshold = Number(model?.tier_threshold);
    const ratio = Number(model?.tier_ratio);
    if (!(threshold > 0) || !(ratio > 0)) return "";
    return `<span class="model-tier-note">输入超过 ${escapeHtml(threshold.toLocaleString("en-US"))}，整单 ${escapeHtml(formatGroupRate(ratio))}</span>`;
}

function modelSpecs(model) {
    const specs = [];
    const context = Number(model?.context_window);
    const output = Number(model?.max_output);
    if (context > 0) specs.push(`<span><b>最大上下文</b>${escapeHtml(context.toLocaleString("en-US"))}</span>`);
    if (output > 0) specs.push(`<span><b>最大输出</b>${escapeHtml(output.toLocaleString("en-US"))}</span>`);
    return specs.length ? `<span class="model-specs">${specs.join("")}</span>` : "";
}

function modelLevels(model) {
    const efforts = Array.isArray(model?.reasoning_efforts) ? model.reasoning_efforts.filter(Boolean) : [];
    if (!efforts.length) return "";
    return `<span class="model-levels"><b>思考</b>${efforts.map((value) => `<i>${escapeHtml(String(value))}</i>`).join("")}</span>`;
}

function paidTitle(model) {
    if (model?.billing_mode === "request") {
        const price = Number(model.price_request);
        return `按次计费：每次请求固定扣 ${price > 0 ? price.toFixed(4) : "—"}，从余额扣费`;
    }
    const pin = Number(model?.price_in), pout = Number(model?.price_out);
    let text = !(pin > 0) && !(pout > 0)
        ? "高级模型：按用量从余额扣费"
        : `高级模型：每 100 万 tokens 输入 ${pin.toFixed(4)} / 输出 ${pout.toFixed(4)}，从余额扣费`;
    if (model?.tier_threshold > 0 && model?.tier_ratio > 0) {
        text += `；输入超过 ${formatTokenSpec(model.tier_threshold)} 时整单 ×${model.tier_ratio}`;
    }
    const name = String(model?.category_name || "").trim();
    const mult = Number(model?.category_multiplier);
    if (name && mult > 0) text += `；分类 ${name} ${formatGroupRate(mult)}`;
    return text;
}

// 输入：token 数；输出：规格简写（200000 → 200K，1000000 → 1M），用于上下文/阶梯阈值。
function formatTokenSpec(n) {
    n = Number(n);
    if (!(n > 0)) return "";
    if (n >= 1000 && n % 1000 === 0) {
        if (n % 1e6 === 0) return `${n / 1e6}M`;
        if (n >= 1e6 && Math.abs(n / 1e6 - Math.round(n / 1e6)) < 0.06) return `${Math.round(n / 1e6)}M`;
        return `${n / 1000}K`;
    }
    if (n >= 1024 && n % 1024 === 0) {
        const kib = n / 1024;
        if (kib % 1024 === 0) return `${kib / 1024}M`;
        return `${kib}K`;
    }
    if (n >= 1e6) {
        const millions = n / 1e6;
        if (Math.abs(millions - Math.round(millions)) < 0.06) return `${Math.round(millions)}M`;
        return `${(Math.round(millions * 10) / 10).toFixed(1).replace(/\.0$/, "")}M`;
    }
    if (n >= 1000) {
        const thousands = n / 1000;
        if (Math.abs(thousands - Math.round(thousands)) < 0.06) return `${Math.round(thousands)}K`;
        return `${Math.round(thousands)}K`;
    }
    return n.toLocaleString();
}

// 输入：模型；输出：能力摘要，如「200K 上下文 · 思考」，未知能力返回空串。
function formatUsd(value, keepCents) {
    const n = Number(value) || 0;
    const text = n.toFixed(2);
    return `$${keepCents ? text : text.replace(/(\.\d*?)0+$/, "$1").replace(/\.$/, "")}`;
}

function formatServerPrice(value) {
    const n = Number(value) || 0;
    return `$${n.toFixed(4).replace(/0+$/, "").replace(/\.$/, "")}`;
}

function listUnitPrice(value, model) {
    const current = Number(value) || 0;
    const mult = Number(model?.category_multiplier);
    if (!(mult > 0)) return current;
    return Math.round((current / mult) * 1e6) / 1e6;
}

function scheduleQuote(model) {
    const tiers = model?.price_tiers;
    if (!tiers?.peak || !tiers?.off) return "";
    const activeKey = tiers.active === "off" ? "off" : "peak";
    const fields = [
        ["in", "输入", "price_in"],
        ["out", "输出", "price_out"],
        ["cache", "缓存读", "price_cache"],
        ["write", "缓存写", "price_cache_write"],
    ];
    const cells = fields.map(([, label, field]) => {
        const peak = Number(tiers.peak[field]) || 0;
        const off = Number(tiers.off[field]) || 0;
        const active = activeKey === "off" ? off : peak;
        const list = listUnitPrice(active, model);
        const showList = formatServerPrice(list) !== formatServerPrice(active);
        return {
            label: `<i>${label}</i>`,
            peak: `<strong class="${activeKey === "peak" ? "tier-on" : "tier-off"}" title="高峰">${escapeHtml(formatServerPrice(peak))}</strong>`,
            off: `<strong class="${activeKey === "off" ? "tier-on" : "tier-off"}" title="空闲">${escapeHtml(formatServerPrice(off))}</strong>`,
            list: `<em title="原价">${showList ? escapeHtml(formatServerPrice(list)) : ""}</em>`,
        };
    });
    return `<span class="model-price-grid is-tiers" title="高亮的一行是现在的价格。依次是高峰、空闲、分组前原价。">${cells.map((cell) => cell.label).join("")}${cells.map((cell) => cell.peak).join("")}${cells.map((cell) => cell.off).join("")}${cells.map((cell) => cell.list).join("")}</span>`;
}

function modelPriceCells(model) {
    const items = [
        { key: "in", label: "输入", current: Number(model?.price_in) || 0 },
        { key: "out", label: "输出", current: Number(model?.price_out) || 0 },
        { key: "cache", label: "缓存读", current: Number(model?.price_cache) || 0 },
        { key: "write", label: "缓存写", current: Number(model?.price_cache_write) || 0 },
    ];
    return items.map((item) => ({ ...item, list: listUnitPrice(item.current, model) }));
}

const VENDOR_ICONS = [
    ["icons/claude.svg", ["claude", "anthropic"]],
    ["icons/openai.svg", ["openai", "chatgpt", "gpt-", "gpt", "o1-", "o3-", "o4-", "dall-e", "dalle"]],
    ["icons/gemini.svg", ["gemini", "gemma"]],
    ["icons/google.svg", ["google", "palm"]],
    ["icons/deepseek.svg", ["deepseek"]],
    ["icons/chatglm.svg", ["chatglm"]],
    ["icons/zhipu.svg", ["zhipu", "glm"]],
    ["icons/kimi.svg", ["kimi"]],
    ["icons/moonshot.svg", ["moonshot"]],
    ["icons/mimo.svg", ["mimo", "xiaomi"]],
    ["icons/grok.svg", ["grok"]],
    ["icons/xai.svg", ["xai"]],
    ["icons/qwen.svg", ["qwen", "qwq", "qvq", "tongyi"]],
    ["icons/doubao.svg", ["doubao", "skylark"]],
    ["icons/bytedance.svg", ["bytedance", "seedance", "seedream"]],
    ["icons/baichuan.svg", ["baichuan"]],
    ["icons/minimax.svg", ["minimax", "abab"]],
    ["icons/hailuo.svg", ["hailuo"]],
    ["icons/mistral.svg", ["mistral", "mixtral", "pixtral", "codestral"]],
    ["icons/meta.svg", ["meta-llama", "llama", "meta"]],
    ["icons/cohere.svg", ["cohere", "command-r", "command-a", "c4ai-aya", "aya-"]],
    ["icons/perplexity.svg", ["perplexity", "sonar", "pplx"]],
    ["icons/yi.svg", ["yi-", "yi/", "01-ai"]],
    ["icons/zeroone.svg", ["zeroone", "ling-"]],
    ["icons/wenxin.svg", ["wenxin", "ernie"]],
    ["icons/baidu.svg", ["baidu"]],
    ["icons/hunyuan.svg", ["hunyuan"]],
    ["icons/spark.svg", ["spark", "iflytek"]],
    ["icons/nvidia.svg", ["nvidia", "nemotron"]],
    ["icons/microsoft.svg", ["microsoft", "phi-", "phi3", "phi4"]],
    ["icons/ai21.svg", ["ai21", "jamba"]],
    ["icons/stepfun.svg", ["stepfun", "step-"]],
    ["icons/sensenova.svg", ["sensenova", "sensechat"]],
    ["icons/internlm.svg", ["internlm", "intern-"]],
    ["icons/skywork.svg", ["skywork"]],
    ["icons/longcat.svg", ["longcat"]],
    ["icons/qingyan.svg", ["qingyan"]],
    ["icons/kling.svg", ["kling"]],
    ["icons/vidu.svg", ["vidu"]],
    ["icons/jimeng.svg", ["jimeng"]],
    ["icons/nova.svg", ["nova-"]],
    ["icons/groq.svg", ["groq"]],
    ["icons/huggingface.svg", ["huggingface"]],
    ["icons/ollama.svg", ["ollama"]],
    ["icons/together.svg", ["together"]],
    ["icons/fireworks.svg", ["fireworks"]],
    ["icons/openrouter.svg", ["openrouter"]],
    ["icons/alibaba.svg", ["alibaba", "alibabacloud"]],
    ["icons/stability.svg", ["stability", "sd3", "stable-diffusion"]],
    ["icons/midjourney.svg", ["midjourney"]],
    ["icons/jina.svg", ["jina"]],
    ["icons/voyage.svg", ["voyage"]],
    ["icons/runway.svg", ["runway"]],
    ["icons/luma.svg", ["luma"]],
    ["icons/suno.svg", ["suno"]],
];

const SERIES_ICON = {
    Claude: "icons/claude.svg",
    DeepSeek: "icons/deepseek.svg",
    Kimi: "icons/kimi.svg",
    MiMo: "icons/mimo.svg",
    Zhipu: "icons/zhipu.svg",
    MiniMax: "icons/minimax.svg",
    Qwen: "icons/qwen.svg",
    Doubao: "icons/doubao.svg",
    Hunyuan: "icons/hunyuan.svg",
    Gemini: "icons/gemini.svg",
    GPT: "icons/openai.svg",
    OpenAI: "icons/openai.svg",
    Grok: "icons/grok.svg",
};

function seriesMark(key) {
    if (!key || key === "all") return "";
    if (key === "其他") return `<span class="series-other" aria-hidden="true"></span>`;
    const src = SERIES_ICON[key];
    return src ? `<img class="series-icon" src="${src}" alt="">` : "";
}

function modelVendorMark(model) {
    const id = String(model?.id || "").toLowerCase();
    const label = String(model?.label || "").toLowerCase();
    const hit = VENDOR_ICONS.find(([, prefixes]) => prefixes.some((prefix) => id.startsWith(prefix) || label.startsWith(prefix)));
    if (!hit) return `<span class="model-vendor is-unknown" aria-hidden="true">?</span>`;
    return `<span class="model-vendor" aria-hidden="true"><img src="${hit[0]}" alt=""></span>`;
}

// 厂商图标是按夜间主题画的：单色图标整个是近白色 #F2F2F4，放到日间主题的白底上就看不见了。
// 图标加载后读一次它的 SVG 源码，含这个颜色的打上 is-light-ink，日间主题由样式把它反相成深色。
const LIGHT_INK = /#F2F2F4/i;
const VENDOR_ICON_SELECTOR = ".model-vendor img, .series-icon";
const lightInkBySrc = new Map();

function iconHasLightInk(src) {
    if (!lightInkBySrc.has(src)) {
        lightInkBySrc.set(src, fetch(src)
            .then((response) => (response.ok ? response.text() : ""))
            .then((svg) => LIGHT_INK.test(svg))
            .catch(() => false));
    }
    return lightInkBySrc.get(src);
}

function markVendorIcon(img) {
    const src = img.getAttribute("src");
    if (!src) return;
    iconHasLightInk(src).then((light) => img.classList.toggle("is-light-ink", light));
}

// load 事件不冒泡，用捕获阶段统一接住：卡片、筛选项、用量图例里的图标都走这一处，不用逐个地方改。
function watchVendorIcons() {
    document.addEventListener("load", (event) => {
        const img = event.target;
        if (img instanceof HTMLImageElement && img.matches(VENDOR_ICON_SELECTOR)) markVendorIcon(img);
    }, true);
    $$(VENDOR_ICON_SELECTOR).forEach(markVendorIcon);
}

function modelBrain(model) {
    const efforts = Array.isArray(model?.reasoning_efforts) ? model.reasoning_efforts.filter(Boolean) : [];
    if (!efforts.length && !model?.supports_thinking) return "";
    return `<span class="model-brain" title="支持思考"><svg aria-hidden="true"><use href="#i-brain"/></svg></span>`;
}

const EFFORT_LABELS = { none: "none", minimal: "minimal", low: "low", medium: "medium", high: "high", xhigh: "xhigh", max: "max" };

const MODEL_SERIES = [
    { title: "Claude", prefixes: ["claude", "anthropic"] },
    { title: "GPT", prefixes: ["gpt", "openai", "chatgpt"] },
    { title: "DeepSeek", prefixes: ["deepseek"] },
    { title: "Kimi", prefixes: ["kimi", "moonshot"] },
    { title: "MiMo", prefixes: ["mimo", "xiaomi"] },
    { title: "Zhipu", prefixes: ["glm", "zhipu", "chatglm"] },
    { title: "MiniMax", prefixes: ["minimax"] },
    { title: "Qwen", prefixes: ["qwen"] },
    { title: "Doubao", prefixes: ["doubao"] },
    { title: "Hunyuan", prefixes: ["hunyuan", "hy"] },
];

function modelSeriesTitle(model) {
    const id = String(model?.id || "").toLowerCase();
    const label = String(model?.label || "").toLowerCase();
    for (const series of MODEL_SERIES) {
        if (series.prefixes.some((prefix) => id.startsWith(prefix) || label.startsWith(prefix))) return series.title;
    }
    return "其他";
}


function modelCategoryName(model) {
    const name = String(model?.category_name || "").trim();
    return name || "未分类";
}

// 输入：模型；输出：分组倍率的稳定键，如 "0.12"。没有倍率时返回空串。
function modelRateKey(model) {
    const mult = Number(model?.category_multiplier);
    if (!(mult > 0)) return "";
    return String(Number(mult.toFixed(4)));
}

// 输入：要参与比较的维度；输出：模型是否符合当前已选的厂商、分组或倍率。
function matchesWall(model, dims) {
    if (dims.series && modelWall.series !== "all" && modelSeriesTitle(model) !== modelWall.series) return false;
    if (dims.category && modelWall.category !== "all" && modelCategoryName(model) !== modelWall.category) return false;
    if (dims.rate && modelWall.rate !== "all" && modelRateKey(model) !== modelWall.rate) return false;
    return true;
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
    for (const chip of [$("#routeBilling")]) {
        if (!chip) continue;
        chip.hidden = false;
        chip.textContent = missing ? "已下线" : (free ? "免费" : paidLabel(modelId));
        chip.dataset.tier = missing ? "gone" : (free ? "free" : "paid");
    }
}

// 模型墙筛选状态：厂商、分类与「仅免费」持久化到 localStorage，搜索词仅本次有效。
const MODEL_WALL_KEY = "bailan.modelWall";
const modelWall = loadModelWallPrefs();

function savedFilterKey(value) {
    return typeof value === "string" && value ? value : "all";
}

// 输入：无；输出：{ series, category, rate, freeOnly, search }，存储损坏或不可用时回落默认值。
function loadModelWallPrefs() {
    try {
        const saved = JSON.parse(localStorage.getItem(MODEL_WALL_KEY) || "{}");
        return {
            series: savedFilterKey(saved.series),
            category: savedFilterKey(saved.category),
            rate: savedFilterKey(saved.rate),
            freeOnly: Boolean(saved.freeOnly),
            sort: saved.sort === "name" || saved.sort === "price" ? saved.sort : "",
            order: saved.order === "desc" ? "desc" : "asc",
            search: "",
        };
    } catch {
        return { series: "all", category: "all", rate: "all", freeOnly: false, sort: "", order: "asc", search: "" };
    }
}

function persistModelWallPrefs() {
    try {
        localStorage.setItem(MODEL_WALL_KEY, JSON.stringify({
            series: modelWall.series,
            category: modelWall.category,
            rate: modelWall.rate,
            freeOnly: modelWall.freeOnly,
            sort: modelWall.sort || "",
            order: modelWall.order === "desc" ? "desc" : "asc",
        }));
    } catch { /* 存储不可用时仅本次生效 */ }
}

// 输入：可选的额外条件；输出：当前「仅免费」下还符合条件的模型。
function wallPool(predicate) {
    return state.models.filter((model) => {
        if (modelWall.freeOnly && !isFreeModel(model)) return false;
        return !predicate || predicate(model);
    });
}

// 输入：无；输出：厂商芯片。数量按当前分组、倍率和仅免费计算，数量为 0 的厂商隐藏，当前选中项保留。
function seriesFilterOptions() {
    const pool = wallPool((model) => matchesWall(model, { category: true, rate: true }));
    const groups = MODEL_SERIES.map((series) => ({
        key: series.title,
        name: series.title,
        count: pool.filter((model) => modelSeriesTitle(model) === series.title).length,
    }));
    groups.push({
        key: "其他",
        name: "其他",
        count: pool.filter((model) => modelSeriesTitle(model) === "其他").length,
    });
    return [{ key: "all", name: "全部", count: pool.length }].concat(
        groups.filter((group) => group.count > 0 || group.key === modelWall.series)
    );
}

// 输入：无；输出：分组芯片。数量按当前厂商、倍率和仅免费计算，顺序与目录一致，未分类放最后。
function categoryFilterOptions() {
    const base = wallPool();
    const pool = base.filter((model) => matchesWall(model, { series: true, rate: true }));
    const names = [];
    const seen = new Set();
    base.forEach((model) => {
        const title = modelCategoryName(model);
        if (seen.has(title) || title === "未分类") return;
        seen.add(title);
        names.push(title);
    });
    if (base.some((model) => modelCategoryName(model) === "未分类")) names.push("未分类");
    return [{ key: "all", name: "全部", count: pool.length }].concat(
        names
            .map((title) => ({
                key: title,
                name: title,
                count: pool.filter((model) => modelCategoryName(model) === title).length,
            }))
            .filter((option) => option.count > 0 || option.key === modelWall.category)
    );
}

// 输入：无；输出：倍率芯片。数量按当前厂商、分组和仅免费计算，从左到右按倍率从小到大。
function rateFilterOptions() {
    const pool = wallPool((model) => matchesWall(model, { series: true, category: true }));
    const keys = [];
    const seen = new Set();
    pool.forEach((model) => {
        const key = modelRateKey(model);
        if (!key || seen.has(key)) return;
        seen.add(key);
        keys.push(key);
    });
    if (modelWall.rate !== "all" && !seen.has(modelWall.rate)) keys.push(modelWall.rate);
    keys.sort((a, b) => Number(a) - Number(b));
    return [{ key: "all", name: "全部", count: pool.length }].concat(
        keys
            .map((key) => ({
                key,
                name: formatGroupRate(key),
                count: pool.filter((model) => modelRateKey(model) === key).length,
            }))
            .filter((option) => option.count > 0 || option.key === modelWall.rate)
    );
}

function moveFilterPill(row, animate) {
    const pill = row?.querySelector(".model-filter-pill");
    const button = row?.querySelector('.model-filter-chip[aria-selected="true"]');
    if (!pill || !button) return;
    // offset* 是布局像素。界面缩放改的是 appShell 的 zoom，getBoundingClientRect 会把缩放算进去，再写回 left/width 就会错位。
    const width = button.offsetWidth;
    const height = button.offsetHeight;
    if (width < 1 || height < 1) return;
    const instant = !animate || row.dataset.pillReady !== "1";
    if (instant) pill.style.transition = "none";
    // 位置用 transform（走合成器，不触发重排）；尺寸随选项变化仍需设宽高。
    pill.style.transform = `translate3d(${button.offsetLeft}px, ${button.offsetTop}px, 0)`;
    pill.style.width = `${width}px`;
    pill.style.height = `${height}px`;
    if (instant) {
        pill.getBoundingClientRect();
        pill.style.transition = "";
    } else {
        row._pillAnimateUntil = performance.now() + 600;
    }
    row.dataset.pillReady = "1";
    row.classList.add("is-placed");
}

function watchFilterRow(row) {
    if (!row || row.dataset.watched === "1" || typeof ResizeObserver !== "function") return;
    row.dataset.watched = "1";
    const observer = new ResizeObserver(() => {
        if (row._pillAnimateUntil && performance.now() < row._pillAnimateUntil) return;
        moveFilterPill(row, false);
    });
    observer.observe(row);
}

function placeModelFilterPills(animate) {
    document.querySelectorAll("#page-models .model-filter-row").forEach((row) => moveFilterPill(row, animate));
}

// 输入：芯片行、选项、当前值、是否带厂商图标；输出：横排芯片，选中块滑到当前项。
function renderFilterLine(row, options, current, withIcon) {
    if (!row) return;
    let pill = row.querySelector(".model-filter-pill");
    if (!pill) {
        pill = document.createElement("span");
        pill.className = "model-filter-pill";
        pill.setAttribute("aria-hidden", "true");
        row.prepend(pill);
    }
    const keep = new Map([...row.querySelectorAll(".model-filter-chip")].map((button) => [button.dataset.key, button]));
    const ordered = [];
    options.forEach((option) => {
        let button = keep.get(option.key);
        if (!button) {
            button = document.createElement("button");
            button.type = "button";
            button.className = "model-filter-chip";
            button.dataset.key = option.key;
            button.setAttribute("role", "option");
        } else {
            keep.delete(option.key);
        }
        button.setAttribute("aria-selected", String(option.key === current));
        button.replaceChildren();
        if (withIcon) {
            const mark = seriesMark(option.key);
            if (mark) button.insertAdjacentHTML("beforeend", mark);
        }
        const name = document.createElement("span");
        name.textContent = option.name;
        const count = document.createElement("span");
        count.className = "model-filter-count";
        count.textContent = String(option.count);
        button.append(name, count);
        ordered.push(button);
    });
    keep.forEach((button) => button.remove());
    ordered.forEach((button) => row.append(button));
    watchFilterRow(row);
    requestAnimationFrame(() => moveFilterPill(row, true));
}

// 输入：模型、当前选中 ID；输出：可点击的模型卡片按钮。
function createModelCard(model, selected) {
    const card = document.createElement("button");
    card.type = "button";
    card.className = "model-card";
    card.dataset.id = model.id;
    card.setAttribute("role", "option");
    card.setAttribute("aria-selected", String(model.id === selected));
    card.classList.toggle("selected", model.id === selected);
    const name = officialModelName(model);
    const prices = modelPriceCells(model);
    const quote = scheduleQuote(model);
    const specs = modelSpecs(model);
    const levels = modelLevels(model);
    const tierNote = modelTierLine(model);
    const priceBlock = quote || `<span class="model-price-grid">
            ${prices.map((item) => `<span class="model-price is-${item.key}">
                <i>${escapeHtml(item.label)}</i>
                <strong title="现价">${escapeHtml(formatServerPrice(item.current))}</strong>
                ${formatServerPrice(item.list) === formatServerPrice(item.current) ? "" : `<em title="原价">${escapeHtml(formatServerPrice(item.list))}</em>`}
            </span>`).join("")}
        </span>`;
    card.innerHTML = `
        <span class="model-card-head">
            <span class="model-card-name">${modelVendorMark(model)}<strong>${escapeHtml(name)}</strong>${modelBrain(model)}</span>
        </span>
        <span class="model-card-tags">${modelTags(model)}</span>
        <span class="model-card-prices">${priceBlock}</span>
        ${quote ? `<span class="model-tier-note">高峰：工作日 09:00–12:00、14:00–18:00</span>` : ""}
        ${tierNote}
        ${specs}
        ${levels}
        <svg class="model-card-check" aria-hidden="true"><use href="#i-check"/></svg>`;
    return card;
}

// 输入：当前选中模型 ID；输出：按厂商、分组、免费和搜索词过滤后重绘芯片与卡片网格。
const MODEL_CHUNK = 9;

function updateModelLoadHint() {
    const hint = $("#modelLoadHint");
    if (!hint) return;
    const total = (modelWall.filtered || []).length;
    const shown = Math.min(modelWall.shown || 0, total);
    if (!total || shown >= total) {
        hint.textContent = total ? `已显示 ${shown} / ${total}` : "";
        return;
    }
    hint.textContent = `已显示 ${shown} / ${total}，继续滚动加载`;
}

// 输入：排序键。第一次点按名称 A→Z、按价格从低到高；再点同一个按钮反过来。
function chooseModelSort(key) {
    if (modelWall.sort === key) {
        modelWall.order = modelWall.order === "asc" ? "desc" : "asc";
    } else {
        modelWall.sort = key;
        modelWall.order = "asc";
    }
    persistModelWallPrefs();
    renderModelWall(state.config.override_model);
}

function modelSortAmount(model) {
    if (isFreeModel(model)) return 0;
    if (model?.billing_mode === "request") {
        const once = Number(model.price_request);
        return once > 0 ? once : Number.POSITIVE_INFINITY;
    }
    const input = Number(model?.price_in) || 0;
    if (input > 0) return input;
    const output = Number(model?.price_out) || 0;
    return output > 0 ? output : Number.POSITIVE_INFINITY;
}

function sortModelWall(models) {
    if (modelWall.sort !== "name" && modelWall.sort !== "price") return;
    const dir = modelWall.order === "desc" ? -1 : 1;
    models.sort((a, b) => {
        if (modelWall.sort === "name") {
            const byName = officialModelName(a).localeCompare(officialModelName(b), "zh");
            if (byName) return byName * dir;
        } else {
            const byPrice = modelSortAmount(a) - modelSortAmount(b);
            if (byPrice) return byPrice * dir;
        }
        return officialModelName(a).localeCompare(officialModelName(b), "zh");
    });
}

function syncModelSortButtons() {
    const arrow = modelWall.order === "desc" ? " ↓" : " ↑";
    for (const [id, key, label] of [["modelSortName", "name", "按名称排序"], ["modelSortPrice", "price", "按价格排序"]]) {
        const button = $(`#${id}`);
        if (!button) continue;
        const on = modelWall.sort === key;
        button.setAttribute("aria-pressed", String(on));
        button.textContent = on ? label + arrow : label;
    }
}

function revealMoreModels() {
    const all = modelWall.filtered || [];
    const shown = modelWall.shown || 0;
    if (shown >= all.length) return;
    const list = $("#modelList");
    const wall = $("#modelWall");
    if (!list || !wall) return;
    const next = Math.min(all.length, shown + MODEL_CHUNK);
    all.slice(shown, next).forEach((model) => wall.append(createModelCard(model, state.config.override_model)));
    modelWall.shown = next;
    updateModelLoadHint();
}

function syncModelListFade() {
    const list = $("#modelList");
    if (!list) return;
    const overflow = list.scrollHeight - list.clientHeight > 8;
    const atEnd = !overflow || list.scrollHeight - list.scrollTop - list.clientHeight < 16;
    list.classList.toggle("has-more-below", overflow && !atEnd);
}

function fillModelList() {
    const list = $("#modelList");
    const total = (modelWall.filtered || []).length;
    if (!list || (modelWall.shown || 0) >= total || list.clientHeight < 40 || list.scrollHeight > list.clientHeight + 8) {
        syncModelListFade();
        return;
    }
    revealMoreModels();
    if ((modelWall.shown || 0) < total) requestAnimationFrame(fillModelList);
    else syncModelListFade();
}

function renderModelWall(selected) {
    const wall = $("#modelWall");
    if (!wall) return;
    const refreshFilters = () => ({
        series: seriesFilterOptions(),
        category: categoryFilterOptions(),
        rate: rateFilterOptions(),
    });
    let options = refreshFilters();
    if (!options.series.some((option) => option.key === modelWall.series)) {
        modelWall.series = "all";
        options = refreshFilters();
    }
    if (!options.category.some((option) => option.key === modelWall.category)) {
        modelWall.category = "all";
        options = refreshFilters();
    }
    if (!options.rate.some((option) => option.key === modelWall.rate)) {
        modelWall.rate = "all";
        options = refreshFilters();
    }
    renderFilterLine($("#modelSeriesRow"), options.series, modelWall.series, true);
    renderFilterLine($("#modelCategoryRow"), options.category, modelWall.category, false);
    renderFilterLine($("#modelRateRow"), options.rate, modelWall.rate, false);
    $("#modelFreeOnly").checked = modelWall.freeOnly;
    const keyword = modelWall.search;
    const models = state.models
        .filter((model) => matchesWall(model, { series: true, category: true, rate: true }))
        .filter((model) => !modelWall.freeOnly || isFreeModel(model))
        .filter((model) => !keyword || `${model.id} ${model.label} ${model.category_name || ""}`.toLowerCase().includes(keyword));
    sortModelWall(models);
    syncModelSortButtons();
    modelWall.filtered = models;
    modelWall.shown = Math.min(MODEL_CHUNK, models.length);
    wall.replaceChildren(...models.slice(0, modelWall.shown).map((model) => createModelCard(model, selected)));
    const empty = $("#modelWallEmpty");
    if (empty) {
        empty.hidden = models.length > 0;
        empty.textContent = "没有符合条件的模型。";
    }
    updateModelLoadHint();
    const list = $("#modelList");
    if (list) list.scrollTop = 0;
    requestAnimationFrame(fillModelList);
}

// 输入：当前选中模型 ID；输出：按后台开放的档位重建思考强度 / 上下文下拉，两者都没有开放时整行隐藏。
function renderModelOptions(selected) {
    const box = $("#modelOptions");
    if (!box) return;
    const model = state.models.find((item) => item.id === selected);
    const efforts = model?.reasoning_efforts || [];
    const contexts = model?.context_options || [];
    const saved = state.config?.model_options?.[selected] || {};
    const fill = (select, items, current, label, defaultText) => {
        if (!select) return;
        select.replaceChildren(...[["", defaultText]].concat(items.map((item) => [String(item), label(item)])).map(([value, text]) => {
            const option = document.createElement("option");
            option.value = value;
            option.textContent = text;
            return option;
        }));
        select.value = items.map(String).includes(String(current)) ? String(current) : "";
        syncPick(select);
    };
    fill($("#modelEffort"), efforts, saved.effort, (item) => EFFORT_LABELS[item] || item, "default");
    fill($("#modelContext"), contexts, saved.context, (item) => `${formatTokenSpec(item)} 上下文`,
        model?.context_window > 0 ? `默认（${formatTokenSpec(model.context_window)}）` : "默认");
    $("#modelEffortField").hidden = efforts.length === 0;
    $("#modelContextField").hidden = contexts.length === 0;
    box.hidden = efforts.length === 0 && contexts.length === 0;
}

// 输入：原生 select；输出：把选项同步到旁边的自定义菜单，并刷新按钮上的当前值。
function syncPick(select) {
    const pick = select?.closest(".pick");
    if (!pick) return;
    const current = select.selectedOptions[0];
    const value = $(".pick-value", pick);
    const menu = pickMenuOf(pick);
    if (value) value.textContent = current ? current.textContent : "";
    if (!menu) return;
    menu.replaceChildren(...[...select.options].map((option) => {
        const item = document.createElement("button");
        item.type = "button";
        item.className = "pick-item";
        item.setAttribute("role", "option");
        item.dataset.value = option.value;
        item.setAttribute("aria-selected", option.selected ? "true" : "false");
        const label = document.createElement("span");
        label.textContent = option.textContent;
        item.append(label);
        if (option.selected) {
            const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
            icon.setAttribute("aria-hidden", "true");
            const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
            use.setAttribute("href", "#i-check");
            icon.append(use);
            item.append(icon);
        }
        return item;
    }));
}

function pickMenuOf(pick) {
    if (!pick) return null;
    return pick.querySelector(":scope > .pick-menu") || pick._menu || null;
}

function resetPickMenu(pick) {
    const menu = pickMenuOf(pick);
    if (!menu) return;
    menu.classList.remove("is-fixed");
    menu.style.zoom = "";
    menu.style.removeProperty("top");
    menu.style.removeProperty("left");
    menu.style.removeProperty("width");
    if (menu.parentElement !== pick) pick.appendChild(menu);
}

function positionPickMenu(pick) {
    const menu = pickMenuOf(pick);
    if (!menu) return;
    resetPickMenu(pick);
    menu.hidden = false;
}

function closePicks(except) {
    $$(".pick.is-open").forEach((pick) => {
        if (pick === except) return;
        pick.classList.remove("is-open");
        const trigger = $(".pick-trigger", pick);
        const menu = pickMenuOf(pick);
        if (trigger) trigger.setAttribute("aria-expanded", "false");
        if (menu) menu.hidden = true;
        resetPickMenu(pick);
    });
}

async function changeModelOption() {
    const model = state.config.override_model;
    if (!model) return;
    try {
        const result = await callApi("set_model_option", model, $("#modelEffort").value, Number($("#modelContext").value) || 0);
        if (!result?.ok) throw new Error(result?.message || "保存失败");
        if (result.config) state.config = { ...state.config, ...result.config };
        toast("模型选项已保存，下一条消息起生效");
    } catch (error) {
        toast(`模型选项保存失败：${errorMessage(error)}`, "error");
        renderModelOptions(model);
    }
}

function populateModels(selected) {
    renderModelWall(selected);
    renderModelOptions(selected);
    $("#routeTarget").textContent = selected ? modelLabel(selected) : "—";
    updateRouteBilling(selected);
    renderChips();
    notifyModelMissing(selected);
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
    if ($("#closeToTrayToggle")) $("#closeToTrayToggle").checked = Boolean(cfg.close_to_tray);
    if ($("#launchAtLoginToggle")) $("#launchAtLoginToggle").checked = Boolean(cfg.launch_at_login);
    if ($("#blockDevinUpdateToggle")) $("#blockDevinUpdateToggle").checked = cfg.block_devin_update !== false;
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

function renderCertificate(status) {
    const exists = status.cert_file_exists;
    const installed = status.cert_installed;
    const hero = $("#certificateHero");
    if (exists == null || installed == null) {
        hero.dataset.state = "checking";
        $("#certificateTitle").textContent = "正在检查系统信任";
        $("#certificateDescription").textContent = "正在读取本机证书。";
        setBadge($("#certificateBadge"), "检测中", "neutral");
        return;
    }
    // 证书已经装好时，「安装」不再是该做的事：降成次要按钮并改叫「重新安装」，主按钮只留给还没装的时候。
    const install = $("#installCertBtn");
    install.classList.toggle("primary", !installed);
    install.classList.toggle("secondary", Boolean(installed));
    $("span", install).textContent = installed ? "重新安装" : "安装证书";
    if (installed) {
        hero.dataset.state = "trusted";
        $("#certificateTitle").textContent = "系统信任已建立";
        $("#certificateDescription").textContent = "HTTPS 拦截可以使用。";
        setBadge($("#certificateBadge"), "已信任", "success");
    } else if (exists === false || installed === false) {
        hero.dataset.state = "untrusted";
        $("#certificateTitle").textContent = exists ? "证书尚未安装" : "尚未生成本地证书";
        $("#certificateDescription").textContent = "安装后才能拦截 HTTPS。";
        setBadge($("#certificateBadge"), "未信任", "warning");
    } else {
        hero.dataset.state = "checking";
    }
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
        setBadge($("#captureBadge"), "启动中", "accent");
        $("#controlState").textContent = "启动中";
        $("#operationPercent").textContent = "···";
    } else if (operation === "stopping") {
        setBadge($("#captureBadge"), "停止中", "warning");
        $("#controlState").textContent = "停止中";
        $("#operationPercent").textContent = "···";
    } else if (running) {
        setBadge($("#captureBadge"), "运行中", "success");
        $("#controlState").textContent = "运行中";
        $("#operationPercent").textContent = "100%";
    } else if (operation === "error") {
        setBadge($("#captureBadge"), "异常", "danger");
        $("#controlState").textContent = "异常";
        $("#operationPercent").textContent = "0%";
    } else {
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
    renderAdaptation(current.adaptation);
    // 没开通效益包、没有余额也能启动拦截；能不能发消息由钱包决定（见 renderLicense）。
    $("#startProxyBtn").disabled = running || busy || updateBlocked();
    $("#stopProxyBtn").disabled = (!running && operation !== "starting") || operation === "stopping";
    renderCertificate(current);
    renderChips();

    const notice = current.notice;
    if (notice && Number(notice.sequence || 0) > state.noticeSequence) {
        state.noticeSequence = Number(notice.sequence || 0);
        showNotice(notice.message, notice.tone || "success", notice.action === "log");
    }
}

const ADAPT_STATE = {
    ok: { label: "生效", cls: "ok", mark: "✓" },
    mismatch: { label: "需适配", cls: "warn", mark: "✗" },
    na: { label: "不适用", cls: "na", mark: "–" },
};

// renderAdaptation 把后台下发的「适配状态」画成一排状态块：哪项生效、哪项这版 Devin 对不上。
// 客户版后台不会下发 adaptation，这里拿不到数据就保持隐藏。
function renderAdaptation(report) {
    const panel = $("#adaptPanel");
    if (!panel) return;
    const features = report && Array.isArray(report.features) ? report.features : [];
    if (!report || !report.devin_found || !features.length) {
        panel.hidden = true;
        return;
    }
    panel.hidden = false;
    const mismatch = Number(report.mismatch_count) || 0;
    panel.dataset.state = mismatch > 0 ? "warn" : "ok";

    const ver = report.devin_version ? `Devin ${report.devin_version}` : "Devin 版本未知";
    const commit = report.devin_commit ? ` · ${String(report.devin_commit).slice(0, 8)}` : "";
    const at = report.at ? ` · ${report.at}` : "";
    $("#adaptSub").textContent = `${ver}${commit}${at}`;

    const count = $("#adaptCount");
    if (mismatch > 0) {
        count.textContent = `${mismatch} 项需重新适配`;
        count.dataset.tone = "warn";
    } else {
        count.textContent = `${Number(report.ok_count) || 0}/${Number(report.total) || features.length} 生效`;
        count.dataset.tone = "ok";
    }

    const grid = $("#adaptGrid");
    grid.innerHTML = "";
    features.forEach((f) => {
        const meta = ADAPT_STATE[f.status] || ADAPT_STATE.na;
        const chip = document.createElement("div");
        chip.className = `adapt-chip ${meta.cls}`;
        if (f.detail) chip.title = f.detail;
        const mark = document.createElement("span");
        mark.className = "adapt-mark";
        mark.textContent = meta.mark;
        const name = document.createElement("span");
        name.className = "adapt-name";
        name.textContent = f.name || f.key || "";
        const tag = document.createElement("span");
        tag.className = "adapt-tag";
        tag.textContent = meta.label;
        chip.append(mark, name, tag);
        grid.appendChild(chip);
    });
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

// 输入：license.Status() 的结果。输出：钱包的展示状态。
// 钱包里两样东西互不影响：效益包在有效期内可用免费模型，余额用于高级模型（也能在商城买效益包）。
function walletView(info) {
    const has = Boolean(info?.activated);
    const packActive = has && Boolean(info.pack_active);
    const packExpired = has && !packActive && Boolean(info.pack_expired);
    const billing = has && Boolean(info.billing_enabled);
    const balance = billing ? Number(info.balance) || 0 : 0;
    const days = Number(info?.remain_days) || 0;
    return {
        has,
        packActive,
        packExpired,
        billing,
        balance,
        days,
        pack: packActive ? "active" : packExpired ? "expired" : "none",
        packText: packActive ? (days > 0 ? `剩 ${days} 天` : "不足 1 天") : packExpired ? "已到期" : "未开通",
        // 两样都没有时发不了消息；拦截本身照常可以启动。
        usable: packActive || balance > 0,
    };
}

// 个人页的钱包概览。和顶栏的钱包胶囊用同一份数据，状态一变就跟着更新。
function renderProfileOverview(info, wallet) {
    const box = $("#profileOverview");
    if (!box) return;
    box.dataset.state = wallet.packActive ? "valid" : wallet.packExpired ? "invalid" : "pending";
    $("#profileLicDays").textContent = wallet.packText;
    $("#profileLicExpiry").textContent = wallet.packActive
        ? `到期 ${info.card_exp || "—"}`
        : wallet.packExpired ? "续费后可用免费模型" : "开通后可用免费模型";
    $("#profileBalance").textContent = wallet.billing ? (info.balance_text || "—") : "—";
    $("#profileBalanceNote").textContent = wallet.has && !wallet.billing
        ? "当前不按用量扣费"
        : wallet.balance > 0 ? "可用高级模型，也可买效益包" : "充值后可用高级模型";
    $("#profileCardKey").textContent = wallet.has ? (info.card_key || "—") : "—";
    $("#profileCardNote").textContent = wallet.has ? "钱包已绑定本机" : "尚未绑定";
}

function renderLicense(license) {
    const panel = $("#licensePanel");
    if (!panel) return;
    const info = license || { activated: false, valid: false };
    const wallet = walletView(info);
    renderProfileOverview(info, wallet);

    // 首页卡片只在发不了消息时出现（没有钱包，或效益包和余额都没有）；钱包的详情在顶栏胶囊里。
    panel.dataset.state = wallet.usable ? "valid" : wallet.has ? "invalid" : "pending";
    panel.hidden = wallet.usable;
    const infoText = $("#licenseInfo");
    if (infoText && !wallet.usable) {
        const hint = "聊天前需开通效益包或充值余额";
        // 有钱包但两样都空：说明空在哪。没有钱包：带上后台给的原因（如卡密被禁用）。
        const reason = wallet.has
            ? `效益包${wallet.packText}，余额为 0`
            : (info.message && info.message !== "ok" ? String(info.message).replace(/[。.\s]+$/, "") : "");
        infoText.textContent = reason ? `${reason}。${hint}` : hint;
        infoText.title = infoText.textContent;
    }

    const chip = $("#licenseChip");
    chip.hidden = !wallet.has;
    if (!wallet.has) {
        setLicensePopOpen(false);
        return;
    }
    // 胶囊颜色：效益包有效为绿；只剩余额（免费模型用不了）为黄；两样都没有为红。
    chip.dataset.tone = wallet.packActive ? "ok" : wallet.balance > 0 ? "warn" : "bad";
    const showBalance = wallet.billing && (wallet.packActive || wallet.balance > 0);
    $("#licenseChipDays").textContent = wallet.packActive
        ? (wallet.billing ? (wallet.days > 0 ? `${wallet.days}天` : "<1天") : wallet.packText)
        : wallet.balance > 0 ? "余额" : wallet.packText;
    const chipBalance = $("#licenseChipBalance");
    chipBalance.hidden = !showBalance;
    if (showBalance) chipBalance.textContent = compactChipMoney(info);
    $("#licenseChipTopup").hidden = !wallet.billing;
    chip.title = `效益包${wallet.packText}${wallet.billing ? ` · 余额 ${info.balance_text || "—"}` : ""}，点击查看钱包`;

    setBadge(
        $("#licensePackBadge"),
        wallet.packActive ? "效益包有效" : wallet.packExpired ? "效益包已到期" : "效益包未开通",
        wallet.packActive ? "success" : wallet.packExpired ? "danger" : "warning",
    );
    $("#licenseMeta").hidden = false;
    $("#licenseExpiry").textContent = wallet.packActive
        ? `${info.card_exp || "—"} 到期`
        : wallet.packExpired ? `已于 ${info.card_exp || "—"} 到期` : "未开通";
    $("#licenseCode").textContent = info.card_key || "—";
    $("#licenseBalanceRow").hidden = !wallet.billing;
    if (wallet.billing) $("#licenseBalance").textContent = info.balance_text || "—";
    $("#licenseRedeem").hidden = false;
}

// 顶栏只放得下原来那一格，金额去掉无意义的尾零。完整余额仍在弹出层里。
function compactChipMoney(info) {
    const amount = Number(info?.balance);
    const currency = info?.currency || "¥";
    if (!Number.isFinite(amount)) return info?.balance_text || "—";
    const rounded = Math.round(amount * 100) / 100;
    return currency + (Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(2));
}

// setLicensePopOpen 输入：是否展开；控制顶栏钱包胶囊的弹出层
function setLicensePopOpen(open) {
    $("#licensePop").hidden = !open;
    $("#licenseChip").setAttribute("aria-expanded", String(open));
}

// 输入：后端返回的最新授权状态。输出：无；写回 state 并重绘钱包相关的界面，商城开着时顺带刷新商城。
function applyLicense(license) {
    if (!license) return;
    state.status.license = license;
    renderStatus(state.status);
    if (state.page === "shop") loadShop({ silent: true });
}

// 输入：输入框、按钮。输出：是否兑换成功。
// TP- 开头的是充值码（加余额）；其余是卡密（开通效益包，本机已有钱包时并入钱包）。
async function redeemCode(input, button) {
    const code = (input?.value || "").trim();
    if (!code) {
        showNotice("请输入卡密或充值码", "error");
        input?.focus();
        return false;
    }
    const topup = code.toUpperCase().startsWith("TP-");
    const label = topup ? "充值" : "兑换";
    setButtonBusy(button, true);
    try {
        const res = await callApi(topup ? "redeem_topup" : "activate_license", code);
        const ok = Boolean(res && res.ok);
        showNotice((res && res.message) || `${label}${ok ? "成功" : "失败"}`, ok ? "success" : "error");
        if (ok && input) input.value = "";
        applyLicense(res && res.license);
        if (ok) pollStatus(false);
        return ok;
    } catch (error) {
        showNotice(`${label}失败：${errorMessage(error)}`, "error");
        return false;
    } finally {
        setButtonBusy(button, false);
    }
}

function handleRedeemTopup() {
    return redeemCode($("#redeemInput"), $("#redeemBtn"));
}

function handleActivateLicense() {
    return redeemCode($("#licenseInput"), $("#activateLicenseBtn"));
}

async function handleChangeLicense() {
    const ok = await confirmAction(
        "清除本机卡密",
        "清除后这台电脑暂时不能发消息。效益包和余额不会丢：重新登录账号，或再次兑换卡密时，会找回这台电脑原来的钱包。",
        "清除",
    );
    if (!ok) return;
    try {
        const res = await callApi("deactivate_license");
        setLicensePopOpen(false);
        applyLicense(res && res.license);
    } catch (error) {
        showNotice(`操作失败：${errorMessage(error)}`, "error");
    }
}

/* ---------- 商城：用余额买效益包、兑换卡密、充值 ---------- */

const SHOP_EMPTY_VIEWS = {
    unsupported: { icon: "#i-info", title: "暂不支持商城", text: "服务端暂不支持商城，请联系管理员升级。", action: "重新检测" },
    error: { icon: "#i-alert", title: "商城加载失败", text: "无法读取商城数据，请稍后重试。", action: "重试" },
};
const shopState = { seq: 0, data: null, buying: false };

function showShopView(view, message = "") {
    $("#shopSkeleton").hidden = view !== "loading";
    $("#shopContent").hidden = view !== "ready";
    const empty = $("#shopEmpty");
    const spec = SHOP_EMPTY_VIEWS[view];
    empty.hidden = !spec;
    if (!spec) return;
    empty.dataset.view = view;
    $(".usage-empty-icon use", empty).setAttribute("href", spec.icon);
    $("#shopEmptyTitle").textContent = spec.title;
    $("#shopEmptyText").textContent = message || spec.text;
    $("#shopEmptyAction span").textContent = spec.action;
}

// 输入：金额、货币符号。输出：去掉无意义尾零的金额（9.9 → ¥9.9，12 → ¥12），最多 4 位小数。
function shopMoney(value, currency = "¥") {
    const n = Number(value) || 0;
    return `${n < 0 ? "-" : ""}${currency}${Number(Math.abs(n).toFixed(4))}`;
}

// 输入：商城接口里的 wallet。输出：效益包的状态、标题和说明。
function shopPackView(wallet) {
    const until = wallet?.pack_expires_at ? new Date(wallet.pack_expires_at) : null;
    const dated = Boolean(until) && !Number.isNaN(until.getTime());
    const pad = (n) => String(n).padStart(2, "0");
    const when = dated
        ? `${until.getFullYear()}-${pad(until.getMonth() + 1)}-${pad(until.getDate())} ${pad(until.getHours())}:${pad(until.getMinutes())}`
        : "";
    if (wallet?.pack_active) {
        const days = dated ? Math.floor((until.getTime() - Date.now()) / 86400000) : 0;
        return { pack: "active", text: days > 0 ? `剩 ${days} 天` : (dated ? "不足 1 天" : "有效"), note: dated ? `${when} 到期` : "可使用免费模型" };
    }
    if (dated) return { pack: "expired", text: "已到期", note: `${when} 已到期` };
    return { pack: "none", text: "未开通", note: "开通后可用免费模型" };
}

function renderShop() {
    const data = shopState.data;
    if (!data) return;
    const currency = data.currency || "¥";
    const wallet = data.wallet || {};
    const balance = Number(wallet.balance) || 0;
    const pack = shopPackView(wallet);
    $("#shopWallet").dataset.pack = pack.pack;
    $("#shopPackState").textContent = pack.text;
    $("#shopPackNote").textContent = pack.note;
    $("#shopBalance").textContent = shopMoney(balance, currency);
    $("#shopBalanceNote").textContent = balance > 0 ? "高级模型按用量扣费" : "充值后可用高级模型";

    const products = Array.isArray(data.products) ? data.products : [];
    $("#shopNone").hidden = products.length > 0;
    $("#shopGrid").innerHTML = products.map((item) => {
        const price = Number(item.price) || 0;
        const days = Number(item.days) || 0;
        const daily = days > 1 ? price / days : 0;
        const perDay = daily > 0 ? `约 ${shopMoney(daily >= 0.01 ? daily.toFixed(2) : daily, currency)} / 天` : "";
        // 余额不够时按钮带去充值，而不是摆一个点不动的按钮；差额向上取到分。
        const short = Math.ceil(Number(((price - balance) * 100).toFixed(6))) / 100;
        const action = balance >= price
            ? `<button class="button primary" type="button" data-shop-buy="${Number(item.id)}"><span>用余额购买</span></button>`
            : `<button class="button" type="button" data-shop-topup><span>还差 ${escapeHtml(shopMoney(short, currency))} · 去充值</span></button>`;
        return `
            <article class="shop-card">
                <header><strong>${escapeHtml(item.name)}</strong><span>${days} 天</span></header>
                <p class="shop-price"><b>${escapeHtml(shopMoney(price, currency))}</b>${perDay ? `<small>${escapeHtml(perDay)}</small>` : ""}</p>
                <p class="shop-desc">${escapeHtml(item.description || "")}</p>
                ${action}
            </article>`;
    }).join("");
}

// 输入：silent 为 true 时不显示骨架屏（已有数据时原地替换）。输出：无；拉取商品和本机钱包并重绘。
async function loadShop({ silent = false } = {}) {
    const seq = ++shopState.seq;
    if (!silent && !shopState.data) showShopView("loading");
    try {
        const result = await callApi("shop_info");
        if (seq !== shopState.seq) return;
        if (!result?.ok) {
            const error = new Error(result?.message || "商城加载失败");
            error.reason = result?.reason || "error";
            throw error;
        }
        const data = result.data || {};
        if (data.ok === false) throw new Error(data.message || "商城暂时不可用");
        shopState.data = data;
        showShopView("ready");
        renderShop();
    } catch (error) {
        if (seq !== shopState.seq) return;
        const reason = error.reason || "error";
        if (silent && shopState.data && reason === "error") return;
        shopState.data = null;
        if (reason === "unsupported") showShopView("unsupported");
        else showShopView("error", errorMessage(error));
    }
}

// 输入：商品 ID、被点的按钮。输出：无；确认后用本机钱包的余额购买，成功后效益包顺延。
async function handleShopBuy(productId, button) {
    const data = shopState.data;
    const product = (data?.products || []).find((item) => Number(item.id) === productId);
    if (!product || shopState.buying) return;
    const currency = data.currency || "¥";
    const confirmed = await confirmAction(
        `购买「${product.name}」`,
        `将从余额扣 ${shopMoney(product.price, currency)}，效益包延长 ${Number(product.days) || 0} 天。当前余额 ${shopMoney(data.wallet?.balance, currency)}。`,
        "确认购买",
    );
    if (!confirmed || shopState.buying) return;
    shopState.buying = true;
    setButtonBusy(button, true);
    try {
        const res = await callApi("shop_buy", productId);
        const ok = Boolean(res && res.ok);
        showNotice((res && res.message) || (ok ? "购买成功" : "购买失败"), ok ? "success" : "error");
        if (res && res.license) applyLicense(res.license);
        else loadShop({ silent: true });
    } catch (error) {
        showNotice(`购买失败：${errorMessage(error)}`, "error");
    } finally {
        shopState.buying = false;
        setButtonBusy(button, false);
    }
}

function handleShopRedeem() {
    return redeemCode($("#shopRedeemInput"), $("#shopRedeemBtn"));
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

function liveLogEntries() {
    return state.pauseIndex === null ? state.logs : state.logs.slice(0, state.pauseIndex);
}

function allLogEntries() {
    return [...state.logArchive, ...liveLogEntries()];
}

function applyLogHistoryMeta(result) {
    state.logFileSkip = Number(result?.file_lines_loaded || result?.entries?.length || state.logs.length || 0);
    state.logFileHasMore = Boolean(result?.has_more);
    state.logArchive = [];
}

async function ensureLogHistory() {
    if (state.logHistoryBootstrapped) return;
    state.logHistoryBootstrapped = true;
    if (state.logs.length >= 120) {
        state.logFileSkip = state.logs.length;
        state.logFileHasMore = true;
        return;
    }
    try {
        const result = await callApi("load_log_history");
        if (!result?.ok) return;
        state.logs = Array.isArray(result.entries) ? result.entries : [];
        state.logCursor = Number(result.latest_id || state.logs.at(-1)?.id || state.logCursor);
        applyLogHistoryMeta(result);
        state.logShown = Math.max(state.logShown, Math.min(allLogEntries().filter(logMatches).length, LOG_CHUNK * 2));
        scheduleLogRender();
    } catch { /* 无历史文件时保持实时缓冲 */ }
}

async function loadOlderLogChunk() {
    if (state.logLoading || !state.logFileHasMore) return;
    state.logLoading = true;
    const hint = $("#logMoreHint");
    const prevHint = hint?.textContent || "";
    if (hint) hint.textContent = "正在加载更早的日志…";
    try {
        const result = await callApi("load_log_chunk", state.logFileSkip, LOG_CHUNK);
        if (!result?.ok) {
            state.logFileHasMore = false;
            return;
        }
        const entries = Array.isArray(result.entries) ? result.entries : [];
        state.logFileHasMore = Boolean(result.has_more);
        state.logFileSkip += Number(result.count || entries.length);
        if (entries.length) {
            state.logArchive = [...entries, ...state.logArchive];
            state.logShown = Math.min(allLogEntries().filter(logMatches).length, state.logShown + entries.length);
            scheduleLogRender();
        } else {
            state.logFileHasMore = false;
        }
    } catch {
        state.logFileHasMore = false;
    } finally {
        state.logLoading = false;
        if (hint && !state.logFileHasMore && state.logShown >= allLogEntries().filter(logMatches).length) {
            hint.textContent = "没有更早的日志了";
        } else if (hint && prevHint && prevHint !== "正在加载更早的日志…") {
            hint.textContent = prevHint;
        }
    }
}

function renderLogs() {
    const visible = allLogEntries().filter(logMatches);
    if (!state.logShown) state.logShown = LOG_CHUNK;
    const count = Math.min(state.logShown, visible.length);
    const shown = visible.slice(visible.length - count).reverse();
    const container = $("#logLines");
    const viewport = $("#logViewport");
    const nearTop = viewport ? viewport.scrollTop < 64 : true;
    const fragment = document.createDocumentFragment();
    shown.forEach((entry) => {
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
    $("#logCount").textContent = `已显示 ${shown.length} / ${visible.length} 行${pending ? `（待显示 ${pending} 行）` : ""}`;
    const moreHint = $("#logMoreHint");
    if (moreHint && !state.logLoading) {
        if (shown.length < visible.length || state.logFileHasMore) {
            moreHint.textContent = "滚到底部继续加载更早的日志";
        } else {
            moreHint.textContent = "停在顶部可跟随最新日志";
        }
    }
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
    if (viewport && state.autoScroll && state.streaming && nearTop) {
        viewport.scrollTop = 0;
    }
}

function handleLogViewportScroll() {
    const viewport = $("#logViewport");
    if (!viewport) return;
    const visible = allLogEntries().filter(logMatches);
    const total = visible.length;
    const nearTop = viewport.scrollTop < 72;
    const nearBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 72;

    if (nearTop && !state.streaming && state.pauseIndex !== null && state.pauseIndex < state.logs.length) {
        const step = Math.min(LOG_CHUNK, state.logs.length - state.pauseIndex);
        state.pauseIndex = Math.min(state.logs.length, state.pauseIndex + step);
        scheduleLogRender();
        return;
    }

    if (nearBottom) {
        if (state.logShown < total) {
            state.logShown = Math.min(total, state.logShown + LOG_CHUNK);
            scheduleLogRender();
            return;
        }
        if (state.logFileHasMore) loadOlderLogChunk();
    }
}

/* ---------- 最近日志（工作台预览）---------- */

const LOG_PREVIEW_COUNT = 40;

function renderLogPreview() {
    const container = $("#logPreview");
    if (!container) return;
    if (state.logs.length < LOG_PREVIEW_COUNT && !state.logHistoryBootstrapped) {
        ensureLogHistory();
    }
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
    if (state.status.license) {
        const wallet = walletView(state.status.license);
        if (wallet.packActive) chips.push({ text: `效益包 · ${wallet.packText}`, tone: "neutral" });
        else if (wallet.balance > 0) chips.push({ text: `效益包${wallet.packText} · 仅高级模型可用`, tone: "warning" });
        else chips.push({ text: "没有效益包和余额 · 暂不能发消息", tone: "warning" });
    }
    if (!state.admin) chips.push({ text: "权限受限", tone: "danger" });
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
    const logNav = document.querySelector('.nav-item[data-route="log"]');
    if (logNav) logNav.dataset.hb = tier;

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

const USAGE_PALETTE = ["#3b82f6", "#4cc38a", "#60a5fa", "#a855f7", "#f59e0b", "#ec4899", "#14b8a6"];
const USAGE_OTHER_COLOR = "#8b929c";
const USAGE_MAX_SERIES = 7;
const USAGE_REFRESH_MS = 60000;

const usageState = { days: 30, page: 1, pageSize: 40, items: [], total: 0, hasMore: false, loadingMore: false, loadMoreError: false, data: null, seq: 0, chart: null, metric: "cost", hiddenModels: new Set(), view: "mine" };
const boardState = { range: "all", metric: "cost", page: 1, items: [], totalPages: 1, data: null, loading: false };
const LOG_CHUNK = 80;

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

function formatPoolObserved(value) {
    const time = new Date(value);
    if (Number.isNaN(time.getTime())) return String(value || "—");
    const pad = (n) => String(n).padStart(2, "0");
    return `${time.getFullYear()}年${pad(time.getMonth() + 1)}月${pad(time.getDate())}日 ${pad(time.getHours())}:${pad(time.getMinutes())}:${pad(time.getSeconds())}`;
}

function formatRecordTime(value) {
    const time = new Date(value);
    if (Number.isNaN(time.getTime())) return String(value || "—");
    const pad = (n) => String(n).padStart(2, "0");
    return `${pad(time.getMonth() + 1)}-${pad(time.getDate())} ${pad(time.getHours())}:${pad(time.getMinutes())}`;
}

const USAGE_EMPTY_VIEWS = {
    inactive: { icon: "#i-bag", title: "还没有钱包", text: "开通效益包或充值余额后，这里会显示模型调用、Token 消耗与扣费明细。", action: "去商城" },
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

async function loadMoreUsage() {
    if (usageState.loadingMore || !usageState.hasMore) return;
    const seq = usageState.seq;
    const page = usageState.page + 1;
    usageState.loadingMore = true;
    usageState.loadMoreError = false;
    renderUsageRecords();
    try {
        const result = await callApi("get_usage", usageState.days, page, usageState.pageSize);
        if (seq !== usageState.seq) return;
        if (!result?.ok) throw new Error(result?.message || "用量查询失败");
        const records = result.data?.records || {};
        const items = Array.isArray(records.items) ? records.items : [];
        usageState.items = usageState.items.concat(items);
        usageState.page = Number(records.page) || page;
        usageState.total = Number(records.total) || usageState.total;
        usageState.hasMore = items.length > 0 && usageState.items.length < usageState.total;
    } catch (error) {
        if (seq !== usageState.seq) return;
        usageState.loadMoreError = true;
    } finally {
        if (seq === usageState.seq) {
            usageState.loadingMore = false;
            renderUsageRecords();
        }
    }
}

const POOL_KIND_ORDER = ["five_hour", "seven_day", "seven_day_sonnet", "seven_day_fable"];
const POOL_KIND_LABEL = {
    five_hour: "5 小时",
    seven_day: "7 天",
    seven_day_sonnet: "7 天（Sonnet）",
    seven_day_fable: "7 天（Fable）",
};
const POOL_STATUS = {
    active: { label: "可用", tone: "success" },
    paused: { label: "已暂停", tone: "warning" },
    unavailable: { label: "不可用", tone: "danger" },
};
const POOL_STATE_RANK = { active: 0, paused: 1, unavailable: 2 };
const POOL_EMPTY_VIEWS = {
    inactive: { icon: "#i-bag", title: "还没有钱包", text: "开通效益包或充值余额后，这里会显示号池账号的状态和用量。", action: "去商城" },
    unsupported: { icon: "#i-info", title: "暂不支持号池状态", text: "服务端暂不支持号池状态，请联系管理员升级。", action: "重新检测" },
    error: { icon: "#i-alert", title: "号池加载失败", text: "无法读取号池数据，请稍后重试。", action: "重试" },
};
const poolState = { seq: 0, data: null, filter: "all", sort: "avail", order: "desc" };
let poolTimer = 0;

function startPoolPolling() {
    if (poolTimer) return;
    poolTimer = setInterval(() => {
        if (state.page === "pool" && !document.hidden) loadPoolStatus({ silent: true });
    }, 60000);
}

function stopPoolPolling() {
    if (!poolTimer) return;
    clearInterval(poolTimer);
    poolTimer = 0;
}

function showPoolView(view, message = "") {
    const skeleton = $("#poolSkeleton");
    const content = $("#poolContent");
    const empty = $("#poolEmpty");
    if (skeleton) skeleton.hidden = view !== "loading";
    if (content) content.hidden = view !== "ready";
    const spec = POOL_EMPTY_VIEWS[view];
    if (empty) empty.hidden = !spec;
    if (!spec || !empty) return;
    empty.dataset.view = view;
    $(".usage-empty-icon use", empty)?.setAttribute("href", spec.icon);
    $("#poolEmptyTitle").textContent = spec.title;
    $("#poolEmptyText").textContent = message || spec.text;
    $("#poolEmptyAction span").textContent = spec.action;
}

function poolWindows(account) {
    const source = Array.isArray(account.windows) ? account.windows : account.usage_windows;
    const list = Array.isArray(source) ? source.slice() : [];
    list.sort((a, b) => {
        const ia = POOL_KIND_ORDER.indexOf(a.kind);
        const ib = POOL_KIND_ORDER.indexOf(b.kind);
        return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
    });
    return list;
}

function poolHigh(account) {
    if (typeof account.high === "boolean") return account.high;
    return poolWindows(account).some((item) => Number(item.utilization) >= 90);
}

function poolStateOf(account) {
    if (account.state === "active" || account.state === "paused" || account.state === "unavailable") return account.state;
    return POOL_STATUS[account.status] ? account.status : "unavailable";
}

function poolResetText(value) {
    if (!value) return "";
    const time = new Date(value);
    if (Number.isNaN(time.getTime())) return "";
    const minutes = Math.round((time.getTime() - Date.now()) / 60000);
    if (minutes <= 0) return "已重置";
    if (minutes < 60) return `${minutes}分钟后重置`;
    const hours = Math.round(minutes / 60);
    if (hours < 48) return `${hours}小时后重置`;
    return `${Math.round(hours / 24)}天后重置`;
}

function poolMeterTone(value) {
    const n = Number(value) || 0;
    if (n >= 90) return "bad";
    if (n >= 60) return "warn";
    return "ok";
}

async function loadPoolStatus({ silent = false } = {}) {
    const seq = ++poolState.seq;
    const button = $("#poolRefreshBtn");
    if (!silent) setButtonBusy(button, true);
    // 已有数据就保持原样显示，新数据到了直接替换；刷新反馈交给按钮上的转圈，整页变暗会让页面忽暗忽亮。
    if (!silent && !poolState.data) showPoolView("loading");
    try {
        const result = await callApi("get_pool_status");
        if (seq !== poolState.seq) return;
        if (!result?.ok) {
            const error = new Error(result?.message || "号池查询失败");
            error.reason = result?.reason || "error";
            throw error;
        }
        const data = result.data || {};
        if (data.ok === false) throw new Error(data.message || "号池暂时不可用");
        poolState.data = data;
        showPoolView("ready");
        renderPool();
    } catch (error) {
        if (seq !== poolState.seq) return;
        const reason = error.reason || "error";
        if (silent && poolState.data && reason === "error") return;
        poolState.data = null;
        if (reason === "not_activated") showPoolView("inactive");
        else if (reason === "unsupported") showPoolView("unsupported");
        else showPoolView("error", errorMessage(error));
    } finally {
        if (seq === poolState.seq) setButtonBusy(button, false);
    }
}

function poolFree(account) {
    const used = Number.isFinite(Number(account.max_utilization))
        ? Number(account.max_utilization)
        : Math.max(0, ...poolWindows(account).map((item) => Number(item.utilization) || 0));
    return 100 - Math.max(0, Math.min(100, used));
}

function poolNextReset(account) {
    let best = Infinity;
    for (const window of poolWindows(account)) {
        if (!window.resets_at) continue;
        const time = new Date(window.resets_at).getTime();
        if (!Number.isNaN(time) && time < best) best = time;
    }
    return best;
}

function choosePoolSort(key) {
    if (poolState.sort === key) {
        poolState.order = poolState.order === "asc" ? "desc" : "asc";
    } else {
        poolState.sort = key;
        poolState.order = key === "reset" ? "asc" : "desc";
    }
    renderPool({ animate: true });
}

function syncPoolSortButtons() {
    const arrow = poolState.order === "desc" ? " ↓" : " ↑";
    for (const [id, key, label] of [["poolSortAvail", "avail", "按可用量"], ["poolSortReset", "reset", "按重置时间"]]) {
        const button = $(`#${id}`);
        if (!button) continue;
        const on = poolState.sort === key;
        button.setAttribute("aria-pressed", String(on));
        button.textContent = on ? label + arrow : label;
    }
}

function syncPoolNav(summary, counts) {
    const nav = document.querySelector('.nav-item[data-route="pool"]');
    const icon = nav?.querySelector(".nav-icon");
    if (!nav || !icon) return;
    const total = Number(summary.total ?? counts.all) || 0;
    const active = Number(summary.active ?? counts.active) || 0;
    const paused = Number(summary.paused ?? counts.paused) || 0;
    const unavailable = Number(summary.unavailable ?? counts.unavailable) || 0;
    const fill = icon.querySelector(".pool-icon-fill");
    if (!total) {
        delete nav.dataset.pool;
        if (fill) fill.style.clipPath = "";
        return;
    }
    const ratio = active / total;
    // 图标图案在 viewBox 里大约从 y=3 到 y=21，按图案高度而不是整格来裁。
    const top = 3 / 24;
    const bottom = 21 / 24;
    const cut = top + (1 - ratio) * (bottom - top);
    if (fill) fill.style.clipPath = `inset(${(cut * 100).toFixed(2)}% 0 0 0)`;
    nav.dataset.pool = "live";
    nav.title = `CC号池状态 (Ctrl+6)：可用 ${active}，暂停 ${paused}，禁用 ${unavailable}`;
}

function placePoolStatsThumb(instant) {
    const bar = $("#poolStats");
    const thumb = $("#poolStatsThumb");
    const active = bar?.querySelector(`[data-pool-filter="${poolState.filter}"]`);
    if (!bar || !thumb || !active) return;
    if (instant) thumb.style.transition = "none";
    thumb.dataset.tone = poolState.filter;
    thumb.style.width = `${active.offsetWidth}px`;
    thumb.style.transform = `translate3d(${active.offsetLeft}px, ${active.offsetTop}px, 0)`;
    if (instant) {
        thumb.getBoundingClientRect();
        thumb.style.transition = "";
    }
}

function renderPool({ animate = false } = {}) {
    const data = poolState.data;
    if (!data || !$("#poolList")) return;
    const items = Array.isArray(data.items) ? data.items : [];
    const summary = data.summary || {};
    const counts = { all: items.length, active: 0, paused: 0, unavailable: 0, high: 0 };
    items.forEach((item) => {
        const state = poolStateOf(item);
        if (counts[state] != null) counts[state] += 1;
        if (poolHigh(item)) counts.high += 1;
    });
    $("#poolCountAll").textContent = formatInt(summary.total ?? data.total ?? counts.all);
    $("#poolCountActive").textContent = formatInt(summary.active ?? counts.active);
    $("#poolCountPaused").textContent = formatInt(summary.paused ?? counts.paused);
    $("#poolCountUnavailable").textContent = formatInt(summary.unavailable ?? counts.unavailable);
    $("#poolCountHigh").textContent = formatInt(summary.high ?? counts.high);
    syncPoolNav(summary, counts);
    $$("[data-pool-filter]").forEach((button) => {
        button.setAttribute("aria-pressed", String(button.dataset.poolFilter === poolState.filter));
    });
    placePoolStatsThumb(!animate);
    syncPoolSortButtons();
    $("#poolStale").hidden = !(data.stale || data.upstream_stale);

    const groupBox = $("#poolGroups");
    if (groupBox) {
        groupBox.hidden = true;
        groupBox.replaceChildren();
    }

    const visible = items.filter((item) => {
        if (poolState.filter === "high" && !poolHigh(item)) return false;
        if (poolState.filter !== "all" && poolState.filter !== "high" && poolStateOf(item) !== poolState.filter) return false;
        return true;
    }).sort((a, b) => {
        const dir = poolState.order === "desc" ? -1 : 1;
        if (poolState.sort === "reset") {
            const left = poolNextReset(a);
            const right = poolNextReset(b);
            if (!Number.isFinite(left) || !Number.isFinite(right)) return Number.isFinite(left) ? -1 : Number.isFinite(right) ? 1 : 0;
            return (left - right) * dir;
        }
        // 「可用量」只对能用的账号有意义：先按状态分组（可用 → 暂停 → 禁用），组内再按可用量排。
        // 暂停的账号用量是 0，纯按可用量排会把它们全顶到最前面。
        const rank = POOL_STATE_RANK[poolStateOf(a)] - POOL_STATE_RANK[poolStateOf(b)];
        if (rank) return rank;
        return (poolFree(a) - poolFree(b)) * dir;
    });

    const list = $("#poolList");
    if (animate) {
        const scroller = $("#poolScroll");
        if (scroller) scroller.scrollTop = 0;
    }
    if (animate && list.childElementCount) {
        list.classList.add("is-switching");
        window.setTimeout(() => {
            if ($("#poolList") !== list) return;
            list.replaceChildren();
            paintPoolCards(list, visible);
            list.classList.remove("is-switching");
        }, 140);
        return;
    }
    list.replaceChildren();
    paintPoolCards(list, visible);
}

function paintPoolCards(list, visible) {
    visible.forEach((item) => {
        const state = poolStateOf(item);
        const status = POOL_STATUS[state] || { label: item.status || "未知", tone: "neutral" };
        const windows = poolWindows(item);
        const worst = windows.reduce((max, window) => Math.max(max, Number(window.utilization) || 0), 0);
        const card = document.createElement("article");
        card.className = "pool-card";
        card.dataset.state = state;
        card.dataset.tone = windows.length ? poolMeterTone(worst) : "ok";
        const head = document.createElement("header");
        head.className = "pool-card-head";
        const dot = document.createElement("span");
        dot.className = "pool-dot";
        const title = document.createElement("div");
        title.className = "pool-title";
        const name = document.createElement("strong");
        name.textContent = item.name || item.id || "未命名";
        title.append(name);
        const badge = document.createElement("span");
        badge.className = "pool-state";
        badge.textContent = status.label;
        head.append(dot, title, badge);
        card.append(head);
        const body = document.createElement("div");
        body.className = "pool-windows";
        windows.forEach((window) => {
            const row = document.createElement("div");
            row.className = "pool-window";
            const meta = document.createElement("div");
            meta.className = "pool-window-meta";
            const label = document.createElement("span");
            label.className = "pool-window-name";
            label.textContent = POOL_KIND_LABEL[window.kind] || window.kind || "用量窗口";
            const reset = poolResetText(window.resets_at);
            if (reset) {
                const note = document.createElement("small");
                note.textContent = reset;
                label.append(note);
            }
            const value = document.createElement("b");
            const percent = Math.max(0, Math.min(100, Number(window.utilization) || 0));
            value.textContent = `${Math.round(percent)}%`;
            meta.append(label, value);
            const meter = document.createElement("div");
            meter.className = "pool-meter";
            meter.dataset.tone = poolMeterTone(percent);
            const bar = document.createElement("span");
            bar.style.width = `${percent}%`;
            meter.append(bar);
            row.append(meta, meter);
            body.append(row);
        });
        card.append(body);
        if (item.observed_at) {
            const foot = document.createElement("footer");
            foot.textContent = formatPoolObserved(item.observed_at);
            card.append(foot);
        }
        list.append(card);
    });
    $("#poolFilterEmpty").hidden = visible.length > 0;
}

function formatBoardTokens(value) {
    const n = Number(value) || 0;
    if (n >= 1e6) return `${(n / 1e6).toFixed(n >= 1e7 ? 0 : 1).replace(/\.0$/, "")}M`;
    if (n >= 1e3) return `${(n / 1e3).toFixed(n >= 1e4 ? 0 : 1).replace(/\.0$/, "")}K`;
    return String(Math.round(n));
}

function formatBoardMoney(value, currency) {
    const n = Number(value);
    if (!Number.isFinite(n)) return "—";
    return `${currency || "¥"}${Math.abs(n).toFixed(2)}`;
}

function boardCell(item, dimension, currency) {
    const key = dimension === "normal" ? "normal" : dimension === "advanced" ? "advanced" : "total";
    const metric = boardState.metric;
    if (metric === "calls") return `${formatBoardTokens(item[`${key}_calls`])} 次`;
    if (metric === "tokens") return formatBoardTokens(item[`${key}_tokens`]);
    return `${formatBoardMoney(item[`${key}_cost`], currency)} (${formatBoardTokens(item[`${key}_tokens`])})`;
}

function boardAvatar(item) {
    const initial = escapeHtml((item.name || "用").slice(0, 1));
    if (!item.avatar) return `<span class="lb-avatar-fallback">${initial}</span>`;
    return `<img src="/userfile/${encodeURIComponent(item.avatar)}" alt="">`;
}

function renderLeaderboard() {
    const data = boardState.data || {};
    const input = $("#leaderboardOptIn");
    const hint = $("#lbOptInHint");
    const announcement = $("#lbAnnouncement");
    const podium = $("#lbPodium");
    const list = $("#lbList");
    const bar = $("#lbMebar");
    const more = $("#lbMore");
    if (input) {
        input.checked = !!data.opt_in;
        input.disabled = !!data.banned;
    }
        if (hint) hint.textContent = data.banned ? "排行榜参与资格已被限制，如有疑问请联系客服。" : "开启后你的昵称、头像与用量对所有用户可见，实时生效。";
    if (announcement) {
        announcement.hidden = !data.announcement;
        announcement.textContent = data.announcement || "";
    }
    const items = boardState.items;
    const currency = data.currency;
    const dimension = "total";
    const metricLabels = { cost: "花费", tokens: "Token", calls: "调用次数" };
    const labels = { total: metricLabels[boardState.metric] || "花费", normal: metricLabels[boardState.metric] || "花费", advanced: metricLabels[boardState.metric] || "花费" };
    const top = items.filter((item) => item.rank <= 3);
    const podiumOrder = [top[1], top[0], top[2]].filter(Boolean);
    if (podium) {
        podium.classList.toggle("is-single", podiumOrder.length < 2);
        podium.innerHTML = podiumOrder.map((item) => {
            const place = item.rank === 1 ? "is-first" : item.rank === 2 ? "is-second" : "is-third";
            return `<div class="lb-podium-place ${place} ${item.me ? "is-me" : ""}">${item.rank === 1 ? `<span class="lb-crown" aria-hidden="true">♛</span>` : ""}<span class="lb-podium-avatar">${boardAvatar(item)}</span><span class="lb-podium-name">${escapeHtml(item.name || "用户")}</span><span class="lb-podium-uid">${escapeHtml(item.uid || "")}</span><div class="lb-step"><strong>${escapeHtml(boardCell(item, dimension, currency))}</strong><small>${labels[dimension]}</small></div></div>`;
        }).join("");
    }
    const rest = items.filter((item) => item.rank > 3);
    if (list) {
        list.className = `lb-list is-dim-${dimension}`;
        const head = rest.length ? `<div class="lb-row lb-row-head" aria-hidden="true"><span>#</span><span>用户</span><span>UID</span><span class="is-normal">普通模型</span><span class="is-advanced">高级模型</span><span class="is-total">总用量</span></div>` : "";
        const rows = rest.map((item) => `<div class="lb-row ${item.me ? "is-me" : ""}"><span class="lb-rank">${item.rank}</span><span class="lb-col-user"><span class="lb-avatar">${boardAvatar(item)}</span><span class="lb-nickname">${escapeHtml(item.name || "用户")}</span></span><span class="lb-uid">${escapeHtml(item.uid || "")}</span><span class="lb-col-usage is-normal">${escapeHtml(boardCell(item, "normal", currency))}</span><span class="lb-col-usage is-advanced">${escapeHtml(boardCell(item, "advanced", currency))}</span><span class="lb-col-usage is-total">${escapeHtml(boardCell(item, "total", currency))}</span></div>`).join("");
        list.innerHTML = items.length ? head + rows : `<div class="lb-empty">暂无参与者，开启参与抢占第一名吧。</div>`;
    }
    if (more) more.hidden = boardState.page >= boardState.totalPages || !items.length;
    if (bar) {
        if (data.me) {
            const mine = boardState.metric === "calls"
                ? `${formatBoardTokens(data.me.calls)} 次`
                : boardState.metric === "tokens"
                    ? formatBoardTokens(data.me.tokens)
                    : `${formatBoardMoney(data.me.cost, currency)} (${formatBoardTokens(data.me.tokens)})`;
            bar.innerHTML = `<span>我的排名：<strong>${data.me.rank}</strong></span><span class="lb-mebar-divider" aria-hidden="true"></span><span>我的${metricLabels[boardState.metric] || "用量"}：<strong>${escapeHtml(mine)}</strong></span>`;
        } else {
            bar.innerHTML = `<span>${data.banned ? "排行榜参与资格已被限制" : "开启参与后可查看我的排名"}</span>${data.banned ? "" : `<button class="button secondary compact" id="lbJoin" type="button">立即参与</button>`}`;
        }
    }
}

async function loadLeaderboard({ append = false } = {}) {
    if (boardState.loading) return;
    boardState.loading = true;
    const page = append ? boardState.page + 1 : 1;
    try {
        const result = await callApi("get_leaderboard", boardState.range, "total", boardState.metric, page);
        if (!result?.ok) throw new Error(result?.message || "排行加载失败");
        const data = result.data || {};
        if (data.disabled || data.detail === "排行榜功能未开放") {
            const tab = $("#usageBoardTab");
            if (tab) tab.hidden = true;
            showUsageSection("mine");
            return;
        }
        boardState.data = data;
        boardState.page = Number(data.page) || page;
        boardState.totalPages = Number(data.total_pages) || 1;
        const items = Array.isArray(data.items) ? data.items : [];
        boardState.items = append ? boardState.items.concat(items.filter((item) => item.rank > 3 || boardState.page === 1)) : items;
        if (append) boardState.items = boardState.items.filter((item, index, all) => all.findIndex((other) => other.user_id === item.user_id) === index);
        renderLeaderboard();
    } catch (error) {
        const message = errorMessage(error);
        if (message.includes("未开放")) {
            const tab = $("#usageBoardTab");
            if (tab) tab.hidden = true;
            showUsageSection("mine");
            return;
        }
        const list = $("#lbList");
        if (list) list.innerHTML = `<div class="lb-empty">${escapeHtml(message)}</div>`;
    } finally {
        boardState.loading = false;
    }
}

function showUsageSection(view) {
    usageState.view = view === "board" ? "board" : "mine";
    const mine = $("#usageMine");
    const rank = $("#usageRank");
    if (mine) mine.hidden = usageState.view !== "mine";
    if (rank) rank.hidden = usageState.view !== "board";
    $$("[data-usage-view]").forEach((button) => button.classList.toggle("active", button.dataset.usageView === usageState.view));
    placeSegmentThumb($("#usageViewSwitch"), false);
    if (usageState.view === "board") loadLeaderboard();
    else if (usageState.data) requestAnimationFrame(() => renderUsageChart(usageState.data.chart, false));
}

async function loadUsage({ silent = false } = {}) {
    usageState.page = 1;
    usageState.items = [];
    usageState.hasMore = false;
    usageState.loadMoreError = false;
    const seq = ++usageState.seq;
    const button = $("#usageRefreshBtn");
    if (!silent) setButtonBusy(button, true);
    // 已有数据就保持原样显示，新数据到了直接替换；刷新反馈交给按钮上的转圈，整页变暗会让页面忽暗忽亮。
    if (!silent && !usageState.data) showUsageView("loading");
    try {
        const result = await callApi("get_usage", usageState.days, 1, usageState.pageSize);
        if (seq !== usageState.seq) return;
        if (!result?.ok) {
            const error = new Error(result?.message || "用量查询失败");
            error.reason = result?.reason || "error";
            throw error;
        }
        const data = result.data || {};
        if (data.ok === false) throw new Error(data.message || data.detail || "用量查询失败");
        usageState.data = data;
        const records = data.records || {};
        usageState.items = Array.isArray(records.items) ? records.items : [];
        usageState.total = Number(records.total) || usageState.items.length;
        usageState.page = Number(records.page) || 1;
        usageState.hasMore = usageState.items.length < usageState.total;
        // 先显示内容区再绘图：隐藏状态下图表量不到宽度。
        showUsageView("ready");
        renderUsage(data);
    } catch (error) {
        if (seq !== usageState.seq) return;
        const reason = error.reason || "error";
        if (silent && usageState.data && reason === "error") return;
        usageState.data = null;
        usageState.chart = null;
        if ($("#usageBalance")) $("#usageBalance").hidden = true;
        if (reason === "not_activated") showUsageView("inactive");
        else if (reason === "unsupported") showUsageView("unsupported");
        else showUsageView("error", errorMessage(error));
    } finally {
        if (seq === usageState.seq) setButtonBusy(button, false);
    }
}

function renderUsage(data) {
    const summary = data.summary || {};
    const billing = data.billing_enabled !== false;
    const balance = $("#usageBalance");
    if (balance) {
        balance.hidden = !billing;
        balance.textContent = `余额 ${formatMoney(data.balance)}`;
    }
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
    const cacheAll = (Number(summary.cache_read_tokens) || 0) + (Number(summary.cache_write_tokens) || 0);
    const freshInput = Number.isFinite(summary.input_tokens)
        ? summary.input_tokens
        : Math.max(0, (Number(summary.prompt_tokens) || 0) - cacheAll);
    $("#usageTokenSplit").textContent = `输入 ${formatTokens(freshInput)} · 缓存 ${formatTokens(cacheAll)} · 输出 ${formatTokens(summary.completion_tokens)}`;
    const rate = Math.min(1, Math.max(0, Number(summary.cache_hit_rate) || 0));
    $("#usageCacheRate").textContent = `${(rate * 100).toFixed(2)}%`;
    $("#usageCacheBar").style.width = `${(rate * 100).toFixed(2)}%`;

    const range = data.range || {};
    $("#usageRangeText").textContent = range.start
        ? `近 ${range.days || usageState.days} 天（${range.start} 至 ${range.end}）· 花费 ${formatMoney(range.cost)} · ${formatInt(range.requests)} 次请求（免费 ${formatInt(range.free_requests)} / 付费 ${formatInt(range.paid_requests)}）· ${formatTokens(range.tokens)} Token（免费 ${formatTokens(range.free_tokens)} / 付费 ${formatTokens(range.paid_tokens)}）`
        : `近 ${usageState.days} 天`;
    renderUsageChart(data.chart);
    renderUsageRecords();
    placeUsageThumbs(true);
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

function usageModelIcon(item) {
    if (!item || item.other) return "";
    return modelVendorMark({ id: item.model, label: item.label });
}

function usageSeriesKey(item) {
    return item?.other ? "其他" : String(item?.model || item?.label || "");
}

function renderUsageLegend(series) {
    $("#usageLegend").innerHTML = series.map((item) => {
        const key = usageSeriesKey(item);
        const hidden = usageState.hiddenModels.has(key);
        return `<li>
            <button type="button" class="usage-legend-item${hidden ? " is-off" : ""}" data-usage-series="${escapeHtml(key)}" aria-pressed="${hidden ? "false" : "true"}" title="${hidden ? "点击显示" : "点击隐藏"}">
                <span class="usage-legend-mark">${usageModelIcon(item)}<i style="background:${item.color}"></i></span>
                <span><strong>${escapeHtml(item.label || item.model)}${item.free ? ` <em class="usage-free">免费</em>` : ""}</strong><small>${escapeHtml(USAGE_METRICS[usageState.metric].format(item.total))}</small></span>
            </button>
        </li>`;
    }).join("");
}

function usageEase(t) {
    return t < 0.5 ? 4 * t * t * t : 1 - ((-2 * t + 2) ** 3) / 2;
}

function usageSeriesWeight(item) {
    const anim = usageState.seriesAnim?.get(usageSeriesKey(item));
    if (anim) return anim.value;
    return usageState.hiddenModels.has(usageSeriesKey(item)) ? 0 : 1;
}

// 单调三次曲线，经过每个数据点，不会甩出相邻点之间。堆叠边界用同一条曲线，面积不会互相穿插。
function usageSmoothParts(points) {
    const n = points.length;
    const segs = [];
    if (n < 2) return { start: points[0] || [0, 0], segs };
    const dx = [];
    const slope = [];
    for (let i = 0; i < n - 1; i++) {
        dx[i] = points[i + 1][0] - points[i][0] || 1;
        slope[i] = (points[i + 1][1] - points[i][1]) / dx[i];
    }
    const tan = new Array(n);
    tan[0] = slope[0];
    tan[n - 1] = slope[n - 2];
    for (let i = 1; i < n - 1; i++) {
        tan[i] = slope[i - 1] * slope[i] <= 0 ? 0 : (slope[i - 1] + slope[i]) / 2;
    }
    for (let i = 0; i < n - 1; i++) {
        if (Math.abs(slope[i]) < 1e-9) {
            tan[i] = 0;
            tan[i + 1] = 0;
            continue;
        }
        const a = tan[i] / slope[i];
        const b = tan[i + 1] / slope[i];
        const h = a * a + b * b;
        if (h > 9) {
            const tau = 3 / Math.sqrt(h);
            tan[i] = tau * a * slope[i];
            tan[i + 1] = tau * b * slope[i];
        }
    }
    for (let i = 0; i < n - 1; i++) {
        segs.push({
            c1: [points[i][0] + dx[i] / 3, points[i][1] + tan[i] * dx[i] / 3],
            c2: [points[i + 1][0] - dx[i] / 3, points[i + 1][1] - tan[i + 1] * dx[i] / 3],
            p: points[i + 1],
        });
    }
    return { start: points[0], segs };
}

function usagePointText(point) {
    return `${point[0].toFixed(1)},${point[1].toFixed(1)}`;
}

function usageCurvePath(points) {
    const { start, segs } = usageSmoothParts(points);
    if (!start) return "";
    let d = `M${usagePointText(start)}`;
    segs.forEach((seg) => {
        d += `C${usagePointText(seg.c1)} ${usagePointText(seg.c2)} ${usagePointText(seg.p)}`;
    });
    return d;
}

function usageAreaPath(upper, lower) {
    const up = usageSmoothParts(upper);
    const down = usageSmoothParts(lower);
    let d = `M${usagePointText(up.start)}`;
    up.segs.forEach((seg) => {
        d += `C${usagePointText(seg.c1)} ${usagePointText(seg.c2)} ${usagePointText(seg.p)}`;
    });
    d += `L${usagePointText(down.segs.length ? down.segs[down.segs.length - 1].p : down.start)}`;
    for (let i = down.segs.length - 1; i >= 0; i--) {
        const seg = down.segs[i];
        const from = i === 0 ? down.start : down.segs[i - 1].p;
        d += `C${usagePointText(seg.c2)} ${usagePointText(seg.c1)} ${usagePointText(from)}`;
    }
    return `${d}Z`;
}

function usageAxisTop(target) {
    const now = performance.now();
    const axis = usageState.axis;
    if (!axis) {
        usageState.axis = { from: target, target, start: now, value: target };
        return { value: target, moving: false };
    }
    if (axis.target !== target) {
        axis.from = axis.value;
        axis.target = target;
        axis.start = now;
    }
    const t = Math.min(1, (now - axis.start) / 380);
    const moving = t < 1 && Math.abs(axis.target - axis.from) > 1e-6;
    axis.value = moving ? axis.from + (axis.target - axis.from) * usageEase(t) : axis.target;
    return { value: Math.max(axis.value, 1e-6), moving };
}

function scheduleUsageChart() {
    if (usageState.chartRaf) return;
    const step = () => {
        const moving = renderUsageChart(usageState.chartSource, false);
        usageState.chartRaf = moving ? requestAnimationFrame(step) : 0;
    };
    usageState.chartRaf = requestAnimationFrame(step);
}

function renderUsageChart(chart, updateLegend = true) {
    usageState.chartSource = chart;
    const now = performance.now();
    let seriesMoving = false;
    if (usageState.seriesAnim) {
        for (const anim of usageState.seriesAnim.values()) {
            const t = Math.min(1, (now - anim.start) / 380);
            anim.value = t >= 1 ? anim.target : anim.from + (anim.target - anim.from) * usageEase(t);
            if (t < 1) seriesMoving = true;
        }
        if (!seriesMoving) usageState.seriesAnim.clear();
    }
    const wrap = $("#usageChartWrap");
    const svg = $("#usageChart");
    const metric = USAGE_METRICS[usageState.metric];
    $("#usageChartTitle").textContent = metric.title;
    const built = usageChartSeries(chart, metric);
    const dates = built.dates;
    const all = built.series;
    const series = all
        .map((item) => ({ ...item, weight: usageSeriesWeight(item) }))
        .filter((item) => item.weight > 0)
        .map((item) => ({ ...item, cumulative: item.cumulative.map((value) => value * item.weight) }));
    const hasSeries = dates.length > 0 && all.length > 0;
    const hasData = dates.length > 0 && series.length > 0;
    $("#usageChartEmpty").hidden = hasSeries;
    if (updateLegend) renderUsageLegend(hasSeries ? all : []);
    hideUsageTooltip();
    const W = Math.round(wrap.clientWidth);
    const H = Math.round(wrap.clientHeight);
    if (!hasSeries || !W || !H || !hasData) {
        svg.innerHTML = "";
        usageState.chart = null;
        if (seriesMoving) scheduleUsageChart();
        return seriesMoving;
    }

    const n = dates.length;
    const stacked = [];
    series.forEach((item, i) => stacked.push(item.cumulative.map((value, j) => value + (i ? stacked[i - 1][j] : 0))));
    const { step, top: targetTop } = usageNiceScale(Math.max(...stacked.at(-1)));
    const axis = usageAxisTop(targetTop);
    const top = axis.value;
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
        areas += `<path class="usage-area" fill="${item.color}" d="${usageAreaPath(upper, lower)}"/>`;
        lines += `<path class="usage-line" stroke="${item.color}" d="${usageCurvePath(upper)}"/>`;
    });
    const dots = series.map((item) => `<circle class="usage-dot" r="3.5" fill="${item.color}" cx="0" cy="0"/>`).join("");

    svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
    svg.innerHTML = `${grid}${areas}${lines}<g class="usage-hover" visibility="hidden"><line class="usage-guide" x1="0" x2="0" y1="${padT}" y2="${padT + plotH}"/>${dots}</g>`;
    usageState.chart = { dates, series, stacked, n, W, padL, plotW, xOf, yOf, hover: -1 };
    const moving = seriesMoving || axis.moving;
    if (moving) scheduleUsageChart();
    return moving;
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

function usageRecordsNearBottom(el) {
    if (!el) return false;
    return el.scrollHeight - el.scrollTop - el.clientHeight < 96;
}

function handleUsageRecordsScroll() {
    const wrap = $("#usageTableWrap");
    if (wrap && wrap.clientHeight > 40 && wrap.scrollHeight > wrap.clientHeight + 4) {
        if (usageRecordsNearBottom(wrap)) loadMoreUsage();
        return;
    }
    if (state.page === "usage" && usageRecordsNearBottom(mainScroller())) loadMoreUsage();
}

function fillUsageRecords() {
    const wrap = $("#usageTableWrap");
    if (!wrap || wrap.clientHeight < 40 || state.page !== "usage" || usageState.loadingMore || !usageState.hasMore) return;
    if (wrap.scrollHeight <= wrap.clientHeight + 4) loadMoreUsage();
}

function renderUsageRecords() {
    const items = usageState.items;
    const total = usageState.total;
    $("#usageRows").innerHTML = items.length
        ? items.map((item) => {
            const label = item.label || item.model || "—";
            const id = item.model && item.model !== label ? `<small>${escapeHtml(item.model)}</small>` : "";
            const effort = String(item.reasoning_effort || "").trim().toLowerCase();
            const effortText = effort ? (EFFORT_LABELS[effort] || effort) : "—";
            const estimated = item.estimated ? `<span class="usage-est" title="上游未返回准确用量，此条费用为估算值">估算</span>` : "";
            const cacheRead = Number(item.cache_read_tokens) || 0;
            const cacheWrite = Number(item.cache_write_tokens) || 0;
            const input = Number.isFinite(item.input_tokens)
                ? item.input_tokens
                : Math.max(0, (Number(item.prompt_tokens) || 0) - cacheRead - cacheWrite);
            const cache = cacheRead || cacheWrite ? `${formatInt(cacheRead)} / ${formatInt(cacheWrite)}` : "—";
            return `<tr>
                <td class="time">${escapeHtml(formatRecordTime(item.time))}</td>
                <td class="model"><strong>${escapeHtml(label)}</strong>${id}</td>
                <td class="effort">${escapeHtml(effortText)}</td>
                <td class="num" title="含缓存总输入 ${formatInt(item.prompt_tokens)}">${formatInt(input)}</td>
                <td class="num">${cache}</td>
                <td class="num">${formatInt(item.completion_tokens)}</td>
                <td class="num cost">${estimated}${item.free ? `<span class="usage-free">免费</span>` : escapeHtml(formatRecordCost(item.cost))}</td>
            </tr>`;
        }).join("")
        : `<tr class="usage-empty-row"><td colspan="7">所选时间范围内暂无调用记录</td></tr>`;
    $("#usagePagerTotal").textContent = `已显示 ${formatInt(items.length)} / ${formatInt(total)} 条`;
    const hint = $("#usageMoreHint");
    if (hint) {
        if (usageState.loadingMore) hint.textContent = "正在加载…";
        else if (usageState.loadMoreError) hint.textContent = "加载失败，点击重试";
        else if (usageState.hasMore) hint.textContent = "滚到底部继续加载";
        else hint.textContent = items.length ? "没有更多记录了" : "";
    }
    requestAnimationFrame(fillUsageRecords);
}

/* ---------- 顶栏用户菜单 ---------- */

function setUserMenuOpen(open) {
    const menu = $("#userMenu");
    const button = $("#brandMark");
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
        // 等回包的这段时间里，历史日志可能已经整批载入并把游标推到了后面。
        // 只收游标之后的条目，不然同一条日志会在界面上出现两次。
        const entries = (Array.isArray(result?.entries) ? result.entries : [])
            .filter((entry) => Number(entry.id) > state.logCursor);
        if (entries.length) {
            state.logs.push(...entries);
            if (state.logs.length > 5000) {
                const removed = state.logs.length - 5000;
                state.logs.splice(0, removed);
                if (state.pauseIndex !== null) state.pauseIndex = Math.max(0, state.pauseIndex - removed);
            }
            state.logCursor = Math.max(state.logCursor, Number(result.latest_id || entries.at(-1).id || 0));
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
    if (!modelId || modelId === state.config.override_model) return;
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
    const previous = state.config[name];
    state.config[name] = value;
    const messages = {
        close_to_tray: ["关闭窗口时会最小化到托盘", "关闭窗口时会直接退出"],
        launch_at_login: ["已设置开机自启", "已取消开机自启"],
        block_devin_update: ["已关闭 Devin 更新", "已允许 Devin 更新"],
    };
    try {
        const result = await callApi("set_option", name, value);
        if (!result?.ok) throw new Error(result?.message || "设置失败");
        if (result.config) state.config = { ...state.config, ...result.config };
        const pair = messages[name];
        toast(pair ? (value ? pair[0] : pair[1]) : (value ? "启动选项已开启" : "启动选项已关闭"));
        return true;
    } catch (error) {
        state.config[name] = previous;
        const box = document.querySelector(`[data-option="${name}"]`);
        if (box) box.checked = Boolean(previous);
        toast(`选项保存失败：${errorMessage(error)}`, "error");
        return false;
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
        override_model: state.config?.override_model || "",
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
        applyLogHistoryMeta(result);
        state.logShown = Math.max(LOG_CHUNK, Math.min(allLogEntries().filter(logMatches).length, LOG_CHUNK * 3));
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
        state.logArchive = [];
        state.logFileSkip = 0;
        state.logFileHasMore = false;
        state.logCursor = Number(result?.latest_id || state.logCursor);
        state.pauseIndex = state.streaming ? null : 0;
        renderLogs();
    } catch (error) {
        toast(`清空失败：${errorMessage(error)}`, "error");
    }
}

// 模型页卡片和筛选栏不滚。指针在它们或页面空白上时，滚轮仍带动下面的模型列表。
function bindModelListPan() {
    const page = $("#page-models");
    if (!page || page.dataset.panBound) return;
    page.dataset.panBound = "1";
    page.addEventListener("wheel", (event) => {
        if (event.target.closest(".pick-menu, #modelList")) return;
        const list = $("#modelList");
        if (!list) return;
        const dy = event.deltaMode === 1 ? event.deltaY * 16 : event.deltaMode === 2 ? event.deltaY * list.clientHeight : event.deltaY;
        const max = list.scrollHeight - list.clientHeight;
        if (max <= 0) return;
        const next = Math.min(max, Math.max(0, list.scrollTop + dy));
        if (next === list.scrollTop) return;
        list.scrollTop = next;
        event.preventDefault();
    }, { passive: false });
}

async function openExternalLink(event, url) {
    event.preventDefault();
    try {
        const result = await callApi("open_url", url);
        if (!result?.ok) throw new Error(result?.message || "打开失败");
    } catch {
        window.open(url, "_blank", "noopener");
    }
}

// 输入：带 .segment-thumb 的分段控件、是否跳过动画。输出：无；把滑块挪到当前选中的按钮下面。
function placeSegmentThumb(bar, instant) {
    const thumb = bar && $(".segment-thumb", bar);
    const active = bar && $("button.active", bar);
    if (!thumb || !active) return;
    const width = active.offsetWidth;
    const x = active.offsetLeft;
    if (!width) return;
    if (thumb.dataset.w === String(width) && thumb.dataset.x === String(x)) return;
    thumb.dataset.w = String(width);
    thumb.dataset.x = String(x);
    if (instant) thumb.style.transition = "none";
    thumb.style.width = `${width}px`;
    thumb.style.transform = `translate3d(${x}px, 0, 0)`;
    if (instant) {
        thumb.getBoundingClientRect();
        thumb.style.transition = "";
    }
}

// 用量页的两个分段控件（图表指标、统计范围）共用同一套滑块。
function placeUsageThumbs(instant) {
    for (const bar of [$("#usageMetric"), $("#usageRange"), $("#usageViewSwitch"), $("#lbRange"), $("#lbMetric")]) {
        placeSegmentThumb(bar, instant);
    }
}

function bindEvents() {
    bindAccountEvents();
    watchVendorIcons();
    $$(".nav-item").forEach((item) => item.addEventListener("click", () => navigate(item.dataset.route)));
    placeNavIndicator(true);
    placeUsageThumbs(true);
    if ("ResizeObserver" in window) {
        const observer = new ResizeObserver(() => placeUsageThumbs(false));
        for (const bar of [$("#usageMetric"), $("#usageRange"), $("#usageViewSwitch"), $("#lbRange"), $("#lbMetric")]) {
            if (bar) observer.observe(bar);
        }
    }
    window.addEventListener("resize", () => {
        placeNavIndicator(true);
        placeUsageThumbs(true);
        if (state.page === "models") fillModelList();
    });
    const workspaceScroller = mainScroller();
    workspaceScroller?.addEventListener("scroll", () => {
        syncTopbarScroll();
        if (state.page === "usage") handleUsageRecordsScroll();
    }, { passive: true });
    $$("#page-models .model-list, #logViewport, #logPreview, #usageTableWrap, #poolScroll").forEach((scroller) => {
        scroller.addEventListener("scroll", () => syncTopbarScroll(), { passive: true });
    });
    syncTopbarScroll();
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
        navigate("log");
        $("#heartbeatPanel")?.scrollIntoView({ behavior: "smooth", block: "center" });
    });
    if ($("#brandChip")) {
        $("#brandChip").addEventListener("click", (event) => {
            event.stopPropagation();
            setLicensePopOpen(false);
            setUserMenuOpen(false);
            navigate("profile");
        });
    }
    $("#licenseChip").addEventListener("click", (event) => {
        event.stopPropagation();
        setUserMenuOpen(false);
        setLicensePopOpen($("#licensePop").hidden);
    });
    $("#licensePop").addEventListener("click", (event) => event.stopPropagation());
    document.addEventListener("click", () => setLicensePopOpen(false));
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
        $("#cursorhubLink").addEventListener("click", (event) => openExternalLink(event, "https://cursorhub.bitheart.top/"));
    }
    // 钱包弹层、首页卡片、个人页的入口都通向商城：买效益包、兑换卡密、充值都在那里。
    ["#openShopBtn", "#homeShopBtn", "#profileTopupBtn"].forEach((selector) => {
        const button = $(selector);
        if (button) button.addEventListener("click", () => {
            setLicensePopOpen(false);
            navigate("shop");
        });
    });
    if ($("#qqGroupBtn")) {
        $("#qqGroupBtn").addEventListener("click", async () => {
            const button = $("#qqGroupBtn");
            const aside = button.querySelector(".profile-card-action");
            try {
                await navigator.clipboard.writeText("386427683");
                button.classList.add("is-copied");
                if (aside) aside.textContent = "已复制";
                toast("QQ 群号已复制：386427683");
                setTimeout(() => {
                    button.classList.remove("is-copied");
                    if (aside) aside.textContent = "复制群号";
                }, 1600);
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
    bindModelListPan();
    $("#modelEffort").addEventListener("change", changeModelOption);
    $("#modelContext").addEventListener("change", changeModelOption);
    document.addEventListener("click", (event) => {
        const item = event.target.closest(".pick-item");
        if (item) {
            const pick = item.closest(".pick") || item.closest(".pick-menu")?._pick;
            const select = pick ? $("select", pick) : null;
            if (select && select.value !== item.dataset.value) {
                select.value = item.dataset.value;
                syncPick(select);
                select.dispatchEvent(new Event("change", { bubbles: true }));
            }
            closePicks();
            return;
        }
        const trigger = event.target.closest(".pick-trigger");
        if (trigger) {
            const pick = trigger.closest(".pick");
            const willOpen = !pick.classList.contains("is-open");
            closePicks();
            if (willOpen) {
                pick.classList.add("is-open");
                trigger.setAttribute("aria-expanded", "true");
                positionPickMenu(pick);
            }
            return;
        }
        if (!event.target.closest(".pick, .pick-menu")) closePicks();
    });
    $("#modelList")?.addEventListener("scroll", () => {
        closePicks();
        const list = $("#modelList");
        if (list && list.scrollHeight - list.scrollTop - list.clientHeight < 160) revealMoreModels();
        syncModelListFade();
    }, { passive: true });
    document.addEventListener("keydown", (event) => {
        if (event.key === "Escape") closePicks();
    });
    $("#modelFilters")?.addEventListener("click", (event) => {
        const chip = event.target.closest(".model-filter-chip");
        if (!chip) return;
        const key = chip.dataset.key || "all";
        const row = chip.closest(".model-filter-row");
        if (row?.id === "modelSeriesRow") {
            if (modelWall.series === key) return;
            modelWall.series = key;
        } else if (row?.id === "modelCategoryRow") {
            if (modelWall.category === key) return;
            modelWall.category = key;
        } else if (row?.id === "modelRateRow") {
            if (modelWall.rate === key) return;
            modelWall.rate = key;
        } else {
            return;
        }
        persistModelWallPrefs();
        renderModelWall(state.config.override_model);
    });
    $("#modelFreeOnly").addEventListener("change", (event) => {
        modelWall.freeOnly = event.target.checked;
        persistModelWallPrefs();
        renderModelWall(state.config.override_model);
    });
    $("#modelSearch").addEventListener("input", (event) => {
        modelWall.search = event.target.value.trim().toLowerCase();
        renderModelWall(state.config.override_model);
    });
    $("#modelSortName")?.addEventListener("click", () => chooseModelSort("name"));
    $("#modelSortPrice")?.addEventListener("click", () => chooseModelSort("price"));
    $("#restartToggle").addEventListener("change", (event) => changeOption("restart_devin_on_start", event.target.checked));
    $("#closeToTrayToggle")?.addEventListener("change", async (event) => {
        const ok = await changeOption("close_to_tray", event.target.checked);
        if (ok && typeof window.bailanWin === "function") window.bailanWin("tray-sync").catch(() => {});
    });
    $("#launchAtLoginToggle")?.addEventListener("change", (event) => changeOption("launch_at_login", event.target.checked));
    $("#blockDevinUpdateToggle")?.addEventListener("change", (event) => changeOption("block_devin_update", event.target.checked));
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
        state.logShown = LOG_CHUNK;
        $$("[data-log-filter]").forEach((item) => item.classList.toggle("active", item === button));
        scheduleLogRender();
    }));
    $("#logSearch").addEventListener("input", (event) => {
        state.logSearch = event.target.value.trim().toLocaleLowerCase();
        state.logShown = LOG_CHUNK;
        scheduleLogRender();
    });
    if ($("#logViewport")) $("#logViewport").addEventListener("scroll", handleLogViewportScroll, { passive: true });
    $("#streamBtn").addEventListener("click", () => {
        state.streaming = !state.streaming;
        state.pauseIndex = state.streaming ? null : state.logs.length;
        renderLogs();
    });
    $("#loadHistoryBtn").addEventListener("click", loadHistory);
    $("#clearLogsBtn").addEventListener("click", clearLogs);
    $("#autoScroll").addEventListener("change", (event) => {
        state.autoScroll = event.target.checked;
        if (state.autoScroll) {
            renderLogs();
            const viewport = $("#logViewport");
            if (viewport) viewport.scrollTop = 0;
        }
    });
    document.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
            if (!$("#modalBackdrop").hidden) {
                $("#modalCancel").click();
                return;
            }
            setUserMenuOpen(false);
        }
        if (event.ctrlKey && event.key.toLocaleLowerCase() === "k") {
            event.preventDefault();
            navigate("log");
            $("#logSearch").focus();
        }
        if (event.ctrlKey && ["1", "2", "3", "4", "5"].includes(event.key)) {
            event.preventDefault();
            navigate(["home", "models", "usage", "log", "config"][Number(event.key) - 1]);
        }
        if (event.ctrlKey && event.key === "6") {
            event.preventDefault();
            navigate("pool");
        }
        if (event.ctrlKey && event.key === "7") {
            event.preventDefault();
            navigate("shop");
        }
    });
    if ($("#usageLegend")) $("#usageLegend").addEventListener("click", (event) => {
        const button = event.target.closest("[data-usage-series]");
        if (!button || !usageState.data) return;
        const key = button.dataset.usageSeries || "";
        const willHide = !usageState.hiddenModels.has(key);
        if (willHide) usageState.hiddenModels.add(key);
        else usageState.hiddenModels.delete(key);
        button.classList.toggle("is-off", willHide);
        button.setAttribute("aria-pressed", willHide ? "false" : "true");
        button.title = willHide ? "点击显示" : "点击隐藏";
        const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
        const anims = usageState.seriesAnim || (usageState.seriesAnim = new Map());
        const prev = anims.get(key);
        const from = prev ? prev.value : (willHide ? 1 : 0);
        const target = willHide ? 0 : 1;
        if (reduced) anims.delete(key);
        else anims.set(key, { from, value: from, target, start: performance.now() });
        renderUsageChart(usageState.data.chart, false);
    });
    $$("[data-usage-metric]").forEach((button) => button.addEventListener("click", () => {
        usageState.metric = button.dataset.usageMetric;
        usageState.axis = null;
        if (usageState.seriesAnim) usageState.seriesAnim.clear();
        $$("[data-usage-metric]").forEach((item) => item.classList.toggle("active", item === button));
        placeSegmentThumb($("#usageMetric"), false);
        if (usageState.data) renderUsageChart(usageState.data.chart);
    }));
    $$("[data-usage-days]").forEach((button) => button.addEventListener("click", () => {
        const days = Number(button.dataset.usageDays);
        if (days === usageState.days && usageState.data) return;
        usageState.days = days;
        usageState.page = 1;
        $$("[data-usage-days]").forEach((item) => item.classList.toggle("active", item === button));
        placeSegmentThumb($("#usageRange"), false);
        loadUsage();
    }));
    $$("[data-usage-view]").forEach((button) => button.addEventListener("click", () => {
        if (button.dataset.usageView === usageState.view) return;
        showUsageSection(button.dataset.usageView);
    }));
    $$("[data-lb-range]").forEach((button) => button.addEventListener("click", () => {
        boardState.range = button.dataset.lbRange;
        $$("[data-lb-range]").forEach((item) => item.classList.toggle("active", item === button));
        placeSegmentThumb($("#lbRange"), false);
        loadLeaderboard();
    }));
    $$("[data-lb-metric]").forEach((button) => button.addEventListener("click", () => {
        boardState.metric = button.dataset.lbMetric;
        $$("[data-lb-metric]").forEach((item) => item.classList.toggle("active", item === button));
        placeSegmentThumb($("#lbMetric"), false);
        loadLeaderboard();
    }));
    if ($("#lbMore")) $("#lbMore").addEventListener("click", () => loadLeaderboard({ append: true }));
    if ($("#usageRank")) $("#usageRank").addEventListener("click", async (event) => {
        const join = event.target.closest("#lbJoin");
        if (!join) return;
        join.disabled = true;
        try {
            const result = await callApi("set_leaderboard", true);
            if (!result?.ok) throw new Error(result?.message || "设置失败");
            await loadLeaderboard();
        } catch (error) {
            toast(`排行设置失败：${errorMessage(error)}`, "error");
        } finally {
            join.disabled = false;
        }
    });
    if ($("#leaderboardOptIn")) $("#leaderboardOptIn").addEventListener("change", async (event) => {
        const input = event.currentTarget;
        const want = !!input.checked;
        input.disabled = true;
        try {
            const result = await callApi("set_leaderboard", want);
            if (!result?.ok) throw new Error(result?.message || "设置失败");
            toast(want ? "已加入排行榜，你的头像与用量将对所有用户可见。" : "已退出排行榜。", "success");
            await loadLeaderboard();
        } catch (error) {
            input.checked = !want;
            toast(`排行设置失败：${errorMessage(error)}`, "error");
        } finally {
            input.disabled = false;
        }
    });
    if ($("#usageTableWrap")) $("#usageTableWrap").addEventListener("scroll", handleUsageRecordsScroll, { passive: true });
    if ($("#usageMoreHint")) $("#usageMoreHint").addEventListener("click", () => {
        if (!usageState.loadMoreError) return;
        usageState.loadMoreError = false;
        loadMoreUsage();
    });
    if ($("#poolSortAvail")) $("#poolSortAvail").addEventListener("click", () => choosePoolSort("avail"));
    if ($("#poolSortReset")) $("#poolSortReset").addEventListener("click", () => choosePoolSort("reset"));
    if ($("#poolRefreshBtn")) $("#poolRefreshBtn").addEventListener("click", () => loadPoolStatus());
    if ($("#poolStats")) $("#poolStats").addEventListener("click", (event) => {
        const button = event.target.closest("[data-pool-filter]");
        if (!button) return;
        const next = button.dataset.poolFilter || "all";
        if (next === poolState.filter) return;
        poolState.filter = next;
        renderPool({ animate: true });
    });
    if ($("#poolEmptyAction")) $("#poolEmptyAction").addEventListener("click", () => {
        if ($("#poolEmpty").dataset.view !== "inactive") {
            loadPoolStatus();
            return;
        }
        navigate("shop");
    });
    if ($("#usageEmptyAction")) $("#usageEmptyAction").addEventListener("click", () => {
        if ($("#usageEmpty").dataset.view !== "inactive") {
            loadUsage();
            return;
        }
        navigate("shop");
    });
    if ($("#shopEmptyAction")) $("#shopEmptyAction").addEventListener("click", () => loadShop());
    if ($("#shopGrid")) $("#shopGrid").addEventListener("click", (event) => {
        const button = event.target.closest("[data-shop-buy], [data-shop-topup]");
        if (!button) return;
        if (button.dataset.shopTopup !== undefined) {
            $("#shopRedeemInput").focus();
            return;
        }
        handleShopBuy(Number(button.dataset.shopBuy), button);
    });
    if ($("#shopRedeemBtn")) $("#shopRedeemBtn").addEventListener("click", handleShopRedeem);
    if ($("#shopRedeemInput")) $("#shopRedeemInput").addEventListener("keydown", (e) => { if (e.key === "Enter") handleShopRedeem(); });
    if ($("#shopBuyKeyLink")) {
        $("#shopBuyKeyLink").addEventListener("click", (event) => openExternalLink(event, $("#shopBuyKeyLink").href));
    }
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
    hide("#relaySection");
    hide("#advancedSection");
    const opSmall = $(".operator-card > small");
    if (opSmall) opSmall.textContent = customer ? "客户版 · Devin" : "Local capture · Devin";
    const cfgDesc = $("#configPageDesc");
    if (cfgDesc && customer) cfgDesc.textContent = "Devin 程序和本地证书。";
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
        // 产品名和版本号写在个人页、设置页的最底下，客户报问题时能说清自己是哪个版本。
        const edition = state.edition === "customer" ? "" : " · 完整版";
        const versionText = `${data.app_title || "DevinHub"} v${data.app_version || "—"}${edition}`;
        $$("[data-app-version]").forEach((item) => { item.textContent = versionText; });
        renderAnnouncement(data.announcement || null);
        renderUpdate(data.update || null);
        $("#configPath").textContent = `配置文件位置：${data.config_path || "—"}`;
        $("#certPath").textContent = data.cert_path || "—";
        renderAdmin(Boolean(data.admin));
        if (typeof adoptServerUiScale === "function" && data.ui_scale_percent != null) {
            adoptServerUiScale(data.ui_scale_percent);
        }
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
    const isDragArea = (target) => !target.closest("button, a, input, textarea, select, label, .brand, .pick, .model-card, .segment-control, .search-box, .switch, .log-line, .preview-line, .model-list, .user-menu, .license-pop, .win-resize-top");

    root.classList.add("framed-app");
    $("#winControls").hidden = false;
    topbar.addEventListener("mousedown", (event) => {
        if (event.button !== 0 || event.detail > 1) return;
        if (event.target.closest(".win-resize-top")) winCall("resize-top");
        else if (isDragArea(event.target)) winCall("drag");
    });
    // 顶栏以外的空白（卡片底、页面空隙）同样可以拖动窗口；模型列表留给滚动。
    document.addEventListener("mousedown", (event) => {
        if (event.button !== 0 || event.detail > 1 || event.target.closest(".topbar")) return;
        if (isDragArea(event.target)) winCall("drag");
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
