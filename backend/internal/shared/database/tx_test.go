package database

import (
	"context"
	"errors"
	"testing"
)

func TestBeginTxNilDB(t *testing.T) {
	if BeginTx(context.Background(), nil) != nil {
		t.Fatal("nil db must yield nil tx")
	}
}

func TestFinishNilTx(t *testing.T) {
	if err := Finish(nil, nil); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	boom := errors.New("boom")
	if err := Finish(nil, boom); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}
