# Google brand verification

Done on 2026-10-26: the Google Cloud project behind ADX's Google sign-in is brand verified, and the consent screen shows "Your branding has been verified and is being shown to users." This runbook is for the next time it's needed: a changed app name or logo, a new project, or a "Need to re-verify" status.

## Why it matters

ADX gets Keyword Planner data only once its Google Cloud project has **Basic** access to the Google Ads API, and Google lets a project apply for Basic only after it is brand verified. On **Explorer** access, the cap is 2,880 operations a day and Keyword Planner is blocked. Brand verification confirms that the app's name, logo, home page and privacy policy belong to the same owner. The automated check takes minutes; a manual review two to three business days. A brand-verified project's Basic application is reviewed in hours rather than days.

Verification needs the OAuth app to be **External** and **In production**. In production, sign-ins no longer expire every seven days, but the Google Ads scope is sensitive, so the "Google hasn't verified this app" warning and the 100-user cap stay until a separate data-access verification, which Basic access does not need.

## Before starting

Most rejections come from a missing or mismatched item here.

- Owner or Editor on the Cloud project ADX uses for Google sign-in.
- App name as people see it at sign-in: **ADX**. It must match what the home page calls the app.
- A square logo of ADX's own mark.
- A support email and a developer contact email someone reads daily. Google's reviewers write to these. The support email is `engineering@wokeworks.ai`.
- A public home page (no login wall) that says what ADX does and links to the privacy policy.
- A privacy policy on the same domain as the home page, saying how ADX accesses, uses, stores and shares Google user data (read-only Google Ads reporting and keyword planning).
- The domain verified in Search Console by an account that owns it and is an Owner or Editor on the Cloud project.
- Unused OAuth clients deleted.

The home page, privacy policy and authorized domain must all use the same top-level domain.

## Steps

About 30 minutes when the home page and privacy policy already exist.

**A. Verify the domain** (skip if done). In Search Console, with the account that owns the domain, add a Domain property for the top-level domain (not a subdomain), add the TXT record it shows to the DNS, and click Verify.

**B. External and In production.** In Cloud Console, pick the ADX project, open APIs & Services → OAuth consent screen (Google Auth Platform) → Audience. If the user type is Internal, click Make external. If the publishing status is Testing, click Publish app and confirm.

**C. Branding.** On the Branding tab, enter the app name, support email and logo. Under App domain, add the home page, the privacy policy and, optionally, the terms of service. Under Authorized domains, add the top-level domain from step A. Under Developer contact information, add the contact email. Save.

**D. Clean up and verify.** On the Clients tab, delete any OAuth client ADX doesn't use. Back on Branding, click **Verify Branding** and wait, usually a few minutes. Don't edit the branding while it runs; click Cancel first if something must change.

- *Ready to publish*: click **Publish branding** within seven days, or the result expires and shows "Need to re-verify".
- *Issues listed*: fix each one, then Verify Branding again, or request a manual review.

**E. Confirm.** The Branding page says the project is brand verified. Sign out of ADX and back in with the Google account that connects the ad accounts, then check on the Connections screen that the Google Ads accounts still show.

## After verification

Apply for Basic access at once: Cloud Console → Google Ads API Overview → Upgrade access level → confirm Basic is the next level → Apply for access.

## If you see

| Message | What it means | Fix |
|---|---|---|
| Home page issue | The page is private, unrelated to the app, or has no privacy policy link | Make it public, name the app, link the policy |
| Privacy policy issue | A different domain from the home page, or no section on Google data | Host it on the same domain; say how Google Ads data is used and stored |
| Domain not verified | The authorized domain is missing from Search Console | Finish step A with an account that owns the domain |
| Name or logo mismatch | The consent screen's name or logo differs from the home page | Use the same name and logo in both places |
| Need to re-verify | Seven days passed after Ready to publish | Verify Branding again, then publish |
| Email from Trust & Safety | The review is paused for more information | Reply from the developer contact email |

## Sources

- Brand verification, Google Ads API documentation
- Submit for brand verification, Google Identity documentation
- Access levels and permissible use, Google Ads API documentation
