// The script every html file of a page opened in the myco App starts
// with. Two things:
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

  var displayLanguage = new URLSearchParams(location.search).get("myco_lang");
  var preferences = {
    language: displayLanguage === "en" || displayLanguage === "zh" ? displayLanguage : (/^zh\b/i.test(navigator.language || "") ? "zh" : "en"),
    theme: window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
  };
  function applyPreferences(next) {
    if (!next || !/^(zh|en)$/.test(next.language) || !/^(dark|light)$/.test(next.theme)) return;
    preferences = {language:next.language, theme:next.theme};
    document.documentElement.lang = preferences.language === "zh" ? "zh-CN" : "en";
    document.documentElement.style.colorScheme = preferences.theme;
    document.documentElement.setAttribute("data-myco-theme", preferences.theme);
    if (handlers && handlers.preferences) handlers.preferences.slice().forEach(function(f) {
      try { f(preferences); } catch (x) { setTimeout(function() {throw x;}); }
    });
  }
  applyPreferences(preferences);
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
    } else if (d.myco === "actions.invoke" || d.myco === "actions.cancel") {
      takeAction(d);
    } else if (d.myco === "preferences") {
      applyPreferences(d.preferences);
    } else if (d.myco === "warn" && typeof d.text === "string") {
      // the App's word to whoever builds this app (an undeclared permission…)
      try { console.warn(d.text); } catch (x) { /* (no console) */ }
    } else if (d.myco === "data" && Array.isArray(d.changes)) {
      // The feed moved on past what this page has seen (too much changed
      // while it was away): what it holds may be stale, so it should list
      // its records again (myco.on("reset")).
      if (d.reset) (handlers.reset || []).slice().forEach(function (f) {
        try { f({ reset: true }); } catch (x) { setTimeout(function () { throw x; }); }
      });
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
    // An address for <audio> / <video> of a recording an app made (a file
    // in ~/.myco/recordings): it seeks and plays at any speed.
    mediaURL: function (path) {
      var m = /^\/p\/[^\/]+\//.exec(location.pathname);
      return (m ? m[0] : "/") + "__myco/media?path=" + encodeURIComponent(path);
    },
  };
  // os.fs.list({path}), os.exec.spawn({shell}) …: the same, by name
  [
    "system.info", "system.disks", "system.processes", "system.usage", "system.apps", "system.settings", "process.kill",
    "fs.paths", "fs.list", "fs.stat", "fs.read", "fs.write", "fs.mkdir", "fs.move", "fs.copy", "fs.delete", "fs.trash", "fs.scan*",
    "fs.search*", "fs.watch*", "fs.thumbnail", "fs.pick", "fs.saveAs", "fs.reveal", "fs.open",
    "ssh.profiles", "ssh.run", "exec.run", "exec.spawn*", "clipboard.read", "clipboard.write", "notify", "screen.capture", "screen.region", "screen.record*", "net.fetch",
    "audio.sources", "audio.record*", "speech.status", "speech.install*", "speech.transcribe*", "speech.diarize*",
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

  // myco.actions: what an agent may ask this page's app to do (page.export:
  // a file of the page). Only in the App's renderer, which opens the page
  // for one job with an instance the App gave it (ctx.render.instance);
  // anywhere else, registering does nothing. One job at a time.
  var registered = null; // {descriptors, onInvoke}
  var inflight = {};     // id -> AbortController
  function renderInstance() { return ctx && ctx.render && typeof ctx.render.instance === "string" ? ctx.render.instance : ""; }
  function announce() {
    var inst = renderInstance();
    if (!inst || !registered) return;
    parent.postMessage({ myco: "actions.registered", instance: inst, actions: registered.descriptors.map(function (a) { return { name: a.name, version: a.version }; }) }, "*");
  }
  function takeAction(d) {
    var inst = renderInstance();
    if (!inst || d.instance !== inst || typeof d.id !== "string") return;
    if (d.myco === "actions.cancel") {
      var c = inflight[d.id];
      if (c) { delete inflight[d.id]; c.abort(); }
      return;
    }
    var reply = function (msg, transfer) {
      if (!inflight[d.id]) return; // cancelled meanwhile: no late result
      delete inflight[d.id];
      msg.myco = "actions.result"; msg.instance = inst; msg.id = d.id;
      parent.postMessage(msg, "*", transfer || []);
    };
    if (!registered || Object.keys(inflight).length) {
      parent.postMessage({ myco: "actions.result", instance: inst, id: d.id, error: { code: registered ? "busy" : "unsupported", message: registered ? "an action is running" : "this page's app registered no actions" } }, "*");
      return;
    }
    var known = registered.descriptors.some(function (a) { return a.name === d.action && a.version === d.version; });
    var ac = new AbortController();
    inflight[d.id] = ac;
    if (!known) { reply({ error: { code: "unsupported", message: "this page's app has no " + d.action + " v" + d.version } }); return; }
    Promise.resolve().then(function () {
      return registered.onInvoke({ id: d.id, action: d.action, version: d.version, args: d.args || {}, signal: ac.signal });
    }).then(function (r) {
      if (!r || typeof r.media_type !== "string" || !r.data) throw Object.assign(new Error("the app returned no file"), { code: "export_failed" });
      var data = r.data;
      return (data instanceof Blob ? data.arrayBuffer() : Promise.resolve(data instanceof ArrayBuffer ? data : new Uint8Array(data).slice().buffer)).then(function (buf) {
        var res = { media_type: r.media_type, data: buf };
        if (r.width) res.width = r.width;
        if (r.height) res.height = r.height;
        if (r.pages) res.pages = r.pages;
        reply({ result: res }, [buf]);
      });
    }).catch(function (e) {
      reply({ error: { code: (e && e.code) || (ac.signal.aborted ? "cancelled" : "export_failed"), message: (e && e.message) || "export failed" } });
    });
  }

  var ready = call("ready").then(function (v) { ctx = v; if(v.preferences) applyPreferences(v.preferences); announce(); return v; });
  ready.catch(function () { /* (a page that does not use the bridge) */ });

  window.myco = {
    ready: ready,
    get preferences() { return preferences; },
    // What this App's bridge can do, for pages to check before trying:
    // "download-media": myco.download saves video and sound too (mp4, …);
    // "os": myco.os (this computer: files, commands, the screen…).
    // "net-media": myco.media.url plays a public audio or video address.
    features: ["download-media", "os", "ssh", "preferences", "remote", "page-actions", "net-media", "data-reset", "data-query"] /*__MYCO_BROWSER_FEATURE__*/,
    // An address for <audio> / <video> of a public http(s) file (a podcast
    // episode): the App streams it, with seeking, since the page itself
    // cannot load media from the internet. Private addresses are refused.
    media: {
      url: function (u) {
        var m = /^\/p\/[^\/]+\//.exec(location.pathname);
        return (m ? m[0] : "/") + "__myco/stream?url=" + encodeURIComponent(String(u));
      },
    },
    os: os,
    // Remote sessions are separate from local OS calls and need App consent.
    browser: (function () {
      function invoke(action, args) {
        return ready.then(function () { return call("browser.call", {action:action,args:args||{}}, (action === "install" || action === "approvals.request" || action === "profiles.configure") ? 600000 : action === "pick" ? 150000 : 45000); });
      }
      return {
        status:function(){return invoke("status");},
        install:function(){return invoke("install");},
        profiles:{
          create:function(name){return invoke("profiles.create",{name:String(name)});},
          open:function(id){return invoke("profiles.open",{id:String(id)});},
          allowAgents:function(id,allowed){return invoke("profiles.allowAgents",{id:String(id),allowed:allowed===true});},
          configure:function(id,settings){return invoke("profiles.configure",{id:String(id),settings:settings});}
        },
        record:{
          active:function(){return invoke("record.active");},
          start:function(options){return invoke("record.start",options);},
          events:function(id,after){return invoke("record.events",{id:String(id),after:after});},
          stop:function(id){return invoke("record.stop",{id:String(id)});}
        },
        runs:{
          start:function(options){return invoke("runs.start",options);},
          get:function(id,after){return invoke("runs.get",{id:String(id),after:after});},
          control:function(id,action,choice){return invoke("runs.control",{id:String(id),action:action,choice:choice});}
        },
        approvals:{
          list:function(){return invoke("approvals.list");},
          request:function(options){return invoke("approvals.request",options);},
          revoke:function(script_id,version){return invoke("approvals.revoke",{script_id:String(script_id),version:version});}
        },
        locate:function(options){return invoke("locate",options);},
        pick:function(options){return invoke("pick",options);},
        snapshot:function(id){return invoke("snapshot",{id:String(id)}).then(function(r){
          var data=atob(r.base64),bytes=new Uint8Array(data.length);
          for(var i=0;i<data.length;i++)bytes[i]=data.charCodeAt(i);
          return new Blob([bytes],{type:r.type});
        });}
      };
    })(),
    remote: {call: function(action, args) {
      return ready.then(function() { return call("remote.call", {action:action, args:args || {}}, action === "install" ? 600000 : 30000); });
    }},
    // register([{name, version, input_schema}], onInvoke): onInvoke({id,
    // action, version, args, signal}) resolves {media_type, data (Blob,
    // ArrayBuffer or bytes), width?, height?, pages?}. Returns dispose.
    actions: {
      register: function (descriptors, onInvoke) {
        if (!Array.isArray(descriptors) || typeof onInvoke !== "function") throw new Error("myco.actions.register(descriptors, onInvoke)");
        var mine = { descriptors: descriptors.filter(function (a) { return a && typeof a.name === "string" && a.version >= 1; }), onInvoke: onInvoke };
        registered = mine;
        announce();
        return function () { if (registered === mine) registered = null; };
      },
    },
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
      list: function (collection, options) {
        var args = { collection: collection || "" };
        if (options !== undefined) args.query = options;
        return call("data.list", args);
      },
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
    projectFiles: {
      info: function () { return call("projectFiles.info", {}); },
      put: function (name, blob) { return call("projectFiles.put", { name: name, blob: blob }, 300000); },
      get: function (name) { return call("projectFiles.get", { name: name }, 300000); },
      download: function (name) { return call("projectFiles.download", { name: name }, 300000); },
    },
    storage: {
      get: function (key) { return call("storage.get", { key: String(key) }); },
      set: function (key, value) { return call("storage.set", { key: String(key), value: value }); },
      remove: function (key) { return call("storage.set", { key: String(key), value: null }); },
    },
  };
})();
