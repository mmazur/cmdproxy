package argmatch

// Match tests whether args matches the positional segment pattern.
// Each segment consumes args according to its quantifier.
// All args must be consumed for a match.
func Match(segments []Segment, args []string) bool {
	return matchAt(segments, 0, args, 0)
}

func matchAt(segments []Segment, segIdx int, args []string, argIdx int) bool {
	// All segments consumed — match only if all args also consumed.
	if segIdx == len(segments) {
		return argIdx == len(args)
	}

	seg := &segments[segIdx]
	remaining := len(args) - argIdx

	max := seg.Quantifier.Max
	if max == -1 || max > remaining {
		max = remaining
	}

	// Validate every consumed argument, including those below the minimum count.
	for count := 0; count <= max; count++ {
		// Check that all count args match the segment's glob.
		if count > 0 && !seg.Match(args[argIdx+count-1]) {
			// The arg at position argIdx+count-1 doesn't match.
			// Since we're increasing count, this arg will always be
			// in the set, so no point trying higher counts.
			break
		}

		// All args in [argIdx, argIdx+count) match — try the rest.
		if count >= seg.Quantifier.Min && matchAt(segments, segIdx+1, args, argIdx+count) {
			return true
		}
	}

	return false
}
