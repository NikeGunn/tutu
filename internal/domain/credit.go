package domain

import "time"

// ─── Credit Types (prepared for Phase 1) ────────────────────────────────────
// These live in domain because they represent core business rules.
// Phase 0 defines the types; Phase 1 implements the ledger.

// EntryType represents the accounting side of a ledger entry.
type EntryType string

const (
	EntryDebit  EntryType = "DEBIT"
	EntryCredit EntryType = "CREDIT"
)

// TransactionType represents the business reason for a credit operation.
type TransactionType string

const (
	TxEarn    TransactionType = "EARN"
	TxSpend   TransactionType = "SPEND"
	TxBond    TransactionType = "BOND"
	TxRelease TransactionType = "RELEASE"
	TxPenalty TransactionType = "PENALTY"
	TxBonus   TransactionType = "BONUS"
)

// LedgerEntry is a single row in the double-entry credit ledger.
type LedgerEntry struct {
	ID          int64           `json:"id"`
	Timestamp   time.Time       `json:"timestamp"`
	Type        TransactionType `json:"type"`
	EntryType   EntryType       `json:"entry_type"`
	Account     string          `json:"account"`
	Amount      int64           `json:"amount"`
	TaskID      string          `json:"task_id,omitempty"`
	Description string          `json:"description,omitempty"`
	Balance     int64           `json:"balance"`
}
