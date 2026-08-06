async function mutate(path) {
  const resp = await fetch(path, { method: "POST" });
  if (!resp.ok) return;
  if (path.endsWith("read-all")) {
    document.querySelectorAll("[data-notification-read]").forEach((n) => n.remove());
    document.getElementById("bespoke-notification-count")?.classList.add("hidden");
    return;
  }
  const id = path.split("/").at(-2);
  if (path.endsWith("dismiss")) document.querySelector(`[data-notification-id="${CSS.escape(id)}"]`)?.remove();
  else document.querySelector(`[data-notification-read="${CSS.escape(id)}"]`)?.remove();
}
document.addEventListener("click", (e) => {
  const dismiss = e.target.closest("[data-notification-dismiss]");
  if (dismiss) return void mutate(`/_notifications/${dismiss.dataset.notificationDismiss}/dismiss`);
  const read = e.target.closest("[data-notification-read]");
  if (read) return void mutate(`/_notifications/${read.dataset.notificationRead}/read`);
  if (e.target.closest("[data-notifications-read-all]")) void mutate("/_notifications/read-all");
});
