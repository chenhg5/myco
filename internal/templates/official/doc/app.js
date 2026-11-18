/* 文档: a document written together, one block a record (blocks: {type,
 * text, checked, order}); the title in doc/main. Each block saves on its
 * own a moment after typing stops, so two people writing different
 * paragraphs never overwrite each other. Markdown-style shortcuts at the
 * start of a block: # ## ### - 1. [] > ``` --- */
(function () {
  "use strict";
  var el = kit.el;
  var TYPES = [["p", "正文"], ["h1", "标题 1"], ["h2", "标题 2"], ["h3", "标题 3"], ["li", "列表"], ["ol", "编号列表"], ["todo", "待办"], ["quote", "引用"], ["code", "代码"], ["hr", "分隔线"]];
  var PH = { p: "写点什么…（# 标题，- 列表，[] 待办）", h1: "标题 1", h2: "标题 2", h3: "标题 3", li: "列表", ol: "编号列表", todo: "待办", quote: "引用", code: "代码" };
  var SHORT = [[/^#\s/, "h1"], [/^##\s/, "h2"], [/^###\s/, "h3"], [/^[-*]\s/, "li"], [/^1[.)]\s/, "ol"], [/^\[\s?\]\s/, "todo"], [/^>\s/, "quote"], [/^```\s?/, "code"]];

  var docs = kit.store("doc"), blocks = kit.store("blocks");
  var app = document.getElementById("app");
  var me = "", page, list, titleInput, typeSel, focused = null;
  var pending = {};

  var plain = (function () { var d = document.createElement("div"); try { d.contentEditable = "plaintext-only"; return d.contentEditable === "plaintext-only"; } catch (e) { return false; } })();
  var textOf = function (node) { return (node.innerText || "").replace(/\n$/, ""); };
  var caret = function (node) {
    var s = window.getSelection();
    if (!s.rangeCount || !node.contains(s.anchorNode)) return 0;
    var r = s.getRangeAt(0).cloneRange();
    r.selectNodeContents(node);
    r.setEnd(s.anchorNode, s.anchorOffset);
    return r.toString().length;
  };
  var place = function (node, at) {
    node.focus();
    var s = window.getSelection(), r = document.createRange();
    var walk = document.createTreeWalker(node, NodeFilter.SHOW_TEXT), t, left = at;
    while ((t = walk.nextNode())) {
      if (left <= t.length) { r.setStart(t, left); r.collapse(true); s.removeAllRanges(); s.addRange(r); return; }
      left -= t.length;
    }
    r.selectNodeContents(node); r.collapse(false); s.removeAllRanges(); s.addRange(r);
  };

  var save = function (id, fields) {
    clearTimeout(pending[id]);
    delete pending[id];
    return blocks.patch(id, fields).catch(function (e) {
      if (e.code === "conflict") { kit.note("别人刚改过这一段，已换成最新的"); renderBlocks(id); } else kit.note(e.message);
    });
  };
  var saveSoon = function (id, node) {
    clearTimeout(pending[id]);
    pending[id] = setTimeout(function () { delete pending[id]; save(id, { text: textOf(node) }); }, 600);
  };
  var flushAll = function () {
    Object.keys(pending).forEach(function (id) {
      var n = list.querySelector('[data-id="' + id + '"] .d-text');
      if (n) save(id, { text: textOf(n) });
    });
  };

  var addAfter = function (after, type, text) {
    var i = blocks.rows.indexOf(after);
    var order = after ? blocks.between(after, blocks.rows[i + 1]) : blocks.next();
    return blocks.put({ type: type || "p", text: text || "", checked: false, order: order }).then(function (r) {
      renderBlocks();
      var n = list.querySelector('[data-id="' + r.id + '"] .d-text');
      if (n) place(n, 0);
      return r;
    }, function (e) { kit.note(e.message); });
  };
  var removeBlock = function (b, focusPrev) {
    var i = blocks.rows.indexOf(b), prev = blocks.rows[i - 1];
    clearTimeout(pending[b.id]);
    delete pending[b.id];
    blocks.remove(b.id).catch(function (e) { kit.note(e.message); });
    renderBlocks();
    if (focusPrev && prev) {
      var n = list.querySelector('[data-id="' + prev.id + '"] .d-text');
      if (n) place(n, textOf(n).length);
    }
  };
  var setType = function (b, type) {
    if (b.data.type === type) return;
    var patch = { type: type };
    if (type === "todo" && b.data.checked === undefined) patch.checked = false;
    save(b.id, patch).then(function () { renderBlocks(); var n = list.querySelector('[data-id="' + b.id + '"] .d-text'); if (n) place(n, textOf(n).length); });
  };

  var blockMenu = function (btn, b) {
    kit.menu(btn, TYPES.map(function (t) { return { label: (b.data.type === t[0] ? "✓ " : "") + t[1], onClick: function () { setType(b, t[0]); } }; })
      .concat(["sep", { label: "在下面加一段", onClick: function () { addAfter(b, "p", ""); } }, { label: "删除这一段", danger: true, onClick: function () { removeBlock(b, false); } }]));
  };

  var keydown = function (b, node, e) {
    if (e.isComposing) return;
    var type = b.data.type || "p";
    if (e.key === "Enter" && !e.shiftKey && type !== "code") {
      e.preventDefault();
      var text = textOf(node), at = caret(node);
      var listy = type === "li" || type === "ol" || type === "todo";
      if (listy && !text) { setType(b, "p"); return; }
      node.textContent = text.slice(0, at);
      save(b.id, { text: text.slice(0, at) });
      addAfter(b, listy ? type : "p", text.slice(at));
      return;
    }
    if (e.key === "Backspace" && caret(node) === 0 && !window.getSelection().toString()) {
      if (type !== "p") { e.preventDefault(); setType(b, "p"); return; }
      var i = blocks.rows.indexOf(b), prev = blocks.rows[i - 1];
      if (!prev) return;
      e.preventDefault();
      var text2 = textOf(node);
      if (prev.data.type === "hr") { removeBlock(prev, false); place(node, 0); return; }
      var pn = list.querySelector('[data-id="' + prev.id + '"] .d-text');
      var join = pn ? textOf(pn).length : (prev.data.text || "").length;
      var merged = (pn ? textOf(pn) : prev.data.text || "") + text2;
      save(prev.id, { text: merged }).then(function () {
        renderBlocks();
        var n = list.querySelector('[data-id="' + prev.id + '"] .d-text');
        if (n) place(n, join);
      });
      blocks.remove(b.id).catch(function () { /* (gone already) */ });
      return;
    }
    if ((e.key === "ArrowUp" && caret(node) === 0) || (e.key === "ArrowDown" && caret(node) === textOf(node).length)) {
      var j = blocks.rows.indexOf(b) + (e.key === "ArrowUp" ? -1 : 1);
      var other = blocks.rows[j] && list.querySelector('[data-id="' + blocks.rows[j].id + '"] .d-text');
      if (other) { e.preventDefault(); place(other, e.key === "ArrowUp" ? textOf(other).length : 0); }
    }
  };
  var input = function (b, node) {
    var text = textOf(node);
    if ((b.data.type || "p") === "p") {
      if (text === "---") { node.textContent = ""; save(b.id, { type: "hr", text: "" }).then(function () { renderBlocks(); addAfter(blocks.get(b.id), "p", ""); }); return; }
      for (var i = SHORT.length - 1; i >= 0; i--) {
        var m = SHORT[i][0].exec(text);
        if (m) {
          var rest = text.slice(m[0].length);
          node.textContent = rest;
          var patch = { type: SHORT[i][1], text: rest };
          if (patch.type === "todo") patch.checked = false;
          save(b.id, patch).then(function () { renderBlocks(); var n = list.querySelector('[data-id="' + b.id + '"] .d-text'); if (n) place(n, textOf(n).length); });
          return;
        }
      }
    }
    saveSoon(b.id, node);
  };

  var rowOf = function (b, n) {
    var type = b.data.type || "p";
    var row = el("div", { class: "d-block t-" + type + (type === "todo" && b.data.checked ? " done" : "") });
    row.dataset.id = b.id;
    row.appendChild(el("button", { type: "button", class: "d-handle", text: "⋮⋮", title: "换成…、删除", "aria-label": "这一段的设置", onclick: function (e) { blockMenu(e.currentTarget, b); } }));
    if (type === "hr") { row.appendChild(el("hr")); return row; }
    if (type === "li") row.appendChild(el("span", { class: "d-mark", text: "•" }));
    if (type === "ol") row.appendChild(el("span", { class: "d-mark", text: n + "." }));
    if (type === "todo") row.appendChild(el("input", { type: "checkbox", checked: !!b.data.checked, "aria-label": "做完了",
      onchange: function (e) { row.classList.toggle("done", e.target.checked); save(b.id, { checked: e.target.checked }); } }));
    var node = el(type === "code" ? "pre" : "div", { class: "d-text", "data-ph": PH[type] || "" });
    node.contentEditable = plain ? "plaintext-only" : "true";
    node.textContent = b.data.text || "";
    node.addEventListener("keydown", function (e) { keydown(blocks.get(b.id) || b, node, e); });
    node.addEventListener("input", function () { input(blocks.get(b.id) || b, node); });
    node.addEventListener("paste", function (e) {
      if (plain) return;
      e.preventDefault();
      document.execCommand("insertText", false, (e.clipboardData || window.clipboardData).getData("text/plain"));
    });
    node.addEventListener("focus", function () { focused = b.id; row.classList.add("is-focus"); syncType(); });
    node.addEventListener("blur", function () { row.classList.remove("is-focus"); if (pending[b.id]) save(b.id, { text: textOf(node) }); });
    row.appendChild(node);
    return row;
  };

  // Draws the blocks; the one being typed in keeps its node (and caret).
  var renderBlocks = function (flash) {
    if (!list) return;
    var active = document.activeElement && document.activeElement.closest && document.activeElement.closest(".d-block");
    var keepId = active && active.dataset.id && pending[active.dataset.id] !== undefined ? active.dataset.id : null;
    if (active && !keepId && document.activeElement.classList.contains("d-text")) keepId = active.dataset.id;
    var olN = 0, frag = document.createDocumentFragment();
    blocks.rows.forEach(function (b) {
      olN = b.data.type === "ol" ? olN + 1 : 0;
      if (b.id === keepId && active) {
        var mark = active.querySelector(".d-mark");
        if (mark && b.data.type === "ol") mark.textContent = olN + ".";
        frag.appendChild(active);
        return;
      }
      var row = rowOf(b, olN);
      if (b.id === flash) row.classList.add("k-flash");
      frag.appendChild(row);
    });
    list.innerHTML = "";
    list.appendChild(frag);
    if (!blocks.rows.length) list.appendChild(el("div", { class: "k-empty", text: "点这里开始写", onclick: function () { addAfter(null, "p", ""); } }));
    if (keepId && active) { var n = active.querySelector(".d-text"); if (n && document.activeElement !== n) n.focus(); }
  };
  var syncType = function () {
    var b = focused && blocks.get(focused);
    if (typeSel && b) typeSel.value = b.data.type || "p";
  };

  var markdown = function () {
    var olN = 0, out = ["# " + (titleInput.value || "文档"), ""];
    blocks.rows.forEach(function (b) {
      var t = b.data.text || "", type = b.data.type || "p";
      olN = type === "ol" ? olN + 1 : 0;
      out.push(type === "h1" ? "# " + t : type === "h2" ? "## " + t : type === "h3" ? "### " + t : type === "li" ? "- " + t : type === "ol" ? olN + ". " + t
        : type === "todo" ? "- [" + (b.data.checked ? "x" : " ") + "] " + t : type === "quote" ? "> " + t.replace(/\n/g, "\n> ")
        : type === "code" ? "```\n" + t + "\n```" : type === "hr" ? "---" : t);
      out.push("");
    });
    return out.join("\n");
  };
  var html = function () {
    var esc = function (s) { return String(s || "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); };
    var body = [], open = "";
    blocks.rows.forEach(function (b) {
      var type = b.data.type || "p", t = esc(b.data.text).replace(/\n/g, "<br>");
      var want = type === "li" || type === "todo" ? "ul" : type === "ol" ? "ol" : "";
      if (open !== want) { if (open) body.push("</" + open + ">"); if (want) body.push("<" + want + ">"); open = want; }
      body.push(type === "li" || type === "ol" ? "<li>" + t + "</li>" : type === "todo" ? '<li><input type="checkbox" disabled' + (b.data.checked ? " checked" : "") + "> " + t + "</li>"
        : type === "quote" ? "<blockquote>" + t + "</blockquote>" : type === "code" ? "<pre>" + esc(b.data.text) + "</pre>" : type === "hr" ? "<hr>" : "<" + (/^h[123]$/.test(type) ? type : "p") + ">" + t + "</" + (/^h[123]$/.test(type) ? type : "p") + ">");
    });
    if (open) body.push("</" + open + ">");
    var title = esc(titleInput.value || "文档");
    return '<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>' + title + '</title><style>body{max-width:760px;margin:40px auto;padding:0 20px;font:16px/1.7 -apple-system,"PingFang SC","Microsoft YaHei",sans-serif;color:#1d1d1f}pre{background:#f5f5f7;padding:12px;border-radius:8px;overflow:auto}blockquote{border-left:3px solid #ddd;margin:0;padding-left:14px;color:#555}ul{padding-left:22px}</style></head><body><h1>' + title + "</h1>" + body.join("\n") + "</body></html>";
  };

  var render = function () {
    app.innerHTML = "";
    var m = docs.get("main");
    titleInput = el("input", { class: "d-title", value: m ? m.data.title || "" : "", placeholder: "无标题", "aria-label": "标题",
      onchange: function () { docs.put({ title: titleInput.value.trim() }, "main").catch(function (e) { kit.note(e.message); }); },
      onkeydown: function (e) { if (e.key === "Enter" && !e.isComposing) { e.preventDefault(); var f = list.querySelector(".d-text"); if (f) place(f, 0); else addAfter(null, "p", ""); } } });
    typeSel = el("select", { class: "field", "aria-label": "这一段是", onchange: function () { var b = focused && blocks.get(focused); if (b) setType(b, typeSel.value); } },
      TYPES.map(function (t) { return el("option", { value: t[0], text: t[1] }); }));
    var name = function () { return (titleInput.value || "文档").replace(/[\\/:*?"<>|]/g, " "); };
    app.appendChild(el("div", { class: "k-top" }, el("span", { style: "flex:1" }), me ? el("span", { class: "k-who", text: "你是 " + me }) : null, typeSel,
      el("button", { type: "button", text: "导出", onclick: function (e) {
        flushAll();
        kit.menu(e.currentTarget, [
          { label: "导出 Markdown（.md）", onClick: function () { kit.save(name() + ".md", markdown(), "text/markdown"); } },
          { label: "导出网页（.html）", onClick: function () { kit.save(name() + ".html", html(), "text/html"); } },
        ]);
      } })));
    list = el("div", { class: "d-list" });
    page = el("div", { class: "d-page" }, titleInput, list,
      el("div", { class: "d-help", text: "回车新的一段；段首打 # 标题、- 列表、1. 编号、[] 待办、> 引用、``` 代码、--- 分隔线。agent 写的段落会直接出现在这里。" }));
    app.appendChild(page);
    renderBlocks();
  };

  kit.start(function (ctx) {
    me = ctx.me && ctx.me.name || "";
    return Promise.all([docs.load(), blocks.load()]).then(function () {
      if (!docs.get("main")) return docs.put({ title: "" }, "main").catch(function () { return docs.load(); });
    }).then(function () {
      render();
      if (!blocks.rows.length) addAfter(null, "p", "");
      kit.listen([docs, blocks], function (ch) {
        if (ch.collection === "doc") { var m = docs.get("main"); if (m && document.activeElement !== titleInput) titleInput.value = m.data.title || ""; }
        else renderBlocks(ch.record);
        if (ch.by && ch.by !== me) kit.note(ch.by + " 刚改了");
      });
      window.addEventListener("beforeunload", flushAll);
    });
  });
})();
