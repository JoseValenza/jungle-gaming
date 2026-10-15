
package domain

import "errors"

var (
	ErrInvalidWallet = errors.New("carteira inválida")
	ErrInsufficientFunds = errors.New("saldo insuficiente")
	ErrCurrencyMismatch = errors.New("moedas incompatíveis")
)

// Wallet representa a carteira financeira de um jogador.
type Wallet struct {
	ID       string
	PlayerID string
	Provider string
	Currency string
	Balance  Money
	Version  int64
}

// NewWallet cria uma carteira com saldo inicial zero.
func NewWallet(id, playerID, provider, currency string) (*Wallet, error) {
	if id == "" || playerID == "" || provider == "" || currency == "" {
		return nil, ErrInvalidWallet
	}

	return &Wallet{
		ID:       id,
		PlayerID: playerID,
		Provider: provider,
		Currency: currency,
		Balance:  Money(0),
		Version:  1,
	}, nil
}

// Debit retira dinheiro da carteira, sem permitir saldo negativo.
func (w *Wallet) Debit(amount Money) error {
	if amount <= 0 {
		return ErrInvalidMoney
	}

	if w.Balance < amount {
		return ErrInsufficientFunds
	}

	newBalance, err := w.Balance.Sub(amount)
	if err != nil {
		return err
	}

	w.Balance = newBalance
	w.Version++
	return nil
}

// Credit adiciona dinheiro à carteira.
func (w *Wallet) Credit(amount Money) error {
	if amount <= 0 {
		return ErrInvalidMoney
	}

	newBalance, err := w.Balance.Add(amount)
	if err != nil {
		return err
	}

	w.Balance = newBalance
	w.Version++
	return nil
}
