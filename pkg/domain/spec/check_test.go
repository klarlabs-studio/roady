package spec

import "testing"

func baseSpec(check *Check) *ProductSpec {
	return &ProductSpec{ID: "s", Title: "S", Features: []Feature{{
		ID: "f", Title: "F",
		Requirements: []Requirement{{ID: "r", Title: "R", Check: check}},
	}}}
}

func TestValidateRejectsAmbiguousCheck(t *testing.T) {
	if errs := baseSpec(&Check{Run: "a", Manual: "b"}).Validate(); len(errs) != 1 {
		t.Fatalf("expected one error for a check with both kinds, got %v", errs)
	}
	if errs := baseSpec(&Check{}).Validate(); len(errs) != 1 {
		t.Fatalf("expected one error for an empty check, got %v", errs)
	}
	if errs := baseSpec(&Check{Run: "go test ./..."}).Validate(); len(errs) != 0 {
		t.Fatalf("a run check is valid, got %v", errs)
	}
}

// Loosening a check must read as a spec change, or drift cannot see an agent
// weakening its own definition of done. Specs without checks keep their hash,
// so locks written before checks existed stay valid.
func TestHashCoversCheck(t *testing.T) {
	without := baseSpec(nil).Hash()
	strict := baseSpec(&Check{Run: "go test ./... -run TestInvoice"}).Hash()
	loose := baseSpec(&Check{Run: "true"}).Hash()
	if strict == loose {
		t.Error("changing a check did not change the spec hash")
	}
	if without == strict {
		t.Error("adding a check did not change the spec hash")
	}
	legacy := (&ProductSpec{ID: "s", Title: "S", Features: []Feature{{ID: "f", Title: "F", Requirements: []Requirement{{ID: "r", Title: "R"}}}}}).Hash()
	if legacy != without {
		t.Error("a spec without checks must keep its previous hash")
	}
}
