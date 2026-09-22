/* MCP点检助手 前端逻辑（无外部依赖，内网离线可用） */
(function () {
  "use strict";

  var $ = function (id) { return document.getElementById(id); };
  var state = { sending: false, modelList: [] };

  /* ---------------- 工具函数 ---------------- */
  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
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

  /* ---------------- 侧边栏：配置 ---------------- */
  function loadConfig() {
    api("GET", "/api/config").then(function (r) {
      var c = r.data;
      $("cfg-base").value = c.base_url || "";
      $("cfg-key").value = c.api_key || "";
      $("cfg-model").value = c.model || "";
      $("cfg-mcp").value = c.mcp_config || "";
      $("ws-path").textContent = c.workspace ? c.workspace : "未选择";
    });
  }

  $("btn-save-cfg").addEventListener("click", function () {
    api("POST", "/api/config", {
      base_url: $("cfg-base").value.trim(),
      api_key: $("cfg-key").value.trim(),
      model: $("cfg-model").value.trim(),
      mcp_config: $("cfg-mcp").value.trim()
    }).then(function (r) {
      var el = $("cfg-msg");
      if (r.data && r.data.ok) {
        el.style.color = "var(--ok)";
        el.textContent = "已保存";
        if (r.data.mcp_reloaded) loadServers();
      } else {
        el.style.color = "var(--err)";
        el.textContent = (r.data && r.data.error) || "保存失败";
      }
      setTimeout(function () { el.textContent = ""; }, 4000);
    });
  });

  $("btn-models").addEventListener("click", function () {
    var btn = this;
    btn.disabled = true; btn.textContent = "…";
    api("POST", "/api/config", {
      base_url: $("cfg-base").value.trim(),
      api_key: $("cfg-key").value.trim(),
      model: $("cfg-model").value.trim(),
      mcp_config: $("cfg-mcp").value.trim()
    }).then(function () {
      return api("GET", "/api/models");
    }).then(function (r) {
      if (r.data && r.data.ok && r.data.models && r.data.models.length) {
        state.modelList = r.data.models;
        renderModelPicker(r.data.models);
      } else {
        $("cfg-msg").style.color = "var(--err)";
        $("cfg-msg").textContent = (r.data && r.data.error) || "获取模型列表失败";
      }
    }).catch(function (e) {
      $("cfg-msg").style.color = "var(--err)";
      $("cfg-msg").textContent = "获取失败: " + e;
    }).finally(function () { btn.disabled = false; btn.textContent = "获取"; });
  });

  function renderModelPicker(models) {
    var old = $("model-picker");
    if (old) old.remove();
    var sel = document.createElement("select");
    sel.id = "model-picker";
    sel.innerHTML = models.map(function (m) { return '<option value="' + esc(m) + '">' + esc(m) + "</option>"; }).join("");
    sel.addEventListener("change", function () { $("cfg-model").value = sel.value; });
    $("cfg-model").parentNode.insertBefore(sel, $("cfg-model").nextSibling);
    $("cfg-msg").style.color = "var(--ok)";
    $("cfg-msg").textContent = "已获取 " + models.length + " 个模型";
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
      box.innerHTML = '<div class="empty">尚未加载任何 MCP 服务器</div>';
      return;
    }
    servers.forEach(function (s) {
      var div = document.createElement("div");
      div.className = "server";
      var statusText = s.status === "ok" ? "已连接" : (s.status === "unsupported" ? "暂不支持" : "连接失败");
      var html = '<div class="head"><span class="dot ' + s.status + '"></span>' +
        '<span class="name" title="' + esc(s.name) + '">' + esc(s.name) + "</span>" +
        '<span class="hint">' + statusText + "</span></div>" +
        '<div class="meta">' + esc(s.type) + (s.url ? " · " + esc(s.url) : "") + "</div>";
      if (s.error) html += '<div class="err">' + esc(s.error) + "</div>";
      if (s.tools && s.tools.length) {
        html += '<div class="tools">' + s.tools.map(function (t) {
          return '<div class="tool"><b>' + esc(t.name) + "</b>" +
            (t.description ? '<span class="desc">' + esc(t.description) + "</span>" : "") + "</div>";
        }).join("") + "</div>";
      } else if (s.status === "ok") {
        html += '<div class="tools"><div class="empty">该服务器未提供工具</div></div>';
      }
      div.innerHTML = html;
      div.querySelector(".head").addEventListener("click", function () {
        var t = div.querySelector(".tools");
        if (t) t.classList.toggle("open");
      });
      box.appendChild(div);
    });
  }

  $("btn-reload").addEventListener("click", function () {
    var btn = this;
    btn.disabled = true; btn.textContent = "…";
    api("POST", "/api/config", {
      base_url: $("cfg-base").value.trim(),
      api_key: $("cfg-key").value.trim(),
      model: $("cfg-model").value.trim(),
      mcp_config: $("cfg-mcp").value.trim()
    }).then(function () { return api("POST", "/api/servers/reload"); })
      .then(function (r) {
        renderServers((r.data && r.data.servers) || []);
      })
      .catch(function () {})
      .finally(function () { btn.disabled = false; btn.textContent = "加载"; });
  });

  $("btn-reset").addEventListener("click", function () {
    api("POST", "/api/reset").then(function () {
      $("chat").innerHTML = "";
      addSys("已开始新会话");
    });
  });

  /* ---------------- 聊天区 ---------------- */
  var chatEl = $("chat");

  function scrollBottom() { chatEl.scrollTop = chatEl.scrollHeight; }

  function addUser(text) {
    var d = document.createElement("div");
    d.className = "msg user";
    d.innerHTML = '<div class="role">你</div><div class="bubble">' + esc(text) + "</div>";
    chatEl.appendChild(d); scrollBottom();
  }

  function addAssistantBox() {
    var d = document.createElement("div");
    d.className = "msg assistant";
    d.innerHTML = '<div class="role">助手</div><div class="bubble md"></div>';
    chatEl.appendChild(d); scrollBottom();
    return d.querySelector(".bubble");
  }

  function addToolCard(kind, name, args, result) {
    var d = document.createElement("div");
    d.className = "msg assistant";
    var html = '<div class="role">助手</div>';
    if (kind === "call") {
      html += '<div class="toolcard"><span class="t">🔧 调用 MCP 工具：' + esc(name) + "</span><pre>" + esc(args || "{}") + "</pre></div>";
    } else {
      html += '<div class="resultcard"><span class="t">📄 工具返回：' + esc(name) + "</span><pre>" + esc(result || "") + "</pre></div>";
    }
    d.innerHTML = html;
    chatEl.appendChild(d); scrollBottom();
    return d;
  }

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
      addSys("请先在上方填写 API Key 并保存", true);
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
        return resp.text().then(function (t) { throw new Error(t || ("HTTP " + resp.status)); });
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
          addToolCard("call", payload.name, payload.args);
        } else if (payload.type === "tool_result") {
          addToolCard("result", payload.name, null, payload.text);
        } else if (payload.type === "error") {
          addSys(payload.text || "发生错误", true);
        } else if (payload.type === "done") {
          // 完成
        }
      }
      function finish() {
        if (hasContent) {
          bubble.innerHTML = md(acc);
        } else {
          bubble.innerHTML = '<span class="hint">（无文本输出）</span>';
        }
        state.sending = false;
        $("btn-send").disabled = false;
        scrollBottom();
      }
      return pump();
    }).catch(function (e) {
      if (!hasContent) bubble.innerHTML = '<span class="hint">（无文本输出）</span>';
      addSys("请求失败: " + (e && e.message ? e.message : e), true);
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
  loadServers();
  addSys("欢迎使用 MCP点检助手：① 填写 DeepSeek API Key 并保存 → ② 选择工作区文件夹 → ③ 加载 mcp.json → ④ 输入点检指令");
})();
