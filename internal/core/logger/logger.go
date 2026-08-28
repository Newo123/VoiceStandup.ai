package corelog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Config содержит параметры настройки логирования
type Config struct {
	FilePath     string     // Путь к файлу (если пусто, в файл писать не будет)
	LogToConsole bool       // Нужно ли дублировать в консоль
	Level        slog.Level // Уровень логирования (slog.LevelDebug, slog.LevelInfo и т.д.)
}

// Init запускает и настраивает глобальный slog.
// Возвращает функцию для закрытия файла, которую нужно вызвать через defer в main.
func initLogger(cfg Config) (func(), error) {
	var writers []io.Writer
	var file *os.File
	var err error

	// Если указан путь к файлу, открываем его
	if cfg.FilePath != "" {
		file, err = os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("ошибка открытия файла логов: %w", err)
		}
		writers = append(writers, file)
	}

	// Если включен вывод в консоль или если файл вообще не был указан
	if cfg.LogToConsole || len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	// Объединяем все потоки в один io.Writer
	combinedWriter := io.MultiWriter(writers...)

	// Настраиваем slog handler
	opts := &slog.HandlerOptions{Level: cfg.Level}
	logger := slog.New(slog.NewJSONHandler(combinedWriter, opts))
	slog.SetDefault(logger)

	// Возвращаем функцию замыкания для безопасного закрытия файла в main
	return func() {
		if file != nil {
			_ = file.Close()
		}
	}, nil
}

func SetupLogger(env string) (func(), error) {
	cfg := Config{
		FilePath:     "./out/app.log",
		LogToConsole: true,
		Level:        slog.LevelDebug,
	}

	// Получаем функцию очистки ресурсов и откладываем её выполнение
	cleanup, err := initLogger(cfg)
	if err != nil {
		return nil, err
	}

	slog.Info("Приложение успешно запущено", slog.String("env", env))

	return cleanup, nil
}
