/*
 * uitest 共享运行时（唯一实现，插件与服务端执行器共用同一份文件，保证定位逻辑一致）
 * 载体：window.__uitestKeys
 *
 * 设计要点
 * 1) 稳定 ID 绑定：给元素算一个"确定性"的 data-uitest-id = ut_<fnv1a32(语义指纹)>。
 *    - 语义指纹只取 tag/type/id/name/aria/placeholder/label 这类"业务上不太变"的信息，
 *      不含 DOM 层级、不含随机数、不含生成时间；
 *    - 页面改版只要语义不变，key 就不变；页面刷新/key 重算结果一致；
 *    - 插件录制时算一次，服务端执行时用同一算法重算，uid 定位永远命中。
 * 2) 多级兜底：uid -> id -> name -> css -> xpath -> label/text/placeholder。
 * 3) 自愈定位：全部兜底失败时，用录制时保存的元素指纹（tag/label/placeholder/文本/aria）
 *    在页面上做相似度打分，唯一最高分且 >= 阈值才算命中，并标记 healed=true 供回写。
 */
(function () {
  'use strict';
  if (window.__uitestKeys && window.__uitestKeys.version === '1.5') return;

  var KEY_ATTR = 'data-uitest-id';
  var VERSION = '1.5';

  /* ---------------- 基础工具 ---------------- */

  function norm(s) {
    if (s === null || s === undefined) return '';
    return String(s).replace(/[\s\u00a0]+/g, ' ').trim();
  }

  function fnv1a32(str) {
    var h = 0x811c9dc5;
    for (var i = 0; i < str.length; i++) {
      h ^= str.charCodeAt(i);
      h += (h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24);
      h = h >>> 0;
    }
    return ('0000000' + h.toString(16)).slice(-8);
  }

  function sleep(ms) {
    return new Promise(function (r) { setTimeout(r, ms); });
  }

  function isVisible(el) {
    if (!el || !el.getBoundingClientRect) return false;
    var st;
    try { st = window.getComputedStyle(el); } catch (e) { return false; }
    if (!st || st.display === 'none' || st.visibility === 'hidden' || st.opacity === '0') return false;
    var r = el.getBoundingClientRect();
    if (r.width <= 0 && r.height <= 0) return false;
    return true;
  }

  /* ---------------- 语义提取 ---------------- */

  var LABEL_CLASS_RE = /(label|form-item|field|caption|title|th)/i;

  // 取元素对应的业务标签文字。
  // 按钮/链接类：直接用自身可见文本（这既是"名字"，也是稳定 key 的语义来源，
  //   避免向上找到表头 "ID" 这类无关文字导致定位键漂移）；
  // 表单控件类：aria-label > label[for] > 祖先 label > 就近表单项标题 > placeholder。
  function labelOf(el) {
    if (!el) return '';
    var v = norm(el.getAttribute && el.getAttribute('aria-label'));
    if (v) return v;
    var tag = (el.tagName || '').toLowerCase();
    var role = norm(el.getAttribute && el.getAttribute('role'));
    if (tag === 'button' || tag === 'a' || tag === 'summary' || role === 'button' || role === 'link' || role === 'tab') {
      var own = visibleText(el);
      if (own) return own.length > 40 ? own.slice(0, 40) : own;
    }
    if (el.id) {
      var lab = document.querySelector('label[for="' + cssEscape(el.id) + '"]');
      if (lab) { v = norm(lab.textContent); if (v) return v; }
    }
    var p = el.closest ? el.closest('label') : null;
    if (p) { v = norm(p.textContent); if (v) return v; }

    // 向上最多 3 层找表单项标题（Element Plus / Ant Design / 原生 fieldset 都覆盖）
    var node = el.parentElement, depth = 0;
    while (node && depth < 3) {
      var cands = node.querySelectorAll('label, .el-form-item__label, .ant-form-item-label, [class*="label"], legend, th');
      for (var i = 0; i < cands.length; i++) {
        if (cands[i].contains(el)) continue;
        if (LABEL_CLASS_RE.test(cands[i].className || '') || cands[i].tagName === 'LABEL' ||
            cands[i].tagName === 'LEGEND' || cands[i].tagName === 'TH') {
          v = norm(cands[i].textContent);
          if (v && v.length <= 40) return v;
        }
      }
      node = node.parentElement; depth++;
    }
    // 上一个兄弟节点的文本（"商品名称：" 这种纯文本标题）
    var prev = el.previousElementSibling, hop = 0;
    while (prev && hop < 2) {
      v = norm(prev.textContent);
      if (v && v.length <= 40) return v.replace(/[:：]\s*$/, '');
      prev = prev.previousElementSibling; hop++;
    }
    v = norm(el.getAttribute && el.getAttribute('placeholder'));
    return v ? v.replace(/^请输入|^请选择/, '') : '';
  }

  var KIND_RULES = [
    [/价格|金额|单价|售价|费用|price|amount|money|fee/i, 'price'],
    [/库存|数量|件数|次数|人数|stock|count|qty|quantity|num/i, 'number'],
    [/手机|电话|联系方式|mobile|phone|tel/i, 'phone'],
    [/邮箱|邮件|email|mail/i, 'email'],
    [/网址|链接|地址|url|link|href/i, 'url'],
    [/时间|日期|开始|结束|date|time/i, 'datetime'],
    [/图片|照片|封面|图标|上传|附件|image|photo|cover|upload|file/i, 'file'],
    [/描述|详情|介绍|备注|内容|说明|desc|remark|content|note/i, 'textarea'],
    [/密码|password|pwd/i, 'password']
  ];

  function fieldKind(el, label) {
    var tag = (el.tagName || '').toLowerCase();
    var type = (el.getAttribute && el.getAttribute('type') || '').toLowerCase();
    if (tag === 'select') return 'select';
    if (tag === 'textarea') return 'textarea';
    if (type === 'checkbox' || type === 'radio') return 'checkbox';
    if (type === 'file') return 'file';
    if (type === 'date' || type === 'datetime-local' || type === 'month' || type === 'time') return 'date';
    if (type === 'number') return 'number';
    if (type === 'password') return 'password';
    if (type === 'email') return 'email';
    var hay = norm(label + ' ' + (el.getAttribute('name') || '') + ' ' + (el.getAttribute('placeholder') || '') + ' ' + (el.id || ''));
    for (var i = 0; i < KIND_RULES.length; i++) {
      if (KIND_RULES[i][0].test(hay)) return KIND_RULES[i][1];
    }
    return 'text';
  }

  function visibleText(el) {
    if (!el) return '';
    var t = norm(el.innerText || el.textContent || '');
    return t.length > 120 ? t.slice(0, 120) : t;
  }

  // 元素指纹：录制时存进 step.target，执行时用于自愈打分
  function describe(el) {
    var label = labelOf(el);
    var options = [];
    if ((el.tagName || '').toLowerCase() === 'select' && el.options) {
      for (var i = 0; i < el.options.length && options.length < 20; i++) {
        var t = norm(el.options[i].textContent);
        if (t && !/^请选择|^请输/.test(t)) options.push(t);
      }
    }
    return {
      tag: (el.tagName || '').toLowerCase(),
      type: (el.getAttribute && el.getAttribute('type') || '').toLowerCase(),
      label: label,
      text: visibleText(el),
      placeholder: norm(el.getAttribute && el.getAttribute('placeholder')),
      aria: norm(el.getAttribute && el.getAttribute('aria-label')),
      role: norm(el.getAttribute && el.getAttribute('role')),
      href: (el.getAttribute && el.getAttribute('href')) || '',
      page: location.pathname + location.hash,
      field_kind: fieldKind(el, label),
      options: options
    };
  }

  /* ---------------- CSS / XPath ---------------- */

  function cssEscape(s) {
    if (window.CSS && CSS.escape) return CSS.escape(s);
    return String(s).replace(/["\\]/g, '\\$&');
  }

  function cssPath(el) {
    if (!el || el.nodeType !== 1) return '';
    var parts = [];
    var node = el;
    while (node && node.nodeType === 1 && node !== document.documentElement) {
      if (node.id) { parts.unshift('#' + cssEscape(node.id)); break; }
      var tag = node.tagName.toLowerCase();
      var idx = 1, sib = node;
      while ((sib = sib.previousElementSibling)) {
        if (sib.tagName === node.tagName) idx++;
      }
      parts.unshift(tag + ':nth-of-type(' + idx + ')');
      node = node.parentElement;
    }
    return parts.length ? parts.join(' > ') : '';
  }

  function uniqueSelector(el) {
    if (!el) return '';
    if (el.id && document.querySelectorAll('#' + cssEscape(el.id)).length === 1) return '#' + cssEscape(el.id);
    var nm = el.getAttribute && el.getAttribute('name');
    if (nm) {
      var sel = el.tagName.toLowerCase() + '[name="' + nm.replace(/"/g, '\\"') + '"]';
      if (document.querySelectorAll(sel).length === 1) return sel;
    }
    return cssPath(el);
  }

  /* ---------------- 稳定 key 计算 ---------------- */

  function computeKey(el) {
    var tag = (el.tagName || '').toLowerCase();
    var sig = [
      tag,
      (el.getAttribute && el.getAttribute('type') || '').toLowerCase(),
      el.id || '',
      (el.getAttribute && el.getAttribute('name')) || '',
      norm(el.getAttribute && el.getAttribute('aria-label')),
      norm(el.getAttribute && el.getAttribute('placeholder')),
      labelOf(el)
    ].filter(function (x) { return !!x; }).join('|');
    var basis = sig.length > tag.length + 1 ? sig : cssPath(el);
    return 'ut_' + fnv1a32(basis);
  }

  var CANDIDATE_SEL = [
    'a', 'button', 'input', 'select', 'textarea', 'label', 'summary',
    '[role="button"]', '[role="link"]', '[role="tab"]', '[role="menuitem"]',
    '[onclick]', '[contenteditable="true"]', '[tabindex]:not([tabindex="-1"])'
  ].join(',');

  // 给页面上所有可交互元素补齐 data-uitest-id（幂等）
  function markAll(root) {
    var scope = root || document;
    var list = scope.querySelectorAll(CANDIDATE_SEL);
    var n = 0;
    for (var i = 0; i < list.length; i++) {
      var el = list[i];
      if (el.hasAttribute(KEY_ATTR)) continue;
      var k = computeKey(el);
      el.setAttribute(KEY_ATTR, k);
      n++;
    }
    return n;
  }

  // 从事件目标往上找"真正该点的那个元素"
  function clickTargetOf(el) {
    var node = el, hop = 0;
    while (node && node !== document.body && hop < 5) {
      if (node.matches && node.matches(CANDIDATE_SEL)) return node;
      node = node.parentElement; hop++;
    }
    return el;
  }

  /* ---------------- 定位 ---------------- */

  function byStrategy(strategy, value) {
    var v = value;
    if (v === undefined || v === null || v === '') return null;
    try {
      switch (strategy) {
        case 'uid': {
          var el = document.querySelector('[' + KEY_ATTR + '="' + String(v).replace(/"/g, '\\"') + '"]');
          return el || null;
        }
        case 'id': {
          var e1 = document.getElementById(v);
          if (e1) return e1;
          return document.querySelector('[id="' + String(v).replace(/"/g, '\\"') + '"]');
        }
        case 'name':
          return document.querySelector('[name="' + String(v).replace(/"/g, '\\"') + '"]');
        case 'placeholder':
          return document.querySelector('[placeholder="' + String(v).replace(/"/g, '\\"') + '"]');
        case 'css':
          return document.querySelector(v);
        case 'xpath': {
          var r = document.evaluate(v, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null);
          return r.singleNodeValue || null;
        }
        case 'text': {
          var want = norm(v);
          var cands = document.querySelectorAll(CANDIDATE_SEL);
          var exact = null, fuzzy = null;
          for (var i = 0; i < cands.length; i++) {
            var t = visibleText(cands[i]);
            if (!t) continue;
            if (t === want) { if (!exact && isVisible(cands[i])) exact = cands[i]; }
            else if (!fuzzy && t.indexOf(want) >= 0 && isVisible(cands[i])) fuzzy = cands[i];
          }
          return exact || fuzzy;
        }
        case 'label': {
          var wl = norm(v);
          var labels = document.querySelectorAll('label, .el-form-item__label, .ant-form-item-label, legend, th');
          for (var j = 0; j < labels.length; j++) {
            if (norm(labels[j].textContent).indexOf(wl) < 0) continue;
            var f = labels[j].getAttribute && labels[j].getAttribute('for');
            if (f) { var t1 = document.getElementById(f); if (t1) return t1; }
            var inner = labels[j].querySelector('input,select,textarea');
            if (inner) return inner;
            var sib = labels[j].parentElement && labels[j].parentElement.querySelector('input,select,textarea');
            if (sib) return sib;
          }
          return null;
        }
        default:
          return null;
      }
    } catch (e) {
      return null;
    }
  }

  // 自愈打分：返回 {el, score, why}
  function healBy(target) {
    if (!target) return null;
    var wantLabel = norm(target.label), wantPh = norm(target.placeholder),
        wantText = norm(target.text), wantAria = norm(target.aria),
        wantTag = (target.tag || '').toLowerCase(), wantType = (target.type || '').toLowerCase();
    if (!wantLabel && !wantPh && !wantText && !wantAria) return null;

    var cands = document.querySelectorAll(CANDIDATE_SEL);
    var best = [], bestScore = 0;
    for (var i = 0; i < cands.length; i++) {
      var el = cands[i];
      if (!isVisible(el)) continue;
      var s = 0, why = [];
      var tag = (el.tagName || '').toLowerCase();
      if (wantTag && tag === wantTag) { s += 2; why.push('标签一致'); }
      var type = (el.getAttribute && el.getAttribute('type') || '').toLowerCase();
      if (wantType && type === wantType) { s += 1; }
      var lb = norm(labelOf(el)), ph = norm(el.getAttribute && el.getAttribute('placeholder')),
          tx = visibleText(el), ar = norm(el.getAttribute && el.getAttribute('aria-label'));
      if (wantLabel) {
        if (lb === wantLabel) { s += 5; why.push('标签文字完全一致'); }
        else if (lb && (lb.indexOf(wantLabel) >= 0 || wantLabel.indexOf(lb) >= 0)) { s += 3; why.push('标签文字包含'); }
      }
      if (wantPh) {
        if (ph === wantPh) { s += 4; why.push('placeholder 一致'); }
        else if (ph && (ph.indexOf(wantPh) >= 0 || wantPh.indexOf(ph) >= 0)) { s += 2; }
      }
      if (wantAria && ar === wantAria) { s += 3; why.push('aria 一致'); }
      if (wantText) {
        if (tx === wantText) { s += 3; why.push('文本一致'); }
        else if (tx && (tx.indexOf(wantText) >= 0 || wantText.indexOf(tx) >= 0)) { s += 1; }
      }
      if (s > bestScore) { bestScore = s; best = [{ el: el, why: why }]; }
      else if (s === bestScore && s > 0) best.push({ el: el, why: why });
    }
    if (bestScore < 4 || best.length !== 1) return null;
    return { el: best[0].el, score: bestScore, why: best[0].why.join('、') };
  }

  function resolve(locator, target) {
    var tried = [];
    var plan = [];
    if (locator && locator.strategy) plan.push({ strategy: locator.strategy, value: locator.value });
    var fb = (locator && locator.fallbacks) || [];
    for (var i = 0; i < fb.length; i++) if (fb[i] && fb[i].strategy) plan.push({ strategy: fb[i].strategy, value: fb[i].value });

    for (var j = 0; j < plan.length; j++) {
      var el = byStrategy(plan[j].strategy, plan[j].value);
      if (el) {
        return {
          el: el, healed: false,
          used: plan[j].strategy + '=' + plan[j].value,
          tried: tried
        };
      }
      tried.push(plan[j].strategy + '=' + plan[j].value);
    }
    // 自愈
    markAll();
    var h = healBy(target);
    if (h) {
      return {
        el: h.el, healed: true,
        used: 'uid=' + (h.el.getAttribute(KEY_ATTR) || '') + ' (自愈:' + h.why + ')',
        tried: tried
      };
    }
    return { el: null, healed: false, used: '', tried: tried };
  }

  /* ---------------- 动作执行 ---------------- */

  function setValue(el, value) {
    var v = value === null || value === undefined ? '' : String(value);
    el.focus && el.focus();
    if (el.isContentEditable) {
      el.textContent = v;
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
      return;
    }
    var proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    var desc = Object.getOwnPropertyDescriptor(proto, 'value');
    if (desc && desc.set) desc.set.call(el, v); else el.value = v;
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    el.dispatchEvent(new Event('blur', { bubbles: false }));
  }

  function pickSelect(el, value) {
    var want = norm(value);
    var opts = el.options || [];
    for (var i = 0; i < opts.length; i++) {
      if (norm(opts[i].value) === want || norm(opts[i].textContent) === want) return opts[i].value;
    }
    for (var j = 0; j < opts.length; j++) {
      if (norm(opts[j].textContent).indexOf(want) >= 0) return opts[j].value;
    }
    return null;
  }

  function pressKey(key) {
    var el = document.activeElement || document.body;
    var codes = { Enter: 13, Tab: 9, Escape: 27, ArrowDown: 40, ArrowUp: 38, Space: 32 };
    var code = codes[key] || 0;
    var evs = [
      new KeyboardEvent('keydown', { key: key, code: key, bubbles: true, cancelable: true }),
      new KeyboardEvent('keypress', { key: key, code: key, bubbles: true, cancelable: true }),
      new KeyboardEvent('keyup', { key: key, code: key, bubbles: true, cancelable: true })
    ];
    for (var i = 0; i < evs.length; i++) el.dispatchEvent(evs[i]);
    // 表单里按回车通常期望提交
    if (key === 'Enter' && el.form && el.form.requestSubmit) {
      try { el.form.requestSubmit(); } catch (e) {}
    }
    return code;
  }

  function checkExpect(expect) {
    var e = norm(expect);
    if (!e) return { ok: true };
    if (e.indexOf('url:') === 0) {
      var part = e.slice(4).trim();
      return { ok: location.href.indexOf(part) >= 0, message: '当前 URL: ' + location.href + '，期望包含: ' + part };
    }
    if (e.indexOf('js:') === 0) {
      try {
        var ok = !!(new Function('return (' + e.slice(3) + ')')).call(window);
        return { ok: ok, message: ok ? '' : '断言表达式为假: ' + e.slice(3) };
      } catch (err) {
        return { ok: false, message: '断言表达式报错: ' + err.message };
      }
    }
    var text = document.body ? (document.body.innerText || '') : '';
    var hit = text.indexOf(e) >= 0;
    return {
      ok: hit,
      message: hit ? '' : '页面未出现期望文本: ' + e,
      snippet: hit ? '' : norm(text).slice(0, 200)
    };
  }

  async function runAction(req) {
    var action = req.action, value = req.value, timeoutMs = req.timeoutMs || 8000;
    var locator = req.locator, target = req.target;

    if (action === 'navigate') {
      return { ok: true, message: '导航到 ' + value, navigate: value };
    }
    if (action === 'script') {
      try { new Function(value).call(window); return { ok: true, message: '执行脚本' }; }
      catch (e) { return { ok: false, message: '脚本报错: ' + e.message }; }
    }
    if (action === 'assert') {
      var r0 = checkExpect(value);
      return { ok: r0.ok, message: r0.message || '断言通过', snippet: r0.snippet };
    }
    if (action === 'wait') {
      var v = norm(value);
      if (/^\d+$/.test(v)) { await sleep(parseInt(v, 10)); return { ok: true, message: '等待 ' + v + 'ms' }; }
      var t0 = Date.now();
      while (Date.now() - t0 < timeoutMs) {
        if (document.body && (document.body.innerText || '').indexOf(v) >= 0) return { ok: true, message: '等待到文本: ' + v };
        await sleep(200);
      }
      return { ok: false, message: '等待超时，未出现文本: ' + v };
    }

    var found = resolve(locator, target);
    if (!found.el) {
      return {
        ok: false,
        message: '元素未找到（已尝试 ' + (found.tried.length ? found.tried.join(' → ') : '无定位') + '，自愈也未命中）',
        tried: found.tried,
        url: location.href
      };
    }
    var el = found.el;
    el.scrollIntoView && el.scrollIntoView({ block: 'center', inline: 'nearest' });
    var info = describe(el);
    var base = { used: found.used, healed: found.healed, tried: found.tried, tag: info.tag, label: info.label, css: uniqueSelector(el), url: location.href };

    function withOk(ok, message, extra) {
      var o = { ok: ok, message: message };
      for (var k in base) o[k] = base[k];
      if (extra) for (var k2 in extra) o[k2] = extra[k2];
      return o;
    }

    try {
      switch (action) {
        case 'click': {
          if (el.disabled) return withOk(false, '元素处于禁用状态');
          if (!isVisible(el)) return withOk(false, '元素不可见，无法点击');
          el.click();
          return withOk(true, '点击 ' + (info.label || info.text || info.tag));
        }
        case 'input': {
          if (el.disabled || el.readOnly) return withOk(false, '输入框禁用或只读');
          setValue(el, value);
          await sleep(60);
          if (String(el.value === undefined ? el.textContent : el.value) !== String(value === undefined ? '' : value)) {
            return withOk(false, '输入后取值不一致：期望 ' + value + '，实际 ' + (el.value === undefined ? el.textContent : el.value));
          }
          return withOk(true, '输入「' + value + '」到 ' + (info.label || info.placeholder || info.tag));
        }
        case 'select': {
          var picked = pickSelect(el, value);
          if (picked === null) return withOk(false, '下拉框未找到选项: ' + value);
          el.value = picked;
          el.dispatchEvent(new Event('input', { bubbles: true }));
          el.dispatchEvent(new Event('change', { bubbles: true }));
          return withOk(true, '选择「' + value + '」到 ' + (info.label || info.tag));
        }
        case 'check':
        case 'uncheck': {
          var want = action === 'check';
          if (!!el.checked === want) return withOk(true, '已处于' + (want ? '勾选' : '未勾选') + '状态');
          el.click();
          return withOk(true, (want ? '勾选' : '取消勾选') + ' ' + (info.label || info.tag));
        }
        case 'hover': {
          ['mouseover', 'mouseenter', 'mousemove'].forEach(function (t) {
            el.dispatchEvent(new MouseEvent(t, { bubbles: t !== 'mouseenter', cancelable: true }));
          });
          return withOk(true, '悬停 ' + (info.label || info.text || info.tag));
        }
        case 'key': {
          pressKey(norm(value) || 'Enter');
          return withOk(true, '按键 ' + (norm(value) || 'Enter'));
        }
        case 'upload':
          return withOk(true, '上传目标已定位');
        default:
          return withOk(false, '未知动作: ' + action);
      }
    } catch (e) {
      return withOk(false, '执行异常: ' + (e && e.message ? e.message : String(e)));
    }
  }

  window.__uitestKeys = {
    version: VERSION,
    KEY_ATTR: KEY_ATTR,
    fnv1a32: fnv1a32,
    norm: norm,
    computeKey: computeKey,
    markAll: markAll,
    resolve: resolve,
    describe: describe,
    labelOf: labelOf,
    fieldKind: fieldKind,
    clickTargetOf: clickTargetOf,
    uniqueSelector: uniqueSelector,
    cssPath: cssPath,
    isVisible: isVisible,
    runAction: runAction,
    checkExpect: checkExpect
  };
})();
