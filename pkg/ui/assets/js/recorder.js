// Bespoke voice capture (ADR-0014): tap [data-voice-post] to record, tap the
// stop square to finish. Captures mono PCM via Web Audio, encodes WAV
// client-side (Lemonade accepts WAV only), POSTs, reloads on success.
// States drive the button's UI via data-state: idle | rec | busy
// (icons/colors live in ui.VoiceButton, not here).
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
    active.stop();
    return;
  }

  let stream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  } catch (err) {
    alert("microphone unavailable: " + err.message);
    return;
  }

  const ctx = new AudioContext();
  const source = ctx.createMediaStreamSource(stream);
  const proc = ctx.createScriptProcessor(4096, 1, 1);
  const chunks = [];
  proc.onaudioprocess = (ev) => chunks.push(new Float32Array(ev.inputBuffer.getChannelData(0)));
  source.connect(proc);
  proc.connect(ctx.destination);

  setState(btn, "rec", "recording — tap ■ to finish");

  active = {
    stop: async () => {
      active = null;
      proc.disconnect();
      source.disconnect();
      stream.getTracks().forEach((t) => t.stop());
      const rate = ctx.sampleRate;
      await ctx.close();

      setState(btn, "busy", "transcribing…");
      try {
        const resp = await fetch(btn.dataset.voicePost, {
          method: "POST",
          headers: { "Content-Type": "audio/wav" },
          body: encodeWav(chunks, rate),
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
    },
  };
});

function encodeWav(chunks, sampleRate) {
  let len = 0;
  for (const c of chunks) len += c.length;
  const pcm = new Int16Array(len);
  let off = 0;
  for (const c of chunks) {
    for (let i = 0; i < c.length; i++) {
      const s = Math.max(-1, Math.min(1, c[i]));
      pcm[off++] = s < 0 ? s * 0x8000 : s * 0x7fff;
    }
  }
  const buf = new ArrayBuffer(44 + pcm.length * 2);
  const v = new DataView(buf);
  const put = (o, s) => {
    for (let i = 0; i < s.length; i++) v.setUint8(o + i, s.charCodeAt(i));
  };
  put(0, "RIFF");
  v.setUint32(4, 36 + pcm.length * 2, true);
  put(8, "WAVE");
  put(12, "fmt ");
  v.setUint32(16, 16, true);
  v.setUint16(20, 1, true); // PCM
  v.setUint16(22, 1, true); // mono
  v.setUint32(24, sampleRate, true);
  v.setUint32(28, sampleRate * 2, true);
  v.setUint16(32, 2, true);
  v.setUint16(34, 16, true);
  put(36, "data");
  v.setUint32(40, pcm.length * 2, true);
  new Int16Array(buf, 44).set(pcm);
  return new Blob([buf], { type: "audio/wav" });
}
