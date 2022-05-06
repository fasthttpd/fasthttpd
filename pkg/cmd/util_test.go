package cmd

import "testing"

func Test_getNetwork(t *testing.T) {
	tests := []struct {
		listen string
		want   string
	}{
		{
			listen: ":8800",
			want:   "tcp4",
		}, {
			listen: "[::1]:8800",
			want:   "tcp6",
		},
	}
	for i, test := range tests {
		got := getNetwork(test.listen)
		if got != test.want {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}
