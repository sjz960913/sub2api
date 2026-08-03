# Collaboration protocol

This directory is the wire-contract source of truth shared by the Go backend,
Flutter client and PC companion.

- `collaboration-openapi.yaml` is OpenAPI 3.1 expressed as JSON-compatible
  YAML. Keeping the source JSON-compatible lets the repository validate it
  without adding a YAML parser to the build image.
- `collaboration-events.schema.json` defines the WebSocket envelope and event
  names.
- `examples/` contains redacted contract fixtures.
- `generated/` contains deterministic language DTOs. Do not edit them by hand.

Validate the current contract with:

```bash
node protocol/scripts/generate.mjs
node protocol/scripts/validate.mjs
node protocol/scripts/mock-smoke.mjs
```

Use Node.js 20 or newer. CI must fail when generated files differ after running
the generator.

The mobile API never receives collaboration charge fields. Billing remains a
server-side concern.
