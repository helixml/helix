package server

// knownEnforcementGaps lists "<METHOD path> | <probe>" pairs that are not
// refused today. Every entry is a defect to fix, not an exception: the
// enforcement test fails when one starts refusing, so the list only shrinks.
var knownEnforcementGaps = map[string]string{}
