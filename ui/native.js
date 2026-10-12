// native.js —— 把旧版 pywebview 的 window.pywebview.api 桥接到本地 Go 控制服务。
// 每个 api.<method>(...args) 调用都会 POST /api/<method>，请求体是参数数组的 JSON。
(function () {
    async function call(method, args) {
        const res = await fetch("/api/" + method, {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify(args || []),
        });
        const text = await res.text();
        let data = null;
        if (text) {
            try {
                data = JSON.parse(text);
            } catch (_e) {
                data = text;
            }
        }
        if (!res.ok) {
            const message = (data && (data.message || data.detail)) || ("HTTP " + res.status);
            throw new Error(message);
        }
        return data;
    }

    const api = new Proxy(
        {},
        {
            get(_target, prop) {
                if (typeof prop !== "string") return undefined;
                return (...args) => call(prop, args);
            },
        }
    );

    window.pywebview = { api };

    window.addEventListener("DOMContentLoaded", () => {
        window.dispatchEvent(new Event("pywebviewready"));
    });
})();
