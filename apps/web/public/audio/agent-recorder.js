// Capture only: mono PCM16 at the StepFun realtime rate (24 kHz).
// The AudioContext may run at 44.1/48 kHz, so do not trust device constraints.
class AgentRecorder extends AudioWorkletProcessor {
  constructor() {
    super();
    this.phase = 0;
    this.sum = 0;
    this.count = 0;
    this.buffer = new Int16Array(2400);
    this.index = 0;
  }
  process(inputs) {
    const channels = inputs[0];
    if (!channels?.length) return true;
    for (let i = 0; i < channels[0].length; i++) {
      let sample = 0;
      for (const channel of channels) sample += channel[i] / channels.length;
      this.sum += sample;
      this.count++;
      this.phase += 24000;
      if (this.phase >= sampleRate) {
        this.phase -= sampleRate;
        const value = Math.max(-1, Math.min(1, this.sum / this.count));
        this.buffer[this.index++] = Math.round(value * (value < 0 ? 32768 : 32767));
        this.sum = 0;
        this.count = 0;
        if (this.index === this.buffer.length) {
          this.port.postMessage(this.buffer.buffer, [this.buffer.buffer]);
          this.buffer = new Int16Array(2400);
          this.index = 0;
        }
      }
    }
    return true;
  }
}
registerProcessor("agent-recorder", AgentRecorder);
