import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import net from 'node:net';

import {createFakeRelayServer} from '../mock/fake-relay.mjs';

const server = createFakeRelayServer();
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const address = server.address();
assert(address && typeof address === 'object');

try {
  const health = await fetch(`http://127.0.0.1:${address.port}/api/v1/collaboration/health`, {headers: {authorization: 'Bearer mock-panel-jwt'}});
  assert.equal(health.status, 200);
  assert.equal((await health.json()).data.protocol_version, 1);

  const frame = await new Promise((resolve, reject) => {
    const socket = net.connect(address.port, '127.0.0.1');
    const key = crypto.randomBytes(16).toString('base64');
    let received = Buffer.alloc(0);
    socket.on('connect', () => socket.write([
      'GET /api/v1/collaboration/ws HTTP/1.1',
      `Host: 127.0.0.1:${address.port}`,
      'Upgrade: websocket',
      'Connection: Upgrade',
      `Sec-WebSocket-Key: ${key}`,
      'Sec-WebSocket-Version: 13',
      'Authorization: Bearer mock-panel-jwt',
      '\r\n',
    ].join('\r\n')));
    socket.on('data', (chunk) => {
      received = Buffer.concat([received, chunk]);
      const boundary = received.indexOf('\r\n\r\n');
      if (boundary < 0 || received.length < boundary + 6) return;
      const headers = received.subarray(0, boundary).toString();
      assert.match(headers, /101 Switching Protocols/);
      const data = received.subarray(boundary + 4);
      const shortLength = data[1] & 0x7f;
      const headerLength = shortLength === 126 ? 4 : 2;
      const length = shortLength === 126 ? data.readUInt16BE(2) : shortLength;
      if (data.length < headerLength + length) return;
      socket.end();
      resolve(JSON.parse(data.subarray(headerLength, headerLength + length).toString()));
    });
    socket.on('error', reject);
  });
  assert.equal(frame.type, 'heartbeat.ack');
  console.log('fake relay REST and WebSocket smoke test passed');
} finally {
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}

// Node 20's built-in fetch dispatcher keeps its connection pool alive briefly;
// this is a standalone smoke process, so exit once all assertions and cleanup
// have completed.
process.exit(0);
