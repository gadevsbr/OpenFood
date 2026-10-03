package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"openfood/internal/app"
	local "openfood/internal/runtime"
)

func openURL(url string) {
	cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	local.Hide(cmd)
	cmd.Start()
}
func fatal(code string) {
	exitCode = 1
	if os.Getenv("OPENFOOD_TEST_HEADLESS") == "1" {
		return
	}
	p, _ := windows.UTF16PtrFromString("OpenFood não iniciou: " + code + ". Consulte operations.log na pasta de dados do OpenFood.")
	title, _ := windows.UTF16PtrFromString("OpenFood")
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptrPointer(p), uintptrPointer(title), 0x10)
}

var exitCode int

func main() {
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()
	data := os.Getenv("OPENFOOD_DATA_DIR")
	if data == "" {
		data = filepath.Join(os.Getenv("LOCALAPPDATA"), "OpenFood")
	}
	exe, e := os.Executable()
	if e != nil {
		return
	}
	bin := filepath.Join(filepath.Dir(exe), "postgresql", "bin")
	mutexName, _ := windows.UTF16PtrFromString("Local\\OpenFood-" + app.Hash(strings.ToLower(data))[:24])
	mutex, e := windows.CreateMutex(nil, false, mutexName)
	if errors.Is(e, windows.ERROR_ALREADY_EXISTS) {
		if len(os.Args) > 1 && os.Args[1] == "--prepare-update" {
			os.Exit(3)
		}
		raw, _ := os.ReadFile(filepath.Join(data, "runtime.json"))
		var state struct{ URL string }
		if json.Unmarshal(raw, &state) == nil && strings.HasPrefix(state.URL, "http://127.0.0.1:") {
			openURL(state.URL)
		}
		if mutex != 0 {
			windows.CloseHandle(mutex)
		}
		return
	}
	if e != nil {
		fatal("instance_lock_failed")
		return
	}
	defer windows.CloseHandle(mutex)
	r, e := local.Load(data, bin)
	if e != nil {
		fatal("local_configuration_failed")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer local.ReleaseToolPath(r.Bin)
	defer cancel()
	boot, stopBoot := context.WithTimeout(ctx, 90*time.Second)
	e = r.Start(boot)
	stopBoot()
	if e != nil {
		r.Code("startup_failed")
		fatal(e.Error())
		return
	}
	defer func() { stop, c := context.WithTimeout(context.Background(), 40*time.Second); defer c(); r.Stop(stop) }()
	if len(os.Args) > 1 && os.Args[1] == "--prepare-update" {
		if _, e = r.Backup(ctx); e != nil {
			fatal("pre_update_backup_failed")
			return
		}
		r.Code("update_prepared")
		return
	}
	a, e := app.Open(ctx, r.DSN, r.Config.SetupToken, r)
	if e != nil {
		fatal("migration_or_database_failed")
		return
	}
	defer a.DB.Close()
	a.OnShutdown = systray.Quit
	l, e := net.Listen("tcp", "127.0.0.1:18880")
	if e != nil {
		l, e = net.Listen("tcp", "127.0.0.1:0")
	}
	if e != nil {
		fatal("http_port_unavailable")
		return
	}
	defer l.Close()
	url := "http://" + l.Addr().String()
	raw, _ := json.Marshal(map[string]string{"URL": url})
	os.WriteFile(filepath.Join(data, "runtime.json"), raw, 0600)
	defer os.Remove(filepath.Join(data, "runtime.json"))
	server := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); a.Worker(ctx) }()
	go server.Serve(l)
	r.Code("application_ready")
	var count int
	a.DB.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count)
	firstURL := url
	if count == 0 {
		firstURL += "/#setup=" + r.Config.SetupToken
	}
	if !(len(os.Args) > 1 && os.Args[1] == "--background") {
		openURL(firstURL)
	}
	systray.Run(func() {
		systray.SetIcon(icon())
		systray.SetTitle("OpenFood")
		systray.SetTooltip("OpenFood pronto — somente local")
		open := systray.AddMenuItem("Abrir painel", "Abrir OpenFood no navegador")
		status := systray.AddMenuItem("Consultar estado", "Saúde e diagnóstico no painel")
		auto := systray.AddMenuItemCheckbox("Iniciar com o Windows", "Inicializar ao entrar nesta conta", autoEnabled())
		keep := systray.AddMenuItemCheckbox("Continuar com navegador fechado", "A operação permanece ativa até encerrar pela bandeja", true)
		keep.Disable()
		systray.AddSeparator()
		quit := systray.AddMenuItem("Encerrar OpenFood", "Encerrar aplicação e banco com segurança")
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					openURL(firstURL)
				case <-status.ClickedCh:
					openURL(url)
				case <-auto.ClickedCh:
					if autoEnabled() {
						setAuto(exe, false)
						auto.Uncheck()
					} else {
						if setAuto(exe, true) == nil {
							auto.Check()
						}
					}
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {
		cancel()
		stop, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		server.Shutdown(stop)
		wg.Wait()
		r.Code("application_stopped")
	})
}
func autoEnabled() bool {
	key, e := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if e != nil {
		return false
	}
	defer key.Close()
	_, _, e = key.GetStringValue("OpenFood")
	return e == nil
}
func setAuto(exe string, enabled bool) error {
	key, _, e := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if e != nil {
		return e
	}
	defer key.Close()
	if !enabled {
		return key.DeleteValue("OpenFood")
	}
	return key.SetStringValue("OpenFood", `"`+exe+`" --background`)
}
