import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Terms of Service",
  description: "The terms for using the hosted ErrorLens service at errorlens.io.",
};

const UPDATED = "26 November 2026";

export default function Terms() {
  return (
    <main className="legal">
      <h1>Terms of Service</h1>
      <p className="muted">Last updated: {UPDATED}</p>

      <p>
        These terms govern your use of the hosted ErrorLens service at errorlens.io and its subdomains (the{" "}
        <b>Service</b>), operated by Moin Khan, an individual based in Pakistan (<b>we</b>, <b>us</b>). By creating an
        account or using the Service you agree to them. If you use the Service for an organization, you confirm you are
        allowed to accept these terms on its behalf, and &quot;you&quot; includes that organization.
      </p>
      <p>
        The ErrorLens software is open source. Running it on your own servers is governed by its licenses — AGPL-3.0 for
        the server, MIT for the SDKs — not by these terms.
      </p>

      <h2>1. Your account</h2>
      <ul>
        <li>You must give accurate information and be at least 16 years old.</li>
        <li>Keep your password and API tokens secret. You are responsible for activity under your account and tokens. We strongly recommend turning on two-factor authentication.</li>
        <li>Tell us straight away at <a href="mailto:security@errorlens.io">security@errorlens.io</a> if you suspect unauthorized access.</li>
        <li>Organization owners control their organization&apos;s members, projects and data.</li>
      </ul>

      <h2>2. Your data</h2>
      <ul>
        <li>
          You keep all rights to the data your applications send to the Service (<b>customer data</b>). You give us
          permission to store and process it only as needed to provide, secure and support the Service for you.
        </li>
        <li>
          You are responsible for having the right to send customer data to us, and for complying with the privacy laws
          that apply to it — including telling your users, in your own privacy policy, that you use an error-monitoring
          service. Configure your SDKs and project privacy settings to send no more personal data than you need.
        </li>
        <li>
          Don&apos;t intentionally send special categories of data (such as health records), full payment card data or
          government identification numbers. ErrorLens tries to remove secrets automatically, but this is a safety net,
          not a guarantee.
        </li>
        <li>How we handle personal data is described in our <Link href="/privacy/">Privacy Policy</Link>.</li>
      </ul>

      <h2>3. Acceptable use</h2>
      <p>You agree not to:</p>
      <ul>
        <li>use the Service for anything unlawful, or to store or send content you have no right to;</li>
        <li>try to access other organizations&apos; data, probe or scan the Service for vulnerabilities without our written permission, or get around its security, quotas or rate limits;</li>
        <li>send traffic that is not genuine monitoring data from your own applications, or overload the Service on purpose;</li>
        <li>use uptime monitors or webhooks to attack, flood or scan other systems;</li>
        <li>resell the hosted Service, or use it to build a competing hosted service, without our permission.</li>
      </ul>
      <p>
        Found a security issue? Please report it to <a href="mailto:security@errorlens.io">security@errorlens.io</a>{" "}
        instead — we appreciate it.
      </p>

      <h2>4. Plans, quotas and payment</h2>
      <ul>
        <li>The free plan includes a monthly quota of error events, currently 5,000. Above the quota, new events are rejected until the next month.</li>
        <li>We may change the free plan&apos;s limits. We will give at least 30 days&apos; notice of any reduction.</li>
        <li>
          If you buy a paid plan, its price and limits are shown before you pay. Subscriptions renew automatically until
          cancelled and can be cancelled at any time, taking effect at the end of the paid period. Except where the law
          requires otherwise, payments are not refundable.
        </li>
      </ul>

      <h2>5. Availability and changes</h2>
      <p>
        We work hard to keep the Service available and your data safe, but we do not promise that it will be
        uninterrupted or error-free, and the free plan has no service-level agreement. We may change, add or remove
        features. If we decide to shut the Service down, we will give at least 60 days&apos; notice so you can move to
        another provider or to a self-hosted ErrorLens.
      </p>

      <h2>6. Ending your use</h2>
      <ul>
        <li>You can stop using the Service at any time, delete your projects, and ask us to delete your account.</li>
        <li>
          We may suspend or close an account that breaks these terms, puts the Service or other users at risk, or that we
          must close by law. Where reasonable, we will warn you first and give you a chance to fix the problem.
        </li>
        <li>After an account is closed, its data is deleted as described in the Privacy Policy.</li>
      </ul>

      <h2>7. Disclaimer</h2>
      <p>
        The Service is provided &quot;as is&quot; and &quot;as available&quot;. To the extent the law allows, we disclaim
        all warranties, express or implied, including fitness for a particular purpose and non-infringement. ErrorLens
        helps you find problems; it does not guarantee that every error, outage or failed job will be detected or that
        every alert will be delivered.
      </p>

      <h2>8. Limitation of liability</h2>
      <p>
        To the extent the law allows, we are not liable for indirect, incidental, special or consequential damages, or
        for lost profits, revenue or data. Our total liability for any claim relating to the Service is limited to the
        amount you paid us in the 12 months before the claim, or USD 100 if that is more. Nothing in these terms limits
        liability that cannot be limited by law.
      </p>

      <h2>9. Indemnity</h2>
      <p>
        You will defend and compensate us against claims by third parties arising from customer data you sent to the
        Service without the right to do so, or from your breach of these terms.
      </p>

      <h2>10. Changes to these terms</h2>
      <p>
        We may update these terms. For changes that matter, we will update the date above and email account owners at
        least 30 days before they take effect. If you keep using the Service after that, the new terms apply; if you
        don&apos;t agree, you can stop using it.
      </p>

      <h2>11. Governing law</h2>
      <p>
        These terms are governed by the laws of Pakistan. If you are a consumer, you also keep any protections the law
        of your country gives you. We would always rather solve a problem by talking first — please write to us.
      </p>

      <h2>12. Contact</h2>
      <p>
        Questions about these terms: <a href="mailto:legal@errorlens.io">legal@errorlens.io</a>.
      </p>
    </main>
  );
}
