package bdkeeper

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // registers a migrate driver.
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
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
	pool               *pgxpool.Pool
	log                Log
	userUpdateInterval func() string
}

func NewBDKeeper(dsn func() string, log Log, userUpdateInterval func() string) *BDKeeper {
	addr := dsn()
	if addr == "" {
		log.Info("database dsn is empty")
		return nil
	}

	config, err := pgxpool.ParseConfig(addr)
	if err != nil {
		log.Info("Unable to parse database DSN: ", zap.Error(err))
		return nil
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Info("Unable to connect to database: ", zap.Error(err))
		return nil
	}

	connConfig, err := pgx.ParseConfig(addr)
	if err != nil {
		log.Info("Unable to parse connection string: %v\n")
	}
	// Register the driver with the name pgx
	sqlDB := stdlib.OpenDB(*connConfig)

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		log.Info("Error getting driver: ", zap.Error(err))
		return nil
	}

	dir, err := os.Getwd()
	if err != nil {
		log.Info("Error getting current directory: ", zap.Error(err))
	}

	// fix error test path
	mp := dir + "/migrations"
	var path string
	if _, err := os.Stat(mp); err != nil {
		path = "../../"
	} else {
		path = dir + "/"
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
	if err != nil && err != migrate.ErrNoChange {
		log.Info("Error while performing migration: ", zap.Error(err))
		return nil
	}

	log.Info("Connected!")

	return &BDKeeper{
		pool:               pool,
		log:                log,
		userUpdateInterval: userUpdateInterval,
	}
}

func (kp *BDKeeper) Close() bool {
	if kp.pool != nil {
		kp.pool.Close()
		kp.log.Info("Database connection pool closed")
		return true
	}
	kp.log.Info("Attempted to close a nil database connection pool")
	return false
}

func (bd *BDKeeper) SaveUser(ctx context.Context, key string, user models.User) error {

	// Convert []byte to string
	passwordHash := hex.EncodeToString(user.Hash)

	query := `
        INSERT INTO Users (
            passportSerie, passportNumber, surname, name, patronymic, address,
            default_end_time, timezone, password_hash
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9
        )
        ON CONFLICT (passportSerie, passportNumber) DO NOTHING
    `

	_, err := bd.pool.Exec(
		ctx,
		query,
		user.PassportSerie,
		user.PassportNumber,
		user.Surname,
		user.Name,
		user.Patronymic,
		user.Address,
		user.DefaultEndTime,
		user.Timezone,
		passwordHash,
	)
	if err != nil {
		bd.log.Info("error saving user to database: ", zap.Error(err))
		return err
	}

	bd.log.Info("User saved successfully: ", zap.String("key", key))
	return nil
}

func (bd *BDKeeper) UpdateUsersInfo(ctx context.Context, users []models.ExtUserData) (err error) {
	if len(users) == 0 {
		return nil
	}

	// Prepare arrays for batch updating
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

	// Read the interval from the environment variable
	updateInterval, err := time.ParseDuration(bd.userUpdateInterval())
	if err != nil {
		return fmt.Errorf("failed to parse USER_UPDATE_INTERVAL: %w", err)
	}

	// Get the current time and calculate the threshold time
	currentTime := time.Now().UTC()
	thresholdTime := currentTime.Add(-updateInterval)

	// Begin the transaction
	tx, err := bd.pool.Begin(ctx)
	if err != nil {
		bd.log.Info("Error while beginning transaction: ", zap.Error(err))
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		} else if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				bd.log.Info("Error during transaction rollback: ", zap.Error(rollbackErr))
			} else {
				bd.log.Info("Transaction rolled back: ", zap.Error(err))
			}
		} else {
			err = tx.Commit(ctx)
			if err != nil {
				bd.log.Info("Error during transaction commit: ", zap.Error(err))
			} else {
				bd.log.Info("Transaction committed successfully")
			}
		}
	}()

	query := `
        UPDATE Users SET
            surname = updated.surname,
            name = updated.name,
            address = updated.address,
            last_checked_at = CURRENT_TIMESTAMP
        FROM (
            SELECT
                unnest($1::int[]) AS passportSerie,
                unnest($2::int[]) AS passportNumber,
                unnest($3::text[]) AS surname,
                unnest($4::text[]) AS name,
                unnest($5::text[]) AS address
        ) AS updated
        WHERE Users.passportSerie = updated.passportSerie
        AND Users.passportNumber = updated.passportNumber
        AND (Users.last_checked_at IS NULL OR Users.last_checked_at <= $6)
    `
	// Execute the query using pgx
	_, err = tx.Exec(
		ctx,
		query,
		passportSeries,
		passportNumbers,
		surnames,
		names,
		addresses,
		thresholdTime,
	)
	if err != nil {
		bd.log.Info("Error during batch updating user data in the database: ", zap.Error(err))
		return err
	}

	bd.log.Info("User data successfully updated")
	return nil
}

func (bd *BDKeeper) UpdateUser(ctx context.Context, user models.User) error {

	query := `
        UPDATE Users SET
            surname = $3,
            name = $4,
            patronymic = $5,
            address = $6,
            default_end_time = $7,
            timezone = $8,            
            password_hash = $9,
            last_checked_at = $10
        WHERE passportSerie = $1 AND passportNumber = $2
    `
	_, err := bd.pool.Exec(
		ctx,
		query,
		user.PassportSerie,
		user.PassportNumber,
		user.Surname,
		user.Name,
		user.Patronymic,
		user.Address,
		user.DefaultEndTime,
		user.Timezone,
		user.Hash,
		user.LastCheckedAt,
	)
	if err != nil {
		bd.log.Info("Error updating user data in the database: ", zap.Error(err))
		return err
	}

	bd.log.Info("User data successfully updated: ", zap.Int("passportSerie", user.PassportSerie), zap.Int("passportNumber", user.PassportNumber))
	return nil
}

func (kp *BDKeeper) LoadUsers(ctx context.Context) (storage.StorageUsers, error) {
	// get users from db
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
        password_hash,
        last_checked_at
    FROM
        Users`

	rows, err := kp.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to load users: %w", err)
	}

	defer rows.Close()

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to load users: %w", err)
	}

	data := make(storage.StorageUsers)

	for rows.Next() {
		var m models.User
		var defaultEndTime pq.NullTime
		var lastCheckedAt pq.NullTime
		var hashHex *string

		err := rows.Scan(
			&m.UUID,
			&m.PassportSerie,
			&m.PassportNumber,
			&m.Surname,
			&m.Name,
			&m.Patronymic,
			&m.Address,
			&defaultEndTime,
			&m.Timezone,
			&hashHex,
			&lastCheckedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load users: %w", err)
		}

		// Декодирование хэша из шестнадцатеричной строки в байты, если значение не NULL
		if hashHex != nil {
			m.Hash, err = hex.DecodeString(*hashHex)
			if err != nil {
				return nil, fmt.Errorf("failed to decode password hash: %w", err)
			}
		}

		// Set DefaultEndTime field only if the value from the database is not NULL
		if defaultEndTime.Valid {
			m.DefaultEndTime = defaultEndTime.Time
		}

		// Set LastCheckedAt field only if the value from the database is not NULL
		if lastCheckedAt.Valid {
			m.LastCheckedAt = lastCheckedAt.Time
		}

		key := fmt.Sprintf("%d %d", m.PassportSerie, m.PassportNumber)
		data[key] = m
	}

	return data, nil
}

func (kp *BDKeeper) DeleteUser(ctx context.Context, passportSerie, passportNumber int) error {

	query := `
        DELETE FROM Users
        WHERE passportSerie = $1 AND passportNumber = $2
    `

	_, err := kp.pool.Exec(ctx, query, passportSerie, passportNumber)
	if err != nil {
		kp.log.Info("error deleting user from database: ", zap.Error(err))
		return err
	}

	kp.log.Info("User deleted successfully", zap.Int("passportSerie", passportSerie), zap.Int("passportNumber", passportNumber))
	return nil
}

func (kp *BDKeeper) GetNonUpdateUsers(ctx context.Context) ([]models.ExtUserData, error) {

	// Read the interval from the environment variable
	updateInterval, err := time.ParseDuration(kp.userUpdateInterval())
	if err != nil {
		return nil, fmt.Errorf("failed to parse USER_UPDATE_INTERVAL: %w", err)
	}

	// Get the current time and calculate the threshold time
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
        public.Users
    WHERE
        last_checked_at IS NULL OR last_checked_at <= $1
    LIMIT 100`

	rows, err := kp.pool.Query(ctx, sql, thresholdTime)
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

func (bd *BDKeeper) SaveTask(ctx context.Context, task models.Task) (int, error) {
	query := `
        INSERT INTO tasks (
            name, description, created_at
        ) VALUES (
            $1, $2, $3
        ) RETURNING id
    `

	var taskID int
	err := bd.pool.QueryRow(
		ctx,
		query,
		task.Name,
		task.Description,
		task.CreatedAt,
	).Scan(&taskID)
	if err != nil {
		bd.log.Info("error saving task to database: ", zap.Error(err))
		return 0, err
	}

	bd.log.Info("Task saved successfully: ", zap.String("name", task.Name), zap.Int("id", taskID))
	return taskID, nil
}

func (kp *BDKeeper) LoadTasks(ctx context.Context) (storage.StorageTasks, error) {

	sql := `
    SELECT
        id,
        name,
        description,
        created_at
    FROM
        tasks`

	rows, err := kp.pool.Query(ctx, sql)
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

func (kp *BDKeeper) DeleteTask(ctx context.Context, id int) error {

	query := `
        DELETE FROM tasks
        WHERE id = $1
    `

	_, err := kp.pool.Exec(ctx, query, id)
	if err != nil {
		kp.log.Info("error deleting task from database: ", zap.Error(err))
		return err
	}

	kp.log.Info("Task deleted successfully", zap.Int("id", id))
	return nil
}

// UpdateTask updates an existing task in the database
func (bd *BDKeeper) UpdateTask(ctx context.Context, task models.Task) error {

	query := `
        UPDATE Tasks SET
            name = $2,
            description = $3              
        WHERE id = $1
    `
	_, err := bd.pool.Exec(
		ctx,
		query,
		task.ID,
		task.Name,
		task.Description,
	)
	if err != nil {
		bd.log.Info("Error updating task in the database: ", zap.Error(err))
		return err
	}

	bd.log.Info("Task successfully updated: ", zap.Int("id", task.ID))
	return nil
}

func (bd *BDKeeper) StartTaskTracking(ctx context.Context, entry models.TimeEntry) error {
	// Convert time taking into account the user's time zone
	location, err := time.LoadLocation(entry.UserTimezone)
	if err != nil {
		bd.log.Info("error loading user timezone: ", zap.Error(err))
		return err
	}
	startTime := time.Now().In(location)

	tx, err := bd.pool.Begin(ctx)
	if err != nil {
		bd.log.Info("Error while beginning transaction: ", zap.Error(err))
		return err
	}

	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				bd.log.Info("Error during transaction rollback: ", zap.Error(rollbackErr))
			} else {
				bd.log.Info("Transaction rolled back: ", zap.Error(err))
			}
		} else {
			err = tx.Commit(ctx)
			if err != nil {
				bd.log.Info("Error during transaction commit: ", zap.Error(err))
			} else {
				bd.log.Info("Transaction committed successfully")
			}
		}
	}()

	// Check for an active entry for the user and task on the specified date
	var existingTaskID int
	query := `
        SELECT id FROM user_tasks
        WHERE user_id = $1 AND task_id = $2 AND event_date = $3 AND end_time IS NULL
    `
	err = tx.QueryRow(ctx, query, entry.UserID, entry.TaskID, entry.EventDate).Scan(&existingTaskID)
	if err != nil && err != sql.ErrNoRows {
		bd.log.Info("error checking existing task in database: ", zap.Error(err))
		return err
	}

	if existingTaskID != 0 {
		return fmt.Errorf("task tracking is already in progress for user %d on task %d for date %s", entry.UserID, entry.TaskID, entry.EventDate)
	}

	// Insert a new entry into the user_tasks table
	insertQuery := `
        INSERT INTO user_tasks (user_id, task_id, event_date, start_time)
        VALUES ($1, $2, $3, $4)
    `
	_, err = tx.Exec(ctx, insertQuery, entry.UserID, entry.TaskID, entry.EventDate, startTime)
	if err != nil {
		bd.log.Info("error saving task tracking to database: ", zap.Error(err))
		return err
	}

	bd.log.Info("Task tracking started successfully for user: ", zap.Int("userID", entry.UserID), zap.Int("taskID", entry.TaskID))
	return nil
}

func (bd *BDKeeper) StopTaskTracking(ctx context.Context, entry models.TimeEntry) error {
	// Convert time taking into account the user's time zone
	location, err := time.LoadLocation(entry.UserTimezone)
	if err != nil {
		bd.log.Info("error loading user timezone: ", zap.Error(err))
		return err
	}
	endTime := time.Now().In(location)

	tx, err := bd.pool.Begin(ctx)
	if err != nil {
		bd.log.Info("Error while beginning transaction: ", zap.Error(err))
		return err
	}

	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				bd.log.Info("Error during transaction rollback: ", zap.Error(rollbackErr))
			} else {
				bd.log.Info("Transaction rolled back: ", zap.Error(err))
			}
		} else {
			err = tx.Commit(ctx)
			if err != nil {
				bd.log.Info("Error during transaction commit: ", zap.Error(err))
			} else {
				bd.log.Info("Transaction committed successfully")
			}
		}
	}()

	// Check for an active entry for the user and task on the specified date
	var id int
	query := `
        SELECT id FROM user_tasks
        WHERE user_id = $1 AND task_id = $2 AND event_date = $3 AND end_time IS NULL
    `
	rows, err := tx.Query(ctx, query, entry.UserID, entry.TaskID, entry.EventDate)
	if err != nil {
		bd.log.Info("error checking existing task in database: ", zap.Error(err))
		return err
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		err = rows.Scan(&id)
		if err != nil {
			bd.log.Info("error scanning task id: ", zap.Error(err))
			return err
		}
		found = true
	}

	if !found {
		return fmt.Errorf("no active task tracking found for user %d on task %d for date %s", entry.UserID, entry.TaskID, entry.EventDate)
	}

	// Update the entry with the end time
	updateQuery := `
        UPDATE user_tasks
        SET end_time = $1
        WHERE id = $2
    `
	_, err = tx.Exec(ctx, updateQuery, endTime, id)
	if err != nil {
		bd.log.Info("error updating task tracking in database: ", zap.Error(err))
		return err
	}

	bd.log.Info("Task tracking stopped successfully for user: ", zap.Int("userID", entry.UserID), zap.Int("taskID", entry.TaskID))
	return nil
}

func (bd *BDKeeper) GetUserTaskSummary(ctx context.Context, userID int, startDate, endDate time.Time, userTimezone string, defaultEndTime time.Time) ([]models.TaskSummary, error) {
	location, err := time.LoadLocation(userTimezone)
	if err != nil {
		bd.log.Info("error loading user timezone: ", zap.Error(err))
		return nil, err
	}

	query := `
        SELECT task_id, start_time, end_time
        FROM user_tasks
        WHERE user_id = $1 AND event_date BETWEEN $2 AND $3
    `
	rows, err := bd.pool.Query(ctx, query, userID, startDate, endDate)
	if err != nil {
		bd.log.Info("error querying task summary: ", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	taskTimeMap := make(map[int]time.Duration)
	for rows.Next() {
		var taskID int
		var startTime, endTime time.Time
		err = rows.Scan(&taskID, &startTime, &endTime)
		if err != nil {
			bd.log.Info("error scanning task summary: ", zap.Error(err))
			return nil, err
		}

		// If end_time is not filled, use default_end_time or the end of the current day
		if endTime.IsZero() {
			if !defaultEndTime.IsZero() {
				endTime = defaultEndTime
			} else {
				endTime = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 23, 59, 59, 0, location)
			}
		}

		duration := endTime.Sub(startTime)
		taskTimeMap[taskID] += duration
	}

	var taskSummaries []models.TaskSummary
	for taskID, totalTime := range taskTimeMap {
		taskSummaries = append(taskSummaries, models.TaskSummary{
			TaskID:    taskID,
			TotalTime: time.Duration.String(totalTime),
		})
	}

	// Sort by descending time
	sort.Slice(taskSummaries, func(i, j int) bool {
		return taskSummaries[i].TotalTime > taskSummaries[j].TotalTime
	})

	return taskSummaries, nil
}

func (kp *BDKeeper) Ping(ctx context.Context) bool {
	// Create a child context with a timeout from the passed context
	ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond) // Increased time to 1 millisecond as 1 microsecond is too short
	defer cancel()

	if err := kp.pool.Ping(ctx); err != nil {
		return false
	}

	return true
}
