package jsonerror

import (
	"encoding/json"
	"testing"
)

func TestLimitExceeded(t *testing.T) {
	e := LimitExceeded("too fast", 5000)
	jsonBytes, err := json.Marshal(&e)
	if err != nil {
		t.Fatalf("TestLimitExceeded: Failed to marshal LimitExceeded error. %s", err.Error())
	}
	want := `{"errcode":"M_LIMIT_EXCEEDED","error":"too fast","retry_after_ms":5000}`
	if string(jsonBytes) != want {
		t.Errorf("TestLimitExceeded: want %s, got %s", want, string(jsonBytes))
	}
}

func TestForbidden(t *testing.T) {
	e := Forbidden("you shall not pass")
	jsonBytes, err := json.Marshal(&e)
	if err != nil {
		t.Fatalf("TestForbidden: Failed to marshal Forbidden error. %s", err.Error())
	}
	want := `{"errcode":"M_FORBIDDEN","error":"you shall not pass"}`
	if string(jsonBytes) != want {
		t.Errorf("TestForbidden: want %s, got %s", want, string(jsonBytes))
	}
}
