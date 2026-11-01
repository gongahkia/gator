# Conversation index consistency

The optional SQLite conversation index is a disposable read cache. JSON conversation, display and revision records remain authoritative.

During chat UX regression testing, `TestSQLiteCatalogConcurrentReadersAndOrganization` intermittently returned an empty incomplete page while a conversation was being renamed. The index detected different metadata signatures before and after reading the canonical/display pair and omitted the row. A shared lease on the existing `.metadata.lock` now protects changed metadata reads against Gator's rename/head writers. Unchanged records still use the signature fast path. This adds no new store and changes no Work execution or admission semantics.

Lock acquisition uses nonblocking attempts under the existing bounded query context. Cancellation releases the read without preventing the next query from returning the renamed chat. Symlink, bounded-file, schema and post-read signature checks remain in place.

Verification: `go test -race ./internal/worksession -run 'TestSQLiteCatalogConcurrentReadersAndOrganization|TestSQLiteCatalogMetadataWaitHonorsCancellation' -count=20` passed (16.148 s). The first test exercises concurrent indexed readers and real organization writes; the second holds the writer lock and checks deadline handling and subsequent recovery. This evidence does not claim safety against malicious same-user filesystem changes.
