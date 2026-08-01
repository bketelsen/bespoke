// Bespoke in-app chat (ADR-0015). Panel + bubble markup live in chat.templ;
// this just moves messages. History is whatever bubbles are in the log.
const panel = () => document.getElementById("bespoke-chat");

document.addEventListener("click", (e) => {
  if (e.target.closest("[data-chat-toggle]")) {
    panel().classList.toggle("hidden");
    panel().querySelector("textarea")?.focus();
  }
});

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
      body: JSON.stringify({ message, history }),
    });
    pending.textContent = resp.ok
      ? (await resp.json()).text
      : "error: " + (await resp.text());
  } catch (err) {
    pending.textContent = "error: " + err.message;
  } finally {
    form.querySelector("button[type=submit]").disabled = false;
  }
});
