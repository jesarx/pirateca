package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jesarx/pirateca/internal/store"
	_ "github.com/lib/pq"
)

type config struct {
	addr string
	env  string
	db   struct {
		dsn string
	}
	uploadsDir    string
	sessionSecret string
}

type application struct {
	config        config
	logger        *slog.Logger
	db            *sql.DB
	store         *store.Store
	templates     map[string]*template.Template
	sessionSecret []byte
	visits        *visitCounter
}

func main() {
	var cfg config

	flag.StringVar(&cfg.addr, "addr", ":4000", "HTTP listen address")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|production)")
	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("PIRATECA_DB_DSN"), "PostgreSQL DSN")
	flag.StringVar(&cfg.uploadsDir, "uploads-dir", "./uploads", "Directory with covers, pdfs, epubs and torrents")
	flag.StringVar(&cfg.sessionSecret, "session-secret", os.Getenv("PIRATECA_SESSION_SECRET"), "Secret for signing session cookies (min 32 chars)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sessionSecret, err := decodeSessionSecret(cfg.sessionSecret)
	if err != nil {
		// Sin secret configurado se genera uno efímero: la app funciona,
		// pero las sesiones de admin mueren con cada reinicio.
		sessionSecret = make([]byte, 32)
		if _, err := rand.Read(sessionSecret); err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
		logger.Warn("no valid session secret configured, using ephemeral secret (admin sessions reset on restart)")
	}

	templates, err := newTemplateCache()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	app := &application{
		config:        cfg,
		logger:        logger,
		templates:     templates,
		sessionSecret: sessionSecret,
		visits:        newVisitCounter(),
	}

	// El DSN es opcional durante el desarrollo del esqueleto; las páginas
	// que consultan la base de datos fallarán con 500 si no se configura.
	if cfg.db.dsn != "" {
		db, err := openDB(cfg.db.dsn)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
		defer db.Close()
		app.db = db
		app.store = store.New(db)
		logger.Info("database connection pool established")
	} else {
		logger.Warn("no db-dsn provided, running without database")
	}

	srv := &http.Server{
		Addr:    cfg.addr,
		Handler: app.routes(),
		// ReadTimeout cubre la lectura del cuerpo completo: la subida de un
		// PDF desde el dashboard es una lectura, así que debe ser generoso
		// (un PDF de decenas de MB por una conexión de subida lenta tarda
		// más de unos pocos segundos). ReadHeaderTimeout mantiene la
		// protección anti-slowloris sobre los encabezados de toda petición.
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute, // descargas y subidas de PDFs grandes
		IdleTimeout:       time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	// Apagado ordenado: en SIGINT/SIGTERM se drenan las conexiones y se
	// hace un último flush del contador de visitas.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flusherDone := make(chan struct{})
	go func() {
		app.startVisitFlusher(ctx)
		close(flusherDone)
	}()

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	logger.Info("starting server", "addr", cfg.addr, "env", cfg.env)
	err = srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		logger.Error(err.Error())
		os.Exit(1)
	}
	stop()
	<-flusherDone
	logger.Info("server stopped")
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(15 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
