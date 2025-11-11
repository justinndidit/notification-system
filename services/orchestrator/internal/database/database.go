package database

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	pgxzero "github.com/jackc/pgx-zerolog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/config"
	customLogger "github.com/justinndidit/notificationSystem/orchestrator/internal/logger"
	"github.com/rs/zerolog"
)

const DatabasePingTimeout = 10

type Database struct {
	Pool   *pgxpool.Pool
	logger *zerolog.Logger
}

type multiTracer struct {
	tracers []any
}

// TraceQueryStart implements pgx tracer interface
func (mt *multiTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	for _, tracer := range mt.tracers {
		if t, ok := tracer.(interface {
			TraceQueryStart(context.Context, *pgx.Conn, pgx.TraceQueryStartData) context.Context
		}); ok {
			ctx = t.TraceQueryStart(ctx, conn, data)
		}
	}
	return ctx
}

// TraceQueryEnd implements pgx tracer interface
func (mt *multiTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	for _, tracer := range mt.tracers {
		if t, ok := tracer.(interface {
			TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData)
		}); ok {
			t.TraceQueryEnd(ctx, conn, data)
		}
	}
}

func New(cfg config.DatabaseConfig, logger *zerolog.Logger) (*Database, error) {
	hostPort := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	// URL-encode the password
	encodedPassword := url.QueryEscape(cfg.Password)
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		cfg.User,
		encodedPassword,
		hostPort,
		cfg.Name,
		cfg.SSLMode,
	)

	pgxPoolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pgx pool config: %w", err)
	}

	pgxLogger := customLogger.NewPgxLogger()
	localTracer := &tracelog.TraceLog{
		Logger:   pgxzero.NewLogger(pgxLogger),
		LogLevel: tracelog.LogLevel(1),
	}
	pgxPoolConfig.ConnConfig.Tracer = &multiTracer{
		tracers: []any{pgxPoolConfig.ConnConfig.Tracer, localTracer},
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), pgxPoolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgx pool: %w", err)
	}

	database := &Database{
		Pool:   pool,
		logger: logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), DatabasePingTimeout*time.Second)
	defer cancel()
	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info().Msg("connected to the database")

	return database, nil
}

func (db *Database) Close() {
	db.logger.Info().Msg("closing database connection pool")
	if db.Pool == nil {
		return
	}
	db.Pool.Close()
}
