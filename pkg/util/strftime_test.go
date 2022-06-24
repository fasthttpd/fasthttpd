package util

import (
	"testing"
	"time"
)

func TestStrftime(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)
	t2 := time.Date(2022, 1, 2, 0, 0, 0, 0, tokyo)

	tests := []struct {
		t      time.Time
		format string
		want   string
	}{
		{
			t:      t1,
			format: "%Y-%m-%d %H:%M:%S",
			want:   "2006-01-02 15:04:05",
		}, {
			t:      t1,
			format: "%% %a %A %b %B %c %C %",
			want:   "% Mon Monday Jan January Mon Jan  2 15:04:05 2006 20 ",
		}, {
			t:      t1,
			format: "%d %D %e %E %f %F",
			want:   "02 01/02/06  2 E f 2006-01-02",
		}, {
			t:      t1,
			format: "%g %G %h %H %I %j %k",
			want:   "06 2006 Jan 15 03 002 15",
		}, {
			t:      t1,
			format: "%l %m %M %n %p %P",
			want:   " 3 01 04 \n pm PM",
		}, {
			t:      t1,
			format: "%r %R %s %S %t %T %u %U",
			want:   "03:04:05 pm 15:04 1136214245 05 \t 15:04:05 1 01",
		}, {
			t:      t1,
			format: "%V %w %W %x %X %y %Y %z %Z",
			want:   "01 1 01 01/02/06 15:04:05 06 2006 +0000 UTC",
		}, {
			t:      t2,
			format: "%u %U %w %W %z %Z",
			want:   "7 01 0 52 +0900 JST",
		},
	}
	for i, test := range tests {
		s := NewStrftime(test.format)
		got := s.Format(test.t)
		if got != test.want {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}
