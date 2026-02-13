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

    // -- State --
    var ws = null;
    var selectedAgent = "+default";
    var isStreaming = false;
    var currentStreamEl = null;
    var pendingApprovalId = null;
    var currentConversationId = null;
    var cachedAgents = [];

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
                finishStream();
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
            messagesEl.appendChild(msgDiv);
        }
        currentStreamEl.textContent += content;
        scrollToBottom();
    }

    function finishStream() {
        if (currentStreamEl) {
            currentStreamEl.classList.remove("streaming-cursor");
            currentStreamEl.innerHTML = renderContent(currentStreamEl.textContent);
            currentStreamEl = null;
        }
        isStreaming = false;
        sendBtn.disabled = false;
        inputEl.focus();
        scrollToBottom();
        // Auto-refresh history after a chat completes
        loadHistory();
    }

    function handleToolCall(msg) {
        // Finish any ongoing stream text first
        if (currentStreamEl) {
            currentStreamEl.classList.remove("streaming-cursor");
            currentStreamEl.innerHTML = renderContent(currentStreamEl.textContent);
            currentStreamEl = null;
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
            '  </div>' +
            '  <div class="tool-call-command">$ ' + escapeHtml(msg.command) + '</div>' +
            '</div>';

        messagesEl.appendChild(toolDiv);
        scrollToBottom();

        // Only show approval modal if not safe
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
                outputDiv.textContent = msg.output;
                card.appendChild(outputDiv);
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

    // -- DOM Helpers --
    function createMessageDiv(role) {
        var div = document.createElement("div");
        div.className = "message";

        var roleDiv = document.createElement("div");
        roleDiv.className = "message-role " + role;
        roleDiv.textContent = role === "user" ? "you" : "esa";

        var contentDiv = document.createElement("div");
        contentDiv.className = "message-content";

        div.appendChild(roleDiv);
        div.appendChild(contentDiv);
        return div;
    }

    function appendUserMessage(text) {
        var msgDiv = createMessageDiv("user");
        msgDiv.querySelector(".message-content").textContent = text;
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

    // Simple markdown renderer
    function renderContent(text) {
        if (!text) return "";

        var html = escapeHtml(text);

        // Code blocks
        html = html.replace(/```(\w*)\n([\s\S]*?)```/g, function (_, lang, code) {
            return '<pre><code>' + code + '</code></pre>';
        });

        // Inline code
        html = html.replace(/`([^`]+)`/g, '<code>$1</code>');

        // Bold
        html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');

        // Italic
        html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');

        // Paragraphs
        html = html
            .split(/\n\n+/)
            .map(function (para) {
                return '<p>' + para.replace(/\n/g, '<br>') + '</p>';
            })
            .join('');

        return html;
    }

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
                    // Truncate long descriptions (strip the command part)
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
                // Show initial agent info
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

        // Update active state
        agentListEl.querySelectorAll(".sidebar-item").forEach(function (el) {
            el.classList.toggle("active", el.getAttribute("data-agent") === agent.replace(/^\+/, ""));
        });

        // Show agent info in main area (only when no chat in progress)
        showAgentInfo(agent);
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

        // Show up to 50 entries
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
        // ts is like "20260213-143022"
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

        // Fetch the full conversation
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
                            // Show as tool result
                            var toolDiv = document.createElement("div");
                            toolDiv.className = "tool-call";
                            var output = msg.content || "";
                            toolDiv.innerHTML =
                                '<div class="tool-call-card approved">' +
                                '  <div class="tool-call-header">' +
                                '    <span class="tool-icon">&#9881;</span>' +
                                '    <span>' + escapeHtml(msg.name || "tool") + '</span>' +
                                '  </div>' +
                                '  <div class="tool-call-output">' + escapeHtml(output) + '</div>' +
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
                                msgDiv.querySelector(".message-content").innerHTML = renderContent(msg.content);
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
        isStreaming = false;
        sendBtn.disabled = false;
        showAgentInfo(selectedAgent);
        inputEl.focus();
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

    // -- Init --
    initTheme();
    newChat();
    connect();
    loadAgents();
    loadHistory();
})();
