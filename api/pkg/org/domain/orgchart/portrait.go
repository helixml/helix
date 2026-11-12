package orgchart

const PortraitCount = 6

// PreferredPortraitIndex preserves the avatar chosen by the original UI hash.
func PreferredPortraitIndex(id NodeID) int {
	hash := uint32(2166136261)
	for _, b := range []byte(id) {
		hash ^= uint32(b)
		hash *= 16777619
	}
	return int(hash % PortraitCount)
}

// AllocatePortraitIndex uses every portrait once before reusing the
// least-used portrait, probing from the bot's stable preferred slot.
func AllocatePortraitIndex(id NodeID, assigned []int) int {
	counts := [PortraitCount]int{}
	for _, index := range assigned {
		if index >= 0 && index < PortraitCount {
			counts[index]++
		}
	}
	minimum := counts[0]
	for _, count := range counts[1:] {
		if count < minimum {
			minimum = count
		}
	}
	preferred := PreferredPortraitIndex(id)
	for offset := 0; offset < PortraitCount; offset++ {
		index := (preferred + offset) % PortraitCount
		if counts[index] == minimum {
			return index
		}
	}
	return preferred
}
