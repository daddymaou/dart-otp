package phone

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"+2348012345678", "+2348012345678", false},
		{"2348012345678", "", true},
		{"08012345678", "", true},
		{"+234 801 234 5678", "+2348012345678", false},
		{"", "", true},
		{"abc", "", true},
	}
	for _, tc := range cases {
		got, err := Normalize(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Normalize(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Normalize(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
