package util

import (
	"flag"
	"strings"
)

// StringList is an array of strings that implements flag.Value.
type StringList []string

var _ flag.Value = (*StringList)(nil)

func (ss *StringList) Set(value string) error {
	*ss = append(*ss, value)
	return nil
}

func (ss *StringList) String() string {
	return strings.Join(*ss, ",")
}

// StringSet is an array of strings that can be duplicated via Append.
type StringSet []string

// Append appends values to the StringSet without duplication.
func (ss StringSet) Append(values ...string) StringSet {
valuesLoop:
	for _, v := range values {
		for _, s := range ss {
			if s == v {
				continue valuesLoop
			}
		}
		ss = append(ss, v)
	}
	return ss
}
