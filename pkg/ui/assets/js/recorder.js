// Bespoke voice capture (ADR-0014): click a [data-voice-post] button to
// record, click again to stop; the blob POSTs to the button's URL and the
// page reloads on success. Loaded by ui.VoiceButton.
document.addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-voice-post]");
  if (!btn) return;
  e.preventDefault();

  if (btn.dataset.recording === "1") {
    window.__bespokeRecorder?.stop();
    return;
  }

  let stream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  } catch (err) {
    alert("microphone unavailable: " + err.message);
    return;
  }

  const rec = new MediaRecorder(stream);
  const chunks = [];
  rec.ondataavailable = (ev) => chunks.push(ev.data);
  rec.onstop = async () => {
    btn.dataset.recording = "0";
    btn.classList.remove("animate-pulse", "text-destructive");
    stream.getTracks().forEach((t) => t.stop());
    const blob = new Blob(chunks, { type: rec.mimeType || "audio/webm" });
    const resp = await fetch(btn.dataset.voicePost, {
      method: "POST",
      headers: { "Content-Type": blob.type },
      body: blob,
    });
    if (resp.ok) location.reload();
    else alert("transcription failed: " + (await resp.text()));
  };

  window.__bespokeRecorder = rec;
  btn.dataset.recording = "1";
  btn.classList.add("animate-pulse", "text-destructive");
  rec.start();
});
