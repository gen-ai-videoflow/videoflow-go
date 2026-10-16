const http = require('node:http');
if (process.env.FIXTURE_FAIL) process.exit(2);
const server = http.createServer((req, res) => {
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify({status: 'ok', server: '6eek-go-v1', media_ready: true}));
});
server.listen(Number(process.env.PORT), '127.0.0.1', () => {
  const line = `6eek ready at http://127.0.0.1:${server.address().port}\n`;
  process.stderr.write(line.slice(0, 18));
  setTimeout(() => process.stderr.write(line.slice(18)), 10);
});
process.on('SIGTERM', () => server.close(() => process.exit(0)));
