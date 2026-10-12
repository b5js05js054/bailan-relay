// account.js —— 账号：登录 / 注册（头像必传）/ 自定义背景（仅存本地）/ 退出登录。
// 依赖 app.js 的全局：$、$$、callApi、errorMessage、toast、setButtonBusy、setUserMenuOpen、
// confirmAction、formatDownloadBytes、pollStatus。头像上传由 Go 侧发送并计字节，这里轮询进度。

const EMAIL_RE = /^[1-9][0-9]{4,11}@qq\.com$/;
const IMAGE_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"];
const AVATAR_MAX = 2 * 1024 * 1024;
const BACKGROUND_MAX = 5 * 1024 * 1024;
const accountState = { loggedIn: null, user: null, mode: "login", avatar: null, uploading: false };

// paintAvatar 输入：头像元素、图片地址（空则显示默认图标）。输出：无。
function paintAvatar(el, url) {
    if (!el) return;
    el.classList.toggle("has-image", Boolean(url));
    el.style.backgroundImage = url ? `url("${url}")` : "";
    el.innerHTML = url ? "" : '<svg><use href="#i-user"/></svg>';
}

// renderAccount 输入：后端 account 视图 {logged_in, user}。输出：无；切换登录遮罩、顶栏资料与自定义背景。
function renderAccount(account) {
    const loggedIn = Boolean(account?.logged_in);
    const user = loggedIn ? account.user || {} : null;
    accountState.loggedIn = loggedIn;
    accountState.user = user;
    document.body.classList.toggle("logged-out", !loggedIn);
    $("#authOverlay").hidden = loggedIn;
    if (!loggedIn) setUserMenuOpen(false);
    $("#userChipEmail").textContent = user?.email || "未登录";
    $("#userMenuEmail").textContent = user?.email || "未登录";
    paintAvatar($("#userChipAvatar"), user?.avatar_url);
    paintAvatar($("#userMenuAvatar"), user?.avatar_url);
    const bg = user?.background_url || "";
    document.body.classList.toggle("has-custom-bg", Boolean(bg));
    document.body.style.setProperty("--user-bg", bg ? `url("${bg}")` : "none");
    $("#resetBgBtn").hidden = !bg;
}

// refreshAccount 启动时调用：Go 侧向后台校验令牌（失效自动登出）后渲染。
async function refreshAccount() {
    try {
        const result = await callApi("user_session");
        renderAccount(result?.account);
    } catch (error) {
        renderAccount(null);
        setAuthError(`无法读取登录状态：${errorMessage(error)}`);
    }
}

function setAuthError(message) {
    $("#authError").textContent = message || "";
}

function setAuthMode(mode) {
    accountState.mode = mode;
    const register = mode === "register";
    $("#authForm").dataset.mode = mode;
    $$("[data-auth-mode]").forEach((btn) => btn.classList.toggle("active", btn.dataset.authMode === mode));
    $("#authTitle").textContent = register ? "注册白岚账号" : "登录白岚";
    $("#authSubtitle").textContent = register ? "仅支持 QQ 邮箱注册，需要上传头像" : "使用 QQ 邮箱账号登录后即可使用";
    $("#authSubmit").textContent = register ? "注册并登录" : "登录";
    $("#authPassword").autocomplete = register ? "new-password" : "current-password";
    $("#authProgress").hidden = true;
    setAuthError("");
}

// checkImage 输入：File、大小上限、用途文案。输出：错误文案（合法返回空串）。
function checkImage(file, max, label) {
    if (!IMAGE_TYPES.includes(file.type)) return `${label}仅支持 jpg / png / webp / gif`;
    if (file.size > max) return `${label}不能超过 ${max / 1024 / 1024}MB`;
    return "";
}

function readAsDataURL(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result));
        reader.onerror = () => reject(reader.error || new Error("读取图片失败"));
        reader.readAsDataURL(file);
    });
}

// renderUploadProgress 输入：Go 侧头像上传进度 {phase, sent, total, percent}。输出：无。
function renderUploadProgress(progress) {
    const percent = Math.min(100, Math.max(0, Number(progress.percent || 0)));
    const phase = progress.phase || "uploading";
    const panel = $("#authProgress");
    panel.hidden = false;
    panel.dataset.phase = phase === "done" ? "ready" : phase;
    $("#authProgressTrack").setAttribute("aria-valuenow", String(Math.round(percent)));
    $("#authProgressFill").style.width = `${percent}%`;
    $("#authProgressPercent").textContent = `${Math.round(percent)}%`;
    $("#authProgressText").textContent = {
        uploading: "正在上传头像…",
        processing: "上传完成，服务器处理中…",
        done: "上传完成",
        error: progress.message || "上传失败",
    }[phase] || "正在上传头像…";
    $("#authProgressBytes").textContent = progress.total > 0
        ? `${formatDownloadBytes(progress.sent)} / ${formatDownloadBytes(progress.total)}`
        : "正在连接服务器";
}

// withUploadProgress 输入：执行上传的 Promise 工厂。输出：上传接口结果。
// 上传期间每 150ms 拉一次 Go 侧进度；结束后再刷一次以显示最终状态。
async function withUploadProgress(run) {
    accountState.uploading = true;
    renderUploadProgress({ phase: "uploading", sent: 0, total: 0, percent: 0 });
    const poll = async () => {
        try {
            const result = await callApi("user_upload_progress");
            if (result?.progress?.phase) renderUploadProgress(result.progress);
        } catch { /* 进度拉取失败不影响上传本身 */ }
    };
    const timer = setInterval(poll, 150);
    try {
        return await run();
    } finally {
        clearInterval(timer);
        await poll();
        accountState.uploading = false;
    }
}

// pickImage 输入：用作提示语的标签、网页 file input。输出：{name, type, size, dataUrl} 或 null（取消/失败已提示）。
// macOS 的 WKWebView 在 root 运行时弹不出文件面板，改走 Go 侧系统文件框；其他平台用网页 file input。
async function pickImage(label, input) {
    if (state.platform !== "darwin" || !state.native) {
        input.click();
        return null;
    }
    const result = await callApi("pick_image", `选择${label}`);
    if (result?.ok) return { name: result.name, type: result.type, size: result.size, dataUrl: result.data_url };
    if (!result?.cancelled) throw new Error(result?.message || "读取图片失败");
    return null;
}

// fileImage 输入：file input 的 change 事件。输出：与 pickImage 相同结构的图片对象或 null。
async function fileImage(event) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return null;
    return { name: file.name, type: file.type, size: file.size, file };
}

async function handleAvatarPick(image) {
    if (!image) return;
    const problem = checkImage(image, AVATAR_MAX, "头像");
    if (problem) {
        setAuthError(problem);
        return;
    }
    const dataUrl = image.dataUrl || await readAsDataURL(image.file);
    accountState.avatar = { dataUrl, name: image.name };
    paintAvatar($("#avatarPreview"), dataUrl);
    $("#authProgress").hidden = true;
    setAuthError("");
}

// validateAuthForm 输出：{email, password} 或 null（已显示错误）。
function validateAuthForm() {
    const email = $("#authEmail").value.trim().toLowerCase();
    const password = $("#authPassword").value;
    const register = accountState.mode === "register";
    const error = !EMAIL_RE.test(email) ? "请输入正确的 QQ 邮箱（数字@qq.com）"
        : password.length < 6 || password.length > 64 ? "密码长度需为 6 ~ 64 位"
        : register && password !== $("#authConfirm").value ? "两次输入的密码不一致"
        : register && !accountState.avatar ? "注册需要上传头像"
        : "";
    setAuthError(error);
    return error ? null : { email, password };
}

async function handleAuthSubmit(event) {
    event.preventDefault();
    if (accountState.uploading) return;
    const form = validateAuthForm();
    if (!form) return;
    const button = $("#authSubmit");
    setButtonBusy(button, true);
    try {
        const result = accountState.mode === "register"
            ? await withUploadProgress(() => callApi("user_register", {
                email: form.email,
                password: form.password,
                avatar: accountState.avatar.dataUrl,
                filename: accountState.avatar.name,
            }))
            : await callApi("user_login", form.email, form.password);
        if (!result?.ok) throw new Error(result?.message || "操作失败");
        $("#authPassword").value = "";
        $("#authConfirm").value = "";
        accountState.avatar = null;
        paintAvatar($("#avatarPreview"), "");
        $("#authProgress").hidden = true;
        renderAccount(result.account);
        toast(result.message || "登录成功");
        pollStatus(true);
    } catch (error) {
        setAuthError(errorMessage(error));
    } finally {
        setButtonBusy(button, false);
    }
}

// applyAccountResult 处理背景/登出接口结果；令牌失效时后端已登出，这里同步回登录页。
function applyAccountResult(result) {
    if (result?.account) renderAccount(result.account);
    if (!result?.ok) throw new Error(result?.message || "操作失败");
    toast(result.message || "已完成");
}

// handleBackgroundPick 选图后交给 Go 侧保存到本地（按账号区分），不上传服务器。
async function handleBackgroundPick(image) {
    if (!image) return;
    const problem = checkImage(image, BACKGROUND_MAX, "背景图");
    if (problem) {
        toast(problem, "error");
        return;
    }
    setUserMenuOpen(false);
    try {
        const dataUrl = image.dataUrl || await readAsDataURL(image.file);
        applyAccountResult(await callApi("user_set_background", { image: dataUrl }));
    } catch (error) {
        toast(`更换背景失败：${errorMessage(error)}`, "error");
    }
}

async function handleResetBackground() {
    setUserMenuOpen(false);
    try {
        applyAccountResult(await callApi("user_clear_background"));
    } catch (error) {
        toast(`恢复默认背景失败：${errorMessage(error)}`, "error");
    }
}

async function handleLogout() {
    setUserMenuOpen(false);
    const ok = await confirmAction("退出登录", "退出后会停止拦截，需要重新登录才能使用。确定吗？", "退出登录");
    if (!ok) return;
    try {
        applyAccountResult(await callApi("user_logout"));
        setAuthMode("login");
        pollStatus(true);
    } catch (error) {
        toast(`退出登录失败：${errorMessage(error)}`, "error");
    }
}

// bindAccountEvents 由 app.js 的 bindEvents 调用一次。
function bindAccountEvents() {
    $$("[data-auth-mode]").forEach((btn) => btn.addEventListener("click", () => setAuthMode(btn.dataset.authMode)));
    $("#authForm").addEventListener("submit", handleAuthSubmit);
    $("#avatarInput").addEventListener("change", async (e) => handleAvatarPick(await fileImage(e)));
    $("#avatarPicker").addEventListener("click", async (e) => {
        if (state.platform !== "darwin" || !state.native) return;
        e.preventDefault();
        try {
            await handleAvatarPick(await pickImage("头像", $("#avatarInput")));
        } catch (error) {
            setAuthError(errorMessage(error));
        }
    });
    $("#changeBgBtn").addEventListener("click", async () => {
        try {
            await handleBackgroundPick(await pickImage("背景图", $("#bgFileInput")));
        } catch (error) {
            toast(`更换背景失败：${errorMessage(error)}`, "error");
        }
    });
    $("#bgFileInput").addEventListener("change", async (e) => handleBackgroundPick(await fileImage(e)));
    $("#resetBgBtn").addEventListener("click", handleResetBackground);
    $("#logoutBtn").addEventListener("click", handleLogout);
}
