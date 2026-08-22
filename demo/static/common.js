// 跨页面共享的工具函数。挂在 window.OC 上。
window.OC = {
  // fetch 封装：非 2xx 抛错（错误信息为响应体文本），2xx 返回解析后的 JSON。
  async api(path, opts) {
    const r = await fetch(path, opts);
    const text = await r.text();
    if (!r.ok) throw new Error(text || ('HTTP ' + r.status));
    return text ? JSON.parse(text) : null;
  },
  json(method, body) {
    return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
  },
  // 秒 -> "X 天 Y 小时" / "X 小时 Y 分钟" / "X 分钟"
  fmtDur(sec) {
    sec = Math.max(0, Math.floor(sec));
    const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
    if (d > 0) return d + ' 天 ' + h + ' 小时';
    if (h > 0) return h + ' 小时 ' + m + ' 分钟';
    return m + ' 分钟';
  },
  fmtDate(iso) {
    const d = new Date(iso), p = n => String(n).padStart(2, '0');
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
  },
  fmtClock(iso) {
    const d = new Date(iso), p = n => String(n).padStart(2, '0');
    return p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds());
  },
};
