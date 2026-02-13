// esa web client
(function () {
    "use strict";

    // -- DOM Elements --
    const messagesEl = document.getElementById("messages");
    const inputEl = document.getElementById("user-input");
    const sendBtn = document.getElementById("send-btn");
    const agentListEl = document.getElementById("agent-list");
    const historyListEl = document.getElementById("history-list");
    const currentAgentEl = document.getElementById("current-agent");
    const statusEl = document.getElementById("connection-status");
    const approvalModal = document.getElementById("approval-modal");
    const approvalCommand = document.getElementById("approval-command");
    const approveBtn = document.getElementById("approve-btn");
    const denyBtn = document.getElementById("deny-btn");
    const denyMessageInput = document.getElementById("deny-message");
    const newChatBtn = document.getElementById("new-chat-btn");
    const sidebarToggle = document.getElementById("sidebar-toggle");
    const sidebar = document.getElementById("sidebar");

    // -- State --
    let ws = null;
    let selectedAgent = "+default";
    let isStreaming = false;
    let currentStreamEl = null;
    let pendingApprovalId = null;

    // -- WebSocket --
    function connect() {
        const proto = location.protocol === "https:" ? "wss:" : "ws:";
        ws = new WebSocket(proto + "//" + location.host + "/ws");

        ws.onopen = function () {
            setStatus("connected");
        };

        ws.onclose = function () {
            setStatus("disconnected");
            // Reconnect after 2s
            setTimeout(connect, 2000);
        };

        ws.onerror = function () {
            setStatus("disconnected");
        };

        ws.onmessage = function (evt) {
            const msg = JSON.parse(evt.data);
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

        appendUserMessage(text);
        ws.send(JSON.stringify({
            type: "message",
            content: text,
            agent: selectedAgent,
        }));

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
            // Start a new assistant message
            const msgDiv = createMessageDiv("assistant");
            const contentDiv = msgDiv.querySelector(".message-content");
            contentDiv.classList.add("streaming-cursor");
            currentStreamEl = contentDiv;
            messagesEl.appendChild(msgDiv);
        }
        // Append text content
        currentStreamEl.textContent += content;
        scrollToBottom();
    }

    function finishStream() {
        if (currentStreamEl) {
            currentStreamEl.classList.remove("streaming-cursor");
            // Render markdown-ish content
            currentStreamEl.innerHTML = renderContent(currentStreamEl.textContent);
            currentStreamEl = null;
        }
        isStreaming = false;
        sendBtn.disabled = false;
        inputEl.focus();
        scrollToBottom();
    }

    function handleToolCall(msg) {
        // Finish any ongoing stream first
        if (currentStreamEl) {
            currentStreamEl.classList.remove("streaming-cursor");
            currentStreamEl.innerHTML = renderContent(currentStreamEl.textContent);
            currentStreamEl = null;
        }

        // Create a tool call card
        const toolDiv = document.createElement("div");
        toolDiv.className = "tool-call";
        toolDiv.id = "tool-" + msg.id;

        const safeLabel = msg.safe ? "auto-approved" : "needs approval";
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

        // If needs approval, show modal
        if (!msg.safe) {
            pendingApprovalId = msg.id;
            showApprovalModal(msg.command);
        }
    }

    function handleToolResult(msg) {
        const toolDiv = document.getElementById("tool-" + msg.id);
        if (toolDiv) {
            const card = toolDiv.querySelector(".tool-call-card");
            if (card) {
                card.classList.remove("pending");
                if (msg.output.startsWith("Error:") || msg.output.includes("cancelled by user")) {
                    card.classList.add("denied");
                } else {
                    card.classList.add("approved");
                }

                // Add output
                const outputDiv = document.createElement("div");
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
        const div = document.createElement("div");
        div.className = "message";

        const roleDiv = document.createElement("div");
        roleDiv.className = "message-role " + role;
        roleDiv.textContent = role === "user" ? "you" : "esa";

        const contentDiv = document.createElement("div");
        contentDiv.className = "message-content";

        div.appendChild(roleDiv);
        div.appendChild(contentDiv);
        return div;
    }

    function appendUserMessage(text) {
        const msgDiv = createMessageDiv("user");
        msgDiv.querySelector(".message-content").textContent = text;
        messagesEl.appendChild(msgDiv);
        scrollToBottom();
    }

    function appendError(text) {
        const msgDiv = createMessageDiv("assistant");
        const content = msgDiv.querySelector(".message-content");
        content.style.color = "var(--red)";
        content.textContent = "Error: " + text;
        messagesEl.appendChild(msgDiv);
        scrollToBottom();
    }

    function scrollToBottom() {
        messagesEl.scrollTop = messagesEl.scrollHeight;
    }

    function escapeHtml(str) {
        const div = document.createElement("div");
        div.textContent = str;
        return div.innerHTML;
    }

    // Simple markdown-ish renderer
    function renderContent(text) {
        if (!text) return "";

        // Escape HTML first
        let html = escapeHtml(text);

        // Code blocks (``` ... ```)
        html = html.replace(/```(\w*)\n([\s\S]*?)```/g, function (_, lang, code) {
            return '<pre><code>' + code + '</code></pre>';
        });

        // Inline code
        html = html.replace(/`([^`]+)`/g, '<code>$1</code>');

        // Bold
        html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');

        // Italic
        html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');

        // Line breaks to paragraphs (double newline = paragraph, single = <br>)
        html = html
            .split(/\n\n+/)
            .map(function (para) {
                return '<p>' + para.replace(/\n/g, '<br>') + '</p>';
            })
            .join('');

        return html;
    }

    // -- Sidebar: Agents --
    function loadAgents() {
        fetch("/api/agents")
            .then(function (r) { return r.json(); })
            .then(function (agents) {
                agentListEl.innerHTML = "";
                if (!agents || agents.length === 0) {
                    agentListEl.innerHTML = '<div class="sidebar-item" style="color:var(--text-muted)">No agents found</div>';
                    return;
                }

                agents.forEach(function (agent) {
                    const item = document.createElement("div");
                    item.className = "sidebar-item" + (("+" + agent.name) === selectedAgent ? " active" : "");
                    item.innerHTML =
                        '<span class="agent-badge">+' + escapeHtml(agent.name) + '</span>' +
                        (agent.is_builtin ? '<span class="builtin-tag">builtin</span>' : '') +
                        (agent.description ? '<br><span style="font-size:11px;color:var(--text-muted)">' + escapeHtml(agent.description) + '</span>' : '');
                    item.addEventListener("click", function () {
                        selectAgent("+" + agent.name);
                    });
                    agentListEl.appendChild(item);
                });
            })
            .catch(function () {
                agentListEl.innerHTML = '<div class="sidebar-item" style="color:var(--red)">Failed to load agents</div>';
            });
    }

    function selectAgent(agent) {
        selectedAgent = agent;
        currentAgentEl.textContent = agent;
        // Update active state
        agentListEl.querySelectorAll(".sidebar-item").forEach(function (el) {
            el.classList.remove("active");
            const badge = el.querySelector(".agent-badge");
            if (badge && badge.textContent === agent) {
                el.classList.add("active");
            }
        });
    }

    // -- Sidebar: History --
    function loadHistory() {
        fetch("/api/history")
            .then(function (r) { return r.json(); })
            .then(function (histories) {
                historyListEl.innerHTML = "";
                if (!histories || histories.length === 0) {
                    historyListEl.innerHTML = '<div class="sidebar-item" style="color:var(--text-muted)">No history</div>';
                    return;
                }

                // Show latest 15
                histories.slice(0, 15).forEach(function (h) {
                    const item = document.createElement("div");
                    item.className = "sidebar-item";
                    const label = h.query || "(no query)";
                    item.innerHTML =
                        '<span class="agent-badge">+' + escapeHtml(h.agent) + '</span>' +
                        escapeHtml(label.length > 40 ? label.substring(0, 37) + '...' : label);
                    item.title = h.timestamp + "\n" + h.query;
                    historyListEl.appendChild(item);
                });
            })
            .catch(function () {
                historyListEl.innerHTML = '<div class="sidebar-item" style="color:var(--red)">Failed to load</div>';
            });
    }

    // -- New Chat --
    function newChat() {
        messagesEl.innerHTML =
            '<div class="welcome"><h2>esa</h2><p>Select an agent and start chatting.</p></div>';
        currentStreamEl = null;
        isStreaming = false;
        sendBtn.disabled = false;
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

    // Auto-resize textarea
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

    // Keyboard shortcut for approval
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

    // -- Init --
    newChat();
    connect();
    loadAgents();
    loadHistory();
})();
