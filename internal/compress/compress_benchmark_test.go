package compress

import (
	"context"
	"testing"
)

func BenchmarkCompressFallback(b *testing.B) {
	stage := New(nil)
	stage.DisableCompress = true
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := stage.Run(context.Background(), rawEnvelope()); err != nil {
			b.Fatalf("compress fallback: %v", err)
		}
	}
}

func BenchmarkValidateDigestHappyPath(b *testing.B) {
	raw := rawEnvelope().Raw
	digest := validDigest()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := validateDigest(raw, digest); err != nil {
			b.Fatalf("validate digest: %v", err)
		}
	}
}

func BenchmarkValidateDigestDroppedItem(b *testing.B) {
	raw := rawEnvelope().Raw
	digest := digestWithInvalidItem(validDigest().Items[0])
	digest.Items[len(digest.Items)-1].Path = "ghost.go"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := validateDigest(raw, digest); err != nil {
			b.Fatalf("validate digest: %v", err)
		}
	}
}
