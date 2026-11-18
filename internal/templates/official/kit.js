/* myco official templates: what they share. The page's data are records
 * (myco.data, through the myco bridge in the App); kit.store keeps one
 * collection here, in order, with what others change applied as it comes.
 * kit.csv and kit.xlsx make exports; kit.save hands them to the person's
 * Downloads (myco.download). No outside libraries: pages run offline. */
(function () {
  "use strict";
  var kit = {};

  kit.el = function (tag, attrs) {
    var e = document.createElement(tag);
    if (attrs) for (var k in attrs) {
      var v = attrs[k];
      if (v === undefined || v === null || v === false) continue;
      if (k === "class") e.className = v;
      else if (k === "text") e.textContent = v;
      else if (k === "html") e.innerHTML = v;
      else if (k.slice(0, 2) === "on") e.addEventListener(k.slice(2), v);
      else if (k === "style") e.style.cssText = v;
      else if (k in e && typeof v !== "string") e[k] = v;
      else e.setAttribute(k, v === true ? "" : v);
    }
    for (var i = 2; i < arguments.length; i++) {
      var c = arguments[i];
      if (c === null || c === undefined || c === false) continue;
      if (Array.isArray(c)) c.forEach(function (x) { if (x) e.appendChild(typeof x === "string" ? document.createTextNode(x) : x); });
      else e.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
    }
    return e;
  };

  var noteTimer = 0;
  kit.note = function (text) {
    var n = document.querySelector(".k-note");
    if (!n) { n = kit.el("div", { class: "k-note", role: "status" }); document.body.appendChild(n); }
    n.textContent = text;
    n.classList.add("on");
    clearTimeout(noteTimer);
    noteTimer = setTimeout(function () { n.classList.remove("on"); }, 2200);
  };

  kit.uid = function (p) { return (p || "c") + Math.random().toString(36).slice(2, 8); };
  kit.debounce = function (f, ms) {
    var t = 0;
    return function () { var a = arguments, self = this; clearTimeout(t); t = setTimeout(function () { f.apply(self, a); }, ms); };
  };
  kit.tagClass = function (options, value) {
    var i = (options || []).indexOf(value);
    return "k-tag t" + ((i < 0 ? 5 : i) % 6);
  };

  // Starts the page once the App's bridge answers; outside the App it says
  // where to open it.
  kit.start = function (run) {
    if (!window.myco) { kit.offline(); return; }
    myco.ready.then(function (ctx) { return run(ctx); }, function () { kit.offline(); })
      .catch(function (e) { kit.note(e && e.message || String(e)); });
  };
  kit.offline = function () {
    document.body.appendChild(kit.el("div", { class: "k-offline", text: "在 myco App 里打开这个网页，才能读写它的数据。" }));
  };

  // One collection, kept in order (by data.order, then as put).
  kit.store = function (collection) {
    var s = { collection: collection, rows: [], listeners: [] };
    var sort = function () {
      s.rows.sort(function (a, b) {
        var x = a.data.order, y = b.data.order;
        if (typeof x === "number" && typeof y === "number" && x !== y) return x - y;
        return 0;
      });
    };
    s.load = function () {
      return myco.data.list(collection).then(function (r) { s.rows = r.records.slice(); sort(); return s.rows; });
    };
    s.get = function (id) { for (var i = 0; i < s.rows.length; i++) if (s.rows[i].id === id) return s.rows[i]; return null; };
    var upsert = function (rec) {
      var cur = s.get(rec.id);
      if (cur) { if (cur.version <= rec.version) { cur.data = rec.data; cur.version = rec.version; } }
      else s.rows.push({ collection: collection, id: rec.id, data: rec.data, version: rec.version });
      sort();
    };
    // Writes a record (data replaces it; id "" for a new one) based on the
    // version this page has. On a conflict the latest is taken and the
    // error goes on (e.code "conflict").
    s.put = function (data, id) {
      var cur = id ? s.get(id) : null;
      return myco.data.put(collection, data, { id: id || "", ifVersion: cur ? cur.version : (id ? 0 : undefined) })
        .then(function (rec) { upsert(rec); return s.get(rec.id); },
          function (e) { if (e && e.code === "conflict" && e.latest) { upsert(e.latest); fire({ collection: collection, record: e.latest.id, op: "put" }); } throw e; });
    };
    // Changes some fields of a record.
    s.patch = function (id, fields) {
      var cur = s.get(id);
      var next = {};
      for (var k in (cur ? cur.data : {})) next[k] = cur.data[k];
      for (var f in fields) { if (fields[f] === null || fields[f] === undefined) delete next[f]; else next[f] = fields[f]; }
      return s.put(next, id);
    };
    s.remove = function (id) {
      var i = s.rows.findIndex(function (r) { return r.id === id; });
      var gone = i >= 0 ? s.rows.splice(i, 1)[0] : null;
      return myco.data.remove(collection, id).catch(function (e) { if (gone) { s.rows.splice(i, 0, gone); } throw e; });
    };
    // The order a record goes at the end (or between two).
    s.next = function () {
      var m = 0;
      s.rows.forEach(function (r) { if (typeof r.data.order === "number" && r.data.order > m) m = r.data.order; });
      return m + 1;
    };
    s.between = function (a, b) {
      var x = a ? a.data.order || 0 : 0, y = b ? b.data.order || x + 2 : x + 2;
      return (x + y) / 2;
    };
    s.on = function (f) { s.listeners.push(f); };
    var fire = function (ch) { s.listeners.forEach(function (f) { try { f(ch); } catch (e) { setTimeout(function () { throw e; }); } }); };
    // A change from the feed; false when this page has it already (its own
    // write coming back).
    s.apply = function (ch) {
      if (ch.collection !== collection) return false;
      var cur = s.get(ch.record);
      if (ch.op === "delete") {
        if (!cur) return false;
        s.rows.splice(s.rows.indexOf(cur), 1);
      } else {
        if (cur && cur.version >= ch.version) return false;
        upsert({ id: ch.record, data: ch.data || {}, version: ch.version });
      }
      fire(ch);
      return true;
    };
    return s;
  };

  // Listens once for every store: what others changed comes to each.
  kit.listen = function (stores, onAny) {
    myco.data.on(function (ch) {
      var news = false;
      stores.forEach(function (s) { if (s.apply(ch)) news = true; });
      if (news && onAny) onAny(ch);
    });
  };

  // ---- exports --------------------------------------------------------
  kit.csv = function (rows) {
    var esc = function (v) {
      var s = v === null || v === undefined ? "" : String(v);
      return /[",\n\r]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
    };
    return "﻿" + rows.map(function (r) { return r.map(esc).join(","); }).join("\r\n");
  };

  var crcTable = null;
  var crc32 = function (bytes) {
    if (!crcTable) {
      crcTable = new Uint32Array(256);
      for (var n = 0; n < 256; n++) { var c = n; for (var k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; crcTable[n] = c >>> 0; }
    }
    var crc = 0xffffffff;
    for (var i = 0; i < bytes.length; i++) crc = crcTable[(crc ^ bytes[i]) & 0xff] ^ (crc >>> 8);
    return (crc ^ 0xffffffff) >>> 0;
  };
  // A zip of files {name: string}, stored (no compression).
  kit.zip = function (files) {
    var enc = new TextEncoder(), parts = [], central = [], offset = 0;
    var u16 = function (v) { return [v & 255, (v >>> 8) & 255]; };
    var u32 = function (v) { return [v & 255, (v >>> 8) & 255, (v >>> 16) & 255, (v >>> 24) & 255]; };
    Object.keys(files).forEach(function (name) {
      var data = enc.encode(files[name]), nm = enc.encode(name), crc = crc32(data);
      var head = [].concat(u32(0x04034b50), u16(20), u16(0x0800), u16(0), u16(0), u16(0x21), u32(crc), u32(data.length), u32(data.length), u16(nm.length), u16(0));
      parts.push(new Uint8Array(head), nm, data);
      central.push(new Uint8Array([].concat(u32(0x02014b50), u16(20), u16(20), u16(0x0800), u16(0), u16(0), u16(0x21), u32(crc), u32(data.length), u32(data.length),
        u16(nm.length), u16(0), u16(0), u16(0), u16(0), u32(0), u32(offset))), nm);
      offset += head.length + nm.length + data.length;
    });
    var cdSize = central.reduce(function (n, p) { return n + p.length; }, 0);
    var count = Object.keys(files).length;
    var end = new Uint8Array([].concat(u32(0x06054b50), u16(0), u16(0), u16(count), u16(count), u32(cdSize), u32(offset), u16(0)));
    var all = parts.concat(central, [end]);
    var out = new Uint8Array(all.reduce(function (n, p) { return n + p.length; }, 0)), at = 0;
    all.forEach(function (p) { out.set(p, at); at += p.length; });
    return out;
  };
  var xmlEsc = function (s) { return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, ""); };
  var colName = function (i) { var s = ""; i++; while (i > 0) { var m = (i - 1) % 26; s = String.fromCharCode(65 + m) + s; i = Math.floor((i - 1) / 26); } return s; };
  // An Excel workbook (.xlsx) with one sheet: rows of strings, numbers,
  // booleans; the first row bold.
  kit.xlsx = function (sheet, rows) {
    var name = String(sheet || "Sheet1").replace(/[\[\]:*?\/\\]/g, " ").slice(0, 31) || "Sheet1";
    var body = rows.map(function (r, ri) {
      return '<row r="' + (ri + 1) + '">' + r.map(function (v, ci) {
        var ref = colName(ci) + (ri + 1), st = ri === 0 ? ' s="1"' : "";
        if (v === null || v === undefined || v === "") return "";
        if (typeof v === "number" && isFinite(v)) return '<c r="' + ref + '"' + st + "><v>" + v + "</v></c>";
        if (typeof v === "boolean") return '<c r="' + ref + '" t="b"' + st + "><v>" + (v ? 1 : 0) + "</v></c>";
        return '<c r="' + ref + '" t="inlineStr"' + st + '><is><t xml:space="preserve">' + xmlEsc(v) + "</t></is></c>";
      }).join("") + "</row>";
    }).join("");
    var NS = "http://schemas.openxmlformats.org/";
    return kit.zip({
      "[Content_Types].xml": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="' + NS + 'package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>',
      "_rels/.rels": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="' + NS + 'package/2006/relationships"><Relationship Id="rId1" Type="' + NS + 'officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>',
      "xl/workbook.xml": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="' + NS + 'spreadsheetml/2006/main" xmlns:r="' + NS + 'officeDocument/2006/relationships"><sheets><sheet name="' + xmlEsc(name) + '" sheetId="1" r:id="rId1"/></sheets></workbook>',
      "xl/_rels/workbook.xml.rels": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="' + NS + 'package/2006/relationships"><Relationship Id="rId1" Type="' + NS + 'officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="' + NS + 'officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>',
      "xl/styles.xml": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="' + NS + 'spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs></styleSheet>',
      "xl/worksheets/sheet1.xml": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="' + NS + 'spreadsheetml/2006/main"><sheetData>' + body + "</sheetData></worksheet>",
    });
  };

  // Saves an export to the person's Downloads.
  kit.save = function (name, content, type) {
    return myco.download(name, content, type).then(function () { kit.note("已存到「下载」：" + name); }, function (e) { kit.note(e && e.message || "没存下来"); });
  };

  // A small menu at an element (or the pointer); items: {label, onClick,
  // danger} | "sep" | {label, input: el}. Closes on a click outside / Esc.
  kit.menu = function (at, items) {
    kit.closeMenu();
    var m = kit.el("div", { class: "k-menu", role: "menu" });
    items.forEach(function (it) {
      if (it === "sep") { m.appendChild(kit.el("hr")); return; }
      if (it.node) { if (it.label) m.appendChild(kit.el("label", { text: it.label })); m.appendChild(it.node); return; }
      m.appendChild(kit.el("button", { type: "button", role: "menuitem", class: it.danger ? "danger" : "", text: it.label,
        onclick: function () { kit.closeMenu(); it.onClick(); } }));
    });
    document.body.appendChild(m);
    var r = at.getBoundingClientRect ? at.getBoundingClientRect() : { left: at.x, bottom: at.y, top: at.y };
    var x = Math.min(r.left, window.innerWidth - m.offsetWidth - 8), y = r.bottom + 4;
    if (y + m.offsetHeight > window.innerHeight - 8) y = Math.max(8, r.top - m.offsetHeight - 4);
    m.style.left = Math.max(8, x) + "px";
    m.style.top = y + "px";
    setTimeout(function () {
      var close = function (e) { if (!m.contains(e.target)) kit.closeMenu(); };
      var key = function (e) { if (e.key === "Escape") kit.closeMenu(); };
      document.addEventListener("mousedown", close);
      document.addEventListener("keydown", key);
      m._off = function () { document.removeEventListener("mousedown", close); document.removeEventListener("keydown", key); };
    });
    var first = m.querySelector("input, select, textarea, button");
    if (first) first.focus();
    return m;
  };
  kit.closeMenu = function () {
    var m = document.querySelector(".k-menu");
    if (m) { if (m._off) m._off(); m.remove(); }
  };

  window.kit = kit;
})();
