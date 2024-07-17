package config

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Options struct {
	flagRunAddr, flagLogLevel, flagDataBaseDSN,
	flagJWTSigningKey, flagConcurrency, flagTaskExecutionInterval,
	flagUserUpdateInterval string
}

func NewOptions() *Options {
	return new(Options)
}

// parseFlags handles command line arguments
// and stores their values in the corresponding variables.
func (o *Options) ParseFlags() {
	// Загрузка переменных окружения из файла .env
	loadEnvFile()

	// Переопределение значений переменных значениями из флагов командной строки
	regStringVar(&o.flagRunAddr, "a", getEnvOrDefault("RUN_ADDRESS", ":8080"), "address and port to run server")
	regStringVar(&o.flagConcurrency, "c", getEnvOrDefault("CONCURRENCY", "5"), "Concurrency")
	regStringVar(&o.flagDataBaseDSN, "d", getEnvOrDefault("DATABASE_URI", ""), "")
	regStringVar(&o.flagTaskExecutionInterval, "i", getEnvOrDefault("TASK_EXECUTION_INTERVAL", "3000"), "Task execution interval in milliseconds")
	regStringVar(&o.flagJWTSigningKey, "j", getEnvOrDefault("JWT_SIGNING_KEY", "test_key"), "jwt signing key")
	regStringVar(&o.flagLogLevel, "l", getEnvOrDefault("LOG_LEVEL", "debug"), "log level")
	regStringVar(&o.flagUserUpdateInterval, "u", getEnvOrDefault("USER_UPDATE_INTERVAL", "1d"), "user update interval")

	// parse the arguments passed to the server into registered variables
	flag.Parse()
}

func (o *Options) RunAddr() string {
	return o.flagRunAddr
}

func (o *Options) LogLevel() string {
	return o.flagLogLevel
}

func (o *Options) DataBaseDSN() string {
	return o.flagDataBaseDSN
}

func (o *Options) JWTSigningKey() string {
	return o.flagJWTSigningKey
}

func (o *Options) Concurrency() string {
	return o.flagConcurrency
}

func (o *Options) TaskExecutionInterval() string {
	return o.flagTaskExecutionInterval
}

func (o *Options) UserUpdateInterval() string {
	return o.flagUserUpdateInterval
}

func regStringVar(p *string, name string, value string, usage string) {
	if flag.Lookup(name) == nil {
		flag.StringVar(p, name, value, usage)
	}
}

// getEnvOrDefault reads an environment variable or returns a default value if the variable is not set or is empty.
func getEnvOrDefault(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return defaultValue
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile() {
	// Определение пути к .env файлу относительно текущего рабочего каталога
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	envPath := filepath.Join(cwd, "..", "..", ".env")

	// Загрузка переменных окружения из .env файла
	err = godotenv.Load(envPath)
	if err != nil {
		log.Printf("No .env file found at %s, proceeding without it", envPath)
	} else {
		log.Printf(".env file loaded from %s", envPath)
	}
}

// GetAsString reads an environment or returns a default value.
func GetAsString(key string, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultValue
}
