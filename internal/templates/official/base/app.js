/* 多维表格: one set of records, several ways to look at them. Its fields are
 * in the base record (main); each record is {values: {fieldId: value},
 * order}. Views: table, board (grouped by a select field; drag to move),
 * calendar (by a date field), gallery, and a form to add records. */
(function () {
  "use strict";
  var el = kit.el;
  var TYPES = [["text", "文本"], ["long", "多行文本"], ["number", "数字"], ["date", "日期"], ["select", "单选"], ["multi", "多选"], ["check", "勾选"], ["person", "人"], ["url", "链接"]];
  var DEFAULT = { title: "多维表格", fields: [
    { id: "f1", name: "标题", type: "text" }, { id: "f2", name: "状态", type: "select", options: ["待办", "进行中", "完成"] },
    { id: "f3", name: "负责人", type: "person" }, { id: "f4", name: "日期", type: "date" },
  ] };
  var VIEWS = [["grid", "表格"], ["board", "看板"], ["calendar", "日历"], ["gallery", "画廊"], ["form", "表单"]];

  var base = kit.store("base"), recs = kit.store("records");
  var app = document.getElementById("app");
  var me = "", query = "", flash = "";
  var pref = (function () { try { return JSON.parse(localStorage.getItem("myco-base-view") || "{}"); } catch (e) { return {}; } })();
  var view = pref.view || "grid";
  var month = new Date(); month.setDate(1);
  var savePref = function () { try { localStorage.setItem("myco-base-view", JSON.stringify(pref)); } catch (e) { /* not kept */ } };

  var meta = function () { var m = base.get("main"); return m ? m.data : DEFAULT; };
  var fields = function () { return (meta().fields || []).filter(function (f) { return f && f.id; }); };
  var fieldOf = function (id) { return fields().filter(function (f) { return f.id === id; })[0]; };
  var titleField = function () { return fields()[0]; };
  var titleOf = function (r) { var t = titleField(); var v = t && (r.data.values || {})[t.id]; return v ? String(v) : "（未命名）"; };
  var saveMeta = function (next) {
    var m = meta();
    return base.put({ title: next.title !== undefined ? next.title : m.title, fields: next.fields || m.fields }, "main")
      .then(function () { render(); }, function (e) { kit.note(e.code === "conflict" ? "别人刚改过字段，已显示最新的" : e.message); render(); });
  };
  var setValue = function (id, field, value) {
    var cur = recs.get(id), values = {};
    var old = (cur && cur.data.values) || {};
    for (var k in old) values[k] = old[k];
    if (value === null || value === undefined || value === "" || (Array.isArray(value) && !value.length)) delete values[field.id]; else values[field.id] = value;
    return recs.patch(id, { values: values }).catch(function (e) { kit.note(e.code === "conflict" ? "别人刚改过这条，已显示最新的" : e.message); render(); });
  };
  var shown = function () {
    if (!query) return recs.rows.slice();
    var q = query.toLowerCase();
    return recs.rows.filter(function (r) { var v = r.data.values || {}; return Object.keys(v).some(function (k) { return String(v[k]).toLowerCase().indexOf(q) >= 0; }); });
  };
  var newRecord = function (values) {
    return recs.put({ values: values || {}, order: recs.next() }).then(function (r) { flash = r.id; render(); return r; }, function (e) { kit.note(e.message); });
  };

  // ---- an editor for one field of one record ----------------------------
  var editor = function (r, f, cls) {
    var v = (r.data.values || {})[f.id], input;
    var commit = function (val) { setValue(r.id, f, val); };
    if (f.type === "check") {
      input = el("input", { type: "checkbox", class: cls || "", checked: !!v, onchange: function () { commit(input.checked); } });
    } else if (f.type === "select") {
      input = el("select", { class: cls || "field", onchange: function () { commit(input.value || null); } },
        el("option", { value: "", text: "" }), (f.options || []).map(function (o) { return el("option", { value: o, text: o }); }));
      input.value = v || "";
    } else if (f.type === "multi") {
      var have = Array.isArray(v) ? v : [];
      input = el("div", { class: "b-multi" }, (f.options || []).map(function (o) {
        var c = el("input", { type: "checkbox", checked: have.indexOf(o) >= 0 });
        c.addEventListener("change", function () {
          var next = (f.options || []).filter(function (x) { return x === o ? c.checked : have.indexOf(x) >= 0; });
          have = next;
          commit(next);
        });
        return el("label", null, c, o);
      }));
    } else if (f.type === "long") {
      input = el("textarea", { class: cls || "field", rows: 3, onchange: function () { commit(input.value || null); } });
      input.value = v || "";
    } else {
      input = el("input", { class: cls || "field", type: f.type === "number" ? "number" : f.type === "date" ? "date" : f.type === "url" ? "url" : "text",
        onchange: function () { commit(f.type === "number" ? (input.value === "" ? null : Number(input.value)) : input.value || null); } });
      input.value = v === undefined || v === null ? "" : v;
    }
    if (input.dataset) { input.dataset.row = r.id; input.dataset.col = f.id; }
    input.setAttribute && input.setAttribute("aria-label", f.name);
    return input;
  };
  var display = function (f, v) {
    if (v === undefined || v === null || v === "") return null;
    if (f.type === "select") return el("span", { class: kit.tagClass(f.options, v), text: v });
    if (f.type === "multi") return el("span", null, (Array.isArray(v) ? v : []).map(function (x) { return el("span", { class: kit.tagClass(f.options, x), text: x, style: "margin-right:4px" }); }));
    if (f.type === "check") return el("span", { text: v ? "✓" : "" });
    return el("span", { text: String(v) });
  };

  // ---- a record opened on the side ---------------------------------------
  var openRecord = function (id) {
    closeRecord();
    var r = recs.get(id);
    if (!r) return;
    var bg = el("div", { class: "b-drawer-bg", onclick: closeRecord });
    var d = el("div", { class: "b-drawer", role: "dialog", "aria-label": titleOf(r) },
      el("h2", null, el("span", { text: titleOf(r) }),
        el("button", { type: "button", class: "ghost", text: "删除", onclick: function () {
          if (!confirm("删除这条记录？")) return;
          closeRecord();
          recs.remove(id).then(render, function (e) { kit.note(e.message); render(); });
          render();
        } }),
        el("button", { type: "button", class: "ghost", text: "关闭", onclick: closeRecord })),
      el("div", { class: "b-form" }, fields().map(function (f) { return el("div", { class: "row" }, el("label", { text: f.name }), editor(r, f)); })));
    d.dataset.record = id;
    document.body.appendChild(bg);
    document.body.appendChild(d);
    var k = function (e) { if (e.key === "Escape") closeRecord(); };
    document.addEventListener("keydown", k);
    d._off = function () { document.removeEventListener("keydown", k); };
  };
  var closeRecord = function () {
    var d = document.querySelector(".b-drawer");
    if (d) { if (d._off) d._off(); d.remove(); }
    var bg = document.querySelector(".b-drawer-bg");
    if (bg) bg.remove();
  };

  // ---- fields ------------------------------------------------------------
  var fieldMenu = function (btn, f) {
    var name = el("input", { class: "field", value: f.name, "aria-label": "字段名" });
    var type = el("select", { class: "field", "aria-label": "类型" }, TYPES.map(function (t) { return el("option", { value: t[0], text: t[1] }); }));
    type.value = f.type;
    var opts = el("textarea", { class: "field", rows: 2, placeholder: "选项，用逗号或换行隔开", "aria-label": "选项" });
    opts.value = (f.options || []).join("，");
    var apply = function () {
      saveMeta({ fields: fields().map(function (x) {
        if (x.id !== f.id) return x;
        var n = { id: x.id, name: name.value.trim() || x.name, type: type.value };
        if (n.type === "select" || n.type === "multi") n.options = opts.value.split(/[,，\n]/).map(function (s) { return s.trim(); }).filter(Boolean);
        return n;
      }) });
    };
    name.addEventListener("change", apply);
    type.addEventListener("change", function () { if ((type.value === "select" || type.value === "multi") && !opts.value.trim()) opts.value = "选项一，选项二"; apply(); });
    opts.addEventListener("change", apply);
    kit.menu(btn, [{ label: "字段名", node: name }, { label: "类型", node: type }, { label: "选项（单选、多选）", node: opts }, "sep",
      fields()[0] && fields()[0].id === f.id ? null : { label: "删除这个字段", danger: true, onClick: function () {
        if (!confirm("删除字段「" + f.name + "」？")) return;
        saveMeta({ fields: fields().filter(function (x) { return x.id !== f.id; }) });
      } }].filter(Boolean));
  };
  var addField = function () {
    var list = fields().slice();
    list.push({ id: kit.uid("f"), name: "新字段 " + (list.length + 1), type: "text" });
    saveMeta({ fields: list }).then(function () {
      var hs = app.querySelectorAll("th.k-colhead button");
      if (hs.length) fieldMenu(hs[hs.length - 1], fields()[fields().length - 1]);
    });
  };

  // ---- views -------------------------------------------------------------
  var grid = function () {
    var fs = fields(), list = shown();
    return el("div", null,
      el("div", { class: "k-grid-wrap" }, el("table", { class: "k-grid" },
        el("thead", null, el("tr", null, el("th", { class: "k-num", text: "#" }), fs.map(function (f) {
          return el("th", { class: "k-colhead" }, el("button", { type: "button", text: f.name, onclick: function (e) { fieldMenu(e.currentTarget, f); } }));
        }), el("th", null, el("button", { type: "button", text: "+ 字段", onclick: addField })))),
        el("tbody", null, list.map(function (r, i) {
          return el("tr", { class: r.id === flash ? "k-flash" : "" },
            el("td", { class: "k-num" }, el("span", { class: "k-rownum", text: String(i + 1) }),
              el("button", { type: "button", class: "k-rowdel", text: "↗", title: "打开", "aria-label": "打开这条", onclick: function () { openRecord(r.id); } })),
            fs.map(function (f) {
              if (f.type === "multi") return el("td", null, el("button", { type: "button", class: "k-cell", style: "text-align:left;background:none", onclick: function () { openRecord(r.id); } }, display(f, (r.data.values || {})[f.id]) || ""));
              return el("td", { class: f.type === "check" ? "k-check" : "" }, editor(r, f, "k-cell" + (f.type === "text" ? " k-wide" : f.type === "number" || f.type === "date" || f.type === "select" ? " k-short" : "")));
            }), el("td"));
        })))),
      list.length ? null : el("div", { class: "k-empty", text: query ? "没有找到「" + query + "」" : "还没有记录：点「+ 记录」，或者让 agent 来填。" }),
      el("button", { type: "button", class: "ghost k-add-row", text: "+ 新的一条", onclick: function () { newRecord(); } }));
  };

  var card = function (r, skip) {
    var fs = fields().slice(1).filter(function (f) { return f.id !== skip; });
    var rowsOf = fs.map(function (f) { var d = display(f, (r.data.values || {})[f.id]); return d ? el("div", { class: "b-card-row" }, el("span", { class: "k", text: f.name }), d) : null; }).filter(Boolean).slice(0, 4);
    var c = el("div", { class: "b-card" + (r.id === flash ? " k-flash" : ""), tabindex: "0", role: "button", onclick: function () { openRecord(r.id); },
      onkeydown: function (e) { if (e.key === "Enter") openRecord(r.id); } }, el("div", { class: "b-card-title", text: titleOf(r) }), rowsOf);
    c.dataset.id = r.id;
    return c;
  };
  var pickField = function (kind, label, key) {
    var fs = fields().filter(function (f) { return kind.indexOf(f.type) >= 0; });
    if (!fs.length) return { field: null, node: el("span", { text: "先加一个" + (kind[0] === "date" ? "日期" : "单选") + "字段" }) };
    // (a status-like field first: 状态, 阶段, 进度)
    var likely = fs.filter(function (f) { return /状态|阶段|进度|status|stage/i.test(f.name); })[0];
    var cur = fieldOf(pref[key]) && kind.indexOf(fieldOf(pref[key]).type) >= 0 ? fieldOf(pref[key]) : likely || fs[0];
    var sel = el("select", { class: "field", "aria-label": label, onchange: function () { pref[key] = sel.value; savePref(); render(); } },
      fs.map(function (f) { return el("option", { value: f.id, text: f.name }); }));
    sel.value = cur.id;
    return { field: cur, node: el("span", null, label + " ", sel) };
  };

  var board = function () {
    var p = pickField(["select"], "按", "group");
    var wrap = el("div", null, el("div", { class: "b-bar", style: "padding: 0 0 10px" }, p.node, p.field ? " 分组，拖动卡片改「" + p.field.name + "」" : ""));
    if (!p.field) return wrap;
    var f = p.field, groups = (f.options || []).concat([""]);
    var list = shown();
    var cols = groups.map(function (g) {
      var mine = list.filter(function (r) { var v = (r.data.values || {})[f.id] || ""; return g === "" ? (f.options || []).indexOf(v) < 0 : v === g; });
      if (g === "" && !mine.length) return null;
      var col = el("div", { class: "b-col" },
        el("div", { class: "b-col-head" }, g ? el("span", { class: kit.tagClass(f.options, g), text: g }) : el("span", { text: "未分组" }), el("span", { class: "n", text: String(mine.length) })),
        mine.map(function (r) {
          var c = card(r, f.id);
          c.draggable = true;
          c.addEventListener("dragstart", function (e) { e.dataTransfer.setData("text/plain", r.id); c.classList.add("dragging"); });
          c.addEventListener("dragend", function () { c.classList.remove("dragging"); });
          return c;
        }),
        el("button", { type: "button", class: "ghost", text: "+ 新建", onclick: function () { var v = {}; if (g) v[f.id] = g; newRecord(v).then(function (r) { if (r) openRecord(r.id); }); } }));
      col.addEventListener("dragover", function (e) { e.preventDefault(); col.classList.add("over"); });
      col.addEventListener("dragleave", function () { col.classList.remove("over"); });
      col.addEventListener("drop", function (e) {
        e.preventDefault();
        col.classList.remove("over");
        var id = e.dataTransfer.getData("text/plain");
        if (id) { flash = id; setValue(id, f, g || null).then(render); }
      });
      return col;
    }).filter(Boolean);
    wrap.appendChild(el("div", { class: "b-board" }, cols));
    return wrap;
  };

  var ymd = function (d) { return d.getFullYear() + "-" + String(d.getMonth() + 1).padStart(2, "0") + "-" + String(d.getDate()).padStart(2, "0"); };
  var calendar = function () {
    var p = pickField(["date"], "按", "date");
    var wrap = el("div", null, el("div", { class: "b-bar", style: "padding: 0 0 10px" }, p.node));
    if (!p.field) return wrap;
    var f = p.field, list = shown();
    var first = new Date(month.getFullYear(), month.getMonth(), 1);
    var start = new Date(first); start.setDate(1 - ((first.getDay() + 6) % 7));
    var today = ymd(new Date());
    wrap.appendChild(el("div", { class: "b-cal-head" },
      el("button", { type: "button", text: "‹", "aria-label": "上个月", onclick: function () { month.setMonth(month.getMonth() - 1); render(); } }),
      el("b", { text: month.getFullYear() + " 年 " + (month.getMonth() + 1) + " 月" }),
      el("button", { type: "button", text: "›", "aria-label": "下个月", onclick: function () { month.setMonth(month.getMonth() + 1); render(); } }),
      el("button", { type: "button", class: "ghost", text: "今天", onclick: function () { month = new Date(); month.setDate(1); render(); } })));
    var cells = ["一", "二", "三", "四", "五", "六", "日"].map(function (d) { return el("div", { class: "dow", text: "周" + d }); });
    for (var i = 0; i < 42; i++) {
      var day = new Date(start); day.setDate(start.getDate() + i);
      var key = ymd(day);
      var evs = list.filter(function (r) { return (r.data.values || {})[f.id] === key; });
      (function (key, evs, out) {
        cells.push(el("div", { class: "b-day" + (out ? " out" : "") + (key === today ? " today" : "") },
          el("span", { class: "d", text: String(Number(key.slice(8))) }),
          el("button", { type: "button", class: "ghost add", text: "+", title: "这天新建一条", onclick: function () { var v = {}; v[f.id] = key; newRecord(v).then(function (r) { if (r) openRecord(r.id); }); } }),
          evs.map(function (r) { return el("button", { type: "button", class: "b-ev", text: titleOf(r), onclick: function () { openRecord(r.id); } }); })));
      })(key, evs, day.getMonth() !== month.getMonth());
    }
    wrap.appendChild(el("div", { class: "b-cal" }, cells));
    var none = list.filter(function (r) { return !(r.data.values || {})[f.id]; }).length;
    if (none) wrap.appendChild(el("div", { class: "b-bar", style: "padding: 8px 0 0", text: none + " 条没有「" + f.name + "」，在表格里能看到" }));
    return wrap;
  };

  var gallery = function () {
    var list = shown();
    return el("div", null, list.length ? el("div", { class: "b-gallery" }, list.map(function (r) { return card(r); })) : el("div", { class: "k-empty", text: "还没有记录。" }));
  };

  var form = function () {
    var draft = { id: "", data: { values: {} } };
    var values = {};
    var box = el("div", { class: "b-form" }, el("p", { style: "color: var(--muted); margin-top: 0", text: "填好点「提交」，就是一条新记录。" }));
    fields().forEach(function (f) {
      var input;
      if (f.type === "select") input = el("select", { class: "field" }, el("option", { value: "", text: "" }), (f.options || []).map(function (o) { return el("option", { value: o, text: o }); }));
      else if (f.type === "long") input = el("textarea", { class: "field", rows: 3 });
      else if (f.type === "check") input = el("input", { type: "checkbox" });
      else if (f.type === "multi") {
        input = el("div", { class: "b-multi" }, (f.options || []).map(function (o) { return el("label", null, el("input", { type: "checkbox", value: o }), o); }));
      } else input = el("input", { class: "field", type: f.type === "number" ? "number" : f.type === "date" ? "date" : "text" });
      values[f.id] = input;
      box.appendChild(el("div", { class: "row" }, el("label", { text: f.name }), input));
    });
    box.appendChild(el("button", { type: "button", class: "primary", text: "提交", onclick: function () {
      var v = {};
      fields().forEach(function (f) {
        var i = values[f.id];
        var x = f.type === "check" ? i.checked : f.type === "multi" ? Array.prototype.filter.call(i.querySelectorAll("input"), function (c) { return c.checked; }).map(function (c) { return c.value; })
          : f.type === "number" ? (i.value === "" ? null : Number(i.value)) : i.value;
        if (x !== null && x !== "" && x !== false && !(Array.isArray(x) && !x.length)) v[f.id] = x;
      });
      if (!Object.keys(v).length) { kit.note("还什么都没填"); return; }
      newRecord(v).then(function (r) { if (r) { kit.note("已提交"); view = "form"; render(); } });
    } }));
    void draft;
    return box;
  };

  // ---- export --------------------------------------------------------------
  var exportRows = function () {
    var fs = fields();
    return [fs.map(function (f) { return f.name; })].concat(shown().map(function (r) {
      return fs.map(function (f) { var v = (r.data.values || {})[f.id]; return Array.isArray(v) ? v.join("、") : v === undefined ? null : v; });
    }));
  };

  var render = function () {
    var a = document.activeElement, keep = a && a.dataset && a.dataset.row && !a.closest(".b-drawer") ? { row: a.dataset.row, col: a.dataset.col, value: a.value } : null;
    var open = document.querySelector(".b-drawer");
    app.innerHTML = "";
    var m = meta();
    var title = el("input", { class: "k-title", value: m.title || "多维表格", "aria-label": "名称", onchange: function () { saveMeta({ title: title.value.trim() || "多维表格" }); } });
    var search = el("input", { class: "field k-search", type: "search", placeholder: "搜索", value: query, "aria-label": "搜索",
      oninput: kit.debounce(function () { query = search.value.trim(); render(); var s = app.querySelector(".k-search"); if (s) { s.focus(); s.setSelectionRange(s.value.length, s.value.length); } }, 200) });
    var t = m.title || "多维表格";
    app.appendChild(el("div", { class: "k-top" }, title, me ? el("span", { class: "k-who", text: "你是 " + me }) : null, search,
      el("button", { type: "button", text: "导出", onclick: function (e) {
        kit.menu(e.currentTarget, [
          { label: "导出 Excel（.xlsx）", onClick: function () { kit.save(t + ".xlsx", kit.xlsx(t, exportRows()), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); } },
          { label: "导出 CSV", onClick: function () { kit.save(t + ".csv", kit.csv(exportRows()), "text/csv"); } },
        ]);
      } }),
      el("button", { type: "button", class: "primary", text: "+ 记录", onclick: function () { newRecord().then(function (r) { if (r && view !== "grid") openRecord(r.id); }); } })));
    app.appendChild(el("div", { class: "k-tabs", role: "tablist" }, VIEWS.map(function (v) {
      return el("button", { type: "button", role: "tab", class: "k-tab" + (view === v[0] ? " on" : ""), "aria-selected": view === v[0] ? "true" : "false", text: v[1],
        onclick: function () { view = v[0]; pref.view = view; savePref(); render(); } });
    })));
    var body = view === "board" ? board() : view === "calendar" ? calendar() : view === "gallery" ? gallery() : view === "form" ? form() : grid();
    app.appendChild(el("div", { class: "k-body" }, body));
    flash = "";
    if (keep) {
      var back = app.querySelector('[data-row="' + keep.row + '"][data-col="' + keep.col + '"]');
      if (back) { if (back.type !== "checkbox") back.value = keep.value; back.focus(); }
    }
    if (open && open.dataset.record && recs.get(open.dataset.record)) {
      var focusIn = open.contains(document.activeElement);
      if (!focusIn) openRecord(open.dataset.record);
    } else if (open) closeRecord();
  };

  kit.start(function (ctx) {
    me = ctx.me && ctx.me.name || "";
    return Promise.all([base.load(), recs.load()]).then(function () {
      if (!base.get("main")) return base.put(DEFAULT, "main").catch(function () { return base.load(); });
    }).then(function () {
      render();
      kit.listen([base, recs], function (ch) {
        if (ch.collection === "records") flash = ch.record;
        render();
        if (ch.by && ch.by !== me) kit.note(ch.by + " 刚改了");
      });
    });
  });
})();
