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
    } else if (d.myco === "os.event" && typeof d.job === "string") {
      var j = jobs[d.job];
      if (j) j._take(d);
    } else if (d.myco === "change") {
      if (ctx) { ctx.state = d.state; ctx.version = d.version; }
      var c = { state: d.state, version: d.version, by: d.by };
      handlers.change.slice().forEach(function (f) {
        try { f(c); } catch (x) { setTimeout(function () { throw x; }); }
      });
    }
  });

  // myco.os: what the page uses of this computer. A call answers once; a
  // job (fs.scan, exec.spawn) sends events while it runs and ends with a
  // value (job.done).
  var jobs = {};
  function osCall(name, args, opts) {
    var a = args || {};
    var ms = (opts && opts.timeout) || (a.timeout_ms ? a.timeout_ms + 30000 : 600000);
    return call("os.call", { name: name, args: a }, ms);
  }
  function Job(name, args) {
    var self = this, listeners = {}, resolve, reject;
    self.id = "";
    self.name = name;
    self.done = new Promise(function (a, b) { resolve = a; reject = b; });
    self.done.catch(function () { /* (handled by whoever awaits it) */ });
    // f(data, event) for "progress", "stdout", "stderr", "started"…; "*" for all
    self.on = function (type, f) {
      (listeners[type] = listeners[type] || []).push(f);
      return function () {
        var l = listeners[type];
        var i = l.indexOf(f);
        if (i >= 0) l.splice(i, 1);
      };
    };
    var started = call("os.start", { name: name, args: args || {} }, 60000).then(function (r) {
      self.id = r.id;
      jobs[r.id] = self;
      return r.id;
    });
    started.catch(function (e) { reject(e); });
    self.write = function (data) { return started.then(function (id) { return call("os.input", { job: id, data: String(data) }); }); };
    self.end = function () { return started.then(function (id) { return call("os.input", { job: id, close: true }); }); };
    self.cancel = function () { return started.then(function (id) { return call("os.cancel", { job: id }); }); };
    self._take = function (d) {
      (d.events || []).forEach(function (e) {
        (listeners[e.type] || []).concat(listeners["*"] || []).forEach(function (f) {
          try { f(e.data, e); } catch (x) { setTimeout(function () { throw x; }); }
        });
      });
      if (!d.done) return;
      delete jobs[self.id];
      if (d.error) {
        var err = new Error(d.error);
        err.code = d.code;
        reject(err);
      } else resolve(d.value);
    };
  }
  var osList = null;
  var os = {
    // {os, arch, caps: [{name, job, about, available, missing}]}
    list: function () {
      if (!osList) {
        osList = call("os.list", {});
        osList.catch(function () { osList = null; });
      }
      return osList;
    },
    // true when this App can do it on this computer
    has: function (name) {
      return os.list().then(function (l) {
        return l.caps.some(function (c) { return c.name === name && c.available; });
      }, function () { return false; });
    },
    call: osCall,
    start: function (name, args) { return new Job(name, args); },
  };
  // os.fs.list({path}), os.exec.spawn({shell}) …: the same, by name
  [
    "system.info", "system.disks", "system.processes", "process.kill",
    "fs.paths", "fs.list", "fs.stat", "fs.read", "fs.write", "fs.mkdir", "fs.move", "fs.copy", "fs.delete", "fs.trash", "fs.scan*", "fs.reveal", "fs.open",
    "exec.run", "exec.spawn*", "clipboard.read", "clipboard.write", "notify", "screen.capture", "net.fetch",
  ].forEach(function (n) {
    var job = n.slice(-1) === "*";
    var name = job ? n.slice(0, -1) : n;
    var parts = name.split(".");
    var f = job
      ? function (args) { return new Job(name, args); }
      : function (args, opts) { return osCall(name, args, opts); };
    if (parts.length === 1) { os[parts[0]] = f; return; }
    os[parts[0]] = os[parts[0]] || {};
    os[parts[0]][parts[1]] = f;
  });

  var ready = call("ready").then(function (v) { ctx = v; return v; });
  ready.catch(function () { /* (a page that does not use the bridge) */ });

  window.myco = {
    ready: ready,
    // What this App's bridge can do, for pages to check before trying:
    // "download-media": myco.download saves video and sound too (mp4, …);
    // "os": myco.os (this computer: files, commands, the screen…).
    features: ["download-media", "os"],
    os: os,
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
      // A Blob goes to the App as it is (postMessage copies it without a
      // string in between: a video can be hundreds of MB); bytes and text
      // become one. Large files take a while to write: up to 10 minutes.
      var blob = content instanceof Blob ? content
        : new Blob([typeof content === "string" ? content : content instanceof ArrayBuffer ? new Uint8Array(content) : content], { type: type || "" });
      return call("download", { name: String(name), type: type || blob.type || "", blob: blob }, 600000);
    },
    storage: {
      get: function (key) { return call("storage.get", { key: String(key) }); },
      set: function (key, value) { return call("storage.set", { key: String(key), value: value }); },
      remove: function (key) { return call("storage.set", { key: String(key), value: null }); },
    },
  };
})();
