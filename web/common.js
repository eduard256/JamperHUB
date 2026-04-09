/* ============================================
   JamperHUB Common JS
   API helpers, polling, utilities, chart helpers
   ============================================ */

var API_BASE = '';

/* ---- API ---- */
var api = {
  get: function(path) {
    return fetch(API_BASE + path).then(function(r) { return r.json(); });
  },
  post: function(path, body) {
    return fetch(API_BASE + path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function(r) { return r.json(); });
  },
  put: function(path, body) {
    return fetch(API_BASE + path, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: typeof body === 'string' ? body : JSON.stringify(body)
    }).then(function(r) { return r.json(); });
  },
  del: function(path) {
    return fetch(API_BASE + path, { method: 'DELETE' }).then(function(r) { return r.json(); });
  }
};

/* ---- POLLING ---- */
function poll(fn, interval) {
  fn();
  return setInterval(fn, interval || 3000);
}

/* ---- FORMAT HELPERS ---- */
function formatBytes(n) {
  if (n == null) return '--';
  if (n >= 1e12) return (n / 1e12).toFixed(1) + ' TB';
  if (n >= 1e9) return (n / 1e9).toFixed(1) + ' GB';
  if (n >= 1e6) return (n / 1e6).toFixed(1) + ' MB';
  if (n >= 1e3) return (n / 1e3).toFixed(0) + ' KB';
  return n + ' B';
}

function formatSpeed(mbps) {
  if (mbps == null) return '--';
  return mbps.toFixed(1) + ' Mbps';
}

function formatLatency(ms) {
  if (ms == null) return '--';
  return ms.toFixed(0) + ' ms';
}

function formatUptime(seconds) {
  if (seconds == null || seconds <= 0) return '--';
  var d = Math.floor(seconds / 86400);
  var h = Math.floor((seconds % 86400) / 3600);
  var m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return d + 'd ' + h + 'h';
  if (h > 0) return h + 'h ' + m + 'm';
  return m + 'm';
}

function formatTimeAgo(dateStr) {
  if (!dateStr) return '--';
  var diff = (Date.now() - new Date(dateStr).getTime()) / 1000;
  if (diff < 60) return Math.floor(diff) + 's ago';
  if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
  if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
  return Math.floor(diff / 86400) + 'd ago';
}

function formatTime(dateStr) {
  if (!dateStr) return '--';
  var d = new Date(dateStr);
  return pad2(d.getHours()) + ':' + pad2(d.getMinutes()) + ':' + pad2(d.getSeconds());
}

function pad2(n) { return n < 10 ? '0' + n : '' + n; }

/* ---- COLOR GENERATOR (deterministic from name) ---- */
function tunnelColor(name) {
  var hash = 0;
  for (var i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash);
  var h = ((hash % 360) + 360) % 360;
  return {
    stroke: 'hsl(' + h + ', 35%, 55%)',
    fill: 'hsla(' + h + ', 35%, 55%, 0.08)',
    point: 'hsl(' + h + ', 35%, 55%)'
  };
}

/* ---- SVG SPARKLINE ---- */
function renderSparkline(container, data, color) {
  if (!container || !data || data.length < 2) return;
  var w = container.clientWidth || 120;
  var h = 28;
  var max = -Infinity, min = Infinity;
  for (var i = 0; i < data.length; i++) {
    if (data[i] > max) max = data[i];
    if (data[i] < min) min = data[i];
  }
  var range = max - min || 1;
  var pts = [];
  for (var i = 0; i < data.length; i++) {
    var x = (i / (data.length - 1)) * w;
    var y = h - ((data[i] - min) / range) * (h - 4) - 2;
    pts.push(x + ',' + y);
  }
  var gid = 'sg' + Math.random().toString(36).slice(2, 8);

  var ns = 'http://www.w3.org/2000/svg';
  var svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 ' + w + ' ' + h);
  svg.setAttribute('preserveAspectRatio', 'none');
  svg.style.width = '100%';
  svg.style.height = '100%';

  var defs = document.createElementNS(ns, 'defs');
  var grad = document.createElementNS(ns, 'linearGradient');
  grad.setAttribute('id', gid);
  grad.setAttribute('x1', '0'); grad.setAttribute('y1', '0');
  grad.setAttribute('x2', '0'); grad.setAttribute('y2', '1');
  var s1 = document.createElementNS(ns, 'stop');
  s1.setAttribute('offset', '0%'); s1.setAttribute('stop-color', color); s1.setAttribute('stop-opacity', '0.3');
  var s2 = document.createElementNS(ns, 'stop');
  s2.setAttribute('offset', '100%'); s2.setAttribute('stop-color', color); s2.setAttribute('stop-opacity', '0');
  grad.appendChild(s1); grad.appendChild(s2);
  defs.appendChild(grad);
  svg.appendChild(defs);

  var area = document.createElementNS(ns, 'path');
  area.setAttribute('d', 'M' + pts[0] + ' ' + pts.slice(1).map(function(p){return 'L'+p}).join(' ') + ' L' + w + ',' + h + ' L0,' + h + ' Z');
  area.setAttribute('fill', 'url(#' + gid + ')');
  svg.appendChild(area);

  var line = document.createElementNS(ns, 'polyline');
  line.setAttribute('points', pts.join(' '));
  line.setAttribute('fill', 'none');
  line.setAttribute('stroke', color);
  line.setAttribute('stroke-width', '1.5');
  line.setAttribute('stroke-linejoin', 'round');
  svg.appendChild(line);

  container.textContent = '';
  container.appendChild(svg);
}

/* ---- ARC GAUGE ---- */
function renderArcGauge(container, value, max, unit, name) {
  var r = 54, cx = 65, cy = 65;
  var circ = 2 * Math.PI * r;
  var arcLen = circ * 0.75;
  var pct = Math.min(value / max, 1);
  var offset = arcLen * (1 - pct);
  var hue = pct * 120;
  var color = 'hsl(' + hue + ', 35%, 40%)';

  var ns = 'http://www.w3.org/2000/svg';
  var e = document.createElement('div');
  e.className = 'arc-gauge';

  var svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 130 130');

  var bg = document.createElementNS(ns, 'circle');
  bg.setAttribute('class', 'arc-bg');
  bg.setAttribute('cx', cx); bg.setAttribute('cy', cy); bg.setAttribute('r', r);
  bg.setAttribute('stroke-dasharray', arcLen + ' ' + circ);
  svg.appendChild(bg);

  var fill = document.createElementNS(ns, 'circle');
  fill.setAttribute('class', 'arc-fill');
  fill.setAttribute('cx', cx); fill.setAttribute('cy', cy); fill.setAttribute('r', r);
  fill.setAttribute('stroke', color);
  fill.setAttribute('stroke-dasharray', arcLen + ' ' + circ);
  fill.setAttribute('stroke-dashoffset', offset);
  fill.style.filter = 'drop-shadow(0 0 4px ' + color + ')';
  svg.appendChild(fill);

  e.appendChild(svg);

  var center = document.createElement('div');
  center.className = 'arc-center';
  var valEl = document.createElement('div');
  valEl.className = 'arc-value';
  valEl.textContent = value;
  var labEl = document.createElement('div');
  labEl.className = 'arc-label';
  labEl.textContent = unit;
  center.appendChild(valEl);
  center.appendChild(labEl);
  e.appendChild(center);

  if (name) {
    var nameEl = document.createElement('div');
    nameEl.className = 'arc-name';
    nameEl.textContent = name;
    e.appendChild(nameEl);
  }

  container.appendChild(e);
  return e;
}

/* ---- UPLOT TOOLTIP PLUGIN ---- */
function tooltipPlugin() {
  var tooltip = document.createElement('div');
  tooltip.className = 'chart-tooltip';
  tooltip.style.display = 'none';

  var timeEl = document.createElement('div');
  timeEl.className = 'chart-tooltip-time';
  var bodyEl = document.createElement('div');
  bodyEl.className = 'chart-tooltip-body';
  tooltip.appendChild(timeEl);
  tooltip.appendChild(bodyEl);

  function show(u) {
    var idx = u.cursor.idx;
    if (idx == null) { tooltip.style.display = 'none'; return; }

    // build tooltip content with safe DOM
    while (bodyEl.firstChild) bodyEl.removeChild(bodyEl.firstChild);

    for (var i = 1; i < u.series.length; i++) {
      var s = u.series[i];
      if (!s.show) continue;
      var v = u.data[i][idx];
      var formatted = s.value ? s.value(u, v) : (v != null ? v.toFixed(1) : '--');
      var color = s.stroke;

      var row = document.createElement('div');
      row.className = 'chart-tooltip-row';

      var dot = document.createElement('span');
      dot.className = 'chart-tooltip-dot';
      dot.style.background = color;

      var lbl = document.createElement('span');
      lbl.className = 'chart-tooltip-label';
      lbl.textContent = s.label;

      var val = document.createElement('span');
      val.className = 'chart-tooltip-val';
      val.textContent = formatted;

      row.appendChild(dot);
      row.appendChild(lbl);
      row.appendChild(val);
      bodyEl.appendChild(row);
    }

    var ts = u.data[0][idx];
    var d = new Date(ts * 1000);
    timeEl.textContent = pad2(d.getHours()) + ':' + pad2(d.getMinutes());
    tooltip.style.display = 'block';

    var left = u.cursor.left + u.over.offsetLeft;
    var top = u.cursor.top + u.over.offsetTop;
    var tw = tooltip.offsetWidth;
    var th = tooltip.offsetHeight;
    var pw = u.root.offsetWidth;

    var x = left + 16;
    if (x + tw > pw) x = left - tw - 8;
    var y = top - th - 8;
    if (y < 0) y = top + 16;

    tooltip.style.left = x + 'px';
    tooltip.style.top = y + 'px';
  }

  return {
    hooks: {
      init: function(u) {
        u.root.style.position = 'relative';
        u.root.appendChild(tooltip);
      },
      setCursor: function(u) { show(u); }
    }
  };
}

/* ---- UPLOT BASE OPTIONS ---- */
function chartOpts(width, height) {
  var gridStyle = { stroke: '#2c2c32', width: 1, dash: [2, 4] };
  var tickStyle = { stroke: '#2c2c32', width: 1, size: 4 };
  var font = '10px Martian Mono';
  return {
    width: width,
    height: height || 200,
    plugins: [tooltipPlugin()],
    cursor: {
      show: true,
      drag: { setScale: false },
      points: { size: 6, fill: '#5a9a5a', stroke: '#1a1a1e', width: 2 }
    },
    axes: [
      { stroke: '#555550', grid: gridStyle, ticks: tickStyle, font: font, gap: 8 },
      { stroke: '#555550', grid: gridStyle, ticks: tickStyle, font: font, gap: 8, size: 50 }
    ],
    legend: { show: true, live: true },
    padding: [12, 8, 0, 0]
  };
}

/* ---- STAT HELPERS ---- */
function arrMin(a) { var m = a[0]; for (var i = 1; i < a.length; i++) if (a[i] != null && a[i] < m) m = a[i]; return m; }
function arrMax(a) { var m = a[0]; for (var i = 1; i < a.length; i++) if (a[i] != null && a[i] > m) m = a[i]; return m; }
function arrAvg(a) { var s = 0, c = 0; for (var i = 0; i < a.length; i++) if (a[i] != null) { s += a[i]; c++; } return c ? s / c : 0; }

/* ---- MODAL HELPERS ---- */
function openModal(title, contentFn) {
  var overlay = document.createElement('div');
  overlay.className = 'modal-overlay';

  var modal = document.createElement('div');
  modal.className = 'modal';

  var titleEl = document.createElement('div');
  titleEl.className = 'modal-title';
  titleEl.textContent = title;
  modal.appendChild(titleEl);

  contentFn(modal, function() { closeModal(overlay); });
  overlay.appendChild(modal);
  document.body.appendChild(overlay);

  requestAnimationFrame(function() { overlay.classList.add('visible'); });

  overlay.addEventListener('click', function(e) {
    if (e.target === overlay) closeModal(overlay);
  });

  return overlay;
}

function closeModal(overlay) {
  overlay.classList.remove('visible');
  setTimeout(function() { overlay.remove(); }, 200);
}

/* ---- DOM HELPERS ---- */
function el(tag, cls, text) {
  var e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}

function qs(sel) { return document.querySelector(sel); }
function qsa(sel) { return document.querySelectorAll(sel); }

/* ---- STABILITY TIMELINE ---- */
function renderStability(container, segments) {
  container.textContent = '';
  container.style.position = 'relative';

  var w = container.clientWidth;
  var h = 60;

  var ns = 'http://www.w3.org/2000/svg';
  var svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('width', w);
  svg.setAttribute('height', h);
  svg.setAttribute('viewBox', '0 0 ' + w + ' ' + h);

  var x = 0;
  for (var i = 0; i < segments.length; i++) {
    var seg = segments[i];
    var sw = seg.pct * w;
    if (seg.proto) {
      var c = tunnelColor(seg.label || seg.proto);
      var rect = document.createElementNS(ns, 'rect');
      rect.setAttribute('x', x); rect.setAttribute('y', 10);
      rect.setAttribute('width', sw); rect.setAttribute('height', 30);
      rect.setAttribute('rx', 3);
      rect.setAttribute('fill', c.fill.replace('0.08', '0.15'));
      rect.setAttribute('stroke', c.stroke);
      rect.setAttribute('stroke-width', 1);
      svg.appendChild(rect);

      if (seg.label && sw > 40) {
        var txt = document.createElementNS(ns, 'text');
        txt.setAttribute('x', x + sw / 2); txt.setAttribute('y', 30);
        txt.setAttribute('text-anchor', 'middle');
        txt.setAttribute('font-family', 'Martian Mono');
        txt.setAttribute('font-size', '7');
        txt.setAttribute('font-weight', '300');
        txt.setAttribute('fill', c.stroke);
        txt.textContent = seg.label;
        svg.appendChild(txt);
      }
    }
    x += sw;
  }

  // time labels
  var nowTs = Date.now();
  var startTs = nowTs - 24 * 60 * 60 * 1000;
  var step = 60;
  for (var tx = 0; tx <= w; tx += step) {
    var pct = tx / w;
    var d = new Date(startTs + pct * 24 * 60 * 60 * 1000);
    var label = pad2(d.getHours()) + ':' + pad2(d.getMinutes());
    if (tx >= w - 20) label = 'now';
    var anchor = tx === 0 ? 'start' : (tx >= w - 20 ? 'end' : 'middle');
    var tl = document.createElementNS(ns, 'text');
    tl.setAttribute('x', tx); tl.setAttribute('y', 56);
    tl.setAttribute('text-anchor', anchor);
    tl.setAttribute('font-family', 'Martian Mono');
    tl.setAttribute('font-size', '7');
    tl.setAttribute('font-weight', '300');
    tl.setAttribute('fill', '#555550');
    tl.textContent = label;
    svg.appendChild(tl);
  }

  container.appendChild(svg);

  // hover cursor
  var cursorLine = document.createElement('div');
  cursorLine.style.cssText = 'position:absolute;top:10px;width:1px;height:30px;background:#555550;pointer-events:none;display:none;z-index:2;';
  container.appendChild(cursorLine);

  var timeTip = document.createElement('div');
  timeTip.style.cssText = 'position:absolute;top:0;padding:2px 6px;background:#222226;border:1px solid #2c2c32;border-radius:4px;font-family:Martian Mono;font-size:0.55rem;font-weight:300;color:#e3e3de;pointer-events:none;display:none;z-index:3;white-space:nowrap;transform:translateX(-50%);';
  container.appendChild(timeTip);

  container.addEventListener('mousemove', function(e) {
    var rect = container.getBoundingClientRect();
    var px = e.clientX - rect.left;
    var p = px / w;
    if (p < 0 || p > 1) return;

    cursorLine.style.left = px + 'px';
    cursorLine.style.display = 'block';

    var ts = new Date(startTs + p * 24 * 60 * 60 * 1000);
    var timeStr = pad2(ts.getHours()) + ':' + pad2(ts.getMinutes());

    var cumX = 0;
    for (var j = 0; j < segments.length; j++) {
      var segW = segments[j].pct * w;
      if (px >= cumX && px < cumX + segW && segments[j].proto) {
        timeStr = timeStr + '  ' + segments[j].label;
        break;
      }
      cumX += segW;
    }

    timeTip.textContent = timeStr;
    timeTip.style.left = px + 'px';
    timeTip.style.display = 'block';
  });

  container.addEventListener('mouseleave', function() {
    cursorLine.style.display = 'none';
    timeTip.style.display = 'none';
  });
}
