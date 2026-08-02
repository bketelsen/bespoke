// Bespoke voice capture (ADR-0014): tap [data-voice-post] to record, tap the
// stop square to finish; WAV POSTs to the app, page reloads on success.
// States drive the button's UI via data-state: idle | rec | busy
// (icons/colors live in ui.VoiceButton, not here).
import { startRecording } from "./wav.js";

let active = null;

const statusEl = (btn) => btn.parentElement.querySelector("[data-voice-status]");

function setState(btn, state, statusText) {
  btn.dataset.state = state;
  btn.disabled = state === "busy";
  const s = statusEl(btn);
  if (s) {
    s.textContent = statusText || "";
    s.classList.toggle("hidden", !statusText);
  }
}

document.addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-voice-post]");
  if (!btn) return;
  e.preventDefault();

  if (btn.dataset.state === "busy") return;
  if (active) {
    const rec = active;
    active = null;
    setState(btn, "busy", "transcribing…");
    try {
      const wav = await rec.stop();
      const resp = await fetch(btn.dataset.voicePost, {
        method: "POST",
        headers: { "Content-Type": "audio/wav" },
        body: wav,
      });
      if (resp.ok) {
        setState(btn, "busy", "saved — refreshing…");
        location.reload();
      } else {
        setState(btn, "idle", "");
        alert("transcription failed: " + (await resp.text()));
      }
    } catch (err) {
      setState(btn, "idle", "");
      alert("transcription failed: " + err.message);
    }
    return;
  }

  try {
    active = await startRecording();
  } catch (err) {
    alert("microphone unavailable: " + err.message);
    return;
  }
  setState(btn, "rec", "recording — tap ■ to finish");
});
