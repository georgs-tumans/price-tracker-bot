package config

import (
	"slices"
	"testing"
)

func TestParseChatIDs(t *testing.T) {
	tests := []struct {
		input   string
		want    []int64
		wantErr bool
	}{
		{input: "", want: nil},
		{input: "12345678", want: []int64{12345678}},
		{input: " 123, -100987654321 ,, 456 ", want: []int64{123, -100987654321, 456}},
		{input: "123,abc", wantErr: true},
	}

	for _, tt := range tests {
		got, err := parseChatIDs(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseChatIDs(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}

		if !slices.Equal(got, tt.want) {
			t.Errorf("parseChatIDs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsChatAllowed(t *testing.T) {
	open := &Configuration{}
	if !open.IsChatAllowed(42) {
		t.Error("expected every chat to be allowed when no allowed chats are configured")
	}

	restricted := &Configuration{AllowedChatIDs: []int64{1, 2}}
	if !restricted.IsChatAllowed(2) {
		t.Error("expected chat 2 to be allowed")
	}

	if restricted.IsChatAllowed(3) {
		t.Error("expected chat 3 to be rejected")
	}
}
