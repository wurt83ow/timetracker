package bdkeeper

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // registers a migrate driver.
	_ "github.com/jackc/pgx/v5/stdlib"                   // registers a pgx driver.
	"github.com/wurt83ow/timetracker/internal/models"
	"github.com/wurt83ow/timetracker/internal/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Log interface {
	Info(string, ...zapcore.Field)
}

type BDKeeper struct {
	conn *sql.DB
	log  Log
}

func NewBDKeeper(dsn func() string, log Log) *BDKeeper {
	addr := dsn()
	if addr == "" {
		log.Info("database dsn is empty")

		return nil
	}

	conn, err := sql.Open("pgx", dsn())
	if err != nil {
		log.Info("Unable to connection to database: ", zap.Error(err))

		return nil
	}

	driver, err := postgres.WithInstance(conn, new(postgres.Config))
	if err != nil {
		log.Info("error getting driver: ", zap.Error(err))

		return nil
	}

	dir, err := os.Getwd()
	if err != nil {
		log.Info("error getting current directory: ", zap.Error(err))
	}

	// fix error test path
	mp := dir + "/migrations"

	var path string
	if _, err := os.Stat(mp); err != nil {
		path = "../../"
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%smigrations", path),
		"postgres",
		driver)
	if err != nil {
		log.Info("Error creating migration instance: ", zap.Error(err))
		return nil
	}

	err = m.Up()
	if err != nil {
		log.Info("Error while performing migration: ", zap.Error(err))
		return nil
	}

	log.Info("Connected!")

	return &BDKeeper{
		conn: conn,
		log:  log,
	}
}

func (bd *BDKeeper) SaveUser(key string, user models.People) error {
	query := `
		INSERT INTO People (
			id, passportSerie, passportNumber, surname, name, patronymic, address,
			default_end_time, timezone, username, password_hash, last_checked_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
		ON CONFLICT (passportSerie, passportNumber) DO NOTHING
	`

	_, err := bd.conn.Exec(
		query,
		user.UUID,
		user.PassportSerie,
		user.PassportNumber,
		user.Surname,
		user.Name,
		user.Patronymic,
		user.Address,
		user.DefaultEndTime,
		user.Timezone,
		user.Email,
		user.Hash,
		user.LastCheckedAt,
	)
	if err != nil {
		bd.log.Info("error saving user to database: ", zap.Error(err))
		return err
	}

	bd.log.Info("User saved successfully: ", zap.String("key", key))
	return nil
}

func (kp *BDKeeper) LoadUsers() (storage.StorageUsers, error) {
	ctx := context.Background()

	// get users from bd
	sql := `
	SELECT
		id,
		passportSerie,
		passportNumber,
		surname,
		name,
		patronymic,
		address,
		default_end_time,
		timezone,
		username,
		password_hash,
		last_checked_at
	FROM
		People`

	rows, err := kp.conn.QueryContext(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to load users: %w", err)
	}

	defer rows.Close()

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to load users: %w", err)
	}

	data := make(storage.StorageUsers)

	for rows.Next() {
		var m models.People

		err := rows.Scan(
			&m.UUID,
			&m.PassportSerie,
			&m.PassportNumber,
			&m.Surname,
			&m.Name,
			&m.Patronymic,
			&m.Address,
			&m.DefaultEndTime,
			&m.Timezone,
			&m.Email,
			&m.Hash,
			&m.LastCheckedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load users: %w", err)
		}

		key := fmt.Sprintf("%d %d", m.PassportSerie, m.PassportNumber)
		data[key] = m
	}

	return data, nil
}

// func (kp *BDKeeper) LoadUsers() (storage.StorageUsers, error) {
// 	ctx := context.Background()

// 	// get users from bd
// 	sql := `
// 	SELECT
// 		user_id,
// 		name,
// 		email,
// 		hash
// 	FROM
// 		users`

// 	rows, err := kp.conn.QueryContext(ctx, sql)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to load users: %w", err)
// 	}

// 	if err = rows.Err(); err != nil {
// 		return nil, fmt.Errorf("failed to load users: %w", err)
// 	}

// 	defer rows.Close()

// 	data := make(storage.StorageUsers)

// 	for rows.Next() {
// 		var m models.People

// 		err := rows.Scan(&m.UUID, &m.Name, &m.Email, &m.Hash)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to load users: %w", err)
// 		}

// 		data[m.Email] = m
// 	}

// 	return data, nil
// }

// func (kp *BDKeeper) SaveUser(key string, data models.People) (models.People, error) {
// 	ctx := context.Background()

// 	var id string

// 	if data.UUID == "" {
// 		neuuid := uuid.New()
// 		id = neuuid.String()
// 	} else {
// 		id = data.UUID
// 	}

// 	sql := `
// 	INSERT INTO users (user_id, email, hash, name)
// 		VALUES ($1, $2, $3, $4)
// 	RETURNING
// 		user_id`
// 	_, err := kp.conn.ExecContext(ctx, sql,
// 		id, data.Email, data.Hash, data.Name)

// 	var (
// 		cond string
// 		hash []byte
// 	)

// 	if data.Hash != nil {
// 		cond = "AND u.hash = $2"
// 		hash = data.Hash
// 	}

// 	sql = `
// 	SELECT
// 		u.user_id,
// 		u.email,
// 		u.hash,
// 		u.name
// 	FROM
// 		users u
// 	WHERE
// 		u.email = $1 %s`
// 	sql = fmt.Sprintf(sql, cond)
// 	row := kp.conn.QueryRowContext(ctx, sql, data.Email, hash)

// 	// read the values from the database record into the corresponding fields of the structure
// 	var m models.People

// 	nerr := row.Scan(&m.UUID, &m.Email, &m.Hash, &m.Name)
// 	if nerr != nil {
// 		return data, fmt.Errorf("failed to save user: %w", nerr)
// 	}

// 	if err != nil {
// 		var e *pgconn.PgError
// 		if errors.As(err, &e) && e.Code == pgerrcode.UniqueViolation {
// 			kp.log.Info("unique field violation on column: ", zap.Error(err))

// 			return m, storage.ErrConflict
// 		}

// 		return m, fmt.Errorf("failed to save user: %w", err)
// 	}

// 	return m, nil
// }

func (kp *BDKeeper) Ping() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
	defer cancel()

	if err := kp.conn.PingContext(ctx); err != nil {
		return false
	}

	return true
}

func (kp *BDKeeper) Close() bool {
	kp.conn.Close()

	return true
}
