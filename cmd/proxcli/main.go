package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"proxcli/internal/bootstrap"
	"proxcli/internal/hub"
	"runtime"
	"syscall"
	"time"
)

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

var version = "dev"

func main() {
	if err := bootstrap.LoadEnv(".env"); err != nil {
		log.Fatal("Не удалось загрузить .env")
	}
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "version", "--version", "-v":
		fmt.Println("ProxCli " + version)
		return
	case "init":
		if err := bootstrap.Init("."); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Созданы .env и .data/cliproxyapi.yaml. Ключи не выводятся.")
		return
	case "hub":
		binary, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		name := "cli-proxy-api"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		child := exec.Command(filepath.Join(filepath.Dir(binary), name), "-config", env("PROXCLI_CORE_CONFIG", ".data/cliproxyapi.yaml"))
		child.Stdin = os.Stdin
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err = child.Run(); err != nil {
			log.Fatal("Ядро остановилось с ошибкой; проверьте бинарь и конфиг")
		}
		return
	case "serve":
	default:
		fmt.Fprintln(os.Stderr, "Использование: proxcli [init|serve|hub|version]")
		os.Exit(2)
	}

	addr := env("PROXCLI_LISTEN", "127.0.0.1:8787")
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Fatal("Неверный PROXCLI_LISTEN")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		log.Fatal("PROXCLI_LISTEN должен использовать loopback IP; для удалённого доступа используйте SSH tunnel")
	}
	app, err := hub.New(hub.Config{Upstream: env("PROXCLI_UPSTREAM", "http://127.0.0.1:8317"), ManagementKey: os.Getenv("PROXCLI_MANAGEMENT_KEY"), AccessKey: os.Getenv("PROXCLI_ACCESS_KEY"), DataDir: env("PROXCLI_DATA_DIR", ".data"), WebDir: env("PROXCLI_WEB_DIR", "web/dist"), ManageRouting: env("PROXCLI_MANAGE_ROUTING", "true") == "true"})
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.RunTelemetry(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("ProxCli: http://%s", addr)
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
