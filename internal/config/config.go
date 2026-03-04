package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

const (
	configPath = "./config/local.yaml"
)

// Config App
type Config struct {
	Env      string        `yaml:"env" env:"ENV" env-default:"development"`
	TokenTTL time.Duration `yaml:"token_ttl" env:"TOKEN_TTL"`
	Server   HTTPServer    `yaml:"http_server" env:"HTTP_SERVER"`
	Storage  Storage       `yaml:"postgres" env:"STORAGE"`
	Cache    Cache         `yaml:"cache" env:"CACHE"`
}

// HTTPServer Config
type HTTPServer struct {
	Address     string        `yaml:"address" env:"HTTP_ADDRESS" env-default:"0.0.0.0:8080"`
	Timeout     time.Duration `yaml:"timeout" env:"HTTP_TIMEOUT" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env:"HTTP_IDLE_TIMEOUT" env-default:"60s"`
}

// Storage Config
type Storage struct {
	Host     string `yaml:"host" env:"DB_HOST" env-required:"true"`
	Port     string `yaml:"port" env:"DB_PORT" env-required:"true"`
	Username string `yaml:"username" env:"DB_USER" env-required:"true"`
	Password string `yaml:"password" env:"DB_PASSWORD" env-required:"true"`
	Database string `yaml:"database" env:"DB_NAME" env-required:"true"`

	MaxOpenConnection int           `yaml:"max_open_connection" env:"DB_MAX_OPEN_CONNECTION" env-default:"25"`
	MaxIdleConnection int           `yaml:"max_idle_connection" env:"DB_MAX_IDLE_CONNECTION" env-default:"25"`
	ConnMaxLifetime   time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME" env-default:"5m"`
}

type Cache struct {
	Addr     string `yaml:"addr" env:"CACHE_ADDR" env-required:"true"`
	Password string `yaml:"password" env:"CACHE_PASSWORD" env-required:"true"`
	Database int    `yaml:"database" env:"CACHE_DB" env-required:"true"`
}

// MustLoad - loads the configuration
func MustLoad() *Config {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file does not exist: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s", err)
	}

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		log.Fatalf("cannot read env: %s", err)
	}

	return &cfg
}

// GetDBConnString - get database connection string
func (s *Storage) GetDBConnString() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		s.Host, s.Port, s.Username, s.Password, s.Database)
}
