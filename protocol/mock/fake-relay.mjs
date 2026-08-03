import crypto from 'node:crypto';
import http from 'node:http';
import {fileURLToPath} from 'node:url';

function json(response, status, value) {
  response.writeHead(status, {'content-type': 'application/json'});
  response.end(JSON.stringify(value));
}

function authorized(request) {
  return request.headers.authorization === 'Bearer mock-panel-jwt';
}

function websocketTextFrame(value) {
  const payload = Buffer.from(JSON.stringify(value));
  if (payload.length < 126) return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  if (payload.length > 65535) throw new Error('mock frame exceeds 16-bit length');
  const header = Buffer.alloc(4);
  header[0] = 0x81;
  header[1] = 126;
  header.writeUInt16BE(payload.length, 2);
  return Buffer.concat([header, payload]);
}

export function createFakeRelayServer() {
  const server = http.createServer((request, response) => {
    if (!authorized(request)) {
      json(response, 401, {code: 401, message: 'unauthorized'});
      return;
    }
    if (request.method === 'GET' && request.url === '/api/v1/collaboration/health') {
      json(response, 200, {code: 0, message: 'success', data: {enabled: true, protocol_version: 1, heartbeat_interval_seconds: 20, websocket_path: '/api/v1/collaboration/ws'}});
      return;
    }
    if (request.method === 'POST' && request.url === '/api/v1/collaboration/commands') {
      // Intentionally never log or echo the request body.
      request.resume();
      json(response, 202, {code: 0, message: 'accepted', data: {command_id: '018f7f3e-86f6-7cc8-98ec-4f56dc1f2322', status: 'accepted', created_at: '2026-08-03T00:00:00Z'}});
      return;
    }
    json(response, 404, {code: 404, message: 'not found'});
  });

  server.on('upgrade', (request, socket) => {
    if (!authorized(request) || request.url !== '/api/v1/collaboration/ws') {
      socket.end('HTTP/1.1 401 Unauthorized\r\nConnection: close\r\n\r\n');
      return;
    }
    const key = request.headers['sec-websocket-key'];
    if (typeof key !== 'string') {
      socket.end('HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n');
      return;
    }
    const accept = crypto.createHash('sha1').update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`).digest('base64');
    socket.write([
      'HTTP/1.1 101 Switching Protocols',
      'Upgrade: websocket',
      'Connection: Upgrade',
      `Sec-WebSocket-Accept: ${accept}`,
      '\r\n',
    ].join('\r\n'));
    socket.write(websocketTextFrame({
      v: 1,
      type: 'heartbeat.ack',
      event_id: '01J00000000000000000000001',
      request_id: null,
      sequence: 1,
      occurred_at: '2026-08-03T00:00:00Z',
      payload: {server_time: '2026-08-03T00:00:00Z'},
    }));
  });

  return server;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.MOCK_RELAY_PORT ?? 18080);
  createFakeRelayServer().listen(port, '127.0.0.1', () => {
    console.log(`fake collaboration relay listening on http://127.0.0.1:${port}`);
  });
}
