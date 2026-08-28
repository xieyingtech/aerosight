import { createServer } from "node:http";

const port = Number(process.env.ALGORITHM_DEMO_PORT ?? 8090);

const server = createServer((request, response) => {
  if (request.method === "GET" && request.url === "/healthz") {
    response.writeHead(200, { "content-type": "application/json" });
    response.end(JSON.stringify({ ok: true }));
    return;
  }
  if (request.method !== "POST" || request.url !== "/infer") {
    response.writeHead(404).end();
    return;
  }
  let raw = "";
  request.setEncoding("utf8");
  request.on("data", (chunk) => { raw += chunk; });
  request.on("end", () => {
    try {
      const input = JSON.parse(raw);
      const language = typeof input.parameters?.language === "string" ? input.parameters.language : "zh-CN";
      response.writeHead(200, { "content-type": "application/json" });
      response.end(JSON.stringify({
        result: {
          text: `AeroSight 通用 OCR 演示 (${language})`,
          blocks: [{
            text: "Dock 2 / Dock 3 inspection",
            confidence: 0.98,
            geometry: { type: "bbox", x: 32, y: 24, width: 320, height: 48 }
          }]
        },
        modelRevision: "demo-ocr-2026.08",
        modelDigest: "sha256:4d56999fb67dd1b2dc4f2135283b0487b650350be65b37a2e81ad0bc6ce9b451",
        inputRunId: input.runId
      }));
    } catch {
      response.writeHead(400, { "content-type": "application/json" });
      response.end(JSON.stringify({ error: "invalid input contract" }));
    }
  });
});

server.listen(port, "127.0.0.1", () => {
  console.log(`Generic algorithm demo listening on http://127.0.0.1:${port}`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
