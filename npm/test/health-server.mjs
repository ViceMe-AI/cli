import http from "node:http";

const server = http.createServer((request, response) => {
  if (request.method === "GET" && request.url === "/v1/health/ready") {
    response.writeHead(204);
    response.end();
    return;
  }
  response.writeHead(404);
  response.end();
});

server.listen(0, "127.0.0.1", () => {
  const address = server.address();
  process.stdout.write(`http://127.0.0.1:${address.port}\n`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
