/// The script every page opened in the app starts with: `window.myco`, the
/// same calls as the desktop app's `internal/console/pagebridge.js`, over
/// the web view's message handler instead of `postMessage` to a parent.
enum PageBridgeScript {
    static let source = #"""
(function () {
  if (window.myco) return;
  var post = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.myco;
  var seq = 0, waiting = {}, handlers = { change: [], data: [] }, ctx = null;
  function call(method, args, ms) {
    return new Promise(function (resolve, reject) {
      if (!post) { reject(new Error("myco: this page is not open in the myco App")); return; }
      var id = ++seq;
      waiting[id] = { resolve: resolve, reject: reject };
      post.postMessage({ id: id, method: method, args: args || {} });
      setTimeout(function () {
        if (!waiting[id]) return;
        delete waiting[id];
        reject(new Error("myco: no answer from the App"));
      }, ms || 15000);
    });
  }
  window.__mycoAnswer = function (id, d) {
    var w = waiting[id];
    if (!w) return;
    delete waiting[id];
    if (d && d.error) {
      var err = new Error(d.error);
      err.code = d.code;
      if (d.latest !== undefined) err.latest = d.latest;
      w.reject(err);
    } else w.resolve(d ? d.value : null);
  };
  window.__mycoEvent = function (d) {
    if (!d || typeof d !== "object") return;
    if (d.myco === "data" && Array.isArray(d.changes)) {
      d.changes.forEach(function (c) {
        handlers.data.slice().forEach(function (f) { try { f(c); } catch (x) { setTimeout(function () { throw x; }); } });
      });
    } else if (d.myco === "change") {
      if (ctx) { ctx.state = d.state; ctx.version = d.version; }
      var c = { state: d.state, version: d.version, by: d.by };
      handlers.change.slice().forEach(function (f) { try { f(c); } catch (x) { setTimeout(function () { throw x; }); } });
    }
  };
  var ready = call("ready").then(function (v) { ctx = v; return v; });
  ready.catch(function () { /* (a page that does not use the bridge) */ });
  window.myco = {
    ready: ready,
    get state() { return ctx ? ctx.state : null; },
    get version() { return ctx ? ctx.version : 0; },
    on: function (event, f) {
      (handlers[event] = handlers[event] || []).push(f);
      return function () { var l = handlers[event]; var i = l.indexOf(f); if (i >= 0) l.splice(i, 1); };
    },
    update: function (next) {
      return ready.then(function () { return call("update", { state: next, version: ctx.version }); })
        .then(function (r) { ctx.state = r.state; ctx.version = r.version; return r; });
    },
    send: function (text) { return call("send", { text: String(text) }, 120000); },
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
      on: function (f) {
        var unsub = window.myco.on("data", f);
        call("data.watch", {}).catch(function () {});
        return unsub;
      },
    },
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
"""#
}
