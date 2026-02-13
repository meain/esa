// esa web client
(function () {
    "use strict";

    // -- DOM Elements --
    var messagesEl = document.getElementById("messages");
    var inputEl = document.getElementById("user-input");
    var sendBtn = document.getElementById("send-btn");
    var agentListEl = document.getElementById("agent-list");
    var historyListEl = document.getElementById("history-list");
    var currentAgentEl = document.getElementById("current-agent");
    var statusEl = document.getElementById("connection-status");
    var approvalModal = document.getElementById("approval-modal");
    var approvalCommand = document.getElementById("approval-command");
    var approveBtn = document.getElementById("approve-btn");
    var denyBtn = document.getElementById("deny-btn");
    var denyMessageInput = document.getElementById("deny-message");
    var newChatBtn = document.getElementById("new-chat-btn");
    var sidebarToggle = document.getElementById("sidebar-toggle");
    var sidebar = document.getElementById("sidebar");
    var themeToggle = document.getElementById("theme-toggle");
    var agentSearch = document.getElementById("agent-search");
    var historySearch = document.getElementById("history-search");
    var modelSelector = document.getElementById("model-selector");
    var exportBtn = document.getElementById("export-btn");

    // -- State --
    var ws = null;
    var selectedAgent = "+default";
    var selectedModel = "";
    var isStreaming = false;
    var currentStreamEl = null;
    var currentStreamRaw = "";
    var pendingApprovalId = null;
    var currentConversationId = null;
    var cachedAgents = [];
    var streamRenderTimer = null;
    var lastStreamRender = 0;

    // -- Theme --
    function initTheme() {
        var saved = localStorage.getItem("esa-theme") || "dark";
        document.documentElement.setAttribute("data-theme", saved);
        updateThemeIcon(saved);
    }

    function toggleTheme() {
        var current = document.documentElement.getAttribute("data-theme");
        var next = current === "dark" ? "light" : "dark";
        document.documentElement.setAttribute("data-theme", next);
        localStorage.setItem("esa-theme", next);
        updateThemeIcon(next);
    }

    function updateThemeIcon(theme) {
        themeToggle.textContent = theme === "dark" ? "\u263E" : "\u2600";
    }

    // -- WebSocket --
    function connect() {
        var proto = location.protocol === "https:" ? "wss:" : "ws:";
        ws = new WebSocket(proto + "//" + location.host + "/ws");

        ws.onopen = function () {
            setStatus("connected");
        };

        ws.onclose = function () {
            setStatus("disconnected");
            setTimeout(connect, 2000);
        };

        ws.onerror = function () {
            setStatus("disconnected");
        };

        ws.onmessage = function (evt) {
            var msg = JSON.parse(evt.data);
            handleMessage(msg);
        };
    }

    function setStatus(state) {
        statusEl.className = "status-" + state;
        statusEl.title = state.charAt(0).toUpperCase() + state.slice(1);
    }

    function sendMessage(text) {
        if (!ws || ws.readyState !== WebSocket.OPEN) return;
        if (!text.trim()) return;

        // Clear welcome screen if present
        var welcome = messagesEl.querySelector(".welcome");
        if (welcome) welcome.remove();

        appendUserMessage(text);

        var payload = {
            type: currentConversationId ? "continue" : "message",
            content: text,
            agent: selectedAgent,
        };
        if (currentConversationId) {
            payload.id = currentConversationId;
        }
        if (selectedModel) {
            payload.model = selectedModel;
        }

        ws.send(JSON.stringify(payload));

        isStreaming = true;
        sendBtn.disabled = true;
        inputEl.value = "";
        inputEl.style.height = "auto";
    }

    function sendApproval(approved, message) {
        if (!ws || ws.readyState !== WebSocket.OPEN) return;
        ws.send(JSON.stringify({
            type: "approval",
            id: pendingApprovalId,
            approved: approved,
            message: message || "",
        }));
        pendingApprovalId = null;
        hideApprovalModal();
    }

    // -- Message Handling --
    function handleMessage(msg) {
        switch (msg.type) {
            case "token":
                handleToken(msg.content);
                break;
            case "tool_call":
                handleToolCall(msg);
                break;
            case "tool_result":
                handleToolResult(msg);
                break;
            case "done":
                handleDone(msg);
                break;
            case "error":
                appendError(msg.content);
                finishStream();
                break;
        }
    }

    function handleToken(content) {
        if (!currentStreamEl) {
            var msgDiv = createMessageDiv("assistant");
            var contentDiv = msgDiv.querySelector(".message-content");
            contentDiv.classList.add("streaming-cursor");
            currentStreamEl = contentDiv;
            currentStreamRaw = "";
            messagesEl.appendChild(msgDiv);
        }
        currentStreamRaw += content;
        renderStreamThrottled();
        scrollToBottom();
    }

    function renderStreamThrottled() {
        var now = Date.now();
        if (now - lastStreamRender > 50) {
            renderStreamNow();
        } else if (!streamRenderTimer) {
            streamRenderTimer = setTimeout(renderStreamNow, 50);
        }
    }

    function renderStreamNow() {
        if (streamRenderTimer) {
            clearTimeout(streamRenderTimer);
            streamRenderTimer = null;
        }
        lastStreamRender = Date.now();
        if (currentStreamEl) {
            currentStreamEl.innerHTML = renderMarkdown(currentStreamRaw);
            currentStreamEl.setAttribute("data-raw", currentStreamRaw);
        }
    }

    function handleDone(msg) {
        // Store conversation ID from server for thread continuity
        if (msg.id) {
            currentConversationId = msg.id;
        }
        finishStream();
    }

    function finishStream() {
        if (streamRenderTimer) {
            clearTimeout(streamRenderTimer);
            streamRenderTimer = null;
        }
        if (currentStreamEl) {
            currentStreamEl.classList.remove("streaming-cursor");
            currentStreamEl.setAttribute("data-raw", currentStreamRaw);
            currentStreamEl.innerHTML = renderMarkdown(currentStreamRaw);
            currentStreamEl = null;
            currentStreamRaw = "";
        }
        isStreaming = false;
        sendBtn.disabled = false;
        inputEl.focus();
        scrollToBottom();
        loadHistory();
    }

    function handleToolCall(msg) {
        if (streamRenderTimer) {
            clearTimeout(streamRenderTimer);
            streamRenderTimer = null;
        }
        if (currentStreamEl) {
            currentStreamEl.classList.remove("streaming-cursor");
            currentStreamEl.setAttribute("data-raw", currentStreamRaw);
            currentStreamEl.innerHTML = renderMarkdown(currentStreamRaw);
            currentStreamEl = null;
            currentStreamRaw = "";
        }

        var toolDiv = document.createElement("div");
        toolDiv.className = "tool-call";
        toolDiv.id = "tool-" + msg.id;

        var safeLabel = msg.safe ? "auto-approved" : "needs approval";
        toolDiv.innerHTML =
            '<div class="tool-call-card ' + (msg.safe ? "approved" : "pending") + '">' +
            '  <div class="tool-call-header">' +
            '    <span class="tool-icon">&#9881;</span>' +
            '    <span>' + escapeHtml(msg.name) + '</span>' +
            '    <span style="margin-left:auto;font-size:10px;color:var(--text-muted)">' + safeLabel + '</span>' +
            '    <button class="copy-btn" title="Copy command" onclick="window._esaCopy(this,\'cmd\')">&#9776;</button>' +
            '  </div>' +
            '  <div class="tool-call-command" data-raw="' + escapeAttr(msg.command) + '">$ ' + escapeHtml(msg.command) + '</div>' +
            '</div>';

        messagesEl.appendChild(toolDiv);
        scrollToBottom();

        if (!msg.safe) {
            pendingApprovalId = msg.id;
            showApprovalModal(msg.command);
        }
    }

    function handleToolResult(msg) {
        var toolDiv = document.getElementById("tool-" + msg.id);
        if (toolDiv) {
            var card = toolDiv.querySelector(".tool-call-card");
            if (card) {
                card.classList.remove("pending");
                if (msg.output && (msg.output.indexOf("Error:") === 0 || msg.output.indexOf("cancelled by user") !== -1)) {
                    card.classList.add("denied");
                } else {
                    card.classList.add("approved");
                }

                var outputDiv = document.createElement("div");
                outputDiv.className = "tool-call-output";
                outputDiv.setAttribute("data-raw", msg.output);
                outputDiv.textContent = msg.output;

                var copyBtn = document.createElement("button");
                copyBtn.className = "copy-btn";
                copyBtn.title = "Copy output";
                copyBtn.innerHTML = "&#9776;";
                copyBtn.onclick = function () { copyFromEl(copyBtn, outputDiv); };

                card.appendChild(outputDiv);
                card.querySelector(".tool-call-header").appendChild(copyBtn);
            }
        }
        scrollToBottom();
    }

    // -- Approval Modal --
    function showApprovalModal(command) {
        approvalCommand.textContent = "$ " + command;
        denyMessageInput.value = "";
        approvalModal.classList.remove("hidden");
        approveBtn.focus();
    }

    function hideApprovalModal() {
        approvalModal.classList.add("hidden");
    }

    // -- Copy --
    function copyText(text, btn) {
        navigator.clipboard.writeText(text).then(function () {
            btn.classList.add("copied");
            var orig = btn.innerHTML;
            btn.innerHTML = "&#10003;";
            setTimeout(function () {
                btn.classList.remove("copied");
                btn.innerHTML = orig;
            }, 1500);
        });
    }

    function copyFromEl(btn, el) {
        var raw = el.getAttribute("data-raw") || el.textContent;
        copyText(raw, btn);
    }

    // Expose for inline onclick in tool calls
    window._esaCopy = function (btn, type) {
        var card = btn.closest(".tool-call-card") || btn.closest(".message");
        if (type === "cmd") {
            var cmdEl = card.querySelector(".tool-call-command");
            copyText(cmdEl.getAttribute("data-raw") || cmdEl.textContent, btn);
        } else if (type === "output") {
            var outEl = card.querySelector(".tool-call-output");
            if (outEl) copyText(outEl.getAttribute("data-raw") || outEl.textContent, btn);
        }
    };

    // -- DOM Helpers --
    function createMessageDiv(role) {
        var div = document.createElement("div");
        div.className = "message";

        var roleDiv = document.createElement("div");
        roleDiv.className = "message-role " + role;

        var roleLabel = document.createElement("span");
        roleLabel.textContent = role === "user" ? "you" : "esa";

        var copyBtn = document.createElement("button");
        copyBtn.className = "copy-btn";
        copyBtn.title = "Copy";
        copyBtn.innerHTML = "&#9776;";

        roleDiv.appendChild(roleLabel);
        roleDiv.appendChild(copyBtn);

        var contentDiv = document.createElement("div");
        contentDiv.className = "message-content";

        copyBtn.addEventListener("click", function () {
            copyFromEl(copyBtn, contentDiv);
        });

        div.appendChild(roleDiv);
        div.appendChild(contentDiv);
        return div;
    }

    function appendUserMessage(text) {
        var msgDiv = createMessageDiv("user");
        var content = msgDiv.querySelector(".message-content");
        content.textContent = text;
        content.setAttribute("data-raw", text);
        messagesEl.appendChild(msgDiv);
        scrollToBottom();
    }

    function appendError(text) {
        var msgDiv = createMessageDiv("assistant");
        var content = msgDiv.querySelector(".message-content");
        content.style.color = "var(--red)";
        content.textContent = "Error: " + text;
        messagesEl.appendChild(msgDiv);
        scrollToBottom();
    }

    function scrollToBottom() {
        messagesEl.scrollTop = messagesEl.scrollHeight;
    }

    function escapeHtml(str) {
        var div = document.createElement("div");
        div.textContent = str;
        return div.innerHTML;
    }

    function escapeAttr(str) {
        return str.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }

    // -- Markdown Renderer --
    function renderMarkdown(text) {
        if (!text) return "";

        var lines = text.split("\n");
        var html = [];
        var i = 0;
        var inList = false;
        var listType = "";

        while (i < lines.length) {
            var line = lines[i];

            // Fenced code blocks
            if (line.match(/^```/)) {
                if (inList) { html.push(listType === "ul" ? "</ul>" : "</ol>"); inList = false; }
                var lang = line.replace(/^```/, "").trim();
                var codeLines = [];
                i++;
                while (i < lines.length && !lines[i].match(/^```/)) {
                    codeLines.push(lines[i]);
                    i++;
                }
                i++; // skip closing ```
                var codeText = escapeHtml(codeLines.join("\n"));
                html.push('<div class="code-block-wrapper"><button class="copy-btn" title="Copy code" onclick="window._esaCopyCode(this)">&#9776;</button><pre><code>' + codeText + '</code></pre></div>');
                continue;
            }

            // Headings
            var headingMatch = line.match(/^(#{1,6})\s+(.+)/);
            if (headingMatch) {
                if (inList) { html.push(listType === "ul" ? "</ul>" : "</ol>"); inList = false; }
                var level = headingMatch[1].length;
                html.push("<h" + level + ">" + renderInline(headingMatch[2]) + "</h" + level + ">");
                i++;
                continue;
            }

            // Horizontal rule
            if (line.match(/^(---|\*\*\*|___)\s*$/)) {
                if (inList) { html.push(listType === "ul" ? "</ul>" : "</ol>"); inList = false; }
                html.push("<hr>");
                i++;
                continue;
            }

            // Blockquote
            if (line.match(/^>\s?/)) {
                if (inList) { html.push(listType === "ul" ? "</ul>" : "</ol>"); inList = false; }
                var quoteLines = [];
                while (i < lines.length && lines[i].match(/^>\s?/)) {
                    quoteLines.push(lines[i].replace(/^>\s?/, ""));
                    i++;
                }
                html.push("<blockquote>" + renderInline(quoteLines.join("<br>")) + "</blockquote>");
                continue;
            }

            // Table
            if (line.match(/\|/) && i + 1 < lines.length && lines[i + 1].match(/^\s*\|?\s*[-:]+[-|:\s]+\s*\|?\s*$/)) {
                if (inList) { html.push(listType === "ul" ? "</ul>" : "</ol>"); inList = false; }
                var headerCells = parsePipeCells(line);
                i++; // skip separator
                i++;
                var tableHtml = "<table><thead><tr>";
                for (var h = 0; h < headerCells.length; h++) {
                    tableHtml += "<th>" + renderInline(headerCells[h]) + "</th>";
                }
                tableHtml += "</tr></thead><tbody>";
                while (i < lines.length && lines[i].match(/\|/)) {
                    var cells = parsePipeCells(lines[i]);
                    tableHtml += "<tr>";
                    for (var c = 0; c < cells.length; c++) {
                        tableHtml += "<td>" + renderInline(cells[c]) + "</td>";
                    }
                    tableHtml += "</tr>";
                    i++;
                }
                tableHtml += "</tbody></table>";
                html.push(tableHtml);
                continue;
            }

            // Unordered list
            var ulMatch = line.match(/^(\s*)[*\-+]\s+(.*)/);
            if (ulMatch) {
                if (!inList || listType !== "ul") {
                    if (inList) html.push(listType === "ul" ? "</ul>" : "</ol>");
                    html.push("<ul>");
                    inList = true;
                    listType = "ul";
                }
                html.push("<li>" + renderInline(ulMatch[2]) + "</li>");
                i++;
                continue;
            }

            // Ordered list
            var olMatch = line.match(/^(\s*)\d+\.\s+(.*)/);
            if (olMatch) {
                if (!inList || listType !== "ol") {
                    if (inList) html.push(listType === "ul" ? "</ul>" : "</ol>");
                    html.push("<ol>");
                    inList = true;
                    listType = "ol";
                }
                html.push("<li>" + renderInline(olMatch[2]) + "</li>");
                i++;
                continue;
            }

            // Close list if we hit a non-list line
            if (inList) {
                html.push(listType === "ul" ? "</ul>" : "</ol>");
                inList = false;
            }

            // Empty line
            if (line.trim() === "") {
                i++;
                continue;
            }

            // Paragraph: collect consecutive non-empty lines
            var paraLines = [];
            while (i < lines.length && lines[i].trim() !== "" && !lines[i].match(/^```/) &&
                   !lines[i].match(/^#{1,6}\s/) && !lines[i].match(/^[*\-+]\s/) &&
                   !lines[i].match(/^\d+\.\s/) && !lines[i].match(/^>\s?/) &&
                   !lines[i].match(/^(---|\*\*\*|___)\s*$/)) {
                paraLines.push(lines[i]);
                i++;
            }
            html.push("<p>" + renderInline(paraLines.join("<br>")) + "</p>");
        }

        if (inList) html.push(listType === "ul" ? "</ul>" : "</ol>");

        return html.join("\n");
    }

    function renderInline(text) {
        // Escape HTML entities but preserve <br> tags we inserted
        text = text.replace(/<br>/g, "\x00BR\x00");
        text = escapeHtml(text);
        text = text.replace(/\x00BR\x00/g, "<br>");

        // Inline code (before other formatting)
        text = text.replace(/`([^`]+)`/g, '<code>$1</code>');

        // Bold
        text = text.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');

        // Italic
        text = text.replace(/(?<!\*)\*([^*]+)\*(?!\*)/g, '<em>$1</em>');

        // Strikethrough
        text = text.replace(/~~([^~]+)~~/g, '<del>$1</del>');

        // Links [text](url)
        text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');

        return text;
    }

    function parsePipeCells(line) {
        return line.split("|").map(function (c) { return c.trim(); }).filter(function (c) { return c !== ""; });
    }

    // Copy code block helper
    window._esaCopyCode = function (btn) {
        var wrapper = btn.closest(".code-block-wrapper");
        var code = wrapper.querySelector("code");
        copyText(code.textContent, btn);
    };

    // -- Agent Info Display --
    function showAgentInfo(agentName) {
        var agent = null;
        for (var i = 0; i < cachedAgents.length; i++) {
            if (cachedAgents[i].name === agentName.replace(/^\+/, "")) {
                agent = cachedAgents[i];
                break;
            }
        }

        var html = '<div class="welcome">' +
            '<h2>esa</h2>' +
            '<p>' + escapeHtml(agentName) + '</p>';

        if (agent) {
            if (agent.description) {
                html += '<p>' + escapeHtml(agent.description) + '</p>';
            }

            if (agent.functions && agent.functions.length > 0) {
                html += '<div class="agent-detail">' +
                    '<h3>Available Functions</h3>' +
                    '<ul class="fn-list">';
                for (var j = 0; j < agent.functions.length; j++) {
                    var fn = agent.functions[j];
                    var safeClass = fn.safe ? "fn-safe" : "fn-unsafe";
                    var safeLabel = fn.safe ? "safe" : "needs approval";
                    var desc = fn.description || "";
                    var descParts = desc.split("\n");
                    var shortDesc = descParts[0];
                    if (shortDesc.length > 80) shortDesc = shortDesc.substring(0, 77) + "...";
                    html += '<li>' +
                        '<span class="fn-name">' + escapeHtml(fn.name) + '</span>' +
                        '<span class="fn-desc">' + escapeHtml(shortDesc) + '</span>' +
                        '<span class="' + safeClass + '">' + safeLabel + '</span>' +
                        '</li>';
                }
                html += '</ul></div>';
            }
        }

        html += '</div>';
        messagesEl.innerHTML = html;
    }

    // -- Sidebar: Agents --
    function loadAgents() {
        fetch("/api/agents")
            .then(function (r) { return r.json(); })
            .then(function (agents) {
                cachedAgents = agents || [];
                renderAgents(cachedAgents);
                showAgentInfo(selectedAgent);
            })
            .catch(function () {
                agentListEl.innerHTML = '<div class="sidebar-item" style="color:var(--red)">Failed to load agents</div>';
            });
    }

    function renderAgents(agents) {
        agentListEl.innerHTML = "";
        if (!agents || agents.length === 0) {
            agentListEl.innerHTML = '<div class="sidebar-item" style="color:var(--text-muted)">No agents found</div>';
            return;
        }

        agents.forEach(function (agent) {
            var item = document.createElement("div");
            item.className = "sidebar-item" + (("+" + agent.name) === selectedAgent ? " active" : "");
            item.setAttribute("data-agent", agent.name);
            item.innerHTML =
                '<span class="agent-badge">+' + escapeHtml(agent.name) + '</span>' +
                (agent.is_builtin ? '<span class="builtin-tag">builtin</span>' : '') +
                (agent.description ? '<br><span style="font-size:11px;color:var(--text-muted)">' + escapeHtml(agent.description) + '</span>' : '');
            item.addEventListener("click", function () {
                selectAgent("+" + agent.name);
            });
            agentListEl.appendChild(item);
        });
    }

    function selectAgent(agent) {
        selectedAgent = agent;
        currentAgentEl.textContent = agent;
        currentConversationId = null;

        agentListEl.querySelectorAll(".sidebar-item").forEach(function (el) {
            el.classList.toggle("active", el.getAttribute("data-agent") === agent.replace(/^\+/, ""));
        });

        showAgentInfo(agent);
    }

    // -- Model Selector --
    function loadModels() {
        fetch("/api/models")
            .then(function (r) { return r.json(); })
            .then(function (models) {
                modelSelector.innerHTML = '<option value="">default model</option>';
                if (!models || models.length === 0) return;
                models.forEach(function (m) {
                    var opt = document.createElement("option");
                    opt.value = m.model;
                    var label = m.alias !== "default" ? m.alias + " (" + m.model + ")" : m.model;
                    if (m.default) label += " *";
                    opt.textContent = label;
                    modelSelector.appendChild(opt);
                });
            })
            .catch(function () {
                // silently ignore - model selector stays with default
            });
    }

    // -- Sidebar: History --
    function loadHistory() {
        fetch("/api/history")
            .then(function (r) { return r.json(); })
            .then(function (histories) {
                renderHistory(histories);
            })
            .catch(function () {
                historyListEl.innerHTML = '<div class="sidebar-item" style="color:var(--red)">Failed to load</div>';
            });
    }

    function renderHistory(histories) {
        historyListEl.innerHTML = "";
        if (!histories || histories.length === 0) {
            historyListEl.innerHTML = '<div class="sidebar-item" style="color:var(--text-muted)">No history</div>';
            return;
        }

        var max = Math.min(histories.length, 50);
        for (var i = 0; i < max; i++) {
            (function (h) {
                var item = document.createElement("div");
                item.className = "sidebar-item";
                item.setAttribute("data-query", (h.query || "").toLowerCase());
                item.setAttribute("data-agent-name", h.agent);
                item.setAttribute("data-conversation", h.conversation_id || "");

                var label = h.query || "(no query)";
                var timeStr = formatTimestamp(h.timestamp);

                item.innerHTML =
                    '<span class="agent-badge">+' + escapeHtml(h.agent) + '</span>' +
                    '<span class="history-time">' + escapeHtml(timeStr) + '</span>' +
                    '<span class="history-query">' + escapeHtml(label) + '</span>';
                item.title = h.query || "";

                item.addEventListener("click", function () {
                    openHistory(h);
                });

                historyListEl.appendChild(item);
            })(histories[i]);
        }
    }

    function formatTimestamp(ts) {
        if (!ts || ts === "unknown") return "";
        if (ts.length >= 15) {
            return ts.substring(0, 4) + "-" + ts.substring(4, 6) + "-" + ts.substring(6, 8) +
                " " + ts.substring(9, 11) + ":" + ts.substring(11, 13);
        }
        return ts;
    }

    // -- Open History --
    function openHistory(h) {
        currentConversationId = h.conversation_id || String(h.index);
        selectedAgent = "+" + h.agent;
        currentAgentEl.textContent = selectedAgent;

        fetch("/api/history/" + encodeURIComponent(currentConversationId))
            .then(function (r) {
                if (!r.ok) throw new Error("Not found");
                return r.json();
            })
            .then(function (history) {
                messagesEl.innerHTML = "";
                if (history.messages) {
                    history.messages.forEach(function (msg) {
                        if (msg.role === "system") return;
                        if (msg.role === "tool") {
                            var toolDiv = document.createElement("div");
                            toolDiv.className = "tool-call";
                            var output = msg.content || "";
                            toolDiv.innerHTML =
                                '<div class="tool-call-card approved">' +
                                '  <div class="tool-call-header">' +
                                '    <span class="tool-icon">&#9881;</span>' +
                                '    <span>' + escapeHtml(msg.name || "tool") + '</span>' +
                                '  </div>' +
                                '  <div class="tool-call-output" data-raw="' + escapeAttr(output) + '">' + escapeHtml(output) + '</div>' +
                                '</div>';
                            messagesEl.appendChild(toolDiv);
                            return;
                        }
                        if (msg.role === "user") {
                            appendUserMessage(msg.content);
                            return;
                        }
                        if (msg.role === "assistant") {
                            if (msg.content) {
                                var msgDiv = createMessageDiv("assistant");
                                var contentEl = msgDiv.querySelector(".message-content");
                                contentEl.setAttribute("data-raw", msg.content);
                                contentEl.innerHTML = renderMarkdown(msg.content);
                                messagesEl.appendChild(msgDiv);
                            }
                            return;
                        }
                    });
                }
                scrollToBottom();
                isStreaming = false;
                sendBtn.disabled = false;
                inputEl.focus();
            })
            .catch(function () {
                appendError("Failed to load conversation history.");
            });
    }

    // -- Search Filtering --
    function filterList(searchInput, listEl) {
        var query = searchInput.value.toLowerCase().trim();
        var items = listEl.querySelectorAll(".sidebar-item");
        items.forEach(function (item) {
            if (!query) {
                item.classList.remove("hidden");
                return;
            }
            var text = item.textContent.toLowerCase();
            var dataQuery = item.getAttribute("data-query") || "";
            var dataAgent = item.getAttribute("data-agent") || item.getAttribute("data-agent-name") || "";
            var match = text.indexOf(query) !== -1 || dataQuery.indexOf(query) !== -1 || dataAgent.indexOf(query) !== -1;
            item.classList.toggle("hidden", !match);
        });
    }

    // -- New Chat --
    function newChat() {
        currentConversationId = null;
        currentStreamEl = null;
        currentStreamRaw = "";
        isStreaming = false;
        sendBtn.disabled = false;
        showAgentInfo(selectedAgent);
        inputEl.focus();
    }

    // -- Export Chat as HTML --
    function exportChat() {
        var content = messagesEl.innerHTML;
        if (!content.trim() || messagesEl.querySelector(".welcome")) return;

        var theme = document.documentElement.getAttribute("data-theme") || "dark";
        fetch("/style.css")
            .then(function (r) { return r.text(); })
            .then(function (css) {
                var html = '<!DOCTYPE html>\n<html lang="en" data-theme="' + theme + '">\n<head>\n' +
                    '<meta charset="UTF-8">\n<title>esa chat export</title>\n' +
                    '<style>\n' + css + '\n' +
                    'body { background: var(--bg-primary); color: var(--text-primary); padding: 20px; }\n' +
                    '#messages { max-width: 800px; margin: 0 auto; }\n' +
                    '.copy-btn { display: none; }\n' +
                    '</style>\n</head>\n<body>\n' +
                    '<div id="messages">' + content + '</div>\n' +
                    '</body>\n</html>';

                var blob = new Blob([html], { type: "text/html" });
                var url = URL.createObjectURL(blob);
                var a = document.createElement("a");
                a.href = url;
                a.download = "esa-chat-" + new Date().toISOString().slice(0, 19).replace(/[T:]/g, "-") + ".html";
                a.click();
                URL.revokeObjectURL(url);
            });
    }

    // -- Event Listeners --
    sendBtn.addEventListener("click", function () {
        sendMessage(inputEl.value);
    });

    inputEl.addEventListener("keydown", function (e) {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            sendMessage(inputEl.value);
        }
    });

    inputEl.addEventListener("input", function () {
        this.style.height = "auto";
        this.style.height = Math.min(this.scrollHeight, 150) + "px";
    });

    approveBtn.addEventListener("click", function () {
        sendApproval(true, "");
    });

    denyBtn.addEventListener("click", function () {
        sendApproval(false, denyMessageInput.value);
    });

    document.addEventListener("keydown", function (e) {
        if (!approvalModal.classList.contains("hidden")) {
            if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                sendApproval(true, "");
            } else if (e.key === "Escape") {
                e.preventDefault();
                sendApproval(false, denyMessageInput.value);
            }
        }
    });

    newChatBtn.addEventListener("click", newChat);

    sidebarToggle.addEventListener("click", function () {
        sidebar.classList.toggle("collapsed");
    });

    themeToggle.addEventListener("click", toggleTheme);

    agentSearch.addEventListener("input", function () {
        filterList(agentSearch, agentListEl);
    });

    historySearch.addEventListener("input", function () {
        filterList(historySearch, historyListEl);
    });

    modelSelector.addEventListener("change", function () {
        selectedModel = modelSelector.value;
    });

    exportBtn.addEventListener("click", exportChat);

    // -- Init --
    initTheme();
    newChat();
    connect();
    loadAgents();
    loadHistory();
    loadModels();
})();
