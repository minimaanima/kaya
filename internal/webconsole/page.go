package webconsole

const loginDocument = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Kaya Remote Console</title>
  <style>
    :root { color-scheme: dark; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; background: #0b0f14; color: #d9e2ec; }
    * { box-sizing: border-box; }
    body { min-block-size: 100vh; min-block-size: 100dvh; margin: 0; display: grid; place-items: center; padding: 1rem; }
    .login { inline-size: min(100%, 30rem); padding: clamp(1.5rem, 6vw, 3rem); border: 1px solid #334155; border-radius: .75rem; background: #111827; box-shadow: 0 1.25rem 3rem rgb(0 0 0 / .35); }
    h1 { margin: 0; font-size: clamp(1.45rem, 6vw, 2rem); color: #f8fafc; }
    p { line-height: 1.55; color: #aebdca; }
    label { display: grid; gap: .5rem; margin-block: 1.5rem 1rem; color: #d9e2ec; }
    input, button { font: inherit; }
    input { inline-size: 100%; min-inline-size: 0; padding: .8rem .9rem; border: 1px solid #475569; border-radius: .4rem; background: #0b1220; color: #f8fafc; }
    button { padding: .8rem 1rem; border: 1px solid #38bdf8; border-radius: .4rem; background: #0284c7; color: #f8fafc; cursor: pointer; }
    button:disabled { cursor: wait; opacity: .65; }
    #login-status { min-block-size: 1.5rem; margin-block: 1rem 0; color: #fca5a5; }
  </style>
</head>
<body>
  <main class="login">
    <h1>Kaya Remote Console</h1>
    <p>Enter the password for this home game server.</p>
    <form id="login-form" action="/login" method="post">
      <label for="password">Password
        <input id="password" name="password" type="password" autocomplete="current-password" required autofocus>
      </label>
      <button id="login" type="submit">Connect</button>
    </form>
    <p id="login-status" role="status" aria-live="polite"></p>
  </main>
  <script>
    const loginForm = document.getElementById("login-form");
    const loginButton = document.getElementById("login");
    const loginStatus = document.getElementById("login-status");

    loginForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      loginButton.disabled = true;
      loginStatus.textContent = "";
      try {
        const response = await fetch("/login", { method: "POST", body: new FormData(loginForm) });
        if (response.ok) {
          window.location.assign("/");
          return;
        }
        loginStatus.textContent = "Incorrect password. Please try again.";
      } catch (_) {
        loginStatus.textContent = "Unable to sign in. Please try again.";
      } finally {
        loginButton.disabled = false;
      }
    });
  </script>
</body>
</html>`

const terminalDocument = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Kaya Remote Console</title>
  <style>
    :root { color-scheme: dark; font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; background: #080c12; color: #d9e2ec; }
    * { box-sizing: border-box; }
    html, body { min-block-size: 100%; margin: 0; overflow-x: hidden; }
    body { min-block-size: 100vh; min-block-size: 100dvh; background: #080c12; }
    button, input { font: inherit; }
    button { min-block-size: 2.6rem; border: 1px solid #475569; border-radius: .4rem; padding: .55rem .8rem; background: #1e293b; color: #f8fafc; cursor: pointer; }
    button:hover:not(:disabled) { border-color: #7dd3fc; background: #334155; }
    button:disabled, input:disabled { cursor: wait; opacity: .65; }
    .console { block-size: 100vh; block-size: 100dvh; display: grid; grid-template-rows: auto minmax(0, 1fr) auto auto; max-inline-size: 64rem; margin-inline: auto; border-inline: 1px solid #1e293b; background: #0f172a; }
    .console-header { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: 1rem; border-block-end: 1px solid #334155; background: #111c30; }
    h1 { margin: 0; font-size: clamp(1.1rem, 5vw, 1.5rem); color: #f8fafc; }
    .subhead { margin: .3rem 0 0; font-size: .82rem; color: #94a3b8; }
    .controls { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: .5rem; }
    #new-run { border-color: #0e7490; background: #164e63; }
    #logout { border-color: #7f1d1d; background: #450a0a; }
    #transcript { min-inline-size: 0; overflow-y: auto; overscroll-behavior: contain; padding: 1rem; display: grid; align-content: start; gap: .75rem; }
    .entry { display: grid; grid-template-columns: minmax(5rem, 7.5rem) minmax(0, 1fr); gap: .75rem; padding: .7rem .8rem; border-inline-start: .25rem solid #475569; border-radius: .25rem; background: #111c30; overflow-wrap: anywhere; }
    .entry-label { font-weight: 700; color: #94a3b8; }
    .entry-text { min-inline-size: 0; white-space: pre-wrap; }
    .entry--player { border-color: #38bdf8; background: #0c4a6e; }
    .entry--player .entry-label { color: #bae6fd; }
    .entry--kaya { border-color: #a78bfa; }
    .entry--kaya .entry-label { color: #c4b5fd; }
    .entry--time { border-color: #fbbf24; background: #422006; }
    .entry--time .entry-label { color: #fde68a; }
    .entry--error { border-color: #fb7185; background: #4c0519; }
    .entry--error .entry-label { color: #fecdd3; }
    .entry--completed { border-color: #4ade80; background: #052e16; }
    .entry--completed .entry-label { color: #bbf7d0; }
    #command-form { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: .6rem; padding: .85rem 1rem; border-block-start: 1px solid #334155; background: #111c30; }
    #message { min-inline-size: 0; inline-size: 100%; min-block-size: 2.6rem; border: 1px solid #475569; border-radius: .4rem; padding: .55rem .75rem; background: #080c12; color: #f8fafc; }
    #message:focus, button:focus-visible { outline: 2px solid #7dd3fc; outline-offset: 2px; }
    #send { border-color: #0284c7; background: #0369a1; }
    #status { min-block-size: 1.4rem; margin: 0; padding: 0 1rem .65rem; color: #fca5a5; background: #111c30; font-size: .86rem; }
    @media (max-width: 34rem) {
      .console { border: 0; }
      .console-header { align-items: flex-start; flex-direction: column; }
      .controls { inline-size: 100%; justify-content: stretch; }
      .controls button { flex: 1 1 8rem; }
      .entry { grid-template-columns: 1fr; gap: .25rem; }
      #command-form { grid-template-columns: 1fr; }
      #send { inline-size: 100%; }
    }
  </style>
</head>
<body>
  <main class="console">
    <header class="console-header">
      <div>
        <h1>Kaya Remote Console</h1>
        <p class="subhead">Secure remote playtest session</p>
      </div>
      <div class="controls">
        <button id="new-run" type="button">New Run</button>
        <button id="logout" type="button">Logout</button>
      </div>
    </header>
    <section id="transcript" aria-live="polite" aria-label="Game transcript" tabindex="-1"></section>
    <form id="command-form">
      <input id="message" name="message" type="text" maxlength="2000" autocomplete="off" placeholder="Enter a command" aria-label="Game command" required>
      <button id="send" type="submit">Send</button>
    </form>
    <p id="status" role="status" aria-live="polite"></p>
  </main>
  <script>
    const transcript = document.getElementById("transcript");
    const commandForm = document.getElementById("command-form");
    const message = document.getElementById("message");
    const send = document.getElementById("send");
    const newRun = document.getElementById("new-run");
    const logout = document.getElementById("logout");
    const status = document.getElementById("status");
    let state = { entries: [], complete: false };
    let pending = false;

    function entryStyle(entry) {
      if (entry.role === "player") return { kind: "player", label: "You" };
      if (entry.role === "kaya") return { kind: "kaya", label: "Kaya" };
      if (entry.role === "error") return { kind: "error", label: "Error" };
      if (entry.text.indexOf("[time +") === 0) return { kind: "time", label: "Time" };
      if (entry.text === "Prototype objective complete.") return { kind: "completed", label: "Complete" };
      return { kind: "system", label: "System" };
    }

    function appendEntry(entry) {
      const style = entryStyle(entry);
      const line = document.createElement("article");
      const label = document.createElement("span");
      const text = document.createElement("span");
      line.className = "entry entry--" + style.kind;
      label.className = "entry-label";
      text.className = "entry-text";
      label.textContent = style.label;
      text.textContent = entry.text;
      line.append(label, text);
      transcript.append(line);
    }

    function updateAvailability() {
      const unavailable = pending || state.complete;
      message.disabled = unavailable;
      send.disabled = unavailable;
      newRun.disabled = pending;
      logout.disabled = pending;
    }

    function focusAndScroll() {
      transcript.scrollTop = transcript.scrollHeight;
      if (!message.disabled) message.focus();
    }

    function render(nextState) {
      state = nextState;
      transcript.replaceChildren();
      for (const entry of state.entries) appendEntry(entry);
      if (state.complete && !state.entries.some(function (entry) { return entry.text === "Prototype objective complete."; })) {
        appendEntry({ role: "system", text: "Prototype objective complete." });
      }
      status.textContent = state.complete ? "Run complete. Start a New Run to continue." : "";
      updateAvailability();
      focusAndScroll();
    }

    function showSafeError(text) {
      status.textContent = text;
      appendEntry({ role: "error", text: text });
      focusAndScroll();
    }

    function setPending(nextPending) {
      pending = nextPending;
      updateAvailability();
    }

    function returnToLogin() {
      window.location.reload();
    }

    async function readState(path, options) {
      const response = await fetch(path, options);
      if (response.status === 401) {
        returnToLogin();
        return null;
      }
      if (!response.ok) throw new Error("request failed");
      return response.json();
    }

    async function refresh() {
      setPending(true);
      try {
        const nextState = await readState("/api/session");
        if (nextState) render(nextState);
      } catch (_) {
        showSafeError("Unable to load the session. Please try again.");
      } finally {
        setPending(false);
      }
    }

    commandForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      const command = message.value.trim();
      if (!command || pending || state.complete) return;
      setPending(true);
      try {
        const nextState = await readState("/api/turn", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ message: command })
        });
        if (nextState) {
          message.value = "";
          render(nextState);
        }
      } catch (_) {
        showSafeError("Unable to send that command. Please try again.");
      } finally {
        setPending(false);
      }
    });

    newRun.addEventListener("click", async function () {
      if (pending || !window.confirm("Start a new run? Your current transcript will be replaced.")) return;
      setPending(true);
      try {
        const nextState = await readState("/api/new-run", { method: "POST" });
        if (nextState) render(nextState);
      } catch (_) {
        showSafeError("Unable to start a new run. Please try again.");
      } finally {
        setPending(false);
      }
    });

    logout.addEventListener("click", async function () {
      if (pending) return;
      setPending(true);
      try {
        const response = await fetch("/logout", { method: "POST" });
        if (response.status === 401) {
          returnToLogin();
          return;
        }
        if (!response.ok) throw new Error("request failed");
        returnToLogin();
      } catch (_) {
        showSafeError("Unable to log out. Please try again.");
      } finally {
        setPending(false);
      }
    });

    refresh();
  </script>
</body>
</html>`
