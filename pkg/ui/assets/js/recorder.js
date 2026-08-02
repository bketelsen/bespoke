// Bespoke voice capture (ADR-0014): click a [data-voice-post] button to
// record, click again to stop. Captures mono PCM via Web Audio and encodes
// WAV client-side — the Lemonade transcription backend accepts WAV only.
// Loaded by ui.VoiceButton.
let active = null;

document.addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-voice-post]");
  if (!btn) return;
  e.preventDefault();

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

  btn.dataset.recording = "1";
  btn.classList.add("animate-pulse", "text-destructive");

  active = {
    stop: async () => {
      active = null;
      proc.disconnect();
      source.disconnect();
      stream.getTracks().forEach((t) => t.stop());
      const rate = ctx.sampleRate;
      await ctx.close();
      btn.dataset.recording = "0";
      btn.classList.remove("animate-pulse", "text-destructive");

      const resp = await fetch(btn.dataset.voicePost, {
        method: "POST",
        headers: { "Content-Type": "audio/wav" },
        body: encodeWav(chunks, rate),
      });
      if (resp.ok) location.reload();
      else alert("transcription failed: " + (await resp.text()));
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
