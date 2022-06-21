package util

import (
	"reflect"
	"testing"
)

func Test_StringSet(t *testing.T) {
	tests := []struct {
		values [][]string
		want   StringSet
	}{
		{
			values: [][]string{{"A", "B", "C"}, {"C"}, {"C", "B"}, {"D"}},
			want:   StringSet{"A", "B", "C", "D"},
		},
	}
	for i, test := range tests {
		var got StringSet
		for _, vs := range test.values {
			got = got.Append(vs...)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] unexpected %#v; want %#v", i, got, test.want)
		}
	}
}
