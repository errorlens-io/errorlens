/**
 * ErrorLens SDK for Node.js.
 *
 * Event capture and transport come from the MIT-licensed open-source error
 * SDK this package builds on; this module adds ErrorLens defaults and a
 * ErrorLens-branded API.
 */
import * as core from "@sentry/node";

export const SDK_NAME = "errorlens.javascript.node";
export const SDK_VERSION = "0.1.0";

type CoreOptions = NonNullable<Parameters<typeof core.init>[0]>;
type CoreEvent = Parameters<NonNullable<CoreOptions["beforeSend"]>>[0];

export type ErrorLensOptions = CoreOptions;

declare const process: { env: Record<string, string | undefined> } | undefined;

function env(name: string): string | undefined {
  return typeof process !== "undefined" ? process.env[name] || undefined : undefined;
}

function brand(event: CoreEvent): CoreEvent {
  event.sdk = {
    name: SDK_NAME,
    version: SDK_VERSION,
    packages: [{ name: "npm:@errorlens/node", version: SDK_VERSION }],
  };
  return event;
}

/**
 * Start reporting errors to ErrorLens. dsn, release and environment default to
 * ERRORLENS_DSN, ERRORLENS_RELEASE and ERRORLENS_ENVIRONMENT. Without a DSN the
 * SDK does nothing, so it is safe to call in tests and local development.
 */
export function init(options: ErrorLensOptions = {}) {
  const userBeforeSend = options.beforeSend;
  return core.init({
    ...options,
    // Identifies events as coming from the ErrorLens SDK.
    _metadata: { sdk: { name: SDK_NAME, version: SDK_VERSION, packages: [{ name: "npm:@errorlens/node", version: SDK_VERSION }] } },
    dsn: options.dsn ?? env("ERRORLENS_DSN"),
    release: options.release ?? env("ERRORLENS_RELEASE"),
    environment: options.environment ?? env("ERRORLENS_ENVIRONMENT"),
    beforeSend: (event, hint) => {
      const e = brand(event as CoreEvent);
      return userBeforeSend ? userBeforeSend(e as never, hint) : e;
    },
  });
}

export const {
  captureException,
  captureMessage,
  captureEvent,
  addBreadcrumb,
  setUser,
  setTag,
  setTags,
  setContext,
  setExtra,
  setExtras,
  withScope,
  withIsolationScope,
  flush,
  close,
  lastEventId,
  isEnabled,
  setupExpressErrorHandler,
  setupFastifyErrorHandler,
  setupKoaErrorHandler,
  setupHapiErrorHandler,
} = core;
