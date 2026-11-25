/**
 * RootCause SDK for browsers.
 *
 * Event capture and transport come from the MIT-licensed open-source error
 * SDK this package builds on; this module adds RootCause defaults and a
 * RootCause-branded API.
 */
import * as core from "@sentry/browser";

export const SDK_NAME = "rootcause.javascript.browser";
export const SDK_VERSION = "0.1.0";

type CoreOptions = NonNullable<Parameters<typeof core.init>[0]>;
type CoreEvent = Parameters<NonNullable<CoreOptions["beforeSend"]>>[0];

export type RootCauseOptions = CoreOptions;

function brand(event: CoreEvent): CoreEvent {
  event.sdk = {
    name: SDK_NAME,
    version: SDK_VERSION,
    packages: [{ name: "npm:@rootcausehq/browser", version: SDK_VERSION }],
  };
  return event;
}

/** Start reporting errors to RootCause. Without a DSN the SDK does nothing. */
export function init(options: RootCauseOptions = {}) {
  const userBeforeSend = options.beforeSend;
  return core.init({
    ...options,
    // Identifies events as coming from the RootCause SDK.
    _metadata: { sdk: { name: SDK_NAME, version: SDK_VERSION, packages: [{ name: "npm:@rootcausehq/browser", version: SDK_VERSION }] } },
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
