package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"go.uber.org/fx"
	"jungle-gaming/cmd/internal/httpapi"
	"jungle-gaming/internal/database"
)

func main() {
	fx.New(
		fx.Provide(
			database.NewPostgres,
			httpapi.NewHandler,
			httpapi.NewOIDCAuth,
		),
		fx.Invoke(func(
			lifecycle fx.Lifecycle,
			db *sql.DB,
			handler http.Handler,
			auth func(http.Handler) http.Handler,
		) {
			server := &http.Server{
				Addr:    ":8080",
				Handler: auth(handler),
			}

			lifecycle.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					go func() {
						if err := server.ListenAndServe(); err != nil &&
							err != http.ErrServerClosed {
							log.Printf("Erro no servidor HTTP: %v", err)
						}
					}()
					return nil
				},
				OnStop: func(ctx context.Context) error {
					err := server.Shutdown(ctx)
					closeErr := db.Close()
					if err != nil {
						return err
					}
					return closeErr
				},
			})
		}),
	).Run()
}