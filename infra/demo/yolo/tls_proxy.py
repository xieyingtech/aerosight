"""Loopback TLS bridge for the algorithm endpoint and signed asset gateway."""
import http.client
import ssl
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
root = Path(__file__).resolve().parents[3]
class Proxy(BaseHTTPRequestHandler):
    def log_message(self, *args): pass  # Signed query strings must not enter logs.
    def handle_request(self):
        if self.path == '/infer' and self.command == 'POST': port = 8091
        elif self.path.startswith('/algorithm-assets/') and self.command == 'GET': port = 8092
        else: self.send_error(404); return
        length = int(self.headers.get('Content-Length', '0'))
        if length > 1024*1024: self.send_error(413); return
        conn = http.client.HTTPConnection('127.0.0.1',port, timeout=90)
        try:
            conn.request(self.command,self.path,body=self.rfile.read(length),headers={'Content-Type':self.headers.get('Content-Type','application/json')})
            response = conn.getresponse(); data=response.read()
            self.send_response(response.status)
            self.send_header('Content-Type',response.getheader('Content-Type','application/octet-stream'))
            self.send_header('Content-Length',str(len(data))); self.end_headers(); self.wfile.write(data)
        except (OSError, http.client.HTTPException): self.send_error(502)
        finally: conn.close()
    do_POST = handle_request
    do_GET = handle_request
server=ThreadingHTTPServer(('127.0.0.1',8444),Proxy)
ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(root/'.build/msup-demo/tls/cert.pem',root/'.build/msup-demo/tls/key.pem')
server.socket=ctx.wrap_socket(server.socket,server_side=True)
server.serve_forever()
