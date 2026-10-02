package domain

import "testing"

func TestCanonicalJSONIsSortedAndExcludesKey(t *testing.T) {
	b, err := validReq(t, KindBet, "10.00").CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"externalTransactionId":"tx-1","gameId":"g1","kind":"BET",` +
		`"money":{"amount":"10.00","currency":"BRL"},"playerId":"p1",` +
		`"providerId":"provider-a","roundId":"r1","walletId":"w1"}`
	if string(b) != want {
		t.Fatalf("unexpected canonical JSON:\n got: %s\nwant: %s", b, want)
	}
}

func TestHashIgnoresIdempotencyKey(t *testing.T) {
	a := validReq(t, KindBet, "10.00")
	b := validReq(t, KindBet, "10.00")
	b.IdempotencyKey = "another-key"
	ha, _ := a.RequestHash()
	hb, _ := b.RequestHash()
	if ha != hb {
		t.Fatal("hash must not depend on the idempotency key")
	}
}

func TestHashChangesWithBusinessFields(t *testing.T) {
	base := validReq(t, KindBet, "10.00")
	hb, _ := base.RequestHash()

	diffAmount := validReq(t, KindBet, "10.01")
	diffKind := validReq(t, KindWin, "10.00")
	diffRound := validReq(t, KindBet, "10.00")
	diffRound.RoundID = "r2"

	for name, r := range map[string]ExternalRequest{
		"amount": diffAmount, "kind": diffKind, "round": diffRound,
	} {
		h, _ := r.RequestHash()
		if h == hb {
			t.Errorf("hash should change when %s changes", name)
		}
	}
}

func TestHashIsStable(t *testing.T) {
	r := validReq(t, KindBet, "10.00")
	h1, _ := r.RequestHash()
	h2, _ := r.RequestHash()
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("hash should be stable and 64 hex chars, got %q / %q", h1, h2)
	}
}

func TestReferenceIsPartOfHash(t *testing.T) {
	a := validReq(t, KindRefund, "10.00")
	b := validReq(t, KindRefund, "10.00")
	b.ReferenceExternalTxID = "tx-other"
	ha, _ := a.RequestHash()
	hb, _ := b.RequestHash()
	if ha == hb {
		t.Fatal("different references must produce different hashes")
	}
}
