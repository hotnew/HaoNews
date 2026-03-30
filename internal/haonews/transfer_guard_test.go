package haonews

import (
	"strings"
	"testing"
)

func TestValidateBundleTransferPayloadLengthRejectsAbsoluteLimit(t *testing.T) {
	t.Parallel()

	payloadLen := uint32(absoluteMaxBundleTransferPayload + 1)
	err := validateBundleTransferPayloadLength(payloadLen, int64(payloadLen)+1)
	if err == nil || !strings.Contains(err.Error(), "absolute limit") {
		t.Fatalf("validateBundleTransferPayloadLength error = %v, want absolute limit", err)
	}
}
