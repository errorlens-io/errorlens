/**
 * ErrorLens SDK for browsers.
 *
 * Event capture and transport come from the MIT-licensed open-source error
 * SDK this package builds on; this module adds ErrorLens defaults and a
 * ErrorLens-branded API.
 */
import * as core from "@sentry/browser";

export const SDK_NAME = "errorlens.javascript.browser";
export const SDK_VERSION = "0.2.0";

type CoreOptions = NonNullable<Parameters<typeof core.init>[0]>;
type CoreEvent = Parameters<NonNullable<CoreOptions["beforeSend"]>>[0];

export type ErrorLensOptions = CoreOptions;

function brand(event: CoreEvent): CoreEvent {
  event.sdk = {
    name: SDK_NAME,
    version: SDK_VERSION,
    packages: [{ name: "npm:@errorlens/browser", version: SDK_VERSION }],
  };
  return event;
}

/** Start reporting errors to ErrorLens. Without a DSN the SDK does nothing. */
export function init(options: ErrorLensOptions = {}) {
  const userBeforeSend = options.beforeSend;
  return core.init({
    ...options,
    // Identifies events as coming from the ErrorLens SDK.
    _metadata: { sdk: { name: SDK_NAME, version: SDK_VERSION, packages: [{ name: "npm:@errorlens/browser", version: SDK_VERSION }] } },
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
  flush,
  close,
  lastEventId,
  isEnabled,
} = core;
