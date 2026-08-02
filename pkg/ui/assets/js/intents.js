// Cross-app intents: selection popover (ADR-0018). Select text anywhere in
// an app and a small popover offers other apps' text intents; choosing one
// navigates to that app's prefilled confirm page. Desktop-first — touch
// selection fights native menus, so coarse pointers skip the popover
// (event-idiom banners cover mobile).
const data = document.getElementById("bespoke-intents");
const intents = data ? JSON.parse(data.textContent) : [];

if (intents.length && !matchMedia("(pointer: coarse)").matches) {
  let pop = null;

  const hide = () => {
    pop?.remove();
    pop = null;
  };

  document.addEventListener("mouseup", (e) => {
    if (e.target.closest("#bespoke-intent-pop")) return;
    setTimeout(() => {
      hide();
      const sel = window.getSelection();
      const text = sel?.toString().trim() ?? "";
      if (text.length < 3 || text.length > 2000) return;
      if (e.target.closest("input, textarea, [contenteditable]")) return;

      const rect = sel.getRangeAt(0).getBoundingClientRect();
      pop = document.getElementById("bespoke-intent-pop-template")
        .content.firstElementChild.cloneNode(true);
      for (const it of intents) {
        const a = pop.querySelector("template").content.firstElementChild.cloneNode(true);
        a.textContent = it.title;
        a.href = it.url + "?text=" + encodeURIComponent(text);
        pop.appendChild(a);
      }
      document.body.appendChild(pop);
      const top = window.scrollY + rect.bottom + 6;
      const left = Math.max(8, Math.min(window.scrollX + rect.left, window.scrollX + window.innerWidth - pop.offsetWidth - 8));
      pop.style.top = top + "px";
      pop.style.left = left + "px";
    }, 0);
  });

  document.addEventListener("selectionchange", () => {
    if (window.getSelection()?.isCollapsed) hide();
  });
}
