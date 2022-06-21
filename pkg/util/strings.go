package util

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
