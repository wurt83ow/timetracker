package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/wurt83ow/timetracker/internal/app"
)

func main() {

    // Создание корневого контекста с возможностью отмены
    ctx, cancel := context.WithCancel(context.Background())

    // Создание канала для обработки сигналов
    signalCh := make(chan os.Signal, 1)
    signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

    // Запуск сервера
    server := app.NewServer(ctx)
    go func() {
        // Ожидание сигнала
        sig := <-signalCh
        log.Printf("Received signal: %+v", sig)

        // Завершение работы сервера
        server.Shutdown()

        // Отмена контекста
        cancel()

    }()

    // Запуск сервера
    server.Serve()
}
