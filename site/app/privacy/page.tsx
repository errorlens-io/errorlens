import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Privacy Policy",
  description: "What the ErrorLens hosted service collects, why, who processes it, how long it is kept, and your rights.",
};

const UPDATED = "26 November 2026";

export default function Privacy() {
  return (
    <main className="legal">
      <h1>Privacy Policy</h1>
      <p className="muted">Last updated: {UPDATED}</p>

      <p>
        This policy explains how personal data is handled by the hosted ErrorLens service at errorlens.io and its
        subdomains (the <b>Service</b>). It does not apply to copies of ErrorLens that people run on their own servers:
        we have no access to those and receive no data from them.
      </p>

      <h2>1. Who we are</h2>
      <p>
        The Service is operated by Moin Khan, an individual based in Pakistan (<b>we</b>, <b>us</b>). For anything about
        privacy, write to <a href="mailto:privacy@errorlens.io">privacy@errorlens.io</a>.
      </p>

      <h2>2. Two kinds of data</h2>
      <p>We handle personal data in two different roles:</p>
      <ul>
        <li>
          <b>Account data</b> — information about you as a user of the Service. We decide how it is used, so we are its
          <i> controller</i>.
        </li>
        <li>
          <b>Customer data</b> — the errors, performance data, logs and check-ins your applications send to the Service.
          It may contain personal data about your own users. You decide what is sent and why; we store and process it
          only to provide the Service to you, so for this data you are the controller and we are your
          <i> processor</i>.
        </li>
      </ul>

      <h2>3. Account data we collect</h2>
      <table>
        <thead><tr><th>Data</th><th>Why</th><th>Legal basis</th></tr></thead>
        <tbody>
          <tr><td>Name, email address, password (stored only as a one-way hash), two-factor settings</td><td>To create and secure your account and sign you in</td><td>Performing our contract with you</td></tr>
          <tr><td>Organization names, memberships, roles and invitations</td><td>To run your organization and its access controls</td><td>Contract</td></tr>
          <tr><td>IP address, browser user agent, sign-in times</td><td>Security: session management, rate limiting, the audit log, preventing abuse</td><td>Our legitimate interest in keeping the Service secure</td></tr>
          <tr><td>Audit log entries (for example, who opened which event)</td><td>So your organization can see who accessed its data</td><td>Contract and legitimate interest</td></tr>
          <tr><td>Usage counts (events received per day)</td><td>To apply quotas and show your usage</td><td>Contract</td></tr>
          <tr><td>Messages you send us</td><td>To answer you</td><td>Legitimate interest</td></tr>
          <tr><td>Billing details, if you subscribe to a paid plan</td><td>To take payment — card details are handled by our payment provider and never reach us</td><td>Contract and legal obligations</td></tr>
        </tbody>
      </table>
      <p>
        We do not use advertising or analytics trackers, and we do not set cookies other than the ones needed to keep
        you signed in and protect sign-in against attacks. We do not sell personal data or share it for advertising.
      </p>

      <h2>4. How we protect customer data</h2>
      <ul>
        <li>Passwords, tokens, keys, cookies, card numbers and similar secrets are <Link href="/docs/data-scrubbing/">removed on arrival</Link>, before anything is stored. You can add your own rules per project.</li>
        <li>End-user IP addresses are truncated by default.</li>
        <li>Event contents are <Link href="/docs/encryption/">encrypted at rest</Link> with AES-256-GCM, with a separate key per organization.</li>
        <li>Each organization&apos;s data is isolated in the database with row-level security.</li>
        <li>Every time someone opens an event, it is recorded in your organization&apos;s audit log.</li>
        <li>
          We do not look at customer data, except when you ask us to for support, or when strictly necessary to keep the
          Service running, investigate abuse or comply with the law.
        </li>
      </ul>
      <p>
        You are responsible for having a lawful basis to send personal data about your users to the Service, and for
        configuring your SDKs and privacy settings to send no more than you need.
      </p>

      <h2>5. Service providers</h2>
      <p>We use a small number of providers to run the Service. They process data only on our instructions:</p>
      <table>
        <thead><tr><th>Provider</th><th>Purpose</th><th>Data</th></tr></thead>
        <tbody>
          <tr><td>Cloudflare, Inc.</td><td>DNS, network protection, content delivery and forwarding of email sent to errorlens.io addresses</td><td>All traffic to the Service passes through it, encrypted; email sent to us</td></tr>
          <tr><td>Hostinger International Ltd.</td><td>The virtual server and storage the Service runs on</td><td>All account and customer data, encrypted at rest as described above</td></tr>
          <tr><td>Resend (Plus Five Five, Inc.)</td><td>Sending email: confirmations, invitations, password resets and alerts</td><td>Recipient address and the email&apos;s content</td></tr>
          <tr><td>Stripe, Inc. (paid plans only)</td><td>Payment processing</td><td>Billing contact and payment details</td></tr>
        </tbody>
      </table>
      <p>
        We may share data when required by law, to protect the rights and safety of people or the Service, or as part of
        a transfer of the Service to a new operator, who would be bound by this policy.
      </p>

      <h2>6. International transfers</h2>
      <p>
        We are based in Pakistan and our providers operate in several countries, so your data may be processed outside
        your own country. Where the law requires it, we rely on safeguards such as the European Commission&apos;s standard
        contractual clauses offered by our providers.
      </p>

      <h2>7. How long we keep data</h2>
      <ul>
        <li><b>Customer data</b>: for each project&apos;s retention period — 30 days unless you change it — then deleted automatically. Deleting a project deletes its data immediately.</li>
        <li><b>Account data</b>: while your account exists. When you ask us to delete your account, we delete it within 30 days, except what we must keep by law.</li>
        <li><b>Audit log entries</b>: for as long as the organization exists, so its owners can review access.</li>
      </ul>

      <h2>8. Your rights</h2>
      <p>
        Depending on where you live, you may have the right to access your personal data, correct it, delete it, object
        to or restrict how we use it, receive a copy in a portable format, and withdraw consent. You can change most
        account data yourself in the dashboard. For anything else, email{" "}
        <a href="mailto:privacy@errorlens.io">privacy@errorlens.io</a>; we answer within 30 days. You may also complain
        to your local data protection authority.
      </p>
      <p>
        If you are an end user of an application that uses ErrorLens, please contact that application&apos;s owner: they
        control that data, and we will help them answer your request.
      </p>

      <h2>9. Security incidents</h2>
      <p>
        If a breach affects personal data we hold, we will notify the affected organizations without undue delay and give
        them the information they need to meet their own obligations.
      </p>

      <h2>10. Children</h2>
      <p>The Service is for developers and businesses, and is not intended for anyone under 16.</p>

      <h2>11. Changes</h2>
      <p>
        If we change this policy in a way that matters, we will update the date above and tell account owners by email
        before the change takes effect.
      </p>

      <h2>12. Contact</h2>
      <p>
        Privacy questions: <a href="mailto:privacy@errorlens.io">privacy@errorlens.io</a>. Security reports:{" "}
        <a href="mailto:security@errorlens.io">security@errorlens.io</a>. See also our{" "}
        <Link href="/terms/">Terms of Service</Link>.
      </p>
    </main>
  );
}
