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
	"github.com/lib/pq"
	"github.com/wurt83ow/timetracker/internal/models"
	"github.com/wurt83ow/timetracker/internal/storage"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Log interface {
	Info(string, ...zapcore.Field)
}

type BDKeeper struct {
	conn               *sql.DB
	log                Log
	userUpdateInterval func() string
}

func NewBDKeeper(dsn func() string, log Log, userUpdateInterval func() string) *BDKeeper {
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
		conn:               conn,
		log:                log,
		userUpdateInterval: userUpdateInterval,
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

func (bd *BDKeeper) UpdateUsersInfo(users []models.ExtUserData) error {
	if len(users) == 0 {
		return nil
	}

	// Подготовка массивов для пакетного обновления
	passportSeries := make([]int, len(users))
	passportNumbers := make([]int, len(users))
	surnames := make([]string, len(users))
	names := make([]string, len(users))
	addresses := make([]string, len(users))

	for i, user := range users {
		passportSeries[i] = user.PassportSerie
		passportNumbers[i] = user.PassportNumber
		surnames[i] = user.Surname
		names[i] = user.Name
		addresses[i] = user.Address
	}

	query := `
		UPDATE People SET
			surname = CASE
				WHEN passportSerie = any($1) AND passportNumber = any($2) THEN unnest($3)
				ELSE surname
			END,
			name = CASE
				WHEN passportSerie = any($1) AND passportNumber = any($2) THEN unnest($4)
				ELSE name
			END,
			address = CASE
				WHEN passportSerie = any($1) AND passportNumber = any($2) THEN unnest($5)
				ELSE address
			END
		WHERE (passportSerie, passportNumber) IN (SELECT unnest($1), unnest($2))
	`

	_, err := bd.conn.Exec(
		query,
		pq.Array(passportSeries),
		pq.Array(passportNumbers),
		pq.Array(surnames),
		pq.Array(names),
		pq.Array(addresses),
	)
	if err != nil {
		bd.log.Info("Ошибка при пакетном обновлении данных пользователей в базе данных: ", zap.Error(err))
		return err
	}

	bd.log.Info("Данные пользователей успешно обновлены")
	return nil
}

func (bd *BDKeeper) UpdateUser(user models.People) error {
	query := `
		UPDATE People SET
			surname = $4,
			name = $5,
			patronymic = $6,
			address = $7,
			default_end_time = $8,
			timezone = $9,
			username = $10,
			password_hash = $11,
			last_checked_at = $12
		WHERE passportSerie = $2 AND passportNumber = $3
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
		bd.log.Info("Ошибка при обновлении данных пользователя в базе данных: ", zap.Error(err))
		return err
	}

	bd.log.Info("Данные пользователя успешно обновлены: ", zap.Int("passportSerie", user.PassportSerie), zap.Int("passportNumber", user.PassportNumber))
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

func (kp *BDKeeper) DeleteUser(passportSerie, passportNumber int) error {
	ctx := context.Background()

	query := `
		DELETE FROM People
		WHERE passportSerie = $1 AND passportNumber = $2
	`

	_, err := kp.conn.ExecContext(ctx, query, passportSerie, passportNumber)
	if err != nil {
		kp.log.Info("error deleting user from database: ", zap.Error(err))
		return err
	}

	kp.log.Info("User deleted successfully", zap.Int("passportSerie", passportSerie), zap.Int("passportNumber", passportNumber))
	return nil
}

func (kp *BDKeeper) GetNonUpdateUsers() ([]models.ExtUserData, error) {
	ctx := context.Background()

	// Read the interval from environment variable
	updateInterval, err := time.ParseDuration(kp.userUpdateInterval())
	if err != nil {
		return nil, fmt.Errorf("failed to parse USER_UPDATE_INTERVAL: %w", err)
	}

	// get current time and calculate the threshold time
	currentTime := time.Now().UTC()
	thresholdTime := currentTime.Add(-updateInterval)

	// Prepare the SQL query
	sql := `
	SELECT
		passportSerie,
		passportNumber,
		surname,
		name,
		address
	FROM
		public.people
	WHERE
		last_checked_at <= $1
	LIMIT 100`

	rows, err := kp.conn.QueryContext(ctx, sql, thresholdTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get non-updated users: %w", err)
	}
	defer rows.Close()

	users := make([]models.ExtUserData, 0)

	for rows.Next() {
		var user models.ExtUserData

		err := rows.Scan(
			&user.PassportSerie,
			&user.PassportNumber,
			&user.Surname,
			&user.Name,
			&user.Address,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to process rows: %w", err)
	}

	return users, nil
}

func (bd *BDKeeper) SaveTask(task models.Task) error {
	query := `
		INSERT INTO tasks (
			name, description, created_at
		) VALUES (
			$1, $2, $3
		)
	`

	_, err := bd.conn.Exec(
		query,
		task.Name,
		task.Description,
		task.CreatedAt,
	)
	if err != nil {
		bd.log.Info("error saving task to database: ", zap.Error(err))
		return err
	}

	bd.log.Info("Task saved successfully: ", zap.String("name", task.Name))
	return nil
}

func (kp *BDKeeper) LoadTasks() (storage.StorageTasks, error) {
	ctx := context.Background()

	sql := `
	SELECT
		id,
		name,
		description,
		created_at
	FROM
		tasks`

	rows, err := kp.conn.QueryContext(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to load tasks: %w", err)
	}

	defer rows.Close()

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to load tasks: %w", err)
	}

	data := make(storage.StorageTasks)

	for rows.Next() {
		var t models.Task

		err := rows.Scan(
			&t.ID,
			&t.Name,
			&t.Description,
			&t.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load tasks: %w", err)
		}

		data[t.ID] = t
	}

	return data, nil
}

func (kp *BDKeeper) DeleteTask(id int) error {
	ctx := context.Background()

	query := `
		DELETE FROM tasks
		WHERE id = $1
	`

	_, err := kp.conn.ExecContext(ctx, query, id)
	if err != nil {
		kp.log.Info("error deleting task from database: ", zap.Error(err))
		return err
	}

	kp.log.Info("Task deleted successfully", zap.Int("id", id))
	return nil
}

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
