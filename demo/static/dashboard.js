// 监控页 Vue 应用。
const { createApp } = Vue;

createApp({
  data() {
    return {
      accounts: [], form: emptyForm(), bulkText: '',
      mode: 'single', showAdd: false, showImport: false, importText: '', importName: '', importError: '', busy: false, refreshing: false,
      msg: '', lastSync: null,
      loaded: false, barReady: false,
      settings: { warnPercent: 80, expirySoonDays: 3 },
      page: 1, pageSize: 24,
      menu: { show: false, x: 0, y: 0, acc: null },
      infoAcc: null, referralAcc: null,
    };
  },
  computed: {
    // 按创建顺序展示（服务端返回即为升序，最新添加在最后），配合分页。
    totalPages() { return Math.max(1, Math.ceil(this.accounts.length / this.pageSize)); },
    pagedAccounts() {
      const start = (this.page - 1) * this.pageSize;
      return this.accounts.slice(start, start + this.pageSize);
    },
    // 页码序列：页数少时全列，否则首尾 + 当前页附近，中间用 '...' 省略。
    pageList() {
      const t = this.totalPages, c = this.page;
      if (t <= 7) return Array.from({ length: t }, (_, i) => i + 1);
      const nums = [...new Set([1, 2, c - 1, c, c + 1, t - 1, t])].filter(n => n >= 1 && n <= t).sort((a, b) => a - b);
      const out = [];
      let prev = 0;
      for (const n of nums) {
        if (n - prev > 1) out.push('...');
        out.push(n);
        prev = n;
      }
      return out;
    },
    infoRows() {
      const a = this.infoAcc;
      if (!a) return [];
      const rows = [
        { k: '账号', v: a.account || '—' },
        { k: '密码', v: a.password || '—' },
        { k: '辅助邮箱', v: a.auxEmail || '—' },
        { k: '工作空间 ID', v: a.workspaceID || '—' },
        { k: 'Auth', v: a.hasAuth ? a.auth : '未配置', cls: a.hasAuth ? 'text-ink' : 'text-mute' },
        { k: 'API Key', v: a.hasKey ? a.apiKey : '未配置', cls: a.hasKey ? 'text-ink' : 'text-mute' },
        { k: '状态', v: a.status },
        { k: '页面登录', v: a.reportEmail || '—' },
        { k: '到期时间', v: a.expiresAt ? this.fmtDate(a.expiresAt) : '—' },
        { k: '剩余', v: a.expiresAt ? (a.expiresIn > 0 ? OC.fmtDur(a.expiresIn) : '已到期') : '—' },
        { k: '最后抓取', v: a.lastChecked ? this.fmtDate(a.lastChecked) : '尚未抓取' },
        { k: '转发次数', v: String(a.proxyCount) },
        { k: '待领取奖励', v: a.unclaimedRewards > 0 ? a.unclaimedRewards + ' 个' : '无', cls: a.unclaimedRewards > 0 ? 'text-warn' : 'text-mute' },
      ];
      if (a.error) rows.push({ k: '错误', v: a.error, cls: 'text-danger' });
      return rows;
    },
    stats() {
      const a = this.accounts;
      const ok = a.filter(x => x.status === 'ok').length;
      const soon = a.filter(x => this.soon(x)).length;
      const bad = a.filter(x => x.status === 'error' || this.tight(x)).length;
      return [
        { label: '总账号', value: a.length, cls: 'text-ink-strong' },
        { label: '正常', value: ok, cls: 'text-primary' },
        { label: '即将到期', value: soon, cls: soon ? 'text-warn' : 'text-ink-strong' },
        { label: '异常/紧张', value: bad, cls: bad ? 'text-danger' : 'text-ink-strong' },
      ];
    },
    aggRows() {
      return [
        { label: '滚动', pct: this.avg('rolling') },
        { label: '每周', pct: this.avg('weekly') },
        { label: '每月', pct: this.avg('monthly') },
      ];
    },
  },
  methods: {
    fmtDur: OC.fmtDur, fmtDate: OC.fmtDate, fmtClock: OC.fmtClock,
    async load() {
      this.accounts = await OC.api('/api/accounts');
      this.lastSync = new Date().toISOString();
      if (this.page > this.totalPages) this.page = this.totalPages;
      this.loaded = true;
    },
    openMenu(e, a) {
      this.menu = {
        show: true, acc: a,
        x: Math.min(e.clientX, window.innerWidth - 150),
        y: Math.min(e.clientY, window.innerHeight - 210),
      };
    },
    menuAction(kind) {
      const a = this.menu.acc; this.menu.show = false;
      if (kind === 'refresh') this.refreshOne(a);
      else if (kind === 'china') this.toggleChina(a);
      else if (kind === 'info') this.openInfo(a);
      else if (kind === 'auth') this.editField(a, 'auth', 'Auth（Cookie 头）');
      else if (kind === 'key') this.editField(a, 'apiKey', 'API Key');
      else if (kind === 'del') this.del(a);
    },
    openInfo(a) { this.infoAcc = a; },
    async toggleChina(a) {
      const enabled = !a.useChinaProviders;
      try { Object.assign(a, await OC.api('/api/accounts/' + a.id + '/china-providers', OC.json('POST', { enabled }))); this.flash(enabled ? '已开启中国模型' : '已关闭中国模型'); }
      catch (e) { this.flash('设置中国模型失败：' + e.message); }
    },
    openReferrals(a) { this.referralAcc = a; },
    async claimReferral(a, ref, idx) {
      if (ref._busy) return;
      ref._busy = true;
      try {
        const updated = await OC.api('/api/accounts/' + a.id + '/referrals/claim', OC.json('POST', { index: idx }));
        Object.assign(a, updated);
        this.referralAcc = a; // 用刷新后的数据重渲染列表
        this.flash('已领取奖励');
      } catch (e) {
        this.flash('领取失败：' + e.message);
      } finally { ref._busy = false; }
    },
    expiryLabel(a) {
      if (!a.expiresAt) return '';
      if (a.expiresIn <= 0) return '已到期';
      const d = Math.floor(a.expiresIn / 86400);
      return d >= 1 ? d + '天' : Math.max(1, Math.floor(a.expiresIn / 3600)) + '小时';
    },
    expiryBadgeCls(a) {
      if (!a.expiresAt) return 'bg-canvas-soft text-mute';
      if (a.expiresIn <= 0) return 'bg-danger/15 text-danger';
      if (this.soon(a)) return 'bg-warn/15 text-warn';
      return 'bg-canvas-soft text-mute';
    },
    async loadSettings() {
      try { this.settings = await OC.api('/api/settings'); } catch (e) {}
    },
    openAdd() { this.msg = ''; this.showAdd = true; },
    closeAdd() { this.showAdd = false; },
    openImport() { this.importText = ''; this.importName = ''; this.importError = ''; this.showImport = true; },
    closeImport() { this.showImport = false; },
    async readImportFile(e) {
      const file = e.target.files && e.target.files[0];
      if (!file) return;
      this.importName = file.name; this.importError = '';
      try { this.importText = await file.text(); } catch (err) { this.importText = ''; this.importError = '无法读取文件'; }
    },
    async exportBackup() {
      try {
        const r = await fetch('/api/accounts/export');
        if (!r.ok) throw new Error(await r.text());
        const blob = await r.blob(); const url = URL.createObjectURL(blob);
        const a = document.createElement('a'); a.href = url; a.download = 'opencode-go-monitor-backup.json'; a.click();
        URL.revokeObjectURL(url); this.flash('备份已导出');
      } catch (e) { this.flash('导出失败：' + e.message); }
    },
    async importBackup() {
      this.busy = true; this.importError = '';
      try {
        const d = await OC.api('/api/accounts/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: this.importText });
        this.closeImport(); this.flash(`导入完成：新增 ${d.added}，更新 ${d.updated}，跳过 ${d.skipped}`); await this.wait(800); this.load();
      } catch (e) { this.importError = e.message; } finally { this.busy = false; }
    },
    async addSingle() {
      if (!this.form.workspaceID && !this.form.account.includes('----')) { this.flash('请填写工作空间 ID'); return; }
      this.busy = true;
      try {
        await OC.api('/api/accounts', OC.json('POST', this.form));
        this.form = emptyForm(); this.showAdd = false; this.flash('已添加，正在后台抓取…');
        await this.wait(1200); this.load();
      } catch (e) { this.flash(e.message); } finally { this.busy = false; }
    },
    async addBulk() {
      this.busy = true;
      try {
        const d = await OC.api('/api/accounts/bulk', OC.json('POST', { text: this.bulkText }));
        this.bulkText = ''; this.showAdd = false; this.flash('已导入 ' + d.added + ' 个账号，正在后台抓取…');
        await this.wait(1500); this.load();
      } catch (e) { this.flash(e.message); } finally { this.busy = false; }
    },
    async refreshAll() {
      this.refreshing = true;
      try { await OC.api('/api/refresh', { method: 'POST' }); } catch (e) {}
      this.refreshing = false; this.load();
    },
    async refreshOne(a) {
      if (a._busy) return;
      a._busy = true; this.flash('正在刷新 ' + (a.account || a.workspaceID) + ' …');
      try { Object.assign(a, await OC.api('/api/accounts/' + a.id + '/refresh', { method: 'POST' })); this.flash('已刷新'); }
      catch (e) { this.flash('刷新失败：' + e.message); } finally { a._busy = false; }
    },
    async editField(a, field, label) {
      const v = prompt('输入新的 ' + label + '：', '');
      if (v === null) return;
      try { await OC.api('/api/accounts/' + a.id, OC.json('PUT', { [field]: v })); await this.wait(800); this.load(); this.flash('已更新'); }
      catch (e) { this.flash('更新失败：' + e.message); }
    },
    async del(a) {
      if (!confirm('删除账号 ' + (a.account || a.workspaceID) + ' ?')) return;
      try { await OC.api('/api/accounts/' + a.id, { method: 'DELETE' }); this.load(); } catch (e) { this.flash(e.message); }
    },
    usageRows(a) {
      return [
        { key: 'r', label: '滚动用量', data: a.rolling },
        { key: 'w', label: '每周用量', data: a.weekly },
        { key: 'm', label: '每月用量', data: a.monthly },
      ];
    },
    avg(key) {
      const v = this.accounts.filter(a => a.status === 'ok' && a[key]).map(a => a[key].usagePercent);
      return v.length ? Math.round(v.reduce((s, x) => s + x, 0) / v.length) : null;
    },
    tight(a) { return a.status === 'ok' && [a.rolling, a.weekly, a.monthly].some(u => u && u.usagePercent >= this.settings.warnPercent); },
    soon(a) { return a.expiresAt && a.expiresIn > 0 && a.expiresIn < this.settings.expirySoonDays * 86400; },
    health(a) {
      if (a.status === 'pending') return { label: '抓取中', cls: 'bg-canvas-soft text-mute', level: 5 };
      if (a.status === 'error') return { label: '异常', cls: 'bg-danger/15 text-danger', level: 0 };
      if (a.expiresAt && a.expiresIn <= 0) return { label: '已到期', cls: 'bg-danger/15 text-danger', level: 1 };
      if ([a.rolling, a.weekly, a.monthly].some(u => u && u.usagePercent >= 100)) return { label: '额度用尽', cls: 'bg-danger/15 text-danger', level: 1 };
      if (this.tight(a)) return { label: '额度紧张', cls: 'bg-warn/15 text-warn', level: 2 };
      if (this.soon(a)) return { label: '即将到期', cls: 'bg-warn/15 text-warn', level: 2 };
      return { label: '正常', cls: 'bg-primary/15 text-primary', level: 4 };
    },
    barCls(u) { return this.barClsN(u ? u.usagePercent : null); },
    pctText(u) { return this.pctTextN(u ? u.usagePercent : null); },
    barClsN(p) {
      if (p == null) return 'bg-hairline';
      if (p >= 100) return 'bg-danger shadow-[0_0_8px_rgba(255,92,92,.55)]';
      if (p >= this.settings.warnPercent) return 'bg-warn shadow-[0_0_8px_rgba(245,183,49,.5)]';
      return 'bg-primary shadow-[0_0_8px_rgba(0,217,146,.55)]';
    },
    pctTextN(p) { if (p == null) return 'text-mute'; if (p >= 100) return 'text-danger'; if (p >= this.settings.warnPercent) return 'text-warn'; return 'text-body'; },
    tabCls(on) { return on ? 'bg-primary text-canvas' : 'text-body hover:text-ink'; },
    flash(m) { this.msg = m; clearTimeout(this._t); this._t = setTimeout(() => this.msg = '', 4000); },
    wait(ms) { return new Promise(r => setTimeout(r, ms)); },
  },
  mounted() {
    this.loadSettings();
    this.load().then(() => {
      this.$nextTick(() => setTimeout(() => { this.barReady = true; }, 60));
    });
    setInterval(() => this.load(), 30000);
  },
}).mount('#app');

function emptyForm() { return { account: '', password: '', auxEmail: '', workspaceID: '', auth: '', apiKey: '' }; }
