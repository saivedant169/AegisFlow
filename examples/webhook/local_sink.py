#!/usr/bin/env python3
import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length)
        try:
            payload = json.loads(body.decode("utf-8"))
        except json.JSONDecodeError:
            payload = body.decode("utf-8", errors="replace")

        print(json.dumps({
            "path": self.path,
            "signature": self.headers.get("X-AegisFlow-Signature", ""),
            "timestamp": self.headers.get("X-AegisFlow-Timestamp", ""),
            "payload": payload,
        }, indent=2), flush=True)

        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok\n")

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    server = HTTPServer(("127.0.0.1", 9099), Handler)
    print("listening on http://127.0.0.1:9099/events", flush=True)
    server.serve_forever()
