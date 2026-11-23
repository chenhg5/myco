# Shopping comparison

Shopping comparison is a shared research page: an agent checks the available
sources and writes offers; people open the page to compare conditions and pick
what suits them. Create it from **Apps → Shopping comparison**, then tell an
agent which product, exact specification, quantity, region and budget to check.
The page's **New comparison** button sends that brief to its publishing agent;
clients without message sending let you copy the request instead.

Agents use the normal page records API, not a new shopping action. Each
`comparisons` record is a complete research snapshot, with stable offer IDs,
source URLs, observation times, stock, currency, shipping and eligibility.
The template's `agent.md` documents every field and a versioned CLI workflow.
Return a page URL with `#compare?id=<record-id>` to open the requested snapshot
on clients supporting app links. Access follows existing page sharing.

The displayed lowest comparable price includes known shipping and requires a
verified matching specification, the same currency, public eligibility,
available stock and evidence no older than 24 hours. Conditional member or
subsidy prices, uncertain specifications, expired offers and unknown shipping
remain visible with explanations. Cashback is displayed separately and never
deducted as an immediate discount. The lowest observed comparable price is not
a guaranteed market-wide minimum; the marketplace checkout price wins.

Filter by platform, search offer text, save candidates, and keep shared purchase
notes. Human decisions live separately from research, so an agent refresh does
not overwrite them. Export downloads the selected comparison and decisions as
JSON. Notes and saved candidates are visible to authorized page members.

No provider credentials, default affiliate identity, marketplace login,
automated scraping, ordering, payments or background monitoring are included.
Agents use tools or provider accounts they are authorized to use. If the agent
cannot obtain useful evidence, it records a blocked result and explains what
is missing. Prices in **Try sample** are explicitly fictional and excluded
from ranking; a blank page starts without offers.
