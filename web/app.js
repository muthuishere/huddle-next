// Huddle dashboard (ADR 20). Vanilla, offline, no inline script or style:
// the page is served under `script-src 'self'; style-src 'self'`.
"use strict";

var TOKEN = (document.querySelector('meta[name="huddle-token"]') || {}).content || "";
var repoName = "";

// ---------- tiny DOM helpers ----------
function el(tag, cls, text) {
  var n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text !== undefined && text !== null) n.textContent = String(text);
  return n;
}
function frag() { return document.createDocumentFragment(); }
function clear(n) { while (n.firstChild) n.removeChild(n.firstChild); }

// table(headers, rows) where a cell is a string or a node.
function table(headers, rows) {
  var t = el("table"), thead = el("thead"), tr = el("tr");
  headers.forEach(function (h) {
    var th = el("th", h && h.num ? "num" : null, h && h.num ? h.label : h);
    tr.appendChild(th);
  });
  thead.appendChild(tr); t.appendChild(thead);
  var tb = el("tbody");
  rows.forEach(function (r) {
    var row = el("tr");
    if (r.cls) row.className = r.cls;
    (r.cells || r).forEach(function (c) {
      var td = el("td");
      if (c && c.nodeType) td.appendChild(c);
      else if (c && typeof c === "object" && "num" in c) { td.className = "num"; td.textContent = c.num; }
      else td.textContent = c === undefined || c === null ? "" : String(c);
      row.appendChild(td);
    });
    tb.appendChild(row);
  });
  t.appendChild(tb);
  return t;
}
function pill(text, kind) { return el("span", "pill" + (kind ? " " + kind : ""), text); }
function cmd(text) { return el("code", "cmd", text); }
function card(title, body) {
  var c = el("div", "card");
  if (title) c.appendChild(el("h2", null, title));
  if (body) c.appendChild(body);
  return c;
}
function stat(n, k) {
  var s = el("div", "stat");
  s.appendChild(el("div", "n", n));
  s.appendChild(el("div", "k", k));
  return s;
}
function empty(msg) { return el("p", "muted", msg); }

// ---------- transport ----------
function q(extra) {
  var p = new URLSearchParams(extra || {});
  if (repoName) p.set("repo", repoName);
  var s = p.toString();
  return s ? "?" + s : "";
}
function showErr(e) {
  var box = document.getElementById("err");
  if (!e) { box.hidden = true; box.textContent = ""; return; }
  box.hidden = false;
  box.textContent = String(e.message || e);
}
function get(path, extra) {
  return fetch(path + q(extra), { headers: { "Accept": "application/json" } })
    .then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (b) {
        if (!r.ok) throw new Error(b.error || r.status + " " + r.statusText);
        return b;
      });
    });
}
function post(path, body, extra) {
  return fetch(path + q(extra), {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Huddle-Token": TOKEN },
    body: JSON.stringify(body || {})
  }).then(function (r) {
    return r.json().catch(function () { return {}; }).then(function (b) {
      if (!r.ok) throw new Error(b.error || r.status + " " + r.statusText);
      return b;
    });
  });
}

// ---------- views ----------
var VIEWS = [
  ["overview", "Overview", viewOverview],
  ["artifacts", "Artifacts", viewArtifacts],
  ["gates", "Gates", viewGates],
  ["timeline", "Timeline", viewTimeline],
  ["personas", "Personas", viewPersonas],
  ["hooks", "Hooks", viewHooks],
  ["analytics", "Analytics", viewAnalytics],
  ["repos", "Repos", viewRepos]
];

function viewOverview(root) {
  return get("/api/overview").then(function (d) {
    var rep = d.report || {};
    var head = el("div", "grid");
    head.appendChild(stat((rep.changes || []).length, "changes"));
    head.appendChild(stat(rep.evals || 0, "evals"));
    head.appendChild(stat(d.branch || "—", "branch"));
    head.appendChild(stat(d.isGit ? "git" : "folder", "mode"));
    root.appendChild(head);

    if (d.next) root.appendChild(card("Next action", cmd(d.next)));

    var warns = d.warnings || [];
    if (warns.length) {
      root.appendChild(card("Warnings", table(["", "What", "Fix"], warns.map(function (w) {
        return [pill(w.level, w.level === "warn" ? "warn" : null), w.message, w.command ? cmd(w.command) : ""];
      }))));
    }

    var ch = rep.changes || [];
    root.appendChild(card("SDLC", ch.length ? table(
      ["Change", "Intent", "Spec", "Plan", "Verdict"],
      ch.map(function (c) {
        var v = c.verdict;
        return {
          cells: [
            c.slug || c.id || "—",
            c.intent ? c.intent.status || "yes" : "—",
            c.spec ? c.spec.status || "yes" : "—",
            c.plan ? c.plan.status || "yes" : "—",
            v ? pill(v.verdict + (v.stale ? " (stale)" : ""), v.stale ? "warn" : (v.verdict === "PASS" ? "ok" : "bad")) : "—"
          ]
        };
      })
    ) : empty("No changes tracked yet — start one with `huddle intent new <slug>`.")));
  });
}

function viewArtifacts(root) {
  return get("/api/artifacts").then(function (d) {
    var rows = d.artifacts || [];
    var bar = el("div", "row");
    var sel = el("select");
    sel.appendChild(new Option("all kinds", ""));
    (d.kinds || []).forEach(function (k) { sel.appendChild(new Option(k, k)); });
    bar.appendChild(sel);

    var lintBtn = el("button", "ghost", "Run lint");
    bar.appendChild(lintBtn);
    var out = el("div");
    lintBtn.addEventListener("click", function () {
      showErr(null);
      post("/api/lint", { kind: sel.value }).then(function (r) {
        clear(out);
        out.appendChild(card("Lint — " + r.checked + " checked",
          r.ok ? el("p", "muted", "No findings.")
               : table(["Artifact", "Rule", "Message"], (r.findings || []).map(function (f) {
                   return [f.id || f.path, f.rule, f.message];
                 }))));
      }).catch(showErr);
    });

    sel.addEventListener("change", function () { render(current); });
    root.appendChild(bar);
    root.appendChild(out);

    root.appendChild(rows.length ? table(
      ["ID", "Kind", "Status", { num: true, label: "Lint" }, "Path"],
      rows.map(function (a) {
        return [a.id, a.kind, pill(a.status, a.status === "approved" ? "ok" : null),
                { num: a.lint }, el("code", "mono", a.path)];
      })
    ) : empty("No artifacts yet."));
  });
}

function viewGates(root) {
  return get("/api/gates").then(function (d) {
    var c = d.counts || {};
    var g = el("div", "grid");
    ["pending", "approved", "rejected", "stale", "override"].forEach(function (k) {
      if (c[k]) g.appendChild(stat(c[k], k));
    });
    if (g.children.length) root.appendChild(g);

    root.appendChild(card("Why not here?", el("p", "muted", d.why || "")));

    var rows = d.gates || [];
    root.appendChild(rows.length ? table(
      ["Gate", "State", "By", "Approve in a terminal"],
      rows.map(function (x) {
        return {
          cells: [x.id, pill(x.state + (x.stale ? " · stale" : ""), x.state === "approved" ? "ok" : (x.state === "rejected" ? "bad" : "warn")),
                  x.by || "—", cmd(x.approveCommand)]
        };
      })
    ) : empty("No gates recorded."));
  });
}

function viewTimeline(root) {
  var bar = el("div", "row");
  var type = el("input"); type.type = "text"; type.placeholder = "type (e.g. decision)";
  var since = el("input"); since.type = "text"; since.placeholder = "since (e.g. 30d)";
  var go = el("button", "ghost", "Filter");
  bar.appendChild(type); bar.appendChild(since); bar.appendChild(go);
  root.appendChild(bar);

  var add = el("div", "card");
  add.appendChild(el("h2", null, "Append to the log"));
  var kind = el("select");
  ["decision", "learning", "question"].forEach(function (k) { kind.appendChild(new Option(k, k)); });
  var door = el("select");
  ["one-way", "two-way"].forEach(function (k) { door.appendChild(new Option("door: " + k, k)); });
  var text = el("textarea"); text.placeholder = "What was decided or learned?";
  var save = el("button", "act", "Append");
  var r1 = el("div", "row"); r1.appendChild(kind); r1.appendChild(door); r1.appendChild(save);
  add.appendChild(r1); add.appendChild(text);
  root.appendChild(add);

  var list = el("div");
  root.appendChild(list);

  function load() {
    showErr(null);
    var extra = {};
    if (type.value.trim()) extra.type = type.value.trim();
    if (since.value.trim()) extra.since = since.value.trim();
    return get("/api/events", extra).then(function (d) {
      clear(list);
      var evs = d.events || [];
      list.appendChild(evs.length ? table(
        ["When", "Type", "By", "What"],
        evs.map(function (e) {
          return {
            cls: e.superseded ? "struck" : null,
            cells: [new Date(e.ts).toLocaleString(), e.type, e.by || "—", e.text || ""]
          };
        })
      ) : empty("No events match."));
    }).catch(showErr);
  }
  go.addEventListener("click", load);
  save.addEventListener("click", function () {
    showErr(null);
    post("/api/log", { type: kind.value, door: door.value, text: text.value })
      .then(function () { text.value = ""; return load(); })
      .catch(showErr);
  });
  return load();
}

function viewPersonas(root) {
  return get("/api/personas").then(function (d) {
    var ps = d.personas || [];
    var detail = el("div");
    root.appendChild(ps.length ? table(
      ["ID", "Name", "Layer", "Style card"],
      ps.map(function (p) {
        var b = el("button", "ghost", "view");
        b.addEventListener("click", function () {
          showErr(null);
          get("/api/persona/style", { id: p.id }).then(function (s) {
            clear(detail);
            var pre = el("pre", null, s.markdown || "");
            detail.appendChild(card("Style — " + p.id, pre));
          }).catch(showErr);
        });
        return [p.id, p.name || "", p.layer || p.source || "builtin", b];
      })
    ) : empty("No personas resolved."));
    root.appendChild(detail);
    if ((d.overflow || []).length) {
      root.appendChild(card("Overflow", el("p", "muted", (d.overflow || []).join(", "))));
    }
  });
}

function viewHooks(root) {
  return get("/api/hooks").then(function (d) {
    var test = el("div", "card");
    test.appendChild(el("h2", null, "Test a hook"));
    var ev = el("input"); ev.type = "text"; ev.placeholder = "gate.request";
    var run = el("button", "act", "Fire");
    var r = el("div", "row"); r.appendChild(ev); r.appendChild(run);
    test.appendChild(r);
    var res = el("div");
    test.appendChild(res);
    run.addEventListener("click", function () {
      showErr(null);
      post("/api/hooks/test", { event: ev.value.trim(), payload: {} }).then(function (x) {
        clear(res);
        res.appendChild(el("pre", null, JSON.stringify(x.result, null, 2)));
      }).catch(showErr);
    });
    root.appendChild(test);

    ["global", "repo"].forEach(function (scope) {
      var rows = (d.byScope && d.byScope[scope]) || [];
      root.appendChild(card(scope + " hooks", (rows && rows.length) ? table(
        ["Event", "Command", "Timeout"],
        rows.map(function (h) { return [h.event, el("code", "mono", h.command || h.run || ""), h.timeout]; })
      ) : empty("None configured.")));
    });

    if (d.repoHooksChanged) {
      root.appendChild(card("REPO_HOOKS_CHANGED",
        el("p", "muted", d.changedCount + " repo hook(s) changed since you last looked — review before trusting them.")));
    }
  });
}

function viewAnalytics(root) {
  return get("/api/analytics").then(function (d) {
    var u = d.usage || {};
    if (!d.enabled) root.appendChild(card("Usage recording is off", el("p", "muted", "Turn it on: huddle config set usage true")));

    var g = el("div", "grid");
    g.appendChild(stat(u.total || 0, "invocations"));
    g.appendChild(stat(u.human || 0, "human"));
    g.appendChild(stat(u.agent || 0, "agent"));
    g.appendChild(stat(u.errors || 0, "errors"));
    root.appendChild(g);

    var cs = u.commands || [];
    root.appendChild(card("Commands vs budget", cs.length ? table(
      ["Command", { num: true, label: "Count" }, { num: true, label: "Errors" },
       { num: true, label: "p50 ms" }, { num: true, label: "p95 ms" }, "Budget"],
      cs.map(function (c) {
        var over = c.budgetMs && c.p50ms > c.budgetMs;
        return {
          cells: [c.cmd, { num: c.count }, { num: c.errors },
                  { num: Math.round(c.p50ms) }, { num: Math.round(c.p95ms) },
                  c.budgetMs ? pill(Math.round(c.budgetMs) + " ms", over ? "bad" : "ok") : "—"]
        };
      })
    ) : empty("No usage recorded yet.")));

    var sk = u.skills || {};
    var skKeys = Object.keys(sk);
    if (skKeys.length) {
      root.appendChild(card("Skills", table(["Skill", { num: true, label: "Invocations" }],
        skKeys.sort(function (a, b) { return sk[b] - sk[a]; }).map(function (k) { return [k, { num: sk[k] }]; }))));
    }

    var f = d.flow;
    if (f) {
      var fg = el("div", "grid");
      fg.appendChild(stat(f.gatesPending || 0, "gates pending"));
      fg.appendChild(stat(Math.round(f.gateWaitP50Min || 0) + "m", "gate wait p50"));
      fg.appendChild(stat(Math.round((f.firstPassRate || 0) * 100) + "%", "first-pass PASS"));
      fg.appendChild(stat(f.staleVerdicts || 0, "stale verdicts"));
      fg.appendChild(stat(f.overrides || 0, "overrides"));
      root.appendChild(el("h2", null, "Flow"));
      root.appendChild(fg);
    }
  });
}

function viewRepos(root) {
  return get("/api/repos").then(function (d) {
    var rows = d.repos || d || [];
    root.appendChild(rows.length ? table(
      ["Repo", { num: true, label: "Events" }, "Last write", "Root"],
      rows.map(function (r) {
        var name = r.current ? el("strong", null, r.name) : el("span", null, r.name);
        return [name, { num: r.events }, r.lastWrite ? new Date(r.lastWrite).toLocaleString() : "—",
                el("code", "mono", r.root || "(root unknown)")];
      })
    ) : empty("No repos with huddle state."));
  });
}

// ---------- shell ----------
var current = "overview";

function render(name) {
  current = name;
  var view = document.getElementById("view");
  clear(view);
  view.appendChild(empty("Loading…"));
  Array.prototype.forEach.call(document.querySelectorAll(".tab"), function (b) {
    b.setAttribute("aria-selected", b.dataset.view === name ? "true" : "false");
  });
  var fn = (VIEWS.find(function (v) { return v[0] === name; }) || VIEWS[0])[2];
  var root = el("div");
  showErr(null);
  Promise.resolve()
    .then(function () { return fn(root); })
    .then(function () { clear(view); view.appendChild(root); })
    .catch(function (e) { clear(view); view.appendChild(empty("")); showErr(e); });
  if (location.hash.slice(1) !== name) location.hash = name;
}

function boot() {
  var tabs = document.getElementById("tabs");
  VIEWS.forEach(function (v) {
    var b = el("button", "tab", v[1]);
    b.dataset.view = v[0];
    b.setAttribute("role", "tab");
    b.addEventListener("click", function () { render(v[0]); });
    tabs.appendChild(b);
  });

  get("/api/repos").then(function (d) {
    var rows = d.repos || [];
    var pick = document.getElementById("repoPick");
    rows.forEach(function (r) {
      var o = new Option(r.name, r.name);
      if (r.current) { o.selected = true; repoName = ""; document.getElementById("repo").textContent = r.name + (r.root ? " · " + r.root : ""); }
      pick.appendChild(o);
    });
    pick.addEventListener("change", function () { repoName = pick.value; render(current); });
  }).catch(function () { /* repo list is optional chrome */ });

  window.addEventListener("hashchange", function () {
    var h = location.hash.slice(1);
    if (h && h !== current) render(h);
  });
  render(location.hash.slice(1) || "overview");
}

boot();
