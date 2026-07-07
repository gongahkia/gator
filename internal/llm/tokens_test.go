package llm

import (
	"errors"
	"sync"
	"testing"

	"github.com/tiktoken-go/tokenizer"
)

func TestEstimate_FallbackWhenCodecUnavailable(t *testing.T) {
	originalLoad := estimateCodecLoad
	t.Cleanup(func() {
		estimateCodecLoad = originalLoad
		estimateCodec = nil
		estimateCodecErr = nil
		estimateCodecOnce = sync.Once{}
	})

	estimateCodecLoad = func() (tokenizer.Codec, error) {
		return nil, errors.New("codec unavailable")
	}
	estimateCodec = nil
	estimateCodecErr = nil
	estimateCodecOnce = sync.Once{}

	if got := Estimate("hello world"); got <= 0 {
		t.Fatalf("Estimate() = %d, want positive fallback", got)
	}
}
