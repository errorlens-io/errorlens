import { useState } from "react";

type Guide = { install: string; code: (dsn: string) => string; lang: string };

const guides: Record<string, Guide> = {
  python: {
    lang: "Python",
    install: "pip install rootcausehq",
    code: (dsn) => `import rootcausehq

rootcausehq.init(
    dsn="${dsn}",
    release="my-app@1.0.0",
    environment="production",
)`,
  },
  node: {
    lang: "Node.js",
    install: "npm install @rootcausehq/node",
    code: (dsn) => `// Load this before anything else in your app.
import * as RootCause from "@rootcausehq/node";

RootCause.init({
  dsn: "${dsn}",
  release: "my-app@1.0.0",
  environment: "production",
});`,
  },
  javascript: {
    lang: "Browser JavaScript",
    install: "npm install @rootcausehq/browser",
    code: (dsn) => `import * as RootCause from "@rootcausehq/browser";

RootCause.init({
  dsn: "${dsn}",
  release: "my-app@1.0.0",
  environment: "production",
});`,
  },
};

const test: Record<string, string> = {
  python: "rootcausehq.capture_message(\"Hello from RootCause\")",
  node: "RootCause.captureMessage(\"Hello from RootCause\");",
  javascript: "RootCause.captureMessage(\"Hello from RootCause\");",
};

export function SetupGuide({ platform, dsn }: { platform: string; dsn: string }) {
  const g = guides[platform];
  if (!g) {
    return (
      <p className="muted small">
        A RootCause SDK for <strong>{platform}</strong> is on the way. Until then, any client that speaks the standard
        error-envelope protocol works with this DSN.
      </p>
    );
  }
  return (
    <div className="setup">
      <Step n={1} title={`Install the ${g.lang} SDK`} code={g.install} />
      <Step n={2} title="Initialize it as early as possible" code={g.code(dsn)} />
      <Step n={3} title="Send a test event" code={test[platform]} />
    </div>
  );
}

function Step({ n, title, code }: { n: number; title: string; code: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="setup-step">
      <div className="setup-title"><span className="step-n">{n}</span>{title}</div>
      <div className="code-block">
        <pre className="mono">{code}</pre>
        <button className="ghost copy" onClick={async () => {
          await navigator.clipboard.writeText(code);
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        }}>{copied ? "Copied" : "Copy"}</button>
      </div>
    </div>
  );
}
