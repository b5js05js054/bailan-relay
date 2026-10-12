/*
 * theme.js
 * Input: persisted theme preference and #themeToggle click events.
 * Output: document theme, native color-scheme, visible status, icon, and ARIA state.
 * Use: keep the day/night control available across all pages and reloads.
 */
const THEME_STORAGE_KEY = "bailan.ui.theme";

/** Read the saved theme; the dark palette is the default unless the user chose day. */
function readThemePreference() {
    try {
        return localStorage.getItem(THEME_STORAGE_KEY) === "day" ? "day" : "night";
    } catch {
        return "night";
    }
}

/** Apply a theme to the document and synchronize the control's accessible state. */
function applyTheme(theme) {
    const isNight = theme === "night";
    const root = document.documentElement;
    const button = document.getElementById("themeToggle");

    root.dataset.theme = isNight ? "night" : "day";
    root.style.colorScheme = isNight ? "dark" : "light";
    if (!button) return;

    button.setAttribute("aria-pressed", String(isNight));
    button.setAttribute("aria-label", isNight ? "关闭夜间模式" : "开启夜间模式");
    const stateLabel = document.getElementById("themeToggleState");
    if (stateLabel) stateLabel.textContent = isNight ? "已开启" : "已关闭";

    const icon = button.querySelector("use");
    if (icon) icon.setAttribute("href", isNight ? "#i-sun" : "#i-moon");
}

/** Switch palettes and persist the new value when storage is available. */
function toggleTheme() {
    const nextTheme = document.documentElement.dataset.theme === "night" ? "day" : "night";
    applyTheme(nextTheme);
    try {
        localStorage.setItem(THEME_STORAGE_KEY, nextTheme);
    } catch { /* 存储不可用时，本次运行仍可切换主题 */ }
}

document.addEventListener("DOMContentLoaded", () => {
    applyTheme(readThemePreference());
    const button = document.getElementById("themeToggle");
    if (button) button.addEventListener("click", toggleTheme);
});
