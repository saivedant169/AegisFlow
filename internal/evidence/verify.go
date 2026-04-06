package evidence

// VerifyResult contains the result of evidence chain verification.
type VerifyResult struct {
	Valid        bool   `json:"valid"`
	TotalRecords int    `json:"total_records"`
	ErrorAtIndex int    `json:"error_at_index,omitempty"`
	Message      string `json:"message"`
}

// Verify checks the integrity of a chain of records.
func Verify(records []Record) VerifyResult {
	if len(records) == 0 {
		return VerifyResult{Valid: true, TotalRecords: 0, Message: "empty chain is valid"}
	}

	for i, rec := range records {
		// Recompute hash
		expected := computeRecordHash(rec)
		if expected != rec.Hash {
			return VerifyResult{
				Valid:        false,
				TotalRecords: len(records),
				ErrorAtIndex: i,
				Message:      "hash mismatch at record " + rec.Envelope.ID,
			}
		}

		// Check chain link (records after the first)
		if i > 0 {
			if rec.PreviousHash != records[i-1].Hash {
				return VerifyResult{
					Valid:        false,
					TotalRecords: len(records),
					ErrorAtIndex: i,
					Message:      "chain break at record " + rec.Envelope.ID,
				}
			}
		}
	}

	return VerifyResult{
		Valid:        true,
		TotalRecords: len(records),
		Message:      "evidence chain integrity verified",
	}
}
