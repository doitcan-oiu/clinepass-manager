// 设置页 Vue 应用：监控/转发设置 + 模型管理。
const { createApp } = Vue;

createApp({
  data() {
    return {
      s: { refreshInterval: '30m', concurrency: 6, requestTimeout: '25s', warnPercent: 80, expirySoonDays: 3, proxyKey: '', maxRetries: 2 },
      models: [], logs: [], nm: { id: '', name: '', protocol: 'openai' }, modelBusy: false,
      busy: false, msg: '',
      base: window.location.origin,
    };
  },
  methods: {
    formatLogDate(iso) { return OC.fmtDate(iso); },
    async load() {
      try { this.s = await OC.api('/api/settings'); } catch (e) { this.flash('读取设置失败'); }
      await this.loadModels(); await this.loadLogs();
    },
    async loadModels() {
      try { this.models = await OC.api('/api/models') || []; } catch (e) {}
    },
    async refreshModels() {
      this.modelBusy = true;
      try { this.models = await OC.api('/api/models/refresh', { method: 'POST' }); this.flash('模型和额度已更新'); }
      catch (e) { this.flash('刷新模型失败：' + e.message); } finally { this.modelBusy = false; }
    },
    async loadLogs() {
      try { this.logs = await OC.api('/api/logs?limit=200') || []; } catch (e) { this.flash('读取日志失败'); }
    },
    async save() {
      this.busy = true;
      try { this.s = await OC.api('/api/settings', OC.json('PUT', this.s)); this.flash('设置已保存'); }
      catch (e) { this.flash('保存失败：' + e.message); } finally { this.busy = false; }
    },
    async addModel() {
      if (!this.nm.id.trim()) { this.flash('请填写模型 ID'); return; }
      try { await OC.api('/api/models', OC.json('POST', this.nm)); this.nm = { id: '', name: '', protocol: 'openai' }; this.loadModels(); this.flash('已添加模型'); }
      catch (e) { this.flash('添加失败：' + e.message); }
    },
    async delModel(m) {
      if (!confirm('删除模型 ' + m.id + ' ?')) return;
      try { await OC.api('/api/models/' + encodeURIComponent(m.id), { method: 'DELETE' }); this.loadModels(); }
      catch (e) { this.flash('删除失败：' + e.message); }
    },
    flash(m) { this.msg = m; clearTimeout(this._t); this._t = setTimeout(() => this.msg = '', 4000); },
  },
  mounted() { this.load(); },
}).mount('#app');
