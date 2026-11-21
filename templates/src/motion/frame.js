// The scene frame: runs inside a sandboxed iframe (an origin of its own, no
// myco bridge), so the scene code people and agents write cannot reach the
// page's data or this computer. It builds every scene of the video, moves
// them to any time (each frame a pure function of time), and draws a frame
// into a bitmap for the page to encode: the DOM is serialized into an SVG
// <foreignObject>, decoded as an image and drawn on a canvas.
//
// The animation engine is our own, with a timeline API close to the one
// motion designers know (to / from / fromTo / set, positions, stagger,
// eases, repeat, yoyo): it writes inline styles only, which is what the
// serialized frame carries.
//
// Plain JavaScript (imported as text and put in the frame as it is).
(function () {
  "use strict";
  var W = 1920, H = 1080, mode = "preview";
  var view = document.getElementById("view");
  var stage = document.getElementById("stage");
  var scenes = [];
  var themeCSS = "", sceneCSS = "";
  var lastTime = 0;

  function post(msg, transfer) { parent.postMessage(msg, "*", transfer || []); }

  // ---------------------------------------------------------------- eases
  function inOf(name, a, b) {
    switch (name) {
      case "power0": case "none": case "linear": return function (p) { return p; };
      case "power1": case "quad": return function (p) { return p * p; };
      case "power2": case "cubic": return function (p) { return p * p * p; };
      case "power3": case "quart": return function (p) { return p * p * p * p; };
      case "power4": case "quint": case "strong": return function (p) { return p * p * p * p * p; };
      case "sine": return function (p) { return 1 - Math.cos(p * Math.PI / 2); };
      case "expo": return function (p) { return p === 0 ? 0 : Math.pow(2, 10 * (p - 1)); };
      case "circ": return function (p) { return 1 - Math.sqrt(1 - p * p); };
      case "back": { var s = isNaN(a) ? 1.70158 : a; return function (p) { return p * p * ((s + 1) * p - s); }; }
      case "elastic": {
        var amp = Math.max(1, isNaN(a) ? 1 : a), per = isNaN(b) ? 0.3 : b, sh = per / (2 * Math.PI) * Math.asin(1 / amp);
        return function (p) { return p === 0 || p === 1 ? p : -(amp * Math.pow(2, 10 * (p - 1)) * Math.sin(((p - 1) - sh) * (2 * Math.PI) / per)); };
      }
      case "bounce": return function (p) { return 1 - bounceOut(1 - p); };
    }
    return null;
  }
  function bounceOut(p) {
    var n = 7.5625, d = 2.75;
    if (p < 1 / d) return n * p * p;
    if (p < 2 / d) return n * (p -= 1.5 / d) * p + 0.75;
    if (p < 2.5 / d) return n * (p -= 2.25 / d) * p + 0.9375;
    return n * (p -= 2.625 / d) * p + 0.984375;
  }
  var easeCache = {};
  function ease(e) {
    if (typeof e === "function") return e;
    var key = String(e == null ? "power1.out" : e).replace(/\s+/g, "");
    if (easeCache[key]) return easeCache[key];
    var f, m = /^steps\((\d+)\)$/.exec(key);
    if (m) { var n = +m[1]; f = function (p) { return Math.min(1, Math.floor(p * n) / n); }; }
    else {
      m = /^([a-z0-9]+)(?:\.(in|out|inOut))?(?:\(([^,)]*)(?:,([^)]*))?\))?$/i.exec(key) || [];
      var fin = inOf((m[1] || "").toLowerCase(), parseFloat(m[3]), parseFloat(m[4]));
      if (!fin) fin = inOf("power1");
      var dir = m[2] || (/^(none|linear|power0)$/.test(m[1] || "") ? "in" : "out");
      f = dir === "in" ? fin
        : dir === "out" ? function (p) { return 1 - fin(1 - p); }
        : function (p) { return p < 0.5 ? fin(p * 2) / 2 : 1 - fin((1 - p) * 2) / 2; };
    }
    easeCache[key] = f;
    return f;
  }

  // ---------------------------------------------------------------- values
  var probe = document.createElement("i");
  probe.style.display = "none";
  document.body.appendChild(probe);
  var colorCache = {};
  function rgba(c) {
    if (colorCache[c]) return colorCache[c];
    probe.style.color = "";
    probe.style.color = c;
    var v = probe.style.color ? getComputedStyle(probe).color : c;
    var m = /^rgba?\(([^)]+)\)$/.exec(v);
    if (m) { var p = m[1].split(/[,\s/]+/).filter(Boolean); v = "rgba(" + p[0] + ", " + p[1] + ", " + p[2] + ", " + (p[3] == null ? 1 : p[3]) + ")"; }
    colorCache[c] = v;
    return v;
  }
  // colours written as #hex, names or rgb() become rgba(), so any two
  // colours have the same numbers to go between
  function normal(s) {
    s = String(s);
    s = s.replace(/#[0-9a-f]{3,8}\b|rgba?\([^)]*\)|hsla?\([^)]*\)/gi, function (c) { return rgba(c); });
    if (/^[a-z]+$/i.test(s) && s !== "none" && s !== "auto" && CSS.supports("color", s)) s = rgba(s);
    return s;
  }
  var NUM = /[-+]?(?:\d*\.\d+|\d+\.?)(?:e[-+]?\d+)?/gi;
  function parse(v) {
    if (typeof v === "number") return { n: [v], s: ["", ""] };
    var s = normal(v), n = [], parts = [], last = 0, m;
    NUM.lastIndex = 0;
    while ((m = NUM.exec(s))) { parts.push(s.slice(last, m.index)); n.push(parseFloat(m[0])); last = m.index + m[0].length; }
    parts.push(s.slice(last));
    return { n: n, s: parts };
  }
  function mix(a, b, p, round) {
    if (a.n.length !== b.n.length) return p < 1 ? (p <= 0 ? a : b) : b;
    var out = { n: new Array(b.n.length), s: b.s };
    for (var i = 0; i < b.n.length; i++) out.n[i] = a.n[i] + (b.n[i] - a.n[i]) * p;
    if (round) out.n = out.n.map(Math.round);
    return out;
  }
  function str(v, unit) {
    if (v.s.length === 2 && v.s[0] === "" && v.s[1] === "" && unit) return fmtNum(v.n[0]) + unit;
    var o = v.s[0];
    for (var i = 0; i < v.n.length; i++) o += fmtNum(v.n[i]) + v.s[i + 1];
    return o;
  }
  function fmtNum(x) { return String(Math.round(x * 10000) / 10000); }
  function rel(raw, cur) {
    if (typeof raw === "string") {
      var m = /^([+\-*])=\s*(-?[\d.]+)(.*)$/.exec(raw);
      if (m && cur && cur.n.length === 1) {
        var d = parseFloat(m[2]), c = cur.n[0];
        return { n: [m[1] === "+" ? c + d : m[1] === "-" ? c - d : c * d], s: cur.s };
      }
    }
    return parse(raw);
  }

  var TRANSFORM = { x: 0, y: 0, xPercent: 0, yPercent: 0, rotation: 0, rotate: 0, rotationX: 0, rotateX: 0, rotationY: 0, rotateY: 0, skewX: 0, skewY: 0, scale: 1, scaleX: 1, scaleY: 1, z: 0 };
  var UNITLESS = { opacity: 1, zIndex: 1, fontWeight: 1, lineHeight: 1, flexGrow: 1, flexShrink: 1, order: 1, autoAlpha: 1, draw: 1, count: 1 };
  var CONTROL = { duration: 1, ease: 1, stagger: 1, delay: 1, repeat: 1, yoyo: 1, repeatDelay: 1, immediateRender: 1, decimals: 1, separator: 1, prefix: 1, suffix: 1, attr: 1, overwrite: 1, onUpdate: 1, onComplete: 1, onStart: 1 };

  // ---------------------------------------------------------------- timeline
  // A scene's timeline: tweens become segments of channels (one element's
  // one property); a seek sets every channel from scratch.
  function Timeline(sceneRoot, length) {
    var groups = [];
    var lastGroup = { start: 0, end: 0 };
    var api = {};
    function endAll() { var e = 0; for (var i = 0; i < groups.length; i++) e = Math.max(e, groups[i].end); return e; }
    function at(pos) {
      if (pos == null || pos === "") return endAll();
      if (typeof pos === "number") return Math.max(0, pos);
      var m = /^([<>])?\s*([+-]=)?\s*(-?[\d.]+)?$/.exec(String(pos).trim());
      if (!m) return endAll();
      var base = m[1] === "<" ? lastGroup.start : m[1] === ">" ? lastGroup.end : endAll();
      var n = m[3] ? parseFloat(m[3]) : 0;
      if (m[2] === "-=") n = -n;
      if (!m[1] && !m[2] && m[3]) return Math.max(0, n);
      return Math.max(0, base + n);
    }
    function targets(t) {
      if (!t) return [];
      if (typeof t === "string") return Array.prototype.slice.call(sceneRoot.querySelectorAll(t));
      if (t.nodeType) return [t];
      if (typeof t.length === "number") { var out = []; for (var i = 0; i < t.length; i++) out = out.concat(targets(t[i])); return out; }
      return [t];
    }
    function staggerOf(st, n, seed) {
      if (!st || n < 2) return function () { return 0; };
      var o = typeof st === "number" ? { each: st } : st;
      var each = o.amount != null ? o.amount / (n - 1) : (o.each || 0);
      var from = o.from || "start";
      var order = [];
      for (var i = 0; i < n; i++) order.push(i);
      if (from === "random") { var r = rng(seed); for (var j = n - 1; j > 0; j--) { var k = Math.floor(r() * (j + 1)); var tmp = order[j]; order[j] = order[k]; order[k] = tmp; } }
      return function (i) {
        var d;
        if (from === "end") d = n - 1 - i;
        else if (from === "center") d = Math.abs(i - (n - 1) / 2);
        else if (from === "edges") d = (n - 1) / 2 - Math.abs(i - (n - 1) / 2);
        else if (from === "random") d = order.indexOf(i);
        else if (typeof from === "number") d = Math.abs(i - from);
        else d = i;
        return d * each;
      };
    }
    function add(kind, t, a, b, pos) {
      var vars = b || a || {};
      var list = targets(t);
      var dur = kind === "set" ? 0 : Math.max(0, vars.duration == null ? 0.5 : +vars.duration || 0);
      var start = at(pos) + (+vars.delay || 0);
      var off = staggerOf(vars.stagger, list.length, groups.length + 1);
      var group = { start: start, end: start, segs: [] };
      var props = {};
      var keys = Object.keys(Object.assign({}, a || {}, b || {}));
      for (var i = 0; i < keys.length; i++) if (!CONTROL[keys[i]]) props[keys[i]] = 1;
      var attrs = Object.keys(Object.assign({}, (a && a.attr) || {}, (b && b.attr) || {}));
      for (var ti = 0; ti < list.length; ti++) {
        var el = list[ti], s = start + off(ti);
        var repeat = vars.repeat == null ? 0 : +vars.repeat;
        var gap = +vars.repeatDelay || 0;
        var seg = {
          el: el, start: s, dur: dur, kind: kind, ease: ease(vars.ease), repeat: repeat, yoyo: !!vars.yoyo, gap: gap,
          immediate: vars.immediateRender != null ? !!vars.immediateRender : kind === "from" || kind === "fromTo",
          fmt: { decimals: vars.decimals, separator: vars.separator, prefix: vars.prefix, suffix: vars.suffix },
        };
        var span = repeat < 0 ? 1e6 : dur * (repeat + 1) + gap * Math.max(0, repeat);
        group.end = Math.max(group.end, s + span);
        for (var p in props) group.segs.push(make(seg, p, a && a[p], b && b[p]));
        for (var q = 0; q < attrs.length; q++) group.segs.push(make(seg, "attr:" + attrs[q], a && a.attr && a.attr[attrs[q]], b && b.attr && b.attr[attrs[q]]));
      }
      if (!list.length) group.end = start + dur;
      groups.push(group);
      lastGroup = group;
      return api;
    }
    function make(seg, prop, a, b) {
      var s = Object.assign({}, seg);
      s.prop = prop === "rotate" ? "rotation" : prop === "rotateX" ? "rotationX" : prop === "rotateY" ? "rotationY" : prop;
      // from: (vars) → a is where it starts; to / set: a is where it goes;
      // fromTo: a → b
      s.rawFrom = seg.kind === "from" || seg.kind === "fromTo" ? a : undefined;
      s.rawTo = seg.kind === "fromTo" ? b : seg.kind === "from" ? undefined : a;
      return s;
    }
    api.to = function (t, vars, pos) { return add("to", t, vars, null, pos); };
    api.from = function (t, vars, pos) { return add("from", t, vars, null, pos); };
    api.fromTo = function (t, a, b, pos) { return add("fromTo", t, a, b, pos); };
    api.set = function (t, vars, pos) { return add("set", t, vars, null, pos); };
    api.add = function () { return api; };
    api.call = function () { return api; };
    api.duration = function () { return length; };
    api.length = length;

    // ---- resolve: per channel, in order of start, what each segment goes from and to
    var channels = null;
    function base(el, prop) {
      if (prop in TRANSFORM) return { n: [TRANSFORM[prop]], s: ["", ""] };
      if (prop === "draw") return { n: [1], s: ["", ""] };
      if (prop === "count") {
        var txt = String(el.textContent).replace(/[^\d.\-]/g, "");
        el.__places = (txt.split(".")[1] || "").length;
        return { n: [parseFloat(txt) || 0], s: ["", ""] };
      }
      if (prop === "autoAlpha") prop = "opacity";
      if (prop.indexOf("attr:") === 0) { var v = el.getAttribute(prop.slice(5)); return parse(v == null ? 0 : v); }
      if (!el.style) return parse(el[prop] == null ? 0 : el[prop]);
      var cs = prop.indexOf("--") === 0 ? getComputedStyle(el).getPropertyValue(prop).trim() : getComputedStyle(el)[prop];
      return parse(cs == null || cs === "" ? 0 : cs);
    }
    function resolve() {
      channels = new Map();
      var all = [];
      for (var g = 0; g < groups.length; g++) for (var i = 0; i < groups[g].segs.length; i++) all.push(groups[g].segs[i]);
      all.forEach(function (s, i) { s.order = i; });
      all.sort(function (x, y) { return x.start - y.start || x.order - y.order; });
      for (var j = 0; j < all.length; j++) {
        var s = all[j];
        var map = channels.get(s.el);
        if (!map) { map = {}; channels.set(s.el, map); }
        var ch = map[s.prop];
        if (!ch) ch = map[s.prop] = { base: base(s.el, s.prop), segs: [], cur: null };
        var cur = ch.cur || ch.base;
        s.from = s.rawFrom === undefined ? cur : rel(s.rawFrom, cur);
        s.to = s.rawTo === undefined ? cur : rel(s.rawTo, cur);
        if (s.prop === "x" || s.prop === "y") percent(s);
        ch.cur = s.yoyo && s.repeat > 0 && s.repeat % 2 === 1 ? s.from : s.to;
        ch.segs.push(s);
      }
    }
    // x: "50%" is xPercent
    function percent(s) {
      if (s.from.s[1] === "%" || s.to.s[1] === "%") s.pct = true;
    }
    function progress(s, t) {
      if (s.dur <= 0) return t >= s.start ? 1 : 0;
      var local = t - s.start;
      if (local <= 0) return 0;
      var cycle = s.dur + s.gap, n = Math.floor(local / cycle);
      if (s.repeat >= 0 && n > s.repeat) return s.yoyo && s.repeat % 2 === 1 ? 0 : 1;
      var within = local - n * cycle;
      var p = within >= s.dur ? 1 : within / s.dur;
      if (s.yoyo && n % 2 === 1) p = 1 - p;
      return s.ease(p);
    }
    api.seek = function (t) {
      if (!channels) resolve();
      channels.forEach(function (map, el) {
        var tf = null, out = {};
        for (var prop in map) {
          var ch = map[prop], v = null, segs = ch.segs, pct = false;
          var cur = null;
          for (var i = 0; i < segs.length; i++) { if (segs[i].start <= t + 1e-9) cur = segs[i]; else break; }
          if (!cur) {
            var first = segs[0];
            v = first.immediate ? first.from : ch.base;
            pct = !!first.pct;
          } else {
            var p = progress(cur, t);
            v = mix(cur.from, cur.to, p, false);
            pct = !!cur.pct;
          }
          if (prop in TRANSFORM) {
            if (!tf) tf = Object.assign({}, TRANSFORM, { _px: {} });
            var key = pct ? (prop === "x" ? "xPercent" : prop === "y" ? "yPercent" : prop) : prop;
            tf[key] = v.n[0];
          } else out[prop] = { v: v, seg: cur || segs[0] };
        }
        apply(el, tf, out);
      });
    };
    return api;
  }

  function apply(el, tf, out) {
    var st = el.style;
    if (tf) {
      var t = "";
      if (tf.xPercent || tf.yPercent) t += "translate(" + fmtNum(tf.xPercent) + "%, " + fmtNum(tf.yPercent) + "%) ";
      if (tf.x || tf.y || tf.z) t += tf.z ? "translate3d(" + fmtNum(tf.x) + "px, " + fmtNum(tf.y) + "px, " + fmtNum(tf.z) + "px) " : "translate(" + fmtNum(tf.x) + "px, " + fmtNum(tf.y) + "px) ";
      if (tf.rotationX) t += "rotateX(" + fmtNum(tf.rotationX) + "deg) ";
      if (tf.rotationY) t += "rotateY(" + fmtNum(tf.rotationY) + "deg) ";
      if (tf.rotation) t += "rotate(" + fmtNum(tf.rotation) + "deg) ";
      if (tf.skewX || tf.skewY) t += "skew(" + fmtNum(tf.skewX) + "deg, " + fmtNum(tf.skewY) + "deg) ";
      var sx = tf.scaleX * tf.scale, sy = tf.scaleY * tf.scale;
      if (sx !== 1 || sy !== 1) t += "scale(" + fmtNum(sx) + ", " + fmtNum(sy) + ")";
      if (st) st.transform = t.trim() || "none";
    }
    for (var prop in out) {
      var v = out[prop].v, seg = out[prop].seg;
      if (prop === "autoAlpha") { st.opacity = fmtNum(v.n[0]); st.visibility = v.n[0] <= 0.001 ? "hidden" : ""; }
      else if (prop === "draw") {
        if (el.__len == null) { try { el.__len = el.getTotalLength(); } catch (e) { el.__len = 0; } }
        var len = el.__len;
        st.strokeDasharray = fmtNum(len) + " " + fmtNum(len);
        st.strokeDashoffset = fmtNum(len * (1 - Math.max(0, Math.min(1, v.n[0]))));
      } else if (prop === "count") el.textContent = counted(v.n[0], seg.fmt, el.__places || 0, seg.to ? Math.abs(seg.to.n[0]) : 0);
      else if (prop.indexOf("attr:") === 0) el.setAttribute(prop.slice(5), str(v));
      else if (!st) el[prop] = v.n.length === 1 && v.s.join("") === "" ? v.n[0] : str(v);
      else if (prop.indexOf("--") === 0) st.setProperty(prop, str(v));
      else st[prop] = str(v, UNITLESS[prop] ? "" : "px");
    }
  }
  // as many decimals as the number written; thousands separated from 10 000
  // (a year stays 2026)
  function counted(n, f, places, end) {
    var d = f.decimals == null ? places : +f.decimals;
    var s = n.toFixed(d);
    var sep = f.separator == null ? (end >= 10000 ? "," : "") : f.separator;
    if (sep) { var parts = s.split("."); parts[0] = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, sep); s = parts.join("."); }
    return (f.prefix || "") + s + (f.suffix || "");
  }
  function rng(seed) {
    var a = (seed * 2654435761) >>> 0 || 1;
    return function () { a ^= a << 13; a >>>= 0; a ^= a >>> 17; a ^= a << 5; a >>>= 0; return a / 4294967296; };
  }

  // ---------------------------------------------------------------- scenes
  function splitText(el, by) {
    if (typeof el === "string") el = null;
    if (!el) return [];
    var text = el.textContent || "";
    el.textContent = "";
    var out = [];
    var parts = by === "words" ? text.split(/(\s+)/) : Array.from(text);
    for (var i = 0; i < parts.length; i++) {
      var p = parts[i];
      if (!p) continue;
      if (/^\s+$/.test(p)) {
        if (p.indexOf("\n") >= 0) el.appendChild(document.createElement("br"));
        else el.appendChild(document.createTextNode(p));
        continue;
      }
      var span = document.createElement("span");
      span.textContent = p;
      span.style.display = "inline-block";
      if (by !== "words" && p === " ") span.style.whiteSpace = "pre";
      el.appendChild(span);
      out.push(span);
    }
    return out;
  }

  var PLACEHOLDER = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600" viewBox="0 0 800 600"><rect width="800" height="600" fill="#8a8f98" fill-opacity="0.22"/>' +
    '<path d="M250 400l110-130 80 95 55-60 105 95z" fill="#8a8f98" fill-opacity="0.55"/><circle cx="540" cy="220" r="34" fill="#8a8f98" fill-opacity="0.55"/></svg>');

  // CSS of a scene, every rule kept inside the scene ("html", "body",
  // ":root", ":scope" and ".scene" mean the scene itself)
  function splitSel(sel) {
    var out = [], depth = 0, cur = "";
    for (var i = 0; i < sel.length; i++) {
      var c = sel[i];
      if (c === "(" || c === "[") depth++;
      if (c === ")" || c === "]") depth--;
      if (c === "," && depth === 0) { out.push(cur); cur = ""; } else cur += c;
    }
    out.push(cur);
    return out;
  }
  function scopeCSS(css, scope) {
    var tmp = document.createElement("style");
    tmp.textContent = css;
    document.head.appendChild(tmp);
    var walk = function (rules) {
      var s = "";
      for (var i = 0; i < rules.length; i++) {
        var r = rules[i];
        if (r.selectorText != null) {
          var sel = splitSel(r.selectorText).map(function (x) {
            x = x.trim();
            var m = /^(?::root|html|body|:scope|\.scene)(?=$|[\s>+~:.[])/.exec(x);
            if (m) return scope + x.slice(m[0].length);
            return scope + " " + x;
          }).join(", ");
          s += sel + r.cssText.slice(r.cssText.indexOf("{"));
        } else if (r.cssRules && !/^@(?:keyframes|-webkit-keyframes|font-face)/.test(r.cssText)) {
          // @media, @container, @supports, @layer: the rules inside, scoped
          s += r.cssText.slice(0, r.cssText.indexOf("{")) + "{" + walk(r.cssRules) + "}";
        } else s += r.cssText;
      }
      return s;
    };
    try { return walk(tmp.sheet ? tmp.sheet.cssRules : []); } catch (e) { return ""; } finally { tmp.remove(); }
  }

  function esc(s) { return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }

  function load(doc) {
    W = doc.w; H = doc.h; mode = doc.mode || "preview";
    var th = doc.theme || {};
    themeCSS = "#stage{--bg:" + (th.bg || "#0e0f12") + ";--fg:" + (th.fg || "#f4f4f5") + ";--accent:" + (th.accent || "#7c9cff") + ";--accent2:" + (th.accent2 || "#f5b971") +
      ";--muted:color-mix(in srgb, var(--fg) 62%, transparent);--font:" + (th.font || "system-ui, -apple-system, 'PingFang SC', 'Microsoft YaHei', sans-serif") + ";}";
    stage.style.width = W + "px";
    stage.style.height = H + "px";
    stage.innerHTML = "";
    var errors = {};
    var t = 0, css = [];
    scenes = (doc.scenes || []).map(function (s, i) {
      var wrap = document.createElement("section");
      wrap.className = "scene";
      wrap.setAttribute("data-scene", s.id);
      wrap.innerHTML = s.html || "";
      var fields = s.fields || {}, assets = doc.assets || {};
      Array.prototype.forEach.call(wrap.querySelectorAll("[data-field]"), function (el) {
        var k = el.getAttribute("data-field"), v = fields[k];
        if (el.tagName === "IMG") {
          var src = v ? assets[v] || (/^data:image\//.test(v) ? v : "") : "";
          if (src) el.setAttribute("src", src);
          else if (el.hasAttribute("data-optional")) el.setAttribute("data-empty", "");
          else if (!/^data:/.test(el.getAttribute("src") || "")) el.setAttribute("src", PLACEHOLDER);
        } else if (v != null && v !== "") {
          el.innerHTML = esc(v).replace(/\n/g, "<br>");
        }
      });
      Array.prototype.forEach.call(wrap.querySelectorAll("img"), function (el) {
        var a = el.getAttribute("data-asset");
        if (a && assets[a]) el.setAttribute("src", assets[a]);
        var src = el.getAttribute("src") || "";
        if (el.hasAttribute("data-empty")) { el.removeAttribute("src"); el.style.display = "none"; }
        else if (!/^data:/.test(src)) el.setAttribute("src", PLACEHOLDER); // (only pictures kept in the video)
      });
      var scope = '[data-scene="' + String(s.id).replace(/["\\]/g, "") + '"]';
      css.push(scopeCSS(s.css || "", scope));
      var dur = Math.max(0.2, +s.duration || 3);
      var sc = { id: s.id, start: t, dur: dur, trans: s.transition || "cut", transTime: Math.max(0, Math.min(dur, s.transition_time == null ? 0.6 : +s.transition_time)), el: wrap, tl: null, error: "" };
      if (i === 0) sc.trans = "cut";
      t += dur;
      stage.appendChild(wrap);
      return sc;
    });
    sceneCSS = css.join("\n");
    var style = document.createElement("style");
    style.textContent = BASE + themeCSS + sceneCSS;
    stage.insertBefore(style, stage.firstChild);
    // scripts run with every scene laid out, so they can measure
    scenes.forEach(function (sc, i) {
      var s = doc.scenes[i];
      var tl = Timeline(sc.el, sc.dur);
      sc.tl = tl;
      if (s.script && String(s.script).trim()) {
        try {
          var fn = new Function("root", "tl", "util", String(s.script));
          fn(sc.el, tl, util(sc.el, sc.dur, i + 1));
        } catch (e) { sc.error = (e && e.message) || String(e); errors[sc.id] = sc.error; }
      }
      try { tl.seek(0); } catch (e) { sc.error = (e && e.message) || String(e); errors[sc.id] = sc.error; }
    });
    fit();
    show(Math.min(lastTime, total()));
    return { errors: errors, total: total() };
  }
  function util(root, dur, seed) {
    return {
      $: function (s) { return root.querySelector(s); },
      $$: function (s) { return Array.prototype.slice.call(root.querySelectorAll(s)); },
      split: function (el, by) { if (typeof el === "string") el = root.querySelector(el); return splitText(el, by || "chars"); },
      random: rng(seed * 7919),
      duration: dur, width: W, height: H,
    };
  }
  function total() { var e = 0; for (var i = 0; i < scenes.length; i++) e = Math.max(e, scenes[i].start + scenes[i].dur); return e; }

  var TRANS = {
    fade: function (p, a, b) { b.opacity = p; },
    slide: function (p, a, b) { b.transform = "translateX(" + (1 - p) * 100 + "%)"; a.transform = "translateX(" + -p * 100 + "%)"; },
    up: function (p, a, b) { b.transform = "translateY(" + (1 - p) * 100 + "%)"; a.transform = "translateY(" + -p * 30 + "%)"; },
    zoom: function (p, a, b) { b.opacity = p; b.transform = "scale(" + (1.12 - 0.12 * p) + ")"; a.transform = "scale(" + (1 + 0.06 * p) + ")"; },
    wipe: function (p, a, b) { b.clipPath = "inset(0 " + (1 - p) * 100 + "% 0 0)"; },
    blur: function (p, a, b) { b.opacity = p; b.filter = "blur(" + (1 - p) * 24 + "px)"; a.filter = "blur(" + p * 24 + "px)"; },
  };
  var inOut = ease("power2.inOut");
  var visible = [];
  function show(t) {
    lastTime = t;
    var T = total();
    if (!scenes.length) { visible = []; return; }
    t = Math.max(0, Math.min(t, T - 1e-4));
    var i = 0;
    for (var k = 0; k < scenes.length; k++) if (scenes[k].start <= t + 1e-9) i = k;
    var cur = scenes[i], local = t - cur.start, prev = i > 0 ? scenes[i - 1] : null;
    var inTrans = prev && cur.trans !== "cut" && TRANS[cur.trans] && local < cur.transTime;
    visible = inTrans ? [prev, cur] : [cur];
    for (var j = 0; j < scenes.length; j++) {
      var sc = scenes[j], on = visible.indexOf(sc) >= 0;
      sc.el.style.display = on ? "" : "none";
      sc.el.style.opacity = ""; sc.el.style.transform = ""; sc.el.style.clipPath = ""; sc.el.style.filter = ""; sc.el.style.zIndex = sc === cur ? "2" : "1";
    }
    if (inTrans) {
      var a = {}, b = {};
      TRANS[cur.trans](inOut(local / cur.transTime), a, b);
      Object.assign(prev.el.style, a);
      Object.assign(cur.el.style, b);
      safeSeek(prev, prev.dur + local);
    }
    safeSeek(cur, local);
  }
  function safeSeek(sc, t) {
    if (!sc.tl) return;
    try { sc.tl.seek(t); } catch (e) { if (!sc.error) { sc.error = (e && e.message) || String(e); post({ t: "error", scene: sc.id, message: sc.error }); } }
  }

  var BASE = "#stage{position:relative;overflow:hidden;background:var(--bg);color:var(--fg);font-family:var(--font);-webkit-font-smoothing:antialiased;line-height:1.2}" +
    "#stage *{box-sizing:border-box}" +
    "#stage .scene{position:absolute;left:0;top:0;width:100%;height:100%;overflow:hidden;background:var(--bg);container-type:size}" +
    "#stage img{display:block}";

  // ---------------------------------------------------------------- preview size
  function fit() {
    if (mode !== "preview") { view.style.transform = ""; return; }
    var k = Math.min(window.innerWidth / W, window.innerHeight / H);
    view.style.width = W + "px"; view.style.height = H + "px";
    view.style.transform = "translate(" + (window.innerWidth - W * k) / 2 + "px," + (window.innerHeight - H * k) / 2 + "px) scale(" + k + ")";
  }
  window.addEventListener("resize", fit);

  // ---------------------------------------------------------------- frames
  var canvas = null, ctx = null;
  function capture(time, w, h) {
    show(time);
    var parts = [];
    for (var i = 0; i < visible.length; i++) parts.push(new XMLSerializer().serializeToString(visible[i].el));
    var css = BASE + themeCSS + sceneCSS;
    var xml = '<div xmlns="http://www.w3.org/1999/xhtml" id="stage" style="width:' + W + "px;height:" + H + 'px"><style>' + esc(css) + "</style>" + parts.join("") + "</div>";
    // (drawn at the video's own size and scaled on the canvas: WebKit draws
    // nothing of a foreignObject an SVG viewBox scales)
    var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="' + W + '" height="' + H + '"><foreignObject x="0" y="0" width="' + W + '" height="' + H + '">' + xml + "</foreignObject></svg>";
    var img = new Image();
    img.width = W; img.height = H;
    img.src = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg);
    return img.decode().then(function () {
      if (!canvas || canvas.width !== w || canvas.height !== h) {
        canvas = typeof OffscreenCanvas !== "undefined" ? new OffscreenCanvas(w, h) : Object.assign(document.createElement("canvas"), { width: w, height: h });
        ctx = canvas.getContext("2d");
      }
      ctx.clearRect(0, 0, w, h);
      ctx.imageSmoothingQuality = "high";
      ctx.drawImage(img, 0, 0, w, h);
      return createImageBitmap(canvas);
    });
  }

  // ---------------------------------------------------------------- messages
  window.addEventListener("message", function (e) {
    if (e.source !== parent) return;
    var m = e.data || {};
    if (m.t === "load") {
      var r;
      try { r = load(m.doc); } catch (err) { r = { errors: { "": (err && err.message) || String(err) }, total: total() }; }
      post({ t: "loaded", seq: m.seq, errors: r.errors, total: r.total });
    } else if (m.t === "seek") {
      show(+m.time || 0);
    } else if (m.t === "capture") {
      capture(+m.time || 0, m.w, m.h).then(function (bmp) { post({ t: "frame", id: m.id, bitmap: bmp }, [bmp]); },
        function (err) { post({ t: "frame", id: m.id, error: (err && err.message) || String(err) || "draw failed" }); });
    }
  });
  document.addEventListener("click", function (e) {
    if (mode !== "preview") return;
    var el = e.target && e.target.closest ? e.target : null;
    if (!el) return;
    var f = el.closest("[data-field]"), s = el.closest("[data-scene]");
    post({ t: "pick", scene: s ? s.getAttribute("data-scene") : "", field: f ? f.getAttribute("data-field") : "" });
  });
  window.onerror = function (msg) { post({ t: "error", scene: "", message: String(msg) }); };
  post({ t: "ready" });
})();
