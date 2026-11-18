/* 表格: a table people and agents fill together. Its columns are in the
 * sheet record (main); each row is a record of rows: {cells: {colId: value},
 * order}. Sorting and searching are this page's own (not saved). */
(function () {
  "use strict";
  var el = kit.el;
  var TYPES = [["text", "文本"], ["long", "多行文本"], ["number", "数字"], ["date", "日期"], ["select", "单选"], ["check", "勾选"], ["person", "人"], ["url", "链接"]];
  var typeName = function (t) { for (var i = 0; i < TYPES.length; i++) if (TYPES[i][0] === t) return TYPES[i][1]; return t; };
  var DEFAULT = { title: "表格", columns: [
    { id: "c1", name: "名称", type: "text" }, { id: "c2", name: "负责人", type: "person" },
    { id: "c3", name: "状态", type: "select", options: ["待办", "进行中", "完成"] }, { id: "c4", name: "截止", type: "date" },
  ] };

  var sheet = kit.store("sheet"), rows = kit.store("rows");
  var sortBy = null, query = "", me = "";
  var app = document.getElementById("app");

  var meta = function () { var m = sheet.get("main"); return m ? m.data : DEFAULT; };
  var cols = function () { return (meta().columns || []).filter(function (c) { return c && c.id; }); };
  var saveMeta = function (next) {
    var m = meta(), data = { title: next.title !== undefined ? next.title : m.title, columns: next.columns || m.columns };
    return sheet.put(data, "main").then(render, function (e) { kit.note(e.code === "conflict" ? "别人刚改过表头，已显示最新的" : e.message); render(); });
  };

  var shown = function () {
    var list = rows.rows.slice();
    if (query) {
      var q = query.toLowerCase();
      list = list.filter(function (r) { var c = r.data.cells || {}; return Object.keys(c).some(function (k) { return String(c[k]).toLowerCase().indexOf(q) >= 0; }); });
    }
    if (sortBy) {
      var col = cols().filter(function (c) { return c.id === sortBy.col; })[0];
      list.sort(function (a, b) {
        var x = (a.data.cells || {})[sortBy.col], y = (b.data.cells || {})[sortBy.col];
        if (x === undefined || x === null || x === "") return 1;
        if (y === undefined || y === null || y === "") return -1;
        var d = col && col.type === "number" ? x - y : col && col.type === "select" ? (col.options || []).indexOf(x) - (col.options || []).indexOf(y) : String(x).localeCompare(String(y), "zh");
        return sortBy.dir * d;
      });
    }
    return list;
  };

  var valueOf = function (col, input) {
    if (col.type === "check") return input.checked;
    var v = input.value;
    if (v === "") return null;
    if (col.type === "number") { var n = Number(v); return isFinite(n) ? n : null; }
    return v;
  };
  var setCell = function (row, col, value) {
    var cells = {};
    var cur = rows.get(row.id);
    var old = (cur && cur.data.cells) || {};
    for (var k in old) cells[k] = old[k];
    if (value === null || value === undefined || value === "") delete cells[col.id]; else cells[col.id] = value;
    rows.patch(row.id, { cells: cells }).catch(function (e) { kit.note(e.code === "conflict" ? "别人刚改过这一行，已显示最新的" : e.message); render(); });
  };

  var editor = function (row, col) {
    var v = (row.data.cells || {})[col.id];
    var input;
    if (col.type === "check") input = el("input", { type: "checkbox", class: "k-cell", checked: !!v });
    else if (col.type === "select") {
      input = el("select", { class: "k-cell k-short" }, el("option", { value: "", text: "" }), (col.options || []).map(function (o) { return el("option", { value: o, text: o }); }));
      input.value = v || "";
    } else if (col.type === "long") input = el("textarea", { class: "k-cell k-wide", rows: 1 });
    else input = el("input", { class: "k-cell" + (col.type === "number" || col.type === "date" ? " k-short" : col.type === "text" ? " k-wide" : ""),
      type: col.type === "number" ? "number" : col.type === "date" ? "date" : col.type === "url" ? "url" : "text" });
    if (col.type !== "check" && col.type !== "select") input.value = v === undefined || v === null ? "" : v;
    input.dataset.row = row.id;
    input.dataset.col = col.id;
    input.setAttribute("aria-label", col.name);
    input.addEventListener("change", function () { setCell(row, col, valueOf(col, input)); });
    if (col.type !== "long") input.addEventListener("keydown", function (e) {
      if (e.key !== "Enter" || e.isComposing) return;
      e.preventDefault();
      var next = app.querySelector('[data-row="' + (nextRow(row.id) || "") + '"][data-col="' + col.id + '"]');
      input.blur();
      if (next) next.focus();
    });
    return input;
  };
  var nextRow = function (id) { var l = shown(); for (var i = 0; i < l.length - 1; i++) if (l[i].id === id) return l[i + 1].id; return null; };

  var colMenu = function (btn, col) {
    var name = el("input", { class: "field", value: col.name, "aria-label": "列名" });
    var type = el("select", { class: "field", "aria-label": "类型" }, TYPES.map(function (t) { return el("option", { value: t[0], text: t[1] }); }));
    type.value = col.type;
    var opts = el("textarea", { class: "field", rows: 2, placeholder: "选项，用逗号或换行隔开", "aria-label": "选项" });
    opts.value = (col.options || []).join("，");
    var apply = function () {
      var list = cols().map(function (c) {
        if (c.id !== col.id) return c;
        var n = { id: c.id, name: name.value.trim() || c.name, type: type.value };
        if (n.type === "select") n.options = opts.value.split(/[,，\n]/).map(function (s) { return s.trim(); }).filter(Boolean);
        return n;
      });
      saveMeta({ columns: list });
    };
    name.addEventListener("change", apply);
    type.addEventListener("change", function () { if (type.value === "select" && !opts.value.trim()) opts.value = "待办，进行中，完成"; apply(); });
    opts.addEventListener("change", apply);
    kit.menu(btn, [
      { label: "列名", node: name }, { label: "类型", node: type }, { label: "单选的选项", node: opts },
      "sep",
      { label: "升序排列", onClick: function () { sortBy = { col: col.id, dir: 1 }; render(); } },
      { label: "降序排列", onClick: function () { sortBy = { col: col.id, dir: -1 }; render(); } },
      sortBy && sortBy.col === col.id ? { label: "不排序", onClick: function () { sortBy = null; render(); } } : null,
      "sep",
      { label: "删除这一列", danger: true, onClick: function () {
        if (!confirm("删除「" + col.name + "」这一列？各行这一格的内容也会看不到。")) return;
        saveMeta({ columns: cols().filter(function (c) { return c.id !== col.id; }) });
      } },
    ].filter(Boolean));
  };

  var addColumn = function () {
    var list = cols().slice();
    list.push({ id: kit.uid("c"), name: "新列 " + (list.length + 1), type: "text" });
    saveMeta({ columns: list }).then(function () {
      var hs = app.querySelectorAll("th.k-colhead button");
      if (hs.length) colMenu(hs[hs.length - 1], cols()[cols().length - 1]);
    });
  };
  var addRow = function () {
    rows.put({ cells: {}, order: rows.next() }).then(function (r) {
      render(r.id);
      var first = app.querySelector('[data-row="' + r.id + '"]');
      if (first) first.focus();
    }, function (e) { kit.note(e.message); });
  };

  var exportRows = function () {
    var cs = cols();
    return [cs.map(function (c) { return c.name; })].concat(shown().map(function (r) {
      return cs.map(function (c) { var v = (r.data.cells || {})[c.id]; return v === undefined ? null : v; });
    }));
  };
  var exportMenu = function (btn) {
    var title = meta().title || "表格";
    kit.menu(btn, [
      { label: "导出 Excel（.xlsx）", onClick: function () { kit.save(title + ".xlsx", kit.xlsx(title, exportRows()), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); } },
      { label: "导出 CSV", onClick: function () { kit.save(title + ".csv", kit.csv(exportRows()), "text/csv"); } },
    ]);
  };

  var render = function (flash) {
    // Keep what is being typed where it is.
    var a = document.activeElement, keep = a && a.dataset && a.dataset.row ? { row: a.dataset.row, col: a.dataset.col, value: a.value, checked: a.checked } : null;
    app.innerHTML = "";
    var m = meta(), cs = cols();
    var title = el("input", { class: "k-title", value: m.title || "表格", "aria-label": "表格名称", onchange: function () { saveMeta({ title: title.value.trim() || "表格" }); } });
    var search = el("input", { class: "field k-search", type: "search", placeholder: "搜索", value: query, "aria-label": "搜索",
      oninput: kit.debounce(function () { query = search.value.trim(); render(); var s = app.querySelector(".k-search"); if (s) { s.focus(); s.setSelectionRange(s.value.length, s.value.length); } }, 200) });
    app.appendChild(el("div", { class: "k-top" }, title, me ? el("span", { class: "k-who", text: "你是 " + me }) : null, search,
      el("button", { type: "button", text: "导出", onclick: function (e) { exportMenu(e.currentTarget); } }),
      el("button", { type: "button", text: "+ 列", onclick: addColumn }),
      el("button", { type: "button", class: "primary", text: "+ 行", onclick: addRow })));
    var list = shown();
    var head = el("tr", null, el("th", { class: "k-num", text: "#" }), cs.map(function (c) {
      var arrow = sortBy && sortBy.col === c.id ? (sortBy.dir > 0 ? " ↑" : " ↓") : "";
      return el("th", { class: "k-colhead", title: typeName(c.type) }, el("button", { type: "button", text: c.name + arrow, onclick: function (e) { colMenu(e.currentTarget, c); } }));
    }));
    var body = list.map(function (r, i) {
      var tr = el("tr", { class: r.id === flash ? "k-flash" : "" },
        el("td", { class: "k-num" }, el("span", { class: "k-rownum", text: String(i + 1) }),
          el("button", { type: "button", class: "k-rowdel", title: "删除这一行", "aria-label": "删除这一行", text: "×",
            onclick: function () { rows.remove(r.id).then(function () { render(); }, function (e) { kit.note(e.message); render(); }); render(); } })),
        cs.map(function (c) { return el("td", { class: c.type === "check" ? "k-check" : "" }, editor(r, c)); }));
      return tr;
    });
    var body2 = el("div", { class: "k-body" },
      el("div", { class: "k-grid-wrap" }, el("table", { class: "k-grid" }, el("thead", null, head), el("tbody", null, body))),
      list.length ? null : el("div", { class: "k-empty", text: query ? "没有找到「" + query + "」" : "还没有内容：点「+ 行」，或者让 agent 来填。" }),
      el("button", { type: "button", class: "ghost k-add-row", text: "+ 新的一行", onclick: addRow }));
    app.appendChild(body2);
    if (keep) {
      var back = app.querySelector('[data-row="' + keep.row + '"][data-col="' + keep.col + '"]');
      if (back) { if (back.type === "checkbox") back.checked = keep.checked; else back.value = keep.value; back.focus(); }
    }
  };

  kit.start(function (ctx) {
    me = ctx.me && ctx.me.name || "";
    return Promise.all([sheet.load(), rows.load()]).then(function () {
      if (!sheet.get("main")) return sheet.put(DEFAULT, "main").catch(function () { return sheet.load(); });
    }).then(function () {
      render();
      kit.listen([sheet, rows], function (ch) {
        render(ch.collection === "rows" ? ch.record : undefined);
        if (ch.by && ch.by !== me) kit.note(ch.by + " 刚改了");
      });
    });
  });
})();
