// Bespoke in-app chat (ADR-0015). Panel + bubble markup live in chat.templ;
// this just moves messages. History is whatever bubbles are in the log.
// The speak toggle (local TTS via /_chat/speak) persists in localStorage.
const panel = () => document.getElementById("bespoke-chat");
const SPEAK_KEY = "bespoke-chat-speak";

const speakBtn = () => panel()?.querySelector("[data-chat-speak-toggle]");
const speakOn = () => localStorage.getItem(SPEAK_KEY) === "1";
document.addEventListener("DOMContentLoaded", () => {
  if (speakOn() && speakBtn()) speakBtn().dataset.on = "1";
});

document.addEventListener("click", (e) => {
  if (e.target.closest("[data-chat-speak-toggle]")) {
    const on = !speakOn();
    localStorage.setItem(SPEAK_KEY, on ? "1" : "0");
    speakBtn().dataset.on = on ? "1" : "0";
    return;
  }
  if (e.target.closest("[data-chat-toggle]")) {
    panel().classList.toggle("hidden");
    if (speakOn() && speakBtn()) speakBtn().dataset.on = "1";
    panel().querySelector("textarea")?.focus();
  }
});

async function speak(text) {
  try {
    // Belt and braces: strip any markdown that slipped past the prompt so
    // the TTS never reads symbols aloud.
    const clean = text
      .replace(/[*_`#>]+/g, "")
      .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1");
    const resp = await fetch("/_chat/speak", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: clean }),
    });
    if (!resp.ok) return; // no TTS backend — stay quiet
    const url = URL.createObjectURL(await resp.blob());
    const player = new Audio(url);
    player.onended = () => URL.revokeObjectURL(url);
    player.play().catch(() => URL.revokeObjectURL(url));
  } catch {
    /* speech is a nicety; never break chat over it */
  }
}

document.addEventListener("submit", async (e) => {
  const form = e.target.closest("#bespoke-chat-form");
  if (!form) return;
  e.preventDefault();

  const input = form.querySelector("textarea");
  const message = input.value.trim();
  if (!message) return;

  const log = document.getElementById("bespoke-chat-log");
  const bubble = (kind, text) => {
    const node = document.getElementById(`bespoke-chat-${kind}`).content.firstElementChild.cloneNode(true);
    node.textContent = text;
    log.appendChild(node);
    log.scrollTop = log.scrollHeight;
    return node;
  };

  const history = [...log.querySelectorAll("[data-role]")].map((n) => ({
    role: n.dataset.role,
    text: n.textContent,
  }));

  bubble("user", message);
  input.value = "";
  const pending = bubble("ai", "…");
  form.querySelector("button[type=submit]").disabled = true;

  try {
    const resp = await fetch("/_chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message, history, speak: speakOn() }),
    });
    if (resp.ok) {
      pending.textContent = (await resp.json()).text;
      if (speakOn()) speak(pending.textContent);
    } else {
      pending.textContent = "error: " + (await resp.text());
    }
  } catch (err) {
    pending.textContent = "error: " + err.message;
  } finally {
    form.querySelector("button[type=submit]").disabled = false;
  }
});
