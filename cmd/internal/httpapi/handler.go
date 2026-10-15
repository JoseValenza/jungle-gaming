package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"jungle-gaming/internal/domain"
)

type moneyRequest struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type createWalletRequest struct {
	PlayerID       string       `json:"playerId"`
	InitialBalance moneyRequest `json:"initialBalance"`
}

type betRequest struct {
	ProviderID                     string       `json:"providerId"`
	ExternalTransactionID          string       `json:"externalTransactionId"`
	PlayerID                       string       `json:"playerId"`
	WalletID                       string       `json:"walletId"`
	RoundID                        string       `json:"roundId"`
	GameID                         string       `json:"gameId"`
	Kind                           string       `json:"kind"`
	Money                          moneyRequest `json:"money"`
	ReferenceExternalTransactionID string       `json:"referenceExternalTransactionId,omitempty"`
}

type moneyResponse struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type walletResponse struct {
	ID       string        `json:"id"`
	PlayerID string        `json:"playerId"`
	Balance  moneyResponse `json:"balance"`
	Version  int64         `json:"version"`
}

type wagerResponse struct {
	TransactionID string        `json:"transactionId"`
	Status        string        `json:"status"`
	Balance       moneyResponse `json:"balance"`
}

var moneyPattern = regexp.MustCompile(`^[0-9]+\.[0-9]{2}$`)

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewHandler(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":   "ready",
			"database": "connected",
		})
	})

	mux.HandleFunc("POST /wallets", func(w http.ResponseWriter, r *http.Request) {
		createWallet(w, r, db)
	})

	mux.HandleFunc("GET /wallets/{walletId}", func(w http.ResponseWriter, r *http.Request) {
		getWallet(w, r, db)
	})

	mux.HandleFunc("POST /wagering/transactions", func(w http.ResponseWriter, r *http.Request) {
		createBet(w, r, db)
	})

	return mux
}

func createWallet(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var request createWalletRequest
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "JSON inválido",
		})
		return
	}

	request.PlayerID = strings.TrimSpace(request.PlayerID)
	request.InitialBalance.Currency = strings.ToUpper(
		strings.TrimSpace(request.InitialBalance.Currency),
	)

	if request.PlayerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "playerId é obrigatório",
		})
		return
	}

	if !validCurrency(request.InitialBalance.Currency) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "currency deve conter três letras",
		})
		return
	}

	if !moneyPattern.MatchString(request.InitialBalance.Amount) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "amount deve usar duas casas decimais, por exemplo 100.00",
		})
		return
	}

	parsedAmount, err := domain.ParseMoney(request.InitialBalance.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "valor inicial inválido",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "não foi possível iniciar a operação no banco",
		})
		return
	}
	defer tx.Rollback()

	var walletID string
	var version int64

	err = tx.QueryRowContext(ctx, `
		INSERT INTO wallets (player_id, currency, balance_minor)
		VALUES ($1, $2, $3)
		RETURNING id::text, version
	`,
		request.PlayerID,
		request.InitialBalance.Currency,
		int64(parsedAmount),
	).Scan(&walletID, &version)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "já existe uma carteira para esse jogador e moeda",
			})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível criar a carteira",
		})
		return
	}

	if parsedAmount > 0 {
		if err := createOpeningRecords(
			ctx,
			tx,
			walletID,
			request.PlayerID,
			request.InitialBalance.Currency,
			int64(parsedAmount),
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "não foi possível registrar a abertura da carteira",
			})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível confirmar a criação da carteira",
		})
		return
	}

	writeJSON(w, http.StatusCreated, walletResponse{
		ID:       walletID,
		PlayerID: request.PlayerID,
		Balance: moneyResponse{
			Amount:   formatMoney(int64(parsedAmount)),
			Currency: request.InitialBalance.Currency,
		},
		Version: version,
	})
}

func createBet(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "o cabeçalho Idempotency-Key é obrigatório",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var request betRequest
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "JSON inválido",
		})
		return
	}

	request.ProviderID = strings.TrimSpace(request.ProviderID)
	request.ExternalTransactionID = strings.TrimSpace(request.ExternalTransactionID)
	request.PlayerID = strings.TrimSpace(request.PlayerID)
	request.WalletID = strings.TrimSpace(request.WalletID)
	request.RoundID = strings.TrimSpace(request.RoundID)
	request.GameID = strings.TrimSpace(request.GameID)
	request.Kind = strings.ToUpper(strings.TrimSpace(request.Kind))
	request.Money.Currency = strings.ToUpper(strings.TrimSpace(request.Money.Currency))

	if request.ProviderID == "" ||
		request.ExternalTransactionID == "" ||
		request.PlayerID == "" ||
		request.WalletID == "" ||
		request.RoundID == "" ||
		request.GameID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "providerId, externalTransactionId, playerId, walletId, roundId e gameId são obrigatórios",
		})
		return
	}

	if request.Kind != "BET" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "este endpoint aceita somente transações do tipo BET",
		})
		return
	}

	if !validCurrency(request.Money.Currency) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "currency deve conter três letras",
		})
		return
	}

	if !moneyPattern.MatchString(request.Money.Amount) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "amount deve usar duas casas decimais, por exemplo 10.00",
		})
		return
	}

	parsedAmount, err := domain.ParseMoney(request.Money.Amount)
	if err != nil || parsedAmount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "o valor da aposta deve ser maior que zero e válido",
		})
		return
	}

	canonicalPayload, err := json.Marshal(request)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível processar a requisição",
		})
		return
	}

	hashBytes := sha256.Sum256(canonicalPayload)
	payloadHash := hex.EncodeToString(hashBytes[:])

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	// Repetições da mesma chave devem devolver a resposta original.
	if replayIdempotency(w, ctx, db, request.ProviderID, key, payloadHash) {
		return
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "não foi possível iniciar a operação no banco",
		})
		return
	}
	defer tx.Rollback()

	// Bloqueia a carteira durante a operação para impedir saldo negativo
	// em apostas concorrentes.
	var playerID, currency string
	var balanceMinor, version int64

	err = tx.QueryRowContext(ctx, `
		SELECT player_id, currency, balance_minor, version
		FROM wallets
		WHERE id = $1
		FOR UPDATE
	`, request.WalletID).Scan(
		&playerID,
		&currency,
		&balanceMinor,
		&version,
	)

	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "carteira não encontrada",
		})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível consultar a carteira",
		})
		return
	}

	// A mesma chave pode ter sido concluída enquanto aguardávamos o bloqueio.
	if replayIdempotency(w, ctx, tx, request.ProviderID, key, payloadHash) {
		return
	}

	if playerID != request.PlayerID || currency != request.Money.Currency {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "a carteira não corresponde ao jogador ou à moeda informados",
		})
		return
	}

	// Um ID externo não pode gerar um segundo movimento usando outra chave.
	var existingID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text
		FROM wager_transactions
		WHERE provider_id = $1 AND external_transaction_id = $2
	`, request.ProviderID, request.ExternalTransactionID).Scan(&existingID)

	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "externalTransactionId já foi utilizado por esse provider",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível verificar a transação existente",
		})
		return
	}

	status := "PROCESSED"
	httpStatus := http.StatusCreated
	newBalance := balanceMinor
	newVersion := version

	if balanceMinor < int64(parsedAmount) {
		status = "REJECTED"
		httpStatus = http.StatusConflict
	} else {
		newBalance = balanceMinor - int64(parsedAmount)
		newVersion++
	}

	var transactionID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO wager_transactions (
			provider_id,
			external_transaction_id,
			player_id,
			wallet_id,
			round_id,
			game_id,
			kind,
			amount_minor,
			currency,
			status,
			idempotency_key,
			payload_hash,
			response_balance_minor
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'BET', $7, $8, $9, $10, $11, $12)
		RETURNING id::text
	`,
		request.ProviderID,
		request.ExternalTransactionID,
		request.PlayerID,
		request.WalletID,
		request.RoundID,
		request.GameID,
		int64(parsedAmount),
		request.Money.Currency,
		status,
		key,
		payloadHash,
		newBalance,
	).Scan(&transactionID)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "transação ou chave de idempotência já utilizada",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível registrar a aposta",
		})
		return
	}

	if status == "PROCESSED" {
		_, err = tx.ExecContext(ctx, `
			UPDATE wallets
			SET balance_minor = $1,
			    version = $2,
			    updated_at = NOW()
			WHERE id = $3
		`, newBalance, newVersion, request.WalletID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "não foi possível atualizar o saldo",
			})
			return
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO ledger_entries (
				wallet_id,
				transaction_id,
				direction,
				amount_minor,
				balance_before_minor,
				balance_after_minor
			)
			VALUES ($1, $2, 'DEBIT', $3, $4, $5)
		`, request.WalletID, transactionID, int64(parsedAmount), balanceMinor, newBalance)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "não foi possível registrar o movimento financeiro",
			})
			return
		}
	}

	response := wagerResponse{
		TransactionID: transactionID,
		Status:        status,
		Balance: moneyResponse{
			Amount:   formatMoney(newBalance),
			Currency: currency,
		},
	}
	responseBody, err := json.Marshal(response)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível preparar a resposta",
		})
		return
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO idempotency_records (
			provider_id,
			idempotency_key,
			payload_hash,
			transaction_id,
			response_status,
			response_body
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`,
		request.ProviderID,
		key,
		payloadHash,
		transactionID,
		httpStatus,
		string(responseBody),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível registrar a idempotência",
		})
		return
	}

	eventType := "WagerTransactionRejected"
	if status == "PROCESSED" {
		eventType = "WagerTransactionProcessed"
	}

	eventPayload, _ := json.Marshal(map[string]any{
		"transactionId": transactionID,
		"walletId":      request.WalletID,
		"playerId":      request.PlayerID,
		"kind":          "BET",
		"amount":        request.Money.Amount,
		"currency":      currency,
		"status":        status,
		"balance":       formatMoney(newBalance),
	})

	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			event_type,
			aggregate_id,
			correlation_id,
			payload
		)
		VALUES ($1, $2, $3, $4::jsonb)
	`, eventType, request.WalletID, transactionID, string(eventPayload))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível registrar o evento",
		})
		return
	}

	if status == "PROCESSED" {
		balanceEvent, _ := json.Marshal(map[string]any{
			"walletId": request.WalletID,
			"balance":  formatMoney(newBalance),
			"currency": currency,
			"version":  newVersion,
		})

		_, err = tx.ExecContext(ctx, `
			INSERT INTO outbox_events (
				event_type,
				aggregate_id,
				correlation_id,
				payload
			)
			VALUES ('WalletBalanceChanged', $1, $2, $3::jsonb)
		`, request.WalletID, transactionID, string(balanceEvent))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "não foi possível registrar a alteração do saldo",
			})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível confirmar a aposta",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_, _ = w.Write(responseBody)
}

func replayIdempotency(
	w http.ResponseWriter,
	ctx context.Context,
	q queryRower,
	providerID string,
	key string,
	payloadHash string,
) bool {
	var storedHash string
	var storedStatus int
	var responseBody []byte

	err := q.QueryRowContext(ctx, `
		SELECT payload_hash, response_status, response_body
		FROM idempotency_records
		WHERE provider_id = $1 AND idempotency_key = $2
	`, providerID, key).Scan(&storedHash, &storedStatus, &responseBody)

	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível consultar a idempotência",
		})
		return true
	}

	if storedHash != payloadHash {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "Idempotency-Key já utilizada com outro conteúdo",
		})
		return true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(storedStatus)
	_, _ = w.Write(responseBody)
	return true
}

func createOpeningRecords(
	ctx context.Context,
	tx *sql.Tx,
	walletID string,
	playerID string,
	currency string,
	amount int64,
) error {
	internalID := "opening:" + walletID

	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%s|%d",
		playerID,
		currency,
		amount,
	)))

	var transactionID string
	err := tx.QueryRowContext(ctx, `
		INSERT INTO wager_transactions (
			provider_id,
			external_transaction_id,
			player_id,
			wallet_id,
			round_id,
			game_id,
			kind,
			amount_minor,
			currency,
			status,
			idempotency_key,
			payload_hash,
			response_balance_minor
		)
		VALUES (
			'internal', $1, $2, $3,
			'internal', 'internal', 'OPENING',
			$4, $5, 'PROCESSED', $1, $6, $4
		)
		RETURNING id::text
	`,
		internalID,
		playerID,
		walletID,
		amount,
		currency,
		hex.EncodeToString(hash[:]),
	).Scan(&transactionID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			wallet_id,
			transaction_id,
			direction,
			amount_minor,
			balance_before_minor,
			balance_after_minor
		)
		VALUES ($1, $2, 'CREDIT', $3, 0, $3)
	`, walletID, transactionID, amount)
	if err != nil {
		return err
	}

	events := []struct {
		eventType string
		payload   map[string]any
	}{
		{
			eventType: "WagerTransactionProcessed",
			payload: map[string]any{
				"transactionId": transactionID,
				"walletId":      walletID,
				"kind":          "OPENING",
				"amount":        formatMoney(amount),
				"currency":      currency,
				"status":        "PROCESSED",
			},
		},
		{
			eventType: "WalletBalanceChanged",
			payload: map[string]any{
				"walletId": walletID,
				"balance":  formatMoney(amount),
				"currency": currency,
				"version":  1,
			},
		},
	}

	for _, event := range events {
		payload, err := json.Marshal(event.payload)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO outbox_events (
				event_type,
				aggregate_id,
				correlation_id,
				payload
			)
			VALUES ($1, $2, $3, $4::jsonb)
		`,
			event.eventType,
			walletID,
			transactionID,
			string(payload),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func getWallet(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var response walletResponse
	var balanceMinor int64

	err := db.QueryRowContext(ctx, `
		SELECT id::text, player_id, currency, balance_minor, version
		FROM wallets
		WHERE id = $1
	`, r.PathValue("walletId")).Scan(
		&response.ID,
		&response.PlayerID,
		&response.Balance.Currency,
		&balanceMinor,
		&response.Version,
	)

	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "carteira não encontrada",
		})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "não foi possível consultar a carteira",
		})
		return
	}

	response.Balance.Amount = formatMoney(balanceMinor)
	writeJSON(w, http.StatusOK, response)
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("envie apenas um objeto JSON")
	}

	return nil
}

func validCurrency(currency string) bool {
	return len(currency) == 3 &&
		currency[0] >= 'A' && currency[0] <= 'Z' &&
		currency[1] >= 'A' && currency[1] <= 'Z' &&
		currency[2] >= 'A' && currency[2] <= 'Z'
}

func formatMoney(amount int64) string {
	return fmt.Sprintf("%d.%02d", amount/100, amount%100)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
