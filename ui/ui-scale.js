/*
 * ui-scale.js — 界面整体缩放：Win32 外框 + 页面 zoom 同步（WebView2）。
 */
const UI_SCALE_STORAGE_KEY = "bailan.ui.scale";
const UI_SCALE_OPTIONS = [80, 90, 100, 110, 120];

function normalizeUiScalePercent(raw) {
    const n = Number(raw);
    if (UI_SCALE_OPTIONS.includes(n)) return n;
    const parsed = Math.round(Number.parseFloat(raw) * 100);
    if (UI_SCALE_OPTIONS.includes(parsed)) return parsed;
    return 100;
}

function readUiScalePercent() {
    try {
        return normalizeUiScalePercent(localStorage.getItem(UI_SCALE_STORAGE_KEY) || "100");
    } catch {
        return 100;
    }
}

function persistUiScale(percent) {
    const value = normalizeUiScalePercent(percent);
    try {
        localStorage.setItem(UI_SCALE_STORAGE_KEY, String(value));
    } catch { /* 存储不可用时仅本次生效 */ }
    return value;
}

/** 页面 zoom + 原生窗口外框（桌面客户端） */
function applyUiScale(percent, { syncNative = true } = {}) {
    const value = normalizeUiScalePercent(percent);
    const zoom = value / 100;
    const root = document.documentElement;
    root.dataset.uiScale = String(value);
    root.style.setProperty("--ui-scale", String(zoom));
    root.style.zoom = "";
    const shell = document.getElementById("appShell");
    if (shell) shell.style.zoom = value === 100 ? "" : String(zoom);
    syncUiScaleControl(value);
    if (syncNative && typeof window.bailanUiScale === "function") {
        window.bailanUiScale(value).catch(() => {});
    }
    return value;
}

function syncUiScaleControl(activePercent) {
    const control = document.getElementById("uiScaleControl");
    if (!control) return;
    control.querySelectorAll("[data-ui-scale]").forEach((button) => {
        const on = Number(button.dataset.uiScale) === activePercent;
        button.classList.toggle("active", on);
        button.setAttribute("aria-pressed", String(on));
    });
}

function bindUiScaleControl() {
    const control = document.getElementById("uiScaleControl");
    if (!control || control.dataset.bound === "1") return;
    control.dataset.bound = "1";
    control.addEventListener("click", (event) => {
        const button = event.target.closest("[data-ui-scale]");
        if (!button) return;
        const percent = persistUiScale(button.dataset.uiScale);
        applyUiScale(percent, { syncNative: true });
    });
}

/** 与 Go 侧 ui-scale.txt / bootstrap 对齐，避免只有页面 zoom、外框不变。 */
function adoptServerUiScale(percent) {
    const value = normalizeUiScalePercent(percent);
    persistUiScale(value);
    applyUiScale(value, { syncNative: true });
}

window.adoptServerUiScale = adoptServerUiScale;

document.addEventListener("DOMContentLoaded", () => {
    bindUiScaleControl();
    const fromStore = readUiScalePercent();
    applyUiScale(fromStore, { syncNative: true });
});
