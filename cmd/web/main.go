package main

import (
	"database/sql"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type config struct {
	addr string
	env  string
	db   struct {
		dsn string
	}
	uploadsDir string
}

type application struct {
	config    config
	logger    *slog.Logger
	db        *sql.DB
	templates map[string]*template.Template
}

func main() {
	var cfg config

	flag.StringVar(&cfg.addr, "addr", ":4000", "HTTP listen address")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|production)")
	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("PIRATECA_DB_DSN"), "PostgreSQL DSN")
	flag.StringVar(&cfg.uploadsDir, "uploads-dir", "./uploads", "Directory with covers, pdfs, epubs and torrents")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	templates, err := newTemplateCache()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	app := &application{
		config:    cfg,
		logger:    logger,
		templates: templates,
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
		logger.Info("database connection pool established")
	} else {
		logger.Warn("no db-dsn provided, running without database")
	}

	srv := &http.Server{
		Addr:         cfg.addr,
		Handler:      app.routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  time.Minute,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	logger.Info("starting server", "addr", cfg.addr, "env", cfg.env)
	err = srv.ListenAndServe()
	logger.Error(err.Error())
	os.Exit(1)
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
