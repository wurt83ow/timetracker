package bdkeeper

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
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

func (bd *BDKeeper) SaveUser(key string, user models.User) error {
	query := `
		INSERT INTO User (
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
		UPDATE User SET
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
		WHERE User.passportSerie = updated.passportSerie
		AND User.passportNumber = updated.passportNumber
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

func (bd *BDKeeper) UpdateUser(user models.User) error {
	query := `
		UPDATE User SET
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
		User`

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
		var m models.User

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
		DELETE FROM User
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
		public.User
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

func (bd *BDKeeper) StartTaskTracking(entry models.TimeEntry) error {
	// Преобразование времени с учетом часового пояса пользователя
	location, err := time.LoadLocation(entry.UserTimezone)
	if err != nil {
		bd.log.Info("error loading user timezone: ", zap.Error(err))
		return err
	}
	startTime := time.Now().In(location)

	// Проверка наличия активной записи для пользователя и задачи на указанную дату
	var existingTaskID int
	query := `
		SELECT id FROM user_tasks
		WHERE user_id = $1 AND task_id = $2 AND event_date = $3 AND end_time IS NULL
	`
	err = bd.conn.QueryRow(query, entry.UserID, entry.TaskID, entry.EventDate).Scan(&existingTaskID)
	if err != nil && err != sql.ErrNoRows {
		bd.log.Info("error checking existing task in database: ", zap.Error(err))
		return err
	}

	if existingTaskID != 0 {
		return fmt.Errorf("task tracking is already in progress for user %d on task %d for date %s", entry.UserID, entry.TaskID, entry.EventDate)
	}

	// Вставка новой записи в таблицу user_tasks
	insertQuery := `
		INSERT INTO user_tasks (user_id, task_id, event_date, start_time)
		VALUES ($1, $2, $3, $4)
	`
	_, err = bd.conn.Exec(insertQuery, entry.UserID, entry.TaskID, entry.EventDate, startTime)
	if err != nil {
		bd.log.Info("error saving task tracking to database: ", zap.Error(err))
		return err
	}

	bd.log.Info("Task tracking started successfully for user: ", zap.Int("userID", entry.UserID), zap.Int("taskID", entry.TaskID))
	return nil
}

func (bd *BDKeeper) StopTaskTracking(entry models.TimeEntry) error {
	// Преобразование времени с учетом часового пояса пользователя
	location, err := time.LoadLocation(entry.UserTimezone)
	if err != nil {
		bd.log.Info("error loading user timezone: ", zap.Error(err))
		return err
	}
	endTime := time.Now().In(location)

	// Проверка наличия активной записи для пользователя и задачи на указанную дату
	var id int
	query := `
        SELECT id FROM user_tasks
        WHERE user_id = $1 AND task_id = $2 AND event_date = $3 AND end_time IS NULL
    `
	rows, err := bd.conn.Query(query, entry.UserID, entry.TaskID, entry.EventDate)
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

	// Обновление записи с завершением времени
	updateQuery := `
        UPDATE user_tasks
        SET end_time = $1
        WHERE id = $2
    `
	_, err = bd.conn.Exec(updateQuery, endTime, id)
	if err != nil {
		bd.log.Info("error updating task tracking in database: ", zap.Error(err))
		return err
	}

	bd.log.Info("Task tracking stopped successfully for user: ", zap.Int("userID", entry.UserID), zap.Int("taskID", entry.TaskID))
	return nil
}

func (bd *BDKeeper) GetUserTaskSummary(userID int, startDate, endDate time.Time, userTimezone string, defaultEndTime time.Time) ([]models.TaskSummary, error) {
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
	rows, err := bd.conn.Query(query, userID, startDate, endDate)
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

		// Если end_time не заполнено, использовать default_end_time или конец текущего дня
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
			TotalTime: totalTime,
		})
	}

	// Сортировка по убыванию времени
	sort.Slice(taskSummaries, func(i, j int) bool {
		return taskSummaries[i].TotalTime > taskSummaries[j].TotalTime
	})

	return taskSummaries, nil
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
