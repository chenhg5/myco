# Shopping comparison

Shopping comparison connects an agent-side Maishou API adapter to a shared
page. The agent directly queries product prices and purchase links; people open
the page to compare conditions and choose. Create it from **Apps → Shopping
comparison** and tell an agent the product, exact specification and delivery
region. **New comparison** sends an API-query brief to the publishing agent;
clients without sending can copy the request. General web search is not used
as a fallback for missing product API access.

## Configure your own Maishou access

The selected reference Skill uses OpenID and account authorization, not a
documented generic API-key field. Obtain your own approved access from Maishou:
OpenID and relevant marketplace bindings, plus your own invitation code and
token if required for purchase links. If the supplier provides a different
developer API-key contract, its documentation must be applied before use.

On the agent's computer, configure MAISHOU_OPENID, optional
MAISHOU_INVITE_CODE and MAISHOU_TOKEN, or a private JSON file passed as
--config-file. Never put credentials in page records, a shared chat, the web
bundle or repository. The app includes no borrowed identity or referral code,
auto-login, subscription purchase or ordering capability. Provider account
terms, quota and fees still apply and have not been established by this client.

The template guide documents how to download and run agent-api.mjs with Node
20+, query selected platforms, paginate, obtain purchase links, and publish
through the existing myco CLI with a version check. A source-type enum does
not guarantee that the caller has access to every listed platform. Partial
failures, expired authorization and missing links are reported explicitly.

**Acceptance status:** request/error contracts and local HTTP handling are
tested. The live anonymous endpoint rejects queries as not logged in or not
authorized. Authenticated product and purchase-link acceptance is pending the
owner's own provider credentials; installing this template alone does not
make a live account available.

## Compare and choose

Each comparisons record is a research snapshot with stable offer IDs, sources,
query times and conditions. API product-level prices may be minimum-SKU or
account-conditional prices. The adapter leaves unconfirmed specifications,
shipping, eligibility and stock unknown; original prices and cashback are not
substituted for payable prices. Where only a platform source entry is available,
the button says **View source**; copy a provided purchase code when available.

The displayed lowest comparable price includes known shipping and requires an
exact specification, the same currency, public eligibility, available stock
and evidence no older than 24 hours. It is the lowest of the queried comparable
offers, not a guaranteed market-wide minimum. Checkout remains authoritative.

Filter, search, save candidates, write shared buying notes and export JSON.
Human decisions live separately from research and survive agent refreshes.
Notes and saved candidates are visible to authorized page members. Return a
page URL with #compare?id=<record-id> for direct navigation on updated clients;
sharing still follows page access. Sample prices are explicitly fictional,
have no buying links and are excluded from ranking. No orders, payments or
background monitoring are included.
