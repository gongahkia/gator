# Optional local speech dependencies

Gator’s local voice integration invokes an independently installed whisper.cpp executable and offers explicit downloads of public OpenAI Whisper weights converted to GGML by the upstream whisper.cpp project. No executable, model weights, or runtime source is bundled in this repository by this feature.

- OpenAI Whisper: https://github.com/openai/whisper; MIT; license retained from tag `v20250625` in `LICENSE-whisper.txt`.
- whisper.cpp: https://github.com/ggml-org/whisper.cpp; MIT; license retained from tag `v1.9.5` in `LICENSE-whisper.cpp.txt`. This version was used for native verification; future upstream behavior is not automatically certified.
- Model distribution: https://huggingface.co/ggerganov/whisper.cpp, immutable revision `5359861c739e955e79d9a303bcbc70fb988958b1`. Only `ggml-tiny.bin`, `ggml-base.bin`, and `ggml-small.bin` are offered, with exact size/SHA-256 verification in `internal/speech/local_models.go`.

The integration and tests are newly written Gator code. The third-party license notices apply to their respective components and do not relicense Gator. Do not package these dependencies or replace pinned weights without reviewing their provenance and applicable notices.
