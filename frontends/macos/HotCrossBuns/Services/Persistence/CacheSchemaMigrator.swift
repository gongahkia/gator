import Foundation

// Routes a decoded CachedAppState through schema migrations when the
// on-disk version is older than the current build's schema. Each case
// encapsulates the field-level transforms needed to bring the structure
// forward one version.
//
// This lives at the CachedAppState decoder seam so migrations run once
// at load time and the rest of the app sees only the latest shape.
enum CacheSchemaMigrator {
    // Migrations only need to run when the wire version is older than
    // `target`. No-op migrations return the target version so the caller
    // can stamp-forward the in-memory model.
    //
    // There are no non-trivial migrations yet. v0 (pre-versioning) and
    // v1 share the same field set; v2 adds account catalog fields derived
    // from the legacy active account; v3 adds account workspaces and stamps
    // the active root payload into the active workspace during decode.
    //
    //     case (0, let tgt):
    //         // v0 → v1: transform whatever changed
    //         return migrate(from: 1, to: tgt)
    //
    // Using the field-level decodeIfPresent defaults in CachedAppState
    // covers cases where a field was added with a sensible zero-value, so
    // migrations only need to appear here when a field's *meaning* or
    // type changed.
    static func migrateInPlace(from: Int, to target: Int) -> Int {
        target
    }
}
