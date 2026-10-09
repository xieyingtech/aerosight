"""Pinned CPU embedding service used by the Go media indexer."""
import hmac
import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from sentence_transformers import SentenceTransformer

MODEL = os.getenv("EMBEDDING_MODEL", "intfloat/multilingual-e5-small")
REVISION = os.getenv("EMBEDDING_REVISION", "614241f622f53c4eeff9890bdc4f31cfecc418b3")
TOKEN = os.getenv("EMBEDDING_API_KEY", "")
model = SentenceTransformer(MODEL, revision=REVISION, device="cpu", trust_remote_code=False)
lock = threading.Lock()

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def send(self, status, body):
        raw = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        if self.path != "/health":
            return self.send(404, {"error": "not_found"})
        self.send(200, {"model": MODEL, "revision": REVISION, "dimension": model.get_sentence_embedding_dimension()})

    def do_POST(self):
        if self.path != "/v1/embeddings":
            return self.send(404, {"error": "not_found"})
        if TOKEN and not hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + TOKEN):
            return self.send(401, {"error": "unauthorized"})
        try:
            size = int(self.headers.get("Content-Length", "0"))
            if not 0 < size <= 1_000_000:
                return self.send(413, {"error": "request_size"})
            request = json.loads(self.rfile.read(size))
            inputs = request["input"]
            if request.get("model") != MODEL or not isinstance(inputs, list) or not 1 <= len(inputs) <= 32:
                return self.send(400, {"error": "invalid_input"})
            if any(not isinstance(item, str) or not item.startswith(("query: ", "passage: ")) or len(item) > 8000 for item in inputs):
                return self.send(400, {"error": "invalid_input"})
            with lock:
                vectors = model.encode(inputs, normalize_embeddings=True, show_progress_bar=False).tolist()
            self.send(200, {"model": MODEL, "revision": REVISION, "data": [{"index": i, "embedding": v} for i, v in enumerate(vectors)]})
        except (ValueError, KeyError, TypeError):
            self.send(400, {"error": "invalid_input"})
        except Exception:
            self.send(503, {"error": "embedding_failed"})

if __name__ == "__main__":
    ThreadingHTTPServer((os.getenv("EMBEDDING_HOST", "127.0.0.1"), int(os.getenv("EMBEDDING_PORT", "6335"))), Handler).serve_forever()
