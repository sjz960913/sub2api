import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

function readJson(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(root, relativePath), 'utf8'));
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function resolveRef(document, ref) {
  assert(ref.startsWith('#/'), `unsupported external ref: ${ref}`);
  return ref.slice(2).split('/').reduce((value, key) => value[key], document);
}

function matchesType(value, expected) {
  if (expected === 'null') return value === null;
  if (expected === 'array') return Array.isArray(value);
  if (expected === 'integer') return Number.isInteger(value);
  if (expected === 'object') return value !== null && typeof value === 'object' && !Array.isArray(value);
  return typeof value === expected;
}

function validate(document, schema, value, at = '$') {
  if (schema.$ref) return validate(document, resolveRef(document, schema.$ref), value, at);
  if (schema.allOf) {
    for (const child of schema.allOf) validate(document, child, value, at);
  }
  if (schema.const !== undefined) assert(value === schema.const, `${at} must equal ${JSON.stringify(schema.const)}`);
  if (schema.enum) assert(schema.enum.some((entry) => entry === value), `${at} is outside enum`);
  if (schema.type) {
    const types = Array.isArray(schema.type) ? schema.type : [schema.type];
    assert(types.some((type) => matchesType(value, type)), `${at} has invalid type`);
    if (value === null) return;
  }
  if (schema.required) {
    for (const key of schema.required) assert(Object.hasOwn(value, key), `${at}.${key} is required`);
  }
  if (schema.additionalProperties === false && schema.properties) {
    for (const key of Object.keys(value)) assert(Object.hasOwn(schema.properties, key), `${at}.${key} is not allowed`);
  }
  if (schema.properties && value && typeof value === 'object' && !Array.isArray(value)) {
    for (const [key, child] of Object.entries(schema.properties)) {
      if (Object.hasOwn(value, key)) validate(document, child, value[key], `${at}.${key}`);
    }
  }
  if (schema.items && Array.isArray(value)) {
    value.forEach((item, index) => validate(document, schema.items, item, `${at}[${index}]`));
  }
  if (typeof value === 'string') {
    if (schema.minLength !== undefined) assert(value.length >= schema.minLength, `${at} is too short`);
    if (schema.maxLength !== undefined) assert(value.length <= schema.maxLength, `${at} is too long`);
    if (schema.pattern) assert(new RegExp(schema.pattern).test(value), `${at} does not match pattern`);
  }
  if (typeof value === 'number') {
    if (schema.minimum !== undefined) assert(value >= schema.minimum, `${at} is below minimum`);
    if (schema.maximum !== undefined) assert(value <= schema.maximum, `${at} is above maximum`);
  }
  if (Array.isArray(value)) {
    if (schema.minItems !== undefined) assert(value.length >= schema.minItems, `${at} has too few items`);
    if (schema.maxItems !== undefined) assert(value.length <= schema.maxItems, `${at} has too many items`);
  }
}

const openapi = readJson('collaboration-openapi.yaml');
const events = readJson('collaboration-events.schema.json');
const sourceDigest = crypto.createHash('sha256')
  .update(fs.readFileSync(path.join(root, 'collaboration-openapi.yaml'), 'utf8'))
  .update(fs.readFileSync(path.join(root, 'collaboration-events.schema.json'), 'utf8'))
  .digest('hex')
  .slice(0, 16);
assert(openapi.openapi === '3.1.0', 'OpenAPI must remain 3.1.0');
assert(openapi.servers[0].url === '/api/v1/collaboration', 'unexpected REST base path');
assert(openapi.security[0].panelBearer, 'Panel JWT security must be global');
assert(!JSON.stringify(openapi.components.schemas.Command).includes('amount'), 'mobile command DTO must not expose amount');
assert(!JSON.stringify(openapi.components.schemas.Command).includes('charge'), 'mobile command DTO must not expose charge');

const fixtures = [
  ['examples/collaboration-health.response.json', openapi.components.schemas.CollaborationHealthEnvelope],
  ['examples/register-device.request.json', openapi.components.schemas.RegisterDeviceRequest],
  ['examples/command.request.json', openapi.components.schemas.CreateCommandRequest],
];
for (const [file, schema] of fixtures) validate(openapi, schema, readJson(file), file);
validate(events, events, readJson('examples/ws-command-dispatched.json'), 'examples/ws-command-dispatched.json');

for (const file of [
  'generated/dart/collaboration_wire.dart',
  'generated/go/collaboration_wire.go',
  'generated/rust/collaboration_wire.rs',
  '../clients/mobile/lib/core/protocol/collaboration_wire.dart',
  '../clients/codex-pc/src-tauri/src/protocol/collaboration_wire.rs',
]) {
  const generated = fs.readFileSync(path.join(root, file), 'utf8');
  assert(generated.includes(sourceDigest), `${file} is stale; run protocol generator`);
}

const appServerFixture = fs.readFileSync(path.join(root, '../clients/codex-pc/tests/fixtures/app-server-basic.jsonl'), 'utf8');
for (const [index, line] of appServerFixture.trim().split('\n').entries()) {
  try { JSON.parse(line); } catch (error) { throw new Error(`invalid app-server JSONL line ${index + 1}: ${error.message}`); }
}

console.log(`validated ${fixtures.length + 1} collaboration fixtures`);
