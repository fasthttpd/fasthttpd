package util

import (
	"reflect"
	"testing"
)

func TestStringList(t *testing.T) {
	got := &StringList{}
	tests := []struct {
		value      string
		want       StringList
		wantString string
	}{
		{
			value:      "1",
			want:       StringList{"1"},
			wantString: "1",
		}, {
			value:      "2",
			want:       StringList{"1", "2"},
			wantString: "1,2",
		},
	}
	for i, test := range tests {
		err := got.Set(test.value)
		if err != nil {
			t.Fatalf("tests[%d] error %v", i, err)
		}
		if !reflect.DeepEqual(*got, test.want) {
			t.Errorf("tests[%d] unexpected %#v; want %#v", i, got, test.want)
		}
		if got.String() != test.wantString {
			t.Errorf("tests[%d] unexpected %q; want %q", i, got.String(), test.wantString)
		}
	}
}

func TestStringSet(t *testing.T) {
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
