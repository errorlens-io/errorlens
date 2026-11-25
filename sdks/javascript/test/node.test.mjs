// Runs the Node SDK against an in-process HTTP server that records envelopes.
import { test, after } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import zlib from "node:zlib";
import { createRequire } from "node:module";

const bodies = [];
const server = http.createServer((req, res) => {
  const chunks = [];
  req.on("data", (c) => chunks.push(c));
  req.on("end", () => {
    let buf = Buffer.concat(chunks);
    if (req.headers["content-encoding"] === "gzip") buf = zlib.gunzipSync(buf);
    bodies.push(buf.toString());
    res.writeHead(200, { "Content-Type": "application/json" }).end("{}");
  });
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const dsn = `http://${"b".repeat(32)}@127.0.0.1:${server.address().port}/3`;

after(() => server.close());

function events() {
  const out = [];
  for (const body of bodies) {
    const lines = body.split("\n");
    lines.forEach((line, i) => {
      try {
        if (JSON.parse(line).type === "event") out.push(JSON.parse(lines[i + 1]));
      } catch {}
    });
  }
  return out;
}

test("ESM and CJS builds expose the same API", () => {
  const require = createRequire(import.meta.url);
  const cjs = require("@rootcausehq/node");
  assert.equal(typeof cjs.init, "function");
  assert.equal(typeof cjs.captureException, "function");
  assert.equal(cjs.SDK_NAME, "rootcause.javascript.node");
});

test("reports branded events with release/environment from env vars", async () => {
  process.env.ROOTCAUSE_DSN = dsn;
  process.env.ROOTCAUSE_RELEASE = "shop@3.1.0";
  process.env.ROOTCAUSE_ENVIRONMENT = "ci";
  const rc = await import("@rootcausehq/node");
  let hookSaw = "";
  rc.init({ beforeSend: (e) => ((hookSaw = e.sdk.name), e), defaultIntegrations: false });
  assert.ok(rc.isEnabled());
  rc.setTag("region", "eu");
  rc.captureException(new TypeError("cannot read properties of undefined"));
  await rc.flush(5000);

  const [ev] = events();
  assert.ok(ev, "no event received");
  assert.equal(ev.sdk.name, "rootcause.javascript.node");
  assert.equal(hookSaw, "rootcause.javascript.node");
  assert.equal(ev.release, "shop@3.1.0");
  assert.equal(ev.environment, "ci");
  assert.equal(ev.tags.region, "eu");
  assert.equal(ev.exception.values.at(-1).type, "TypeError");
  assert.ok(!JSON.stringify(ev.sdk).toLowerCase().includes("sentry"), JSON.stringify(ev.sdk));
  await rc.close(2000);
});
