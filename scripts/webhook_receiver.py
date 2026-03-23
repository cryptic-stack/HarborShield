import http.server
import json
import sys


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8")
        record = {
            "path": self.path,
            "headers": dict(self.headers),
            "body": body,
        }
        with open(sys.argv[2], "a", encoding="utf-8") as handle:
            handle.write(json.dumps(record) + "\n")
        self.send_response(204)
        self.end_headers()

    def log_message(self, format, *args):
        return


if __name__ == "__main__":
    server = http.server.ThreadingHTTPServer(("0.0.0.0", int(sys.argv[1])), Handler)
    server.serve_forever()
