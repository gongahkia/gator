I've learned a lot by looking at other people's code so
maybe someone can learn something from looking at mine.

= = = = = = = =

Gator is a tiny terminal work agent.

If you want, it will research selected sources, reconcile
tables, write documents, and delegate bounded coding work
inside one persistent conversation. Sources are captured
before execution. Deliverables, code candidates,
verification, and external-action proposals remain
reviewable before transfer. Reads do not grant mutation.

For more info, see: [docs/WORK.md](docs/WORK.md)

= = = = = = =

This project pins Go 1.25.13. Git and a strict process
sandbox are needed for Code work: Bubblewrap on Linux or
Seatbelt on macOS.

```sh
GOTOOLCHAIN=go1.25.13 make build
./bin/gator
```

Configure a provider inside the TUI with `/model`. Native
credentials stay in the private credential store. See
[local models](docs/LOCAL_MODELS.md),
[custom providers](docs/CUSTOM_PROVIDERS.md),
[Google Work](docs/GOOGLE_WORK.md), and
[Work](docs/WORK.md).
