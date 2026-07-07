# Architecture

Start here, then follow the focused docs:

- [Related work and positioning](../RELATED_WORK.md)
- [Design and stage model](../DESIGN.md)
- [Schemas and envelope contract](../SCHEMAS.md)
- [Model API contracts](../MODEL_APIS.md)
- [MCP context sidecar](MCP.md)
- [Patch format](../PATCH_FORMAT.md)
- [Benchmarks](../BENCHMARKS.md)
- [Testing strategy](../TESTING.md)
- [Benchmark results](RESULTS.md)

The short version: `paw` is `gather | compress | plan | edit | verify`. Deterministic stages gather context, apply patches, and verify. Model stages are split into a drone that compresses raw context and a brain that plans and edits from validated digests.
