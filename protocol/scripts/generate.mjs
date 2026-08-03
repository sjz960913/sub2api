import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');
const sourceText = fs.readFileSync(path.join(root, 'collaboration-openapi.yaml'), 'utf8');
const eventText = fs.readFileSync(path.join(root, 'collaboration-events.schema.json'), 'utf8');
const openapi = JSON.parse(sourceText);
const eventSchema = JSON.parse(eventText);
const digest = crypto.createHash('sha256').update(sourceText).update(eventText).digest('hex').slice(0, 16);
const schemas = {...openapi.components.schemas, EventEnvelope: eventSchema};

const targets = [
  'CollaborationHealth', 'DeviceCapabilities', 'RegisterDeviceRequest',
  'RegisterDeviceResult', 'Device', 'RenameDeviceRequest', 'SessionSyncRequest',
  'ThreadSyncRequest', 'SyncAccepted', 'CodexSession', 'SessionSync',
  'ThreadSummary', 'ContentPart', 'ThreadItem', 'ThreadSync',
  'CreateCommandRequest', 'CommandInput', 'ClientContext', 'Command',
  'ErrorSummary', 'EventEnvelope',
];

function refName(ref) {
  return ref.split('/').at(-1);
}

function lowerCamel(value) {
  const [first, ...rest] = value.split('_');
  return first + rest.map((part) => part[0].toUpperCase() + part.slice(1)).join('');
}

function resolved(schema) {
  return schema.$ref ? schemas[refName(schema.$ref)] : schema;
}

function nullable(schema) {
  const value = resolved(schema).type;
  return Array.isArray(value) && value.includes('null');
}

function baseType(schema) {
  const value = resolved(schema).type;
  if (Array.isArray(value)) return value.find((entry) => entry !== 'null');
  return value;
}

function isObjectRef(schema) {
  return Boolean(schema.$ref && baseType(schema) === 'object');
}

function dartType(schema, forceNullable = false) {
  let type;
  if (schema.$ref && isObjectRef(schema)) type = refName(schema.$ref);
  else if (baseType(schema) === 'array') type = `List<${dartType(resolved(schema).items).replace(/\?$/, '')}>`;
  else if (baseType(schema) === 'integer') type = 'int';
  else if (baseType(schema) === 'number') type = 'double';
  else if (baseType(schema) === 'boolean') type = 'bool';
  else if (baseType(schema) === 'object') type = 'Map<String, dynamic>';
  else type = 'String';
  return nullable(schema) || forceNullable ? `${type}?` : type;
}

function goType(schema) {
  let type;
  if (schema.$ref && isObjectRef(schema)) type = refName(schema.$ref);
  else if (baseType(schema) === 'array') type = `[]${goType(resolved(schema).items).replace(/^\*/, '')}`;
  else if (baseType(schema) === 'integer') type = 'int64';
  else if (baseType(schema) === 'number') type = 'float64';
  else if (baseType(schema) === 'boolean') type = 'bool';
  else if (baseType(schema) === 'object') type = 'map[string]any';
  else type = 'string';
  return nullable(schema) && !type.startsWith('[]') ? `*${type}` : type;
}

function rustType(schema) {
  let type;
  if (schema.$ref && isObjectRef(schema)) type = refName(schema.$ref);
  else if (baseType(schema) === 'array') type = `Vec<${rustType(resolved(schema).items).replace(/^Option<(.+)>$/, '$1')}>`;
  else if (baseType(schema) === 'integer') type = 'i64';
  else if (baseType(schema) === 'number') type = 'f64';
  else if (baseType(schema) === 'boolean') type = 'bool';
  else if (baseType(schema) === 'object') type = 'serde_json::Map<String, serde_json::Value>';
  else type = 'String';
  return nullable(schema) ? `Option<${type}>` : type;
}

function dartDecode(schema, access, forceNullable = false) {
  if (schema.$ref && isObjectRef(schema)) {
    const name = refName(schema.$ref);
    return nullable(schema) || forceNullable
      ? `${access} == null ? null : ${name}.fromJson(${access} as Map<String, dynamic>)`
      : `${name}.fromJson(${access} as Map<String, dynamic>)`;
  }
  if (baseType(schema) === 'array') {
    const item = resolved(schema).items;
    const itemExpression = item.$ref && isObjectRef(item)
      ? `${refName(item.$ref)}.fromJson(item as Map<String, dynamic>)`
      : `item as ${dartType(item).replace(/\?$/, '')}`;
    const decoded = `(${access} as List<dynamic>).map((item) => ${itemExpression}).toList()`;
    return forceNullable || nullable(schema) ? `${access} == null ? null : ${decoded}` : decoded;
  }
  const type = dartType(schema, forceNullable);
  if (baseType(schema) === 'object') return `${access} as Map<String, dynamic>${nullable(schema) || forceNullable ? '?' : ''}`;
  if (nullable(schema) || forceNullable) return `${access} as ${type}`;
  return `${access} as ${type}`;
}

function dartEncode(schema, field) {
  if (schema.$ref && isObjectRef(schema)) return nullable(schema) ? `${field}?.toJson()` : `${field}.toJson()`;
  if (baseType(schema) === 'array' && resolved(schema).items.$ref && isObjectRef(resolved(schema).items)) {
    return `${field}.map((item) => item.toJson()).toList()`;
  }
  return field;
}

function generateDart() {
  const blocks = targets.map((name) => {
    const schema = schemas[name];
    const properties = schema.properties ?? {};
    const required = new Set(schema.required ?? []);
    const fields = Object.entries(properties).map(([key, value]) => `  final ${dartType(value, !required.has(key))} ${lowerCamel(key)};`).join('\n');
    const constructor = Object.entries(properties).map(([key, value]) => {
      const optional = !required.has(key) || nullable(value);
      return `    ${optional ? '' : 'required '}this.${lowerCamel(key)},`;
    }).join('\n');
    const decode = Object.entries(properties).map(([key, value]) => `      ${lowerCamel(key)}: ${dartDecode(value, `json['${key}']`, !required.has(key))},`).join('\n');
    const encode = Object.entries(properties).map(([key, value]) => `      '${key}': ${dartEncode(value, lowerCamel(key))},`).join('\n');
    return `class ${name} {\n${fields}\n\n  const ${name}({\n${constructor}\n  });\n\n  factory ${name}.fromJson(Map<String, dynamic> json) => ${name}(\n${decode}\n  );\n\n  Map<String, dynamic> toJson() => {\n${encode}\n  };\n}`;
  });
  return `// GENERATED FILE. Source digest: ${digest}\n// Run: node protocol/scripts/generate.mjs\n\n${blocks.join('\n\n')}\n`;
}

function generateGo() {
  const blocks = targets.map((name) => {
    const schema = schemas[name];
    const required = new Set(schema.required ?? []);
    const fields = Object.entries(schema.properties ?? {}).map(([key, value]) => {
      const tag = required.has(key) ? key : `${key},omitempty`;
      return `\t${key.split('_').map((part) => part[0].toUpperCase() + part.slice(1)).join('')} ${goType(value)} \`json:"${tag}"\``;
    }).join('\n');
    return `type ${name} struct {\n${fields}\n}`;
  });
  return `// Code generated by protocol/scripts/generate.mjs; DO NOT EDIT.\n// Source digest: ${digest}\n\npackage collaborationwire\n\n${blocks.join('\n\n')}\n`;
}

function generateRust() {
  const blocks = targets.map((name) => {
    const schema = schemas[name];
    const required = new Set(schema.required ?? []);
    const fields = Object.entries(schema.properties ?? {}).map(([key, value]) => {
      let type = rustType(value);
      if (!required.has(key) && !type.startsWith('Option<')) type = `Option<${type}>`;
      const attribute = type.startsWith('Option<') ? '    #[serde(skip_serializing_if = "Option::is_none")]\n' : '';
      return `${attribute}    pub ${key}: ${type},`;
    }).join('\n');
    return `#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]\npub struct ${name} {\n${fields}\n}`;
  });
  return `// Generated by protocol/scripts/generate.mjs; DO NOT EDIT.\n// Source digest: ${digest}\n\nuse serde::{Deserialize, Serialize};\n\n${blocks.join('\n\n')}\n`;
}

const outputs = [
  ['generated/dart/collaboration_wire.dart', generateDart()],
  ['generated/go/collaboration_wire.go', generateGo()],
  ['generated/rust/collaboration_wire.rs', generateRust()],
  ['../clients/mobile/lib/core/protocol/collaboration_wire.dart', generateDart()],
  ['../clients/codex-pc/src-tauri/src/protocol/collaboration_wire.rs', generateRust()],
];
for (const [relativePath, content] of outputs) {
  const outputPath = path.join(root, relativePath);
  fs.mkdirSync(path.dirname(outputPath), {recursive: true});
  fs.writeFileSync(outputPath, content);
}
console.log(`generated ${outputs.length} DTO files from ${digest}`);
