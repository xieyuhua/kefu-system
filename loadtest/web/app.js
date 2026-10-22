/* HTTP 压测台 · 前端（零依赖）：地址勘察 → 勾选接口 → 压测 → AI 分析 */
(function () {
  'use strict';

  var $ = function (id) { return document.getElementById(id); };

  var state = {
    candidates: [],
    series: [],
    logCursor: 0,
    logOn: true,
    logAuto: true,
    smoothOn: true,
    running: false,
    aiReady: false,
    hasReport: false
  };

  /* ---------------- 基础工具 ---------------- */

  function fmt(n) {
    if (n === null || n === undefined) return '0';
    if (typeof n !== 'number') return String(n);
    return n.toLocaleString('zh-CN');
  }
  function fmtMs(v) {
    if (!v) return '0 ms';
    if (v >= 1000) return (v / 1000).toFixed(2) + ' s';
    return Math.round(v) + ' ms';
  }
  function esc(s) {
    return String(s === null || s === undefined ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }
  function toast(msg, isErr) {
    var el = $('toast');
    el.textContent = msg;
    el.className = 'toast show' + (isErr ? ' err' : '');
    clearTimeout(el._t);
    el._t = setTimeout(function () { el.className = 'toast'; }, 4200);
  }
  async function api(path, opts) {
    var res = await fetch(path, opts);
    var body = await res.json().catch(function () { return null; });
    if (!res.ok) throw new Error((body && body.message) || ('请求失败 HTTP ' + res.status));
    return body ? body.data : null;
  }
  function postJSON(path, payload) {
    return api(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload || {})
    });
  }

  /* ---------------- AI 设置 ---------------- */

  function renderAIState(ai) {
    state.aiReady = !!(ai && ai.ready);
    var pill = $('aiPill');
    pill.className = 'pill' + (state.aiReady ? ' running' : '');
    $('aiText').textContent = state.aiReady ? ('AI ' + (ai.model || '已就绪')) : 'AI 未配置（用内置规则）';
    var hint = $('aiKeyHint');
    if (hint) {
      hint.textContent = ai && ai.key_mask
        ? ('当前已保存密钥：' + ai.key_mask + '（留空表示沿用）')
        : '未保存密钥；不配置也能用，只是「整理」和「分析」走内置规则。';
    }
  }

  async function loadAIConfig() {
    try {
      var ai = await api('/api/ai/config');
      renderAIState(ai);
      if (ai) {
        $('aiBase').value = ai.base_url || '';
        $('aiModel').value = ai.model || '';
      }
    } catch (e) { /* 忽略 */ }
  }

  async function saveAIConfig() {
    var payload = {
      base_url: $('aiBase').value.trim(),
      model: $('aiModel').value.trim(),
      api_key: $('aiKey').value.trim(),
      timeout_sec: Number($('aiTimeout').value) || 90
    };
    try {
      var ai = await postJSON('/api/ai/config', payload);
      renderAIState(ai);
      $('aiKey').value = '';
      $('aiDialog').close();
      toast(ai.ready ? 'AI 已接入：' + ai.model : '已保存（未填全，暂用内置规则）', !ai.ready);
    } catch (e) {
      toast(e.message, true);
    }
  }

  /* ---------------- ① 勘察 ---------------- */

  async function discover() {
    var target = $('target').value.trim();
    if (!target) { toast('请先填写目标地址', true); return; }
    var btn = $('btnDiscover');
    btn.disabled = true;
    btn.textContent = '勘察中…';
    try {
      var res = await postJSON('/api/ai/discover', {
        target: target,
        channel: $('channel').value.trim() || 'web',
        concurrency: Number($('initConc').value) || 10
      });
      state.candidates = (res.candidates || []).map(function (c) {
        c._checked = !!c.recommended;
        c._conc = c.concurrency || 5;
        c._int = c.interval_ms === undefined ? 500 : c.interval_ms;
        c._jit = c.jitter_ms || 0;
        c._path = c.path || '';
        c._body = c.body || '';
        c._setup = c.setup || 'none';
        c._auth = c.auth || 'none';
        return c;
      });
      renderFacts(res);
      if (res.page) {
        // 把你贴的页面地址归一化成接口基址，渠道也回填
        if (res.page.base_url) $('target').value = res.page.base_url;
        if (res.page.channel) $('channel').value = res.page.channel;
      }
      renderCandidates();
      toast('勘察完成：' + state.candidates.length + ' 个候选接口（' + (res.source === 'ai' ? 'AI 整理' : '内置规则') + '）');
      if (res.error) toast(res.error, true);
    } catch (e) {
      toast(e.message, true);
    } finally {
      btn.disabled = false;
      btn.textContent = '让 AI 分析接口';
    }
  }

  function renderFacts(res) {
    $('factsWrap').style.display = '';
    $('srcTag').textContent = res.source === 'ai' ? ('AI 整理 · ' + (res.model || '')) : '内置规则整理';
    $('discoverTime').textContent = '耗时 ' + Math.round(res.elapsed_ms || 0) + 'ms';
    var box = $('facts');
    box.innerHTML = '';
    (res.facts || []).forEach(function (f) {
      var div = document.createElement('div');
      div.className = 'fact ' + (f.level || 'info');
      div.innerHTML = '<b>' + esc(f.kind) + '</b> ' + esc(f.detail);
      box.appendChild(div);
    });
    (res.hints || []).forEach(function (h) {
      var div = document.createElement('div');
      div.className = 'fact hint-fact';
      div.textContent = '提示：' + h;
      box.appendChild(div);
    });
  }

  /* ---------------- ② 勾选接口 ---------------- */

  function riskClass(risk) {
    switch (risk) {
      case '只读': return 'tag-read';
      case '写入': return 'tag-write';
      case '危险': return 'tag-danger';
      default: return 'tag-unknown';
    }
  }

  function renderCandidates() {
    var list = state.candidates;
    $('candEmpty').style.display = list.length ? 'none' : '';
    $('candWrap').style.display = list.length ? '' : 'none';
    var body = $('candBody');
    body.innerHTML = '';
    list.forEach(function (c, i) {
      var tr = document.createElement('tr');
      tr.innerHTML =
        '<td><input type="checkbox" class="sel" data-i="' + i + '"' + (c._checked ? ' checked' : '') + '></td>' +
        '<td>' +
          '<div class="cand-name">' + esc(c.name) + ' <span class="tag ' + riskClass(c.risk) + '">' + esc(c.risk || '未知') + '</span></div>' +
          '<div class="cand-path">' + esc(c.method) + ' ' + esc(c._path) + '</div>' +
          (c.reason || c.note ? '<div class="cand-note">' + esc(c.reason || '') + (c.note ? '　' + esc(c.note) : '') + '</div>' : '') +
          '<details class="cand-more"><summary>参数</summary>' +
            '<label class="field"><span>路径</span><input type="text" data-i="' + i + '" data-k="path" value="' + esc(c._path) + '"></label>' +
            '<label class="field"><span>请求体（JSON 模板，可空）</span><textarea rows="2" data-i="' + i + '" data-k="body">' + esc(c._body) + '</textarea></label>' +
            '<label class="mini"><input type="checkbox" data-i="' + i + '" data-k="setup"' + (c._setup === 'guest_init' ? ' checked' : '') + '> 每条并发先调用访客初始化（/api/guest/init）</label>' +
            '<label class="mini" style="margin-left:12px"><input type="checkbox" data-i="' + i + '" data-k="auth"' + (c._auth === 'bearer' ? ' checked' : '') + '> 需要登录 Token</label>' +
          '</details>' +
        '</td>' +
        '<td><input class="mini-num" type="number" min="1" max="2000" data-i="' + i + '" data-k="conc" value="' + c._conc + '"></td>' +
        '<td><input class="mini-num" type="number" min="0" step="50" data-i="' + i + '" data-k="int" value="' + c._int + '"></td>' +
        '<td><button class="tiny" data-act="probe" data-i="' + i + '" title="先单独跑一次，确认这个接口能通过">试跑</button></td>';
      body.appendChild(tr);
    });
    updateSelection();
  }

  function updateSelection() {
    var picked = state.candidates.filter(function (c) { return c._checked; });
    var conc = picked.reduce(function (a, c) { return a + (Number(c._conc) || 0); }, 0);
    $('selHint').textContent = '已选 ' + picked.length + ' 个接口 · 总并发 ' + conc;
    $('btnStart').disabled = !picked.length || state.running;
  }

  async function probeOne(i) {
    var c = state.candidates[i];
    if (!c) return;
    toast('正在试跑 ' + c.method + ' ' + c._path + ' …');
    try {
      var res = await postJSON('/api/probe/one', {
        target: $('target').value.trim(),
        channel: $('channel').value.trim() || 'web',
        token: $('token').value.trim(),
        timeout_ms: Number($('timeout').value) || 15000,
        scenario: {
          method: c.method, path: c._path, body: c._body,
          setup: c._setup, auth: c._auth, headers: c.headers || null
        }
      });
      toast(res.verdict, !res.ok);
    } catch (e) {
      toast(e.message, true);
    }
  }

  /* ---------------- 压测控制 ---------------- */

  function collectScenarios() {
    var out = [];
    state.candidates.forEach(function (c) {
      if (!c._checked) return;
      out.push({
        id: c.id,
        name: c.name,
        method: c.method,
        path: c._path,
        body: c._body,
        setup: c._setup,
        auth: c._auth,
        headers: c.headers || null,
        concurrency: Number(c._conc) || 1,
        interval_ms: Number(c._int) || 0,
        jitter_ms: Number(c._jit) || 0,
        per_user_messages: 0,
        risk: c.risk,
        note: c.note
      });
    });
    return out;
  }

  async function start() {
    var scenarios = collectScenarios();
    if (!scenarios.length) { toast('请先勾选要压的接口', true); return; }
    var cfg = {
      target: $('target').value.trim(),
      channel: $('channel').value.trim() || 'web',
      token: $('token').value.trim(),
      duration_sec: Number($('duration').value) || 0,
      ramp_up_ms: Number($('rampup').value) || 0,
      timeout_ms: Number($('timeout').value) || 15000,
      keep_alive: $('keepAlive').checked,
      uid_mode: 'per-user',
      uid_prefix: $('uidPrefix').value.trim() || 'lt_',
      nickname_prefix: '压测用户',
      use_web_socket: $('useWs').checked,
      scenarios: scenarios
    };
    $('btnStart').disabled = true;
    $('btnStart').textContent = '启动中…';
    try {
      await postJSON('/api/run', cfg);
      $('btnJson').disabled = true;
      $('btnCsv').disabled = true;
      $('btnMd').disabled = true;
      $('btnAdvise').disabled = true;
      $('adviceSrc').textContent = '未生成';
      $('adviceBox').innerHTML = '<div class="empty">压测结束后点「分析这轮结果」。</div>';
      toast('压测已启动：' + scenarios.length + ' 个接口，总并发 ' +
        scenarios.reduce(function (a, s) { return a + s.concurrency; }, 0));
      pollNow();
    } catch (e) {
      toast(e.message, true);
      $('btnStart').disabled = false;
    } finally {
      $('btnStart').textContent = '开始压测';
    }
  }

  async function stop() {
    $('btnStop').disabled = true;
    $('btnStop').textContent = '停止中…';
    try {
      await postJSON('/api/stop');
      toast('已停止，正在生成报告…');
    } catch (e) {
      toast(e.message, true);
    } finally {
      $('btnStop').textContent = '停止';
    }
  }

  /* ---------------- ③ 看板 ---------------- */

  function render(s) {
    var sum = s.summary || {};
    var running = !!s.running;
    state.running = running;
    state.hasReport = !!s.has_report;

    var pill = $('statusPill');
    pill.className = 'pill' + (running ? ' running' : (s.has_report ? ' done' : ''));
    $('statusText').textContent = running ? '压测中' : (s.has_report ? '已完成' : '空闲');

    $('btnStart').disabled = running;
    $('btnStop').disabled = !running;

    $('mSent').textContent = fmt(sum.sent);
    $('mOk').textContent = fmt(sum.ok);
    $('mFailed').textContent = fmt(sum.failed);
    $('mFailRate').innerHTML = ((sum.fail_rate || 0) * 100).toFixed(2) + '<small>%</small>';
    $('mRps').textContent = Math.round(sum.current_rps || 0);
    $('mAvgRps').textContent = (sum.avg_rps || 0).toFixed(1);

    var lat = sum.latency || {};
    $('lAvg').textContent = fmtMs(lat.avg_ms);
    $('lP50').textContent = fmtMs(lat.p50_ms);
    $('lP90').textContent = fmtMs(lat.p90_ms);
    $('lP95').textContent = fmtMs(lat.p95_ms);
    $('lP99').textContent = fmtMs(lat.p99_ms);
    $('lMax').textContent = fmtMs(lat.max_ms);

    $('bar').style.width = (Math.max(0, Math.min(1, s.progress || 0)) * 100).toFixed(1) + '%';

    var bits = ['已运行 ' + (sum.elapsed_sec || 0).toFixed(0) + ' 秒'];
    if (sum.total_concurrency) bits.push('总并发 ' + sum.total_concurrency);
    if (sum.init_ok || sum.init_failed) bits.push('会话初始化 ' + fmt(sum.init_ok) + '/' + fmt(sum.init_failed));
    if (sum.ws_connects || sum.ws_live) bits.push('长连接 在线 ' + fmt(sum.ws_live) + ' · 断线 ' + fmt(sum.ws_disconnects));
    if (sum.inflight) bits.push((running ? '在途 ' : '结束仍在途 ') + fmt(sum.inflight) + ' 条');
    $('progressHint').textContent = bits.join('　·　');

    if (s.config && s.config.target) {
      $('targetHint').textContent = '目标 ' + s.config.target + '　接口 ' + (s.config.scenarios ? s.config.scenarios.length : 0) + ' 个';
    }

    renderScenarios(sum.scenarios || []);
    renderBanner(sum);
    renderErrors(sum.errors || [], sum.samples || [], sum.ok + sum.failed);

    var series = smoothSeries(sum.series || [], state.smoothOn);
    drawChart($('chartTps'), series, [
      { key: 'sent', color: '#4f8cff', halo: true },
      { key: 'ok', color: '#22c98b' },
      { key: 'fail', color: '#f2555a' }
    ], { unit: '条 / 秒', isLatency: false });
    drawChart($('chartLat'), series, [{ key: 'avg_ms', color: '#f5a524' }], { unit: 'ms', isLatency: true });

    if (!running && s.has_report) {
      $('btnJson').disabled = false;
      $('btnCsv').disabled = false;
      $('btnAdvise').disabled = false;
      if (s.last_report_files && s.last_report_files.length) {
        $('reportHint').textContent = '报告已保存：' + s.last_report_files.join('　');
      }
      if (s.verdict && !running) {
        $('bar').style.width = '100%';
      }
    }
    if (s.ai) renderAIState(s.ai);
  }

  function renderScenarios(scs) {
    var card = $('cardScen');
    if (!scs.length) { card.style.display = 'none'; return; }
    card.style.display = '';
    var body = $('scenBody');
    body.innerHTML = '';
    scs.forEach(function (sc) {
      var tr = document.createElement('tr');
      var failRate = ((sc.fail_rate || 0) * 100).toFixed(1) + '%';
      tr.innerHTML =
        '<td><div class="cand-name">' + esc(sc.name) + '</div>' +
        '<div class="cand-path">' + esc(sc.method) + ' ' + esc(sc.path) + '</div></td>' +
        '<td class="num">' + sc.concurrency + '</td>' +
        '<td class="num">' + fmt(sc.sent) + '</td>' +
        '<td class="num">' + failRate + '</td>' +
        '<td class="num">' + fmtMs(sc.latency ? sc.latency.avg_ms : 0) + '</td>' +
        '<td class="num">' + fmtMs(sc.latency ? sc.latency.p95_ms : 0) + '</td>';
      body.appendChild(tr);
    });
  }

  function renderBanner(sum) {
    var banner = $('failBanner');
    var failed = sum.failed || 0;
    if (!failed) { banner.style.display = 'none'; return; }
    var top = (sum.errors && sum.errors[0]) || null;
    var rate = (sum.fail_rate || 0) * 100;
    var html = '<b>已有 ' + fmt(failed) + ' 条失败（失败率 ' + rate.toFixed(1) + '%）</b>';
    if (top) html += '<br>最常见原因：<span class="mono">[' + esc(top.kind) + '] ' + esc(top.detail) + '</span>';
    if (rate >= 99) html += '<br>几乎全部失败：先点对应接口的「测这一个」确认路径/参数/鉴权，再看实时日志。';
    banner.innerHTML = html;
    banner.style.display = '';
  }

  function renderErrors(errors, samples, done) {
    var tb = $('errBody');
    if (!errors.length) {
      tb.innerHTML = '<tr><td colspan="3" class="empty">暂无错误</td></tr>';
    } else {
      var html = '';
      errors.forEach(function (e) {
        var pct = done > 0 ? (e.count / done * 100).toFixed(1) + '%' : '—';
        html += '<tr><td><b>' + esc(e.kind) + '</b><div class="mono">' + esc(e.detail) + '</div></td>' +
          '<td class="num">' + fmt(e.count) + '</td><td class="num">' + pct + '</td></tr>';
      });
      tb.innerHTML = html;
    }
    var sb = $('sampleBody');
    if (!samples.length) {
      sb.innerHTML = '<tr><td colspan="4" class="empty">暂无错误</td></tr>';
    } else {
      var h = '';
      samples.slice(0, 20).forEach(function (s) {
        h += '<tr><td>' + esc(s.at) + '</td><td class="num">' + s.vu + '</td><td>' + esc(s.kind) +
          '</td><td class="mono">' + esc(s.detail) + '</td></tr>';
      });
      sb.innerHTML = h;
    }
  }

  /* ---------------- 日志 ---------------- */

  async function pollLogs() {
    if (!state.logOn) return;
    try {
      var data = await api('/api/logs?since=' + state.logCursor);
      if (!data) return;
      if (data.entries && data.entries.length) appendLogs(data.entries);
      state.logCursor = data.cursor;
    } catch (e) { /* 忽略 */ }
  }

  function appendLogs(entries) {
    var box = $('logBox');
    var frag = document.createDocumentFragment();
    entries.forEach(function (e) {
      var line = document.createElement('div');
      line.className = 'log-line ' + (e.level || 'info');
      var t = document.createElement('span');
      t.className = 't';
      t.textContent = e.at;
      var m = document.createElement('span');
      m.className = 'm';
      m.textContent = e.msg;
      line.appendChild(t);
      line.appendChild(m);
      frag.appendChild(line);
    });
    box.appendChild(frag);
    while (box.childElementCount > 900) box.removeChild(box.firstElementChild);
    if (state.logAuto) box.scrollTop = box.scrollHeight;
  }

  /* ---------------- ④ AI 分析 ---------------- */

  async function requestAdvice() {
    var btn = $('btnAdvise');
    btn.disabled = true;
    btn.textContent = '分析中…';
    $('adviceBox').innerHTML = '<div class="empty">正在结合指标生成瓶颈定位与整改方案…</div>';
    try {
      var res = await postJSON('/api/ai/advise');
      $('adviceSrc').textContent = res.source === 'ai' ? ('AI · ' + (res.model || '')) : '内置规则';
      $('adviceBox').innerHTML = mdToHtml(res.text || '');
      $('btnMd').disabled = false;
      if (res.error) toast(res.error, true);
      else toast('分析完成（' + (res.source === 'ai' ? '大模型' : '内置规则') + '）');
    } catch (e) {
      $('adviceBox').innerHTML = '<div class="empty">分析失败：' + esc(e.message) + '</div>';
      toast(e.message, true);
    } finally {
      btn.disabled = false;
      btn.textContent = '分析这轮结果';
    }
  }

  function inlineMd(s) {
    return esc(s)
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>');
  }

  function mdTable(rows) {
    var out = '<table class="md-table">';
    rows.forEach(function (row, idx) {
      if (/^\|[\s:\-|]+\|$/.test(row)) return;
      var cells = row.split('|').slice(1, -1).map(function (c) { return c.trim(); });
      if (!cells.length) return;
      var tag = idx === 0 ? 'th' : 'td';
      out += '<tr>' + cells.map(function (c) { return '<' + tag + '>' + inlineMd(c) + '</' + tag + '>'; }).join('') + '</tr>';
    });
    return out + '</table>';
  }

  function mdToHtml(md) {
    var lines = String(md || '').split('\n');
    var html = '', inList = false, tableRows = [];
    function closeList() { if (inList) { html += '</ul>'; inList = false; } }
    function flushTable() { if (tableRows.length) { html += mdTable(tableRows); tableRows = []; } }
    lines.forEach(function (raw) {
      var t = raw.replace(/\s+$/, '').trim();
      if (!t) { closeList(); flushTable(); return; }
      if (t.charAt(0) === '|') { tableRows.push(t); return; }
      flushTable();
      var h = t.match(/^(#{1,4})\s+(.*)$/);
      if (h) {
        closeList();
        var lv = Math.min(5, h[1].length + 2);
        html += '<h' + lv + '>' + inlineMd(h[2]) + '</h' + lv + '>';
        return;
      }
      var li = t.match(/^[-*]\s+(.*)$/) || t.match(/^\d+[.)]\s+(.*)$/);
      if (li) {
        if (!inList) { html += '<ul>'; inList = true; }
        html += '<li>' + inlineMd(li[1]) + '</li>';
        return;
      }
      closeList();
      html += '<p>' + inlineMd(t) + '</p>';
    });
    closeList();
    flushTable();
    return html || '<div class="empty">（空）</div>';
  }

  /* ---------------- 画图 ---------------- */

  function smoothSeries(series, on) {
    if (!on || series.length < 3) return series;
    var half = 2;
    return series.map(function (p, i) {
      var from = Math.max(0, i - half), to = Math.min(series.length - 1, i + half);
      var n = 0, sSent = 0, sOk = 0, sFail = 0, sLat = 0, latN = 0;
      for (var j = from; j <= to; j++) {
        sSent += Number(series[j].sent) || 0;
        sOk += Number(series[j].ok) || 0;
        sFail += Number(series[j].fail) || 0;
        var v = Number(series[j].avg_ms) || 0;
        if (v > 0) { sLat += v; latN++; }
        n++;
      }
      return { second: p.second, sent: sSent / n, ok: sOk / n, fail: sFail / n, avg_ms: latN ? sLat / latN : 0 };
    });
  }

  function niceMax(v) {
    if (v <= 0) return 1;
    var exp = Math.pow(10, Math.floor(Math.log10(v)));
    var f = v / exp;
    var m = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10;
    return m * exp;
  }

  function drawChart(canvas, series, defs, opts) {
    if (!canvas) return;
    var dpr = window.devicePixelRatio || 1;
    var w = canvas.clientWidth || canvas.parentElement.clientWidth || 600;
    var h = parseInt(canvas.getAttribute('height'), 10) || 150;
    if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
    }
    var ctx = canvas.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    var padL = 46, padR = 12, padT = 10, padB = 22;
    var iw = Math.max(10, w - padL - padR);
    var ih = Math.max(10, h - padT - padB);

    ctx.font = '11px "Segoe UI", system-ui, sans-serif';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';

    var data = series.slice(-120);
    var maxV = 0;
    data.forEach(function (p) {
      defs.forEach(function (d) { maxV = Math.max(maxV, Number(p[d.key]) || 0); });
    });
    maxV = niceMax(maxV || 1);

    var lines = 4;
    ctx.strokeStyle = '#232936';
    ctx.lineWidth = 1;
    for (var i = 0; i <= lines; i++) {
      var y = padT + ih * i / lines;
      ctx.beginPath();
      ctx.moveTo(padL, y);
      ctx.lineTo(padL + iw, y);
      ctx.stroke();
      ctx.fillStyle = '#7e879a';
      ctx.fillText(Math.round(maxV * (lines - i) / lines) + '', padL - 8, y);
    }

    if (!data.length) {
      ctx.fillStyle = '#5f6779';
      ctx.textAlign = 'center';
      ctx.fillText('等待数据…', padL + iw / 2, padT + ih / 2);
      return;
    }

    ctx.textAlign = 'center';
    ctx.textBaseline = 'top';
    var step = Math.max(1, Math.ceil(data.length / 8));
    for (var j = 0; j < data.length; j += step) {
      var x = padL + (data.length === 1 ? iw / 2 : iw * j / (data.length - 1));
      ctx.fillStyle = '#5f6779';
      ctx.fillText(data[j].second + 's', x, padT + ih + 6);
    }

    defs.forEach(function (d) {
      ctx.beginPath();
      ctx.strokeStyle = d.color;
      ctx.lineWidth = d.halo ? 8 : 2;
      ctx.globalAlpha = d.halo ? 0.22 : 1;
      ctx.lineJoin = 'round';
      data.forEach(function (p, idx) {
        var xx = padL + (data.length === 1 ? iw / 2 : iw * idx / (data.length - 1));
        var v = Number(p[d.key]) || 0;
        var yy = padT + ih - ih * (v / maxV);
        if (idx === 0) ctx.moveTo(xx, yy); else ctx.lineTo(xx, yy);
      });
      ctx.stroke();
      ctx.globalAlpha = 1;

      if (defs.length === 1) {
        ctx.lineTo(padL + iw, padT + ih);
        ctx.lineTo(padL, padT + ih);
        ctx.closePath();
        var grad = ctx.createLinearGradient(0, padT, 0, padT + ih);
        grad.addColorStop(0, d.color + '44');
        grad.addColorStop(1, d.color + '00');
        ctx.fillStyle = grad;
        ctx.fill();
      }
    });

    ctx.fillStyle = '#5f6779';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'top';
    ctx.fillText(opts.unit, w - 4, 2);
  }

  /* ---------------- 轮询 ---------------- */

  async function pollNow() {
    try {
      render(await api('/api/status'));
    } catch (e) {
      $('statusText').textContent = '控制台连接中断';
    }
  }

  function poll() {
    setInterval(function () { pollNow(); pollLogs(); }, 1000);
    pollNow();
    pollLogs();
  }

  /* ---------------- 事件绑定 ---------------- */

  function bind() {
    $('btnAISetup').addEventListener('click', function () {
      loadAIConfig();
      $('aiDialog').showModal();
    });
    $('btnAISave').addEventListener('click', function (ev) {
      ev.preventDefault();
      saveAIConfig();
    });

    $('btnDiscover').addEventListener('click', discover);
    $('btnStart').addEventListener('click', start);
    $('btnStop').addEventListener('click', stop);
    $('btnAdvise').addEventListener('click', requestAdvice);

    $('btnJson').addEventListener('click', function () { window.location.href = '/api/report'; });
    $('btnCsv').addEventListener('click', function () { window.location.href = '/api/report.csv'; });
    $('btnMd').addEventListener('click', function () { window.location.href = '/api/advice.md'; });

    $('btnSelRec').addEventListener('click', function () {
      state.candidates.forEach(function (c) { c._checked = !!c.recommended; });
      renderCandidates();
    });
    $('btnSelNone').addEventListener('click', function () {
      state.candidates.forEach(function (c) { c._checked = false; });
      renderCandidates();
    });

    // 候选接口表格：勾选 / 参数编辑 / 试跑
    $('candBody').addEventListener('change', function (ev) {
      var el = ev.target;
      var i = Number(el.dataset.i);
      var c = state.candidates[i];
      if (!c) return;
      if (el.classList.contains('sel')) {
        c._checked = el.checked;
        updateSelection();
        return;
      }
      switch (el.dataset.k) {
        case 'conc': c._conc = Number(el.value) || 1; break;
        case 'int': c._int = Number(el.value) || 0; break;
        case 'path': c._path = el.value.trim(); break;
        case 'body': c._body = el.value; break;
        case 'setup': c._setup = el.checked ? 'guest_init' : 'none'; break;
        case 'auth': c._auth = el.checked ? 'bearer' : 'none'; break;
      }
      updateSelection();
    });
    $('candBody').addEventListener('input', function (ev) {
      var el = ev.target;
      if (el.dataset.k === 'conc' || el.dataset.k === 'int' || el.dataset.k === 'path' || el.dataset.k === 'body') {
        var c = state.candidates[Number(el.dataset.i)];
        if (!c) return;
        if (el.dataset.k === 'conc') c._conc = Number(el.value) || 1;
        if (el.dataset.k === 'int') c._int = Number(el.value) || 0;
        if (el.dataset.k === 'path') c._path = el.value.trim();
        if (el.dataset.k === 'body') c._body = el.value;
        updateSelection();
      }
    });
    $('candBody').addEventListener('click', function (ev) {
      var btn = ev.target.closest('button[data-act="probe"]');
      if (btn) probeOne(Number(btn.dataset.i));
    });

    $('logToggle').addEventListener('change', function () {
      state.logOn = this.checked;
      $('logBox').classList.toggle('off', !state.logOn);
      if (state.logOn) pollLogs();
    });
    $('logAuto').addEventListener('change', function () { state.logAuto = this.checked; });
    $('logClear').addEventListener('click', function () { $('logBox').innerHTML = ''; });
    $('chartSmooth').addEventListener('change', function () {
      state.smoothOn = this.checked;
      pollNow();
    });

    window.addEventListener('resize', function () { pollNow(); });
  }

  document.addEventListener('DOMContentLoaded', function () {
    bind();
    loadAIConfig();
    poll();

    // 支持用链接直达：?target=...&auto=1 自动勘察，&advice=1 自动分析最近一轮
    var params = new URLSearchParams(location.search);
    if (params.get('target')) {
      $('target').value = params.get('target');
    }
    if (params.get('channel')) {
      $('channel').value = params.get('channel');
    }
    if (params.get('auto') === '1') {
      discover();
    }
    if (params.get('advice') === '1') {
      setTimeout(function () { requestAdvice(); }, 1200);
    }
  });
})();
