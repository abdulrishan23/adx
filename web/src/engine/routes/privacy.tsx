export const privacyContact = "hisham.ahamad@wokeworks.ai";

export function PrivacyPage() {
  return (
    <article className="mx-auto max-w-3xl space-y-10 leading-7">
      <header className="space-y-4 border-b pb-8">
        <p className="text-sm text-muted-foreground">WokeWorks · ADX for Reportage</p>
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Privacy policy</h1>
        <p className="text-sm text-muted-foreground">Last updated: 13 October 2026</p>
        <p>
          This policy explains how ADX accesses, uses, stores and shares information. It also
          explains how to disconnect accounts and request deletion of your data.
        </p>
        <a className="underline underline-offset-4" href="#data-deletion">
          Request data deletion
        </a>
      </header>

      <section aria-labelledby="about-adx" className="space-y-4">
        <h2 id="about-adx" className="text-xl font-semibold">
          Who we are
        </h2>
        <p>
          WokeWorks develops and operates ADX for Reportage. ADX helps its authorized marketing
          staff report on advertising, understand lead sources and evaluate campaign results.
        </p>
        <p>
          Reportage determines how its customer and lead records are used. WokeWorks processes those
          records to provide ADX and support Reportage&apos;s authorized instructions.
        </p>
        <p>
          For privacy questions, contact Hisham Ahamad at{" "}
          <a href={`mailto:${privacyContact}`} className="underline underline-offset-4">
            {privacyContact}
          </a>
          .
        </p>
      </section>

      <section aria-labelledby="data-access" className="space-y-4">
        <h2 id="data-access" className="text-xl font-semibold">
          Information we access and collect
        </h2>
        <p>
          The information processed depends on the connections you authorize and the features you
          use.
        </p>
        <ul className="list-disc space-y-3 pl-5">
          <li>
            <strong>Google Ads:</strong> account identifiers, currency, campaigns, ad groups, ads,
            keywords, search terms and performance figures, including spend and conversion totals.
          </li>
          <li>
            <strong>Meta:</strong> business, ad account, Page and Instagram identifiers; campaign
            and ad performance; Page and Instagram insights; and authorized lead form submissions.
          </li>
          <li>
            <strong>GoHighLevel:</strong> lead identifiers, names, phone numbers, email addresses,
            sources, statuses, project information and custom fields. This can include deal
            information supplied to GoHighLevel by Reportage&apos;s other systems.
          </li>
          <li>
            <strong>ADX accounts:</strong> user names, email addresses, roles and records of actions
            within the service.
          </li>
          <li>
            <strong>Connection and technical data:</strong> authorization tokens, connection
            settings, sync history, request identifiers, timestamps and diagnostic logs needed to
            operate ADX.
          </li>
        </ul>
        <p>
          Connecting an account authorizes access only to the permissions granted for that
          connection.
        </p>
      </section>

      <section aria-labelledby="data-use" className="space-y-4">
        <h2 id="data-use" className="text-xl font-semibold">
          How we use information
        </h2>
        <ul className="list-disc space-y-3 pl-5">
          <li>
            Produce advertising reports and compare campaign performance with lead and sales
            outcomes.
          </li>
          <li>
            Attribute leads to sources and projects, calculate lead scores, and identify
            data-quality gaps.
          </li>
          <li>
            Provide authorized users with reports, alerts and answers based on connected account
            data.
          </li>
          <li>
            Carry out account changes or outcome uploads only when the relevant feature is enabled
            and the action has the required authorization.
          </li>
          <li>
            Maintain connections, investigate failures, protect the service and respond to support
            requests.
          </li>
        </ul>
        <p>
          We do not sell personal information or connected account data. We do not use Google API
          data to train general-purpose AI or machine-learning models.
        </p>
      </section>

      <section aria-labelledby="google-data" className="space-y-4">
        <h2 id="google-data" className="text-xl font-semibold">
          Google API data
        </h2>
        <p>
          ADX&apos;s use and transfer of information received from Google APIs follows the{" "}
          <a
            href="https://developers.google.com/terms/api-services-user-data-policy"
            className="underline underline-offset-4"
          >
            Google API Services User Data Policy
          </a>
          , including its Limited Use requirements where applicable.
        </p>
        <p>
          Google data is used to provide the reporting and account features you authorize. It is not
          sold to data brokers or used for unrelated advertising, credit decisions or surveillance.
        </p>
        <p>
          Transfers are limited to authorized service features, security needs, legal requirements,
          or other circumstances permitted by Google&apos;s policy.
        </p>
      </section>

      <section aria-labelledby="data-sharing" className="space-y-4">
        <h2 id="data-sharing" className="text-xl font-semibold">
          Who can receive information
        </h2>
        <ul className="list-disc space-y-3 pl-5">
          <li>
            Authorized Reportage users and WokeWorks staff who provide the service or support it.
          </li>
          <li>
            Hosting, backup and diagnostic service providers needed to operate and protect ADX.
          </li>
          <li>
            Connected platforms when an authorized action sends information back to that platform.
          </li>
          <li>
            Authorities when disclosure is required by law, or necessary to protect rights and
            security.
          </li>
        </ul>
        <p>
          When you use ADX through Claude, the tool results you request are sent to Anthropic
          through that client. This can include campaign figures and aggregated lead information.
        </p>
        <p>
          Phone numbers and email addresses are excluded from MCP lead tools unless Reportage
          authorizes that transfer. Claude also operates under the terms and privacy settings of
          your Claude account.
        </p>
      </section>

      <section aria-labelledby="hosting-security" className="space-y-4">
        <h2 id="hosting-security" className="text-xl font-semibold">
          Hosting and security
        </h2>
        <p>
          ADX uses WokeWorks&apos; Hostinger infrastructure in Mumbai, India. Production lead data
          is hosted there only with Reportage&apos;s authorization. Service providers can process
          information in other countries under the arrangements for those services.
        </p>
        <p>
          Access to connected account data is restricted to authorized use. We apply technical and
          organizational measures appropriate to the data, including protection of connection
          credentials and controls on access to operational systems.
        </p>
        <p>
          No system can guarantee absolute security. Report suspected unauthorized access to the
          privacy contact.
        </p>
      </section>

      <section aria-labelledby="data-retention" className="space-y-4">
        <h2 id="data-retention" className="text-xl font-semibold">
          How long we keep information
        </h2>
        <p>
          We retain information only while needed for the authorized service, its reporting history,
          security and support, or applicable legal obligations. The period depends on the data and
          the purpose for which it is held.
        </p>
        <p>
          Information that is no longer needed is deleted or anonymized. Revocation of access stops
          further authorized retrieval but does not, by itself, delete information already stored.
          You can request deletion using the instructions below.
        </p>
        <p>
          Where backups are used, removed data can remain until those backups are replaced or
          expire. It is not used for ordinary operations. Completed deletion requests must also be
          applied if a backup is restored.
        </p>
      </section>

      <section aria-labelledby="disconnect" className="space-y-4">
        <h2 id="disconnect" className="text-xl font-semibold">
          Disconnect an account
        </h2>
        <p>
          You can revoke Google access through your{" "}
          <a
            href="https://myaccount.google.com/connections"
            className="underline underline-offset-4"
          >
            Google Account connections
          </a>{" "}
          or remove Meta access through Facebook&apos;s Apps and Websites or Business Integrations
          settings. Your GoHighLevel administrator can revoke the relevant integration credentials.
        </p>
        <p>Contact us if you also want the information already held by ADX deleted.</p>
      </section>

      <section aria-labelledby="data-deletion" className="scroll-mt-8 space-y-4 border-t pt-8">
        <h2 id="data-deletion" className="text-xl font-semibold">
          Request data deletion
        </h2>
        <p>
          Email{" "}
          <a
            href={`mailto:${privacyContact}?subject=ADX%20data%20deletion%20request`}
            className="underline underline-offset-4"
          >
            {privacyContact}
          </a>{" "}
          with the subject <strong>ADX data deletion request</strong>.
        </p>
        <ol className="list-decimal space-y-3 pl-5">
          <li>Identify the connected account or lead record affected by the request.</li>
          <li>
            Describe the information you want deleted and your relationship to that account or
            record.
          </li>
          <li>Provide an email address where we can contact you about the request.</li>
        </ol>
        <p>Do not send passwords, access tokens or unnecessary personal information.</p>
        <p>
          We review requests, confirm identity or authority when necessary, and coordinate with
          Reportage for records it controls. We explain the outcome and any information retained
          because of a legal obligation or other permitted reason.
        </p>
        <p>
          Deletion applies to the information held by ADX. It does not automatically delete the
          original record in Google, Meta or GoHighLevel. A source record can be imported again
          while its connection remains active; tell us if the request also requires stopping that
          import.
        </p>
        <p>You can also use this contact to request access to or correction of your information.</p>
      </section>

      <section aria-labelledby="policy-updates" className="space-y-4">
        <h2 id="policy-updates" className="text-xl font-semibold">
          Changes to this policy
        </h2>
        <p>
          We update this page when data practices change and revise the date at the top. We provide
          notice and obtain additional authorization when required before using data for a new
          purpose.
        </p>
      </section>
    </article>
  );
}
