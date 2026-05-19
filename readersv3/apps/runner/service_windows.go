//go:build windows

package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/svc"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/runtime"
)

func installService(cfg *config.Config) error {
	info, err := buildServiceInstallInfo(cfg)
	if err != nil {
		return err
	}
	binPath := fmt.Sprintf(`"%s" -config "%s"`, info.Executable, info.ConfigPath)
	exists := exec.Command("sc.exe", "query", info.ServiceName).Run() == nil
	if exists {
		for _, args := range [][]string{
			{"sc.exe", "stop", info.ServiceName},
			{"sc.exe", "config", info.ServiceName, "binPath=", binPath, "start=", "auto", "DisplayName=", info.DisplayName},
		} {
			_ = exec.Command(args[0], args[1:]...).Run()
		}
	} else {
		cmd := exec.Command("sc.exe", "create", info.ServiceName, "binPath=", binPath, "start=", "auto", "DisplayName=", info.DisplayName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("create service failed: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	for _, args := range [][]string{
		{"sc.exe", "description", info.ServiceName, info.Description},
		{"sc.exe", "failure", info.ServiceName, "reset=", "86400", "actions=", "restart/60000/restart/120000/restart/300000"},
		{"sc.exe", "start", info.ServiceName},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil && !ignorableServiceError(args, string(output)) {
			return fmt.Errorf("%s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	fmt.Printf("Serviciu instalat cu succes: %s\n", info.ServiceName)
	fmt.Printf("Pentru a-l porni: sc start %s\n", info.ServiceName)
	fmt.Printf("Pentru a-l opri: sc stop %s\n", info.ServiceName)
	fmt.Printf("Pentru restart: sc stop %s && sc start %s\n", info.ServiceName, info.ServiceName)
	fmt.Printf("Status: sc query %s\n", info.ServiceName)
	return nil
}

func ignorableServiceError(args []string, output string) bool {
	joined := strings.Join(args, " ")
	text := strings.ToLower(output)
	if strings.Contains(joined, "sc.exe start") && strings.Contains(text, "service has already been started") {
		return true
	}
	if strings.Contains(joined, "sc.exe stop") && strings.Contains(text, "service has not been started") {
		return true
	}
	return false
}

func runServiceManager(cfg *config.Config, app *runtime.App) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, err
	}
	if !isService {
		return false, nil
	}
	info, err := buildServiceInstallInfo(cfg)
	if err != nil {
		return true, err
	}
	return true, svc.Run(info.ServiceName, &windowsService{app: app})
}

type windowsService struct {
	app *runtime.App
}

func (s *windowsService) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.app.Start(ctx)
	}()
	status <- svc.Status{State: svc.Running, Accepts: accepted}
	for {
		select {
		case change := <-req:
			switch change.Cmd {
			case svc.Interrogate:
				status <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				<-errCh
				return false, 0
			default:
			}
		case err := <-errCh:
			if err != nil {
				return false, 1
			}
			return false, 0
		}
	}
}
