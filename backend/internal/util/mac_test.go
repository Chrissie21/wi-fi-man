package util

import "testing"

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "aa:bb:cc:dd:ee:ff", want: "AA:BB:CC:DD:EE:FF"},
		{in: "aa-bb-cc-dd-ee-ff", want: "AA:BB:CC:DD:EE:FF"},
		{in: "AABB.CCDD.EEFF", want: "AA:BB:CC:DD:EE:FF"},
		{in: "AA:BB:CC:DD:EE", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range tests {
		got, err := NormalizeMAC(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("expected error for input %q", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalize %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalize %q: got %q want %q", tc.in, got, tc.want)
		}
	}
}
