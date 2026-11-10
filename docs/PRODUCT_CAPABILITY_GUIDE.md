# Shared product capability descriptions

The runtime and native terminal interface consume
`internal/productguide/capabilities.json` for connected-account onboarding,
empty source selection, everyday help, pilot setup and assistant self-knowledge.
The Go binary embeds it; Bun bundles the same JSON for the TUI. No runtime file or network
lookup is needed. The guide contains no user configuration or credentials and
cannot grant tools or permissions.

The shared descriptions distinguish supported Google tools from an individual
run's selected account, scopes, access checks and exact action approval. They
cover thread/recipient/attachment sends, original-file selection through
`list_input_files`, and native permission upgrades. Onboarding points to the
build's actual Connections options, supporting
both distributor-configured sign-in and a user-owned Desktop OAuth client. It no
longer promises Calendar. Source selection and the pilot no longer offer it as
an available connected source. Local plans can still track user-provided dates.

An independent inventory test compares every built-in Google operation with the
guide. Adding or removing an operation requires reconciling the description.
This is a drift check, not authorization: the actual registry, selected sources,
OAuth grant, policy and action broker still decide what a run can do. Real
account access must be checked rather than inferred from the product guide.

Assistant guidance also removes an obsolete statement that a remote companion
does not exist. It describes the experimental companion as a separately
configured online host, without claiming deployment, pairing, availability,
access to offline laptop files or automatic migration of desktop tasks.

Scope: this centralizes the connected-account and execution-availability claims
identified in competitive finding 15. Other navigation, voice, file-format and
release descriptions still need their own production-backed consistency checks.
This change does not restore Calendar or complete always-on deployment.

The 3 November public-research addition updates shared help with task-only web
review and the distinction between configured search and direct page reading.
The empty connected-account field now says no accounts are selected; it does not
claim all available resources are local. Actual web scope is still checked by
`web_access` and its foreground review, never by these descriptions.

Rollback: revert the guide and its consumers together. No user data or settings
migration is introduced; previously retained conversations remain unchanged.
