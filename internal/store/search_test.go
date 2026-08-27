package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFTSCaseSearch(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	case1, err := st.CreateCase(ctx, "g1", "ban", "mod1", "t1", "raiding the vault", 0, nil, "discord", "")
	if err != nil {
		t.Fatalf("create case 1: %v", err)
	}

	if _, err := st.CreateCase(ctx, "g1", "warn", "mod1", "t2", "spam links", 0, nil, "discord", ""); err != nil {
		t.Fatalf("create case 2: %v", err)
	}
	if err := st.AddCaseNote(ctx, case1.ID, "mod1", "he was raiding with alt accounts"); err != nil {
		t.Fatalf("add note: %v", err)
	}

	hits, err := st.SearchCases(ctx, "vault", 25)
	if err != nil {
		t.Fatalf("search vault: %v", err)
	}
	if len(hits) != 1 || hits[0].Src != "case" || hits[0].TargetID != "t1" {
		t.Fatalf("want one case hit on t1, got %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "vault") {
		t.Fatalf("snippet missing term: %q", hits[0].Snippet)
	}

	hits, err = st.SearchCases(ctx, "alt accounts", 25)
	if err != nil {
		t.Fatalf("search note: %v", err)
	}
	if len(hits) != 1 || hits[0].Src != "note" || hits[0].CaseID != case1.ID {
		t.Fatalf("want one note hit on case %d, got %+v", case1.ID, hits)
	}

	hits, err = st.SearchCases(ctx, "raids", 25)
	if err != nil {
		t.Fatalf("stemmed search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want case+note hits via stemming, got %+v", hits)
	}

	if _, err := st.SearchCases(ctx, `vault" OR 1=1 --`, 25); err != nil {
		t.Fatalf("garbage query should not error: %v", err)
	}

	if err := st.UpdateCaseReason(ctx, "g1", 2, "crypto scam pump"); err != nil {
		t.Fatalf("update reason: %v", err)
	}
	hits, _ = st.SearchCases(ctx, "scam", 25)
	if len(hits) != 1 || hits[0].TargetID != "t2" {
		t.Fatalf("want updated reason hit on t2, got %+v", hits)
	}
}

func TestCSRFEpochBump(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	sess := &Session{ID: "sess1", DiscordUserID: "u1", Username: "op",
		ExpiresAt: time.Now().Add(24 * time.Hour)}
	if err := st.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := st.GetSession(ctx, "sess1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Epoch != 0 {
		t.Fatalf("fresh session epoch = %d, want 0", got.Epoch)
	}

	if err := st.BumpAllSessionEpochs(ctx); err != nil {
		t.Fatalf("bump: %v", err)
	}
	got, _ = st.GetSession(ctx, "sess1")
	if got.Epoch != 1 {
		t.Fatalf("epoch after bump = %d, want 1", got.Epoch)
	}
}

