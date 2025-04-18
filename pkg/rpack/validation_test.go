package rpack

import "testing"

func TestCueValidator(t *testing.T) {
	const schema = `#Schema: { field!: string & "right-choice"  }`
	v, err := NewCueValidator([]byte(schema), "#Schema")
	if err != nil {
		t.Fatalf("Failed setting up validation: %s", err)
	}
	{ // Valid
		err = v.Validate(struct {
			Field string `json:"field"`
		}{
			Field: "right-choice",
		})
		if err != nil {
			t.Fatalf("Validation failed: %s", err)
		}
	}

	{ // Invalid
		err = v.Validate(struct {
			Field string `json:"field"`
		}{
			Field: "wrong-choice",
		})
		if err == nil {
			t.Fatalf("Validation should have failed for `wrong-choice`")
		}
	}
}

func TestEmptyValidator(t *testing.T) {
	v := &EmptyValidator{}
	err := v.Validate(nil)
	if err != nil {
		t.Fatalf("Validation failed")
	}
}
