/* MCP Preflight（MCP点检助手）前端逻辑（无外部依赖，内网离线可用） */
(function () {
  "use strict";

  var $ = function (id) { return document.getElementById(id); };
  var state = { sending: false, modelList: [], lang: "zh-CN", theme: "light", providers: [], provider: "deepseek" };

  /* ---------------- 工具函数 ---------------- */
  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  }

  /* ---------------- 国际化 ---------------- */
  // 新增语言时：在此增加一份对照，并在 index.html 的 <select id="cfg-lang"> 中加选项
  var I18N = {
    "zh-CN": {
      brand_sub: "mcppreflight",
      sec_model: "模型设置",
      lbl_provider: "服务商",
      lbl_base: "API 地址",
      lbl_key: "API Key",
      lbl_model: "模型",
      btn_fetch: "从 API 获取可用模型列表",
      btn_fetch_short: "获取",
      provider_custom: "自定义（手动填写地址）",
      btn_save: "保存配置",
      sec_workspace: "工作区",
      btn_pick: "选择工作区文件夹",
      ws_none: "未选择",
      sec_mcp: "MCP 服务器",
      lbl_mcp_path: "mcp.json 路径",
      btn_load: "加载",
      btn_reload_title: "重新读取 mcp.json 并连接",
      servers_empty: "尚未加载任何 MCP 服务器",
      status_ok: "已连接",
      status_unsupported: "暂不支持",
      status_error: "连接失败",
      no_tools: "该服务器未提供工具",
      sec_appearance: "外观",
      lbl_lang: "语言",
      lbl_theme: "主题",
      theme_light: "浅色",
      theme_dark: "暗色",
      btn_new_session: "新会话",
      panel_title: "MCP 工具调用",
      btn_clear: "清空",
      btn_clear_title: "清空右侧调用记录",
      mcp_empty: "尚无工具调用",
      composer_ph: "输入点检指令，例如：请对当前工作区执行点检并输出报告（Enter 发送，Shift+Enter 换行）",
      btn_send: "发送",
      role_user: "你",
      role_assistant: "助手",
      saved: "已保存",
      save_failed: "保存失败",
      fetch_failed: "获取模型列表失败",
      fetch_error: "获取失败: ",
      models_got: "已获取 {n} 个模型",
      new_session_started: "已开始新会话",
      need_key_first: "请先在上方填写 API Key 并保存",
      no_output: "（无文本输出）",
      request_failed: "请求失败: ",
      error_generic: "发生错误",
      tool_calling: "调用中…",
      tool_done: "完成",
      tool_failed: "失败",
      tool_args: "参数",
      tool_result: "返回",
      tool_error: "错误",
      chars_suffix: "（{n} 字）",
      expand: "展开全文",
      collapse: "收起",
      tool_marker: "🔧 调用了 {name}（详情见右侧）",
      resizer_title: "拖动调整右侧面板宽度（双击恢复默认）",
      welcome: "欢迎使用 MCP点检助手：① 填写 API Key 并保存 → ② 选择工作区文件夹 → ③ 加载 mcp.json → ④ 输入点检指令。中间是对话与报告，右侧面板实时显示 MCP 工具调用参数与返回。",
      tools_full: "工具调用：完整支持",
      tools_partial: "工具调用：部分支持",
      tools_none: "工具调用：不支持",
      models_api_no: "该服务商不提供模型列表接口，请从上方候选模型中输入或手动填写。"
    },
    "en": {
      brand_sub: "mcppreflight",
      sec_model: "Model",
      lbl_provider: "Provider",
      lbl_base: "Base URL",
      lbl_key: "API Key",
      lbl_model: "Model",
      btn_fetch: "Fetch available models from the API",
      btn_fetch_short: "Fetch",
      provider_custom: "Custom (enter the URL manually)",
      btn_save: "Save settings",
      sec_workspace: "Workspace",
      btn_pick: "Choose workspace folder",
      ws_none: "Not selected",
      sec_mcp: "MCP servers",
      lbl_mcp_path: "mcp.json path",
      btn_load: "Load",
      btn_reload_title: "Re-read mcp.json and connect",
      servers_empty: "No MCP server loaded yet",
      status_ok: "Connected",
      status_unsupported: "Unsupported",
      status_error: "Connection failed",
      no_tools: "This server exposes no tools",
      sec_appearance: "Appearance",
      lbl_lang: "Language",
      lbl_theme: "Theme",
      theme_light: "Light",
      theme_dark: "Dark",
      btn_new_session: "New session",
      panel_title: "MCP tool calls",
      btn_clear: "Clear",
      btn_clear_title: "Clear the call log",
      mcp_empty: "No tool calls yet",
      composer_ph: "Type an inspection command, e.g. inspect the current workspace and produce a report (Enter to send, Shift+Enter for a new line)",
      btn_send: "Send",
      role_user: "You",
      role_assistant: "Assistant",
      saved: "Saved",
      save_failed: "Save failed",
      fetch_failed: "Failed to fetch the model list",
      fetch_error: "Fetch failed: ",
      models_got: "Fetched {n} models",
      new_session_started: "New session started",
      need_key_first: "Please enter and save your API key above",
      no_output: "(no text output)",
      request_failed: "Request failed: ",
      error_generic: "Something went wrong",
      tool_calling: "Calling…",
      tool_done: "Done",
      tool_failed: "Failed",
      tool_args: "Args",
      tool_result: "Result",
      tool_error: "Error",
      chars_suffix: "({n} chars)",
      expand: "Expand",
      collapse: "Collapse",
      tool_marker: "🔧 Called {name} (details on the right)",
      resizer_title: "Drag to resize the right panel (double-click to reset)",
      welcome: "Welcome to MCP Preflight: 1) enter and save your API key → 2) choose a workspace folder → 3) load mcp.json → 4) type an inspection command. The middle column shows the conversation and report; the right panel shows live MCP tool call details.",
      tools_full: "Tool calling: fully supported",
      tools_partial: "Tool calling: partially supported",
      tools_none: "Tool calling: not supported",
      models_api_no: "This provider has no model-list API; pick from the suggestions above or type a model name."
    }
  };

  function t(key, vars) {
    var dict = I18N[state.lang] || I18N["zh-CN"];
    var s = dict[key];
    if (s == null) s = I18N["zh-CN"][key];
    if (s == null) return key;
    if (vars) {
      for (var k in vars) {
        s = s.split("{" + k + "}").join(String(vars[k]));
      }
    }
    return s;
  }

  // 把静态文案按当前语言刷一遍（data-i18n / data-i18n-title / data-i18n-ph）
  function applyI18N() {
    var dict = I18N[state.lang] || I18N["zh-CN"];
    var nodes = document.querySelectorAll("[data-i18n]");
    for (var i = 0; i < nodes.length; i++) {
      var k = nodes[i].getAttribute("data-i18n");
      if (dict[k] != null) nodes[i].textContent = dict[k];
    }
    var titles = document.querySelectorAll("[data-i18n-title]");
    for (var j = 0; j < titles.length; j++) {
      var tk = titles[j].getAttribute("data-i18n-title");
      if (dict[tk] != null) titles[j].setAttribute("title", dict[tk]);
    }
    var phs = document.querySelectorAll("[data-i18n-ph]");
    for (var m = 0; m < phs.length; m++) {
      var pk = phs[m].getAttribute("data-i18n-ph");
      if (dict[pk] != null) phs[m].setAttribute("placeholder", dict[pk]);
    }
    if (document.documentElement) document.documentElement.setAttribute("lang", state.lang);
  }

  function setLang(lang, persist) {
    state.lang = (lang === "en") ? "en" : "zh-CN";
    applyI18N();
    renderProviderOptions();
    if (persist) saveSettings();
  }

  function setTheme(theme, persist) {
    state.theme = (theme === "dark") ? "dark" : "light";
    if (document.body) document.body.classList.toggle("dark", state.theme === "dark");
    if (persist) saveSettings();
  }

  // 极简 Markdown 渲染（标题/列表/代码块/行内代码/粗斜体/引用/链接/表格/分隔线）
  function md(src) {
    return renderBlocks(src);
  }

  // 分块渲染实现：
  function renderBlocks(src) {
    var codeBlocks = [];
    var text = String(src || "").replace(/```(\w*)\n?([\s\S]*?)```/g, function (_, lang, code) {
      codeBlocks.push("<pre><code>" + esc(code.replace(/\n$/, "")) + "</code></pre>");
      return "\u0000CB" + (codeBlocks.length - 1) + "\u0000";
    });
    var lines = text.split("\n");
    var html = [], inList = null, inTable = false, para = [];
    function flushPara() {
      if (para.length) {
        html.push("<p>" + inline(para.join(" ")) + "</p>");
        para = [];
      }
    }
    function closeList() {
      if (inList) { html.push("</" + inList + ">"); inList = null; }
    }
    function closeTable() {
      if (inTable) { html.push("</table>"); inTable = false; }
    }
    function inline(s) {
      var codes = [];
      s = s.replace(/`([^`\n]+)`/g, function (_, c) { codes.push("<code>" + esc(c) + "</code>"); return "\u0000IC" + (codes.length - 1) + "\u0000"; });
      s = esc(s);
      s = s.replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>");
      s = s.replace(/^\*([^*\n]+)\*/g, "<i>$1</i>");
      s = s.replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
      s = s.replace(/\u0000IC(\d+)\u0000/g, function (_, i) { return codes[+i]; });
      return s;
    }
    for (var i = 0; i < lines.length; i++) {
      var line = lines[i];
      var m;
      if ((m = line.match(/^\u0000CB(\d+)\u0000$/))) {
        flushPara(); closeList(); closeTable();
        html.push(codeBlocks[+m[1]]);
        continue;
      }
      if (/^\s*$/.test(line)) { flushPara(); closeList(); closeTable(); continue; }
      if ((m = line.match(/^(#{1,4})\s+(.*)$/))) {
        flushPara(); closeList(); closeTable();
        html.push("<h" + m[1].length + ">" + inline(m[2]) + "</h" + m[1].length + ">");
        continue;
      }
      if (/^\s*(---|\*\*\*|___)\s*$/.test(line)) { flushPara(); closeList(); closeTable(); html.push("<hr>"); continue; }
      if ((m = line.match(/^\s*[-*+]\s+(.*)$/))) {
        flushPara(); closeTable();
        if (inList !== "ul") { closeList(); html.push("<ul>"); inList = "ul"; }
        html.push("<li>" + inline(m[1]) + "</li>");
        continue;
      }
      if ((m = line.match(/^\s*\d+[.、]\s*(.*)$/))) {
        flushPara(); closeTable();
        if (inList !== "ol") { closeList(); html.push("<ol>"); inList = "ol"; }
        html.push("<li>" + inline(m[1]) + "</li>");
        continue;
      }
      if ((m = line.match(/^\s*>\s?(.*)$/))) {
        flushPara(); closeList(); closeTable();
        html.push("<blockquote>" + inline(m[1]) + "</blockquote>");
        continue;
      }
      if (line.indexOf("|") !== -1 && lines[i + 1] && /^\s*\|?[\s:|-]+\|[\s:|-]*$/.test(lines[i + 1])) {
        flushPara(); closeList();
        html.push("<table>");
        var heads = line.split("|").map(function (x) { return x.trim(); });
        if (heads.length && heads[0] === "") heads.shift();
        if (heads.length && heads[heads.length - 1] === "") heads.pop();
        html.push("<tr>" + heads.map(function (h) { return "<th>" + inline(h) + "</th>"; }).join("") + "</tr>");
        i++;
        inTable = true;
        continue;
      }
      if (inTable) {
        if (line.indexOf("|") !== -1) {
          var cells = line.split("|").map(function (x) { return x.trim(); });
          if (cells.length && cells[0] === "") cells.shift();
          if (cells.length && cells[cells.length - 1] === "") cells.pop();
          html.push("<tr>" + cells.map(function (c) { return "<td>" + inline(c) + "</td>"; }).join("") + "</tr>");
          continue;
        }
        closeTable();
      }
      para.push(line.trim());
    }
    flushPara(); closeList(); closeTable();
    var out = html.join("\n");
    out = out.replace(/\u0000CB(\d+)\u0000/g, function (_, i) { return codeBlocks[+i]; });
    return out;
  }

  function api(method, url, body) {
    return fetch(url, {
      method: method,
      headers: body ? { "Content-Type": "application/json" } : {},
      body: body ? JSON.stringify(body) : undefined
    }).then(function (r) {
      return r.json().then(function (j) { return { status: r.status, data: j }; });
    });
  }

  /* ---------------- 侧边栏：模型服务商 ---------------- */
  function findProvider(id) {
    for (var i = 0; i < state.providers.length; i++) {
      if (state.providers[i].id === id) return state.providers[i];
    }
    return null;
  }

  function providerLabel(p) {
    if (p.id === "custom") return t("provider_custom");
    if (state.lang === "en" && p.name_en) return p.name_en;
    return p.name;
  }

  function loadProviders() {
    api("GET", "/api/providers").then(function (r) {
      state.providers = (r.data && r.data.providers) || [];
      renderProviderOptions();
    });
  }

  function renderProviderOptions() {
    var sel = $("cfg-provider");
    if (!sel) return;
    sel.innerHTML = state.providers.map(function (p) {
      return '<option value="' + esc(p.id) + '">' + esc(providerLabel(p)) + "</option>";
    }).join("");
    // 配置里出现未知 ID（如手工编辑过 config.json）时回退到第一个，避免下拉空白
    var known = state.providers.some(function (p) { return p.id === state.provider; });
    if (!known && state.providers.length) state.provider = state.providers[0].id;
    sel.value = state.provider || "deepseek";
    applyProviderMeta();
  }

  // 根据当前服务商刷新：模型候选、工具调用支持度提示
  function applyProviderMeta() {
    var p = findProvider(state.provider);
    var dl = $("model-list");
    if (dl) {
      dl.innerHTML = (p && p.models ? p.models : []).map(function (m) {
        return '<option value="' + esc(m) + '"></option>';
      }).join("");
    }
    var note = $("provider-note");
    if (note && p) {
      var lines = [];
      if (p.tools === "full") lines.push(t("tools_full"));
      else if (p.tools === "partial") lines.push(t("tools_partial"));
      else if (p.tools === "none") lines.push(t("tools_none"));
      if (p.tools_note) lines.push(state.lang === "en" && p.tools_note_en ? p.tools_note_en : p.tools_note);
      if (p.key_hint) lines.push(state.lang === "en" && p.key_hint_en ? p.key_hint_en : p.key_hint);
      if (!p.models_api && p.id !== "custom") lines.push(t("models_api_no"));
      note.textContent = lines.join("\n");
    }
  }

  /* ---------------- 侧边栏：配置 ---------------- */
  function currentConfigBody() {
    return {
      provider: state.provider,
      base_url: $("cfg-base").value.trim(),
      api_key: $("cfg-key").value.trim(),
      model: $("cfg-model").value.trim(),
      mcp_config: $("cfg-mcp").value.trim(),
      lang: state.lang,
      theme: state.theme
    };
  }

  function loadConfig() {
    api("GET", "/api/config").then(function (r) {
      var c = r.data;
      state.provider = c.provider || "deepseek";
      state.lang = (c.lang === "en") ? "en" : "zh-CN";
      state.theme = (c.theme === "dark") ? "dark" : "light";
      $("cfg-base").value = c.base_url || "";
      $("cfg-key").value = c.api_key || "";
      $("cfg-model").value = c.model || "";
      $("cfg-mcp").value = c.mcp_config || "";
      $("cfg-lang").value = state.lang;
      $("cfg-theme").value = state.theme;
      $("ws-path").textContent = c.workspace ? c.workspace : t("ws_none");
      setTheme(state.theme, false);
      applyI18N();
      renderProviderOptions();
      loadServers();
    });
  }

  function showCfgMsg(text, isError) {
    var el = $("cfg-msg");
    el.style.color = isError ? "var(--err)" : "var(--ok)";
    el.textContent = text;
    setTimeout(function () { el.textContent = ""; }, 4000);
  }

  function saveSettings() {
    api("POST", "/api/config", currentConfigBody()).then(function (r) {
      if (!(r.data && r.data.ok)) {
        showCfgMsg((r.data && r.data.error) || t("save_failed"), true);
      }
    });
  }

  $("btn-save-cfg").addEventListener("click", function () {
    api("POST", "/api/config", currentConfigBody()).then(function (r) {
      var el = $("cfg-msg");
      if (r.data && r.data.ok) {
        el.style.color = "var(--ok)";
        el.textContent = t("saved");
        if (r.data.mcp_reloaded) loadServers();
      } else {
        el.style.color = "var(--err)";
        el.textContent = (r.data && r.data.error) || t("save_failed");
      }
      setTimeout(function () { el.textContent = ""; }, 4000);
    });
  });

  $("cfg-provider").addEventListener("change", function () {
    state.provider = this.value;
    var p = findProvider(state.provider);
    if (p && p.base_url) {
      $("cfg-base").value = p.base_url;
      if (p.models && p.models.length) $("cfg-model").value = p.models[0];
    }
    applyProviderMeta();
    saveSettings();
  });

  $("cfg-lang").addEventListener("change", function () { setLang(this.value, true); });
  $("cfg-theme").addEventListener("change", function () { setTheme(this.value, true); });

  $("btn-models").addEventListener("click", function () {
    var btn = this;
    btn.disabled = true; btn.textContent = "…";
    api("POST", "/api/config", currentConfigBody()).then(function () {
      return api("GET", "/api/models");
    }).then(function (r) {
      if (r.data && r.data.ok && r.data.models && r.data.models.length) {
        state.modelList = r.data.models;
        renderModelPicker(r.data.models);
      } else {
        showCfgMsg((r.data && r.data.error) || t("fetch_failed"), true);
      }
    }).catch(function (e) {
      showCfgMsg(t("fetch_error") + e, true);
    }).finally(function () { btn.disabled = false; btn.textContent = t("btn_fetch"); });
  });

  function renderModelPicker(models) {
    var old = $("model-picker");
    if (old) old.remove();
    var sel = document.createElement("select");
    sel.id = "model-picker";
    sel.innerHTML = models.map(function (m) { return '<option value="' + esc(m) + '">' + esc(m) + "</option>"; }).join("");
    sel.addEventListener("change", function () { $("cfg-model").value = sel.value; });
    $("cfg-model").parentNode.insertBefore(sel, $("cfg-model").nextSibling);
    showCfgMsg(t("models_got", { n: models.length }), false);
  }

  $("btn-pick").addEventListener("click", function () {
    api("POST", "/api/pick-folder").then(function (r) {
      if (r.data && r.data.ok && r.data.path) {
        $("ws-path").textContent = r.data.path;
      }
    });
  });

  /* ---------------- 侧边栏：MCP 服务器 ---------------- */
  function loadServers() {
    api("GET", "/api/servers").then(function (r) {
      renderServers(r.data.servers || []);
    });
  }

  function renderServers(servers) {
    var box = $("server-list");
    box.innerHTML = "";
    if (!servers.length) {
      box.innerHTML = '<div class="empty">' + esc(t("servers_empty")) + "</div>";
      return;
    }
    servers.forEach(function (s) {
      var div = document.createElement("div");
      div.className = "server";
      var statusText = s.status === "ok" ? t("status_ok") : (s.status === "unsupported" ? t("status_unsupported") : t("status_error"));
      var html = '<div class="head"><span class="dot ' + s.status + '"></span>' +
        '<span class="name" title="' + esc(s.name) + '">' + esc(s.name) + "</span>" +
        '<span class="hint">' + statusText + "</span></div>" +
        '<div class="meta">' + esc(s.type) + (s.url ? " · " + esc(s.url) : "") + "</div>";
      if (s.error) html += '<div class="err">' + esc(s.error) + "</div>";
      if (s.tools && s.tools.length) {
        html += '<div class="tools">' + s.tools.map(function (t2) {
          return '<div class="tool"><b>' + esc(t2.name) + "</b>" +
            (t2.description ? '<span class="desc">' + esc(t2.description) + "</span>" : "") + "</div>";
        }).join("") + "</div>";
      } else if (s.status === "ok") {
        html += '<div class="tools"><div class="empty">' + esc(t("no_tools")) + "</div></div>";
      }
      div.innerHTML = html;
      div.querySelector(".head").addEventListener("click", function () {
        var t3 = div.querySelector(".tools");
        if (t3) t3.classList.toggle("open");
      });
      box.appendChild(div);
    });
  }

  $("btn-reload").addEventListener("click", function () {
    var btn = this;
    btn.disabled = true; btn.textContent = "…";
    api("POST", "/api/config", currentConfigBody())
      .then(function () { return api("POST", "/api/servers/reload"); })
      .then(function (r) {
        renderServers((r.data && r.data.servers) || []);
      })
      .catch(function () {})
      .finally(function () { btn.disabled = false; btn.textContent = t("btn_load"); });
  });

  $("btn-reset").addEventListener("click", function () {
    api("POST", "/api/reset").then(function () {
      $("chat").innerHTML = "";
      resetToolPanel();
      addSys(t("new_session_started"));
    });
  });

  $("btn-clear-tools").addEventListener("click", function () {
    resetToolPanel();
  });

  /* ---------------- 聊天区 ---------------- */
  var chatEl = $("chat");

  function scrollBottom() { chatEl.scrollTop = chatEl.scrollHeight; }

  function addUser(text) {
    var d = document.createElement("div");
    d.className = "msg user";
    d.innerHTML = '<div class="role">' + esc(t("role_user")) + '</div><div class="bubble">' + esc(text) + "</div>";
    chatEl.appendChild(d); scrollBottom();
  }

  function addAssistantBox() {
    var d = document.createElement("div");
    d.className = "msg assistant";
    d.innerHTML = '<div class="role">' + esc(t("role_assistant")) + '</div><div class="bubble md"></div>';
    chatEl.appendChild(d); scrollBottom();
    return d.querySelector(".bubble");
  }

  /* ================= 右侧 MCP 工具调用面板 ================= */
  var toolQueue = {}; // 工具名 -> 待填结果的卡片数组

  function scrollPanel() {
    var el = $("mcp-log");
    if (el) el.scrollTop = el.scrollHeight;
  }

  function resetToolPanel() {
    var log = $("mcp-log");
    log.innerHTML = "";
    toolQueue = {};
    var hint = document.createElement("div");
    hint.id = "mcp-empty";
    hint.className = "empty";
    hint.textContent = t("mcp_empty");
    log.appendChild(hint);
  }

  // 中间对话流里的轻量提示，避免打断 LLM 反馈的阅读
  function addToolMarker(name) {
    var d = document.createElement("div");
    d.className = "toolmark";
    d.textContent = t("tool_marker", { name: name });
    chatEl.appendChild(d); scrollBottom();
  }

  function addToolCall(name, args) {
    var empty = $("mcp-empty");
    if (empty) empty.parentNode.removeChild(empty);
    var d = document.createElement("div");
    d.className = "tcall";
    var head = document.createElement("div");
    head.className = "tcall-head";
    head.innerHTML = '<span class="dot"></span><span class="tname">' + esc(name) + '</span><span class="tstate">' + esc(t("tool_calling")) + "</span>";
    d.appendChild(head);
    if (args) {
      // 参数：浅黄色块
      var block = document.createElement("div");
      block.className = "block args";
      var lab = document.createElement("div");
      lab.className = "lab";
      lab.textContent = t("tool_args");
      block.appendChild(lab);
      var pre = document.createElement("pre");
      pre.textContent = args;
      block.appendChild(pre);
      d.appendChild(block);
    }
    $("mcp-log").appendChild(d);
    if (!toolQueue[name]) toolQueue[name] = [];
    toolQueue[name].push(d);
    scrollPanel();
  }

  function addToolResult(name, result, failed) {
    var q = toolQueue[name];
    var card = (q && q.length) ? q.shift() : null;
    if (!card) {
      // 没有配对的调用记录（异常情况）：单独建卡
      var empty = $("mcp-empty");
      if (empty) empty.parentNode.removeChild(empty);
      card = document.createElement("div");
      card.className = "tcall";
      card.innerHTML = '<div class="tcall-head"><span class="dot"></span><span class="tname">' + esc(name) + '</span><span class="tstate"></span></div>';
      $("mcp-log").appendChild(card);
    }
    var dot = card.querySelector(".dot");
    var stateEl = card.querySelector(".tstate");
    if (failed) {
      card.classList.add("failed");
      dot.classList.add("error");
      stateEl.textContent = t("tool_failed");
    } else {
      dot.classList.add("ok");
      stateEl.textContent = t("tool_done");
    }
    // 返回：浅绿色块（失败为浅红色）
    var block = document.createElement("div");
    block.className = "block result" + (failed ? " failed" : "");
    var lab = document.createElement("div");
    lab.className = "lab";
    lab.textContent = failed ? t("tool_error") : t("tool_result");
    var len = (result || "").length;
    if (len) {
      var lenSpan = document.createElement("span");
      lenSpan.className = "len";
      lenSpan.textContent = t("chars_suffix", { n: len });
      lab.appendChild(lenSpan);
    }
    block.appendChild(lab);
    var pre = document.createElement("pre");
    pre.textContent = result || "";
    block.appendChild(pre);
    card.appendChild(block);
    if (len > 1200) {
      pre.classList.add("clamped");
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "expand-btn";
      btn.textContent = t("expand");
      btn.addEventListener("click", function () {
        var open = pre.classList.toggle("open");
        btn.textContent = open ? t("collapse") : t("expand");
        scrollPanel();
      });
      block.appendChild(btn);
    }
    scrollPanel();
  }

  /* ---------- 右侧面板宽度拖拽 ---------- */
  (function () {
    var resizer = $("panel-resizer");
    var panel = $("mcp-panel");
    if (!resizer || !panel) return;
    var KEY = "mcppreflight.panel-width";
    var saved = null;
    try { saved = window.localStorage ? window.localStorage.getItem(KEY) : null; } catch (e) { saved = null; }
    if (saved) {
      var w0 = parseInt(saved, 10);
      if (w0 >= 260 && w0 <= 900) panel.style.width = w0 + "px";
    }
    var startX = 0, startW = 0, dragging = false;
    resizer.addEventListener("mousedown", function (e) {
      dragging = true;
      startX = e.clientX;
      startW = panel.offsetWidth || 352;
      resizer.classList.add("active");
      document.body.classList.add("resizing");
      e.preventDefault();
    });
    document.addEventListener("mousemove", function (e) {
      if (!dragging) return;
      // 向左拖动 → 面板变宽
      var w = startW + (startX - e.clientX);
      var max = Math.max(280, (window.innerWidth || 1280) - 460);
      w = Math.max(260, Math.min(max, w));
      panel.style.width = w + "px";
    });
    document.addEventListener("mouseup", function () {
      if (!dragging) return;
      dragging = false;
      resizer.classList.remove("active");
      document.body.classList.remove("resizing");
      try {
        if (window.localStorage) window.localStorage.setItem(KEY, String(panel.offsetWidth));
      } catch (e) { /* 忽略存储失败 */ }
    });
    // 双击恢复默认宽度
    resizer.addEventListener("dblclick", function () {
      panel.style.width = "";
      try { if (window.localStorage) window.localStorage.removeItem(KEY); } catch (e) { /* 忽略 */ }
    });
  })();

  function addSys(text, isError) {
    var d = document.createElement("div");
    d.className = "sysline" + (isError ? " error" : "");
    d.textContent = text;
    chatEl.appendChild(d); scrollBottom();
  }

  function send() {
    var text = $("input").value.trim();
    if (!text || state.sending) return;
    if (!$("cfg-key").value.trim()) {
      addSys(t("need_key_first"), true);
      return;
    }
    state.sending = true;
    $("btn-send").disabled = true;
    $("input").value = "";
    addUser(text);

    var bubble = addAssistantBox();
    var acc = "";
    var hasContent = false;

    fetch("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: text })
    }).then(function (resp) {
      if (!resp.ok) {
        return resp.text().then(function (t2) { throw new Error(t2 || ("HTTP " + resp.status)); });
      }
      var reader = resp.body.getReader();
      var decoder = new TextDecoder();
      var buf = "";
      function pump() {
        return reader.read().then(function (r) {
          if (r.done) { finish(); return; }
          buf += decoder.decode(r.value, { stream: true });
          var idx;
          while ((idx = buf.indexOf("\n\n")) !== -1) {
            var rawEvent = buf.slice(0, idx);
            buf = buf.slice(idx + 2);
            handleEvent(rawEvent);
          }
          return pump();
        });
      }
      function handleEvent(raw) {
        var line = raw.split("\n").filter(function (l) { return l.indexOf("data:") === 0; })[0];
        if (!line) return;
        var payload;
        try { payload = JSON.parse(line.slice(5).trim()); } catch (e) { return; }
        if (payload.type === "assistant_delta") {
          hasContent = true;
          acc += payload.text;
          bubble.innerHTML = md(acc) + '<span class="typing"></span>';
          scrollBottom();
        } else if (payload.type === "tool_call") {
          addToolMarker(payload.name);
          addToolCall(payload.name, payload.args);
        } else if (payload.type === "tool_result") {
          addToolResult(payload.name, payload.text, !!payload.failed);
        } else if (payload.type === "error") {
          addSys(payload.text || t("error_generic"), true);
        } else if (payload.type === "done") {
          // 完成
        }
      }
      function finish() {
        if (hasContent) {
          bubble.innerHTML = md(acc);
        } else {
          bubble.innerHTML = '<span class="hint">' + esc(t("no_output")) + "</span>";
        }
        state.sending = false;
        $("btn-send").disabled = false;
        scrollBottom();
      }
      return pump();
    }).catch(function (e) {
      if (!hasContent) bubble.innerHTML = '<span class="hint">' + esc(t("no_output")) + "</span>";
      addSys(t("request_failed") + (e && e.message ? e.message : e), true);
      state.sending = false;
      $("btn-send").disabled = false;
    });
  }

  $("btn-send").addEventListener("click", send);
  $("input").addEventListener("keydown", function (e) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  });

  /* ---------------- 初始化 ---------------- */
  loadConfig();
  loadProviders();
  addSys(t("welcome"));
})();
