// The script every html file of a page opened in the myco App starts with
// (docs/design/browser.md). Two things:
//
// 1. A link to elsewhere opens in the system's browser (the App is asked),
//    not in the page panel.
// 2. The myco bridge: the page's shared state on the hub (read, written,
//    changed by others), a message to the agent that published it, and
//    storage of its own.
//
//      const ctx = await myco.ready;        // {page, me, state, version}
//      myco.on("change", (c) => draw(c.state));
//      const r = await myco.update(next);   // {ok, state, version}
//      await myco.send("text");             // a task to its agent, as you
//      await myco.storage.set("k", value); await myco.storage.get("k");
//
// It talks to the App with postMessage; outside the App, myco.ready fails
// and a page shows what it has.
(function () {
  "use strict";
  var inApp = window.parent !== window;

  function out(u) {
    try {
      var x = new URL(u, location.href);
      return x.origin !== location.origin && /^(https?|mailto):$/.test(x.protocol) ? x.href : "";
    } catch (e) { return ""; }
  }
  if (inApp) {
    document.addEventListener("click", function (e) {
      if (e.defaultPrevented || e.button !== 0) return;
      var a = e.target && e.target.closest ? e.target.closest("a[href]") : null;
      if (!a) return;
      var u = out(a.getAttribute("href"));
      if (!u) return;
      e.preventDefault();
      parent.postMessage({ myco: "open-url", url: u }, "*");
    }, true);
    var open = window.open;
    window.open = function (u) {
      var x = out(String(u || ""));
      if (x) { parent.postMessage({ myco: "open-url", url: x }, "*"); return null; }
      return open.apply(window, arguments);
    };
  }

  var seq = 0;
  var waiting = {};
  var handlers = { change: [], data: [] };
  var ctx = null;

  function call(method, args, ms) {
    return new Promise(function (resolve, reject) {
      if (!inApp) { reject(new Error("myco: this page is not open in the myco App")); return; }
      var id = ++seq;
      waiting[id] = { resolve: resolve, reject: reject };
      parent.postMessage({ myco: "call", id: id, method: method, args: args || {} }, "*");
      setTimeout(function () {
        if (!waiting[id]) return;
        delete waiting[id];
        reject(new Error("myco: no answer from the App"));
      }, ms || 15000);
    });
  }

  window.addEventListener("message", function (e) {
    if (e.source !== window.parent) return;
    var d = e.data;
    if (!d || typeof d !== "object") return;
    if (d.myco === "result" && waiting[d.id]) {
      var w = waiting[d.id];
      delete waiting[d.id];
      if (d.error) {
        var err = new Error(d.error);
        err.code = d.code;
        if (d.latest) err.latest = d.latest;
        w.reject(err);
      } else w.resolve(d.value);
    } else if (d.myco === "data" && Array.isArray(d.changes)) {
      // What others (an agent, another person) changed in the records.
      d.changes.forEach(function (c) {
        handlers.data.slice().forEach(function (f) {
          try { f(c); } catch (x) { setTimeout(function () { throw x; }); }
        });
      });
    } else if (d.myco === "change") {
      if (ctx) { ctx.state = d.state; ctx.version = d.version; }
      var c = { state: d.state, version: d.version, by: d.by };
      handlers.change.slice().forEach(function (f) {
        try { f(c); } catch (x) { setTimeout(function () { throw x; }); }
      });
    }
  });

  var ready = call("ready").then(function (v) { ctx = v; return v; });
  ready.catch(function () { /* (a page that does not use the bridge) */ });

  window.myco = {
    ready: ready,
    get state() { return ctx ? ctx.state : null; },
    get version() { return ctx ? ctx.version : 0; },
    on: function (event, f) {
      (handlers[event] = handlers[event] || []).push(f);
      return function () {
        var l = handlers[event];
        var i = l.indexOf(f);
        if (i >= 0) l.splice(i, 1);
      };
    },
    // Writes the shared state, based on the version this page has. When
    // someone wrote first: {ok: false} with the latest state and version.
    update: function (next) {
      return ready.then(function () {
        return call("update", { state: next, version: ctx.version });
      }).then(function (r) {
        ctx.state = r.state;
        ctx.version = r.version;
        return r;
      });
    },
    send: function (text) { return call("send", { text: String(text) }, 120000); },
    // The page's records (hubs from v0.20.5): collections of JSON objects,
    // each written on its own with a version. put without opts.id makes a
    // new one; with opts.ifVersion it is refused (err.code "conflict",
    // err.latest the record now) if someone changed it since.
    data: {
      list: function (collection) { return call("data.list", { collection: collection || "" }); },
      get: function (collection, id) { return call("data.get", { collection: collection, id: id }); },
      put: function (collection, data, opts) {
        opts = opts || {};
        return call("data.put", { collection: collection, data: data, id: opts.id || "", ifVersion: opts.ifVersion === undefined ? null : opts.ifVersion });
      },
      remove: function (collection, id, opts) {
        return call("data.remove", { collection: collection, id: id, ifVersion: (opts && opts.ifVersion) || 0 });
      },
      // f({seq, collection, record, op: "put" | "delete", data, version, by})
      on: function (f) {
        var unsub = window.myco.on("data", f);
        call("data.watch", {}).catch(function () { /* (an older App) */ });
        return unsub;
      },
    },
    // Saves a file the page made (an export: csv, xlsx, pdf…) to the
    // person's Downloads. content: a string, Blob, ArrayBuffer or bytes.
    download: function (name, content, type) {
      return Promise.resolve(content instanceof Blob ? content.arrayBuffer() : content).then(function (c) {
        var bytes = typeof c === "string" ? new TextEncoder().encode(c) : c instanceof ArrayBuffer ? new Uint8Array(c) : new Uint8Array(c.buffer || c);
        var bin = "";
        for (var i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
        return call("download", { name: String(name), type: type || "", base64: btoa(bin) }, 60000);
      });
    },
    storage: {
      get: function (key) { return call("storage.get", { key: String(key) }); },
      set: function (key, value) { return call("storage.set", { key: String(key), value: value }); },
      remove: function (key) { return call("storage.set", { key: String(key), value: null }); },
    },
  };
})();
