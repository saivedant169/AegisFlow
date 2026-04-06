package main

import (
	"strings"
	"testing"
)

func TestFormatVerifyResult_Pass(t *testing.T) {
	r := VerifyResponse{
		Valid:        true,
		TotalRecords: 42,
		Message:      "evidence chain integrity verified",
	}
	out := formatVerifyResult(r)
	if !strings.Contains(out, "PASS") {
		t.Fatal("expected PASS in output")
	}
	if !strings.Contains(out, "Total entries: 42") {
		t.Fatal("expected total entries in output")
	}
	if strings.Contains(out, "Error at index") {
		t.Fatal("should not show error index for valid result")
	}
}

func TestFormatVerifyResult_Fail(t *testing.T) {
	r := VerifyResponse{
		Valid:        false,
		TotalRecords: 10,
		ErrorAtIndex: 5,
		Message:      "hash mismatch at record abc123",
	}
	out := formatVerifyResult(r)
	if !strings.Contains(out, "FAIL") {
		t.Fatal("expected FAIL in output")
	}
	if !strings.Contains(out, "Error at index: 5") {
		t.Fatal("expected error index in output")
	}
	if !strings.Contains(out, "hash mismatch") {
		t.Fatal("expected message in output")
	}
}

func TestFormatVerifyResult_EmptyChain(t *testing.T) {
	r := VerifyResponse{
		Valid:        true,
		TotalRecords: 0,
		Message:      "empty chain is valid",
	}
	out := formatVerifyResult(r)
	if !strings.Contains(out, "PASS") {
		t.Fatal("expected PASS for empty chain")
	}
	if !strings.Contains(out, "Total entries: 0") {
		t.Fatal("expected zero entries")
	}
}
