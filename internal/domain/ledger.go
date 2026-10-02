package domain

import "time"

// Direction tells whether a ledger entry adds or removes money.
type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

// LedgerEntry is an immutable record of one balance change.
// There are no setters: corrections require new entries.
type LedgerEntry struct {
	id            string
	walletID      string
	transactionID string
	direction     Direction
	money         Money
	balanceBefore Money
	balanceAfter  Money
	createdAt     time.Time
}

// NewLedgerEntry validates that balanceAfter = balanceBefore ± money.
func NewLedgerEntry(id, walletID, transactionID string, direction Direction,
	money, balanceBefore, balanceAfter Money, createdAt time.Time) (LedgerEntry, error) {

	if id == "" || walletID == "" || transactionID == "" {
		return LedgerEntry{}, ErrInvalidLedgerEntry
	}
	if direction != DirectionDebit && direction != DirectionCredit {
		return LedgerEntry{}, ErrInvalidLedgerEntry
	}
	if !money.IsPositive() || balanceBefore.IsNegative() || balanceAfter.IsNegative() {
		return LedgerEntry{}, ErrInvalidLedgerEntry
	}

	var expected Money
	var err error
	if direction == DirectionCredit {
		expected, err = balanceBefore.Add(money)
	} else {
		expected, err = balanceBefore.Sub(money)
	}
	if err != nil {
		return LedgerEntry{}, err
	}
	cmp, err := expected.Compare(balanceAfter)
	if err != nil {
		return LedgerEntry{}, err
	}
	if cmp != 0 {
		return LedgerEntry{}, ErrInvalidLedgerEntry
	}

	return LedgerEntry{
		id: id, walletID: walletID, transactionID: transactionID,
		direction: direction, money: money,
		balanceBefore: balanceBefore, balanceAfter: balanceAfter,
		createdAt: createdAt,
	}, nil
}

func (e LedgerEntry) ID() string            { return e.id }
func (e LedgerEntry) WalletID() string      { return e.walletID }
func (e LedgerEntry) TransactionID() string { return e.transactionID }
func (e LedgerEntry) Direction() Direction  { return e.direction }
func (e LedgerEntry) Money() Money          { return e.money }
func (e LedgerEntry) BalanceBefore() Money  { return e.balanceBefore }
func (e LedgerEntry) BalanceAfter() Money   { return e.balanceAfter }
func (e LedgerEntry) CreatedAt() time.Time  { return e.createdAt }
